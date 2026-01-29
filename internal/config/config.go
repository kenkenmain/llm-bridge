package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Repos     map[string]RepoConfig `yaml:"repos"`
	Defaults  Defaults              `yaml:"defaults"`
	Providers ProviderConfigs       `yaml:"providers"`
}

type RepoConfig struct {
	Provider   string `yaml:"provider"`
	ChannelID  string `yaml:"channel_id"`
	LLM        string `yaml:"llm"`
	WorkingDir string `yaml:"working_dir"`
}

type Defaults struct {
	LLM             string `yaml:"llm"`
	ClaudePath      string `yaml:"claude_path"`
	OutputThreshold int    `yaml:"output_threshold"`
	IdleTimeout     string `yaml:"idle_timeout"`
	ResumeSession   *bool  `yaml:"resume_session"`
}

func (d Defaults) GetClaudePath() string {
	if d.ClaudePath == "" {
		return "claude"
	}
	return d.ClaudePath
}

func (d Defaults) GetResumeSession() bool {
	if d.ResumeSession == nil {
		return true
	}
	return *d.ResumeSession
}

func (d Defaults) GetIdleTimeoutDuration() time.Duration {
	dur, err := time.ParseDuration(d.IdleTimeout)
	if err != nil {
		return 10 * time.Minute
	}
	return dur
}

type ProviderConfigs struct {
	Discord DiscordConfig `yaml:"discord"`
}

type DiscordConfig struct {
	BotToken string `yaml:"bot_token"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Defaults.LLM == "" {
		cfg.Defaults.LLM = "claude"
	}
	if cfg.Defaults.OutputThreshold == 0 {
		cfg.Defaults.OutputThreshold = 1500
	}
	if cfg.Defaults.IdleTimeout == "" {
		cfg.Defaults.IdleTimeout = "10m"
	}

	return &cfg, nil
}

func DefaultPath() string {
	return filepath.Join(".", "llm-bridge.yaml")
}
