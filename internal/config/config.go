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
	LLM             string          `yaml:"llm"`
	ClaudePath      string          `yaml:"claude_path"`
	OutputThreshold int             `yaml:"output_threshold"`
	IdleTimeout     string          `yaml:"idle_timeout"`
	ResumeSession   *bool           `yaml:"resume_session"`
	RateLimit       RateLimitConfig `yaml:"rate_limit"`
}

// RateLimitConfig holds per-user and per-channel rate limit settings.
type RateLimitConfig struct {
	UserRate     float64 `yaml:"user_rate"`     // messages per second per user (default: 0.5 = 1 msg every 2s)
	UserBurst    int     `yaml:"user_burst"`    // burst capacity per user (default: 3)
	ChannelRate  float64 `yaml:"channel_rate"`  // messages per second per channel (default: 2.0)
	ChannelBurst int     `yaml:"channel_burst"` // burst capacity per channel (default: 10)
	Enabled      *bool   `yaml:"enabled"`       // enable/disable rate limiting (default: true)
}

// GetRateLimitEnabled returns whether rate limiting is enabled.
// Defaults to true if not explicitly set.
func (r RateLimitConfig) GetRateLimitEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// GetUserRate returns the user rate limit in messages per second.
// Defaults to 0.5 (1 message every 2 seconds).
func (r RateLimitConfig) GetUserRate() float64 {
	if r.UserRate == 0 {
		return 0.5
	}
	return r.UserRate
}

// GetUserBurst returns the user burst capacity.
// Defaults to 3.
func (r RateLimitConfig) GetUserBurst() int {
	if r.UserBurst == 0 {
		return 3
	}
	return r.UserBurst
}

// GetChannelRate returns the channel rate limit in messages per second.
// Defaults to 2.0.
func (r RateLimitConfig) GetChannelRate() float64 {
	if r.ChannelRate == 0 {
		return 2.0
	}
	return r.ChannelRate
}

// GetChannelBurst returns the channel burst capacity.
// Defaults to 10.
func (r RateLimitConfig) GetChannelBurst() int {
	if r.ChannelBurst == 0 {
		return 10
	}
	return r.ChannelBurst
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
