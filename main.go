package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/lootek/yt-rpi-player/internal/app"
	"github.com/lootek/yt-rpi-player/internal/config"
	"github.com/lootek/yt-rpi-player/internal/webui"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	runImmediately := flag.Bool("run-now", false, "run all jobs once on startup before scheduling")
	webUI := flag.Bool("web-ui", false, "start the web UI server alongside the scheduler")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.Global.WebUI.Enabled = cfg.Global.WebUI.Enabled || *webUI

	if len(cfg.Jobs) == 0 && !cfg.Global.WebUI.Enabled {
		log.Fatalf("no jobs configured and web UI is disabled")
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	playerTimeout, err := time.ParseDuration(cfg.Global.Player.Timeout)
	if err != nil {
		log.Fatalf("invalid player timeout %q: %v", cfg.Global.Player.Timeout, err)
	}

	application := app.New(cfg, logger)

	if cfg.Global.YtDLP.Cookies != "" {
		authCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if title, err := application.CheckAuth(authCtx); err != nil {
			logger.Printf("yt-dlp auth check failed: %v", err)
		} else {
			logger.Printf("yt-dlp auth check passed; Watch Later reachable: %q", title)
		}
		cancel()
	}

	c := cron.New(
		cron.WithLocation(time.Local),
		cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)),
	)

	for _, job := range cfg.Jobs {
		job := job
		_, err := c.AddFunc(job.Cron, func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Printf("[job:%s] panic recovered: %v", job.Name, r)
				}
			}()
			jobCtx, cancel := context.WithTimeout(ctx, playerTimeout)
			defer cancel()
			if err := application.RunJob(jobCtx, job); err != nil {
				logger.Printf("[job:%s] error: %v", job.Name, err)
			}
		})
		if err != nil {
			log.Fatalf("schedule job %q: %v", job.Name, err)
		}
		logger.Printf("scheduled job %q with cron %q", job.Name, job.Cron)
	}

	if *runImmediately {
		logger.Printf("running all jobs once on startup")
		for _, job := range cfg.Jobs {
			jobCtx, cancel := context.WithTimeout(ctx, playerTimeout)
			if err := application.RunJob(jobCtx, job); err != nil {
				logger.Printf("[job:%s] error: %v", job.Name, err)
			}
			cancel()
		}
	}

	var srv *webui.Server
	if cfg.Global.WebUI.Enabled {
		webTimeout, err := time.ParseDuration(cfg.Global.WebUI.Timeout)
		if err != nil {
			log.Fatalf("invalid web_ui.timeout %q: %v", cfg.Global.WebUI.Timeout, err)
		}

		history := webui.OpenHistory(filepath.Dir(cfg.Global.WebUI.HistoryPath))
		svc := webui.NewService(cfg, logger, history)
		if webTimeout > 0 {
			svc.SetTimeout(webTimeout)
		}
		go svc.Start(ctx)

		srv = webui.NewServer(svc, logger)
		go func() {
			logger.Printf("web UI listening on %s", cfg.Global.WebUI.Listen)
			if err := srv.ListenAndServe(cfg.Global.WebUI.Listen); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Printf("web UI server error: %v", err)
			}
		}()
	}

	c.Start()
	logger.Printf("scheduler started; waiting for jobs (press Ctrl+C to exit)")
	<-ctx.Done()
	stop()
	logger.Printf("shutting down")

	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Printf("web UI shutdown error: %v", err)
		}
	}

	c.Stop()
}
