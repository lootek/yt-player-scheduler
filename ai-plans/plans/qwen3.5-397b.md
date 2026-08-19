# Web UI for yt-player-scheduler

## Context

The service at commit `bfe645af` is a Go-based YouTube scheduler that:
- Runs cron-scheduled jobs to search YouTube daily feeds by keyword + date
- Downloads audio via yt-dlp and plays through MPD or ffplay/mpv
- Stores configuration in YAML (jobs, player/MPD settings, yt-dlp options)

The task is to add a **web UI** for on-demand downloading of YouTube videos/playlists/channels to `/media/music/youtube` on the remote Pi (ithilien), independent of the scheduler's cron jobs.

## Requirements

### Functional
1. Web UI with form to paste YouTube URL (video, channel, or playlist)
2. Configuration options:
   - Download directory (default: `/media/music/youtube`)
   - Checkbox: "Queue in MPD" (add to MPD playlist after download)
   - Checkbox: "Auto-play now" (start playback immediately via MPD)
   - Checkbox: "Dump video" (download full video+audio as mkv; when unchecked, audio-only m4a)
3. Use legacy `~/scripts/yt.sh` naming pattern: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
4. Single shared `archive.txt` in root download directory for deduplication across all downloads
5. Build web UI from scratch (not extending the later eefb43b implementation)

### Technical Constraints
- Target base: commit `bfe645af23127578dea7ee3f1caf3ae98f19442b`
- Go with gin, html/template, embed for static assets
- yt-dlp for downloads (reuse existing `internal/ytdlp` package, extend with `DownloadMedia`)
- MPD integration via `internal/player/mpd_player.go` (add `EnqueueMPD` function)
- Basic auth support (username/password in config)
- Worker pool for concurrent downloads (configurable max_concurrent)
- Job queue with status tracking (queued/running/done/failed)
- History persistence (JSONL format)

## Implementation Plan

### Phase 1: Extend Core Packages

#### 1.1 `internal/config/config.go`
Add `WebUIConfig` struct:
```go
type WebUIConfig struct {
    Enabled       bool   `yaml:"enabled"`
    Listen        string `yaml:"listen"`
    Username      string `yaml:"username"`
    Password      string `yaml:"password"`
    DownloadDir   string `yaml:"download_dir"`
    Subdir        string `yaml:"subdir"`
    MaxConcurrent int    `yaml:"max_concurrent"`
    Timeout       string `yaml:"timeout"`
    ArchivePath   string `yaml:"archive_path"`
}
```
Add to `GlobalConfig` and apply defaults.

#### 1.2 `internal/ytdlp/ytdlp.go`
Add `DownloadMedia` method:
- Accept `DownloadMediaRequest` with URL, DownloadDir, Subdir, ArchivePath, Video flag
- Use yt-dlp with: `-i --download-archive <archive.txt> --add-metadata`
- Video mode: `-f bestvideo+bestaudio --merge-output-format mkv`
- Audio-only mode: `-x --audio-format m4a`
- Output template: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- Return `[]string` of downloaded file paths

#### 1.3 `internal/player/mpd_player.go`
Add `EnqueueMPD` function:
- Accept multiple file paths, append to MPD playlist
- Support auto-play flag to start playing first item
- Map paths relative to `music_root` for MPD discovery

### Phase 2: Web UI Package (`internal/webui/`)

#### 2.1 `job.go`
Define `Job` struct with ID, URL, Video, MPD, AutoPlay, Status, Files, Error, Log, timestamps.

#### 2.2 `history.go`
Implement `History` with append/list operations for `history.jsonl` persistence.

#### 2.3 `service.go`
Implement `Service` with:
- Worker pool (goroutines = `MaxConcurrent`)
- Job queue (channel-based)
- In-memory job state map
- Methods: `Enqueue`, `Get`, `List`, `Start`

#### 2.4 `server.go`
Implement HTTP server with:
- Routes: `GET /`, `POST /download`, `GET /status`, `GET /api/jobs`, `GET /api/jobs/{id}`
- Basic auth middleware
- Panic recovery
- Static file serving (JS)
- Template rendering

#### 2.5 `templates/index.html`
Form with:
- URL input (required)
- Checkboxes: "Queue in MPD", "Auto-play now", "Dump video"
- Submit button
- Jobs table for status display

#### 2.6 `static/app.js`
Client-side:
- Poll `/api/jobs` for status updates
- Render job rows with status badges
- Click-to-expand log view

#### 2.7 `templates.go`
Embed templates and static files via `//go:embed`.

### Phase 3: Wire Up in `main.go`

- Parse `-web-ui` flag (or use config)
- Initialize `History`, `Service`, `Server`
- Start worker pool in background goroutine
- Graceful shutdown on SIGINT/SIGTERM

### Phase 4: Update Configuration

#### `config.example.yaml`
Add `web_ui` section:
```yaml
web_ui:
  enabled: true
  listen: ":8080"
  username: ""
  password: ""
  download_dir: /media/music/youtube
  subdir: ""
  max_concurrent: 2
  timeout: "2h"
  archive_path: /media/music/youtube/archive.txt
```

## Files to Modify/Create

### Modify
- `internal/config/config.go` - Add WebUIConfig
- `internal/ytdlp/ytdlp.go` - Add DownloadMedia method
- `internal/player/mpd_player.go` - Add EnqueueMPD function
- `main.go` - Wire up web UI server
- `config.example.yaml` - Document web_ui section

### Create
- `internal/webui/job.go`
- `internal/webui/history.go`
- `internal/webui/service.go`
- `internal/webui/server.go`
- `internal/webui/templates.go`
- `internal/webui/templates/index.html`
- `internal/webui/templates/status.html`
- `internal/webui/templates/layout.html`
- `internal/webui/static/app.js`

## Verification

1. Build: `go build -o yt-rpi-player .`
2. Configure: Add `web_ui` section to `config.yaml` with credentials
3. Run: `./yt-rpi-player -config config.yaml`
4. Access: `http://localhost:8080`
5. Test scenarios:
   - Download audio-only from video URL
   - Download video (mkv) from video URL
   - Download playlist
   - Download channel
   - Verify archive.txt deduplication
   - Verify MPD enqueue with/without auto-play
   - Verify file naming matches `yt.sh` pattern
