#!/bin/bash
set -e

echo "=== YouTube Stream Extraction Diagnostic ==="
echo

# Test video URL
VIDEO_URL="https://www.youtube.com/watch?v=dQw4w9WgXcQ"

echo "1. Checking yt-dlp installation..."
if command -v yt-dlp &> /dev/null; then
    echo "✓ yt-dlp found: $(yt-dlp --version)"
else
    echo "✗ yt-dlp not found"
    exit 1
fi
echo

echo "2. Checking Node.js installation..."
if command -v node &> /dev/null; then
    echo "✓ node found: $(node --version)"
elif command -v nodejs &> /dev/null; then
    echo "✓ nodejs found: $(nodejs --version)"
else
    echo "✗ Node.js not found (required for signature extraction)"
fi
echo

echo "3. Checking npm installation..."
if command -v npm &> /dev/null; then
    echo "✓ npm found: $(npm --version)"
else
    echo "✗ npm not found"
fi
echo

echo "4. Checking ffplay installation..."
if command -v ffplay &> /dev/null; then
    echo "✓ ffplay found"
else
    echo "✗ ffplay not found"
    exit 1
fi
echo

echo "5. Testing basic yt-dlp search..."
yt-dlp --flat-playlist --dump-json --no-warnings --ignore-errors --limit 1 "ytsearchdate1:music" 2>&1 | head -5
echo

echo "6. Testing stream extraction (default)..."
echo "Command: yt-dlp -f 'bestaudio[ext=m4a]/bestaudio' -g '$VIDEO_URL'"
if yt-dlp -f "bestaudio[ext=m4a]/bestaudio" -g "$VIDEO_URL" 2>&1; then
    echo "✓ Default extraction works"
else
    echo "✗ Default extraction failed"
fi
echo

echo "7. Testing stream extraction (with JS engine)..."
echo "Command: yt-dlp --extractor-args 'youtube:js_engine=nodejs,player_client=default' -f 'bestaudio[ext=m4a]/bestaudio' -g '$VIDEO_URL'"
if yt-dlp --extractor-args "youtube:js_engine=nodejs,player_client=default" -f "bestaudio[ext=m4a]/bestaudio" -g "$VIDEO_URL" 2>&1; then
    echo "✓ JS engine extraction works"
else
    echo "✗ JS engine extraction failed"
fi
echo

echo "8. Testing stream extraction (android client)..."
echo "Command: yt-dlp --extractor-args 'youtube:player_client=android' -f 'bestaudio[ext=m4a]/bestaudio' -g '$VIDEO_URL'"
if yt-dlp --extractor-args "youtube:player_client=android" -f "bestaudio[ext=m4a]/bestaudio" -g "$VIDEO_URL" 2>&1; then
    echo "✓ Android client extraction works"
else
    echo "✗ Android client extraction failed"
fi
echo

echo "9. Testing yt-dlp with verbose output..."
yt-dlp --verbose -f "bestaudio[ext=m4a]/bestaudio" -g "$VIDEO_URL" 2>&1 | grep -E "(ERROR|WARNING|Extracting|Selected format)" | head -20
echo

echo "=== Diagnostic Complete ==="
