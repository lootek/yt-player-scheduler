package player

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fhs/gompd/v2/mpd"
	"github.com/lootek/yt-rpi-player/internal/config"
)

// resumeEntry is a remembered prior-playback position to be restored after a
// newly appended item finishes.
type resumeEntry struct {
	songid  int
	elapsed float64 // seconds, parsed from MPD Status()["elapsed"]
}

// resumeState is a process-wide singleton tracking the LIFO stack of remembered
// items and the single background watcher goroutine that resumes them. Both
// EnqueueMPD (web UI) and PlayWithMPD (cron) share this state so chain resumes
// work across the two entry points.
var resumeState = struct {
	mu      sync.Mutex
	cfg     config.MPDConfig
	init    bool
	stack   []resumeEntry
	watcher *watcher
	watchWG sync.WaitGroup
}{}

type watcher struct {
	cfg      config.MPDConfig
	expected int // songid the watcher is currently tracking as "the playing item"
	done     chan struct{}
}

func initResume(cfg config.MPDConfig) {
	resumeState.mu.Lock()
	defer resumeState.mu.Unlock()
	if !resumeState.init {
		resumeState.cfg = cfg
		resumeState.init = true
	}
}

func dialMPD(cfg config.MPDConfig) (*mpd.Client, error) {
	if cfg.Password != "" {
		return mpd.DialAuthenticated(cfg.Network, cfg.Address, cfg.Password)
	}
	return mpd.Dial(cfg.Network, cfg.Address)
}

// parseSongid returns the songid from MPD Status() attrs, or -1 if absent /
// unparseable (e.g. empty playlist / stopped state).
func parseSongid(attrs mpd.Attrs) int {
	s := attrs["songid"]
	if s == "" {
		return -1
	}
	id, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return id
}

// parseElapsed returns the elapsed seconds from MPD Status() attrs, or 0 if
// absent / unparseable.
func parseElapsed(attrs mpd.Attrs) float64 {
	s := attrs["elapsed"]
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// pushRememberedLocked appends a remembered item to the stack. Caller holds
// resumeState.mu.
func pushRememberedLocked(songid int, elapsed float64) {
	if songid < 0 {
		return
	}
	resumeState.stack = append(resumeState.stack, resumeEntry{songid: songid, elapsed: elapsed})
}

// startWatcherIfNeededLocked launches the background watcher goroutine if it
// is not already running, with its expected songid set to the just-appended new
// item. Caller holds resumeState.mu.
func startWatcherIfNeededLocked(expected int) {
	if resumeState.watcher != nil {
		// Update the existing watcher's expected id so it tracks the newer item.
		resumeState.watcher.expected = expected
		return
	}
	w := &watcher{cfg: resumeState.cfg, expected: expected, done: make(chan struct{})}
	resumeState.watcher = w
	resumeState.watchWG.Add(1)
	go w.run()
}

// popResumeLocked pops the top of the stack. Caller holds resumeState.mu.
// Returns the entry and ok=false if the stack is empty.
func popResumeLocked() (resumeEntry, bool) {
	if len(resumeState.stack) == 0 {
		return resumeEntry{}, false
	}
	top := resumeState.stack[len(resumeState.stack)-1]
	resumeState.stack = resumeState.stack[:len(resumeState.stack)-1]
	return top, true
}

func (w *watcher) run() {
	defer resumeState.watchWG.Done()
	client, err := dialMPD(w.cfg)
	if err != nil {
		log.Printf("mpd resume watcher: dial failed: %v", err)
		resumeState.mu.Lock()
		resumeState.watcher = nil
		resumeState.mu.Unlock()
		return
	}
	defer client.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.done:
			return
		case <-ticker.C:
			st, err := client.Status()
			if err != nil {
				// Reconnect once on transient failure; if it persists, the
				// watcher exits and a later append will start a fresh one.
				client.Close()
				client, err = dialMPD(w.cfg)
				if err != nil {
					log.Printf("mpd resume watcher: reconnect failed: %v", err)
					resumeState.mu.Lock()
					resumeState.watcher = nil
					resumeState.mu.Unlock()
					return
				}
				continue
			}

			resumeState.mu.Lock()
			curID := parseSongid(st)
			// New item still playing → keep waiting.
			if curID == w.expected && st["state"] == "play" {
				resumeState.mu.Unlock()
				continue
			}
			// Transition detected: pop top and resume it.
			top, ok := popResumeLocked()
			if !ok {
				// Nothing left to resume; watcher exits.
				resumeState.watcher = nil
				resumeState.mu.Unlock()
				return
			}
			w.expected = top.songid
			resumeState.mu.Unlock()

			if err := client.PlayID(top.songid); err != nil {
				log.Printf("mpd resume watcher: playid failed on %v: %v", top.songid, err)
			}
			if err := client.SeekID(top.songid, int(top.elapsed)); err != nil {
				log.Printf("mpd resume watcher: seekid failed on %v: %v", top.songid, err)
			}
		}
	}
}

// EnqueueMPD appends one or more URIs to the MPD playlist without blocking.
// If autoPlay is true, it starts playing the first added item. If MPD was
// playing at append time, the prior item is remembered and resumed after the
// new item finishes.
func EnqueueMPD(ctx context.Context, cfg config.MPDConfig, downloadDir string, uris []string, autoPlay bool) error {
	if len(uris) == 0 {
		return nil
	}

	client, err := dialMPD(cfg)
	if err != nil {
		return fmt.Errorf("mpd dial (%s:%s): %w", cfg.Network, cfg.Address, err)
	}
	defer client.Close()

	initResume(cfg)

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
		// Capture MPD state before issuing PlayID. If something was playing,
		// remember it so the watcher can resume it after the new item finishes.
		if st, err := client.Status(); err == nil {
			if st["state"] == "play" {
				curID := parseSongid(st)
				if curID >= 0 {
					el := parseElapsed(st)
					resumeState.mu.Lock()
					pushRememberedLocked(curID, el)
					startWatcherIfNeededLocked(firstID)
					resumeState.mu.Unlock()
				}
			}
		} else {
			log.Printf("mpd enqueue: status query failed (resume skipped): %v", err)
		}

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
// It blocks until the added item is no longer the current song. If MPD was
// playing at append time, the prior item is remembered and resumed after the
// new item finishes (the first hop inline; subsequent hops by the watcher).
func PlayWithMPD(ctx context.Context, cfg config.MPDConfig, downloadDir string, uri string) error {
	client, err := dialMPD(cfg)
	if err != nil {
		return fmt.Errorf("mpd dial (%s:%s): %w", cfg.Network, cfg.Address, err)
	}
	defer client.Close()

	initResume(cfg)

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

	// Remember the currently-playing item (if any) before issuing PlayID.
	if st, err := client.Status(); err == nil {
		if st["state"] == "play" {
			curID := parseSongid(st)
			if curID >= 0 {
				el := parseElapsed(st)
				resumeState.mu.Lock()
				pushRememberedLocked(curID, el)
				startWatcherIfNeededLocked(id)
				resumeState.mu.Unlock()
			}
		}
	} else {
		log.Printf("mpd play: status query failed (resume skipped): %v", err)
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
				// Resume of the prior item is handled by the background watcher
				// (started above) which survives after this function returns.
				return nil
			}
		}
	}
}