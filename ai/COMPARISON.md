# Plan Comparison: yt-lootek Web UI Extension

All four plans address the same prompt: extend `yt-daily-player` with a web UI for ad-hoc YouTube downloads + optional MPD scheduling.

## Differences

| Dimension | gemma4-26b | gemma4-agent-26b | sonnet-4-6 | opus-4-7 |
|---|---|---|---|---|
| **Length** | 77 lines | 62 lines | 221 lines | 228 lines |
| **Project name accuracy** | `yt-daily-player` ✓ | `yt-player-scheduler` ✗ (invented) | `yt-rpi-player` (binary name) | `yt-daily-player` ✓ |
| **Deployment context** | None | None | Pi `192.168.10.22`, `network_mode: host`, `/media/music/youtube` | Same + ithilien hostname, references existing `archive.txt` |
| **Legacy `yt.sh` recipe quoted** | No | Mentions naming pattern | Mentions flags inline | Full script quoted + flag-by-flag rationale |
| **Sync vs async download** | Sync (in-request) | Sync (in-request) | Async with job store + status page | Async with FIFO queue, worker goroutine, history cap N=200 |
| **Job state model** | None | None | `JobStore` w/ `sync.RWMutex`, statuses Pending/Running/Done/Failed | `Manager` + `Job` struct w/ files, logs, timestamps |
| **New packages** | None (stuffs into `app`) | `app/api.go` (new file) | `internal/webui` (3 files) | `internal/web` + `internal/queue` (separation of concerns) |
| **Config additions** | Vague ("if needed") | `DownloadDir`, `YtdlpArgs` on Global | `WebConfig{Enabled, Listen, DownloadDir}` w/ defaults + jobs-required guard relaxed | `WebConfig{Enabled, Address, DownloadDir}` w/ explicit independence from cron's `YtDLP.DownloadDir` |
| **ytdlp client changes** | Reuses existing `Download()` | Modifies `Download()` to take `videoOnly` | Adds `DownloadWeb()` (additive, untouched original) | Adds `DownloadAdHoc()` w/ `--print after_move:filepath`, `--ignore-config`, streamed output |
| **MPD integration** | Ambiguous: "add to cron" | Reuses cron job logic | New `AddToMPDQueue()` — dial, update, AddID, no PlayID | New `MPDQueueAppend()` — explicitly per-file (handles playlists), no playback monitor |
| **Playback non-interruption** | Not addressed | Not addressed | Implied (no PlayID) | Explicitly stated as user requirement |
| **Frontend** | Inline HTML + JS fetch | `web/index.html` + `script.js` | `html/template` embedded as Go consts, no JS, meta-refresh | `embed.FS` w/ `templates/*.tmpl`, meta-refresh |
| **Streaming yt-dlp output to UI** | No | No | Yes — `bufio.Scanner` goroutines, line-flush writer | Yes — `io.MultiWriter` w/ progress parser collecting `after_move:filepath` |
| **File map specificity** | 4 files listed | 5 files listed | 6 files w/ exact line refs | 13 files (new + edited) w/ exact line refs |
| **Docker-compose change** | Not mentioned | Not mentioned | Add `/media/music/youtube` mount | Widen existing brewiarz-only subpath mount; notes host-net = no `ports:` needed |
| **Edge cases / pitfalls** | None | None | None | 6 explicit gotchas: MPD path prefix, container mount, port collision, panic→daemon crash, long downloads/no timeout, multi-file playlists |
| **Tests** | Manual + integration mention | curl + browser | Build + manual e2e | Manual e2e + unit tests (`queue`, `config`) + handler test |
| **Verification steps** | 2 high-level | 4 high-level | 8 concrete steps | 7 numbered scenarios incl. archive-skip + failure paths |
| **Format header** | Frontmatter (skill-style) | Plain heading | Plain heading | Plain heading w/ "decisions locked" table |
| **Self-doubt / hedging** | One "Self-correction" note | None | One "(Check actual field name…)" | None — confident, references verified facts |

## Similarities

| Common element | All four agree on |
|---|---|
| **Stack** | Go + `net/http` (no framework), HTML templates, `yt-dlp` reuse |
| **Endpoint shape** | `GET /` (form) + `POST /download` (or `/jobs`) |
| **Two-checkbox UX** | Audio-vs-video + schedule-for-MPD (gemma plans omit one or label loosely) |
| **Reuse** | Existing `internal/ytdlp` and `internal/player` packages |
| **Goroutine** | Web server starts in goroutine alongside cron |
| **Output dir** | Configurable `download_dir` (sonnet/opus default to `/media/music/youtube`) |

## Net assessment

- **gemma4-26b / gemma4-agent-26b**: surface-level scaffolding, sync downloads, no deployment awareness, no playlist/long-running handling. Would compile, but ship broken UX (request hangs for hours on a channel).
- **sonnet-4-6**: production-ready async design, clean additive APIs (no signature breaks), tight scope. Reads like a junior-senior engineer plan.
- **opus-4-7**: most context-aware (references actual file/line numbers, legacy script, MPD path-prefix logic, container mount gotcha), explicitly handles edge cases gemma plans entirely miss. Heaviest, but the extra weight is load-bearing — it's the only plan that flags the docker-compose mount issue that would silently break MPD updates.
