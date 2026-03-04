package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/lootek/yt-rpi-player/internal/config"
	"github.com/lootek/yt-rpi-player/internal/player"
	"github.com/lootek/yt-rpi-player/internal/query"
	"github.com/lootek/yt-rpi-player/internal/ytdlp"
)

type App struct {
	cfg    config.Config
	logger *log.Logger
	ytdlp  ytdlp.Client
}

func New(cfg config.Config, logger *log.Logger) *App {
	return &App{
		cfg:    cfg,
		logger: logger,
		ytdlp:  ytdlp.New(cfg.Global.YtDLP),
	}
}

func (a *App) RunJob(ctx context.Context, job config.JobConfig) error {
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

	if a.cfg.Global.MPD.Enabled {
		// MPD can handle stream URL or direct YouTube URL if it has plugins (like mpd-youtube)
		// but let's assume we want to resolve it first or just pass the videoURL.
		// Usually MPD with plugins is better, but here we already have yt-dlp.
		streamURL, err := a.ytdlp.ResolveStream(ctx, videoURL)
		if err != nil {
			return fmt.Errorf("resolve stream for MPD: %w", err)
		}

		if err := player.PlayWithMPD(ctx, a.cfg.Global.MPD, streamURL); err != nil {
			return fmt.Errorf("play audio via MPD: %w", err)
		}
	} else if a.cfg.Global.Player.Command == "mpv" {
		// Optimization: if command is mpv, use its internal yt-dlp integration
		if err := player.PlayWithMPV(ctx, a.cfg.Global.Player, videoURL); err != nil {
			return fmt.Errorf("play audio via mpv: %w", err)
		}
	} else {
		streamURL, err := a.ytdlp.ResolveStream(ctx, videoURL)
		if err != nil {
			return fmt.Errorf("resolve stream: %w", err)
		}

		if err := player.Play(ctx, a.cfg.Global.Player, streamURL); err != nil {
			return fmt.Errorf("play audio: %w", err)
		}
	}
	a.logger.Printf("[job:%s] playback finished", jobName)
	return nil
}

func (a *App) CheckAuth(ctx context.Context) (string, error) {
	return a.ytdlp.CheckAuth(ctx)
}
