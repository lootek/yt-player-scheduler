# Web UI for yt-rpi-player

## Context

The `yt-rpi-player` service on `ithilien` currently only schedules
cron-based YouTube searches and plays the top hit. We want a small LAN-facing
web UI on the same Go binary that lets a user paste a YouTube link
(video, channel, or playlist) and trigger a download directly. Output goes
to a user-chosen subdirectory under `/media/music/youtube` on the Pi
(which is already bind-mounted into the container at the same path). Two
optional checkboxes control behavior: enqueue into MPD after download, and
"dump video" (bestvideo+bestaudio mkv) vs audio-only m4a. Naming pattern
must match the legacy `~/scripts/yt.sh` exactly.

Out of scope: changing the existing cron/search path, switching to a
web framework, adding auth, or moving the binary off the Pi.

## Stack

- `net/http` + `html/template` + a single `static/app.js` (vanilla JS).
- New `internal/webui` package, no extra Go deps.
- Auth: none (LAN-only, host network).
- Persistence: append-only `history.jsonl` next to `config.yaml`.
- Reuse: `ytdlp.Client` (new method), `player.MPD` (split out enqueue).

## Reuse audit (key findings)

- `internal/ytdlp/ytdlp.go` `Client.Download` (line 39) is hard-coded to
  audio-only + flat template; no archive, no video, no metadata, no
  playlist flag. We'll **leave it alone** and add a new method.
- `internal/player/mpd_player.go` `PlayWithMPD` (line 25) dials MPD,
  rewrites the prefix, calls `Update` + `AddID` + `PlayID`, then polls
  status. We need a sibling `EnqueueMPD` that does the first three and
  returns the id without playing or polling.
- `internal/config/config.go` `GlobalConfig` / `YtDLPConfig` have a
  single global `download_dir` and no UI fields; we'll add a
  `WebUIConfig` block.
- `main.go` is a thin cron loop; the web server can run as a goroutine
  after `c.Start()`.
- `docker-compose.yaml` already has `network_mode: host`; no `ports:`
  block needed — we just pick a high port (`8088`) and document it.
- Module path is `github.com/lootek/yt-rpi-player` (dir name is the
  older `yt-player-scheduler`).

## Plan

### 1. New code: `internal/webui/`

```
internal/webui/
  job.go        — Job model, NewID() (8 hex from crypto/rand)
  history.go    — History{path}, Append/List/Update; O_APPEND writes
  service.go    — Service struct + worker goroutine + Enqueue() + run()
  server.go     — http.ServeMux routes + html/template render
  templates.go  — //go:embed templates/ static/
  templates/
    layout.html — base shell
    index.html  — form + jobs table
  static/
    app.js      — fetch + 2s poll, no deps
```

**Routes** (mounted on a fresh `*http.ServeMux`):
- `GET  /` — render `index.html`
- `POST /api/enqueue` — body `{url, subdir, schedule_mpd, dump_video}` → `{id, status}`
- `GET  /api/jobs?limit=50` — list
- `GET  /api/jobs/{id}` — single
- `GET  /api/suggest?url=…` — runs `ytdlp.Inspect` and returns `{subdir, mode}`
- `GET  /static/*` — embedded

`Service` keeps one in-memory `map[string]*Job` for fast reads; `History`
is the durable store (rewritten on terminal state, appended on enqueue).
Worker count: 1 (yt-dlp is the bottleneck; the Pi has one disk).

### 2. Modify: `internal/ytdlp/ytdlp.go`

Add (do **not** touch existing `Download`):
- `type DownloadOptions struct { Subdir string; Video bool; OutputTemplate string; ArchivePath string }`
- `func (c Client) DownloadLegacy(ctx, url string, opts DownloadOptions) ([]string, error)`
- `func (c Client) Inspect(ctx, url string) (VideoEntry, error)` — single `--dump-single-json`

`DownloadLegacy` arg composition (mirrors `~/scripts/yt.sh`):

```
audio:  -x --audio-format m4a  --add-metadata
        --download-archive <ArchivePath>     # always, both modes
video:  -f bestvideo+bestaudio --merge-output-format mkv --add-metadata
        --download-archive <ArchivePath>
output: -o '<base>/<Subdir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'
return: walk <base>/<Subdir>/ after yt-dlp exits, collect *.m4a/*.mkv/*.mp4
        new since job start. yt-dlp --print after_move:filepath gives a
        single line; playlists need the walk fallback.
```

`baseArgs()`, `prepareCookies()`, `binary()` are reused as-is.

### 3. Modify: `internal/player/mpd_player.go`

Split out:
- `func EnqueueMPD(ctx, cfg, downloadDir, uri string) (int, error)` — dial,
  prefix-rewrite, `Update`, `AddID`, **return id**, no `PlayID`, no poll.
- `PlayWithMPD` becomes `id, err := EnqueueMPD(...); PlayID(id); then the
  existing poll loop`. Cron path stays byte-identical in behavior.

### 4. Modify: `internal/config/config.go`

Add to `GlobalConfig`:
```go
WebUI WebUIConfig `yaml:"webui"`
```
```go
type WebUIConfig struct {
    Enabled         bool   `yaml:"enabled"`           // default false
    Listen          string `yaml:"listen"`            // default ":8088"
    HistoryDir      string `yaml:"history_dir"`       // default = dir(config.yaml)
    DefaultMode     string `yaml:"default_mode"`      // "audio"|"video", default "audio"
    DownloadBaseDir string `yaml:"download_base_dir"` // default = YtDLP.DownloadDir
}
```
Defaults in `applyDefaults`. All fields optional so existing deployments
keep running with `webui` absent.

### 5. Modify: `main.go`

After `c.Start()`, if `cfg.Global.WebUI.Enabled`:
```go
hist, _ := webui.OpenHistory(historyDir)
svc := webui.NewService(cfg, logger, hist)
go svc.Start(ctx)
srv := webui.NewServer(svc, tpl, logger)
go srv.ListenAndServe(cfg.Global.WebUI.Listen)
logger.Printf("webui listening on %s", cfg.Global.WebUI.Listen)
```
On `<-ctx.Done()` workers drain as the queue closes when the context
cancels.

### 6. Config schema diff (`config.example.yaml`)

```yaml
global:
  webui:
    enabled: true
    listen: ":8088"           # LAN only — auth is not implemented
    history_dir: ""           # empty = same dir as config.yaml
    default_mode: audio       # "audio" or "video"
    download_base_dir: ""     # empty = global.ytdlp.download_dir
```

### 7. Frontend (`templates/*.html`, `static/app.js`)

- `layout.html`: `<!doctype html>` shell, header, `{{template "content" .}}`,
  `<script src="/static/app.js" defer>`.
- `index.html`: form (url, subdir, two checkboxes, "Suggest" button →
  `GET /api/suggest`), submit → `POST /api/enqueue`, `<table id="jobs">`
  thead/tbody populated by JS.
- `app.js` (~60 LOC): `fetch` enqueue, `setInterval(poll, 2000)`, render
  via `textContent` (no innerHTML), status badge per row, error tooltip
  for `failed` rows.

### 8. Docker / deploy

- No `ports:` block needed (`network_mode: host`).
- No new volumes (`history.jsonl` lives next to `config.yaml`; archive
  lives inside the user-chosen subdir).
- Port `8088` documented in README.

## Critical files

- `/Users/piotr/projects/lootek/yt-player-scheduler/internal/ytdlp/ytdlp.go`
- `/Users/piotr/projects/lootek/yt-player-scheduler/internal/player/mpd_player.go`
- `/Users/piotr/projects/lootek/yt-player-scheduler/internal/config/config.go`
- `/Users/piotr/projects/lootek/yt-player-scheduler/main.go`
- `/Users/piotr/projects/lootek/yt-player-scheduler/config.example.yaml`
- new: `/Users/piotr/projects/lootek/yt-player-scheduler/internal/webui/*`
- new: `/Users/piotr/projects/lootek/yt-player-scheduler/internal/webui/templates/*`
- new: `/Users/piotr/projects/lootek/yt-player-scheduler/internal/webui/static/app.js`

## Verification

On `ithilien` (host networking, mDNS: `ithilien.local`):

1. `cd ~/projects/lootek/yt-player-scheduler && docker compose build && docker compose up -d`
2. `curl -sf http://ithilien.local:8088/api/jobs` → `[]`.
3. `curl -sf -XPOST http://ithilien.local:8088/api/enqueue \
   -H 'content-type: application/json' \
   -d '{"url":"https://www.youtube.com/watch?v=…","subdir":"brewiarz","schedule_mpd":true,"dump_video":false}'` → `{"id":"…","status":"pending"}`.
4. `docker logs -f yt-rpi-player` shows the yt-dlp cmd + final path; `ls
   /media/music/youtube/brewiarz/<uploader>/<playlist_title>/` shows
   `Title (id).m4a` matching the legacy pattern.
5. `mpc playlist` shows the enqueued track.
6. `cat <history_dir>/history.jsonl` shows the appended job line.
7. Repeat step 3 with `"dump_video":true`; verify
   `…/brewiarz/<uploader>/<playlist_title>/Title (id).mkv` and
   `archive.txt` in the subdir.
8. Negative test: bad URL → job ends `failed` with `error` populated; UI
   shows red row.
9. Regression: trigger a cron job (`-run-now`) and confirm
   `internal/ytdlp.Download` is byte-identical to before.
10. UI smoke test in browser: open `http://ithilien.local:8088/`, submit
    a playlist, watch the table fill in and flip to `done`.
