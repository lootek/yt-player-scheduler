# Plan: Web UI for yt-rpi-player

## Context

The `yt-rpi-player` Go service (~/projects/github/lootek/yt-daily-player) is a headless cron scheduler that searches YouTube daily feeds and plays audio on a Raspberry Pi. The goal is to extend it with a minimal web UI so a user can paste any YouTube URL (video, channel, playlist), choose audio-only vs. video download, and optionally queue the result into MPD — all without touching the existing scheduler functionality.

Deployment: Docker on `pi@192.168.10.22`, `network_mode: host`, so `:8080` binds directly on the Pi. Media lives at `/media/music/youtube` (2.1TB ext4, existing `archive.txt`).

---

## Implementation Plan

### Step 1 — Config changes

**File:** `internal/config/config.go`

Add `WebConfig` struct:
```go
type WebConfig struct {
    Enabled     bool   `yaml:"enabled"`
    Listen      string `yaml:"listen"`       // default: ":8080"
    DownloadDir string `yaml:"download_dir"` // default: "/media/music/youtube"
}
```

Add to `GlobalConfig`:
```go
Web WebConfig `yaml:"web"`
```

Add defaults in `applyDefaults()`:
```go
if cfg.Global.Web.Listen == "" {
    cfg.Global.Web.Listen = ":8080"
}
if cfg.Global.Web.DownloadDir == "" {
    cfg.Global.Web.DownloadDir = "/media/music/youtube"
}
```

Relax the jobs-required check (line 76-78):
```go
if len(cfg.Jobs) == 0 && !cfg.Global.Web.Enabled {
    return cfg, errors.New("no jobs configured")
}
```

---

### Step 2 — New `DownloadWeb()` method on ytdlp.Client

**File:** `internal/ytdlp/ytdlp.go` (additive; existing `Download()` untouched)

```go
type DownloadWebOptions struct {
    URL         string
    DownloadDir string    // e.g. /media/music/youtube
    MusicOnly   bool      // true: audio-only M4A; false: bestvideo+bestaudio MKV
    Progress    io.Writer // receives yt-dlp stdout+stderr lines
}

func (c Client) DownloadWeb(ctx context.Context, opts DownloadWebOptions) error
```

yt-dlp flags (consistent with legacy `yt.sh`):
- `-i` — ignore errors (essential for playlists)
- `--download-archive {DownloadDir}/archive.txt`
- `--output {DownloadDir}/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- Audio: `-x --audio-format m4a -f bestaudio[ext=m4a]/bestaudio`
- Video: `-f bestvideo+bestaudio --merge-output-format mkv --embed-metadata`
- Reuse `baseArgs()` and `prepareCookies()` from existing code

Use `cmd.StdoutPipe()` + `cmd.StderrPipe()` with `bufio.Scanner` goroutines to stream output to `opts.Progress`. No `--print after_move:filepath` — playlists produce multiple files.

---

### Step 3 — New `AddToMPDQueue()` function

**File:** `internal/player/mpd_player.go` (additive; existing `PlayWithMPD()` untouched)

```go
func AddToMPDQueue(ctx context.Context, cfg config.MPDConfig, downloadDir string, uri string) error
```

Same as `PlayWithMPD()` but: dial → (optionally update library + wait 30s) → `AddID(uri, -1)` → return. No `PlayID()`, no monitoring loop.

---

### Step 4 — New `internal/webui` package (3 files)

#### `internal/webui/jobs.go`

In-memory job store with `sync.RWMutex`:

```go
type JobStatus int  // Pending, Running, Done, Failed

type Job struct {
    ID        string
    URL       string
    MusicOnly bool
    AddToMPD  bool
    Status    JobStatus
    StartedAt time.Time
    EndedAt   time.Time
    Output    []byte   // accumulated yt-dlp output
    Err       string
}

type JobStore struct { ... }

func NewJobStore() *JobStore
func (s *JobStore) Create(url string, musicOnly, addToMPD bool) string  // returns hex ID
func (s *JobStore) Get(id string) (Job, bool)
func (s *JobStore) AppendOutput(id string, p []byte)
func (s *JobStore) Finish(id string, err error)
func (s *JobStore) List() []Job  // most-recent first
```

#### `internal/webui/server.go`

```go
type Server struct {
    cfg    config.Config
    ytdlp  ytdlp.Client
    store  *JobStore
    logger *log.Logger
    srv    *http.Server
}

func NewServer(cfg config.Config, yt ytdlp.Client, logger *log.Logger) *Server
func (s *Server) Start() error
func (s *Server) Shutdown(ctx context.Context) error
```

Routes:
| Method | Path | Handler |
|--------|------|---------|
| GET | `/` | index: render form + last-20 jobs |
| POST | `/download` | validate URL, create job, launch goroutine, redirect to `/status?id=X` |
| GET | `/status` | show job output; `<meta refresh content="3">` while running |

`runDownloadJob()` goroutine:
1. Create `jobOutputWriter` that calls `store.AppendOutput()` per line
2. Call `ytdlp.DownloadWeb(context.Background(), ...)`  ← background ctx; not cancelled on tab close
3. On success + addToMPD: call `player.AddToMPDQueue(...)`
4. Call `store.Finish(id, err)`

`jobOutputWriter` implements `io.Writer`, buffers partial lines, flushes complete lines to store.

#### `internal/webui/templates.go`

Embedded HTML as `const` strings, parsed with `html/template`. Two templates:

- **`index`**: form (URL input, two checkboxes pre-checked/unchecked, submit button) + recent jobs table
- **`status`**: `<pre>` with yt-dlp output (dark terminal style), `<meta refresh>` while running, link back to `/`

No external CSS/JS. `shortID` template func returns first 8 chars of job ID.

---

### Step 5 — Wire into `main.go`

Add after `app.New(...)`:
```go
if cfg.Global.Web.Enabled {
    webServer := webui.NewServer(cfg, application.YtDLP(), logger)
    if err := webServer.Start(); err != nil {
        log.Fatalf("start web server: %v", err)
    }
    defer webServer.Shutdown(context.Background())  // with 10s timeout
    logger.Printf("web UI listening on %s", cfg.Global.Web.Listen)
}
```

Add accessor to `internal/app/app.go`:
```go
func (a *App) YtDLP() ytdlp.Client { return a.ytdlp }
```

(Check actual field name in `app.go` — explorer found it as `a.ytdlp`.)

---

### Step 6 — Deployment

**`docker-compose.yaml`:** Add volume mount (covers existing cache subdir too):
```yaml
- /media/music/youtube:/media/music/youtube
```

**`config.yaml` on Pi:** Add:
```yaml
global:
  web:
    enabled: true
    listen: ":8080"
    download_dir: /media/music/youtube
```

**`config.example.yaml`:** Same section.

---

## No new dependencies

All new code uses only Go stdlib: `net/http`, `html/template`, `sync`, `bufio`, `bytes`, `crypto/rand`, `io`, `time`, `path/filepath`.

---

## Verification

1. `go build ./...` in repo root — must compile clean
2. Start locally: `./yt-rpi-player -config config.yaml` with `web.enabled: true`
3. Open `http://192.168.10.22:8080` in browser — form renders
4. Paste a short YouTube video URL, check "Music only", submit
5. Confirm redirect to `/status?id=X`, output lines appear, auto-refresh stops on completion
6. Check `/media/music/youtube/<uploader>/<playlist>/` for downloaded file
7. Repeat with "Add to MPD queue" checked — verify MPD queue gains the track
8. Rebuild Docker image: `docker compose build && docker compose up -d`
