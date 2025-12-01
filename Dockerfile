# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.23-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/yt-rpi-player .

FROM --platform=linux/arm64 debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg yt-dlp pulseaudio-utils && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/yt-rpi-player /usr/local/bin/yt-rpi-player
COPY config.example.yaml /app/config.example.yaml

# Point this to the host PulseAudio socket when running the container.
ENV PULSE_SERVER=unix:/tmp/pulse-socket

ENTRYPOINT ["yt-rpi-player", "-config", "/app/config.yaml"]
