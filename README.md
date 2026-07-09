# yt-daily-player

A small Go scheduler that searches YouTube daily feeds by keyword + date and plays audio on a headless Raspberry Pi 4B through PulseAudio. Jobs and player settings are stored in a YAML file.

## Features
- Cron-like scheduling (via robfig/cron) configured in YAML
- yt-dlp based search using `ytsearch` (keywords + current date)
- Streams audio only (default player: `ffplay` to PulseAudio)
- **MPD support**: can add streams directly to a local Music Player Daemon playlist
- Optional `-run-now` flag to run all jobs once on startup

## What is yt-dlp?
[`yt-dlp`](https://github.com/yt-dlp/yt-dlp) is a maintained fork of `youtube-dl` used to search YouTube (`ytsearch`), resolve media URLs, and stream audio without a browser.

## Configuration
Create a `config.yaml` based on `config.example.yaml`:

```yaml
global:
  search_limit: 5             # fallback when a job does not override
  player:
    command: ffplay           # any audio-only player is fine
    args: [-nodisp, -autoexit, -loglevel, warning, "{url}"] # {url} placeholder required
    env:
      PULSE_SERVER: "unix:/tmp/pulse-socket" # point to host PulseAudio
    timeout: 20m              # per-job timeout for search + playback
  # Alternatively, use local MPD server:
  mpd:
    enabled: true             # if true, overrides 'player'
    network: tcp
    address: localhost:6600
  ytdlp:
    binary: yt-dlp            # override if you have a wrapper script
    cookies: /app/cookies.txt # optional: exported cookies to use YouTube Premium (no ads)
    extra_args: []            # extra flags passed to every yt-dlp call

jobs:
  - name: morning-feed
    cron: "0 8 * * *"         # standard 5-field cron
    keywords: ["my keyword", "daily playlist"]
    date_format: "2006-01-02" # Go layout; defaults to 2006-01-02
    search_limit: 5           # optional per-job override
```

Notes:
- Queries append the current date using `date_format`.
- `player.args` must include `{url}` once; if omitted the URL is appended automatically.
- If you need authenticated searches (e.g., to use YouTube Premium and avoid ads), export cookies and set `global.ytdlp.cookies` to the file path.

## Running locally on the Pi
Prereqs: `yt-dlp`, `ffmpeg` (for `ffplay`), and PulseAudio already running.

```bash
GOOS=linux GOARCH=arm64 go build -o yt-rpi-player .
./yt-rpi-player -config config.yaml -run-now
```

## Container usage (recommended on Pi)
Build for arm64 and run with the host PulseAudio socket mounted:

```bash
docker build --platform linux/arm64 -t yt-rpi-player .
docker run --rm --name yt-rpi-player \
  --network host \
  -v $XDG_RUNTIME_DIR/pulse/native:/tmp/pulse-socket \
  -e PULSE_SERVER=unix:/tmp/pulse-socket \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  yt-rpi-player
```

Tips:
- `--device /dev/snd` is usually not needed when talking to PulseAudio, but add it if your setup requires.
- To debug audio, you can set `player.command` to `mpv` or `aplay` if you prefer different tooling.

## Docker Compose
`docker-compose.yaml` is included for convenience:

```bash
export XDG_RUNTIME_DIR=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}  # ensure the PulseAudio path is set
docker compose up --build
```

Key bits:
- Uses host networking so PulseAudio and yt-dlp see the same network as the Pi.
- Mounts `./config.yaml` into the container.
- Mounts `${XDG_RUNTIME_DIR}/pulse/native` to `/tmp/pulse-socket` and sets `PULSE_SERVER` accordingly.
- Uncomment the cookies volume in `docker-compose.yaml` if you need authenticated `yt-dlp` access.

## Using YouTube Premium (no ads)
1. Export cookies from your browser session (logged into your Premium account):
   - Easiest: `yt-dlp --cookies-from-browser safari --cookies cookies.txt "https://www.youtube.com/watch?v=p4AZvHxOQ5U" --max-downloads 0`
   - Or use a browser extension (e.g., “cookies.txt”) to save a `cookies.txt`.
2. Place the file in your project and set `global.ytdlp.cookies` to its container path (default `/app/cookies.txt`).
3. When using Docker/Compose, mount the file: add `- ./cookies.txt:/app/cookies.txt:ro` under `volumes` for the service.
4. Rebuild/restart the container. All searches and stream resolutions will use the cookies so ads should not play.

## Web UI
Enable the on-demand download UI in `config.yaml`:

```yaml
global:
  web_ui:
    enabled: true
    listen: ":8080"
    username: "pi"        # Basic auth username (empty = no auth)
    password: "secret"    # Basic auth password
    download_dir: "/media/music/youtube"
    subdir: ""            # optional global subdir under download_dir
    max_concurrent: 2
    timeout: "2h"
```

Or start it with the `-web-ui` flag:

```bash
./yt-rpi-player -config config.yaml -web-ui
```

Open `http://ithilien.local:8080/` (host networking is used in the compose file). Paste a YouTube video, channel, or playlist URL and choose:

- **Queue in MPD** — add downloaded file(s) to the MPD playlist.
- **Auto-play now** — start playing the first added item (only meaningful with MPD).
- **Download video too** — download full video+audio as MKV; otherwise audio-only M4A.

Downloads use the legacy `yt.sh` naming pattern:
`<download_dir>/<uploader>/<playlist_title>/<title> (<id>).<ext>` and write to `archive.txt` so re-downloads are skipped.

A durable `history.jsonl` is kept next to `config.yaml`. The status page auto-refreshes every 5 seconds.

### Security notes
- URL input is passed directly as a `yt-dlp` command argument; no shell interpolation is used.
- Set `web_ui.username` and `web_ui.password` for Basic auth. Leaving them empty disables authentication.
- Concurrent downloads share the same `archive.txt`; rare archive races are a known caveat.

## Flags
- `-config path`: path to YAML config (default `config.yaml`).
- `-run-now`: execute all jobs once immediately after startup before scheduling.
- `-web-ui`: start the web UI server alongside the scheduler.

## Disclaimer

This is a personal hobby project provided as-is under the MIT License, with no warranty.

It is a thin wrapper around [`yt-dlp`](https://github.com/yt-dlp/yt-dlp). You are responsible for how you use it:

- Use it only for content you are entitled to access, and respect YouTube's Terms of Service and the copyright laws in your jurisdiction.
- The optional cookies feature is intended for authenticating with **your own** account (e.g. to use an existing YouTube Premium subscription for ad-free playback). Never commit your `cookies.txt` or share credentials.
- This project is not affiliated with, endorsed by, or sponsored by YouTube or Google.
