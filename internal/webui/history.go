package webui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// HistoryEvent is a single line in the append-only history log.
type HistoryEvent struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	URL       string    `json:"url"`
	Video     bool      `json:"video"`
	MPD       bool      `json:"mpd"`
	AutoPlay  bool      `json:"auto_play"`
	Status    string    `json:"status,omitempty"`
	Error     string    `json:"error,omitempty"`
	Files     []string  `json:"files,omitempty"`
	Timestamp time.Time `json:"ts"`
}

// History is an append-only JSON-lines store for web UI jobs.
type History struct {
	path string
	mu   sync.Mutex
}

// OpenHistory opens or creates a history log at the given directory.
// The file is named history.jsonl and lives inside dir.
func OpenHistory(dir string) *History {
	return &History{path: filepath.Join(dir, "history.jsonl")}
}

// Append writes a single event atomically to the history file.
func (h *History) Append(e HistoryEvent) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal history event: %w", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write history event: %w", err)
	}
	return nil
}

// List reads the history file and returns the latest snapshot per job ID,
// sorted by CreatedAt descending, limited to the requested count.
func (h *History) List(limit int) ([]*Job, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.Open(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	snapshots := make(map[string]*Job)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e HistoryEvent
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		job, ok := snapshots[e.ID]
		if !ok {
			job = &Job{
				ID:        e.ID,
				URL:       e.URL,
				Video:     e.Video,
				MPD:       e.MPD,
				AutoPlay:  e.AutoPlay,
				CreatedAt: e.Timestamp,
			}
			snapshots[e.ID] = job
		}
		switch e.Kind {
		case "enqueue":
			job.Status = string(statusQueued)
		case "start":
			job.Status = string(statusRunning)
			job.StartedAt = e.Timestamp
		case "done":
			job.Status = string(statusDone)
			job.Files = e.Files
			job.FinishedAt = e.Timestamp
		case "failed":
			job.Status = string(statusFailed)
			job.Error = e.Error
			job.FinishedAt = e.Timestamp
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan history file: %w", err)
	}

	var jobs []*Job
	for _, job := range snapshots {
		jobs = append(jobs, job)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.After(jobs[j].CreatedAt)
	})

	if limit > 0 && len(jobs) > limit {
		jobs = jobs[:limit]
	}
	return jobs, nil
}
