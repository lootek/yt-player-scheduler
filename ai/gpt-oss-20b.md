# Plan for Adding Web UI to yt-dlp Service

## Overview
Add a lightweight web interface that allows users to paste a YouTube URL, choose whether to download audio only or full video, and optionally play the file through MPD. The UI will run concurrently with the existing scheduler.

## Tasks
1. **Extend CLI flags**
   - Add `--web` (bool) and `--web-port` (int, default 8080) to `cmd/yt-daily-player/main.go`.
   - When `--web` is true, start HTTP server concurrently with the scheduler.

2. **Create web package**
   - File: `internal/web/server.go`
   - Endpoints:
     - `GET /` – renders a simple form.
     - `POST /download` – processes form, performs validation, and triggers download/play.
   - Use `html/template` to render the form.
   - Use `net/http` for server, `context` for graceful shutdown.

3. **Add download logic for UI**
   - In `internal/app/app.go`, add `DownloadAndPlay` method that:
     - Calls `a.ytdlp.DownloadWithOptions(...)` to get local path.
     - If MPD enabled and user requested, call `player.PlayWithMPD(...)`.
     - Otherwise, play with default player.
   - Handles cases where `DownloadDir` is unset (falls back to streaming).

4. **Extend ytdlp client**
   - Add method `DownloadWithOptions(ctx, videoURL, jobName string, dumpVideo, downloadAll bool)`:
     - Builds argument list similar to `Download`.
     - Skips `-x` & `--audio-format` if `dumpVideo` is true.
     - Adds `--yes-playlist` when `downloadAll` true.
     - Uses same output template.
     - Returns the local file path.

5. **Detect playlist/channel**
   - In the `/download` handler, check if the URL contains "playlist" or "channel" to set `downloadAll=true`.

6. **Graceful shutdown**
   - In `main.go`, keep reference to the HTTP server.
   - After `c.Stop()` or context cancellation, call `srv.Shutdown(ctx)`.

7. **Testing**
   - Write integration test `internal/web/server_test.go`:
     - Spin up server with a temporary config.
     - Use `httptest.NewServer` to post a URL.
     - Verify that the download command was executed and the expected file exists.
   - Consider unit‑testing the handler by mocking the `App` download method.

8. **Documentation**
   - Update `README.md` to document `--web` usage.

9. **Optional enhancements (not mandatory)**
   - Add basic authentication or CSRF token if the UI is exposed publicly.
   - Add progress or status page to show currently downloading files.

## Trade‑offs & Considerations
- **Concurrency**: Multiple concurrent downloads can saturate bandwidth; consider adding a semaphore or limiting concurrent jobs.
- **Security**: The UI is unprotected; anyone who can access the port can trigger downloads. If this is a concern, add a simple auth layer.
- **Playlist size**: Large playlists may hit the global timeout. The UI handler should use a longer timeout or spawn background goroutine; current plan uses scheduler timeout for simplicity.
- **Channel handling**: Detecting channel URLs by string is fragile; a future improvement could use `yt-dlp --dump-json` to introspect.
- **Logging**: The UI will return plain text on success/failure; logs are kept in the scheduler log file.

## Deliverables
- New CLI flags and conditional web server start in `main.go`.
- `internal/web/server.go` with two endpoints and template.
- New method `DownloadAndPlay` in `app.go`.
- New method `DownloadWithOptions` in `ytdlp.go`.
- Tests in `internal/web/server_test.go`.
- Updated README section for web usage.
- Plan written to `/Users/piotr/ai/yt-player-scheduler/claude-opus-4-7.md`.

---

**Implementation Notes**
- Use `context.WithTimeout` for the download/play operation based on `cfg.Global.Player.Timeout`.
- Sanitize job names by stripping non‑alphanumerics and lower‑casing.
- For the MPD playback path, re‑use existing `player.PlayWithMPD` function.
- For the web server, handle errors by writing HTTP status codes and simple error pages.

# End of plan