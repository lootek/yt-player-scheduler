package webui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

type jobStatus string

const (
	statusQueued  jobStatus = "queued"
	statusRunning jobStatus = "running"
	statusDone    jobStatus = "done"
	statusFailed  jobStatus = "failed"
)

// Job represents a single on-demand download request.
type Job struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	Video      bool      `json:"video"`
	MPD        bool      `json:"mpd"`
	AutoPlay   bool      `json:"auto_play"`
	Status     string    `json:"status"`
	Files      []string  `json:"files,omitempty"`
	Error      string    `json:"error,omitempty"`
	Log        *bytes.Buffer `json:"-"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

func newJob(url string, video, mpd, autoPlay bool) *Job {
	return &Job{
		ID:        newID(),
		URL:       url,
		Video:     video,
		MPD:       mpd,
		AutoPlay:  autoPlay,
		Status:    string(statusQueued),
		Log:       bytes.NewBuffer(nil),
		CreatedAt: time.Now().UTC(),
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails.
		return fmt.Sprintf("%x", time.Now().UnixNano())[:16]
	}
	return hex.EncodeToString(b)
}
