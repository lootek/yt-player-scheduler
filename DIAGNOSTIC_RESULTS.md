# Diagnostic Results (2026-02-15)

## Environment
- Raspberry Pi (Debian, Linux 6.6.74+rpt-rpi-v8 aarch64)
- Docker 20.10.24, Compose v2.34.0
- yt-dlp 2026.02.12.233641, Node.js v18.20.4, mpv 0.35.1

## Test Results

| Method | Status | Notes |
|--------|--------|-------|
| Default web client | ✅ SUCCESS | Works despite JS warning |
| mpv direct | ✅ SUCCESS | Functional |
| Android client | ❌ FAILED | Requires PO token (Feb 2026) |
| iOS client | ❌ FAILED | Requires PO token |

## Findings
- **Default extraction works** without mobile clients
- Mobile fallbacks now broken (require GVS PO tokens)
- mpv is installed and functional
- "No JS runtime" warning is misleading - extraction succeeds

## Recommendations
1. **Current approach**: Remove android/ios attempts (lines 168-171 in ytdlp.go)
2. **Alternative**: Switch to mpv via `RunJobWithMPV` in main.go
