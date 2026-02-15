# YouTube Premium Cookies Setup

## Benefits
- Ad-free streams
- Better format selection
- Legitimate use of your Premium subscription

## Quick Setup

### 1. Export Cookies
**Browser extension** (recommended):
- Install "Get cookies.txt" for Safari/Firefox
- Visit youtube.com (logged in)
- Click extension → Export → saves to Downloads

**Or use yt-dlp**:
```bash
yt-dlp --cookies-from-browser firefox --cookies cookies.txt --skip-download https://youtube.com
```

### 2. Deploy to Pi
```bash
scp cookies.txt pi@192.168.10.22:~/yt-daily-player/
ssh pi@192.168.10.22 'cd ~/yt-daily-player && docker compose restart'
```

### 3. Test
```bash
ssh pi@192.168.10.22
docker compose run --rm yt-rpi-player sh -c \
  'yt-dlp --cookies /app/cookies.txt -F VIDEO_URL | head -20'
```

## Troubleshooting
```bash
# Verify file exists
ssh pi@192.168.10.22 "ls -la ~/yt-daily-player/cookies.txt"

# Check mounted in container
docker compose exec yt-rpi-player ls -la /app/cookies.txt
```

**Cookies expired?** Re-export and restart container.

## Security
- ✅ Read-only in container
- ✅ Never leaves your Pi
- ⚠️ In .gitignore (don't commit)
- ⚠️ Re-export every 1-6 months

## Note
**App works without cookies** - you'll just have ads in streams.
