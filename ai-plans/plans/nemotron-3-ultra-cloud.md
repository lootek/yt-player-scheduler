# Plan: Add Web UI for YouTube Downloads to yt-player-scheduler

## Context

The current `yt-player-scheduler` (at commit `bfe645af23127578dea7ee3f1caf3ae98f19442b`) is a Go service that:
- Runs scheduled YouTube searches via cron using `yt-dlp` with `ytsearchdate`
- Plays audio through `ffplay` (PulseAudio) or adds to MPD playlist
- Can optionally download audio to a local directory before playing
- Configured via YAML (`config.yaml`)

**Goal**: Extend with a web UI where users can paste a YouTube URL (video, channel, or playlist) to download content to a configured directory. Options:
- Checkbox: "Add to MPD playlist" (schedule for playing)
- Checkbox: "Download video" (vs audio-only/music)

The legacy script at `~/projects/lootek/scripts/yt.sh` uses:
- Output template: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- Download directory: `/media/music/youtube`
- Format: bestvideo+bestaudio merged to mkv
- Archive file for deduplication

On the remote box `pi@ithilien`, the dedicated download directory is `/media/music/youtube`.

## Current Architecture

```
main.go                 # Entry point, cron scheduling
internal/
  config/config.go      # YAML config structs (GlobalConfig, JobConfig, etc.)
  app/app.go            # Core logic: RunJob, CheckAuth
  ytdlp/ytdlp.go        # yt-dlp wrapper: Search, Download, ResolveStream
  player/player.go      # Audio playback (ffplay, mpv, MPD)
  player/mpd.go         # MPD integration (need to verify)
  query/query.go        # Query building for date-based searches
```

Key config fields (from `config.yaml`):
- `global.ytdlp.download_dir` - where to download
- `global.mpd.enabled` - whether MPD is used
- `global.mpd.music_root` - MPD music root for local files

## Implementation Plan

### 1. Add HTTP Server & Web UI

**New files:**
- `internal/web/server.go` - HTTP server setup, routes
- `internal/web/handlers.go` - Request handlers for download API
- `internal/web/templates.go` - HTML template (embedded or static file)
- `web/static/` - CSS/JS for the UI (optional, can inline)

**Dependencies to add:**
- `github.com/gin-gonic/gin` or `net/http` + `html/template` (prefer stdlib for simplicity)
- `embed` for embedding static assets

**Routes:**
- `GET /` - Serve web UI (single page)
- `POST /api/download` - Accept download request
- `GET /api/status/:id` - Poll download status (SSE or polling)
- `GET /api/config` - Return relevant config (download dir, MPD enabled)

### 2. Extend Config for Web UI

Add to `GlobalConfig` in `internal/config/config.go`:
```go
type GlobalConfig struct {
    // ... existing fields
    WebUI WebUIConfig `yaml:"webui"`
}

type WebUIConfig struct {
    Enabled      bool   `yaml:"enabled"`
    Address      string `yaml:"address"`       // e.g., ":8080"
    DownloadDir  string `yaml:"download_dir"`  // override global.ytdlp.download_dir if set
    Username     string `yaml:"username"`      // optional basic auth
    Password     string `yaml:"password"`      // optional basic auth
}
```

Default: disabled, address `:8080`, download_dir falls back to `global.ytdlp.download_dir`.

### 3. Download Logic (Reuse ytdlp.Client)

Extend `internal/ytdlp/ytdlp.go` with new method:
```go
func (c Client) DownloadURL(ctx context.Context, url string, opts DownloadOptions) (string, error)
```

Where `DownloadOptions`:
```go
type DownloadOptions struct {
    AudioOnly    bool   // --extract-audio --audio-format m4a (default true)
    OutputTmpl   string // custom output template
    AddToMPD     bool   // whether to add to MPD playlist after download
    JobName      string // for naming/organization
}
```

**Naming pattern** (matching legacy yt.sh):
- Default: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- For single videos (no playlist): `%(uploader)s/%(title)s (%(id)s).%(ext)s`

**Format selection:**
- Audio only (default): `-f bestaudio[ext=m4a]/bestaudio --extract-audio --audio-format m4a`
- With video: `-f bestvideo+bestaudio --merge-output-format mkv`

### 4. MPD Integration After Download

Reuse existing `player.PlayWithMPD` but for local files:
- After download, if `AddToMPD` is true, call `player.AddToMPDPlaylist(cfg.MPD, localPath)`
- Need to verify `internal/player/mpd.go` exists or add function

### 5. Main.go Integration

- Parse new `-webui` flag or check config `global.webui.enabled`
- Start HTTP server in goroutine alongside cron scheduler
- Share config and logger with web handlers

### 6. Web UI (Single Page)

Simple HTML page with:
- Input field for YouTube URL
- Checkbox: "Add to MPD playlist" (only shown if MPD enabled in config)
- Checkbox: "Download video" (unchecked = audio only)
- Submit button
- Progress/status area (polling or SSE)
- List of recent downloads with status

### 7. Docker/Deployment Updates

- Update `Dockerfile` to expose web UI port (8080)
- Update `docker-compose.yaml` to expose port
- Document new config options in `config.example.yaml` and `README.md`

## Key Questions for Clarification

1. **Authentication**: Should the web UI have auth? (Basic auth via config, or rely on network isolation?)
2. **Concurrent downloads**: Allow multiple simultaneous downloads, or queue them?
3. **Status persistence**: Store download history in memory, file, or SQLite?
4. **Video format**: For "download video", use mkv (legacy) or mp4 (more compatible)?
5. **Playlist handling**: Download all videos in playlist/channel, or just first N?
6. **Cookie handling**: Reuse existing cookie mechanism from `global.ytdlp.cookies`?