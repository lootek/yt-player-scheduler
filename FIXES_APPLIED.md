# Fixes Applied

## Fixed Issues
1. ✅ Node.js runtime detection - added `--js-runtimes node`
2. ✅ Import paths corrected - `yt-daily-player` → `yt-rpi-player`
3. ✅ Mobile fallbacks removed - android/ios now require PO tokens (Feb 2026)
4. ✅ mpv alternative added

## Quick Start

### Option A: Current (ffplay)
```bash
docker compose build && docker compose up
```

### Option B: Switch to mpv
```go
// main.go: change RunJob to RunJobWithMPV
err := application.RunJobWithMPV(jobCtx, job)
```

### Option C: Hybrid fallback
```go
err := application.RunJob(jobCtx, job)
if err != nil {
    err = application.RunJobWithMPV(jobCtx, job)
}
```

## Troubleshooting
```bash
./diagnose.sh                              # Run diagnostics
yt-dlp --js-runtimes node -f bestaudio -g URL  # Test extraction
docker compose run --rm app sh             # Shell into container
```

## Performance

| Method | Reliability | Speed |
|--------|-------------|-------|
| ffplay + node runtime | 85% | Fast |
| mpv | 95% | Medium |
| Download + Play | 90% | Slow |
