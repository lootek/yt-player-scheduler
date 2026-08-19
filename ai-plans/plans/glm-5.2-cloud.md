# Plan: On-demand Web UI for yt-player-scheduler

## Context

`yt-player-scheduler` at `bfe645a` is a cron-driven Go service: each job runs a `ytsearchdate` query via `yt-dlp`, optionally downloads the result to `download_dir`, then plays it via `ffplay`/`mpv`/MPD. It runs on `pi@ithilien` under `~/yt-daily-player/` against MPD at `192.168.10.22:6600` and stores downloads under `/media/music/youtube/yt-rpi-player-cache/...`. Deployment is via `docker compose` with `network_mode: host`.

The only way today to get a specific YouTube video/playlist/channel onto the box is to hand-edit `config.yaml` and add a cron job. We want a lightweight on-demand web UI — paste a URL, pick two flags, and let the box fetch it into `/media/music/youtube` using the same conventions as the legacy `~/projects/lootek/scripts/yt.sh` (uploader/playlist/title naming, `--download-archive` for idempotent re-runs). Optionally enqueue the freshly-downloaded files into MPD.

Decisions locked with the user:
- **MPD checkbox** = after download, add new files to MPD queue (one-shot `AddID`), not a recurring cron job.
- **Naming** = `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s` (yt.sh pattern).
- **Playlist/channel** = download everything; `--download-archive archive.txt` skips already-seen IDs.
- **Video vs audio** = highest quality in either mode; container format is not important. Video mode uses `bestvideo+bestaudio/best` merged to mkv; audio mode extracts best-quality audio to m4a.
- **Deployment** = same Go binary, gated by a `web_ui` config block. **Basic auth required** when enabled.
- **UI scope** = form + live status (progress) + persistent history (JSONL in `download_dir`).

## Architecture

New self-contained `internal/webui` package. The scheduler binary becomes a dual-purpose process: cron loop (unchanged) + optional HTTP server started only when `web_ui.enabled: true`. No new container, no new process. The existing `internal/ytdlp` package gains a second download method that matches yt.sh semantics; the existing `Download` (used by cron jobs) is untouched. The existing `internal/player/mpd_player.go` gains a non-blocking enqueue helper alongside the existing blocking `PlayWithMPD`.

### Files to add

- `internal/webui/server.go` — HTTP server, route table, basic-auth middleware, graceful shutdown.
- `internal/webui/service.go` — in-memory job queue, worker pool (`max_concurrent`), job state, yt-dlp stderr progress parsing.
- `internal/webui/history.go` — append-only JSONL reader/writer at `<download_dir>/.webui-history.jsonl`.
- `internal/webui/templates.go` — `//go:embed` of `templates/*.html`.
- `internal/webui/templates/index.html` — form + status + history shell.
- `internal/webui/static/static.go` — `//go:embed` of `static/app.js` + `static/style.css` (small).
- `internal/webui/static/app.js` — poll `/api/status` every 2s, render job table.
- `internal/webui/static/style.css` — minimal styling.

### Files to modify

- `internal/config/config.go` — add `WebUIConfig` struct and `Global.WebUI` field; defaults in `applyDefaults`.
- `internal/ytdlp/ytdlp.go` — add `DownloadURL(ctx, url, opts) ([]string, error)` next to `Download`. Reuses `baseArgs()`, `prepareCookies()`, `binary()`. Opts: `{VideoMode bool; OutputDir string; ArchivePath string}`.
- `internal/player/mpd_player.go` — add `Enqueue(ctx, cfg, downloadDir, uri) error` that mirrors the rel-path + `Update` logic of `PlayWithMPD` but only calls `AddID(uri, -1)` (no blocking status loop). Optionally `Play` if MPD is stopped.
- `main.go` — after `cron.New(...)` and `c.Start()`, if `cfg.Global.WebUI.Enabled`, construct `webui.NewServer(...)` and `go srv.Start(ctx)`. Add `web_ui` flag/note in usage.
- `config.example.yaml` — document the new `web_ui` block.
- `docker-compose.yaml` — no port mapping needed (`network_mode: host` already exposes the port); add a comment + optional volume for `download_dir` if not already mounted (it is on ithilien).

### Config schema addition

```yaml
global:
  web_ui:
    enabled: true
    listen: ":8080"
    username: "piotr"
    password: "..."        # required when enabled; refuse to start if missing
    download_dir: "/media/music/youtube"
    max_concurrent: 2
    timeout: "2h"
    history_path: ""       # default: <download_dir>/.webui-history.jsonl
```

If `enabled: true` and `username` or `password` is empty → `log.Fatalf` at startup.

### HTTP surface

- `GET /` — render `index.html` (form + history shell).
- `POST /download` — read `url`, `dump_video` (bool), `enqueue_mpd` (bool); validate URL is a youtube.com/youtu.be URL (reject others); enqueue job; redirect to `/`.
- `GET /api/status` — JSON: `{jobs: [...], history: [...]}`. Jobs include `id, url, mode, enqueue_mpd, state, progress_pct, current_file, started_at, finished_at, error, files[]`.
- Basic auth middleware on every route.

### Job runner

- Bounded channel of pending jobs; N workers where N = `max_concurrent`.
- Each worker:
  1. Create ctx with `web_ui.timeout`.
  2. Call `ytdlp.DownloadURL(ctx, url, opts)` — yt.sh flags + `--print after_move:filepath` + `--download-archive <download_dir>/archive.txt`.
  3. Stream stderr through a line parser that updates `job.ProgressPct` and `job.CurrentFile` from `[download] xx.x%` and `Downloading N of M` lines.
  4. Collect printed filepaths → `job.Files`.
  5. If `enqueue_mpd` and `cfg.Global.MPD.Enabled`: for each new file call `player.Enqueue(ctx, mpdCfg, downloadDir, file)`.
  6. On success/failure append to history JSONL.
- Serialize same-URL submissions (skip if a job for that URL is already running).

### yt-dlp flags for `DownloadURL`

Base: reuse `baseArgs()` (cookies, PO token, remote-components, js-runtimes=node, user-agent, extra-args) + `--download-archive <download_dir>/archive.txt` + `--no-warnings` + `--ignore-errors` + `--add-metadata`.

- Video mode: `-f bestvideo+bestaudio/best --merge-output-format mkv`
- Audio mode: `-x --audio-format m4a -f bestaudio/best`

Output template (both modes): `<download_dir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`. yt-dlp substitutes `NA` for missing `playlist_title` on single videos — acceptable per "don't care that much" answer. If that produces ugly `NA/` dirs, swap to `%(playlist_title&)%(playlist_title)s/%(title)s/%(id)s.%(ext)s` conditional form; note as a follow-up if it surfaces.

Add `--print after_move:filepath` to collect the resulting files. Parse stdout lines (one filepath per downloaded file).

### MPD enqueue

`player.Enqueue(ctx, cfg, downloadDir, uri)`:
- `mpd.Dial` / `DialAuthenticated` (same as `PlayWithMPD`).
- If `downloadDir != "" && cfg.MusicRoot != "" && strings.HasPrefix(uri, downloadDir)`: compute `rel, _ := filepath.Rel(cfg.MusicRoot, uri)`, `client.Update(rel)`, small sleep (reuse the 30s `time.Sleep` only if needed; 2–3s is usually enough on ithilien — note as tunable), `uri = rel`.
- `client.AddID(uri, -1)`.
- If MPD state is `stop`, `client.Play(0)` to kick off. No status-poll loop — return immediately.
- Close client. Non-blocking from the caller's perspective.

On ithilien: `download_dir=/media/music/youtube`, `music_root=/media/music` → rel = `youtube/<uploader>/...`. Matches existing scheduler's path-mapping logic.

### History format

JSONL, one record per finished job:
```json
{"id":"...","url":"...","mode":"video|audio","enqueue_mpd":true,"state":"done|error","started_at":"...","finished_at":"...","files":["/media/music/youtube/..."],"error":""}
```
Append via `os.OpenFile(..., O_APPEND|O_CREATE|O_WRONLY, 0644)`. Load last 50 on `/api/status`.

## Verification

End-to-end on a Mac (build for linux/arm64, copy to ithilien or build on Pi):

1. `cd ~/projects/lootek/yt-player-scheduler && GOOS=linux GOARCH=arm64 go build -o yt-rpi-player .`
2. Add a `web_ui` block to ithilien's `~/yt-daily-player/config.yaml` with a real username/password, `download_dir: /media/music/youtube`, `listen: :8080`.
3. `docker compose up --build` (or restart the container).
4. From a browser on the LAN: `http://ithilien:8080/` → basic-auth prompt → form renders.
5. Paste a small public playlist URL (2–3 items), uncheck `dump_video`, leave `enqueue_mpd` unchecked → submit. Watch `/api/status` update with progress; confirm files land at `/media/music/youtube/<uploader>/<playlist>/<title> (<id>).m4a`, and IDs appear in `/media/music/youtube/archive.txt`. Re-submit the same URL → no re-download.
6. Repeat with `dump_video` checked → `.mkv` produced with `bestvideo+bestaudio`.
7. Repeat with `enqueue_mpd` checked → `mpc playlist` on ithilien shows the new files; playback advances through them.
8. Confirm `~/yt-daily-player/` container logs show no regressions in cron-driven `RunJob` (existing scheduler path untouched).
9. Curl `GET /api/status` with basic auth → JSON well-formed.
10. Negative: set `web_ui.enabled: true` with empty `password` → binary refuses to start.

## Implementation subagents (per global instruction)

After plan approval, execution is split across 4–5 foreground subagents, each committing per-fix on `master` (lootek repo, no MRs). Each subagent `cd`s into the repo first.

1. **Config + main wiring** — `internal/config/config.go`, `main.go`, `config.example.yaml`, `docker-compose.yaml` comment.
2. **ytdlp.DownloadURL** — `internal/ytdlp/ytdlp.go` (new method, no regression to existing `Download`).
3. **player.Enqueue** — `internal/player/mpd_player.go` (new helper next to `PlayWithMPD`).
4. **webui package** — `internal/webui/{server,service,history,templates}.go` + embedded templates/static.
5. **End-to-end verify on ithilien** — build, deploy, run the verification checklist above, fix anything that surfaces.

## Out of scope

- No cron-job creation from the UI (MPD checkbox is one-shot enqueue only).
- No per-job item limit field (everything is downloaded; `--download-archive` is the dedup).
- No multi-user / session auth (single basic-auth credential pair from config).
- No HTTPS (LAN-only; terminate at reverse proxy if ever needed).
- No changes to the existing scheduler/cron path or its naming pattern.

## After approval

Copy this plan to `~/ai/yt-player-scheduler/glm-5.2-cloud.md` (first execution step), then proceed with subagents.