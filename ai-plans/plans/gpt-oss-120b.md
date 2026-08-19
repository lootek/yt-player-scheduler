---
name: kind-juggling-wombat
description: Add web UI for downloading YouTube content with optional MPD playback and video/audio selection
metadata:
  type: project
---

## Context
User wants a web UI at `0.0.0.0:8080` to paste a YouTube video/playlist/channel URL. The service should download to the configured directory (`YtDLP.DownloadDir`, pointing to `/media/music/youtube`). Provide checkboxes: “Play via MPD” (audio only) and “Download video” (otherwise audio only). UI must respect naming pattern of legacy `~/scripts/yt.sh`.

## Approach
- Extend config with UI bind address (default `0.0.0.0:8080`).
- Add `DownloadVideo` method to `internal/ytdlp/client.go` for full‑video download (no `-x`, `--audio-format`).
- Implement `ui.go` with net/http server, HTML form, handlers that:
  * Parse form fields,
  * Call the appropriate ytdlp download method,
  * If “Play via MPD” selected and playback mode is audio‑only, invoke `player.PlayWithMPD`.
- Start the UI server from `main.go` in a goroutine, shutting it down on context cancellation.
- Keep naming template consistent with `~/scripts/yt.sh`: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`.
- Update Dockerfile / compose to expose port 8080 (optional).
- Add minimal tests for the handlers.

## Steps
1. **Config**
   - Add `type UIConfig struct { BindAddress string `yaml:"bind_address"` }` to `GlobalConfig`.
   - Set default `bind_address: "0.0.0.0:8080"` in `applyDefaults`.
   - Update `config.example.yaml` with UI block.

2. **ytdlp client**
   - Add method `DownloadVideo(ctx context.Context, videoURL, jobName string) (string, error)` using `outputTemplate` and args without `-x`/`--audio-format`.
   - Use same `outputTemplate` as `Download` but with video naming.

3. **Web UI**
   - Create `ui.go`:
     * Define HTML template (form with fields `url`, `play` checkbox, `video` checkbox).
     * GET `/` renders form.
     * POST `/download` invokes download logic, returns simple success page.
   - Register routes with `http.NewServeMux()`.

4. **main.go**
   - After `c.Start()` launch UI server:
     ```go
     srv := &http.Server{Addr: cfg.Global.UI.BindAddress, Handler: uiMux}
     go func(){ logger.Printf("UI listening on %s", cfg.Global.UI.BindAddress); if err:=srv.ListenAndServe(); err!=nil && err!=http.ErrServerClosed{ logger.Fatalf("UI server error: %v", err) } }()
     go func(){ <-ctx.Done(); srv.Shutdown(context.Background()) }()
     ```
   - Ensure imports for net/http, ui package.

5. **Docker / compose**
   - Add `- "8080:8080"` to service ports.

6. **Tests (optional)**
   - Write unit tests for GET and POST handlers using `httptest`.

7. **Documentation**
   - Add brief usage note in README (optional).

## Verification
- Run the binary, visit `http://<host>:8080`, submit a YouTube URL, check that audio is saved in `/media/music/youtube` with expected naming.
- Verify “Play via MPD” checkbox queues audio in MPD.
- Verify “Download video” checkbox creates a video file (e.g., .mkv) and does not trigger MPD playback.
- Ensure existing scheduled jobs continue unchanged.
