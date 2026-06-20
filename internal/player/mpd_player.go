package player

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fhs/gompd/v2/mpd"
	"github.com/lootek/yt-rpi-player/internal/config"
)

// EnqueueMPD appends one or more URIs to the MPD playlist without blocking.
// If autoPlay is true, it starts playing the first added item.
func EnqueueMPD(ctx context.Context, cfg config.MPDConfig, downloadDir string, uris []string, autoPlay bool) error {
	if len(uris) == 0 {
		return nil
	}

	var client *mpd.Client
	var err error

	if cfg.Password != "" {
		client, err = mpd.DialAuthenticated(cfg.Network, cfg.Address, cfg.Password)
	} else {
		client, err = mpd.Dial(cfg.Network, cfg.Address)
	}

	if err != nil {
		return fmt.Errorf("mpd dial (%s:%s): %w", cfg.Network, cfg.Address, err)
	}
	defer client.Close()

	updatedDirs := make(map[string]struct{})
	var relURIs []string
	firstID := -1

	for _, uri := range uris {
		rel, ok := mapToMusicRoot(cfg, downloadDir, uri)
		if ok {
			parent := filepath.Dir(rel)
			if parent != "." {
				updatedDirs[parent] = struct{}{}
			}
		}
		relURIs = append(relURIs, rel)
	}

	for dir := range updatedDirs {
		if _, err := client.Update(dir); err != nil {
			return fmt.Errorf("mpd update failed on %s: %w", dir, err)
		}
	}
	if len(updatedDirs) > 0 {
		// wait for mpd update
		time.Sleep(time.Second * 30)
	}

	for _, rel := range relURIs {
		id, err := client.AddID(rel, -1)
		if err != nil {
			return fmt.Errorf("mpd addid failed on %v: %w", rel, err)
		}
		if firstID == -1 {
			firstID = id
		}
	}

	if autoPlay && firstID != -1 {
		if err := client.PlayID(firstID); err != nil {
			return fmt.Errorf("mpd playid failed on %v: %w", firstID, err)
		}
	}

	return nil
}

// mapToMusicRoot rewrites an absolute file path into a path relative to
// cfg.MusicRoot when the file lives under downloadDir which itself lives
// under MusicRoot. The boolean indicates whether a rewrite happened.
func mapToMusicRoot(cfg config.MPDConfig, downloadDir, uri string) (string, bool) {
	if downloadDir == "" || cfg.MusicRoot == "" {
		return uri, false
	}
	if !strings.HasPrefix(uri, downloadDir) {
		return uri, false
	}
	if !strings.HasPrefix(downloadDir, cfg.MusicRoot) {
		return uri, false
	}
	rel, err := filepath.Rel(cfg.MusicRoot, uri)
	if err != nil {
		return uri, false
	}
	return rel, true
}

// PlayWithMPD adds the URI to the end of the MPD playlist and starts playing it.
func PlayWithMPD(ctx context.Context, cfg config.MPDConfig, downloadDir string, uri string) error {
	var client *mpd.Client
	var err error

	if cfg.Password != "" {
		client, err = mpd.DialAuthenticated(cfg.Network, cfg.Address, cfg.Password)
	} else {
		client, err = mpd.Dial(cfg.Network, cfg.Address)
	}

	if err != nil {
		return fmt.Errorf("mpd dial (%s:%s): %w", cfg.Network, cfg.Address, err)
	}
	defer client.Close()

	// If the URI is within DownloadDir and we have a MusicRoot configured,
	// replace the DownloadDir prefix with MusicRoot so MPD can find it.
	rel, ok := mapToMusicRoot(cfg, downloadDir, uri)
	if ok {
		uri = rel

		// log.Printf("updating mpd at %q...", rel)
		if _, err := client.Update(rel); err != nil {
			return fmt.Errorf("mpd update failed on %s: %w", rel, err)
		}

		// wait for mpd update
		time.Sleep(time.Second * 30)
	}

	// Add to the end of the playlist and get its ID
	id, err := client.AddID(uri, -1)
	if err != nil {
		return fmt.Errorf("mpd addid failed on %v: %w", uri, err)
	}

	// log.Printf("added %q to playlist at %v", uri, id)

	// Start playing the added item
	if err := client.PlayID(id); err != nil {
		return fmt.Errorf("mpd playid failed on %v: %w", id, err)
	}

	// Monitor playback status until it finishes or context is cancelled.
	// This maintains consistency with other player implementations that block.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			status, err := client.Status()
			if err != nil {
				return fmt.Errorf("mpd status failed: %w", err)
			}

			// If MPD is not playing anything, or playing a different ID, we assume we're done.
			// Note: status["songid"] is the ID of the current song.
			if status["state"] != "play" || status["songid"] != fmt.Sprintf("%d", id) {
				log.Printf("mpd status: %#v", status)
				return nil
			}
		}
	}
}
