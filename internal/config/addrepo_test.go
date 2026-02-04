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
