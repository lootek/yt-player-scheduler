---
name: web-ui-extension-plan
description: Plan to add a web UI for downloading and scheduling YouTube content.
metadata:
  type: project
---

# Web UI Extension Plan

## Context
The `yt-daily-player` service currently uses a `cron` scheduler to search for YouTube videos based on keywords and play them via `mpv` or `mpd`. The goal is to extend this with a web interface that allows users to manually trigger downloads of specific YouTube links (videos, channels, or playlists) and optionally schedule them for playback.

## Objectives
- Implement a web server within the existing Go application.
- Add a simple web UI (HTML/CSS) for interacting with the service.
- Implement a `POST /download` endpoint that:
    - Accepts a YouTube URL.
    - Accepts a `schedule_for_mpd` boolean flag.
    - Downloads the content using the existing `ytdlp` client.
    - If `schedule_for_mpd` is true, adds the content to the playback schedule.
- Ensure the download directory is configurable (existing functionality).

## Implementation Details

### 1. Backend Changes (Go)

#### `internal/config/config.go`
- (Check if needed) Add configuration for the web server (e.g., `web_port`).

#### `internal/app/app.go`
- **Refactor `App` struct**:
    - Move the `cron` instance from `main.go` into the `App` struct so it can be accessed by the web server.
    - Add an `http.Server` or a way to start the server.
- **Implement Web Server**:
    - Use `net/http` to create a simple server.
    	- Endpoint `GET /`: Serves the `index.html`.
	- Endpoint `POST /download`:
	    - Parse JSON body: `{ "url": "...", "schedule": true/false }`.
	    - Use `a.ytdlp.Download(ctx, url, ...)` to perform the download.
	    - If `schedule` is true:
	        - Since we don't have a way to specify a time via the UI yet, I'll implement a "play once" approach or add it to the `cron` for a default interval (e.g., immediately). *Self-correction*: A better way might be to have a separate "queue" or just use the `cron` to trigger a playback of this specific URL.
	        - For simplicity, I'll implement "schedule" as adding a one-time job to the `cron` instance that plays the downloaded file.

#### `main.go`
- Initialize the `App` and start the web server in a goroutine.
- Ensure graceful shutdown of the web server along with the main application.

### 2. Frontend Changes (HTML/JS)
- Create a new directory `web/` (or similar) to hold the static assets.
- `index.html`:
    - A simple form with an `<input type="text">` for the URL.
    - A `<input type="checkbox">` for "Schedule for MPD".
    - A `<button>` to submit.
    - JavaScript to `fetch` the `POST /download` endpoint and show status/errors.

### 3. Reusing Existing Components
- Use `internal/ytdlp` for all downloading and info retrieval.
- Use `internal/player` for playback.
- Use `internal/config` for settings.

## Verification Plan
- **Manual Test**:
    - Run the service.
    - Access `http://localhost:8080` (or configured port).
    - Submit a YouTube video URL.
    - Verify the file appears in the `download_dir`.
    - Verify that with the "Schedule" checkbox, a playback attempt is triggered.
- **Integration Test**:
    - Ensure that adding a job via the Web UI doesn't interfere with existing `cron` jobs.
    - Verify that the download directory remains the same as configured in `config.yaml`.

## Critical Files to Modify
- `main.go`
- `internal/app/app.go`
- `internal/config/config.go` (if needed)
- Create `web/index.html`
