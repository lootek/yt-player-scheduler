# Web UI for on-demand YouTube downloads

## Context

`yt-daily-player` (module `github.com/lootek/yt-rpi-player`) is a headless daily audio
scheduler running on a Raspberry Pi. Today it is **cron-driven only**: each configured job
builds a search query from keywords + today's date, takes the first yt-dlp result, downloads
audio, and plays it through MPD. There is no way to say "grab *this* video".

Meanwhile a legacy shell script, `/home/pi/scripts/yt.sh`, does the ad-hoc downloading by
hand — it reads a hand-maintained URL list (`/media/music/youtube/list.sh`) and bulk-fetches
video into an uploader-keyed tree. Editing a shell script over SSH to add one video is the
friction this change removes.

**Goal:** a small web UI where pasting a YouTube video / channel / playlist URL downloads it
into a configured directory, with two independent checkboxes — *queue it in MPD* and
*download video (not just audio)*.

### Baseline and a note on prior art

This plan targets **commit `bfe645a`** ("less logging noise, try bringing back end
condition", Mar 11 2026), which has no web layer at all: `internal/{app,config,player,query,ytdlp}`,
zero `net/http`, zero tests.

Worth recording: while investigating I found that **`origin/main` (`2143c24`) already contains
an implementation of this exact feature** in `internal/webui/`, and it is live on the pi at
`http://192.168.10.22:8080`. Piotr's instruction was to disregard that and design it from
scratch against `bfe645a`. So this is a clean-slate design, written as if the feature does not
exist. Where my design lands on the same answer as upstream, that is convergence, not copying —
but it does mean the upstream tree is a useful cross-check during review.

Also noted during investigation, and **not** addressed by this plan:
- `yt.sh` and `yt-lupin.sh` both `cd /media/pi/music/youtube/`, a path that no longer exists
  (real path is `/media/music/youtube`). With no `set -e` the failed `cd` is ignored. Both
  scripts are effectively dead, and `youtube-dl` is not installed on the pi.
- `internal/ytdlp.Search` uses `ytsearchdate<N>:`, which yt-dlp removed in 2026.02. Pre-existing
  scheduler bug, out of scope here.
- `internal/app/app_mpv.go` (`RunJobWithMPV`) is dead code that duplicates `RunJob`.

### Environment facts that constrain the design

Verified by SSH, not assumed:
- **"ithilien" IS the pi.** `192.168.10.22` reports `hostname` = `ithilien`. `/media/music` is
  its own local ext4 RAID array (`/dev/md127p1`, 2.1T, 544G free) — *not* a remote mount. There
  is no second box and no NFS/CIFS to design around.
- MPD `/etc/mpd.conf`: `music_directory "/media/music"`, `auto_update "yes"`, no password.
  So a file at `/media/music/youtube/A/B.m4a` is addressed to MPD as `youtube/A/B.m4a`.
- The app runs in Docker, `network_mode: host`, with `/media/music/youtube` bind-mounted
  **path-identically** inside and out. That identity is what makes path→MPD-URI mapping work;
  preserve it.
- yt-dlp in the container is pinned to nightly `2026.06.21.235142` with an explicit comment
  that newer builds return 403s. **Do not bump it.** (This Mac has newer, `2026.07.04` — so
  local testing is not authoritative for download success.)
- `/media/music/youtube/archive.txt` has **1042 entries** in `youtube <id>` format, written by
  `yt.sh`. It is the shared dedup ledger.

## Decisions

| Question | Decision |
|---|---|
| Download target | `/media/music/youtube`, **sharing the existing `archive.txt`** |
| MPD on checkbox | **Enqueue all files, never auto-play** |
| Execution model | **Background worker pool + job IDs**, `max_concurrent` default 2 |

Sharing `archive.txt` is the load-bearing choice: it means anything `yt.sh` already fetched
will not re-download, and new web-UI downloads are invisible to a future `yt.sh` run for the
same reason. Dedup history stays single-sourced.

Enqueue-without-play is deliberate: a pasted channel URL can be 50 files, and hijacking
whatever is currently playing on the living-room speaker would be a bad surprise. Playback
stays a manual act.

## yt-dlp invocation — consistency with `yt.sh`

`yt.sh` verbatim (the whole file):

```bash
cd /media/pi/music/youtube/
/usr/local/bin/youtube-dl -i --download-archive archive.txt -f bestvideo+bestaudio \
  --merge-output-format mkv --add-metadata -a <(bash ./list.sh) \
  -o '%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s'
```

The new downloader carries over every flag that matters, and the naming template **exactly**:

```
-i                                  # skip dead/private/geo-blocked entries; essential for bulk
--add-metadata                      # tag title/uploader/date into the container
--download-archive <archive.txt>    # shared 1042-entry dedup ledger
--output <dir>/%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s
```

Format selection splits on the **video checkbox**:
- checked → `-f bestvideo+bestaudio --merge-output-format mkv` (identical to `yt.sh`)
- unchecked → `-x --audio-format m4a` (**new**; `yt.sh` has no audio-only mode, and music-only
  is the whole point of the third checkbox)

Added for the UI, absent from `yt.sh`:
```
--newline --progress                       # line-oriented progress, parseable
--print before_download:PENDING:%(filename)s
--print after_move:DONE:%(filepath)s       # one line PER ITEM — see gotcha below
```

Deliberately **not** added (matching `yt.sh`): no thumbnails, no subtitles, no SponsorBlock,
no `--restrict-filenames`.

`baseArgs()` is reused unchanged, so `extra_args`, `--user-agent`, PO-token flags,
`--remote-components ejs:npm` and `--js-runtimes node` all still apply.

**Gotcha to respect:** the existing `ytdlp.Download` recovers its result by taking the *last
line* of stdout. That is only valid for a single video. A playlist emits one `DONE:` line per
item, so the new code must **collect all** matching lines, not take the last. This is why
`DownloadMedia` is a new method rather than a tweak to `Download` — the scheduler's
single-video contract stays untouched.

**Second gotcha:** for a standalone video `%(playlist_title)s` has no value and yt-dlp
substitutes the literal `NA`, giving `Uploader/NA/Title (id).mkv`. That is what `yt.sh` has
always produced, so keep it — consistency beats tidiness here.

## Architecture

One new package, `internal/webui`, plus one new method on the existing yt-dlp client and one
new function in the existing player package. The scheduler path is not modified.

```
HTTP POST /download
      │  (returns immediately with a job ID)
      ▼
  Service.Enqueue ──▶ buffered chan *Job ──▶ N workers (max_concurrent)
                                                  │
                                                  ├─▶ ytdlp.DownloadMedia   (long: minutes→hours)
                                                  │      streams progress into job.Log
                                                  │      reports each finished file
                                                  │
                                                  └─▶ player.EnqueueMPD     (if checkbox set)
                                                         AddID per file, no PlayID
  GET /api/jobs ◀── in-memory job state + history.jsonl
```

### Files

**New — `internal/webui/job.go`**
`Job` struct: `ID` (8 random bytes hex), `URL`, `Video`, `MPD` bools, `Status`
(`queued|running|done|failed`), `PendingFiles`, `Files`, `Error`, `Log *bytes.Buffer`,
timestamps. `Log` is JSON-excluded and served separately.

**New — `internal/webui/service.go`**
Owns the queue and job state. `mu sync.RWMutex` guards `jobs map[string]*Job` + `order []string`.
- `Enqueue(url string, video, mpd bool) (string, error)` — creates the job, appends a history
  event, non-blocking send onto the channel; returns an error if the queue is full rather than
  blocking the HTTP handler.
- `Start(ctx)` — spawns `max_concurrent` workers, blocks on `ctx.Done()`, then drains.
- `runJob` — `context.WithTimeout(parent, cfg.Timeout)`, `defer recover()` so one bad job
  cannot take the process down, then download → optional MPD enqueue.
- **Per-URL mutex** (`urlLocks map[string]*sync.Mutex`): two concurrent jobs for the same URL
  would race on identical output paths *and* on `archive.txt`, which is append-only with no
  internal locking. Serialize by URL.
- `Get`/`List` return **deep copies** (`clone`) — handing a live `*Job` to a handler while a
  worker mutates it is a data race.

**New — `internal/webui/server.go`**
`net/http.ServeMux` only, no router dependency. Go 1.22 method patterns:
`GET /{$}`, `POST /download`, `GET /status`, `GET /api/jobs`, `GET /api/jobs/{id}`,
`GET /log/{id}`, `GET /static/`. Two middlewares: `recoverer` (panic → 500) and `withAuth`
(optional Basic auth via `crypto/subtle.ConstantTimeCompare`; logs a warning when
username/password are empty, since the listener is on a host-network `0.0.0.0`).
`POST /download` parses the form, reads `video` / `mpd` checkboxes, enqueues, and 303-redirects
to `/status?id=…`.

**New — `internal/webui/history.go`**
Append-only JSON-lines at `<download_dir>/history.jsonl`, mutex-guarded, `O_APPEND`.
`Append(event)` on each state change. `List(limit)` replays the file, folds events into
per-ID snapshots, sorts by creation time desc.
**Wire `List` into `GET /api/jobs`**, merged with in-memory state — otherwise the job table is
empty after every restart and the file is write-only. (I verified this exact failure mode
against the live upstream deployment: 117 events on disk, `/api/jobs` returning `[]`.)

**New — `internal/webui/templates.go` + `templates/*.html` + `static/app.js`**
`//go:embed` both trees so the binary stays self-contained (the Docker image has no asset
copy step). `index.html` = URL field + the two checkboxes + a jobs table. `status.html` =
single-job view. `app.js` polls `/api/jobs` every 2s and renders rows via DOM API —
`textContent`, never `innerHTML`, since titles are attacker-influenced text.

**Modified — `internal/config/config.go`**
Add `WebUIConfig` under `GlobalConfig` as `web_ui`:

```go
type WebUIConfig struct {
    Enabled       bool   `yaml:"enabled"`
    Listen        string `yaml:"listen"`         // default ":8080"
    Username      string `yaml:"username"`
    Password      string `yaml:"password"`
    DownloadDir   string `yaml:"download_dir"`   // /media/music/youtube
    ArchivePath   string `yaml:"archive_path"`   // default <DownloadDir>/archive.txt
    MaxConcurrent int    `yaml:"max_concurrent"` // default 2
    Timeout       string `yaml:"timeout"`        // default "2h"
    HistoryPath   string `yaml:"history_path"`   // default <DownloadDir>/history.jsonl
}
```

Extend `applyDefaults` for those defaults. Relax the `Load` guard: today it hard-fails on
`len(cfg.Jobs) == 0`, but a download-only deployment with no cron jobs is now legitimate —
fail only when there are no jobs **and** the web UI is disabled.

**Modified — `internal/ytdlp/ytdlp.go`**
Add `DownloadMediaRequest` / `DownloadMediaResult` and `DownloadMedia(ctx, req)`. Reuses
`baseArgs()`, `binary()` and `prepareCookies()` — the last one matters: it copies the cookie
jar to a temp file per invocation, and yt-dlp *mutates* cookie jars, so concurrent workers
sharing one jar would corrupt it. Progress parsing needs a small `lineParser` writer
(buffer bytes, emit on `\n`, invoke a callback per line) and a mutex-wrapped `syncWriter`,
because stdout and stderr both feed one log buffer from two goroutines.

**Modified — `internal/player/mpd_player.go`**
Add `EnqueueMPD(ctx, cfg, downloadDir string, uris []string) error`. Extract the existing
path→MPD-relative logic out of `PlayWithMPD` into a shared `mapToMusicRoot(cfg, downloadDir, uri)`
helper (it currently only handles one URI inline) and reuse it for both. `EnqueueMPD` collects
the distinct parent dirs, issues one `client.Update(dir)` per dir, waits for the rescan, then
`AddID(rel, -1)` per file. **No `PlayID`.** No status-polling loop — it returns as soon as the
queue is populated, unlike `PlayWithMPD` which blocks for the whole track.

*Note:* `mpd.conf` has `auto_update "yes"`, so the explicit `Update` is belt-and-braces — but
inotify on a 1.3 TB tree lags, and `AddID` fails outright on a path MPD has not indexed yet, so
keep it.

**Modified — `main.go`**
Add a `-web-ui` flag OR'd with `cfg.Global.WebUI.Enabled`. When on: open history, build the
service, `go svc.Start(ctx)`, `go srv.ListenAndServe(...)`. On shutdown, `srv.Shutdown` with a
10s timeout before `c.Stop()`. The cron scheduler is untouched and both share the one
signal-derived context.

**Modified — `config.example.yaml`, `README.md`, `docker-compose.yaml`**
Document the block. Compose already bind-mounts `/media/music/youtube` path-identically and
uses host networking, so port 8080 needs no new mapping — verify, don't re-add.

## Tests

The repo has **zero** `_test.go` files, so this establishes the pattern. Table-driven, stdlib
`testing` only, no new deps. Focus on pure logic — not on invoking real yt-dlp:

- `internal/ytdlp` — argument construction: audio vs video branch produces the exact expected
  flag sequence; the output template string is byte-identical to `yt.sh`'s; `archive_path`
  override respected. Plus the `DONE:`-line collector: assert a simulated multi-item playlist
  stream yields **all** paths (the regression the old last-line logic would fail).
- `internal/player` — `mapToMusicRoot`: `/media/music/youtube/A/B.m4a` → `youtube/A/B.m4a`;
  passthrough when `MusicRoot` is empty or the path is outside the download dir.
- `internal/config` — defaults applied; jobs-empty + web-UI-enabled loads OK; jobs-empty +
  web-UI-disabled still errors.
- `internal/webui` — `History` round-trip (append events, `List` folds to correct final
  status); handler tests via `httptest` for the checkbox→bool mapping, the 303 redirect, 404
  on unknown job ID, and 401 when auth is configured.

## Verification

1. `go build ./... && go vet ./... && go test ./...` on this Mac.
2. Run locally with a scratch config pointing `download_dir` at a temp dir, `mpd.enabled: false`:
   `./yt-rpi-player -config /tmp/t.yaml -web-ui`. Confirm the page loads, an unauthenticated
   request is refused once credentials are set, and a single short video lands at
   `Uploader/NA/Title (id).mkv`. Confirm `archive.txt` gains exactly one line, and that
   re-submitting the same URL is skipped by the archive.
3. Playlist behaviour: submit a 3-item playlist, audio-only. Confirm three `.m4a` files, three
   `DONE:` paths in `Files`, and a live-updating pending state. **Local yt-dlp is newer than
   the pinned one, so treat download *success* here as indicative only** — the arg-construction
   assertions in the tests are the real contract.
4. On the pi, in a **worktree, not the deployed directory** (avoid disturbing the running
   service): rebuild the image, run the container with `web_ui.enabled: true` on an
   **alternate port**, and submit one video with *Queue in MPD* ticked. Verify with
   `mpc playlist` that it was appended and `mpc status` shows playback **unchanged** — that is
   the enqueue-don't-play requirement.
5. Confirm `/api/jobs` is still populated after a container restart (the history-replay path).
6. Check ownership of new files: the container runs as root over a `pi`-owned tree, so
   downloads land root-owned. Decide explicitly whether that is acceptable before wider use.

## Out of scope

Fixing `yt.sh`'s dead `cd` path; the `ytsearchdate` scheduler bug; deleting `app_mpv.go`;
migrating `list.sh` into the UI; per-request target directories (the brief specifies the
directory comes from configuration); auth beyond optional Basic; upgrading yt-dlp.
