package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func getVideoTitle(link string) (string, *time.Duration, error) {
	service, err := youtube.NewService(context.Background(), option.WithAPIKey(os.Getenv("YOUTUBE_API_KEY")))
	if err != nil {
		log.Err(err).Msg("Error creating new YouTube client")
		return "", nil, err
	}

	// get the video ID from the URL
	uri, err := url.Parse(link)
	if err != nil {
		return "", nil, fmt.Errorf("error parsing URL: %v", err)
	}
	id := uri.Query().Get("v")

	// Make the API call to YouTube.
	call := service.Videos.List([]string{"snippet", "contentDetails"}).Hl("en").Id(id)
	response, err := call.Do()
	if err != nil {
		log.Err(err).Msg("Error making API call to YouTube")
		return "", nil, err
	}

	if len(response.Items) == 0 {
		return "", nil, fmt.Errorf("no video found with ID %v", link)
	}

	dur, err := time.ParseDuration(strings.ToLower(response.Items[0].ContentDetails.Duration[2:]))
	if err != nil {
		log.Warn().Err(err).Msg("Error parsing content duration")
		dur = time.Duration(0)
		// return "", nil, err
	}

	return response.Items[0].Snippet.Localized.Title, &dur, nil
}

func searchYouTube(query string) ([]*youtube.Video, error) {
	service, err := youtube.NewService(context.Background(), option.WithAPIKey(os.Getenv("YOUTUBE_API_KEY")))
	if err != nil {
		log.Err(err).Msg("Error creating new YouTube client")
		return nil, err
	}

	call := service.Search.List([]string{"id", "snippet"}).Q(query).Type("video").MaxResults(5)
	response, err := call.Do()
	if err != nil {
		log.Err(err).Msg("Error making API call to YouTube")
		return nil, err
	}

	// get localised titles
	var ids []string
	for _, item := range response.Items {
		ids = append(ids, item.Id.VideoId)
	}

	vcall := service.Videos.List([]string{"snippet", "contentDetails"}).Hl("en").Id(strings.Join(ids, ","))
	vresponse, err := vcall.Do()
	if err != nil {
		log.Err(err).Msg("Error making API call to YouTube")
		return nil, err
	}

	return vresponse.Items, nil
}
