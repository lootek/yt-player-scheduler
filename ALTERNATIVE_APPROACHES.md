# Alternative Approaches

## Solutions

### 1. mpv with Built-in yt-dlp (Recommended)
mpv handles extraction internally - more reliable than two-step process.

```go
// main.go: Use RunJobWithMPV instead of RunJob
err := application.RunJobWithMPV(jobCtx, job)
```

### 2. Download + Play
Most reliable but slower - downloads first, then plays.
```bash
yt-dlp -f bestaudio -o /tmp/audio.m4a "$URL" && ffplay /tmp/audio.m4a
```

### 3. Streamlink
```bash
streamlink --player ffplay "youtube.com/watch?v=ID" audio_mp4
```

### 4. Non-YouTube Alternatives
- Spotify API + spotifyd
- Local library with mpd
- Podcast RSS feeds

## Debugging
```bash
./diagnose.sh                    # Run diagnostics
yt-dlp --verbose -f bestaudio -g "URL"
docker compose run --rm app sh  # Shell into container
```
