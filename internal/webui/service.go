package webui

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lootek/yt-rpi-player/internal/config"
	"github.com/lootek/yt-rpi-player/internal/player"
	"github.com/lootek/yt-rpi-player/internal/ytdlp"
)

const defaultHistoryLimit = 200

// Service manages the web UI download queue and in-memory job state.
type Service struct {
	cfg     config.Config
	ytdlp   ytdlp.Client
	logger  *log.Logger
	history *History
	mu      sync.RWMutex
	jobs    map[string]*Job
	order   []string
	queue   chan *Job
	timeout time.Duration
	// urlLocks serialize downloads of the same URL so concurrent jobs do not
	// race on the same output files and archive entries.
	urlLocks   map[string]*sync.Mutex
	urlLocksMu sync.Mutex
}

// NewService creates a new web UI service.
func NewService(cfg config.Config, logger *log.Logger, history *History) *Service {
	timeout, _ := time.ParseDuration(cfg.Global.WebUI.Timeout)
	if timeout <= 0 {
		timeout = 2 * time.Hour
	}
	return NewServiceWithTimeout(cfg, logger, history, timeout)
}

// NewServiceWithTimeout creates a service with an explicit per-job timeout.
func NewServiceWithTimeout(cfg config.Config, logger *log.Logger, history *History, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 2 * time.Hour
	}
	return &Service{
		cfg:      cfg,
		ytdlp:    ytdlp.New(cfg.Global.YtDLP, logger),
		logger:   logger,
		history:  history,
		jobs:     make(map[string]*Job),
		queue:    make(chan *Job, cfg.Global.WebUI.MaxConcurrent*4),
		timeout:  timeout,
		urlLocks: make(map[string]*sync.Mutex),
	}
}

// SetTimeout updates the per-job timeout after construction.
func (s *Service) SetTimeout(timeout time.Duration) {
	if timeout > 0 {
		s.timeout = timeout
	}
}

// Start launches the worker pool and blocks until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < s.cfg.Global.WebUI.MaxConcurrent; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx)
		}()
	}

	<-ctx.Done()
	close(s.queue)
	wg.Wait()
}

// Enqueue creates a new job and pushes it onto the queue.
func (s *Service) Enqueue(url string, video, mpd, autoPlay bool) (string, error) {
	job := newJob(url, video, mpd, autoPlay)

	s.mu.Lock()
	s.jobs[job.ID] = job
	s.order = append(s.order, job.ID)
	s.mu.Unlock()

	if err := s.history.Append(HistoryEvent{
		ID:       job.ID,
		Kind:     "enqueue",
		URL:      job.URL,
		Video:    job.Video,
		MPD:      job.MPD,
		AutoPlay: job.AutoPlay,
	}); err != nil {
		s.logger.Printf("webui: failed to append history enqueue event: %v", err)
	}

	select {
	case s.queue <- job:
		return job.ID, nil
	default:
		return "", fmt.Errorf("queue is full")
	}
}

// Get returns a job by ID.
func (s *Service) Get(id string) (*Job, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return s.clone(job), true
}

// List returns recent jobs in deterministic order.
func (s *Service) List(limit int) []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}

	jobs := make([]*Job, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(jobs) < limit; i-- {
		if job, ok := s.jobs[s.order[i]]; ok {
			jobs = append(jobs, s.clone(job))
		}
	}
	return jobs
}

func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-s.queue:
			if !ok {
				return
			}
			s.runJob(ctx, job)
		}
	}
}

func (s *Service) runJob(parentCtx context.Context, job *Job) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Printf("webui worker panic recovered for job %s: %v", job.ID, r)
			s.setFailed(job, "internal panic")
		}
	}()

	jobCtx, cancel := context.WithTimeout(parentCtx, s.timeout)
	defer cancel()

	s.setStatus(job, string(statusRunning))

	lock := s.lockForURL(job.URL)
	lock.Lock()
	files, err := s.download(jobCtx, job)
	lock.Unlock()
	if err != nil {
		s.setFailed(job, err.Error())
		return
	}

	job.Files = files

	if job.MPD && s.cfg.Global.MPD.Enabled && len(files) > 0 {
		if err := player.EnqueueMPD(jobCtx, s.cfg.Global.MPD, s.cfg.Global.WebUI.DownloadDir, files, job.AutoPlay); err != nil {
			s.logger.Printf("webui job %s: enqueue to MPD failed: %v", job.ID, err)
			job.Log.WriteString(fmt.Sprintf("\nMPD enqueue error: %v\n", err))
		}
	}

	s.setDone(job)
}

func (s *Service) lockForURL(url string) *sync.Mutex {
	s.urlLocksMu.Lock()
	defer s.urlLocksMu.Unlock()
	if s.urlLocks[url] == nil {
		s.urlLocks[url] = &sync.Mutex{}
	}
	return s.urlLocks[url]
}

func (s *Service) download(ctx context.Context, job *Job) ([]string, error) {
	archivePath := ""
	if s.cfg.Global.WebUI.DownloadDir != "" {
		archivePath = s.cfg.Global.WebUI.DownloadDir + "/archive.txt"
	}

	req := ytdlp.DownloadMediaRequest{
		URL:         job.URL,
		DownloadDir: s.cfg.Global.WebUI.DownloadDir,
		Subdir:      s.cfg.Global.WebUI.Subdir,
		ArchivePath: archivePath,
		Video:       job.Video,
		LogWriter:   job.Log,
	}

	return s.ytdlp.DownloadMedia(ctx, req)
}

func (s *Service) setStatus(job *Job, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status = status
	switch status {
	case string(statusRunning):
		job.StartedAt = time.Now().UTC()
		_ = s.history.Append(HistoryEvent{
			ID:       job.ID,
			Kind:     "start",
			URL:      job.URL,
			Video:    job.Video,
			MPD:      job.MPD,
			AutoPlay: job.AutoPlay,
		})
	}
}

func (s *Service) setDone(job *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status = string(statusDone)
	job.FinishedAt = time.Now().UTC()
	_ = s.history.Append(HistoryEvent{
		ID:        job.ID,
		Kind:      "done",
		URL:       job.URL,
		Video:     job.Video,
		MPD:       job.MPD,
		AutoPlay:  job.AutoPlay,
		Files:     job.Files,
		Timestamp: job.FinishedAt,
	})
}

func (s *Service) setFailed(job *Job, errStr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job.Status = string(statusFailed)
	job.Error = errStr
	job.FinishedAt = time.Now().UTC()
	_ = s.history.Append(HistoryEvent{
		ID:        job.ID,
		Kind:      "failed",
		URL:       job.URL,
		Video:     job.Video,
		MPD:       job.MPD,
		AutoPlay:  job.AutoPlay,
		Error:     errStr,
		Timestamp: job.FinishedAt,
	})
}

func (s *Service) clone(job *Job) *Job {
	cpy := *job
	cpy.Log = bytes.NewBuffer(job.Log.Bytes())
	cpy.Files = append([]string(nil), job.Files...)
	return &cpy
}
