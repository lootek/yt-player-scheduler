# Plan: Web UI for yt-rpi-player

## Context

The service in `~/projects/lootek/yt-player-scheduler` is a small Go cron scheduler that searches YouTube daily (keyword + date) and plays audio through MPD or `ffplay`. It is deployed on `pi@ithilien` as `~/yt-daily-player/` and already uses `/media/music/youtube/yt-rpi-player-cache/brewiarz` for scheduler downloads.

We want to add a lightweight web UI so a user can paste a YouTube video, channel, or playlist link and download it to a configurable directory. Two checkboxes control behaviour:

- **Queue in MPD** — add downloaded file(s) to the MPD playlist.
- **Auto-play now** — only meaningful with "Queue in MPD"; when checked, start playing the first added item immediately, otherwise only append.
- **Download video too** — when checked, download video+audio as MKV using the legacy script's options; when unchecked, audio-only M4A.

The legacy `~/scripts/yt.sh` uses:

```bash
cd /media/pi/music/youtube/
youtube-dl -i --download-archive archive.txt \
  -f bestvideo+bestaudio --merge-output-format mkv --add-metadata \
  -o '%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'
```

The web UI should mirror that naming pattern and archive behaviour, writing into a dedicated `web_ui.download_dir` defaulting to `/media/music/youtube` on the Pi.

## Decisions already made with the user

1. **Integration**: same binary, opt-in via a `-web-ui` flag, running alongside the scheduler.
2. **Download directory**: separate `web_ui.download_dir` config field (Pi default `/media/music/youtube`).
3. **MPD behaviour**: two checkboxes — "Queue in MPD" and "Auto-play now"; append-only when auto-play is off.

## Recommended implementation

### 1. Configuration

Add a `WebUIConfig` struct in `internal/config/config.go` and a top-level `WebUI` field.

```go
type Config struct {
    Global GlobalConfig `yaml:"global"`
    Jobs   []JobConfig  `yaml:"jobs"`
    WebUI  WebUIConfig  `yaml:"web_ui"`
}

type WebUIConfig struct {
    Enabled     bool   `yaml:"enabled"`
    ListenAddr  string `yaml:"listen_addr"`
    DownloadDir string `yaml:"download_dir"`
}
```

Defaults:

```go
if cfg.WebUI.ListenAddr == "" {
    cfg.WebUI.ListenAddr = ":8080"
}
```

`DownloadDir` has **no hard-coded default**; it must be provided in `config.yaml` when the web UI is enabled. The Pi deployment will set it to `/media/music/youtube`.

Validation: move the "no jobs configured" fatal error out of `config.Load` and into `main.go`, so the app can run in web-only mode without scheduled jobs.

Update `config.example.yaml`:

```yaml
web_ui:
  enabled: false
  listen_addr: ":8080"
  download_dir: "/media/music/youtube"
```

### 2. Main entry point

Add a flag in `main.go`:

```go
webUI := flag.Bool("web-ui", false, "start the web UI server alongside the scheduler")
```

After loading config, set `cfg.WebUI.Enabled = cfg.WebUI.Enabled || *webUI`. If enabled, create the web UI server and start it in a goroutine. Use a second goroutine to call `srv.Shutdown` when the signal context is cancelled. The cron scheduler still starts only when jobs exist.

### 3. yt-dlp download helper

Add a new method in `internal/ytdlp/ytdlp.go` for arbitrary URL downloads:

```go
func (c Client) DownloadURL(ctx context.Context, downloadDir, url string, video bool, outLog io.Writer) ([]string, error)
```

Behaviour:

- `os.MkdirAll(downloadDir, 0755)` if needed.
- Reuse `c.prepareCookies()` and `c.baseArgs()` so cookies, PO tokens, user-agent, and `extra_args` are inherited from `global.ytdlp`.
- Output template: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s` inside `downloadDir`.
- Archive: `--download-archive <downloadDir>/archive.txt`.
- `-i` / `--ignore-errors` for playlists/channels.
- If `video`:
  - `-f bestvideo+bestaudio --merge-output-format mkv --add-metadata`
- Else:
  - `-x --audio-format m4a`
- Run `yt-dlp` with a timeout context.
- Capture file paths printed by `--print after_move:filepath` from stdout; stream stderr (and any non-file stdout) to `outLog` so the UI shows progress.

### 4. MPD queue helper

Create `internal/player/mpd_queue.go` with a non-blocking helper:

```go
func QueueInMPD(cfg config.MPDConfig, musicRoot string, files []string, autoplay bool) error
```

- Dial MPD (optionally authenticated).
- For each file under `musicRoot`, call `client.Update(<parent path>)` once per distinct directory, wait/poll for the update to finish, then call `client.AddID(uri, -1)` using a path relative to `musicRoot`.
- If `autoplay`, call `client.PlayID(id)` on the first added item.
- Do **not** block monitoring playback; return immediately after queueing.

This reuses the existing MPD path-mapping logic from `mpd_player.go` but is fire-and-forget.

### 5. Web UI package

Create `internal/webui/`:

- `jobs.go` — in-memory job manager (`pending` → `running` → `done`/`failed`) with per-job log buffer. Jobs are keyed by a short ID.
- `server.go` — `http.Server` with three routes:
  - `GET /` — HTML form.
  - `POST /download` — validate URL, create a job, spawn a goroutine, return `{id, state}`.
  - `GET /status/{id}` — return `{id, state, log, files, error}`.
- `templates.go` — embedded HTML + a small polling script. No external build.
- `worker.go` — run the download, stream yt-dlp output to the job log, then optionally queue in MPD.

The worker receives the main signal context so shutdown cancels active downloads.

Add an accessor in `internal/app/app.go` so the web UI can call the existing yt-dlp client:

```go
func (a *App) YTDLP() ytdlp.Client { return a.ytdlp }
```

### 6. Docker / deployment

`docker-compose.yaml`:

- Keep `network_mode: host`.
- Mount `/media/music/youtube:/media/music/youtube` (in addition to the existing scheduler cache mount) so `web_ui.download_dir` resolves to the same host path MPD sees.
- Add `EXPOSE 8080` to the `Dockerfile` for documentation.

Pi `config.yaml` should set:

```yaml
web_ui:
  enabled: true
  listen_addr: "0.0.0.0:8080"
  download_dir: "/media/music/youtube"
global:
  mpd:
    music_root: "/media/music"
```

MPD `music_root` must be a parent of `download_dir` so relative URIs work.

### 7. Security / safety

- URL is passed directly as an `exec.Command` argument to `yt-dlp`; no shell interpolation, so shell injection is not possible.
- No authentication in v1. Keep the default `listen_addr` as `:8080`; on the Pi use host firewall or bind to `127.0.0.1:8080` + SSH tunnel if desired.
- Concurrent downloads are allowed; they share `archive.txt` in the download dir. Accept this for v1; if archive races appear, serialize downloads in the job manager.
- Per-job timeout (e.g. 2 hours) to prevent runaway channel/playlist downloads.

## Verification steps

1. `go build .` succeeds.
2. Run `./yt-rpi-player -config config.example.yaml -web-ui` and open `http://127.0.0.1:8080/`.
3. Submit a single video, audio-only, queue + auto-play:
   - Status transitions `pending` → `running` → `done`.
   - File appears at `<download_dir>/<uploader>/<playlist>/<title> (<id>).m4a`.
   - `archive.txt` is updated.
   - MPD playlist contains the item and playback starts.
4. Submit a playlist with "Download video too" checked:
   - `.mkv` files are produced.
   - Files are queued but not auto-played when auto-play is unchecked.
5. Submit a link with "Queue in MPD" unchecked:
   - Files download, MPD playlist is untouched.
6. Run without `-web-ui` and confirm existing scheduled jobs still work.
7. Build Docker image, deploy to `ithilien`, and verify via browser on the LAN.

## Files to modify / create

- `internal/config/config.go` — add `WebUIConfig` and update defaults/validation.
- `config.example.yaml` — add `web_ui` section.
- `main.go` — add `-web-ui` flag, wire server lifecycle.
- `internal/app/app.go` — add `YTDLP()` accessor.
- `internal/ytdlp/ytdlp.go` — add `DownloadURL` for video/channel/playlist downloads.
- `internal/player/mpd_queue.go` — new fire-and-forget MPD queue helper.
- `internal/webui/jobs.go` — new in-memory job manager.
- `internal/webui/server.go` — HTTP routes and handlers.
- `internal/webui/templates.go` — embedded HTML form.
- `internal/webui/worker.go` — background download worker.
- `docker-compose.yaml` — mount `/media/music/youtube` and expose port documentation.
- `Dockerfile` — optional `EXPOSE 8080`.
- `README.md` — document the new web UI flag and config.
