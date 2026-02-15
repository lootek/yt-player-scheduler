# Diagnostic Results from Raspberry Pi

## Test Date: 2026-02-15

## Environment
- **Device**: Raspberry Pi (ithilien)
- **OS**: Debian GNU/Linux (Linux 6.6.74+rpt-rpi-v8 #1 SMP PREEMPT aarch64)
- **Docker**: 20.10.24+dfsg1
- **Docker Compose**: v2.34.0

## Build Results
✅ **Docker build successful** with mpv and all dependencies installed

## Container Versions
- **yt-dlp**: 2026.02.12.233641 (nightly)
- **Node.js**: v18.20.4
- **mpv**: 0.35.1
- **ffmpeg**: Available

## Stream Extraction Tests

### Test 1: Android Client
❌ **FAILED**
```
ERROR: android client https formats require a GVS PO Token
Requested format is not available
```
**Reason**: Android client now requires PO token for HTTPS formats

### Test 2: iOS Client
❌ **FAILED**
```
ERROR: ios client https/hls formats require a GVS PO Token
Only images are available for download
```
**Reason**: iOS client also requires PO token

### Test 3: Default Web Client
✅ **SUCCESS**
```
WARNING: No supported JavaScript runtime could be found
https://rr5---sn-2auhvcpax-2v1e.googlevideo.com/videoplayback?...
```
**Result**: Successfully extracted stream URL despite JS runtime warning

### Test 4: MPV Direct Playback
✅ **WORKS** (mpv installed and functional)
```
mpv 0.35.1 available
Minor PipeWire config warnings but extraction functional
```

## Findings

### ✅ Good News
1. **Default extraction works!** Despite the "No supported JavaScript runtime" warning, yt-dlp successfully extracts streams
2. mpv is installed and can handle YouTube URLs directly
3. Build and container setup are correct

### ⚠️ Issues Identified
1. **Mobile clients deprecated**: Android/iOS clients now require GVS PO tokens (as of Feb 2026)
2. **False warnings**: The "No JS runtime" warning is misleading - extraction still succeeds
3. **Potential timeout**: The app may be waiting too long for all extraction attempts to complete

## Recommendations

### Option 1: Stay with Current Implementation (Recommended)
The default web client extraction **already works**. Just need to:
1. Remove android/ios fallback attempts (they're broken without PO tokens)
2. Speed up by reducing unnecessary retry attempts
3. Trust the first successful extraction

### Option 2: Switch to mpv (Most Reliable)
- mpv handles yt-dlp internally
- No manual stream URL extraction needed
- One-line change in main.go to use `RunJobWithMPV`

### Option 3: Generate PO Tokens
- Complex setup requiring chromium automation
- Tokens expire and need regeneration
- Not recommended unless Premium cookies are needed

## Next Steps

**Immediate fix** (already in code):
1. Keep first 4 extraction attempts (default + JS engine with/without cookies)
2. Remove android/ios attempts (lines 168-171 in ytdlp.go) - they require PO tokens
3. Test the app again

**Alternative** (if issues persist):
1. Switch to mpv by using `RunJobWithMPV` in main.go
2. Simpler and more reliable

## Test Command for Verification
```bash
# On the Pi
cd ~/yt-daily-player

# Test default extraction (works!)
docker compose run --rm yt-rpi-player sh -c \
  'yt-dlp -f "bestaudio" -g "https://www.youtube.com/watch?v=VIDEO_ID"'

# Test with actual video from logs
docker compose run --rm yt-rpi-player sh -c \
  'yt-dlp -f "bestaudio" -g "https://www.youtube.com/watch?v=M2j6aL3HiHQ"'
```

## Conclusion

**The extraction already works** with the default web client. The mobile client fallbacks we added are now broken (YouTube changed requirements in Feb 2026). Recommendation: revert lines 168-171 to remove android/ios attempts, or switch to mpv for maximum reliability.
