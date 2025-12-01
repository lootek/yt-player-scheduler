package player

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/lootek/yt-rpi-player/internal/config"
)

func Play(ctx context.Context, cfg config.PlayerConfig, streamURL string) error {
	args := make([]string, 0, len(cfg.Args))
	placeholderFound := false
	for _, a := range cfg.Args {
		if strings.Contains(a, "{url}") {
			placeholderFound = true
		}
		args = append(args, strings.ReplaceAll(a, "{url}", streamURL))
	}
	if !placeholderFound {
		args = append(args, streamURL)
	}
	cmd := exec.CommandContext(ctx, cfg.Command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), formatEnv(cfg.Env)...)
	return cmd.Run()
}

func formatEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}
