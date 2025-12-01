package ytdlp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("pipe stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start yt-dlp: %w", err)
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
		return nil, fmt.Errorf("scan yt-dlp output: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("yt-dlp search failed: %w", err)
	}
	return results, nil
}

func (c Client) ResolveStream(ctx context.Context, videoURL string) (string, error) {
	args := append(c.baseArgs(),
		"-f", "bestaudio[ext=m4a]/bestaudio",
		"-g", videoURL,
	)
	cmd := exec.CommandContext(ctx, c.binary(), args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("yt-dlp -g: %w", err)
	}
	stream := strings.TrimSpace(string(out))
	if stream == "" {
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
