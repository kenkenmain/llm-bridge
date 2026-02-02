package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  test-repo:
    provider: discord
    channel_id: "123456"
    llm: claude
    working_dir: /tmp/test

defaults:
  llm: codex
  output_threshold: 2000
  idle_timeout: 5m
  resume_session: false

providers:
  discord:
    bot_token: test-token
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("len(Repos) = %d, want 1", len(cfg.Repos))
	}

	repo := cfg.Repos["test-repo"]
	if repo.Provider != "discord" {
		t.Errorf("repo.Provider = %q, want %q", repo.Provider, "discord")
	}
	if repo.ChannelID != "123456" {
		t.Errorf("repo.ChannelID = %q, want %q", repo.ChannelID, "123456")
	}

	if cfg.Defaults.LLM != "codex" {
		t.Errorf("Defaults.LLM = %q, want %q", cfg.Defaults.LLM, "codex")
	}
	if cfg.Defaults.OutputThreshold != 2000 {
		t.Errorf("Defaults.OutputThreshold = %d, want 2000", cfg.Defaults.OutputThreshold)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos: {}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Defaults.LLM != "claude" {
		t.Errorf("Defaults.LLM = %q, want %q (default)", cfg.Defaults.LLM, "claude")
	}
	if cfg.Defaults.OutputThreshold != 1500 {
		t.Errorf("Defaults.OutputThreshold = %d, want 1500 (default)", cfg.Defaults.OutputThreshold)
	}
	if cfg.Defaults.IdleTimeout != "10m" {
		t.Errorf("Defaults.IdleTimeout = %q, want %q (default)", cfg.Defaults.IdleTimeout, "10m")
	}
}

func TestLoad_EnvVarExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	t.Setenv("TEST_BOT_TOKEN", "secret-token-123")

	content := `
repos: {}
providers:
  discord:
    bot_token: ${TEST_BOT_TOKEN}
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Providers.Discord.BotToken != "secret-token-123" {
		t.Errorf("Discord.BotToken = %q, want %q", cfg.Providers.Discord.BotToken, "secret-token-123")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("Load() expected error for nonexistent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte("invalid: yaml: content:"), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("Load() expected error for invalid YAML")
	}
}

func TestDefaults_GetClaudePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"empty returns default", "", "claude"},
		{"custom path", "/usr/local/bin/claude", "/usr/local/bin/claude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Defaults{ClaudePath: tt.path}
			if got := d.GetClaudePath(); got != tt.want {
				t.Errorf("GetClaudePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDefaults_GetResumeSession(t *testing.T) {
	t.Run("nil returns true", func(t *testing.T) {
		d := Defaults{}
		if got := d.GetResumeSession(); got != true {
			t.Errorf("GetResumeSession() = %v, want true", got)
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		val := true
		d := Defaults{ResumeSession: &val}
		if got := d.GetResumeSession(); got != true {
			t.Errorf("GetResumeSession() = %v, want true", got)
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		val := false
		d := Defaults{ResumeSession: &val}
		if got := d.GetResumeSession(); got != false {
			t.Errorf("GetResumeSession() = %v, want false", got)
		}
	})
}

func TestDefaults_GetIdleTimeoutDuration(t *testing.T) {
	tests := []struct {
		name    string
		timeout string
		want    time.Duration
	}{
		{"valid duration", "5m", 5 * time.Minute},
		{"valid hours", "1h", 1 * time.Hour},
		{"invalid returns default", "invalid", 10 * time.Minute},
		{"empty returns default", "", 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Defaults{IdleTimeout: tt.timeout}
			if got := d.GetIdleTimeoutDuration(); got != tt.want {
				t.Errorf("GetIdleTimeoutDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultPath(t *testing.T) {
	got := DefaultPath()
	if got != "llm-bridge.yaml" {
		t.Errorf("DefaultPath() = %q, want %q", got, "llm-bridge.yaml")
	}
}

// Rate limit config tests

func TestRateLimitConfig_Defaults(t *testing.T) {
	// Zero-value config should return sensible defaults
	var rl RateLimitConfig

	if got := rl.GetRateLimitEnabled(); got != true {
		t.Errorf("GetRateLimitEnabled() = %v, want true", got)
	}
	if got := rl.GetUserRate(); got != 0.5 {
		t.Errorf("GetUserRate() = %v, want 0.5", got)
	}
	if got := rl.GetUserBurst(); got != 3 {
		t.Errorf("GetUserBurst() = %v, want 3", got)
	}
	if got := rl.GetChannelRate(); got != 2.0 {
		t.Errorf("GetChannelRate() = %v, want 2.0", got)
	}
	if got := rl.GetChannelBurst(); got != 10 {
		t.Errorf("GetChannelBurst() = %v, want 10", got)
	}
}

func TestRateLimitConfig_CustomValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos: {}
defaults:
  rate_limit:
    enabled: true
    user_rate: 1.0
    user_burst: 5
    channel_rate: 5.0
    channel_burst: 20
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	rl := cfg.Defaults.RateLimit
	if got := rl.GetUserRate(); got != 1.0 {
		t.Errorf("GetUserRate() = %v, want 1.0", got)
	}
	if got := rl.GetUserBurst(); got != 5 {
		t.Errorf("GetUserBurst() = %v, want 5", got)
	}
	if got := rl.GetChannelRate(); got != 5.0 {
		t.Errorf("GetChannelRate() = %v, want 5.0", got)
	}
	if got := rl.GetChannelBurst(); got != 20 {
		t.Errorf("GetChannelBurst() = %v, want 20", got)
	}
}

func TestRateLimitConfig_EnabledDefault(t *testing.T) {
	// nil Enabled should default to true
	rl := RateLimitConfig{}
	if got := rl.GetRateLimitEnabled(); got != true {
		t.Errorf("nil Enabled should default to true, got %v", got)
	}

	// Explicit true
	valTrue := true
	rl = RateLimitConfig{Enabled: &valTrue}
	if got := rl.GetRateLimitEnabled(); got != true {
		t.Errorf("explicit true should return true, got %v", got)
	}

	// Explicit false
	valFalse := false
	rl = RateLimitConfig{Enabled: &valFalse}
	if got := rl.GetRateLimitEnabled(); got != false {
		t.Errorf("explicit false should return false, got %v", got)
	}
}

func TestRateLimitConfig_DisabledViaYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos: {}
defaults:
  rate_limit:
    enabled: false
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Defaults.RateLimit.GetRateLimitEnabled() {
		t.Error("rate limiting should be disabled when enabled: false in YAML")
	}
}

func TestLoad_WithWorktrees(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: feature-auth
        path: /code/myproject-auth
        channel_id: "222222"
      - name: bugfix-crash
        path: /code/myproject-crash
        channel_id: "333333"
        branch: bugfix/crash
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Repos) != 3 {
		t.Fatalf("len(Repos) = %d, want 3", len(cfg.Repos))
	}

	auth, ok := cfg.Repos["myproject/feature-auth"]
	if !ok {
		t.Fatal("missing repo entry myproject/feature-auth")
	}
	if auth.ChannelID != "222222" {
		t.Errorf("feature-auth ChannelID = %q, want %q", auth.ChannelID, "222222")
	}
	if auth.WorkingDir != "/code/myproject-auth" {
		t.Errorf("feature-auth WorkingDir = %q, want %q", auth.WorkingDir, "/code/myproject-auth")
	}
	if auth.GitRoot != "/code/myproject" {
		t.Errorf("feature-auth GitRoot = %q, want %q", auth.GitRoot, "/code/myproject")
	}

	crash, ok := cfg.Repos["myproject/bugfix-crash"]
	if !ok {
		t.Fatal("missing repo entry myproject/bugfix-crash")
	}
	if crash.ChannelID != "333333" {
		t.Errorf("bugfix-crash ChannelID = %q, want %q", crash.ChannelID, "333333")
	}
	if crash.Branch != "bugfix/crash" {
		t.Errorf("bugfix-crash Branch = %q, want %q", crash.Branch, "bugfix/crash")
	}
}

func TestLoad_WorktreeExpansion_Inherits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: child
        path: /code/myproject-child
        channel_id: "222222"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	child, ok := cfg.Repos["myproject/child"]
	if !ok {
		t.Fatal("missing repo entry myproject/child")
	}
	if child.Provider != "discord" {
		t.Errorf("child Provider = %q, want %q (inherited from parent)", child.Provider, "discord")
	}
	if child.LLM != "claude" {
		t.Errorf("child LLM = %q, want %q (inherited from parent)", child.LLM, "claude")
	}
}

func TestLoad_WorktreeExpansion_PreservesParent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: child
        path: /code/myproject-child
        channel_id: "222222"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	parent, ok := cfg.Repos["myproject"]
	if !ok {
		t.Fatal("parent repo entry myproject should still exist")
	}
	if parent.ChannelID != "111111" {
		t.Errorf("parent ChannelID = %q, want %q", parent.ChannelID, "111111")
	}
	if parent.WorkingDir != "/code/myproject" {
		t.Errorf("parent WorkingDir = %q, want %q", parent.WorkingDir, "/code/myproject")
	}
	if parent.Provider != "discord" {
		t.Errorf("parent Provider = %q, want %q", parent.Provider, "discord")
	}
	if len(parent.Worktrees) != 1 {
		t.Errorf("parent Worktrees len = %d, want 1", len(parent.Worktrees))
	}
}

func TestLoad_NoWorktrees_BackwardCompatible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  simple-repo:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/simple
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Repos) != 1 {
		t.Errorf("len(Repos) = %d, want 1", len(cfg.Repos))
	}

	repo, ok := cfg.Repos["simple-repo"]
	if !ok {
		t.Fatal("missing repo entry simple-repo")
	}
	if repo.ChannelID != "111111" {
		t.Errorf("repo.ChannelID = %q, want %q", repo.ChannelID, "111111")
	}
	if len(repo.Worktrees) != 0 {
		t.Errorf("repo.Worktrees len = %d, want 0", len(repo.Worktrees))
	}
}

func TestConfig_Validate_DuplicateChannelIDs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  repo-a:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/a
  repo-b:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/b
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for duplicate channel_id")
	}
	if !strings.Contains(err.Error(), "duplicate channel_id") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "duplicate channel_id")
	}
}

func TestConfig_Validate_EmptyWorktreePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: bad-wt
        path: ""
        channel_id: "222222"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty worktree path")
	}
	if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "empty path")
	}
}

func TestConfig_Validate_RelativeWorktreePath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: bad-wt
        path: relative/path
        channel_id: "222222"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for relative worktree path")
	}
	if !strings.Contains(err.Error(), "non-absolute path") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "non-absolute path")
	}
}

func TestConfig_Validate_EmptyWorktreeName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: ""
        path: /code/myproject-wt
        channel_id: "222222"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load() expected error for empty worktree name")
	}
	if !strings.Contains(err.Error(), "empty name") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "empty name")
	}
}

func TestConfig_Validate_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `
repos:
  myproject:
    provider: discord
    channel_id: "111111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: feature
        path: /code/myproject-feature
        channel_id: "222222"
  other-repo:
    provider: discord
    channel_id: "333333"
    llm: claude
    working_dir: /code/other
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Repos) != 3 {
		t.Errorf("len(Repos) = %d, want 3", len(cfg.Repos))
	}
}
