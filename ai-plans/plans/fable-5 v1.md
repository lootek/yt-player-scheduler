# Web UI for on-demand YouTube downloads — yt-player-scheduler @ bfe645af

## Context

`yt-player-scheduler` (repo: `~/projects/lootek/yt-player-scheduler`, module `github.com/lootek/yt-rpi-player`) at revision **`bfe645af23127578dea7ee3f1caf3ae98f19442b`** ("less logging noise, try bringing back end condition") is a Go daemon for a headless RPi 4B ("ithilien"):

- Cron jobs (robfig/cron, YAML config) build a keyword+date query, search YouTube via `yt-dlp ytsearch`, then either stream (`ffplay`/`mpv`) or download audio first (`ytdlp.Download`, m4a) and play — optionally via **MPD** (`gompd/v2`, with password auth and `music_root` path mapping in `internal/player/mpd_player.go`).
- Structure at this revision: `main.go`, `internal/{app,config,player,query,ytdlp}`. **No web UI exists.**
- Deployed via Docker (host network, PulseAudio socket) at `pi@ithilien:~/yt-daily-player/` (newer version there — explicitly out of scope).

**Goal:** add a web UI where the user pastes a YouTube **video / playlist / channel** URL and the service downloads it to a configured directory (`/media/music/youtube` on ithilien), with two checkboxes:
1. **Queue in MPD** — schedule downloaded file(s) onto the MPD playlist (default: off).
2. **Download video too** — full video+audio (MKV); otherwise audio-only M4A (default: off).

Conventions must follow the legacy `~/scripts/yt.sh`: output layout `<download_dir>/<uploader>/<playlist_title>/<title> (<id>).<ext>`, `archive.txt` download-archive dedup, m4a audio / mkv video.

Agreed scope (user answers): form + job status page + **persistent history** (JSONL); **optional Basic auth**; checkbox defaults off; plan built from reflog-based reconstruction of `bfe645af` (verify with git at implementation time — Bash tool outage blocked `git show`/ssh during planning).

## Design

Server-rendered Go `html/template` pages + a small vanilla JS poller. No new heavy dependencies — stdlib `net/http` only (repo already uses only cron/gompd/yaml).

### 1. Config — `internal/config/config.go`

Add `WebUIConfig` under `GlobalConfig`:

```go
type WebUIConfig struct {
    Enabled       bool   `yaml:"enabled"`
    Listen        string `yaml:"listen"`         // default ":8080"
    Username      string `yaml:"username"`       // empty => no auth
    Password      string `yaml:"password"`
    DownloadDir   string `yaml:"download_dir"`   // e.g. /media/music/youtube
    MaxConcurrent int    `yaml:"max_concurrent"` // default 2
    Timeout       string `yaml:"timeout"`        // per-job, default "2h"
    HistoryPath   string `yaml:"history_path"`   // default <download_dir>/history.jsonl
}
```

Defaults in `applyDefaults()`; `DownloadDir` falls back to `ytdlp.download_dir`. Relax the "no jobs configured" fatal in `Load()`/`main.go` to allow web-UI-only operation.

### 2. Downloader — `internal/ytdlp/ytdlp.go`

New `DownloadMedia(ctx, DownloadMediaRequest) (DownloadMediaResult, error)` alongside the existing `Download()` (which stays untouched for cron jobs). Request: `URL, DownloadDir, ArchivePath, Video bool, LogWriter io.Writer, OnPending/OnDone func(path)`.

yt-dlp invocation (reuse existing `prepareCookies()` and the shared arg builder present at the revision):

```
yt-dlp -i --add-metadata \
  --download-archive <download_dir>/archive.txt \
  --output '<download_dir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s' \
  --newline --progress \
  --print 'before_download:PENDING:%(filename)s' \
  --print 'after_move:DONE:%(filepath)s' \
  [audio: -x --audio-format m4a | video: -f bestvideo+bestaudio --merge-output-format mkv] \
  [--cookies <tmp>] <URL>
```

- One code path handles video/playlist/channel URLs — yt-dlp expands them natively; `-i` keeps going past member errors.
- Stream stdout line-by-line: `PENDING:` → planned file, `DONE:` → completed file (drives live status); also parse stderr `"] Destination: "` lines as pending fallback. Tee both streams into the job's log buffer.
- URL is passed as a single `exec` argument — no shell interpolation (injection-safe).

### 3. MPD enqueue — `internal/player/mpd_player.go`

New `EnqueueMPD(ctx, cfg MPDConfig, downloadDir string, uris []string) error`: dial (with password support, as `PlayWithMPD` does), map each path via the existing `mapToMusicRoot()`, `client.Update()` the affected relative dirs, wait for the update, then `AddID(rel, -1)` each file. Unlike `PlayWithMPD` it does **not** start playback and does not block monitoring status.

### 4. New package — `internal/webui/`

- `job.go` — `Job{ID, URL, Video, MPD bool, Status (queued|running|done|failed), Pending/Files []string, Error string, Log *bytes.Buffer, CreatedAt/StartedAt/FinishedAt}`.
- `service.go` — in-memory job map + buffered channel queue (`MaxConcurrent*4`, reject with 503 when full); worker pool of `MaxConcurrent` goroutines started from `main`; per-URL mutex so identical URLs don't race on output files/archive; per-job `context.WithTimeout(Timeout)`; on success with MPD checked → `player.EnqueueMPD` with the completed files.
- `history.go` — append-only JSONL at `HistoryPath` (event per enqueue/start/done/failed); `List(limit)` folds events into latest-state jobs so the UI survives restarts.
- `server.go` — routes: `GET /` (form + recent jobs), `POST /download` (form fields: `url`, `video`, `mpd`; redirect to status), `GET /status?id=`, `GET /api/jobs`, `GET /api/jobs/{id}`, `GET /log/{id}` (plain text), `GET /static/*`. Basic-auth middleware active only when username+password set (constant-time compare). Graceful `Shutdown()`.
- `templates/index.html`, `templates/status.html`, `static/app.js` — `//go:embed`; form shows the target directory from config; two checkboxes (both unchecked by default); jobs table polls `/api/jobs` every 2s showing status, pending/done files, log link.

### 5. Wiring — `main.go`

- New flag `-web-ui` (OR-ed with `config web_ui.enabled`).
- When enabled: open history, construct service, `go svc.Start(ctx)`, `go srv.ListenAndServe(cfg.Global.WebUI.Listen)`; shut down HTTP server with a 10s timeout on SIGINT/SIGTERM. Scheduler behavior unchanged.

### 6. Docs & deploy — `config.example.yaml`, `README.md`, `docker-compose.yaml`

- Example config block with `download_dir: /media/music/youtube`.
- Compose: add `- /media/music/youtube:/media/music/youtube` volume (same path inside/outside so `mapToMusicRoot` against `mpd.music_root: /media/music` works); host networking already exposes `:8080`.
- README: usage section, security notes.

## Files

| Action | Path |
|---|---|
| modify | `main.go`, `internal/config/config.go`, `internal/ytdlp/ytdlp.go`, `internal/player/mpd_player.go`, `config.example.yaml`, `README.md`, `docker-compose.yaml` |
| create | `internal/webui/{server,service,job,history,templates}.go`, `internal/webui/templates/{index,status}.html`, `internal/webui/static/app.js` |

## Implementation order

1. Pre-flight (needs working Bash): `git show bfe645af -- <files>` to confirm the reconstruction; fetch `pi@ithilien:~/scripts/yt.sh` and reconcile exact yt-dlp flags/naming (esp. whether it uses `%(playlist_title)s` fallback like `%(playlist_title|NA)s`); branch from `bfe645af`.
2. Config + defaults.
3. `ytdlp.DownloadMedia`.
4. `player.EnqueueMPD`.
5. `webui` package (service → history → server → templates/JS).
6. `main.go` wiring + docs/compose.
7. Commit per logical step (straight to master per repo conventions, co-authored footer).

## Verification

- `go build ./...` && `go vet ./...`.
- Unit-test `history.go` fold logic and the PENDING/DONE line parser (table tests, no network).
- Local smoke test: run with a scratch config (`download_dir: /tmp/ytui`, mpd disabled), open `http://localhost:8080`, paste a short public video → file appears as `<uploader>/NA/<title> (<id>).m4a`; re-submit → skipped via archive; playlist URL → multiple files under `<uploader>/<playlist_title>/`; video checkbox → `.mkv`. Verify Basic auth 401 when configured.
- On ithilien (post-deploy, out of plan scope): MPD checkbox → files appear on MPD playlist (`mpc playlist`).

## Post-approval bookkeeping

Copy this plan to `~/ai/yt-ui-ext/claude-fable-5.md` (session dir, named after the model) as requested.
