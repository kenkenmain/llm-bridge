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
	Provider   string           `yaml:"provider"`
	ChannelID  string           `yaml:"channel_id"`
	LLM        string           `yaml:"llm"`
	WorkingDir string           `yaml:"working_dir"`
	Worktrees  []WorktreeConfig `yaml:"worktrees,omitempty"`
	GitRoot    string           `yaml:"git_root,omitempty"`
	Branch     string           `yaml:"branch,omitempty"`
}

type WorktreeConfig struct {
	Name      string `yaml:"name"`
	Path      string `yaml:"path"`
	ChannelID string `yaml:"channel_id"`
	Branch    string `yaml:"branch,omitempty"`
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

// Validate checks the config for consistency errors.
// It should be called after worktree expansion, so duplicate channel ID
// detection operates on the final set of repo entries.
func (c *Config) Validate() error {
	// Check worktree field integrity on repos that still carry worktree definitions.
	for name, repo := range c.Repos {
		for _, wt := range repo.Worktrees {
			if wt.Name == "" {
				return fmt.Errorf("worktree in repo %q has empty name", name)
			}
			if wt.Path == "" {
				return fmt.Errorf("worktree %q in repo %q has empty path", wt.Name, name)
			}
			if !filepath.IsAbs(wt.Path) {
				return fmt.Errorf("worktree %q in repo %q has non-absolute path %q", wt.Name, name, wt.Path)
			}
		}
	}

	// Check for duplicate channel IDs across all repo entries (including expanded worktrees).
	channelIDs := make(map[string]string) // channel_id -> repo name
	for name, repo := range c.Repos {
		if repo.ChannelID != "" {
			if existing, ok := channelIDs[repo.ChannelID]; ok {
				return fmt.Errorf("duplicate channel_id %q in repos %q and %q", repo.ChannelID, existing, name)
			}
			channelIDs[repo.ChannelID] = name
		}
	}
	return nil
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

	// Expand worktrees into separate repo entries.
	// Collect expansions in a separate map to avoid mutating cfg.Repos during iteration.
	expansions := make(map[string]RepoConfig)
	for name, repo := range cfg.Repos {
		if len(repo.Worktrees) == 0 {
			continue
		}
		for _, wt := range repo.Worktrees {
			childName := name + "/" + wt.Name
			if _, exists := cfg.Repos[childName]; exists {
				return nil, fmt.Errorf("worktree %q in repo %q conflicts with existing repo %q", wt.Name, name, childName)
			}
			if _, exists := expansions[childName]; exists {
				return nil, fmt.Errorf("duplicate worktree name %q in repo %q", wt.Name, name)
			}
			expansions[childName] = RepoConfig{
				Provider:   repo.Provider,
				ChannelID:  wt.ChannelID,
				LLM:        repo.LLM,
				WorkingDir: wt.Path,
				GitRoot:    repo.WorkingDir,
				Branch:     wt.Branch,
			}
		}
	}
	for name, repo := range expansions {
		cfg.Repos[name] = repo
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func DefaultPath() string {
	return filepath.Join(".", "llm-bridge.yaml")
}
