package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"play": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		link := i.ApplicationCommandData().Options[0].StringValue()

		// if vc, ok := ActiveVoiceConnections[i.GuildID]; ok {
		// 	// TODO: queue songs
		// 	StateMutex.RLock()
		// 	state, ok := StatePerConnection[i.GuildID]
		// 	StateMutex.RUnlock()
		// 	if ok {
		// 		state.stop = true
		// 		StateMutex.Lock()
		// 		StatePerConnection[i.GuildID] = state
		// 		StateMutex.Unlock()
		// 	}

		// 	video_info, err := getVideoTitle(link)
		// 	if err != nil {
		// 		log.Error().Msgf("Error getting video title: %v", err)
		// 		return
		// 	}

		// 	log.Debug().Str("guild_id", vc.GuildID).Bool("already_connected", true).Msgf("Playing video: %v (%v)", video_info.Snippet.Title, video_info.ContentDetails.Duration)

		// 	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		// 		Type: discordgo.InteractionResponseChannelMessageWithSource,
		// 		Data: &discordgo.InteractionResponseData{
		// 			Embeds: []*discordgo.MessageEmbed{
		// 				{
		// 					Title:       "Now Playing", // TODO: enqueue songs
		// 					Description: fmt.Sprintf("[%v](%v)", video_info.Snippet.Title, fmt.Sprintf("https://www.youtube.com/watch?v=%v", video_info.Id)),
		// 					Color:       COLOR_OK,
		// 					Timestamp:   time.Now().Format(time.RFC3339),
		// 					// Image: &discordgo.MessageEmbedImage{
		// 					// 	URL: video_info.Snippet.Thumbnails.Maxres.Url,
		// 					// },
		// 				},
		// 			},
		// 		},
		// 	})
		// 	if err != nil {
		// 		log.Error().Msgf("Error responding to interaction: %v", err)
		// 	}

		// 	PlayYoutubeID(vc, link)
		// 	return
		// }

		if _, ok := ActiveVoiceConnections[i.GuildID]; ok {
			StateMutex.RLock()
			state, ok := StatePerConnection[i.GuildID]
			StateMutex.RUnlock()
			log.Debug().Msgf("State: %+v", state)
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

		//log.Debug().Msgf("User %v is in voice channel %v", i.Member.User.ID, state.ChannelID)

		dgv, err := s.ChannelVoiceJoin(i.GuildID, state.ChannelID, false, false)
		if err != nil {
			log.Error().Msgf("Error joining voice channel: %v", err)
			return
		}

		VCMutex.Lock()
		ActiveVoiceConnections[i.GuildID] = dgv
		VCMutex.Unlock()

		log.Debug().Msgf("Joined voice channel %v", state.ChannelID)
		// log.Debug().Msgf("%+v", dgv)

		video_info, err := getVideoTitle(link)
		if err != nil {
			log.Error().Msgf("Error getting video title: %v", err)
			return
		}

		log.Debug().Str("guild_id", dgv.GuildID).Bool("already_connected", false).Msgf("Playing video: %v (%v)", video_info.Snippet.Title, video_info.ContentDetails.Duration)

		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Embeds: []*discordgo.MessageEmbed{
					{
						Title:       "Now Playing", // TODO: enqueue songs
						Description: fmt.Sprintf("[%v](%v)", video_info.Snippet.Title, fmt.Sprintf("https://www.youtube.com/watch?v=%v", video_info.Id)),
						Color:       COLOR_OK,
						Timestamp:   time.Now().Format(time.RFC3339),
						// Image: &discordgo.MessageEmbedImage{
						// 	URL: video_info.Snippet.Thumbnails.Maxres.Url,
						// },
					},
				},
			},
		})
		if err != nil {
			log.Error().Msgf("Error responding to interaction: %v", err)
		}

		go PlayYoutubeID(dgv, link)
	},
	"stop": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
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

	},
	"loop": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	},
	"disconnect": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		VCMutex.RLock()
		vc, ok := ActiveVoiceConnections[i.GuildID]
		VCMutex.RUnlock()
		if ok {
			err := vc.Speaking(false)
			if err != nil {
				log.Error().Err(err).Msg("Couldn't stop speaking")
			}
			vc.Disconnect()
			delete(ActiveVoiceConnections, i.GuildID)
			err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Embeds: []*discordgo.MessageEmbed{
						{
							Title:       "Disconnected",
							Color:       COLOR_OK,
							Timestamp:   time.Now().Format(time.RFC3339),
						},
					},
					Flags: discordgo.MessageFlagsEphemeral,
				},
			})
			if err != nil {
				log.Error().Msgf("Error responding to interaction: %v", err)
			}
		}
	},
	"volume": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
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
	},
}

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "play",
		Description: "Play a song from YouTube",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "link",
				Description: "YouTube video link",
				Required:    true,
			},
		},
	},
	{
		Name:        "stop",
		Description: "Stop the current song",
	},
	{
		Name:        "loop",
		Description: "Toggle looping the current song (default: true)",
	},
	{
		Name:        "disconnect",
		Description: "Disconnect from the voice channel",
	},
	{
		Name:        "volume",
		Description: "Set the volume of the bot",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionNumber,
				Name:        "volume",
				Description: "Volume (0-100)",
				Required:    true,
			},
		},
	},
}

func addCommands(s *discordgo.Session) {
	s.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		switch i.Type {
		case discordgo.InteractionApplicationCommand:
			if h, ok := commandHandlers[i.ApplicationCommandData().Name]; ok {
				h(s, i)
			}
		}
	})
}

func addComponents(s *discordgo.Session) {
	registeredCommands := make([]*discordgo.ApplicationCommand, len(commands))
	var wg = sync.WaitGroup{}
	for i, v := range commands {
		wg.Add(1)
		go func(i int, v *discordgo.ApplicationCommand) {
			defer wg.Done()
			cmd, err := s.ApplicationCommandCreate(s.State.Application.ID, "", v)
			if err != nil {
				log.Panic().Msgf("Cannot create '%v' command: %v", v.Name, err)
			}
			registeredCommands[i] = cmd
		}(i, v)
	}
	wg.Wait()

	log.Info().Msgf("Registered %v commands", len(registeredCommands))
}
