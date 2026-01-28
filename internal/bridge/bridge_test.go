package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/anthropics/llm-bridge/internal/config"
	"github.com/anthropics/llm-bridge/internal/provider"
	"github.com/anthropics/llm-bridge/internal/router"
)

func testConfig() *config.Config {
	return &config.Config{
		Repos: map[string]config.RepoConfig{
			"test-repo": {
				Provider:   "discord",
				ChannelID:  "channel-123",
				LLM:        "claude",
				WorkingDir: "/tmp/test",
			},
			"other-repo": {
				Provider:   "discord",
				ChannelID:  "channel-456",
				LLM:        "claude",
				WorkingDir: "/tmp/other",
			},
		},
		Defaults: config.Defaults{
			LLM:             "claude",
			OutputThreshold: 1500,
			IdleTimeout:     "10m",
		},
	}
}

func TestNew(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	if b.cfg != cfg {
		t.Error("config not set")
	}
	if b.providers == nil {
		t.Error("providers map should be initialized")
	}
	if b.repos == nil {
		t.Error("repos map should be initialized")
	}
	if b.output == nil {
		t.Error("output handler should be initialized")
	}
}

func TestBridge_ChannelIDsForProvider(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	ids := b.channelIDsForProvider("discord")
	if len(ids) != 2 {
		t.Errorf("expected 2 channel IDs, got %d", len(ids))
	}

	// Check both channels are present
	found := make(map[string]bool)
	for _, id := range ids {
		found[id] = true
	}
	if !found["channel-123"] || !found["channel-456"] {
		t.Errorf("missing expected channel IDs: %v", ids)
	}

	// Non-existent provider
	ids = b.channelIDsForProvider("telegram")
	if len(ids) != 0 {
		t.Errorf("expected 0 channel IDs for telegram, got %d", len(ids))
	}
}

func TestBridge_RepoForChannel(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	repo := b.repoForChannel("channel-123")
	if repo != "test-repo" {
		t.Errorf("repoForChannel(channel-123) = %q, want test-repo", repo)
	}

	repo = b.repoForChannel("channel-456")
	if repo != "other-repo" {
		t.Errorf("repoForChannel(channel-456) = %q, want other-repo", repo)
	}

	repo = b.repoForChannel("unknown")
	if repo != "" {
		t.Errorf("repoForChannel(unknown) = %q, want empty", repo)
	}
}

func TestBridge_GetStatus_NoRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	status := b.getStatus("unknown-channel")
	if status != "No repo configured for this channel" {
		t.Errorf("unexpected status: %q", status)
	}
}

func TestBridge_GetStatus_NotRunning(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	status := b.getStatus("channel-123")
	if status != "LLM: not running (repo: test-repo)" {
		t.Errorf("unexpected status: %q", status)
	}
}

func TestBridge_CancelLLM_NoRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	result := b.cancelLLM("unknown-channel")
	if result != "No repo configured" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestBridge_CancelLLM_NotRunning(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	result := b.cancelLLM("channel-123")
	if result != "LLM not running" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestBridge_RestartLLM_NoRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	result := b.restartLLM("unknown-channel")
	if result != "No repo configured" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestBridge_RestartLLM_NotRunning(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	result := b.restartLLM("channel-123")
	if result != "LLM stopped. Will restart on next message." {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestBridge_GetTerminalRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	// First call auto-selects a repo
	repo := b.getTerminalRepo()
	if repo != "test-repo" && repo != "other-repo" {
		t.Errorf("expected one of the repos, got %q", repo)
	}

	// Setting explicit repo
	b.mu.Lock()
	b.terminalRepoName = "other-repo"
	b.mu.Unlock()

	repo = b.getTerminalRepo()
	if repo != "other-repo" {
		t.Errorf("expected other-repo, got %q", repo)
	}
}

func TestBridge_GetTerminalRepo_EmptyConfig(t *testing.T) {
	cfg := &config.Config{
		Repos: map[string]config.RepoConfig{},
	}
	b := New(cfg)

	repo := b.getTerminalRepo()
	if repo != "" {
		t.Errorf("expected empty string for no repos, got %q", repo)
	}
}

func TestBridge_AddChannelToSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	session := &repoSession{
		name:     "test-repo",
		channels: []channelRef{{provider: mockProv, channelID: "channel-1"}},
		merger:   NewMerger(2 * time.Second),
	}

	// Add new channel
	mockProv2 := provider.NewMockProvider("terminal")
	b.addChannelToSession(session, mockProv2, "channel-2")
	if len(session.channels) != 2 {
		t.Errorf("expected 2 channels, got %d", len(session.channels))
	}

	// Adding same channel again should not duplicate
	b.addChannelToSession(session, mockProv2, "channel-2")
	if len(session.channels) != 2 {
		t.Errorf("duplicate add should not increase count, got %d", len(session.channels))
	}
}

func TestBridge_HandleBridgeCommand_Help(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")

	route := router.Route{
		Type:    router.RouteToBridge,
		Command: "help",
	}

	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestBridge_Stop(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	b.providers["discord"] = mockProv

	err := b.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if !mockProv.WasStopCalled() {
		t.Error("provider Stop should have been called")
	}
}

func TestBridge_BroadcastOutput_Short(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	session := &repoSession{
		name:     "test-repo",
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	b.broadcastOutput(session, "short message")

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "short message" {
		t.Errorf("message content = %q, want 'short message'", msgs[0].Content)
	}
}

func TestBridge_BroadcastOutput_Long(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.OutputThreshold = 10 // Very short threshold for testing
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	session := &repoSession{
		name:     "test-repo",
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	b.broadcastOutput(session, "this is a longer message that exceeds threshold")

	files := mockProv.GetSentFiles()
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if string(files[0].Content) != "this is a longer message that exceeds threshold" {
		t.Errorf("file content mismatch")
	}
}

func TestBridge_BroadcastOutput_Empty(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	session := &repoSession{
		name:     "test-repo",
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	b.broadcastOutput(session, "")

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 0 {
		t.Errorf("empty content should not send, got %d messages", len(msgs))
	}
}

func TestBridge_BroadcastOutput_MultipleChannels(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv1 := provider.NewMockProvider("discord")
	mockProv2 := provider.NewMockProvider("terminal")
	session := &repoSession{
		name: "test-repo",
		channels: []channelRef{
			{provider: mockProv1, channelID: "channel-1"},
			{provider: mockProv2, channelID: "channel-2"},
		},
		merger: NewMerger(2 * time.Second),
	}

	b.broadcastOutput(session, "broadcast test")

	msgs1 := mockProv1.GetSentMessages()
	msgs2 := mockProv2.GetSentMessages()

	if len(msgs1) != 1 || len(msgs2) != 1 {
		t.Errorf("expected 1 message each, got %d and %d", len(msgs1), len(msgs2))
	}
}

func TestBridge_CheckIdleTimeouts(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	// This test just ensures the function doesn't panic with empty repos
	b.checkIdleTimeouts(10 * time.Minute)

	// Test passes if no panic
}

func TestBridge_ProcessMessage_BridgeCommand(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	b.providers["discord"] = mockProv

	ctx := context.Background()
	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "/help",
		Source:    "discord",
	}

	b.processMessage(ctx, mockProv, msg)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}
