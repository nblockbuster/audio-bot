package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"

	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

func getVideoTitle(link string) (*youtube.Video, error) {

	service, err := youtube.NewService(context.Background(), option.WithAPIKey(os.Getenv("YOUTUBE_API_KEY")))
	if err != nil {
		log.Fatalf("Error creating new YouTube client: %v", err)
	}

	// get the video ID from the URL
	uri, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("error parsing URL: %v", err)
	}
	id := uri.Query().Get("v")

	// Make the API call to YouTube.
	call := service.Videos.List([]string{"snippet", "contentDetails"}).Id(id)
	response, err := call.Do()
	if err != nil {
		log.Fatalf("Error making API call to YouTube: %v", err)
	}

	if len(response.Items) == 0 {
		return nil, fmt.Errorf("no video found with ID %v", link)
	}

	return response.Items[0], nil
}
