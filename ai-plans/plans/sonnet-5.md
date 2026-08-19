# Add ad-hoc web UI for YouTube downloads to yt-player-scheduler

## Context

`yt-player-scheduler` (repo `~/projects/lootek/yt-player-scheduler`, module `github.com/lootek/yt-rpi-player`) at pinned revision `bfe645af23127578dea7ee3f1caf3ae98f19442b` is a pure CLI cron-scheduler: it periodically searches YouTube for keyword+date matches and plays them via MPD/mpv/a generic player command. It has no HTTP server anywhere in this revision.

The goal is to add a small, self-hosted web UI so a user can paste a YouTube video/channel/playlist link and have it downloaded to a configured directory on demand, independent of the cron-scheduled keyword jobs — with an option to also queue the result into MPD. This must reuse the download conventions of the legacy `~/scripts/yt.sh` (naming pattern, `-i`/archive-file dedup, video vs audio handling) since that's the established convention on the target box (`pi@ithilien`, dedicated destination `/media/music/youtube`).

Note: the same repo's current `HEAD` (`e6800b5`, deployed live on ithilien) already has its own `internal/webui/` implementation of a very similar feature — by explicit decision, this plan designs a fresh implementation on top of the pinned commit and does not read or reuse that newer code.

**Confirmed decisions** (via clarifying questions):
- Web UI requires HTTP Basic Auth, config-driven (optional username/password; if both set, auth is enforced).
- "Add to MPD" checkbox → after download, append file(s) to MPD's play queue (`AddID`) in order; do **not** force/block playback like the existing `PlayWithMPD`.
- Channel links download in full; dedup relies on a shared `--download-archive` file across resubmissions (same convention as legacy `yt.sh`) — no artificial per-submission cap.
- Job history persists across restarts via an append-only JSONL log.
- No new Go dependencies — stdlib only (`net/http` w/ Go 1.22+ pattern routing, `html/template`, `crypto/subtle`, `encoding/json`). Go is already at 1.24.0.

## 1. Config: `internal/config/config.go`

Add to `GlobalConfig`:
```go
WebUI WebUIConfig `yaml:"web_ui"`

type WebUIConfig struct {
    Enabled     bool   `yaml:"enabled"`
    ListenAddr  string `yaml:"listen_addr"`  // default ":8081"
    DownloadDir string `yaml:"download_dir"` // required if enabled; independent of ytdlp.download_dir
    ArchiveFile string `yaml:"archive_file"` // default "<download_dir>/archive.txt"
    HistoryFile string `yaml:"history_file"` // default "<download_dir>/webui-history.jsonl"
    Concurrency int    `yaml:"concurrency"`  // default 1
    JobTimeout  string `yaml:"job_timeout"`  // default "4h"
    Username    string `yaml:"username"`
    Password    string `yaml:"password"`
}
```
- `applyDefaults()`: fill `ListenAddr`/`Concurrency`/`JobTimeout`, derive `ArchiveFile`/`HistoryFile` from `DownloadDir` when unset.
- `Load()`: relax `len(cfg.Jobs) == 0` to only error when `!cfg.Global.WebUI.Enabled` too — a web-UI-only config (no cron jobs) becomes valid. Add a new check: `WebUI.Enabled && DownloadDir == ""` → error at load time.

## 2. yt-dlp integration: `internal/ytdlp/ytdlp.go` (additive, `Download()` untouched)

New method reusing existing `c.baseArgs()`, `c.binary()`, `c.prepareCookies()`:
```go
type DownloadMode int
const ( ModeAudio DownloadMode = iota; ModeVideo )

func (c Client) DownloadBatch(ctx context.Context, url, archivePath string, mode DownloadMode, onOutput func(line string)) ([]string, error)
func buildDownloadArgs(cfg config.YtDLPConfig, mode DownloadMode, archivePath, outputTemplate, resultsFile, cookiePath string) []string // pure, unit-testable
```

Output template (both modes), matching legacy `yt.sh` naming, using the `%(playlist_title|)s` empty-default form to avoid a literal `NA` directory for lone videos:
```
<download_dir>/%(uploader)s/%(playlist_title|)s/%(title)s (%(id)s).%(ext)s
```

**Audio-only** (checkbox unchecked, default):
```
yt-dlp <baseArgs...> -i --download-archive <archive> -x --audio-format m4a --add-metadata \
  --output "<template>" --print-to-file after_move:filepath <results-tmp> --no-warnings \
  [--cookies <tmp>] <url>
```
**Video** (checkbox checked):
```
yt-dlp <baseArgs...> -i --download-archive <archive> -f bestvideo+bestaudio --merge-output-format mkv --add-metadata \
  --output "<template>" --print-to-file after_move:filepath <results-tmp> --no-warnings \
  [--cookies <tmp>] <url>
```
Use `--print-to-file` (write results to a temp file) instead of legacy's stdout `--print`, so stdout stays available for live progress and N-file results parsing isn't the "take the last line" hack the existing `Download()` uses. Create the results file via `os.CreateTemp(downloadDir, "results-*.txt")`, read+remove after `cmd.Wait()`. Stream stdout/stderr lines to `onOutput` for the job's live log.

Error semantics: `cmd.Run()` success → all good. Error but results file non-empty → partial success (some items in a channel/playlist legitimately failed, `-i` kept going). Error and results file empty → real failure.

## 3. MPD: `internal/player/mpd_player.go` (extract, don't rewrite `PlayWithMPD`)

Extract two helpers used internally by the existing (untouched-behavior) `PlayWithMPD`:
```go
func dialMPD(cfg config.MPDConfig) (*mpd.Client, error)
func relativizeUnderMusicRoot(downloadDir, musicRoot, uri string) (rel string, ok bool)
```
Add new non-blocking batch function:
```go
// Appends paths to MPD's queue in order (AddID), no PlayID, no blocking on playback.
// Triggers one `update` for the batch's common uploader directory and polls
// Status()["updating_db"] until it clears (bounded ~2min timeout) instead of a blind sleep.
func EnqueueFiles(ctx context.Context, cfg config.MPDConfig, downloadDir string, paths []string) (ids []int, err error)
```

## 4. New package `internal/webui/`

```
internal/webui/
  server.go      // Server, New(), Handler(), RunWorkers()
  auth.go        // BasicAuthMiddleware (pass-through if user/pass unset; crypto/subtle compare)
  job.go         // Job, JobState (queued/running/done/failed)
  store.go       // in-memory index + JSONL append/replay
  worker.go      // Pool: bounded worker goroutines over a buffered job channel
  downloader.go  // Downloader interface + ytdlp.Client adapter
  handlers.go    // handleIndex, handleSubmit, handleJobs, handleJobDetail
  templates.go   // go:embed templates/*.tmpl
  templates/{index,jobs,job_detail}.tmpl
```

Routes (Go 1.22+ `ServeMux`): `GET /` (submit form), `POST /submit` (validate → `store.Create` → `pool.Enqueue` → redirect `/jobs/{id}`), `GET /jobs` (list), `GET /jobs/{id}` (detail).

Job state machine: `queued → running → {done|failed}`. Worker: `SetRunning` → `dl.DownloadBatch` (with its own `job_timeout`-bounded context, independent of the cron path's `player.timeout`) → if `AddToMPD && mpd.Enabled`: `player.EnqueueFiles` (log-only on error, doesn't flip a successful download to failed) → `SetDone`/`SetFailed`.

JSONL event log (one line per state transition, never rewritten):
```json
{"job_id":"...","ts":"...","event":"queued","url":"...","video":false,"add_to_mpd":true}
{"job_id":"...","ts":"...","event":"done","files":["..."],"mpd_enqueued":true}
```
`NewStore` replay: group by `job_id`, keep latest event; any job still `queued`/`running` at load time gets a synthetic `failed`/"interrupted by restart" event appended (keeps history strictly append-only, self-heals on the next start). Live per-job log lines are in-memory only (capped ring buffer), not persisted.

Pages: submit form (URL + "Download video" checkbox + "Add to MPD queue" checkbox), job list (table with auto-refresh meta tag while anything is active), job detail (metadata, state, resulting files, error, tailing log). Plain `html/template`, no JS framework.

## 5. `main.go` + wiring

- If `cfg.Global.WebUI.Enabled`: build a `webui.Server` (its own `ytdlp.Client` pointed at `WebUI.DownloadDir`, independent of the cron jobs' `YtDLP.DownloadDir`), start `RunWorkers(ctx)` and `http.Server.ListenAndServe()` in goroutines.
- Graceful shutdown: `webServer.Shutdown(ctx)` alongside the existing `c.Stop()` on the existing `signal.NotifyContext`.
- No `go.mod`/`Dockerfile` changes needed.

## 6. Deployment on ithilien

- `docker-compose.yaml`: widen the volume mount from the narrow `.../yt-rpi-player-cache/brewiarz` path to all of `/media/music/youtube:/media/music/youtube:rw` (same path both sides; old narrow path is a subdirectory so nothing regresses). `network_mode: host` already exposes any new listener without a `ports:` entry.
- `config.yaml`: add
  ```yaml
  web_ui:
    enabled: true
    listen_addr: ":8081"
    download_dir: /media/music/youtube
    archive_file: /media/music/youtube/archive.txt   # same file legacy yt.sh already uses
    history_file: /media/music/youtube/webui-history.jsonl
    concurrency: 1
    job_timeout: 4h
    username: "<set>"
    password: "<set>"
  ```
  Confirm `mpd.music_root: /media/music` (already correct per current deployed config).

## 7. Verification

**Unit tests (no real yt-dlp/MPD needed):**
- `config.Load()`: zero-jobs+disabled → error; zero-jobs+enabled → ok; defaulting.
- `ytdlp.buildDownloadArgs`: exact arg-slice assertions for both modes.
- `player.relativizeUnderMusicRoot`: table tests.
- `webui.Store`: JSONL round-trip in a temp dir; simulate a crash mid-`running` and verify the reconciliation event on reopen.
- `webui.Pool`: fake `Downloader` (canned success/failure/delay) to verify concurrency limiting and state transitions.
- `webui.auth`: `httptest` — no creds configured (pass-through), correct creds (200), wrong/missing (401).
- `webui.handlers`: `httptest.NewServer` with a fake downloader — `/`, `/submit`, `/jobs`, `/jobs/{id}` status codes and content.

**Manual, local machine, real yt-dlp:**
- Point `web_ui.download_dir` at a scratch dir; submit a short public video (audio mode). Confirm file lands at `<dir>/<uploader>/<title> (<id>).m4a` with no `NA` directory, `archive.txt` gains a line, job shows `done`.
- Resubmit the same URL in video mode — expect it to be skipped by the shared archive (yt-dlp keys on `<extractor> <id>` regardless of format); confirm this is the expected behavior, not a bug.
- Submit a small 2-3 video playlist; confirm all files are captured and listed in order.

**End-to-end on ithilien after deploy:**
1. `docker compose up -d --build`.
2. `curl http://ithilien:8081/` → `401`; `curl -u user:pass ...` → `200`.
3. Submit a short test video via the browser with "add to MPD" checked, audio mode.
4. Confirm the file lands under `/media/music/youtube/<uploader>/...` correctly named, `archive.txt` updated.
5. `mpc -h ithilien queue` shows the track appended without interrupting current playback (validates non-blocking `EnqueueFiles`).
6. `cat` the configured `webui-history.jsonl` — `queued`/`done` lines present.
7. Mid-download of a small playlist, `docker compose restart yt-rpi-player`: container restarts cleanly, the interrupted job shows `failed`/"interrupted by restart", and existing cron jobs are unaffected (regression check on the `mpd_player.go` extraction).

### Critical files
- `internal/config/config.go`
- `internal/ytdlp/ytdlp.go`
- `internal/player/mpd_player.go`
- `main.go`
- `docker-compose.yaml`, `config.example.yaml`
- New: `internal/webui/*`
