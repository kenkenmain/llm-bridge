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

// NewDefaults returns the default values for the Defaults struct.
func NewDefaults() Defaults {
	return Defaults{
		LLM:             "claude",
		OutputThreshold: 1500,
		IdleTimeout:     "10m",
	}
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

// GetClaudePath returns the path to the Claude CLI binary.
// Defaults to "claude" if not explicitly set.
func (d Defaults) GetClaudePath() string {
	if d.ClaudePath == "" {
		return "claude"
	}
	return d.ClaudePath
}

// GetResumeSession returns whether LLM sessions should resume on reconnect.
// Defaults to true if not explicitly set.
func (d Defaults) GetResumeSession() bool {
	if d.ResumeSession == nil {
		return true
	}
	return *d.ResumeSession
}

// GetIdleTimeoutDuration returns the idle timeout as a time.Duration.
// Falls back to 10 minutes if the stored value is unparseable (defensive;
// loadRaw validates this at load time).
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
	BotToken      string `yaml:"bot_token"`
	ApplicationID string `yaml:"application_id"`
	PublicKey      string `yaml:"public_key"`
	TestChannelID string `yaml:"test_channel_id"`
}

const (
	DefaultDiscordApplicationID = "1468294340190408764"
	DefaultDiscordPublicKey     = "514e7d7f6bcc0907e4207ede9b77d6c789609df45727be9437e2bade64fb8147"
	DefaultDiscordTestChannelID = "1468297189879975998"
)

func (d DiscordConfig) GetBotToken() string {
	return d.BotToken
}

func (d DiscordConfig) GetApplicationID() string {
	if d.ApplicationID == "" {
		return DefaultDiscordApplicationID
	}
	return d.ApplicationID
}

func (d DiscordConfig) GetPublicKey() string {
	if d.PublicKey == "" {
		return DefaultDiscordPublicKey
	}
	return d.PublicKey
}

func (d DiscordConfig) GetTestChannelID() string {
	if d.TestChannelID == "" {
		return DefaultDiscordTestChannelID
	}
	return d.TestChannelID
}

// Validate checks the config for structural integrity and channel_id uniqueness.
// It covers both top-level repo channel_ids and worktree-level channel_ids.
// Load() calls this before worktree expansion; worktree name conflicts with
// existing repos are caught separately during expansion. AddRepo() also calls
// it on the raw form to validate before writing.
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

	// Check for duplicate channel IDs across all repo entries and worktree
	// definitions. This catches collisions both after worktree expansion
	// (Load path) and on raw configs (AddRepo path).
	channelIDs := make(map[string]string) // channel_id -> source label
	for name, repo := range c.Repos {
		if repo.ChannelID != "" {
			if existing, ok := channelIDs[repo.ChannelID]; ok {
				// Sort names for deterministic error messages.
				a, b := existing, name
				if a > b {
					a, b = b, a
				}
				return fmt.Errorf("duplicate channel_id %q in repos %q and %q", repo.ChannelID, a, b)
			}
			channelIDs[repo.ChannelID] = name
		}
		for _, wt := range repo.Worktrees {
			if wt.ChannelID != "" {
				wtLabel := name + "/" + wt.Name
				if existing, ok := channelIDs[wt.ChannelID]; ok {
					a, b := existing, wtLabel
					if a > b {
						a, b = b, a
					}
					return fmt.Errorf("duplicate channel_id %q in repos %q and %q", wt.ChannelID, a, b)
				}
				channelIDs[wt.ChannelID] = wtLabel
			}
		}
	}
	return nil
}

// loadRaw reads the config file, applies environment variable expansion and
// default values, but does NOT expand worktrees. Use this for operations
// that need to write the config back to disk (like AddRepo).
func loadRaw(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Apply NewDefaults() values for any zero-value fields.
	defaults := NewDefaults()
	if cfg.Defaults.LLM == "" {
		cfg.Defaults.LLM = defaults.LLM
	}
	if cfg.Defaults.OutputThreshold == 0 {
		cfg.Defaults.OutputThreshold = defaults.OutputThreshold
	}
	if cfg.Defaults.IdleTimeout == "" {
		cfg.Defaults.IdleTimeout = defaults.IdleTimeout
	}

	// Validate output_threshold is non-negative.
	if cfg.Defaults.OutputThreshold < 0 {
		return nil, fmt.Errorf("invalid output_threshold %d: must be non-negative", cfg.Defaults.OutputThreshold)
	}

	// Validate idle_timeout is a parseable duration.
	if _, err := time.ParseDuration(cfg.Defaults.IdleTimeout); err != nil {
		return nil, fmt.Errorf("invalid idle_timeout %q: %w", cfg.Defaults.IdleTimeout, err)
	}

	// Validate rate limit values are non-negative.
	rl := cfg.Defaults.RateLimit
	if rl.UserRate < 0 {
		return nil, fmt.Errorf("invalid user_rate %v: must be non-negative", rl.UserRate)
	}
	if rl.UserBurst < 0 {
		return nil, fmt.Errorf("invalid user_burst %d: must be non-negative", rl.UserBurst)
	}
	if rl.ChannelRate < 0 {
		return nil, fmt.Errorf("invalid channel_rate %v: must be non-negative", rl.ChannelRate)
	}
	if rl.ChannelBurst < 0 {
		return nil, fmt.Errorf("invalid channel_burst %d: must be non-negative", rl.ChannelBurst)
	}

	return &cfg, nil
}

// Load reads the config from path, applies defaults, validates, and
// expands worktrees into top-level repo entries.
func Load(path string) (*Config, error) {
	cfg, err := loadRaw(path)
	if err != nil {
		return nil, err
	}

	// Validate the raw config before expanding worktrees. This catches field
	// integrity issues and duplicate channel_ids (including worktree-level
	// channel_ids). After expansion, worktree channel_ids become top-level
	// entries and would double-count if Validate ran again.
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
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

	return cfg, nil
}

// DefaultPath returns the default config file path relative to the current directory.
func DefaultPath() string {
	return filepath.Join(".", "llm-bridge.yaml")
}
