package main

import (
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

var commandHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"play": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		link := i.ApplicationCommandData().Options[0].StringValue()
		playCommand(s, i, link)
	},
	"stop": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		stopCommand(s, i)
	},
	"loop": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		loopCommand(s, i)
	},
	"disconnect": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		disconnectCommand(s, i)
	},
	"volume": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		volumeCommand(s, i)
	},
}

var componentHandlers = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
	"select_searched": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		playSearchedTrack(s, i)
	},
}

var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "play",
		Description: "Play a song from YouTube",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "query",
				Description: "YouTube video link or search query",
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
		case discordgo.InteractionMessageComponent:
			if h, ok := componentHandlers[i.MessageComponentData().CustomID]; ok {
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
