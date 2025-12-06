package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/lootek/yt-rpi-player/internal/app"
	"github.com/lootek/yt-rpi-player/internal/config"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to YAML configuration")
	runImmediately := flag.Bool("run-now", false, "run all jobs once on startup before scheduling")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
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

	c.Start()
	logger.Printf("scheduler started; waiting for jobs (press Ctrl+C to exit)")
	<-ctx.Done()
	stop()
	logger.Printf("shutting down")
	c.Stop()
}
