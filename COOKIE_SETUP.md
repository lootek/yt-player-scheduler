# Setting Up YouTube Premium Cookies

## Why Use Cookies?
- ✅ Ad-free streams (no ads in videos)
- ✅ Better format selection
- ✅ Legitimate personal use of your Premium subscription
- ✅ Very low risk (your own account, low volume)

## Method 1: Browser Extension (Recommended) ⭐

### Steps:
1. **Install Extension**:
   - Safari: Search for "Get cookies.txt" extension
   - Firefox: https://addons.mozilla.org/en-US/firefox/addon/cookies-txt/

2. **Export Cookies**:
   - Open Safari/Firefox
   - Go to https://youtube.com (make sure you're logged in)
   - Click the extension icon
   - Click "Export" → saves `cookies.txt` to Downloads

3. **Copy to Repository**:
   ```bash
   cp ~/Downloads/cookies.txt ~/projects/github/lootek/yt-daily-player/cookies.txt
   ```

4. **Deploy to Pi**:
   ```bash
   scp ~/projects/github/lootek/yt-daily-player/cookies.txt pi@192.168.10.22:~/yt-daily-player/
   ssh pi@192.168.10.22 'cd ~/yt-daily-player && docker compose restart'
   ```

## Method 2: Manual Cookie Export (Firefox)

Firefox makes it easier to access cookies:

1. Open Firefox and go to YouTube (logged in)
2. Press `F12` to open Developer Tools
3. Go to "Storage" tab → "Cookies" → "https://youtube.com"
4. Right-click → "Export All" (if available)
5. Or manually copy important cookies

### Key Cookies Needed:
- `VISITOR_INFO1_LIVE`
- `PREF`
- `LOGIN_INFO`
- `SID`, `HSID`, `SSID`, `APISID`, `SAPISID`

## Method 3: Use yt-dlp's Cookie Extraction

yt-dlp can extract cookies from browsers directly:

```bash
# On your Mac
cd ~/projects/github/lootek/yt-daily-player

# Extract from Firefox (if you use it)
yt-dlp --cookies-from-browser firefox --cookies cookies.txt --skip-download https://youtube.com/watch?v=dQw4w9WgXcQ

# Then copy to Pi
scp cookies.txt pi@192.168.10.22:~/yt-daily-player/
```

## Method 4: Safari Manual Export (Most Reliable)

Since Safari's cookie database is protected, use Firefox temporarily:

1. **Install Firefox** (if not already)
2. **Login to YouTube** in Firefox
3. **Export cookies** using extension or yt-dlp
4. **Copy to Pi** as shown above

## Testing

After copying cookies.txt to Pi, test:

```bash
ssh pi@192.168.10.22
cd ~/yt-daily-player

# Test cookies work
docker compose run --rm yt-rpi-player sh -c \
  'yt-dlp --cookies /app/cookies.txt -F https://youtube.com/watch?v=dQw4w9WgXcQ | head -20'

# Should show Premium formats without ads
```

## Troubleshooting

### "No such file or directory: cookies.txt"
```bash
# Make sure file exists on Pi
ssh pi@192.168.10.22 "ls -la ~/yt-daily-player/cookies.txt"

# Check it's mounted in container
ssh pi@192.168.10.22 "docker compose -f ~/yt-daily-player/docker-compose.yaml exec yt-rpi-player ls -la /app/cookies.txt"
```

### "Cookies expired"
- Re-export cookies from browser
- Copy new version to Pi
- Restart container

### "Still seeing ads"
- Verify you're logged into YouTube Premium in browser
- Check cookie file has content: `cat cookies.txt | grep youtube`
- Try re-authenticating in browser before export

## Security Notes

- ✅ **Safe**: cookies.txt is read-only in container
- ✅ **Private**: File never leaves your Pi
- ✅ **Personal use**: Legitimate use of your Premium subscription
- ⚠️ **Don't commit**: cookies.txt is in .gitignore
- ⚠️ **Expires**: Re-export if streams stop working (typically every 1-6 months)

## Quick Reference

```bash
# Export cookies (Firefox method)
yt-dlp --cookies-from-browser firefox --cookies cookies.txt --skip-download https://youtube.com

# Copy to Pi
scp cookies.txt pi@192.168.10.22:~/yt-daily-player/

# Restart app
ssh pi@192.168.10.22 'cd ~/yt-daily-player && docker compose restart'

# Verify it's working
ssh pi@192.168.10.22 'cd ~/yt-daily-player && docker compose logs --tail=50'
```

## Alternative: No Cookies

If you don't want to set up cookies, **the app works fine without them**! You'll just have:
- Ads in streams (usually at beginning/end)
- Standard format selection (still good quality audio)

The app is fully functional either way! 🎉
