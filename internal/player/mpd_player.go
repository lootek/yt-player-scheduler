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
	if downloadDir != "" && cfg.MusicRoot != "" && strings.HasPrefix(uri, downloadDir) && strings.HasPrefix(downloadDir, cfg.MusicRoot) {
		rel, err := filepath.Rel(cfg.MusicRoot, uri)
		if err == nil {
			uri = rel
		}

		log.Printf("updating mpd at %q...", rel)
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

	log.Printf("added %q to playlist at %v", uri, id)

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

			log.Printf("mpd status: %#v", status)

			// If MPD is not playing anything, or playing a different ID, we assume we're done.
			// Note: status["songid"] is the ID of the current song.
			// if status["state"] != "play" || status["songid"] != fmt.Sprintf("%d", id) {
			// 	return nil
			// }
		}
	}
}
