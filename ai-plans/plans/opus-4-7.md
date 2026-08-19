# Plan: Web UI for ad-hoc YouTube downloads in `yt-daily-player`

## Context

`yt-daily-player` (~/projects/github/lootek/yt-daily-player) is a headless Go daemon running on `pi@192.168.10.22` (a.k.a. `ithilien`) inside Docker (`network_mode: host`). It runs cron-scheduled YouTube searches, downloads the top hit via `yt-dlp`, and plays it through MPD or `ffplay`. It has no HTTP/web layer.

We want to extend it with a small web UI where the user can paste a YouTube URL (video / channel / playlist) and have it downloaded into the local music library. Behavior is controlled by two checkboxes:

- **Schedule for MPD** — if checked, after each file finishes downloading, `mpd update` is run on its relative path and the file is appended to MPD's current queue (no playback interruption). If unchecked, the file is just dumped on disk.
- **Include video** — if unchecked (default), audio-only m4a (current behavior, audio-extracted). If checked, full video using the legacy `~/scripts/yt.sh` recipe (`bestvideo+bestaudio` merged to `mkv`, `--add-metadata`).

The download target is `/media/music/youtube/` on the Pi (already populated by the legacy `yt.sh`). Naming pattern, archive file, and yt-dlp flags must match `yt.sh` so the new feature blends with the existing library.

The existing scheduled jobs (brewiarz/jutrznia) keep their current `%(uploader)s - %(title)s [%(id)s].m4a` pattern — only the new web-UI download path follows the yt.sh convention.

## Decisions locked with the user

| Decision | Choice |
|---|---|
| Download UX for long jobs | Async job queue + `/jobs` page with status |
| MPD action when scheduled | `mpd update` + append to current queue (no interruption) |
| Auth / exposure | None; bind to `0.0.0.0:8080`, LAN-trusted |
| Existing cron jobs | Untouched; new download path uses legacy `yt.sh` flags |
| Job state persistence | In-memory only |
| "Include video" format | `bestvideo+bestaudio --merge-output-format mkv --add-metadata` |
| MPD ops in UI | None beyond the checkbox; no per-row "play now" button |

## Reference: legacy `yt.sh`

```
/usr/local/bin/youtube-dl -i \
    --download-archive archive.txt \
    -f bestvideo+bestaudio \
    --merge-output-format mkv \
    --add-metadata \
    -a <(bash ./list.sh) \
    -o '%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'
```

CWD is `/media/pi/music/youtube/` on the legacy script; on the Pi today the equivalent live tree is `/media/music/youtube/` (already verified — same directory layout, same `archive.txt` already present at `/media/music/youtube/archive.txt`).

Key conventions from `yt.sh` to carry over:
- `-i` (ignore download errors, keep going through a playlist/channel)
- `--download-archive archive.txt` so previously-downloaded items in `/media/music/youtube/archive.txt` are skipped
- `--add-metadata`
- Output template `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- For audio-only: switch `-f bestvideo+bestaudio --merge-output-format mkv` for `-x --audio-format m4a -f bestaudio[ext=m4a]/bestaudio` (matches existing `internal/ytdlp/ytdlp.go:54-60` style but keeps the `yt.sh` output template + archive)

## Architecture

### New package `internal/web`

Plain `net/http` (no framework — we already pull only stdlib + yaml + cron + gompd; no need for chi/gin). Three endpoints:

| Method + path | Purpose |
|---|---|
| `GET /` | Render the form (HTML template, embedded via `embed.FS`) |
| `POST /jobs` | Accept `url`, `schedule_mpd` (bool), `include_video` (bool); enqueue, redirect to `/jobs` |
| `GET /jobs` | Render queue + history table; auto-refresh `<meta http-equiv="refresh" content="3">` |
| `GET /jobs/{id}/log` | Tail of yt-dlp stdout/stderr for that job (plain text) |

Keep templates and any CSS in `internal/web/templates/*.tmpl` and embed with `//go:embed`.

### New package `internal/queue`

In-memory FIFO worker:

```go
type Job struct {
    ID            string    // ulid or unix-nano hex
    URL           string
    ScheduleMPD   bool
    IncludeVideo  bool
    Status        string    // "queued"|"running"|"done"|"failed"
    Files         []string  // post-download paths reported by yt-dlp --print after_move:filepath
    Logs          *bytes.Buffer
    Err           string
    CreatedAt     time.Time
    StartedAt     time.Time
    FinishedAt    time.Time
}

type Manager struct {
    cfg     config.Config
    ytdlp   ytdlp.Client     // existing client, we add a new method
    logger  *log.Logger
    mu      sync.Mutex
    jobs    map[string]*Job
    order   []string         // for deterministic ordering in /jobs
    work    chan *Job
}
```

- One worker goroutine (single-threaded download — yt-dlp on a Pi shouldn't run two concurrent jobs anyway).
- `Enqueue(url, mpd, video) string` returns the job ID; pushes to `work`.
- `Run(ctx)` consumes `work`, sets status, calls a new `ytdlp.DownloadAdHoc(...)` that streams output into `job.Logs`, then if `ScheduleMPD` → calls a new `player.MPDQueueAppend(...)` per returned file (no playback monitoring loop).
- Job history: keep last N=200 in-memory; trim `order` from the front when over cap.

### New method `ytdlp.Client.DownloadAdHoc`

Mirrors `Download()` but:
- Uses `download_dir` from a NEW config field `web.download_dir` (defaults to `/media/music/youtube` if unset). Falls through to `cfg.Global.YtDLP.DownloadDir` only if `web.download_dir` is empty AND we want to share the cron cache (we don't — keep them separate).
- Output template: `<download_dir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- Always appends `-i` and `--download-archive <download_dir>/archive.txt`
- Always appends `--add-metadata`
- Adds `--ignore-config` so user-level yt-dlp config doesn't surprise us
- Format selection:
  - `IncludeVideo=true`: `-f bestvideo+bestaudio --merge-output-format mkv`
  - `IncludeVideo=false`: `-x --audio-format m4a -f bestaudio[ext=m4a]/bestaudio`
- `--print after_move:filepath` (already used in `Download()`) — collect every line as a separate completed file (channels/playlists yield many)
- `--newline` so progress lines flush; `--progress` for nicer logs
- Streams stdout/stderr live into `job.Logs` (instead of `bytes.Buffer`-then-return) by setting `cmd.Stdout = io.MultiWriter(progressParser, job.Logs)`. `progressParser` is a tiny `io.Writer` that pulls lines starting with `<download_dir>/` (post-`after_move`) and appends them to `job.Files`.

Reuses existing `prepareCookies()`, `baseArgs()`, `binary()` helpers.

### New helper `player.MPDQueueAppend`

Refactor the front part of `PlayWithMPD` (`internal/player/mpd_player.go:16-52`) into a non-blocking variant:

```go
func MPDQueueAppend(cfg config.MPDConfig, downloadDir, uri string) error
```

- Dial, optional auth (existing logic)
- If `downloadDir` is set & `cfg.MusicRoot` set & `uri` is under `downloadDir` & `downloadDir` is under `MusicRoot`: compute `rel`, call `client.Update(rel)`, sleep 30s
- `client.AddID(uri, -1)` and **return** — no `PlayID`, no playback monitor (this is the "append, don't interrupt" behavior the user asked for)

Keep `PlayWithMPD` untouched so the cron path is unaffected.

### Config additions (`internal/config/config.go`)

```go
type GlobalConfig struct {
    // ... existing fields ...
    Web WebConfig `yaml:"web"`
}

type WebConfig struct {
    Enabled     bool   `yaml:"enabled"`      // default false (opt-in)
    Address     string `yaml:"address"`      // default ":8080"
    DownloadDir string `yaml:"download_dir"` // default "/media/music/youtube"
}
```

Defaults applied in `applyDefaults()` (`config.go:82-114`): if `Web.Enabled`, fill in address & download_dir. The web download dir is independent of `Global.YtDLP.DownloadDir` (which the cron jobs use for brewiarz cache).

### Wiring in `main.go`

After scheduler start (around `main.go:79-85`), if `cfg.Global.Web.Enabled`:

```go
qm := queue.NewManager(cfg, application.YtDLPClient(), logger)
go qm.Run(ctx)
srv := web.NewServer(cfg.Global.Web, qm)
go srv.Run(ctx) // log & exit on listener error
```

App needs an exported accessor for its `ytdlp.Client` (or queue.Manager constructs its own — cleaner; let's do that to avoid a new public method on App).

## File map (new + edits)

**New:**
- `internal/web/server.go` — `http.Server` setup, routes, embed.FS
- `internal/web/handlers.go` — index/submit/jobs/log handlers
- `internal/web/templates/index.tmpl` — form (URL textarea, two checkboxes, submit)
- `internal/web/templates/jobs.tmpl` — table of jobs with status + link to logs
- `internal/queue/manager.go` — Manager + Job + worker loop
- `internal/queue/manager_test.go` — table test for status transitions, with a fake `Downloader` interface
- `internal/ytdlp/download_adhoc.go` — `DownloadAdHoc` method (or extend `ytdlp.go`)

**Edited:**
- `internal/config/config.go` — add `WebConfig`, defaults
- `internal/player/mpd_player.go` — add `MPDQueueAppend` (don't touch `PlayWithMPD`)
- `main.go` — start web server + queue manager when `web.enabled`
- `config.example.yaml` — show the new `web:` block, commented
- `docker-compose.yaml` — broaden mount from `/media/music/youtube/yt-rpi-player-cache/brewiarz` to `/media/music/youtube` (read-write); host networking already exposes :8080
- `README.md` — short "Web UI" section

## Concrete details that bit me during exploration (carry into impl)

1. **MPD path mapping** (`mpd_player.go:33`) requires `downloadDir` to be a prefix of `MusicRoot`. Today's config has `music_root: /media/music`. With `web.download_dir = /media/music/youtube`, that's fine — `Rel("/media/music", "/media/music/youtube/Channel/.../foo.mkv") = "youtube/Channel/.../foo.mkv"`. Verified mentally, but worth a unit test with a fake mpd client.
2. **Container mount** is currently the brewiarz subpath only. Must be widened in docker-compose, otherwise downloads succeed inside the container but never appear on the Pi's actual `/media/music/youtube` tree, and `mpd update` will see nothing.
3. **`network_mode: host`** means `:8080` on the container = `:8080` on the Pi — no `ports:` mapping needed. Just don't collide with another listener on the Pi (mpd is on 6600, others unknown — quick `ss -tlnp` check during verification).
4. **Cron jobs share the binary** so a panic in the web layer would crash the whole daemon. Wrap handlers + worker loop in `defer recover()` that logs and continues.
5. **Long downloads** (channel = hours). Don't put a `context.WithTimeout` on the worker context — let SIGTERM cancel via the parent ctx, but otherwise let yt-dlp run as long as it needs.
6. **Channels/playlists yield many files** — `--print after_move:filepath` prints one line per completed item. The progress parser must collect all of them, and `MPDQueueAppend` must be called per file (not just the last).

## Verification

End-to-end on the Pi:

1. **Build & deploy**
   - `cd ~/projects/github/lootek/yt-daily-player && docker compose build` (cross-builds for arm64 — confirm Dockerfile already targets that)
   - `scp` source or rsync to `pi@192.168.10.22:~/yt-daily-player/`, then on the Pi: `docker compose up -d`
   - `docker logs -f yt-rpi-player` — confirm "web UI listening on :8080" and existing scheduler still says "scheduled job ..."

2. **Single-video, audio-only, no MPD**
   - Open `http://192.168.10.22:8080/`
   - Paste a short YouTube URL, leave both checkboxes unchecked, submit
   - On `/jobs`, status goes queued → running → done
   - On the Pi: `ls /media/music/youtube/<Uploader>/` shows `Title (id).m4a`
   - On the Pi: `grep <id> /media/music/youtube/archive.txt` finds the entry

3. **Video + MPD**
   - Same form, same URL, both checkboxes ON
   - File appears as `.mkv` under `<Uploader>/`
   - `mpc playlist | tail` (or whatever MPD client) shows the new track at the END (existing playback should not have been interrupted)
   - `mpc current` should not have changed if something was already playing

4. **Playlist test**
   - Paste a small public YouTube playlist (e.g. 3 items)
   - Confirm 3 separate files land under `<Uploader>/<Playlist>/`
   - Confirm 3 entries appended to MPD queue (if MPD checkbox on)
   - Re-submit same playlist URL: archive.txt skips all 3 and the job ends quickly (proves archive is honored)

5. **Existing cron job still works**
   - Wait for next brewiarz/jutrznia cron tick (or `-run-now` once)
   - Confirm it still downloads to its dedicated cache, plays via MPD, no regression

6. **Failure paths**
   - Submit garbage URL → job goes to status `failed` with stderr in the log view
   - Stop MPD service on the Pi, submit with MPD checkbox on → download still completes; queue-append step logs an error but doesn't kill the worker

7. **Unit tests**
   - `go test ./internal/queue/...` covers transitions: queued→running→done, queued→running→failed
   - `go test ./internal/config/...` covers WebConfig defaults
   - Optional: a small handler test for `POST /jobs` parsing form values
