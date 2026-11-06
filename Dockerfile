# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM golang:1.27.3@sha256:0c050feb47802ae436b9030833694a0b80160d08e3b3049ad30bf7c9e724e09d
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -o rittal-exporter

FROM alpine:3.24.4@sha256:93b41ef40b1677fce8fcc64ff4af09daddfacf5040afde618bc46c1e2d1cc7d4
COPY --from=0 /app/rittal-exporter /usr/local/bin/rittal-exporter