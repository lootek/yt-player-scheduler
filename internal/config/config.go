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
	YtDLP       YtDLPConfig  `yaml:"ytdlp"`
}

type PlayerConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
	Timeout string            `yaml:"timeout"`
}

type YtDLPConfig struct {
	Binary              string   `yaml:"binary"`
	Cookies             string   `yaml:"cookies"`
	ExtraArgs           []string `yaml:"extra_args"`
	POToken             string   `yaml:"po_token"`
	POTokenProvider     string   `yaml:"po_token_provider"`
	POTokenProviderArgs []string `yaml:"po_token_provider_args"`
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
	if len(cfg.Jobs) == 0 {
		return cfg, errors.New("no jobs configured")
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
