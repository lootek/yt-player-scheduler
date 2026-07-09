#!/usr/bin/env python3
"""
Export YouTube cookies from Safari to Netscape cookies.txt format.
Works with Safari's binary cookie format on macOS.
"""

import struct
import sys
from pathlib import Path
from datetime import datetime, timezone

SAFARI_COOKIE_LOCATIONS = [
    Path.home() / 'Library/Cookies/Cookies.binarycookies',
    Path.home() / 'Library/Containers/com.apple.Safari/Data/Library/Cookies/Cookies.binarycookies',
    Path.home() / 'Library/Safari/Cookies/Cookies.binarycookies',
]

def find_safari_cookies():
    """Find Safari cookies file."""
    for path in SAFARI_COOKIE_LOCATIONS:
        if path.exists():
            return path
    return None

def read_safari_cookies(cookie_file):
    """Parse Safari's binary cookie format."""
    cookies = []

    try:
        with open(cookie_file, 'rb') as f:
            # Read magic bytes
            magic = f.read(4)
            if magic != b'cook':
                print(f"❌ Invalid cookie file format")
                return []

            # Read number of pages
            num_pages = struct.unpack('>I', f.read(4))[0]

            # Read page sizes
            page_sizes = []
            for _ in range(num_pages):
                page_sizes.append(struct.unpack('>I', f.read(4))[0])

            # Read each page
            for page_size in page_sizes:
                page_data = f.read(page_size)

                # Parse page header
                if len(page_data) < 4:
                    continue

                page_magic = page_data[:4]
                if page_magic != b'\x00\x00\x01\x00':
                    continue

                num_cookies = struct.unpack('<I', page_data[4:8])[0]

                # Read cookie offsets
                cookie_offsets = []
                for i in range(num_cookies):
                    offset = struct.unpack('<I', page_data[8 + i*4:12 + i*4])[0]
                    cookie_offsets.append(offset)

                # Parse each cookie
                for offset in cookie_offsets:
                    if offset >= len(page_data):
                        continue

                    try:
                        cookie_data = page_data[offset:]

                        # Cookie structure (simplified parsing)
                        # Skip to interesting fields
                        pos = 4  # Skip size

                        # Read flags
                        flags = struct.unpack('<I', cookie_data[pos:pos+4])[0]
                        pos += 4

                        # Skip some fields
                        pos += 4  # unknown

                        # Read URL offset
                        url_offset = struct.unpack('<I', cookie_data[pos:pos+4])[0]
                        pos += 4

                        # Read name offset
                        name_offset = struct.unpack('<I', cookie_data[pos:pos+4])[0]
                        pos += 4

                        # Read path offset
                        path_offset = struct.unpack('<I', cookie_data[pos:pos+4])[0]
                        pos += 4

                        # Read value offset
                        value_offset = struct.unpack('<I', cookie_data[pos:pos+4])[0]
                        pos += 4

                        pos += 8  # Skip comment

                        # Read expiry date (Mac absolute time - seconds since 2001-01-01)
                        expiry_date = struct.unpack('<d', cookie_data[pos:pos+8])[0]
                        pos += 8

                        # Convert Mac absolute time to Unix timestamp
                        # Mac epoch: 2001-01-01, Unix epoch: 1970-01-01
                        # Difference: 978307200 seconds
                        if expiry_date > 0:
                            unix_timestamp = int(expiry_date + 978307200)
                        else:
                            unix_timestamp = 0

                        # Read strings (null-terminated)
                        def read_string(data, offset):
                            end = data.find(b'\x00', offset)
                            if end == -1:
                                return ""
                            return data[offset:end].decode('utf-8', errors='ignore')

                        domain = read_string(cookie_data, url_offset)
                        name = read_string(cookie_data, name_offset)
                        path = read_string(cookie_data, path_offset)
                        value = read_string(cookie_data, value_offset)

                        # Filter for YouTube/Google cookies
                        if domain and ('youtube.com' in domain or 'google.com' in domain):
                            is_secure = bool(flags & 0x1)

                            cookies.append({
                                'domain': domain,
                                'name': name,
                                'value': value,
                                'path': path,
                                'expires': unix_timestamp,
                                'secure': is_secure
                            })

                    except Exception as e:
                        # Skip malformed cookies
                        continue

    except FileNotFoundError:
        print(f"❌ Safari cookies not found at: {cookie_file}")
        print("Make sure you've logged into YouTube in Safari.")
        return []
    except Exception as e:
        print(f"❌ Error reading cookies: {e}")
        return []

    return cookies

def export_to_netscape(cookies, output_file='cookies.txt'):
    """Export cookies to Netscape format."""
    if not cookies:
        print("❌ No YouTube cookies found. Please login to YouTube in Safari first.")
        return False

    with open(output_file, 'w') as f:
        f.write("# Netscape HTTP Cookie File\n")
        f.write("# This file was generated by export-cookies-safari.py\n")
        f.write("# Edit at your own risk.\n\n")

        for cookie in cookies:
            domain = cookie['domain']
            name = cookie['name']
            value = cookie['value']
            path = cookie['path']
            expires = cookie['expires']
            secure = cookie['secure']

            # Netscape format: domain, flag, path, secure, expiration, name, value
            domain_flag = "TRUE" if domain.startswith('.') else "FALSE"
            secure_flag = "TRUE" if secure else "FALSE"

            line = f"{domain}\t{domain_flag}\t{path}\t{secure_flag}\t{expires}\t{name}\t{value}\n"
            f.write(line)

    print(f"✅ Exported {len(cookies)} YouTube/Google cookies to {output_file}")
    return True

def main():
    print("Safari YouTube Cookie Exporter")
    print("=" * 50)
    print()

    output = 'cookies.txt'
    if len(sys.argv) > 1:
        output = sys.argv[1]

    print("Looking for Safari cookies...")
    cookie_file = find_safari_cookies()

    if not cookie_file:
        print("❌ Safari cookies not found in any known location:")
        for loc in SAFARI_COOKIE_LOCATIONS:
            print(f"   ✗ {loc}")
        print("\n⚠️  Safari may require Full Disk Access permission.")
        print("\nTo grant permission:")
        print("  1. Open System Settings → Privacy & Security → Full Disk Access")
        print("  2. Add Terminal (or iTerm) to the list")
        print("  3. Restart terminal and try again")
        return 1

    print(f"✅ Found cookies at: {cookie_file}")
    cookies = read_safari_cookies(cookie_file)

    if not cookies:
        print("\n⚠️  No YouTube cookies found!")
        print("\nMake sure:")
        print("  1. You're logged into YouTube in Safari")
        print("  2. Safari is closed (optional but recommended)")
        print("  3. You've visited youtube.com recently")
        return 1

    success = export_to_netscape(cookies, output)

    if success:
        print(f"\n✅ Success! Cookies saved to: {output}")
        print(f"\nNext steps:")
        print(f"  1. Copy to Pi: scp {output} ${{RPI_USER}}@${{RPI_IP}}:~/yt-daily-player/")
        print(f"  2. Restart: ssh ${{RPI_USER}}@${{RPI_IP}} 'cd ~/yt-daily-player && docker compose restart'")
        print(f"\n🎉 Your streams will now be ad-free!")
        return 0

    return 1

if __name__ == '__main__':
    sys.exit(main())
