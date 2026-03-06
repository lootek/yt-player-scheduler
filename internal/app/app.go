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

	var playURL string
	if a.cfg.Global.YtDLP.DownloadDir != "" {
		a.logger.Printf("[job:%s] downloading to %s", jobName, a.cfg.Global.YtDLP.DownloadDir)
		localPath, err := a.ytdlp.Download(ctx, videoURL)
		if err != nil {
			return fmt.Errorf("download for job: %w", err)
		}
		playURL = localPath
	} else {
		streamURL, err := a.ytdlp.ResolveStream(ctx, videoURL)
		if err != nil {
			return fmt.Errorf("resolve stream: %w", err)
		}
		playURL = streamURL
	}

	if a.cfg.Global.MPD.Enabled {
		if err := player.PlayWithMPD(ctx, a.cfg.Global.MPD, playURL); err != nil {
			return fmt.Errorf("play audio via MPD: %w", err)
		}
	} else if a.cfg.Global.Player.Command == "mpv" {
		// If DownloadDir is used, play the local file. Otherwise, use mpv internal yt-dlp integration.
		if a.cfg.Global.YtDLP.DownloadDir != "" {
			if err := player.Play(ctx, a.cfg.Global.Player, playURL); err != nil {
				return fmt.Errorf("play audio via mpv: %w", err)
			}
		} else {
			if err := player.PlayWithMPV(ctx, a.cfg.Global.Player, videoURL); err != nil {
				return fmt.Errorf("play audio via mpv: %w", err)
			}
		}
	} else {
		if err := player.Play(ctx, a.cfg.Global.Player, playURL); err != nil {
			return fmt.Errorf("play audio: %w", err)
		}
	}
	a.logger.Printf("[job:%s] playback finished", jobName)
	return nil
}

func (a *App) CheckAuth(ctx context.Context) (string, error) {
	return a.ytdlp.CheckAuth(ctx)
}
