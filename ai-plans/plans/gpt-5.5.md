# Plan: add on-demand YouTube download web UI

## Context

At revision `bfe645af23127578dea7ee3f1caf3ae98f19442b`, `yt-player-scheduler` is a Go cron service that searches YouTube by configured keywords plus the current date, then either downloads audio or resolves a stream and plays it via MPD or a player command. The requested change adds an on-demand web UI to the same service so a user can paste a YouTube video/channel/playlist URL and download it into a configured library directory, optionally adding/playing the result through MPD and optionally keeping video instead of audio-only.

This plan targets the exact revision above, not current HEAD or the newer deployed copy on `pi@ithilien`.

## Current behavior at the target revision

- `main.go:19` defines `-config`, `-run-now`, loads YAML, creates `app.New`, schedules jobs with `robfig/cron`, starts cron, and blocks on signal.
- `internal/config/config.go:19` defines `Config`, `GlobalConfig`, `MPDConfig`, and `YtDLPConfig`; `Load` currently rejects configs with no jobs at `internal/config/config.go:66`.
- `internal/app/app.go:29` implements `RunJob`: build dated query, search with `ytdlp.Client.Search`, download through `ytdlp.Client.Download` when `global.ytdlp.download_dir` is set, then play through MPD/player.
- `internal/ytdlp/ytdlp.go:39` implements `Download`; it currently extracts m4a audio using `--audio-format m4a`, writes to `%(uploader)s - %(title)s [%(id)s].%(ext)s`, and prints final paths via `--print after_move:filepath`.
- `internal/player/mpd_player.go:16` implements `PlayWithMPD`, including `client.Update` and converting paths under MPD `music_root` to relative URIs before `AddID`/`PlayID`.
- Legacy `pi@ithilien:~/scripts/yt.sh` uses `/usr/local/bin/youtube-dl -i --download-archive archive.txt -f bestvideo+bestaudio --merge-output-format mkv --add-metadata -a <(bash ./list.sh) -o '%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'` from `/media/pi/music/youtube/`.

## Recommended implementation

### 1. Add web UI configuration

Modify `internal/config/config.go`:

- Add `WebUIConfig` under `GlobalConfig`:
  - `enabled bool` (`yaml:"enabled"`)
  - `listen string` (`yaml:"listen"`, default `:8080`)
  - `download_dir string` (`yaml:"download_dir"`, default empty; required when enabled)
  - `max_concurrent int` (`yaml:"max_concurrent"`, default `1` or `2`)
  - `timeout string` (`yaml:"timeout"`, default `2h`)
  - optional `username` / `password` basic auth fields only if desired for LAN exposure; otherwise omit for first pass.
- Change `Load` validation so `jobs` may be empty when `global.web_ui.enabled` is true. Keep rejecting configs with neither jobs nor enabled web UI.
- Keep scheduled job `global.ytdlp.download_dir` separate from web downloads. For `ithilien`, configure `global.web_ui.download_dir: /media/music/youtube`.

Update `config.example.yaml` with:

```yaml
global:
  web_ui:
    enabled: true
    listen: ":8080"
    download_dir: "/media/music/youtube"
    max_concurrent: 1
    timeout: 2h
```

### 2. Generalize yt-dlp download support

Modify `internal/ytdlp/ytdlp.go` without breaking `RunJob`:

- Introduce a request struct, e.g.:

```go
type DownloadRequest struct {
    URL         string
    DownloadDir string
    IncludeVideo bool
}
```

- Add `DownloadURL(ctx, req)` returning `[]string` of final file paths.
- Keep existing `Download(ctx, videoURL, jobName)` as a wrapper for scheduled jobs, preserving current audio-only behavior unless deliberately changed.
- For web downloads, use legacy-compatible yt-dlp flags:
  - Always include `-i` / `--ignore-errors` so channel/playlist downloads continue past broken entries.
  - Always include `--download-archive <download_dir>/archive.txt`.
  - Always include `--add-metadata`.
  - Always use legacy naming pattern rooted in the configured directory: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`.
  - Always include `--print after_move:filepath` to collect downloaded paths.
- For `IncludeVideo == true`, use legacy video behavior:
  - `-f bestvideo+bestaudio`
  - `--merge-output-format mkv`
- For `IncludeVideo == false`, use music-only behavior aligned with current service defaults:
  - `-x`
  - `--audio-format m4a`
  - same archive, metadata, and output pattern.
- Continue reusing `baseArgs()` and `prepareCookies()` so configured cookies/user-agent/PO token/remote components behave consistently for scheduled and web downloads.
- Ensure the method creates `DownloadDir` and returns every non-empty printed file path, not just the last line, because playlists/channels can download multiple items.

### 3. Add web package with minimal server and embedded HTML

Add `internal/web/server.go` and optionally `internal/web/templates.go`:

- Use only Go stdlib `net/http`, `html/template`, `sync`, and existing project packages.
- Server fields:
  - config (`config.WebUIConfig`, `config.MPDConfig`, shared `config.YtDLPConfig`)
  - `ytdlp.Client`
  - logger
  - semaphore channel sized by `max_concurrent`
  - in-memory recent job list/status map.
- Routes:
  - `GET /` renders a single-page form.
  - `POST /downloads` validates URL, checkboxes, starts a background goroutine, redirects to `/`.
  - `GET /status` or embedded status section renders recent jobs; avoid WebSockets for first implementation.
- Form fields:
  - URL input for YouTube video/channel/playlist.
  - Checkbox `Update MPD library after download`, default unchecked.
  - Checkbox `Add to MPD queue after download`, default unchecked.
  - Checkbox `Play through MPD after download`, default unchecked.
  - Checkbox `Download video (MKV); unchecked means music-only M4A`, default unchecked.
- Validation:
  - parse URL with `net/url`.
  - accept only `youtube.com`, subdomains of `youtube.com`, and `youtu.be`.
  - reject empty URL and disabled/missing download directory.
- Background job behavior:
  - create context with `web_ui.timeout`.
  - call `ytdlp.DownloadURL` with `download_dir` from web config and `IncludeVideo` from checkbox.
  - record status: queued/running/done/failed, started/finished times, requested URL, mode, final paths, error.
  - cap history to a small fixed size such as 20 jobs.

### 4. MPD integration for web downloads

Add lower-level MPD helpers in `internal/player/mpd_player.go` instead of overloading the existing scheduled-player path:

- Extract path conversion and `client.Update` behavior from `PlayWithMPD` into reusable helpers.
- Add `UpdateMPD(ctx, cfg, downloadDir, path) error` for the `Update MPD library` checkbox.
- Add `QueueWithMPD(ctx, cfg, downloadDir, path) error` for the `Add to MPD queue` checkbox.
- Keep `PlayWithMPD` for scheduled jobs and use it only when `Play through MPD` is checked.
- Pass `downloadDir = global.web_ui.download_dir` so `/media/music/youtube/...` under `mpd.music_root: /media/music` maps to MPD-relative `youtube/...` URIs.
- Default all MPD-related checkboxes to false.
- If any MPD checkbox is checked, require usable MPD config; otherwise mark the job failed with a clear error.
- If multiple playlist/channel files are downloaded, apply selected MPD actions to each file in downloaded-path order. If `Play through MPD` is checked, use current blocking semantics; otherwise queue/update should return quickly.

### 5. Wire server into `main.go`

Modify `main.go`:

- After `application := app.New(...)`, if `cfg.Global.WebUI.Enabled`, construct the web server with shared config and logger.
- Start it in a goroutine with `http.Server.ListenAndServe`.
- On shutdown, call `Shutdown` with a short context before stopping cron.
- Keep cron jobs exactly as they are.
- Allow configs with web UI only, jobs only, or both.

### 6. Deployment updates for `ithilien`

Modify `docker-compose.yaml` at the target repo revision:

- Keep `network_mode: host` so `:8080` is reachable on the Pi LAN and MPD at `192.168.10.22:6600` or localhost remains reachable.
- Mount the full YouTube library path, not only the scheduled cache:

```yaml
- /media/music/youtube:/media/music/youtube
```

- Keep `/app/config.yaml:ro` and cookies mount as currently used.

Update `config.example.yaml` and deployed `config.yaml` guidance so:

```yaml
mpd:
  enabled: true
  address: 192.168.10.22:6600
  music_root: "/media/music"
ytdlp:
  download_dir: /media/music/youtube/yt-rpi-player-cache/brewiarz
web_ui:
  enabled: true
  listen: ":8080"
  download_dir: "/media/music/youtube"
```

No implementation should copy behavior from deployed HEAD’s existing `web_ui` config beyond using it as deployment context.

### 7. Tests and verification

Add tests where possible:

- `internal/config/config_test.go`:
  - defaults for web UI listen/timeout/max concurrency.
  - config with web UI enabled and no jobs loads.
  - config with neither jobs nor web UI still errors.
- `internal/ytdlp/ytdlp_test.go`:
  - factor argument construction into a small helper so tests can assert video mode includes `-f bestvideo+bestaudio`, `--merge-output-format mkv`, archive path, metadata, and legacy output pattern.
  - assert audio mode includes `-x --audio-format m4a` and the same archive/output pattern.
- `internal/web/server_test.go`:
  - URL validation accepts YouTube/youtu.be and rejects other hosts.
  - handler rejects empty/non-YouTube URLs.

Manual end-to-end verification:

1. Build/test locally: `go test ./...` and `go build ./...`.
2. Run locally with a temp config enabling `web_ui` and `download_dir` pointing to a session scratch directory.
3. Open `http://localhost:8080`, submit a known short YouTube video with music-only unchecked/checked as appropriate, confirm files land under uploader/playlist/title naming.
4. On `ithilien`, after deployment, run through Docker Compose and test:
   - music-only download to `/media/music/youtube/...`.
   - video download produces `.mkv` with archive/metadata behavior.
   - MPD checkboxes default off.
   - Update-only refreshes MPD library without queueing or playing.
   - Queue-only adds a downloaded file without interrupting playback.
   - Play-now plays a downloaded file and MPD can resolve it relative to `/media/music`.
