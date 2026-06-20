# YouTube Player Scheduler - Web UI Extension Plan

## Context

The yt-player-scheduler is a Go-based service that runs on a Raspberry Pi to automatically download and play YouTube audio content based on scheduled jobs defined in a YAML configuration. The service currently operates in a headless mode with cron-like scheduling, but lacks a web interface for manual on-demand downloads.

## Current Implementation Overview

### Core Components
1. **Scheduler**: Uses robfig/cron to run jobs based on cron expressions
2. **YouTube Integration**: Uses yt-dlp for searching, downloading, and streaming
3. **Playback**: Supports both direct playback (ffplay) and MPD integration
4. **Deployment**: Docker-based deployment on Raspberry Pi with PulseAudio integration

### Key Files
- `main.go`: Entry point with cron scheduling
- `internal/ytdlp/ytdlp.go`: YouTube-dlp integration for search/download/streaming
- `internal/app/app.go`: Core application logic
- `config.yaml`: Job definitions and configuration

### Current Download Functionality
- Downloads audio-only content using yt-dlp
- Supports YouTube Premium cookies for ad-free experience
- Can download to local cache directory or stream directly
- File naming pattern: `%(uploader)s - %(title)s [%(id)s].%(ext)s`

### Remote Deployment (ithilien)
- Directory: `/media/music/youtube/` for downloaded content
- Cache directory: `/media/music/youtube/yt-rpi-player-cache/brewiarz`
- Docker-based deployment with host networking
- PulseAudio integration for audio playback

## Requirements for Web UI Extension

1. **Web Interface**: Allow users to paste YouTube URLs (video, channel, playlist) for manual downloads
2. **Configuration**: Use configurable download directory (default to `/media/music/youtube/`)
3. **Consistency**: Follow legacy `~/scripts/yt.sh` patterns for download options and naming
4. **Options**:
   - Checkbox for MPD scheduling (whether to add to MPD playlist)
   - Checkbox for video vs audio-only download
5. **Integration**: Extend existing codebase rather than creating separate service

## Implementation Plan

### Phase 1: Web Server Integration
1. Add HTTP server to existing Go application
2. Create API endpoints for:
   - Submitting YouTube URLs
   - Checking download status
   - Viewing configuration
3. Add web UI templates for the frontend

### Phase 2: Download Functionality Extension
1. Extend ytdlp package to handle on-demand downloads
2. Add support for video/audio selection
3. Implement consistent naming patterns with legacy script
4. Add MPD integration option for scheduling

### Phase 3: Web UI Development
1. Create responsive web interface
2. Implement URL submission form with options
3. Add status display for downloads
4. Ensure consistent styling with existing project

### Phase 4: Configuration and Deployment
1. Add web server configuration to config.yaml
2. Update Docker configuration to expose web port
3. Ensure proper permissions for download directories
4. Test deployment on ithilien

## Technical Details

### URL Types to Support
- Single videos: `https://youtube.com/watch?v=...`
- Playlists: `https://youtube.com/playlist?list=...`
- Channels: `https://youtube.com/@channel` or `https://youtube.com/channel/...`

### Download Options
- Audio-only (default, current behavior)
- Full video download option
- MPD scheduling option
- Custom download directory (from config)

### File Naming Consistency
Current legacy script pattern:
```
%(uploader)s/%(playlist_title)s/%(title)s (%(id)s).%(ext)s
```

Our implementation should follow:
```
%(uploader)s - %(title)s [%(id)s].%(ext)s
```

### API Endpoints
- `GET /` - Web UI
- `POST /download` - Submit URL for download
- `GET /status` - Check download status
- `GET /config` - View current configuration

### Security Considerations
- Input validation for YouTube URLs
- Proper authentication/authorization for web interface
- Secure handling of cookies and credentials
- Rate limiting to prevent abuse

## Integration Points

1. **Existing ytdlp package**: Extend with on-demand download functionality
2. **Configuration system**: Add web server settings to config.yaml
3. **Docker deployment**: Expose web port in docker-compose.yaml
4. **MPD integration**: Reuse existing MPD player code for scheduling option

## Testing Plan

1. Unit tests for new web server components
2. Integration tests for download functionality
3. Manual testing of web UI with various URL types
4. Deployment testing on ithilien
5. Verification of file naming consistency with legacy script