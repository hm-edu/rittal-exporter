# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM golang:1.27.3@sha256:9048d5e7751706200eb1719c7dae0436950d362f29a1da91fa05e5c63867c60f
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o rittal-exporter

FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a
COPY --from=0 /app/rittal-exporter /usr/local/bin/rittal-exporter