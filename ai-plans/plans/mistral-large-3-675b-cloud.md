# Plan: Add Web UI for YouTube Content Download

## Context
The existing `yt-player-scheduler` is a Go service that schedules and plays YouTube content based on cron jobs defined in YAML. The goal is to extend its functionality by adding a web UI that allows users to:

1. Paste a YouTube video, channel, or playlist link
2. Download the content to `/media/music/youtube` (remote box: `ithilien`)
3. Optionally schedule it for playback via MPD (checkbox)
4. Choose between video or audio-only download (checkbox)

The web UI will be integrated into the existing Go application using a lightweight web framework like Gin or Echo.

## Implementation Plan

### 1. Web Framework Integration
**Files to modify:**
- `main.go` (add web server initialization)
- Add new directory: `internal/web/` (web handlers, templates, static files)

**Approach:**
- Use the [Gin](https://github.com/gin-gonic/gin) framework for its simplicity and performance
- Initialize the web server on a configurable port (default: `8080`)
- Add a new flag `-web-port` to allow port customization

### 2. Web UI Structure
**Files to create:**
- `internal/web/handlers.go` (HTTP handlers)
- `internal/web/templates/index.html` (HTML template)
- `internal/web/static/style.css` (CSS styling)
- `internal/web/static/script.js` (frontend logic)

**UI Components:**
- Form with:
  - Text input for YouTube URL (required)
  - Checkbox: "Add to MPD playlist" (default: unchecked)
  - Checkbox: "Audio only" (default: checked)
  - Submit button
- Status area for download progress/results
- Basic styling for mobile/desktop compatibility

### 3. Download Logic
**Files to modify:**
- `internal/ytdlp/ytdlp.go` (extend existing client)
- `internal/web/handlers.go` (call yt-dlp with user options)

**Approach:**
- Extend the existing `ytdlp.Client` to support:
  - Playlist/channel downloads (recursive)
  - Audio-only extraction (`-x --audio-format m4a`)
  - Video download (`--format best`)
  - Output template consistent with legacy `yt.sh`:
    ```go
    outputTemplate := filepath.Join(c.cfg.DownloadDir, "%(uploader)s - %(title)s [%(id)s].%(ext)s")
    ```
- Add a new method `DownloadContent(ctx context.Context, url string, audioOnly bool, addToMPD bool) error`

### 4. MPD Integration
**Files to modify:**
- `internal/player/mpd_player.go` (extend to support adding local files)

**Approach:**
- Extend `PlayWithMPD` to accept local file paths
- Add a new method `AddToMPDPlaylist(mpdConfig config.MPDConfig, filePath string) error`
- Ensure the file is within `music_root` for MPD to find it

### 5. Configuration
**Files to modify:**
- `config.example.yaml` (add web server config)
- `internal/config/config.go` (add web config struct)

**New config options:**
```yaml
web:
  enabled: true
  port: 8080
  # Basic HTTP auth credentials
  auth:
    username: "admin"
    password: "change-me"
```

### 6. Authentication
**Files to modify:**
- `internal/web/handlers.go` (add middleware)

**Approach:**
- Use Gin's middleware for basic HTTP auth
- Validate credentials against config
- Protect all routes except static files

### 7. Deployment Considerations
**Files to modify:**
- `Dockerfile` (expose web port)
- `docker-compose.yaml` (add port mapping)

**Changes:**
- Expose port `8080` in Dockerfile
- Add port mapping in `docker-compose.yaml`:
  ```yaml
  ports:
    - "8080:8080"
  ```

## Verification
1. **Local Testing:**
   - Run the service with `-web-port 8080`
   - Access `http://localhost:8080`
   - Test with various YouTube URLs (video, playlist, channel)
   - Verify downloads appear in `/media/music/youtube` (remote)
   - Verify MPD playlist updates when checkbox is checked

2. **Container Testing:**
   - Build and run the container
   - Access the web UI via the mapped port
   - Verify all functionality works as in local testing

3. **Edge Cases:**
   - Invalid YouTube URLs
   - Private/unavailable content
   - Large playlists/channels
   - Network interruptions during download

## Open Questions
1. Should the web UI support progress tracking for downloads? (e.g., WebSocket updates)
2. Should there be a way to view/download history or manage existing downloads?
3. Should the web UI be accessible from outside the local network? (Security implications)
4. Should the basic auth credentials be configurable via environment variables for container deployments?