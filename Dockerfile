# syntax=docker/dockerfile:1

FROM --platform=linux/arm64 golang:1.24-bookworm

# Install system dependencies and latest yt-dlp nightly (better EJS support) early to maximize build cache reuse.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates ffmpeg mpv pulseaudio-utils python3 curl nodejs npm apt-utils chromium && \
    curl -fsSL https://github.com/yt-dlp/yt-dlp-nightly-builds/releases/latest/download/yt-dlp -o /usr/local/bin/yt-dlp && \
    chmod +x /usr/local/bin/yt-dlp && \
    # Ensure node is discoverable by yt-dlp (some distros only provide nodejs).
    if [ -x /usr/bin/nodejs ] && [ ! -x /usr/bin/node ]; then ln -s /usr/bin/nodejs /usr/bin/node; fi && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Cache Go modules separately from source to avoid re-downloading on code changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /usr/local/bin/yt-rpi-player .

COPY config.example.yaml /app/config.example.yaml

# Point this to the host PulseAudio socket when running the container.
ENV PULSE_SERVER=unix:/tmp/pulse-socket

CMD ["yt-rpi-player", "-config", "/app/config.yaml", "--run-now"]
