package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/lootek/yt-rpi-player/internal/config"
	"github.com/lootek/yt-rpi-player/internal/player"
	"github.com/lootek/yt-rpi-player/internal/query"
)

// RunJobWithMPV runs a job using mpv's built-in yt-dlp support.
// This bypasses manual stream extraction and lets mpv handle YouTube URLs directly.
func (a *App) RunJobWithMPV(ctx context.Context, job config.JobConfig) error {
	jobName := job.Name
	if jobName == "" {
		jobName = strings.Join(job.Keywords, " ")
	}

	queryString := query.Build(job.Keywords, job.DateFormat, job.DateLocale)
	limit := job.SearchLimit
	if limit <= 0 {
		limit = a.cfg.Global.SearchLimit
	}
	a.logger.Printf("[job:%s] searching YouTube for %q (limit %d)", jobName, queryString, limit)

	results, err := a.ytdlp.Search(ctx, queryString, limit)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("no results for query %q", queryString)
	}

	videoURL := results[0].VideoURL()
	a.logger.Printf("[job:%s] playing %q (%s)", jobName, results[0].Title, videoURL)

	// Use mpv directly with YouTube URL - no stream extraction needed
	if err := player.PlayWithMPV(ctx, a.cfg.Global.Player, videoURL); err != nil {
		return fmt.Errorf("play audio: %w", err)
	}
	a.logger.Printf("[job:%s] playback finished", jobName)
	return nil
}
