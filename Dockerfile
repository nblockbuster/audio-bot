# syntax=docker/dockerfile:1

FROM golang:1.23-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./

RUN apk add tar
RUN apk add build-base
RUN apk add yt-dlp

RUN CGO_ENABLED=1 GOOS=linux go build -o /audio-bot
CMD ["/audio-bot"]