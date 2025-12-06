# syntax=docker/dockerfile:1

FROM --platform=linux/arm64 golang:1.24-bookworm AS builder
WORKDIR /src

COPY go.mod go.sum ./
#RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod go mod download
#RUN go mod download

RUN go get golang.org/x/text/language
RUN go get golang.org/x/text/message

COPY . .
#RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /out/yt-rpi-player .

FROM --platform=linux/arm64 debian:12-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg pulseaudio-utils python3 curl nodejs && \
    curl -fsSL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /out/yt-rpi-player /usr/local/bin/yt-rpi-player
COPY config.example.yaml /app/config.example.yaml

# Point this to the host PulseAudio socket when running the container.
ENV PULSE_SERVER=unix:/tmp/pulse-socket

ENTRYPOINT ["yt-rpi-player", "-config", "/app/config.yaml"]
