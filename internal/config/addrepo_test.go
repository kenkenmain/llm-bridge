package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddRepo_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := AddRepo(path, "test-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "123",
		LLM:        "claude",
		WorkingDir: "/tmp/test",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("config file was not created at %s", path)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo, ok := cfg.Repos["test-repo"]
	if !ok {
		t.Fatalf("repo %q not found in config", "test-repo")
	}
	if repo.ChannelID != "123" {
		t.Errorf("repo.ChannelID = %q, want %q", repo.ChannelID, "123")
	}
}

func TestAddRepo_ExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `repos:
  existing-repo:
    provider: discord
    channel_id: "111"
    llm: claude
    working_dir: /tmp/existing
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	err := AddRepo(path, "new-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "222",
		LLM:        "claude",
		WorkingDir: "/tmp/new",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	existing, ok := cfg.Repos["existing-repo"]
	if !ok {
		t.Fatalf("repo %q not found in config", "existing-repo")
	}
	if existing.ChannelID != "111" {
		t.Errorf("existing-repo.ChannelID = %q, want %q", existing.ChannelID, "111")
	}

	newRepo, ok := cfg.Repos["new-repo"]
	if !ok {
		t.Fatalf("repo %q not found in config", "new-repo")
	}
	if newRepo.ChannelID != "222" {
		t.Errorf("new-repo.ChannelID = %q, want %q", newRepo.ChannelID, "222")
	}
}

func TestAddRepo_DuplicateChannelID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `repos:
  existing-repo:
    provider: discord
    channel_id: "111"
    llm: claude
    working_dir: /tmp/existing
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	err := AddRepo(path, "new-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "111",
		LLM:        "claude",
		WorkingDir: "/tmp/new",
	})
	if err == nil {
		t.Fatal("AddRepo() expected error for duplicate channel_id")
	}
	if !strings.Contains(err.Error(), "duplicate channel_id") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "duplicate channel_id")
	}

	// Verify original file is unchanged.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Repos) != 1 {
		t.Errorf("len(Repos) = %d, want 1 (original file should be unchanged)", len(cfg.Repos))
	}
	if _, ok := cfg.Repos["existing-repo"]; !ok {
		t.Errorf("existing-repo should still be present in config")
	}
}

func TestAddRepo_WithGitRootAndBranch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := AddRepo(path, "wt-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "123",
		LLM:        "claude",
		WorkingDir: "/tmp/wt",
		GitRoot:    "/tmp/main",
		Branch:     "feature/auth",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo, ok := cfg.Repos["wt-repo"]
	if !ok {
		t.Fatalf("repo %q not found in config", "wt-repo")
	}
	if repo.GitRoot != "/tmp/main" {
		t.Errorf("repo.GitRoot = %q, want %q", repo.GitRoot, "/tmp/main")
	}
	if repo.Branch != "feature/auth" {
		t.Errorf("repo.Branch = %q, want %q", repo.Branch, "feature/auth")
	}
}

func TestAddRepo_OverwritesExistingRepo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	content := `repos:
  my-repo:
    provider: discord
    channel_id: "111"
    llm: claude
    working_dir: /tmp/old
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	err := AddRepo(path, "my-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "111",
		LLM:        "claude",
		WorkingDir: "/tmp/new",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo, ok := cfg.Repos["my-repo"]
	if !ok {
		t.Fatalf("repo %q not found in config", "my-repo")
	}
	if repo.WorkingDir != "/tmp/new" {
		t.Errorf("repo.WorkingDir = %q, want %q", repo.WorkingDir, "/tmp/new")
	}
}

func TestAddRepo_CorruptExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write invalid YAML that will fail to parse
	if err := os.WriteFile(path, []byte("{{invalid yaml: ["), 0600); err != nil {
		t.Fatalf("write corrupt config: %v", err)
	}

	err := AddRepo(path, "new-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "123",
		LLM:        "claude",
		WorkingDir: "/tmp/test",
	})
	if err == nil {
		t.Fatal("AddRepo() expected error for corrupt existing config")
	}
	if !strings.Contains(err.Error(), "load config") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "load config")
	}

	// Verify original file is NOT overwritten
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile error = %v", readErr)
	}
	if string(data) != "{{invalid yaml: [" {
		t.Errorf("corrupt config file was overwritten, content = %q", string(data))
	}
}

func TestAddRepo_ValidationError_WorktreeEmptyName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Create a config with a repo that has a worktree with empty name.
	// AddRepo should reject this via Validate() before writing.
	err := AddRepo(path, "myproject", RepoConfig{
		Provider:   "discord",
		ChannelID:  "123",
		LLM:        "claude",
		WorkingDir: "/tmp/test",
		Worktrees: []WorktreeConfig{
			{Name: "", Path: "/tmp/wt", ChannelID: "456"},
		},
	})
	if err == nil {
		t.Fatal("AddRepo() expected error for worktree with empty name")
	}
	if !strings.Contains(err.Error(), "empty name") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "empty name")
	}

	// File should not be created
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("config file should not exist after validation failure")
	}
}

func TestAddRepo_NewFile_HasDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	err := AddRepo(path, "test-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "123",
		LLM:        "claude",
		WorkingDir: "/tmp/test",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	// Read raw YAML to verify defaults were actually written by AddRepo,
	// not just applied at Load() time.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	raw := string(data)
	if !strings.Contains(raw, "llm: claude") {
		t.Errorf("raw YAML missing default llm, got:\n%s", raw)
	}
	if !strings.Contains(raw, "output_threshold: 1500") {
		t.Errorf("raw YAML missing default output_threshold, got:\n%s", raw)
	}
	if !strings.Contains(raw, "idle_timeout: 10m") {
		t.Errorf("raw YAML missing default idle_timeout, got:\n%s", raw)
	}
}

func TestAddRepo_ExistingConfigNoReposKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// A config file with defaults but no repos section — cfg.Repos will be nil after Load.
	content := `defaults:
  llm: claude
  output_threshold: 1500
  idle_timeout: 10m
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	err := AddRepo(path, "new-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "123",
		LLM:        "claude",
		WorkingDir: "/tmp/test",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	repo, ok := cfg.Repos["new-repo"]
	if !ok {
		t.Fatalf("repo %q not found in config", "new-repo")
	}
	if repo.ChannelID != "123" {
		t.Errorf("repo.ChannelID = %q, want %q", repo.ChannelID, "123")
	}
}

func TestAddRepo_PreservesWorktreeConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Config with worktree definitions — AddRepo must not expand worktrees
	// into the written YAML, or the config becomes unloadable on next Load().
	content := `repos:
  myproject:
    provider: discord
    channel_id: "111"
    llm: claude
    working_dir: /code/myproject
    worktrees:
      - name: feature
        path: /code/myproject-feature
        channel_id: "222"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	err := AddRepo(path, "other-repo", RepoConfig{
		Provider:   "discord",
		ChannelID:  "333",
		LLM:        "claude",
		WorkingDir: "/tmp/other",
	})
	if err != nil {
		t.Fatalf("AddRepo() error = %v", err)
	}

	// Verify the written config is still loadable (no worktree expansion corruption).
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() after AddRepo error = %v (worktree round-trip corruption)", err)
	}

	// Should have 3 entries: myproject, myproject/feature (expanded by Load), other-repo.
	if len(cfg.Repos) != 3 {
		t.Errorf("len(Repos) = %d, want 3", len(cfg.Repos))
	}
	if _, ok := cfg.Repos["myproject"]; !ok {
		t.Errorf("missing repo %q", "myproject")
	}
	if _, ok := cfg.Repos["myproject/feature"]; !ok {
		t.Errorf("missing expanded worktree repo %q", "myproject/feature")
	}
	if _, ok := cfg.Repos["other-repo"]; !ok {
		t.Errorf("missing repo %q", "other-repo")
	}
}
