# syntax=docker/dockerfile:1

FROM golang:1.23.6
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o rittal-exporter

FROM alpine:3.21.2@sha256:56fa17d2a7e7f168a043a2712e63aed1f8543aeafdcee47c58dcffe38ed51099
COPY --from=0 /app/rittal-exporter /usr/local/bin/rittal-exporter