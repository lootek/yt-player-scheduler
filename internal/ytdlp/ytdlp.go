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

// CheckAuth verifies that yt-dlp can access Watch Later using the configured cookies.
// It returns the title of the first item when successful.
func (c Client) CheckAuth(ctx context.Context) (string, error) {
	if c.cfg.Cookies == "" {
		return "", errors.New("no cookies configured")
	}

	cookiePath, cleanup, err := c.prepareCookies()
	if err != nil {
		return "", fmt.Errorf("prepare cookies: %w", err)
	}
	defer cleanup()

	args := append(c.baseArgs(),
		"--cookies", cookiePath,
		"--playlist-items", "1",
		"--skip-download",
		"-O", "%(title)s",
		"https://www.youtube.com/playlist?list=WL",
	)
	cmd := exec.CommandContext(ctx, c.binary(), args...)
	cmdLine := cmd.String()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)

	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("yt-dlp auth check (%s): %s: %w", cmdLine, msg, err)
		}
		return "", fmt.Errorf("yt-dlp auth check (%s): %w", cmdLine, err)
	}
	title := strings.TrimSpace(stdout.String())
	if title == "" {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("yt-dlp auth check (%s): empty output (stderr: %s)", cmdLine, msg)
		}
		return "", errors.New("yt-dlp auth check: empty output")
	}
	return title, nil
}

func (c Client) Search(ctx context.Context, query string, limit int) ([]VideoEntry, error) {
	if limit <= 0 {
		limit = config.DefaultSearchLimit
	}
	cookiePath, cleanup, err := c.prepareCookies()
	if err != nil {
		return nil, fmt.Errorf("prepare cookies: %w", err)
	}
	defer cleanup()

	searchSpec := fmt.Sprintf("ytsearchdate%d:%s", limit, query)
	args := append(c.baseArgs(),
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--ignore-errors",
		"--limit", fmt.Sprint(limit),
		searchSpec,
	)
	if cookiePath != "" {
		args = append(args, "--cookies", cookiePath)
	}
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
	cookiePath, cleanup, err := c.prepareCookies()
	if err != nil {
		return "", fmt.Errorf("prepare cookies: %w", err)
	}
	defer cleanup()

	type extractorAttempt struct {
		args       []string
		useCookies bool
	}

	attempts := []extractorAttempt{
		{args: nil, useCookies: true}, // default: pass cookies if available
	}
	if cookiePath != "" {
		// Try once without cookies; sometimes logged-in context breaks formats.
		attempts = append(attempts, extractorAttempt{args: nil, useCookies: false})
	}
	if !hasExtractorArgs(c.cfg.ExtraArgs) {
		// Force JS engine detection (helps when yt-dlp fails to see node).
		attempts = append(attempts,
			extractorAttempt{args: []string{"--extractor-args", "youtube:js_engine=nodejs,player_client=default"}, useCookies: true},
		)
		if cookiePath != "" {
			attempts = append(attempts,
				extractorAttempt{args: []string{"--extractor-args", "youtube:js_engine=nodejs,player_client=default"}, useCookies: false},
			)
		}
		// Fallback clients that often bypass signature challenges (without cookies).
		// NOTE: As of Feb 2026, android/ios clients require GVS PO tokens and fail without them.
		// Keeping this commented out until PO token generation is implemented.
		// attempts = append(attempts,
		// 	extractorAttempt{args: []string{"--extractor-args", "youtube:player_client=android"}, useCookies: false},
		// 	extractorAttempt{args: []string{"--extractor-args", "youtube:player_client=ios"}, useCookies: false},
		// )
	}

	var lastErr error
	for i, extra := range attempts {
		if i > 0 {
			fmt.Fprintf(os.Stderr, "yt-dlp: retrying with args %v (cookies:%v)\n", extra.args, extra.useCookies && cookiePath != "")
		}
		stream, err := c.resolveStreamWithArgs(ctx, videoURL, cookiePath, extra.args, extra.useCookies)
		if err == nil {
			return stream, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("empty stream URL")
}

func (c Client) baseArgs() []string {
	args := make([]string, 0, len(c.cfg.ExtraArgs)+2)
	args = append(args, c.cfg.ExtraArgs...)
	if c.cfg.UserAgent != "" {
		args = append(args, "--user-agent", c.cfg.UserAgent)
	}
	if c.cfg.POToken != "" {
		args = append(args, "--po-token", c.cfg.POToken)
	}
	if c.cfg.POTokenProvider != "" {
		args = append(args, "--po-token-provider", c.cfg.POTokenProvider)
	}
	if len(c.cfg.POTokenProviderArgs) > 0 {
		args = append(args, "--po-token-provider-args")
		args = append(args, c.cfg.POTokenProviderArgs...)
	}
	if !hasRemoteComponents(args) {
		args = append(args, "--remote-components", "ejs:npm")
	}
	// Explicitly tell yt-dlp to use 'node' runtime (not 'nodejs')
	args = append(args, "--js-runtimes", "node")
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

func (c Client) resolveStreamWithArgs(ctx context.Context, videoURL, cookiePath string, extraArgs []string, useCookies bool) (string, error) {
	args := append([]string{}, c.baseArgs()...)
	args = append(args, extraArgs...)
	args = append(args,
		"-f", "bestaudio[ext=m4a]/bestaudio",
		"-g", videoURL,
	)
	if useCookies && cookiePath != "" {
		args = append(args, "--cookies", cookiePath)
	}
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

func (c Client) prepareCookies() (string, func(), error) {
	if c.cfg.Cookies == "" {
		return "", func() {}, nil
	}

	src, err := os.Open(c.cfg.Cookies)
	if err != nil {
		return "", func() {}, fmt.Errorf("open cookies file %q: %w", c.cfg.Cookies, err)
	}
	defer src.Close()

	tmp, err := os.CreateTemp("", "yt-dlp-cookies-*.txt")
	if err != nil {
		return "", func() {}, fmt.Errorf("create temp cookies file: %w", err)
	}

	cleanup := func() { _ = os.Remove(tmp.Name()) }

	if _, err := io.Copy(tmp, src); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("copy cookies to temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close temp cookies file: %w", err)
	}

	return tmp.Name(), cleanup, nil
}

func hasExtractorArgs(args []string) bool {
	for i, a := range args {
		if a == "--extractor-args" {
			return true
		}
		if strings.HasPrefix(a, "--extractor-args=") {
			return true
		}
		// Handle positional value after flag.
		if a == "--extractor-args" && i+1 < len(args) && strings.HasPrefix(args[i+1], "youtube:player_client=") {
			return true
		}
	}
	return false
}

func hasRemoteComponents(args []string) bool {
	for _, a := range args {
		if a == "--remote-components" {
			return true
		}
		if strings.HasPrefix(a, "--remote-components=") {
			return true
		}
	}
	return false
}
