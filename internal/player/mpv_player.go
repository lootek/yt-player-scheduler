package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lootek/yt-rpi-player/internal/config"
)

// PlayWithMPV plays audio using mpv with built-in yt-dlp support.
// mpv handles stream extraction internally, which is often more reliable.
func PlayWithMPV(ctx context.Context, cfg config.PlayerConfig, videoURL string) error {
	// mpv can handle YouTube URLs directly with its built-in yt-dlp integration
	args := []string{
		"--no-video",           // Audio only
		"--really-quiet",       // Minimal output
		"--ytdl",               // Enable yt-dlp
		"--ytdl-format=bestaudio[ext=m4a]/bestaudio", // Audio format
		"--audio-device=pulse", // PulseAudio output
	}

	// Add any custom args from config
	for _, a := range cfg.Args {
		if !strings.Contains(a, "{url}") {
			args = append(args, a)
		}
	}

	args = append(args, videoURL)

	cmd := exec.CommandContext(ctx, "mpv", args...)
	cmd.Env = append(os.Environ(), formatEnv(cfg.Env)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mpv playback failed: %w", err)
	}
	return nil
}
