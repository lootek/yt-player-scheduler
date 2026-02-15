# Alternative Approaches for YouTube Audio Playback

## Current Issue
The app works until extracting YouTube streams. This is due to YouTube's aggressive anti-bot measures including:
- Signature verification challenges
- IP/user-agent blocking
- PO token requirements
- Frequent API changes

## Solution 1: Enable Mobile Client Fallbacks ✅ IMPLEMENTED
**Status**: Already implemented in this PR

Enable android/ios client fallbacks that often bypass signature challenges:
```go
// internal/ytdlp/ytdlp.go:167-171 (now uncommented)
attempts = append(attempts,
    extractorAttempt{args: []string{"--extractor-args", "youtube:player_client=android"}, useCookies: false},
    extractorAttempt{args: []string{"--extractor-args", "youtube:player_client=ios"}, useCookies: false},
)
```

**Testing**:
```bash
yt-dlp --extractor-args "youtube:player_client=android" -f "bestaudio" -g "VIDEO_URL"
```

## Solution 2: Use mpv with Built-in yt-dlp Support
**Why**: mpv handles yt-dlp integration internally, avoiding two-step extraction

**Advantages**:
- No manual stream URL extraction needed
- mpv updates its yt-dlp integration automatically
- Better error handling
- Simpler architecture

**Implementation**:
```go
// Use PlayWithMPV instead of Play in app.go
import "github.com/lootek/yt-daily-player/internal/player"

// Replace:
// err := player.Play(ctx, a.cfg.Global.Player, streamURL)
// With:
err := player.PlayWithMPV(ctx, a.cfg.Global.Player, videoURL)
```

**Dockerfile changes**:
```dockerfile
RUN apt-get install -y mpv yt-dlp
```

**Config changes**:
```yaml
player:
  command: mpv  # Instead of ffplay
  args: []      # mpv handles args internally
```

## Solution 3: Use Streamlink
**Why**: Streamlink is more stable for live/VOD streaming

```bash
streamlink --player ffplay \
  --player-args "-nodisp -autoexit" \
  "youtube.com/watch?v=VIDEO_ID" \
  audio_mp4
```

**Implementation**: Create `streamlink_player.go`

## Solution 4: Use yt-dlp Download + Play (Most Reliable)
**Why**: Eliminates streaming issues entirely

**Pros**:
- No stream extraction failures
- Can retry downloads
- Cached for replays
- Works with any player

**Cons**:
- Slower startup (download first)
- Requires disk space
- Not suitable for long videos

**Implementation**:
```bash
# Download first
yt-dlp -f bestaudio -o /tmp/audio.m4a "$VIDEO_URL"
# Play from disk
ffplay -nodisp -autoexit /tmp/audio.m4a
```

## Solution 5: Spotify/Local Music Alternative
If YouTube becomes too unreliable, consider:
- **Spotify API** with spotifyd (headless Spotify player)
- **Local music library** with mpd (Music Player Daemon)
- **Podcast feeds** via RSS with podboat

## Solution 6: YouTube Music API (Unofficial)
Use ytmusicapi (Python) for more stable access:
```python
from ytmusicapi import YTMusic
ytmusic = YTMusic()
search_results = ytmusic.search("query", filter="songs")
# More reliable than yt-dlp scraping
```

## Recommended Testing Order
1. ✅ Try Solution 1 (mobile clients) - **already enabled**
2. Run `./diagnose.sh` to identify exact failure
3. If still failing, switch to Solution 2 (mpv)
4. If video IDs work but streams fail, try Solution 4 (download+play)
5. If YouTube is completely blocked, consider Solution 5 (alternatives)

## Debugging Commands
```bash
# Run diagnostic
./diagnose.sh

# Test current implementation
go run . -config config.yaml

# Test with verbose yt-dlp
yt-dlp --verbose -f bestaudio -g "VIDEO_URL" 2>&1 | tee debug.log

# Test android client directly
yt-dlp --extractor-args "youtube:player_client=android" -f bestaudio -g "VIDEO_URL"

# Check if it's an IP block
curl -I https://www.youtube.com/watch?v=dQw4w9WgXcQ
```

## Docker Testing
```bash
# Rebuild with changes
docker compose build

# Run with logs
docker compose up

# Shell into container for debugging
docker compose run --rm app sh
yt-dlp --version
node --version
```

## Quick Fix Checklist
- [ ] Uncomment android/ios fallbacks (Done ✓)
- [ ] Update yt-dlp to latest: `pip install -U yt-dlp`
- [ ] Verify Node.js is available: `which node`
- [ ] Check PulseAudio: `pactl info`
- [ ] Test stream extraction: `./diagnose.sh`
- [ ] Check network/firewall rules
- [ ] Verify user-agent in config.yaml
- [ ] Update PO token if using Premium cookies
