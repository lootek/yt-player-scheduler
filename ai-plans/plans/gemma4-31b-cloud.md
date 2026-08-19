# Plan: Add Web UI for Manual YouTube Downloads

## Context
The current `yt-player-scheduler` is a headless service that schedules YouTube searches and playback. The user wants to extend this with a web-based interface to manually trigger downloads of specific YouTube videos, channels, or playlists. These downloads should follow the patterns established in the legacy `yt.sh` script and allow for options like including video and adding the result to the MPD playlist.

## Implementation Approach

### 1. Enhance `internal/ytdlp`
Modify the `ytdlp.Client` to support a more flexible download process.

- **New Method `DownloadCustom`**:
  - Parameters: `ctx context.Context`, `videoURL string`, `dumpVideo bool`, `outputTemplate string`, `archivePath string`.
  - If `dumpVideo` is `false`: Use `-x` and `--audio-format m4a` (music only).
  - If `dumpVideo` is `true`: Use `-f bestvideo+bestaudio --merge-output-format mkv`.
  - Use the provided `outputTemplate` and `--download-archive` with `archivePath`.
  - Return the final file path.

### 2. Enhance `internal/player`
Create a non-blocking version of MPD addition.

- **New Function `AddToMPD`**:
  - Extract the core logic from `PlayWithMPD` (Dial, Update, AddID).
  - Remove the `PlayID` call and the playback monitoring loop.
  - This allows the web UI to trigger an "Add to playlist" action without waiting for the song to finish playing.

### 3. Implement Web UI (`internal/web`)
Introduce a simple HTTP server.

- **Handler Setup**:
  - Create a new package `internal/web` or add to `internal/app`.
  - Use `net/http` for a lightweight implementation.
  - **Route `/`**: Serve a simple HTML page with:
    - Text input for YouTube URL.
    - Checkbox: "Include Video" (defaults to audio only).
    - Checkbox: "Add to MPD" (defaults to unchecked).
    - Submit button.
  - **Route `/download`**:
    - POST handler that:
      - Parses the URL and checkboxes.
      - Calls `ytdlp.DownloadCustom`.
      - If "Add to MPD" is checked, calls `player.AddToMPD`.
      - Returns a success/failure message.

### 4. Configuration Updates
- Update `config.Config` to include a `WebServer` section:
  - `Port int` (e.g., 8080).
- Update `config.yaml` on the `ithilien` remote box:
  - Ensure `global.ytdlp.download_dir` is set to `/media/music/youtube`.
  - Add `web_server.port`.

### 5. Integration in `main.go`
- Initialize the web server in `main.go` using the `App` instance.
- Run the web server in a separate goroutine so it doesn't block the scheduler.

## Legacy Consistency (`yt.sh`)
- **Output Template**: Use `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`.
- **Archive**: Use `/media/music/youtube/archive.txt`.
- **Format**: `bestvideo+bestaudio` $\to$ `mkv` for video; `m4a` for audio.

## Critical Files to Modify
- `internal/ytdlp/ytdlp.go`: Add `DownloadCustom`.
- `internal/player/mpd_player.go`: Add `AddToMPD`.
- `internal/config/config.go`: Add web server settings.
- `main.go`: Start the HTTP server.
- New file `internal/web/handler.go`: Implement the UI and API.

## Verification Plan
1. **Local Test**: Run the service locally, access the web UI, and verify a download to a local folder.
2. **Remote Test (`ithilien`)**:
   - Deploy the updated binary.
   - Access `http://ithilien:8080`.
   - Download a video $\to$ Verify file exists in `/media/music/youtube/` with correct naming.
   - Download music $\to$ Verify `.m4a` file exists.
   - Check "Add to MPD" $\to$ Verify the file appears in the MPD playlist.
   - Verify `archive.txt` is updated and prevents re-downloads.
