package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"os/exec"
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"layeh.com/gopus"
)

const (
	channels  int = 2                   // 1 for mono, 2 for stereo
	frameRate int = 48000               // audio sampling rate
	frameSize int = 960                 // uint16 size of each audio frame
	maxBytes  int = (frameSize * 2) * 2 // max size of opus data
)

func SendPCM(v *discordgo.VoiceConnection, pcm <-chan []int16) {
	if pcm == nil {
		log.Error().Msg("PCM channel is nil")
		return
	}

	var err error

	opusEncoder, err := gopus.NewEncoder(frameRate, channels, gopus.Audio)

	if err != nil {
		log.Error().Err(err).Msg("Error creating opus encoder")
		return
	}

	for {
		recv, ok := <-pcm
		if !ok {
			log.Info().Msg("PCM channel closed")
			return
		}
		StateMutex.RLock()
		volume := StatePerConnection[v.GuildID].volume
		StateMutex.RUnlock()
		recv = adjustVolume(recv, volume)

		opus, err := opusEncoder.Encode(recv, frameSize, maxBytes)
		if err != nil {
			log.Error().Err(err).Msg("Error encoding opus data")
			return
		}

		if !v.Ready || v.OpusSend == nil {
			log.Error().Msgf("Discordgo not ready for opus packets. %+v : %+v", v.Ready, v.OpusSend)
			return
		}
		v.OpusSend <- opus
	}
}

func terminateProcesses(ytdlp, ffmpeg *exec.Cmd) {
	if ytdlp != nil && ytdlp.Process != nil {
		ytdlp.Process.Kill()
	}
	if ffmpeg != nil && ffmpeg.Process != nil {
		ffmpeg.Process.Kill()
	}
}

func PlayYoutubeID(v *discordgo.VoiceConnection, id string) {
	startProcesses := func() (*exec.Cmd, *exec.Cmd, io.ReadCloser, io.ReadCloser, io.WriteCloser, error) {
		ytdlp := exec.Command("yt-dlp", id, "-o", "-")
		ytpipe, err := ytdlp.StdoutPipe()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}

		ffmpeg := exec.Command("ffmpeg", "-i", "pipe:", "-map", "0", "-map", "-0:v", "-f", "s16le", "-ar", strconv.Itoa(frameRate), "-ac", strconv.Itoa(channels), "pipe:1")
		ffmpegout, err := ffmpeg.StdoutPipe()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		ffmpegin, err := ffmpeg.StdinPipe()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}

		if err := ytdlp.Start(); err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if err := ffmpeg.Start(); err != nil {
			terminateProcesses(ytdlp, ffmpeg)
			return nil, nil, nil, nil, nil, err
		}
		return ytdlp, ffmpeg, ytpipe, ffmpegout, ffmpegin, nil
	}

	ytdlp, ffmpeg, ytpipe, ffmpegout, ffmpegin, err := startProcesses()
	if err != nil {
		log.Error().Err(err).Msg("Error starting processes")
		return
	}

	exit_loop := false

	err = v.Speaking(true)
	if err != nil {
		log.Error().Err(err).Msg("Couldn't set speaking")
	}

	defer func() {
		err := v.Speaking(false)
		if err != nil {
			log.Error().Err(err).Msg("Couldn't stop speaking")
		}
	}()

	for {
		ffmpegbuf := bufio.NewReaderSize(ffmpegout, 65536)

		go func() {
			for {
				buf := make([]byte, 65536)
				read, err := ytpipe.Read(buf)
				if read == 0 {
					ffmpegin.Close()
					break
				}
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Error().Err(err).Msg("Error peeking data from yt-dlp")
					break
				}
				_, err = ffmpegin.Write(buf[:read])
				if err != nil {
					log.Warn().Err(err).Msg("Error writing data to ffmpeg")
					break
				}
			}
		}()

		send := make(chan []int16, 2)

		closea := make(chan bool)
		go func() {
			SendPCM(v, send)
			closea <- true
		}()

		// /stop command
		go func() {
			for {
				StateMutex.RLock()
				state, ok := StatePerConnection[v.GuildID]
				StateMutex.RUnlock()
				if ok && state.stop {
					// log.Debug().Msg("stop")
					send = nil
					terminateProcesses(ytdlp, ffmpeg)
					exit_loop = true
					state.stop = false
					StateMutex.Lock()
					StatePerConnection[v.GuildID] = state
					StateMutex.Unlock()
				}
			}
		}()

		go func() {
			for {
				err := ffmpeg.Wait()
				if err != nil {
					log.Warn().Err(err).Msg("ffmpeg exited with error")
				}

				if exit_loop {
					return
				}

				StateMutex.RLock()
				state, ok := StatePerConnection[v.GuildID]
				StateMutex.RUnlock()

				if ok && state.loop_forever {
					terminateProcesses(ytdlp, ffmpeg)
					ytdlp, ffmpeg, ytpipe, ffmpegout, ffmpegin, err = startProcesses()
					if err != nil {
						log.Error().Err(err).Msg("Error restarting processes")
						return
					}
					ffmpegbuf = bufio.NewReaderSize(ffmpegout, 65536)
					go func() {
						for {
							buf := make([]byte, 65536)
							read, err := ytpipe.Read(buf)
							if read == 0 {
								ffmpegin.Close()
								break
							}
							if err != nil {
								if err == io.EOF {
									break
								}
								log.Error().Err(err).Msg("Error peeking data from yt-dlp")
								break
							}
							_, err = ffmpegin.Write(buf[:read])
							if err != nil {
								log.Warn().Err(err).Msg("Error writing data to ffmpeg")
								break
							}
						}
					}()
				} else {
					break
				}
			}
		}()

		// Main playback loop
		for {
			if exit_loop {
				return
			}
			audiobuf := make([]int16, frameSize*channels)
			err = binary.Read(ffmpegbuf, binary.LittleEndian, &audiobuf)
			if err != nil {
				if err.Error() != "read |0: file already closed" {
					log.Error().Err(err).Msg("Error reading from ffmpeg stdout")
				}
				terminateProcesses(ytdlp, ffmpeg)
				return
			}

			select {
			case send <- audiobuf:
			case <-closea:
				log.Debug().Msg("close")
				return
			}
		}
	}
}

func adjustVolume(s16_data []int16, amt float64) []int16 {
	for i := range s16_data {
		sample := float64(s16_data[i])
		sample *= amt
		if sample > 32767 {
			sample = 32767
		} else if sample < -32768 {
			sample = -32768
		}
		s16_data[i] = int16(sample)
	}
	return s16_data
}
