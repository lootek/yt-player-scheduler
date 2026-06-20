# Plan: Add a web UI for on-demand YouTube downloads to yt-daily-player

## Context

`github.com/lootek/yt-rpi-player` (deployed on `ithilien` as `~/yt-daily-player`, container `yt-rpi-player`, host networking) is today a **headless cron scheduler**: `main.go` loads YAML, registers cron jobs, and on each tick searches YouTube by keyword+date and plays/downloads the top hit. There is no interactive entry point.

The user wants an **on-demand web UI**: paste a YouTube **video / channel / playlist** URL and have it downloaded to a configurable directory on the Pi. Downloading must optionally (per a checkbox) enqueue the result into MPD, and a second checkbox decides **video vs. music-only**. This replaces the manual `~/scripts/yt.sh` + `list.sh` workflow with a friendly UI, while keeping the existing daily cron player untouched.

### Established facts (from exploration)
- Repo: single Go module `github.com/lootek/yt-rpi-player`, Go 1.24, deps: `robfig/cron/v3`, `gopkg.in/yaml.v3`, `fhs/gompd/v2`. No HTTP framework — use stdlib `net/http`.
- `internal/ytdlp/ytdlp.go` already has `Download(ctx, videoURL, jobName) (path, error)` doing `-x --audio-format m4a`, template `%(uploader)s - %(title)s [%(id)s].%(ext)s`, and `--print after_move:filepath`. `baseArgs()` already injects cookies/po-token/extractor/runtime flags — **reuse it**.
- `internal/player/mpd_player.go` `PlayWithMPD(ctx, cfg, downloadDir, uri)` does update→AddID→**PlayID**→monitor. We need an enqueue-only variant (+ optional play).
- `internal/config/config.go`: `GlobalConfig` holds `Player`, `MPD`, `YtDLP` (with `DownloadDir`). MPD config has `MusicRoot`. `Load()` errors out if `len(Jobs)==0` — must relax so the service can run web-only.
- Legacy `~/scripts/yt.sh` (on ithilien): `youtube-dl -i --download-archive archive.txt -f bestvideo+bestaudio --merge-output-format mkv --add-metadata -o '%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'`. **Naming + option pattern to mirror.**
- Pi: aarch64, MPD 0.23.5 active, `music_directory "/media/music"`, target dir `/media/music/youtube` (560G free). MPD decoders cover m4a/aac, opus, mp3, flac, and full video via ffmpeg.
- **Container mount gap**: compose currently mounts only `…/yt-rpi-player-cache/brewiarz`, not all of `/media/music/youtube`. Web downloads to `/media/music/youtube` require mounting the full dir (same host path inside container so MPD relative paths line up).

### Decisions (confirmed with user)
- **MPD action** when "play through MPD" is ticked: **update DB + enqueue** (append to queue, no force-play); a **separate "auto-play" checkbox** triggers immediate play of the first added track.
- **Auth**: HTTP **Basic auth**, credentials from `config.yaml`.
- **Music-only format**: best audio MPD can decode on the Pi → keep **m4a/aac** (`-x --audio-format m4a`), consistent with existing `Download()`.
- **Concurrency**: **limited concurrent** downloads, worker count from config (default 2).

---

## Implementation

### 1. Config — `internal/config/config.go`
- Add `Web WebConfig` to `GlobalConfig`.
  ```go
  type WebConfig struct {
      Enabled       bool   `yaml:"enabled"`
      Listen        string `yaml:"listen"`          // default ":8080"
      Username      string `yaml:"username"`
      Password      string `yaml:"password"`
      DownloadDir   string `yaml:"download_dir"`    // default target, e.g. /media/music/youtube
      MaxConcurrent int    `yaml:"max_concurrent"`  // default 2
      Archive       string `yaml:"archive"`         // download-archive file; default <download_dir>/archive.txt
  }
  ```
- `applyDefaults`: set `Listen=":8080"`, `MaxConcurrent=2` when unset; default `DownloadDir` to `YtDLP.DownloadDir` if empty.
- **Relax `Load()`**: only error on `len(Jobs)==0` when `!Global.Web.Enabled` (allow web-only deployments with no cron jobs).

### 2. yt-dlp — `internal/ytdlp/ytdlp.go`
Add `DownloadMedia(ctx, req DownloadRequest) ([]string, error)` returning **all** downloaded file paths (channels/playlists yield many). Mirror legacy `yt.sh` options; build on existing `baseArgs()` and `prepareCookies()`.

```go
type DownloadRequest struct {
    URL         string
    DownloadDir string  // overrides cfg.DownloadDir per-request
    Video       bool    // true = video, false = music-only
    Archive     string  // download-archive path (skip already-downloaded)
}
```
Args:
- Common: `-i --add-metadata --no-warnings --download-archive <archive>`, `--output <template>`, `--print after_move:filepath`, plus cookies if configured.
- **Video** (`Video==true`): `-f bestvideo+bestaudio --merge-output-format mkv`, template `<dir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s` (legacy-exact).
- **Music-only** (`Video==false`): `-x --audio-format m4a -f bestaudio/best`, same nested template (ext → m4a).
- Collect every non-empty `after_move:filepath` line into the returned slice (current `Download()` takes only the last line; the new method keeps all). Stream stdout with `bufio.Scanner` for progress, like `Search()`.
- `%(playlist_title)s` resolves to `NA` for single videos — acceptable; matches legacy behavior.

### 3. MPD — `internal/player/mpd_player.go`
Add `EnqueueMPD(ctx, cfg, downloadDir string, uris []string, autoPlay bool) error`:
- Dial (reuse auth logic), then for each uri: apply the same `MusicRoot`-prefix→relative mapping already in `PlayWithMPD`, `client.Update(rel)` (once per distinct dir), `client.AddID(rel, -1)`.
- If `autoPlay`, `client.PlayID(firstID)` once; **do not block-monitor** (web request returns promptly).
- Factor the path-mapping + update snippet out of `PlayWithMPD` into a small helper to avoid duplication.

### 4. New package — `internal/web/`
- `server.go`: stdlib `net/http`. Routes:
  - `GET /` — render HTML form (single self-contained template, no JS framework): URL input; checkboxes **Download video** (default off → music-only), **Add to MPD** (default on), **Auto-play** (default off, only meaningful with MPD); download-dir text field prefilled from `WebConfig.DownloadDir`.
  - `POST /download` — parse form, build `downloader.Request`, submit to queue, redirect to `/` with a flash/redirect to `/status`.
  - `GET /status` — list active/queued/finished jobs with state + last progress line (poll-refresh via `<meta http-equiv=refresh>` for simplicity; no SSE needed).
- `auth.go`: Basic-auth middleware wrapping all routes; constant-time compare against `WebConfig.Username`/`Password`. Skip if both empty (logs a warning).
- Templates inline via `embed` or string consts (keep dependency-free).

### 5. Download queue — `internal/downloader/`
- `queue.go`: worker pool of `WebConfig.MaxConcurrent` goroutines consuming a buffered channel of `Request`. Each `Request` carries URL, Video, MPD/autoplay flags, target dir.
- Per job: call `ytdlp.DownloadMedia`, then if MPD requested call `player.EnqueueMPD` with returned paths.
- Track job state (`queued`/`running`/`done`/`error`, progress line, file count) in a mutex-guarded map exposed to the web layer. Bounded history.

### 6. Wiring — `main.go`
- After building `app`, if `cfg.Global.Web.Enabled`: construct the downloader queue + `web.Server`, run `server.ListenAndServe` in a goroutine, shut down on ctx cancel (`server.Shutdown`).
- Cron setup stays; when no jobs and web enabled, skip cron registration gracefully.

### 7. Deployment — `docker-compose.yaml` (+ README)
- Replace the brewiarz-only mount with the **full** dir, same path both sides so MPD relative paths resolve:
  `- /media/music/youtube:/media/music/youtube`
- Web port is reachable via existing `network_mode: host`; document `web.listen: ":8080"` and that the UI is at `http://ithilien.local:8080`.
- `config.example.yaml` / `config.yaml`: add a `web:` block (enabled, listen, username/password, download_dir `/media/music/youtube`, max_concurrent, archive) and set MPD `music_root: /media/music`.

---

## Subagent split (implementation)
1. **Config + main wiring** — `internal/config/config.go`, `main.go`, example/real YAML.
2. **yt-dlp download** — `internal/ytdlp/ytdlp.go` (`DownloadMedia`).
3. **MPD enqueue** — `internal/player/mpd_player.go` (`EnqueueMPD` + refactor).
4. **web + downloader** — `internal/web/`, `internal/downloader/`.
5. **deploy/docs** — `docker-compose.yaml`, `README.md`, configs.
Each as a separate commit (per repo conventions); `github.com/lootek` → commit straight to `main`, GPG-signed, no Jira/MR.

## Verification
- `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build .` locally; `go vet ./...`.
- Local smoke: run with a web-enabled `config.yaml` (no MPD), submit a single short video URL with **music-only**, confirm an `.m4a` lands in `download_dir` with the legacy nested naming, and the archive file gets the id.
- Submit a small **playlist** → multiple files; confirm queue respects `max_concurrent`.
- On ithilien: rebuild container (`docker compose up --build -d`) with the full `/media/music/youtube` mount; from a LAN browser hit `http://ithilien.local:8080`, auth, submit a video with **Add to MPD** on → confirm `mpc playlist` shows the new track and (with auto-play) it starts; verify the daily cron job still fires.
- Confirm Basic auth rejects missing/wrong credentials (401).
