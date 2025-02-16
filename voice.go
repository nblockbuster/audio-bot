package main

import (
	"bufio"
	"encoding/binary"
	"io"
	"os/exec"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"layeh.com/gopus"
)

// TODO: Bot eventually leaves and rejoins the voice channel if it's playing in a different server as well

const (
	channels  int = 2                   // 1 for mono, 2 for stereo
	frameRate int = 48000               // audio sampling rate
	frameSize int = 960                 // uint16 size of each audio frame
	maxBytes  int = (frameSize * 2) * 2 // max size of opus data
)

var (
	queue = make(map[string][]string)
)

func SendPCM(v *discordgo.VoiceConnection, pcm <-chan []int16) {
	if pcm == nil {
		log.Error().Msg("PCM channel is nil")
		return
	}

	opusEncoder, err := gopus.NewEncoder(frameRate, channels, gopus.Audio)

	if err != nil {
		log.Error().Err(err).Msg("Error creating opus encoder")
		return
	}

	for {
		recv, ok := <-pcm
		if !ok {
			log.Info().Msg("PCM channel closed")
			err := v.Speaking(false)
			if err != nil {
				log.Error().Err(err).Msg("Couldn't stop speaking")
			}
			log.Info().Msg("PCM channel closed")
			return
		}
		StateMutex.RLock()
		volume := StatePerConnection[v.GuildID].volume
		StateMutex.RUnlock()
		recv = SetGain(recv, volume)

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
	log.Debug().Msg("killing processes")
	if ytdlp != nil && ytdlp.Process != nil {
		log.Debug().Msgf("killing ytdlp %v", ytdlp.Process.Pid)
		ytdlp.Process.Kill()
		ytdlp.Process.Release()
	}
	if ffmpeg != nil && ffmpeg.Process != nil {
		log.Debug().Msgf("killing ffmpeg %v", ffmpeg.Process.Pid)
		ffmpeg.Process.Kill()
		ffmpeg.Process.Release()
	}
}

func PlayYoutubeID(v *discordgo.VoiceConnection, play_id string) {

	StateMutex.Lock()
	s, ok := StatePerConnection[v.GuildID]
	if ok {
		s.stop = false
		StatePerConnection[v.GuildID] = s
	}
	StateMutex.Unlock()

	var is_voice_empty bool
	var time_voice_empty time.Time

	var ytdlp *exec.Cmd
	var ffmpeg *exec.Cmd
	var ytpipe io.ReadCloser
	var ffmpegout io.ReadCloser
	var ffmpegin io.WriteCloser

	startProcesses := func(id string) error {
		var err error

		ytdlp = exec.Command("yt-dlp", id, "-o", "-")
		ytpipe, err = ytdlp.StdoutPipe()
		if err != nil {
			return err
		}

		ffmpeg = exec.Command("ffmpeg", "-i", "pipe:", "-map", "0", "-map", "-0:v", "-f", "s16le", "-ar", strconv.Itoa(frameRate), "-ac", strconv.Itoa(channels), "pipe:1")
		ffmpegout, err = ffmpeg.StdoutPipe()
		if err != nil {
			return err
		}
		ffmpegin, err = ffmpeg.StdinPipe()
		if err != nil {
			return err
		}

		if err := ytdlp.Start(); err != nil {
			return err
		}
		if err := ffmpeg.Start(); err != nil {
			terminateProcesses(ytdlp, ffmpeg)
			return err
		}
		return nil
	}

	err := startProcesses(play_id)
	if err != nil {
		log.Error().Err(err).Msg("Error starting processes")
		return
	}

	exit_loop := false

	err = v.Speaking(true)
	if err != nil {
		log.Error().Err(err).Msg("Couldn't set speaking")
	}

	for {
		ffmpegbuf := bufio.NewReaderSize(ffmpegout, 65536)

		go func() {
			for {
				if exit_loop {
					break
				}
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

		go func() {
			for {
				if exit_loop {
					break
				}
				StateMutex.Lock()
				state, ok := StatePerConnection[v.GuildID]
				if ok && state.stop {
					log.Debug().Msg("state stop")
					terminateProcesses(ytdlp, ffmpeg)
					exit_loop = true
					StatePerConnection[v.GuildID] = state
				}
				StateMutex.Unlock()
			}
		}()

		go func() {
			for {
				if exit_loop {
					return
				}

				StateMutex.RLock()
				state, ok := StatePerConnection[v.GuildID]
				StateMutex.RUnlock()

				if ok && state.loop_forever {
					for {
						err := ffmpeg.Wait()
						if err != nil {
							log.Warn().Err(err).Msg("ffmpeg exited with error")
						}
						err = v.Speaking(false)
						if err != nil {
							log.Warn().Err(err).Msg("Couldn't stop speaking")
						}
						if exit_loop || state.stop {
							break
						}
						err = startProcesses(play_id)
						if err != nil {
							log.Error().Err(err).Msg("Error restarting processes")
							return
						}
					}
				} else {
					err := ffmpeg.Wait()
					if err != nil {
						log.Warn().Err(err).Msg("ffmpeg exited with error")
					}
					err = v.Speaking(false)
					if err != nil {
						log.Warn().Err(err).Msg("Couldn't stop speaking")
					}
				}
			}
		}()

		go func() {
			for {
				if is_voice_empty && time.Since(time_voice_empty) > 10*time.Minute {
					exit_loop = true
					err := v.Disconnect()
					log.Err(err)
					v.Close()
					return
				}
				if !v.Ready {
					continue
				}
				g, err := GlobalSession.State.Guild(v.GuildID)
				if err != nil {
					log.Warn().Err(err)
					continue
				}
				var cur_channel_members int
				if len(g.VoiceStates) == 0 {
					continue
				}
				for _, s := range g.VoiceStates {
					if s == nil || v == nil || s.Member == nil || s.Member.User == nil {
						continue
					}
					if s.ChannelID == v.ChannelID && s.Member.User.ID != GlobalSession.State.User.ID {
						cur_channel_members += 1
					}
				}

				if cur_channel_members == 0 && !is_voice_empty {
					time_voice_empty = time.Now()
					is_voice_empty = true
				}

				time.Sleep(10 * time.Second)

			}
		}()

		time.Sleep(1 * time.Second)

		// Main playback loop
		for {
			if exit_loop {
				log.Debug().Msg("exitloop")
				exit_loop = false

				return
			}
			audiobuf := make([]int16, frameSize*channels)
			err = binary.Read(ffmpegbuf, binary.LittleEndian, &audiobuf)
			if err != nil {
				log.Error().Err(err).Msg("Error reading from ffmpeg stdout")
				break
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
