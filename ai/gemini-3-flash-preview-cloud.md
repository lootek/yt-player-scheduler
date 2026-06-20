# Plan - YT Player Scheduler Web UI Extension

Extend the `yt-player-scheduler` service with a web interface for manual downloading of YouTube videos, channels, or playlists with MPD integration options.

## Context

The current service is a headless scheduler that downloads and plays YouTube content based on cron jobs. The goal is to add an interactive web UI that allows users to:
1. Paste a YouTube link.
2. Choose between Audio-only or Video (mkv).
3. Optionally add the downloaded content to the MPD queue.
4. Maintain consistency with the legacy `yt.sh` naming pattern: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`.

## Proposed Changes

### 1. Configuration
- Update `internal/config/config.go`:
    - Add `WebConfig` to `GlobalConfig`: `Enabled` (bool), `Address` (string), `DownloadDir` (string).
    - Add defaults in `applyDefaults`.
    - Note: On `ithilien`, the user specified `/media/music/youtube` as the target directory.

### 2. `yt-dlp` Wrapper
- Update `internal/ytdlp/ytdlp.go`:
    - Refactor `Download` to accept a `DownloadOptions` struct:
        ```go
        type DownloadOptions struct {
            URL            string
            DownloadDir    string
            AudioOnly      bool
            OutputTemplate string
        }
        ```
    - Implement the legacy naming pattern as default for web downloads.
    - If `AudioOnly` is false, use `-f bestvideo+bestaudio --merge-output-format mkv --add-metadata`.
    - If `AudioOnly` is true, use `-x --audio-format m4a`.

### 3. Application Logic
- Update `internal/app/app.go`:
    - Add `ProcessWebDownload(ctx context.Context, url string, audioOnly bool, scheduleMPD bool) error`.
    - This method will call `ytdlp.Download` and then optionally `player.PlayWithMPD` (if `scheduleMPD` is true).

### 4. Web Interface
- Create `internal/web/` package:
    - `server.go`: `net/http` server with embedded assets (`embed` package).
    - `assets/index.html`: Simple HTML/JS UI with:
        - URL input.
        - "Audio only" checkbox (checked by default).
        - "Add to MPD" checkbox.
        - Status log.

### 5. Main Entry Point
- Update `main.go`:
    - Start the web server in a goroutine if enabled in config.

## Critical Files
- `internal/config/config.go`
- `internal/ytdlp/ytdlp.go`
- `internal/app/app.go`
- `internal/web/server.go`
- `internal/web/assets/index.html`
- `main.go`

## Verification
1. Run the service locally with web UI enabled.
2. Access `http://localhost:8080`.
3. Test downloading a single video (Audio/Video).
4. Test downloading a playlist.
5. Verify naming pattern in the download directory.
6. Verify MPD queueing (if MPD is available).

---
Model: gemini-3-flash-preview:cloud
Date: 2026-06-20
