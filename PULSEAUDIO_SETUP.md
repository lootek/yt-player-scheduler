# PulseAudio TCP Configuration

The container uses PulseAudio over TCP to play audio on the host.

## Host Configuration Required

Edit  and enable TCP module with anonymous auth:

```
load-module module-native-protocol-tcp auth-anonymous=1
```

Restart PulseAudio (or restart mpd if it spawns PulseAudio):
```bash
sudo systemctl restart mpd
```

Verify TCP port is listening:
```bash
ss -tlnp | grep 4713
```

## Container Configuration

Set environment variable in docker-compose.yaml:
```yaml
environment:
  PULSE_SERVER: tcp:localhost:4713
```

## Testing

Test audio from container:
```bash
docker exec -e PULSE_SERVER=tcp:localhost:4713 yt-rpi-player \
  ffplay -nodisp -autoexit -t 3 -f lavfi -i "sine=frequency=440:duration=3"
```

You should hear a 440Hz tone for 3 seconds.
