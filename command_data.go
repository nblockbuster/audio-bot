package main

import (
	"fmt"
	"html"
	"net/url"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

func volumeCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	StateMutex.RLock()
	state, ok := StatePerConnection[i.GuildID]
	StateMutex.RUnlock()
	if ok {
		volume := i.ApplicationCommandData().Options[0].FloatValue()
		if volume < 0 {
			volume = 0
		} else if volume > 100 {
			volume = 100
		}
		//log.Debug().Msgf("Setting volume to %v (%v)", volume, volume/100)
		state.volume = volume / 100
		StateMutex.Lock()
		StatePerConnection[i.GuildID] = state
		StateMutex.Unlock()

		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Volume",
						Description: fmt.Sprintf("Volume set to %v", volume),
						Color:       COLOR_OK,
						Timestamp:   time.Now().Format(time.RFC3339),
					},
				},
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}
	}
}

func disconnectCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	VCMutex.RLock()
	vc, ok := ActiveVoiceConnections[i.GuildID]
	VCMutex.RUnlock()
	if ok {
		err := vc.Speaking(false)
		if err != nil {
			log.Error().Err(err).Msg("Couldn't stop speaking")
		}
		vc.Disconnect()
		VCMutex.Lock()
		delete(ActiveVoiceConnections, i.GuildID)
		VCMutex.Unlock()
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:     "Disconnected",
						Color:     COLOR_OK,
						Timestamp: time.Now().Format(time.RFC3339),
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}
	}
}

func loopCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	StateMutex.RLock()
	vs, ok := StatePerConnection[i.GuildID]
	StateMutex.RUnlock()
	if ok {
		vs.loop_forever = !vs.loop_forever
		StateMutex.Lock()
		StatePerConnection[i.GuildID] = vs
		StateMutex.Unlock()
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "Looping",
					Description: fmt.Sprintf("Looping is now %v", vs.loop_forever),
					Color:       COLOR_OK,
					Timestamp:   time.Now().Format(time.RFC3339),
				},
			},
		},
	})
	if err != nil {
		log.Error().Msgf("Error responding to interaction: %v", err)
	}
}

func stopCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	StateMutex.RLock()
	state, ok := StatePerConnection[i.GuildID]
	StateMutex.RUnlock()
	if ok {
		state.stop = true
		StateMutex.Lock()
		StatePerConnection[i.GuildID] = state
		StateMutex.Unlock()
	}
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:     "Stopping...",
					Color:     COLOR_OK,
					Timestamp: time.Now().Format(time.RFC3339),
				},
			},
		},
	})
	if err != nil {
		log.Error().Msgf("Error responding to interaction: %v", err)
	}
}

func playCommand(s *discordgo.Session, i *discordgo.InteractionCreate, link string) {
	uri, err := url.Parse(link)
	if err != nil {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Error",
						Description: "Could not parse URL",
						Color:       COLOR_ERROR,
						Timestamp:   time.Now().Format(time.RFC3339),
					},
				},
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}
		return
	}

	if uri.Host != "www.youtube.com" && uri.Host != "youtube.com" {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Error",
						Description: "Only YouTube links are supported",
						Color:       COLOR_ERROR,
						Timestamp:   time.Now().Format(time.RFC3339),
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}
		return
	} else if uri.Query().Get("v") == "" {
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Error",
						Description: "Please provide a valid YouTube link",
						Color:       COLOR_ERROR,
						Timestamp:   time.Now().Format(time.RFC3339),
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}
		return
	}

	stripped_link := fmt.Sprintf("https://www.youtube.com/watch?v=%v", uri.Query().Get("v"))

	// TODO: bot needs to handle multiple voice connections in different guilds
	// this is a hack to prevent the bot from joining multiple voice channels
	if len(ActiveVoiceConnections) > 0 {
		for k := range ActiveVoiceConnections {
			if k != i.GuildID {
				err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Embeds: []*discordgo.MessageEmbed{
							{
								Title:       "Error",
								Description: "The bot is already playing music in another voice channel",
								Color:       COLOR_ERROR,
							},
						},
						Flags: discordgo.MessageFlagsEphemeral,
					},
				})
				if err != nil {
					log.Error().Msgf("Error responding to interaction: %v", err)
				}
				return
			}
		}
	}

	if _, ok := ActiveVoiceConnections[i.GuildID]; ok {
		StateMutex.RLock()
		state, ok := StatePerConnection[i.GuildID]
		StateMutex.RUnlock()
		// log.Debug().Msgf("State: %+v", state)
		if ok {
			state.stop = true
			StateMutex.Lock()
			StatePerConnection[i.GuildID] = state
			StateMutex.Unlock()
			log.Debug().Msg("Stopping current song")
		}
	}

	StateMutex.Lock()
	if _, ok := StatePerConnection[i.GuildID]; !ok {
		StatePerConnection[i.GuildID] = VoiceState{loop_forever: true, stop: false, volume: 1.0}
	}
	StateMutex.Unlock()

	state, err := s.State.VoiceState(i.GuildID, i.Member.User.ID)
	if err != nil {
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Error",
						Description: "You must be in a voice channel to use this command",
						Color:       COLOR_ERROR,
						Timestamp:   time.Now().Format(time.RFC3339),
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}

		return
	}

	dgv, err := s.ChannelVoiceJoin(i.GuildID, state.ChannelID, false, false)
	if err != nil {
		log.Error().Msgf("Error joining voice channel: %v", err)
		return
	}

	VCMutex.Lock()
	ActiveVoiceConnections[i.GuildID] = dgv
	VCMutex.Unlock()

	log.Debug().Msgf("Joined voice channel %v", state.ChannelID)

	title, dur, err := getVideoTitle(link)
	if err != nil {
		log.Error().Msgf("Error getting video title: %v", err)
		return
	}

	log.Debug().Str("guild_id", dgv.GuildID).Bool("already_connected", false).Msgf("Playing video: %v (%v)", title, dur)

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{
				{
					Title:       "Now Playing",
					Description: fmt.Sprintf("[%v](%v)", title, stripped_link),
					Color:       COLOR_OK,
					Timestamp:   time.Now().Format(time.RFC3339),
				},
			},
		},
	})
	if err != nil {
		log.Error().Msgf("Error responding to interaction: %v", err)
	}

	go PlayYoutubeID(dgv, stripped_link)
}

func searchCommand(s *discordgo.Session, i *discordgo.InteractionCreate) {
	query := i.ApplicationCommandData().Options[0].StringValue()
	results, err := searchYouTube(query)
	if err != nil {
		log.Error().Msgf("Error searching YouTube: %v", err)
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Error",
						Description: "Error searching YouTube",
						Color:       COLOR_ERROR,
						Timestamp:   time.Now().Format(time.RFC3339),
					},
				},
				Flags: discordgo.MessageFlagsEphemeral,
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}
		return
	}

	options := make([]discordgo.SelectMenuOption, 0)
	for _, v := range results {
		title := html.UnescapeString(v.Snippet.Localized.Title)
		if len(title) > 100 {
			title = title[:100]
		}

		options = append(options, discordgo.SelectMenuOption{
			Label: title,
			Value: v.Id,
		})
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						&discordgo.SelectMenu{
							Placeholder: "Select a video",
							CustomID:    "select_searched",
							Options:     options,
						},
					},
				},
			},
		},
	})
	if err != nil {
		log.Error().Msgf("Error responding to interaction: %v", err)
	}
}

func playSearchedTrack(s *discordgo.Session, i *discordgo.InteractionCreate) {
	link := fmt.Sprintf("https://www.youtube.com/watch?v=%v", i.MessageComponentData().Values[0])
	playCommand(s, i, link)
}
