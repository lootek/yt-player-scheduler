# Plan: Web UI for yt-player-scheduler

## Context

The existing service (`~/projects/lootek/yt-player-scheduler`) is a Go cron scheduler that searches YouTube for keywords and plays results via MPD/mpv. The user wants to add a web UI for on-demand downloading: paste a YouTube URL (video, channel, or playlist), optionally download video (not just audio), optionally queue in MPD, and have files land in a configurable directory with naming consistent with the legacy `~/scripts/yt.sh` script.

## Summary

Add an HTTP server to the existing Go binary (no new dependencies — stdlib only). A single-page web UI with a form and a task list. The existing `ytdlp.Client` gets two new methods (`DownloadWithOptions`, `GetInfo`). An in-memory task store tracks download progress. All templates and static files are embedded via `//go:embed`.

## Config Changes

**`internal/config/config.go`** — add `WebConfig` struct:

```go
type WebConfig struct {
    Enabled        bool   `yaml:"enabled"`
    ListenAddr     string `yaml:"listen_addr"`     // default "0.0.0.0:8080"
    DownloadDir    string `yaml:"download_dir"`    // default "/media/music/youtube"
    ArchiveFile    string `yaml:"archive_file"`    // default "archive.txt" (relative to DownloadDir)
    OutputTemplate string `yaml:"output_template"` // default "%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s"
}
```

Add `Web WebConfig` to `GlobalConfig`. Apply defaults in `applyDefaults()`. If `web:` section is absent, `Enabled` stays false — full backward compatibility.

## New Package: `internal/web/`

```
internal/web/
    server.go       — HTTP server setup, embed directives, start/shutdown
    handler.go      — page render + API handlers (create/list/get tasks)
    taskstore.go    — in-memory task store with mutex
    templates/
        index.html  — single-page UI
    static/
        style.css   — minimal dark-theme CSS
```

### Task Store (`taskstore.go`)

- `Task` struct: ID, URL, Title, Uploader, VideoMode, AddToMPD, Status, Error, FilePath, CreatedAt, CompletedAt
- Statuses: pending, downloading, done, error, skipped
- `TaskStore` with `sync.RWMutex`, bounded to 100 tasks (evict oldest completed)
- Methods: `Create()`, `Get()`, `List()`, `UpdateStatus()`, `SetMetadata()`, `SetFilePath()`

### HTTP API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Serve the HTML page |
| GET | `/static/style.css` | Serve embedded CSS |
| POST | `/api/tasks` | Create download task (body: `{url, video_mode, add_to_mpd}`) → 202 |
| GET | `/api/tasks` | List all tasks as JSON |
| GET | `/api/tasks/{id}` | Get single task as JSON |

### Handler Flow (POST /api/tasks)

1. Parse JSON, validate URL contains `youtube.com` or `youtu.be`
2. Create task (status: pending), return 202 immediately
3. Spawn goroutine:
   - `GetInfo()` → populate title/uploader
   - `DownloadWithOptions()` with appropriate format flags and archive file
   - If `add_to_mpd` and MPD enabled → `player.PlayWithMPD()`
   - Update status to done/error/skipped

### Frontend (`templates/index.html`)

- Form: URL input, "Download video" checkbox, "Add to MPD" checkbox (shown only if MPD enabled)
- Task table: URL, Title, Status badge (color-coded), Created time
- Vanilla JS: form submit handler, `setInterval(pollTasks, 3000)` for live updates
- Template variables: `{{.MPDEnabled}}`, `{{.DownloadDir}}`

## ytdlp Client Extensions (`internal/ytdlp/ytdlp.go`)

### New Types

```go
type DownloadOptions struct {
    URL            string
    OutputTemplate string
    VideoMode      bool   // true = video+audio mkv, false = audio-only m4a
    ArchiveFile    string // path to download archive
    JobName        string
}

var ErrAlreadyArchived = errors.New("all videos already in download archive")
```

### New Methods

- **`DownloadWithOptions(ctx, opts) (string, error)`** — unified download with format control and archive support. When `VideoMode`: `-f bestvideo+bestaudio --merge-output-format mkv`. When `!VideoMode`: `-x --audio-format m4a`. When `ArchiveFile != ""`: `--download-archive <file>`. Returns file path or `ErrAlreadyArchived`.
- **`GetInfo(ctx, url) (title, uploader, error)`** — fetches metadata without downloading (`--skip-download --playlist-items 1 -O`).

### Refactor

Existing `Download()` method delegates to `DownloadWithOptions()` internally so there's one canonical download path.

## Changes to Existing Files

### `internal/app/app.go`
Add getter: `func (a *App) YtDLP() ytdlp.Client { return a.ytdlp }`

### `main.go`
After cron setup, if `cfg.Global.Web.Enabled`:
- Create `web.Server`, call `Start()` (starts listening in goroutine)
- On shutdown (`<-ctx.Done()`), call `webSrv.Shutdown()` with 5s timeout

## Deployment

- **Dockerfile**: No changes (templates embedded at compile time)
- **docker-compose.yaml**: Add port mapping `8080:8080` and volume mount for `/media/music/youtube`
- **Production config** on ithilien: Add `web:` section with `enabled: true`, `download_dir: /media/music/youtube`, archive and template defaults

## Naming Consistency with Legacy

| Aspect | Legacy `yt.sh` | New Default |
|--------|---------------|-------------|
| Output template | `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s` | Same |
| Download dir | `/media/pi/music/youtube/` | `/media/music/youtube` |
| Archive file | `archive.txt` (in download dir) | Same |
| Video format | `bestvideo+bestaudio` → mkv | Same (when video_mode=true) |
| Audio format | N/A (legacy always did video) | m4a (when video_mode=false) |

## Implementation Order

1. Config: `WebConfig` + defaults in `config.go`, example in `config.example.yaml`
2. ytdlp: `DownloadOptions`, `DownloadWithOptions()`, `GetInfo()`, `ErrAlreadyArchived`, refactor `Download()`
3. Task store: `taskstore.go`
4. Templates + static: `index.html`, `style.css`
5. Handlers: `handler.go`
6. Server: `server.go` with embed, route registration, start/shutdown
7. Wire up: `app.go` getter, `main.go` server lifecycle
8. Deployment: `docker-compose.yaml` port + volume

## Verification

1. `go build ./...` — must compile with no new dependencies
2. Start with `web.enabled: false` — service behaves exactly as before (no port open)
3. Start with `web.enabled: true` — HTTP server listens, `curl localhost:8080/` returns HTML
4. POST a video URL — task appears in list, file lands in download dir with correct naming
5. POST a playlist URL — all videos downloaded, archive.txt updated, skips on re-submit
6. Checkbox "Download video" — produces `.mkv` instead of `.m4a`
7. Checkbox "Add to MPD" — file queued in MPD after download
8. `Ctrl+C` — both cron and HTTP server shut down gracefully
