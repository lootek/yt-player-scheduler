# Plan: Extend yt-player-scheduler with Web UI and Download functionality

## Context
The `yt-player-scheduler` service is currently a headless, cron-based scheduler that performs searches and downloads via `yt-dlp`. It lacks a user interface for on-demand downloads. The goal is to add a web-based interface and API to allow users to trigger downloads (video or audio only) for specific YouTube links, with the option to either just download them to a directory or also schedule them for playback via MPD.

## Implementation Plan

### 1. Configuration Update
- **File**: `internal/config/config.go`
- **Changes**: 
    - Add `DownloadDir` (string) to `GlobalConfig` to specify the destination.
    - Add `YtdlpArgs` or specific flags to support the `yt.sh` pattern (e.g., `--add-metadata`, `--embed-thumbnail`).
    - Ensure the configuration can be extended to handle new download-specific parameters if needed.

### 2. Web API Implementation
- **File**: `internal/app/api.go` (New file) or within `internal/app/app.go`.
- **Changes**:
    - Implement an HTTP server using `net/http`.
    - Create a `POST /download` endpoint.
    - **Request Body**:
        - `url` (string): The YouTube link.
        - `video_only` (bool): If `false`, download audio only.
        - `schedule_mpd` (bool): If `true`, add this as a one-off job to the cron scheduler.
    - **Logic**:
        - If `schedule_mpd` is `true`, use the existing job execution logic to trigger a download and subsequent playback.
        - If `false`, simply trigger the `ytdlp.Download` method.

### 3. Web UI Implementation
- **Files**: `web/index.html` and `web/script.js` (New files).
- **Changes**:
    - Create a simple, clean HTML interface.
    - **Features**:
        - Input field for the YouTube URL.
        - Checkbox for "Download Video" (default checked).
        - Checkbox for "Schedule for Playback (MPD)" (default unchecked).
        - "Download" button.
    - **JavaScript**: Use `fetch` to send the `POST` request to the API.
    - **Static File Serving**: Update the Go server to serve these files from the `web/` directory.

### 4. Downloading Logic Enhancement
- **File**: `internal/ytdlp/ytdlp.go`
- **Changes**:
    - Update the `Download` method to accept a `videoOnly` boolean.
    - Ensure the naming pattern is consistent with `~/scripts/yt.sh`: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`.
    - Ensure the download uses the `DownloadDir` from the configuration.

### 5. Main Entry Point Integration
- **File**: `main.go`
- **Changes**:
    - Initialize and start the HTTP server in a separate goroutine alongside the existing cron scheduler.

## Verification Plan
1.  **API Test**: Use `curl` to send a `POST` request to `localhost:<port>/download` with various payloads (video vs audio, schedule vs no-schedule).
2.  **UI Test**: Open the web interface in a browser, enter a URL, and verify the request is sent and processed.
3.  **Download Verification**:
    - Check that files appear in the configured `DownloadDir`.
    - Verify the file naming pattern matches the requirement.
    - Verify that the "audio only" checkbox correctly produces `.m4a` files.
4.  **Scheduling Verification**: 
    - When `schedule_mpd` is `true`, verify that the service attempts to play the file (checking logs/mpv behavior).
    - When `schedule_mpd` is `false`, verify that only the download occurs without triggering the player.
