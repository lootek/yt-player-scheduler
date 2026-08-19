# Web UI for on-demand YT downloads — yt-player-scheduler @ bfe645af

## Context

`yt-player-scheduler` (Go, `github.com/lootek/yt-rpi-player`, at ~/projects/lootek/yt-player-scheduler) is a cron-driven service: keyword jobs → yt-dlp search → download (audio cache) → play through MPD (or mpv/ffplay). At rev `bfe645af` it has **no HTTP surface at all**. Deployed on pi@ithilien (`~/yt-daily-player`, docker, host networking, MPD on :6600 with `music_directory /media/music`) — but deployed is a *newer* rev; per user decision we **branch off `bfe645af`** and ignore master (which already has its own, different webui — expect heavy conflicts if ever merged; master stays untouched).

Goal: a web UI where the user pastes a YouTube URL (video / playlist / channel), it downloads to a config-provided dir, with two checkboxes:
- **music only** → native `bestaudio` (no re-encode; decided: `-f "bestaudio[ext=m4a]/bestaudio"`)
- **schedule for playing through MPD** → append to queue + override current playback + resume prior track at saved position when new block finishes (decided)

Legacy consistency (`~/scripts/yt.sh`): output template `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`, shared `archive.txt` at dir root, `-i`, `--add-metadata`; video mode `-f bestvideo+bestaudio --merge-output-format mkv`. Download dir on ithilien: `/media/music/youtube` (inside MPD's music root). Note: legacy tree shows `Uploader/NA/<title>` for single videos (`playlist_title`=NA) — consistent by definition, don't "fix".

Existing cron flow must keep working unchanged (its own download template/path untouched).

## Config (commit 1) — `internal/config/config.go` + `config.example.yaml`

```go
type WebUIConfig struct {
    Enabled       bool   `yaml:"enabled"`        // default false
    Listen        string `yaml:"listen"`         // default ":8080"
    Username      string `yaml:"username"`       // basic auth when both set
    Password      string `yaml:"password"`
    DownloadDir   string `yaml:"download_dir"`   // fallback: global.ytdlp.download_dir; error if neither
    MaxConcurrent int    `yaml:"max_concurrent"` // default 1
    Timeout       string `yaml:"timeout"`        // per-download-job duration, default "2h"
}
// add WebUI WebUIConfig `yaml:"web_ui"` to GlobalConfig; defaults in applyDefaults
```
`Load()` still requires ≥1 job (test configs carry a dummy job). Example yaml block added.

## yt-dlp download (commit 2) — new `internal/ytdlp/download_to.go` (+ test)

```go
func (c Client) DownloadTo(ctx context.Context, opts DownloadOptions) ([]string, error)
// DownloadOptions{URL, Mode (audio|video), DownloadDir, ArchivePath, OnFile func(string), Log io.Writer}
```
Args (pure `buildDownloadArgs` for unit test): reuse `baseArgs()` (extra_args/user-agent/po-token/remote-components/js-runtimes — keeps `player_client=mweb` etc.) + `-i --download-archive <ArchivePath> --add-metadata --no-progress --no-warnings --output <DownloadDir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s --print after_move:filepath` + mode flags + `--cookies` via existing `prepareCookies()` + URL. Stream stdout paths (multi-file: playlists/channels) via bufio.Scanner (pattern of `Search()`), stderr → Log in a goroutine (WG, avoids pipe deadlock). Semantics: exit 0 + 0 files = all archived (done, nothing new); exit≠0 = error incl. stderr tail (partial files returned, caller fails job, no enqueue).

## MPD enqueue + resume (commit 3) — new `internal/player/mpd_queue.go`

`EnqueueWithResume(ctx, cfg config.MPDConfig, downloadDir string, files []string, logf) error` (PlayWithMPD untouched). Verified gompd v2.3.0 facts: `Attrs map[string]string`; `Status()` keys `state/songid/elapsed/updating_db`; `AddID(uri,-1)`; `PlayID(id)`; `SeekSongID(id, dur)`; `Pause(bool)`; `Update(uri)` → job id, tolerate `mpd.ErrorUpdateAlready`; no ctx support, not concurrency-safe → one connection per call, own poll loop.

1. Relativize paths vs `MusicRoot` (extract shared helper with mpd_player.go:33-46); error if outside root.
2. `Update(commonAncestorDir)` + `waitForUpdate`: poll `Status()["updating_db"]` every 1s (cap 5m) — replaces blind 30s sleep.
3. Snapshot prior: `state`, `songid`, `elapsed`, was-paused.
4. `AddID` each file; `PlayID(first new)`.
5. Poll 2s: done when state=stop or current songid ∉ newIDs (manual skip = treated as done — documented).
6. Resume: `PlayID(priorID)` (+ log-skip if gone), `SeekSongID(priorPos)`, re-pause if was paused. Nothing-playing case: no resume step.

## Web UI (commit 4) — new `internal/webui/` + `main.go` wiring

Files: `server.go` (Server: Start(ctx)/Stop(), dir fallback resolution, net.Listen fail-fast, N download workers + 1 playback goroutine, http.Server shutdown), `jobs.go` (Store: mutex + map + order; Job{ID,URL,AudioOnly,Schedule,Status queued|running|playing|done|failed|canceled,Files,Error,Log≤200,timestamps}; cap 100 jobs), `queue.go` (per-job `context.WithTimeout(serverCtx, Timeout)` → DownloadTo with OnFile/Log into store), `playback.go` (serial consumer — MPD single stream; `EnqueueWithResume` without download timeout; only shutdown cancels), `api.go`, `static.go` (`//go:embed static/index.html`, vanilla JS, no new deps), `static/index.html` (form: URL + 2 checkboxes; job cards with status/file-count/log tail; poll `/api/jobs` 2s).

API (Go 1.22 patterns; go.mod is 1.24): `POST /api/download` `{"url","audio_only","schedule"}` → 202 `{"id"}`; 400 for non-YT host (exact-match allowlist: youtube.com/www/m/music, youtu.be); `GET /api/jobs` newest-first; `GET /` page. Optional basic auth (constant-time compare) when username+password set. Panic-recover middleware.

`main.go` (at bfe645af): after `app.New` — `if cfg.Global.WebUI.Enabled { webui.New(cfg, logger).Start(ctx); defer Stop() }`. Cron/-run-now/signals untouched.

## Docs (commit 5) — README + example config + compose note

Document web_ui config, MPD resume behavior, NA-dir note, LAN security note, and deployment delta: docker-compose must mount `/media/music/youtube:/media/music/youtube` **rw** (today only the brewiarz cache is mounted; archive.txt must be writable). Host network → :8080 reachable on LAN, no port mapping.

## Execution

Branch: `git -C ~/projects/lootek/yt-player-scheduler checkout -b webui bfe645af` (worktree clean; master/deployed untouched). One foreground subagent per commit, sequential, each cd'd to the repo (user rule: separate commit per concern); `go build ./... && go vet ./... && go test ./...` must pass per commit. No pushes/deploy without explicit go-ahead.

## Verification

1. Unit: `buildDownloadArgs` table test; URL-allowlist test; Store race test (`-race`).
2. Local e2e: scratch config (`web_ui{enabled, listen 127.0.0.1:18080, download_dir <scratch>, max_concurrent 1, timeout 10m}` + dummy job) → `go run . -config …`; curl POST real short video audio_only → poll jobs → assert `<dir>/<Uploader>/NA/<Title> (<id>).m4a` + archive.txt; re-POST → done, 0 files; POST vimeo URL → 400.
3. On ithilien (safe, deployed stack untouched): cross-compile `GOOS=linux GOARCH=arm64`, scp binary + test config (`listen :18081`, `download_dir /media/music/youtube/webui-test` → isolated archive.txt, real `music_root: /media/music`, mpd.enabled). Play long track via mpc; POST short video schedule:true; assert: jump to new track, then resume prior at saved position; repeat paused / idle / manual-skip cases. Cleanup: Ctrl+C (graceful), rm test dir, mpc update.
4. Real deploy (separate approval): compose volume rw, config.yaml web_ui section, rebuild image — replaces the currently deployed newer build on :8080.

## Post-approval bookkeeping

- Copy this plan to `~/ai/yt-ui-ext/glm-5.3.md` (model-named, per request).
- Write `~/ai/yt-ui-ext/.session` with `$CLAUDE_SESSION_ID`.

## Known limitations (accepted)

- gompd blocking calls can stall playback goroutine on dead TCP (single consumer, downloads unaffected).
- shared archive.txt read at start → max_concurrent>1 may double-download (default 1, documented).
- restart mid-override loses resume memory (queue keeps playing); single/random MPD modes may end block early (warned via log).
- basic auth over plain HTTP LAN.
