# yt-player-scheduler: on-demand download Web UI (based on bfe645af)

## Context

`yt-player-scheduler` at `bfe645af` is a Go cron scheduler: YAML-configured jobs search YouTube daily feeds via yt-dlp (`ytsearchdate`), optionally download audio to `download_dir`, and play via ffplay/mpv or MPD (`PlayWithMPD`: DB update → `AddID` → `PlayID` → poll until finished). It runs in Docker on `pi@ithilien` (`yt-rpi-player:local`, config at `~/yt-daily-player/config.yaml`, MPD `music_directory` = `/media/music`).

Goal: add a web UI where a user pastes a YouTube video/channel/playlist URL and it gets downloaded to a configured directory (`/media/music/youtube` on ithilien), with two checkboxes:
1. **Queue in MPD** — append to MPD queue, play now, and resume the previously playing item (at saved position) once the appended item(s) finish.
2. **Download video** — unchecked = audio only (native bestaudio, no re-encode; MPD on ithilien decodes webm/opus/m4a — verified).

Download options/naming stay consistent with legacy `pi@ithilien:~/scripts/yt.sh`:
`-i --download-archive archive.txt -f bestvideo+bestaudio --merge-output-format mkv --add-metadata -o '%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'`
…with one agreed change: **drop the playlist dir when it would be "NA"** (single-video URLs → `%(uploader)s/%(title)s (%(id)s).%(ext)s`), and align `yt.sh` itself to the same behavior.

Git note: `bfe645af` shares no ancestor with current `main` (public-release rewrite) and is on no branch. Work happens on a new branch `web-ui` created at `bfe645af`. Decision: **branch + push + deploy to ithilien** (replaces the currently running newer version there).

## Workspace setup (step 0)

- Copy this approved plan to `~/ai/yt-ui-ext/claude-fable-5.md`.
- Clone `git@github.com:lootek/yt-player-scheduler.git` to `~/ai/yt-ui-ext/yt-player-scheduler`; fetch `bfe645af` (present in the `~/projects` copy — add it as a local remote if not on origin: `git remote add local ~/projects/lootek/yt-player-scheduler && git fetch local bfe645af`); `git checkout -b web-ui bfe645af`.
- Add `yt-ui-ext/yt-player-scheduler/` to `~/ai/.gitignore` so the clone isn't committed with session dumps.

## Design

### Config (`internal/config/config.go`)

New `global.web_ui` section — **same shape as the one already present in the deployed `config.yaml`** so it drops in without config edits:

```yaml
web_ui:
  enabled: true
  listen: ":8080"
  username: ""            # optional HTTP basic auth (enabled when password set)
  password: ""
  download_dir: "/media/music/youtube"
  subdir: ""              # optional subdir under download_dir
  max_concurrent: 2
  timeout: "2h"           # per download job
  history_path: ""        # optional JSON job-history persistence
```

Defaults in `applyDefaults`: listen `:8080`, max_concurrent 2, timeout `2h`. Relax `Load()`: `no jobs configured` is only an error when `web_ui.enabled` is false.

### yt-dlp extensions (`internal/ytdlp/ytdlp.go`)

Reuse existing plumbing: `baseArgs()` (extra_args/UA/PO tokens/js-runtime), `prepareCookies()`, the `--print after_move:filepath` output-parsing idiom from `Download()`.

- `Probe(ctx, url) (ProbeResult, error)` — `yt-dlp -J --flat-playlist --no-warnings <url>` → `{Type: "video"|"playlist", Title, Uploader, EntryCount}`. Used to pick the output template and to label the job in the UI.
- `DownloadMedia(ctx, opts, onFile func(path string)) ([]string, error)` with `opts{URL, Dir, Video bool, IsPlaylist bool}`:
  - common args: baseArgs + cookies + `--ignore-errors`, `--download-archive <Dir>/archive.txt`, `--embed-metadata`, `--no-warnings`, `--print after_move:filepath`
  - video mode: `-f bestvideo+bestaudio --merge-output-format mkv`
  - audio mode: `-f bestaudio` (native container; no `-x`)
  - `-o`: playlist/channel → `<Dir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`; single video → `<Dir>/%(uploader)s/%(title)s (%(id)s).%(ext)s`
  - stream stdout line-by-line; each completed file → `onFile` callback (drives job progress + incremental MPD enqueue); return all final paths.
  - URLs already in archive produce no output → job ends as "0 new files (already archived)". Best effort for MPD-checkbox case: resolve the would-be path via `--print filepath --skip-download` (without the archive flag) and enqueue it if it exists on disk.

### MPD enqueue + resume (`internal/player/mpd_player.go`)

`EnqueueMPD(ctx, cfg, downloadDir string, paths []string, autoplay bool) error`:
- Translate absolute paths to MPD-relative using the existing MusicRoot prefix logic (mpd_player.go:35-47 at bfe645af).
- `client.Update(rel)` then poll `Status()["updating_db"]` until clear (bounded wait) — replaces the fixed 30s sleep.
- `AddID` each file; when `autoplay` (the checkbox): remember currently playing `(songid, elapsed)`, `PlayID` the first added item, and resume the remembered item at its saved position after appended items finish.
- **Port commit `4035aad` from the local `main` line** (`~/projects/lootek/yt-player-scheduler`, file `internal/player/mpd_player.go`) — it implements exactly these semantics: LIFO resume stack + single shared background watcher, shared between web-UI enqueues and cron `PlayWithMPD`, handling chained appends and the paused/stopped/empty-playlist cases. Adapt, don't redesign.

### Web UI (`internal/webui/`, new package)

Stdlib only (`net/http`, `embed`); no new Go dependencies.

- `server.go` — HTTP server with graceful shutdown; optional basic-auth middleware.
  - `GET /` → embedded `index.html`: URL input; checkboxes **“Download video (unchecked = audio only)”** (default off) and **“Queue in MPD after download”** (default off); jobs table (status, title, files done/total, errors) refreshed by 2s polling.
  - `POST /api/downloads` `{url, video, mpd}` → validate URL (youtube.com / youtu.be / music.youtube.com), create job, return `{id}`.
  - `GET /api/downloads` → job list JSON.
- `manager.go` — in-memory job store + worker pool (`max_concurrent`); per-job flow: probe → download (with per-file callback) → optional incremental `EnqueueMPD`; per-job `context.WithTimeout(web_ui.timeout)`. Statuses: `queued, probing, downloading, enqueuing, done, error`. Optional `history_path` JSON persistence (load at start, save on state change).

### Wiring (`main.go`)

When `cfg.Global.WebUI.Enabled`: start webui server in a goroutine alongside the cron scheduler; shut down on signal ctx. Scheduler behavior unchanged. Update `config.example.yaml` + README section.

### Legacy `yt.sh` alignment (on ithilien)

Rewrite `~/scripts/yt.sh` (keep `yt.sh.bak`): cd to `/media/music/youtube` (fixes stale `/media/pi/music/youtube`), loop over `list.sh` URLs, pick template per URL pattern (`playlist?list=` / `/@` / `/channel/` / `/videos` / `/streams` → playlist template, else single-video template), same flags as before. Switch binary to `yt-dlp` (install to `/usr/local/bin/yt-dlp` from the nightly release, same source as the Dockerfile) since host `youtube-dl` no longer works against YouTube; all legacy flags are supported by yt-dlp.

## Execution: subagents & commits (branch `web-ui`, repo clone in session dir)

Foreground subagents, one commit each, `cd` into the clone first:

1. **Agent A — config + ytdlp**: `web_ui` config section, relaxed validation, `Probe`, `DownloadMedia` + table tests for arg/template construction. *Commit: "config+ytdlp: web_ui section, probe and yt.sh-compatible downloads"*
2. **Agent B — MPD enqueue/resume**: `EnqueueMPD`, update-poll instead of sleep, port of `4035aad` resume watcher + unit tests for path translation. *Commit: "player/mpd: enqueue with autoplay and resume of prior item"* (parallel with A)
3. **Agent C — webui + wiring**: `internal/webui` (server, manager, embedded page), `main.go`, `config.example.yaml`, README. *Commit: "webui: on-demand download UI"* (after A+B)
4. **Agent D — verify + deploy**: local e2e (below), push `web-ui` to origin, deploy to ithilien, update `yt.sh`. *Commits: any fixes found during e2e; no repo commit for yt.sh (lives on the Pi, backed up in place).*

## Verification

Local (macOS, in the clone):
- `go build ./... && go vet ./...` and `go test ./...`
- Run with a test config: `download_dir` → session tmp dir, `mpd.enabled: false`, `web_ui.enabled: true`, no jobs. `curl -X POST localhost:8080/api/downloads -d '{"url":"<short video>","video":false,"mpd":false}'`; assert file lands as `Uploader/Title (id).webm|m4a` and job reaches `done`. Repeat with a small playlist URL → `Uploader/Playlist/…`; repeat same URL → "already archived". Load `GET /` in a browser.

Deploy to ithilien (`~/yt-daily-player` is a git repo — replaces the running newer version, as agreed):
- Push `web-ui`; on the Pi: fetch + `git checkout web-ui`, keep existing `config.yaml` (its `web_ui:` section already matches this schema), verify `docker-compose.yaml` mounts `/media/music` and uses host networking, `docker compose up --build -d` (or the existing build/run flow for `yt-rpi-player:local`).
- E2E from LAN: open `http://ithilien:8080`, paste a short video, audio-only + MPD checkbox → file appears under `/media/music/youtube/...`, MPD queue gets it, playback switches to it and resumes the prior item afterward. Check `docker logs` and that cron jobs still fire.
- `yt.sh`: run once, confirm new naming and that `archive.txt` continues to be honored.
