package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultDateFormat  = "2006-01-02"
	DefaultSearchLimit = 5
	DefaultTimeout     = "20m"
)

type Config struct {
	Global GlobalConfig `yaml:"global"`
	Jobs   []JobConfig  `yaml:"jobs"`
}

type GlobalConfig struct {
	SearchLimit int          `yaml:"search_limit"`
	Player      PlayerConfig `yaml:"player"`
	MPD         MPDConfig    `yaml:"mpd"`
	YtDLP       YtDLPConfig  `yaml:"ytdlp"`
	WebUI       WebUIConfig  `yaml:"web_ui"`
}

type PlayerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
	Timeout string            `yaml:"timeout"`
}

type MPDConfig struct {
	Enabled   bool   `yaml:"enabled"`
	Network   string `yaml:"network"`
	Address   string `yaml:"address"`
	Password  string `yaml:"password"`
	MusicRoot string `yaml:"music_root"`
}

type YtDLPConfig struct {
	Binary              string   `yaml:"binary"`
	Cookies             string   `yaml:"cookies"`
	DownloadDir         string   `yaml:"download_dir"`
	ExtraArgs           []string `yaml:"extra_args"`
	POToken             string   `yaml:"po_token"`
	POTokenProvider     string   `yaml:"po_token_provider"`
	POTokenProviderArgs []string `yaml:"po_token_provider_args"`
	UserAgent           string   `yaml:"user_agent"`
}

type WebUIConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Listen        string `yaml:"listen"`
	Username      string `yaml:"username"`
	Password      string `yaml:"password"`
	DownloadDir   string `yaml:"download_dir"`
	Subdir        string `yaml:"subdir"`
	MaxConcurrent int    `yaml:"max_concurrent"`
	Timeout       string `yaml:"timeout"`
	HistoryPath   string `yaml:"history_path"`
}

type JobConfig struct {
	Name        string   `yaml:"name"`
	Cron        string   `yaml:"cron"`
	Keywords    []string `yaml:"keywords"`
	DateFormat  string   `yaml:"date_format"`
	DateLocale  string   `yaml:"date_locale"`
	SearchLimit int      `yaml:"search_limit"`
}

func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse yaml: %w", err)
	}
	applyDefaults(&cfg)
	if len(cfg.Jobs) == 0 && !cfg.Global.WebUI.Enabled {
		return cfg, errors.New("no jobs configured and web UI is disabled")
	}
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Global.SearchLimit <= 0 {
		cfg.Global.SearchLimit = DefaultSearchLimit
	}
	if cfg.Global.Player.Command == "" {
		cfg.Global.Player.Command = "ffplay"
	}
	if cfg.Global.Player.Timeout == "" {
		cfg.Global.Player.Timeout = DefaultTimeout
	}
	if len(cfg.Global.Player.Args) == 0 {
		cfg.Global.Player.Args = []string{"-nodisp", "-autoexit", "-loglevel", "warning", "{url}"}
	}
	if cfg.Global.YtDLP.Binary == "" {
		cfg.Global.YtDLP.Binary = "yt-dlp"
	}
	if cfg.Global.MPD.Network == "" {
		cfg.Global.MPD.Network = "tcp"
	}
	if cfg.Global.MPD.Address == "" {
		cfg.Global.MPD.Address = "localhost:6600"
	}
	if cfg.Global.WebUI.Listen == "" {
		cfg.Global.WebUI.Listen = ":8080"
	}
	if cfg.Global.WebUI.MaxConcurrent <= 0 {
		cfg.Global.WebUI.MaxConcurrent = 2
	}
	if cfg.Global.WebUI.Timeout == "" {
		cfg.Global.WebUI.Timeout = "2h"
	}
	if cfg.Global.WebUI.DownloadDir == "" {
		cfg.Global.WebUI.DownloadDir = cfg.Global.YtDLP.DownloadDir
	}
	for i := range cfg.Jobs {
		if cfg.Jobs[i].DateFormat == "" {
			cfg.Jobs[i].DateFormat = DefaultDateFormat
		}
		if cfg.Jobs[i].SearchLimit <= 0 {
			cfg.Jobs[i].SearchLimit = cfg.Global.SearchLimit
		}
		if cfg.Jobs[i].Name == "" {
			cfg.Jobs[i].Name = strings.Join(cfg.Jobs[i].Keywords, " ")
		}
	}
}
