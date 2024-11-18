package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	_ "github.com/joho/godotenv/autoload"
)

type VoiceState struct {
	loop_forever bool
	stop         bool
	volume       float64
}

var (
	ActiveVoiceConnections = make(map[string]*discordgo.VoiceConnection)
	VCMutex                = sync.RWMutex{}

	StatePerConnection = make(map[string]VoiceState)
	StateMutex         = sync.RWMutex{}

	GlobalSession *discordgo.Session
)

const COLOR_OK = 0xcba6f7
const COLOR_ERROR = 0xf38ba8

func main() {
	var err error

	log.Logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		log.Panic().Err(err).Msg("")
	}
	time.Local = loc

	s, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		log.Error().Msgf("Error creating Discord session: %v", err)
		return
	}

	GlobalSession = s

	s.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Info().Msgf("Logged in as: %v#%v", s.State.User.Username, s.State.User.Discriminator)
	})

	err = s.Open()
	if err != nil {
		log.Error().Msgf("Error opening connection to Discord: %v", err)
		return
	}

	addCommands(s)
	addComponents(s)

	// TODO: implement queueing
	// TODO: implement skipping
	// TODO: implement pausing
	// TODO: implement resuming
	// TODO: possibly add multitrack recording support
	// TODO: play in multiple servers at once

	log.Info().Msg("Bot is now running. Press CTRL-C to exit.")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	cleanup(s)
}

func cleanup(s *discordgo.Session) {
	for _, v := range ActiveVoiceConnections {
		v.Close()
	}
	s.Close()
}
