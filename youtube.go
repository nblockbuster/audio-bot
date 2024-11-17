package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func getVideoTitle(link string) (string, *time.Duration, error) {

	service, err := youtube.NewService(context.Background(), option.WithAPIKey(os.Getenv("YOUTUBE_API_KEY")))
	if err != nil {
		log.Err(err).Msg("Error creating new YouTube client")
	}

	// get the video ID from the URL
	uri, err := url.Parse(link)
	if err != nil {
		return "", nil, fmt.Errorf("error parsing URL: %v", err)
	}
	id := uri.Query().Get("v")

	// Make the API call to YouTube.
	call := service.Videos.List([]string{"snippet", "contentDetails"}).Id(id)
	response, err := call.Do()
	if err != nil {
		log.Err(err).Msg("Error making API call to YouTube")
	}

	if len(response.Items) == 0 {
		return "", nil, fmt.Errorf("no video found with ID %v", link)
	}

	dur, err := time.ParseDuration(response.Items[0].ContentDetails.Duration)
	if err != nil {
		log.Err(err).Msg("Error parsing content duration")
	}

	return response.Items[0].Snippet.Localized.Title, &dur, nil
}
