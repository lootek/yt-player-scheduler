package ytdlp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/lootek/yt-rpi-player/internal/config"
)

type Client struct {
	cfg config.YtDLPConfig
}

type VideoEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	WebpageURL string `json:"webpage_url"`
	URL        string `json:"url"`
}

func New(cfg config.YtDLPConfig) Client {
	return Client{cfg: cfg}
}

func (c Client) Search(ctx context.Context, query string, limit int) ([]VideoEntry, error) {
	if limit <= 0 {
		limit = config.DefaultSearchLimit
	}
	searchSpec := fmt.Sprintf("ytsearchdate%d:%s", limit, query)
	args := append(c.baseArgs(),
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--ignore-errors",
		"--limit", fmt.Sprint(limit),
		searchSpec,
	)
	cmd := exec.CommandContext(ctx, c.binary(), args...)
	cmdLine := cmd.String()
	stderr := &bytes.Buffer{}
	cmd.Stderr = io.MultiWriter(os.Stderr, stderr)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stdout (%s): %w", cmdLine, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start yt-dlp (%s): %w", cmdLine, err)
	}

	var results []VideoEntry
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		line := scanner.Bytes()
		var v VideoEntry
		if err := json.Unmarshal(line, &v); err != nil {
			continue
		}
		if v.WebpageURL == "" && v.URL == "" && v.ID == "" {
			continue
		}
		results = append(results, v)
		if len(results) >= limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan yt-dlp output (%s): %w", cmdLine, err)
	}
	if err := cmd.Wait(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("yt-dlp search failed (%s): %s: %w", cmdLine, msg, err)
		}
		return nil, fmt.Errorf("yt-dlp search failed (%s): %w", cmdLine, err)
	}
	return results, nil
}

func (c Client) ResolveStream(ctx context.Context, videoURL string) (string, error) {
	args := append(c.baseArgs(),
		"-f", "bestaudio[ext=m4a]/bestaudio",
		"-g", videoURL,
	)
	cmd := exec.CommandContext(ctx, c.binary(), args...)
	cmdLine := cmd.String()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("yt-dlp -g (%s): %s: %w", cmdLine, msg, err)
		}
		return "", fmt.Errorf("yt-dlp -g (%s): %w", cmdLine, err)
	}
	stream := strings.TrimSpace(stdout.String())
	if stream == "" {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("empty stream URL (%s) (stderr: %s)", cmdLine, msg)
		}
		return "", errors.New("empty stream URL")
	}
	return stream, nil
}

func (c Client) baseArgs() []string {
	args := make([]string, 0, len(c.cfg.ExtraArgs)+2)
	args = append(args, c.cfg.ExtraArgs...)
	if c.cfg.Cookies != "" {
		args = append(args, "--cookies", c.cfg.Cookies)
	}
	return args
}

func (c Client) binary() string {
	if c.cfg.Binary != "" {
		return c.cfg.Binary
	}
	return "yt-dlp"
}

func (v VideoEntry) VideoURL() string {
	switch {
	case v.WebpageURL != "":
		return v.WebpageURL
	case v.URL != "":
		return v.URL
	case v.ID != "":
		return "https://www.youtube.com/watch?v=" + v.ID
	default:
		return ""
	}
}
