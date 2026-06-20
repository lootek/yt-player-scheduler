# Web UI for yt-player-scheduler

## Context

The existing service (`~/projects/lootek/yt-player-scheduler`) is a Go-based cron scheduler that:
- Searches YouTube daily using `ytsearchdate` with keywords + date
- Downloads audio to `/media/music/youtube/yt-rpi-player-cache/` on remote host `pi@ithilien`
- Plays via MPD or ffplay to PulseAudio
- Configured via YAML (`config.yaml`)

Legacy script `~/scripts/yt.sh` on ithilien downloads full videos (video+audio) to `/media/music/youtube/` using:
- Format: `bestvideo+bestaudio --merge-output-format mkv --add-metadata`
- Output template: `%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s`
- Reads URLs from `list.sh` (channels, playlists, single videos)

**Task**: Add a web UI where users can paste YouTube URLs (video/channel/playlist), choose download options, and save to `/media/music/youtube` (configurable root).

## Requirements

1. **Web UI** (Go + simple HTML, no build step)
   - Input field for YouTube URL (video, channel, or playlist)
   - Checkbox: "Download video" (vs audio-only)
   - Checkbox: "Schedule for MPD playback" (adds to scheduler queue vs download-only)
   - Input field: Download root directory (default: `/media/music/youtube`)
   - Submit button + status display

2. **Backend changes**
   - New HTTP server in `main.go` serving the UI
   - New endpoint `POST /download` that:
     - Accepts URL, video toggle, schedule toggle, download path
     - Calls `ytdlp.Download()` with appropriate format args
     - Optionally schedules for MPD (reuse existing `RunJob` logic)
   - Extend `YtDLPConfig` with `download_root` for UI downloads (separate from scheduler cache)

3. **yt-dlp format handling**
   - Audio-only: `-x --audio-format m4a` (existing)
   - Video: `bestvideo+bestaudio --merge-output-format mp4 --add-metadata` (matches legacy intent, mp4 container)
   - Output template: `%(uploader)s/%(title)s (%(id)s).%(ext)s` (consistent with `yt.sh`)

4. **Configuration**
   - Add `ui` section to `config.yaml`:
     ```yaml
     ui:
       enabled: true
       listen: ":8080"
       download_root: /media/music/youtube
     ```

## Implementation Plan

### Files to modify

1. **`config/config.go`**
   - Add `UIConfig` struct with `Enabled`, `Listen`, `DownloadRoot` fields
   - Add `UI UIConfig` to `GlobalConfig`

2. **`internal/ytdlp/ytdlp.go`**
   - Add `DownloadWithOptions(ctx, url, jobName, downloadDir, video bool)` method
   - Accept format selection (audio-only vs video+audio)
   - Use output template matching legacy: `%(uploader)s/%(title)s (%(id)s).%(ext)s`

3. **`internal/app/app.go`**
   - Add `DownloadURL(ctx, url, downloadDir, video, schedule bool)` method
   - If `schedule=true` and MPD enabled, add to playlist via `PlayWithMPD`

4. **`main.go`**
   - Add HTTP server with routes: `GET /`, `POST /download`
   - Serve static HTML form
   - Handle form submission, call `app.DownloadURL()`

5. **New file: `templates/download.html`** (or embed in `main.go`)
   - Simple HTML form with URL input, checkboxes, submit button

6. **`config.example.yaml`**
   - Document new `ui` section

### Verification

1. Build and run on ithilien (or locally with `-run-now`)
2. Access `http://localhost:8080`
3. Test with:
   - Single video URL, audio-only
   - Playlist URL, video download
   - Channel URL with "schedule for MPD" checked
4. Verify files land in `/media/music/youtube/<uploader>/<title> (<id>).<ext>`

## Notes

- Keep UI downloads separate from scheduler cache (different root dir config)
- Reuse existing cookie handling, PO token, extractor args from `YtDLPConfig`
- No database needed - fire-and-forget downloads
- Consider adding progress feedback (yt-dlp stdout parsing)
