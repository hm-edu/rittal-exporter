# syntax=docker/dockerfile:1

FROM golang:1.26.4
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o rittal-exporter

FROM alpine:3.24.0@sha256:8ddefa941e689fc29abcdeb8dae3b3c6d139cc08ce9a52633931160701770685
COPY --from=0 /app/rittal-exporter /usr/local/bin/rittal-exporter