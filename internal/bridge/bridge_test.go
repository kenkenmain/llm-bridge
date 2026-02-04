package bridge

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/llm-bridge/internal/config"
	"github.com/anthropics/llm-bridge/internal/git"
	"github.com/anthropics/llm-bridge/internal/llm"
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

func TestBridge_HandleBridgeCommand_Status(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "status"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "not running") {
		t.Errorf("expected 'not running' in status, got %q", msgs[0].Content)
	}
}

func TestBridge_HandleBridgeCommand_Cancel(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "cancel"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
}

func TestBridge_HandleBridgeCommand_Restart(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "restart"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "restart") {
		t.Errorf("expected 'restart' in response, got %q", msgs[0].Content)
	}
}

func TestBridge_HandleBridgeCommand_Unknown(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "foobar"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Unknown command") {
		t.Errorf("expected 'Unknown command' in response, got %q", msgs[0].Content)
	}
}

func TestBridge_GetStatus_Running(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	b.repos["test-repo"] = &repoSession{
		name: "test-repo",
		llm:  mockLLM,
	}

	status := b.getStatus("channel-123")
	if !strings.Contains(status, "claude running") {
		t.Errorf("expected running status, got %q", status)
	}
	if !strings.Contains(status, "test-repo") {
		t.Errorf("expected repo name in status, got %q", status)
	}
}

func TestBridge_CancelLLM_Running(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	b.repos["test-repo"] = &repoSession{
		name: "test-repo",
		llm:  mockLLM,
	}

	result := b.cancelLLM("channel-123")
	if result != "Sent interrupt signal" {
		t.Errorf("expected 'Sent interrupt signal', got %q", result)
	}
}

func TestBridge_RestartLLM_Running(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	cancelled := false
	b.repos["test-repo"] = &repoSession{
		name:      "test-repo",
		llm:       mockLLM,
		cancelCtx: func() { cancelled = true },
	}

	result := b.restartLLM("channel-123")
	if result != "LLM stopped. Will restart on next message." {
		t.Errorf("unexpected result: %q", result)
	}

	if !cancelled {
		t.Error("cancelCtx should have been called")
	}

	// Session should be removed
	b.mu.Lock()
	_, exists := b.repos["test-repo"]
	b.mu.Unlock()
	if exists {
		t.Error("session should have been deleted")
	}
}

func TestBridge_HandleLLMMessage_NoRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	msg := provider.Message{
		ChannelID: "unknown-channel",
		Content:   "hello",
		Source:    "discord",
	}
	route := router.Route{Type: router.RouteToLLM, Raw: "hello"}

	b.handleLLMMessage(context.Background(), mockProv, msg, route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "No repo configured") {
		t.Errorf("expected error message, got %q", msgs[0].Content)
	}
}

func TestBridge_HandleLLMMessage_WithSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "refactor the auth module",
		Source:    "discord",
	}
	route := router.Route{Type: router.RouteToLLM, Raw: "refactor the auth module"}

	b.handleLLMMessage(context.Background(), mockProv, msg, route)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
	if sentMsgs[0].Content != "refactor the auth module" {
		t.Errorf("LLM message = %q", sentMsgs[0].Content)
	}
}

func TestBridge_HandleLLMMessage_SendError(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockLLM.setSendError(context.DeadlineExceeded)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "test",
		Source:    "discord",
	}
	route := router.Route{Type: router.RouteToLLM, Raw: "test"}

	b.handleLLMMessage(context.Background(), mockProv, msg, route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected error message, got %d messages", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Error") {
		t.Errorf("expected error message, got %q", msgs[0].Content)
	}
}

func TestBridge_ProcessMessage_LLMRoute(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "do something",
		Source:    "discord",
	}

	b.processMessage(context.Background(), mockProv, msg)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
}

func TestBridge_ProcessMessage_DoubleColonCommand(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "::commit",
		Source:    "discord",
	}

	b.processMessage(context.Background(), mockProv, msg)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
	if sentMsgs[0].Content != "/commit" {
		t.Errorf("expected '/commit', got %q", sentMsgs[0].Content)
	}
}

func TestBridge_CheckIdleTimeouts_WithIdleSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockLLM.setLastActivity(time.Now().Add(-20 * time.Minute))

	mockProv := provider.NewMockProvider("discord")
	cancelled := false

	b.repos["test-repo"] = &repoSession{
		name:      "test-repo",
		llm:       mockLLM,
		channels:  []channelRef{{provider: mockProv, channelID: "channel-123"}},
		cancelCtx: func() { cancelled = true },
	}

	b.checkIdleTimeouts(10 * time.Minute)

	if !cancelled {
		t.Error("idle session should have been cancelled")
	}

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected idle notification, got %d messages", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "idle timeout") {
		t.Errorf("expected idle timeout message, got %q", msgs[0].Content)
	}

	b.mu.Lock()
	_, exists := b.repos["test-repo"]
	b.mu.Unlock()
	if exists {
		t.Error("idle session should have been deleted")
	}
}

func TestBridge_CheckIdleTimeouts_ActiveSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	// Recent activity - should NOT be timed out

	b.repos["test-repo"] = &repoSession{
		name: "test-repo",
		llm:  mockLLM,
	}

	b.checkIdleTimeouts(10 * time.Minute)

	b.mu.Lock()
	_, exists := b.repos["test-repo"]
	b.mu.Unlock()
	if !exists {
		t.Error("active session should NOT have been deleted")
	}
}

func TestBridge_HandleMessages_ContextCancel(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool, 1)
	go func() {
		b.handleMessages(ctx, mockProv)
		done <- true
	}()

	cancel()

	select {
	case <-done:
		// handleMessages exited
	case <-time.After(100 * time.Millisecond):
		t.Error("handleMessages should exit on context cancel")
	}
}

func TestBridge_HandleMessages_ChannelClosed(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	ctx := context.Background()

	done := make(chan bool, 1)
	go func() {
		b.handleMessages(ctx, mockProv)
		done <- true
	}()

	_ = mockProv.Stop() // Closes the messages channel

	select {
	case <-done:
		// handleMessages exited
	case <-time.After(100 * time.Millisecond):
		t.Error("handleMessages should exit when channel is closed")
	}
}

func TestBridge_Stop_WithSessions(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	cancelled := false

	b.repos["test-repo"] = &repoSession{
		name:      "test-repo",
		llm:       mockLLM,
		cancelCtx: func() { cancelled = true },
	}

	mockProv := provider.NewMockProvider("discord")
	b.providers["discord"] = mockProv

	err := b.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if !cancelled {
		t.Error("session cancelCtx should have been called")
	}

	if mockLLM.Running() {
		t.Error("LLM should have been stopped")
	}
}

func TestBridge_ProcessTerminalMessage_SelectNoArgs(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	// Override writer to capture output
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "/select",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)
	// Test passes if no panic - output goes to writer
}

func TestBridge_ProcessTerminalMessage_SelectUnknownRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "/select nonexistent",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)
	// Test passes if no panic
}

func TestBridge_ProcessTerminalMessage_SelectValidRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "/select test-repo",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)

	b.mu.Lock()
	selected := b.terminalRepoName
	b.mu.Unlock()

	if selected != "test-repo" {
		t.Errorf("expected selected repo 'test-repo', got %q", selected)
	}
}

func TestBridge_ProcessTerminalMessage_NoRepos(t *testing.T) {
	cfg := &config.Config{
		Repos: map[string]config.RepoConfig{},
	}
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "do something",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)
	// Test passes if no panic - "No repos configured" is sent to terminal
}

func TestBridge_ProcessTerminalMessage_BridgeCommand(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "/status",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)
	// Test passes if no panic
}

func TestBridge_ProcessTerminalMessage_LLMRoute(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{},
		merger:   NewMerger(2 * time.Second),
	}
	b.terminalRepoName = "test-repo"

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "build the project",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
}

func TestBridge_GetOrCreateSession_NewSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return mockLLM, nil
	}

	mockProv := provider.NewMockProvider("discord")
	repo := cfg.Repos["test-repo"]

	session, err := b.getOrCreateSession(context.Background(), "test-repo", repo, mockProv)
	if err != nil {
		t.Fatalf("getOrCreateSession error = %v", err)
	}
	if session.name != "test-repo" {
		t.Errorf("session name = %q, want test-repo", session.name)
	}
	if len(session.channels) != 1 {
		t.Errorf("expected 1 channel, got %d", len(session.channels))
	}
}

func TestBridge_GetOrCreateSession_ExistingSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	repo := cfg.Repos["test-repo"]
	session, err := b.getOrCreateSession(context.Background(), "test-repo", repo, mockProv)
	if err != nil {
		t.Fatalf("getOrCreateSession error = %v", err)
	}
	if session.llm != mockLLM {
		t.Error("should reuse existing session")
	}
}

func TestBridge_GetOrCreateSession_FactoryError(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return nil, fmt.Errorf("factory error")
	}

	mockProv := provider.NewMockProvider("discord")
	repo := cfg.Repos["test-repo"]

	_, err := b.getOrCreateSession(context.Background(), "test-repo", repo, mockProv)
	if err == nil {
		t.Error("expected error from factory")
	}
}

func TestBridge_HandleLLMMessage_CreatesSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return mockLLM, nil
	}

	mockProv := provider.NewMockProvider("discord")
	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "start task",
		Source:    "discord",
	}
	route := router.Route{Type: router.RouteToLLM, Raw: "start task"}

	b.handleLLMMessage(context.Background(), mockProv, msg, route)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
}

func TestBridge_ProcessTerminalMessage_CreatesSession(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)
	b.terminalRepoName = "test-repo"

	mockLLM := newMockLLM("claude")
	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return mockLLM, nil
	}

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "do work",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
}

func TestBridge_ProcessTerminalMessage_SessionCreateError(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)
	b.terminalRepoName = "test-repo"

	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return nil, fmt.Errorf("spawn failed")
	}

	term := provider.NewTerminal("terminal")
	msg := provider.Message{
		ChannelID: "terminal",
		Content:   "do work",
		Source:    "terminal",
	}

	b.processTerminalMessage(context.Background(), term, msg)
	// Should not panic - error goes to terminal
}

// Tests for Start method
func TestBridge_Start_NoProviders(t *testing.T) {
	cfg := &config.Config{
		Repos:    map[string]config.RepoConfig{},
		Defaults: config.Defaults{IdleTimeout: "10m"},
	}
	b := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Start should run and return after context is done
	err := b.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}
}

func TestBridge_Start_WithTerminal(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := b.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Terminal provider should have been started
	if _, ok := b.providers["terminal"]; !ok {
		t.Error("terminal provider should be registered")
	}
}

func TestBridge_Start_WithDiscord(t *testing.T) {
	cfg := testConfig()
	cfg.Providers.Discord.BotToken = "test-token"

	b := New(cfg)

	// Inject mock discord factory
	mockDiscord := provider.NewMockProvider("discord")
	b.discordFactory = func(token string, channelIDs []string) provider.Provider {
		return mockDiscord
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := b.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	// Discord provider should have been started
	if _, ok := b.providers["discord"]; !ok {
		t.Error("discord provider should be registered")
	}
	if !mockDiscord.WasStartCalled() {
		t.Error("discord Start should have been called")
	}
}

func TestBridge_Start_NoTokenSkipsDiscord(t *testing.T) {
	cfg := testConfig()
	// BotToken empty, no default — Discord should not start

	discordFactoryCalled := false
	b := New(cfg)
	b.discordFactory = func(token string, channelIDs []string) provider.Provider {
		discordFactoryCalled = true
		return provider.NewMockProvider("discord")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = b.Start(ctx)

	if discordFactoryCalled {
		t.Error("discord factory should not be called when token is empty")
	}
}

func TestBridge_Start_DiscordError(t *testing.T) {
	cfg := testConfig()
	cfg.Providers.Discord.BotToken = "test-token"

	b := New(cfg)

	// Inject mock discord factory that returns error
	mockDiscord := provider.NewMockProvider("discord")
	mockDiscord.SetStartError(fmt.Errorf("connection failed"))
	b.discordFactory = func(token string, channelIDs []string) provider.Provider {
		return mockDiscord
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := b.Start(ctx)
	if err == nil {
		t.Error("Start() should return error when discord fails")
	}
	if !strings.Contains(err.Error(), "discord") {
		t.Errorf("error should mention discord, got: %v", err)
	}
}

func TestBridge_Start_NoChannelsForDiscord(t *testing.T) {
	cfg := testConfig()
	cfg.Providers.Discord.BotToken = "test-token"
	// Change all repos to use telegram (not configured)
	for name, repo := range cfg.Repos {
		repo.Provider = "telegram"
		cfg.Repos[name] = repo
	}

	b := New(cfg)

	// Discord factory should not be called since no channels use discord
	discordFactoryCalled := false
	b.discordFactory = func(token string, channelIDs []string) provider.Provider {
		discordFactoryCalled = true
		return provider.NewMockProvider("discord")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = b.Start(ctx)

	if discordFactoryCalled {
		t.Error("discord factory should not be called when no channels use discord")
	}
}

// Tests for handleTerminalMessages
func TestBridge_HandleTerminalMessages_ContextCancel(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		b.handleTerminalMessages(ctx, term)
		done <- true
	}()

	cancel()

	select {
	case <-done:
		// handleTerminalMessages exited
	case <-time.After(100 * time.Millisecond):
		t.Error("handleTerminalMessages should exit when context is cancelled")
	}
}

func TestBridge_HandleTerminalMessages_ChannelClosed(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	term := provider.NewTerminal("terminal")
	ctx := context.Background()

	done := make(chan bool)
	go func() {
		b.handleTerminalMessages(ctx, term)
		done <- true
	}()

	_ = term.Stop() // Closes the messages channel

	select {
	case <-done:
		// handleTerminalMessages exited
	case <-time.After(100 * time.Millisecond):
		t.Error("handleTerminalMessages should exit when channel is closed")
	}
}

// Tests for idleTimeoutLoop
func TestBridge_IdleTimeoutLoop_ContextCancel(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.IdleTimeout = "1ms" // Very short for testing
	b := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		b.idleTimeoutLoop(ctx)
		done <- true
	}()

	cancel()

	select {
	case <-done:
		// idleTimeoutLoop exited
	case <-time.After(100 * time.Millisecond):
		t.Error("idleTimeoutLoop should exit when context is cancelled")
	}
}

// Tests for readOutput
func TestBridge_ReadOutput_NilOutput(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	// Output returns nil by default

	session := &repoSession{
		name: "test-repo",
		llm:  mockLLM,
	}

	// Should return early without panic
	done := make(chan bool)
	go func() {
		b.readOutput(session, "test-repo")
		done <- true
	}()

	select {
	case <-done:
		// readOutput returned
	case <-time.After(100 * time.Millisecond):
		t.Error("readOutput should return immediately when output is nil")
	}
}

func TestBridge_ReadOutput_WithOutput(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	// Create a pipe for simulating output
	pr, pw := io.Pipe()
	mockLLM.SetOutput(pr)

	mockProv := provider.NewMockProvider("test")
	_ = mockProv.Start(context.Background())

	session := &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	done := make(chan bool)
	go func() {
		b.readOutput(session, "test-repo")
		done <- true
	}()

	// Write some output and close
	_, _ = pw.Write([]byte("Hello, world!\n"))
	_ = pw.Close()

	select {
	case <-done:
		// readOutput returned
	case <-time.After(500 * time.Millisecond):
		t.Error("readOutput should return when pipe is closed")
	}

	// Verify output was broadcast
	msgs := mockProv.GetSentMessages()
	if len(msgs) == 0 {
		t.Error("expected output to be broadcast")
	}
}

func TestBridge_ReadOutput_BufferFlush(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.OutputThreshold = 10 // Very small threshold
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	pr, pw := io.Pipe()
	mockLLM.SetOutput(pr)

	mockProv := provider.NewMockProvider("test")
	_ = mockProv.Start(context.Background())

	session := &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	done := make(chan bool)
	go func() {
		b.readOutput(session, "test-repo")
		done <- true
	}()

	// Write more than threshold
	_, _ = pw.Write([]byte("This is a longer message that exceeds the threshold\n"))
	_ = pw.Close()

	select {
	case <-done:
		// readOutput returned
	case <-time.After(500 * time.Millisecond):
		t.Error("readOutput should return when pipe is closed")
	}
}

// Rate limiting integration tests

func TestBridge_New_InitializesLimiters(t *testing.T) {
	cfg := testConfig()
	// Default config has rate limiting enabled (nil Enabled defaults to true)
	b := New(cfg)

	if b.userLimiter == nil {
		t.Error("userLimiter should be initialized when rate limiting is enabled")
	}
	if b.channelLimiter == nil {
		t.Error("channelLimiter should be initialized when rate limiting is enabled")
	}
}

func TestBridge_New_LimitersNilWhenDisabled(t *testing.T) {
	cfg := testConfig()
	disabled := false
	cfg.Defaults.RateLimit.Enabled = &disabled
	b := New(cfg)

	if b.userLimiter != nil {
		t.Error("userLimiter should be nil when rate limiting is disabled")
	}
	if b.channelLimiter != nil {
		t.Error("channelLimiter should be nil when rate limiting is disabled")
	}
}

func TestBridge_RateLimitedUser(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.RateLimit = config.RateLimitConfig{
		UserRate:     0.1,  // very slow refill
		UserBurst:    1,    // only 1 allowed
		ChannelRate:  100,  // high channel limit so it doesn't interfere
		ChannelBurst: 100,
	}
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	ctx := context.Background()

	// First message should pass (uses AuthorID for rate limiting)
	msg1 := provider.Message{
		ChannelID: "channel-123",
		Content:   "hello",
		Author:    "testuser",
		AuthorID:  "user-snowflake-123",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msg1)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("first message should pass, got %d LLM messages", len(sentMsgs))
	}

	// Second message from same AuthorID should be rate limited
	msg2 := provider.Message{
		ChannelID: "channel-123",
		Content:   "world",
		Author:    "testuser",
		AuthorID:  "user-snowflake-123",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msg2)

	sentMsgs = mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("second message should be rate limited, got %d LLM messages", len(sentMsgs))
	}

	// Check rejection message was sent
	provMsgs := mockProv.GetSentMessages()
	found := false
	for _, m := range provMsgs {
		if strings.Contains(m.Content, "Rate limited") && strings.Contains(m.Content, "testuser") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected rate limit rejection message mentioning user")
	}
}

func TestBridge_RateLimitedChannel(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.RateLimit = config.RateLimitConfig{
		UserRate:     100, // high user limit so it doesn't interfere
		UserBurst:    100,
		ChannelRate:  0.1, // very slow refill
		ChannelBurst: 1,   // only 1 allowed per channel
	}
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	ctx := context.Background()

	// First message should pass
	msg1 := provider.Message{
		ChannelID: "channel-123",
		Content:   "hello",
		Author:    "user1",
		AuthorID:  "id-1",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msg1)

	// Second message from different user on same channel should be rate limited
	msg2 := provider.Message{
		ChannelID: "channel-123",
		Content:   "hi",
		Author:    "user2",
		AuthorID:  "id-2",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msg2)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected only 1 LLM message (second should be channel-limited), got %d", len(sentMsgs))
	}

	// Check channel rejection message
	provMsgs := mockProv.GetSentMessages()
	found := false
	for _, m := range provMsgs {
		if strings.Contains(m.Content, "too many messages in this channel") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected channel rate limit rejection message")
	}
}

func TestBridge_RateLimitBypassBridgeCommands(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.RateLimit = config.RateLimitConfig{
		UserRate:     0.001, // extremely slow
		UserBurst:    0,     // zero burst - would block everything
		ChannelRate:  0.001,
		ChannelBurst: 0,
	}
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	ctx := context.Background()

	// Bridge commands should bypass rate limiting entirely
	for i := 0; i < 5; i++ {
		msg := provider.Message{
			ChannelID: "channel-123",
			Content:   "/help",
			Author:    "testuser",
			AuthorID:  "user-123",
			Source:    "discord",
		}
		b.processMessage(ctx, mockProv, msg)
	}

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 5 {
		t.Errorf("all 5 bridge commands should bypass rate limiting, got %d responses", len(msgs))
	}

	// Verify they are help responses, not rate limit messages
	for _, m := range msgs {
		if strings.Contains(m.Content, "Rate limited") {
			t.Error("bridge commands should never receive rate limit messages")
		}
	}
}

func TestBridge_RateLimitDisabled(t *testing.T) {
	cfg := testConfig()
	disabled := false
	cfg.Defaults.RateLimit = config.RateLimitConfig{
		Enabled:      &disabled,
		UserRate:     0.001,
		UserBurst:    0,
		ChannelRate:  0.001,
		ChannelBurst: 0,
	}
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	ctx := context.Background()

	// Send multiple messages - none should be rate limited
	for i := 0; i < 5; i++ {
		msg := provider.Message{
			ChannelID: "channel-123",
			Content:   fmt.Sprintf("message %d", i),
			Author:    "testuser",
			AuthorID:  "user-123",
			Source:    "discord",
		}
		b.processMessage(ctx, mockProv, msg)
	}

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 5 {
		t.Errorf("with rate limiting disabled, all 5 messages should pass, got %d", len(sentMsgs))
	}

	// No rate limit messages should have been sent
	provMsgs := mockProv.GetSentMessages()
	for _, m := range provMsgs {
		if strings.Contains(m.Content, "Rate limited") {
			t.Error("no rate limit messages should be sent when disabled")
		}
	}
}

func TestBridge_RateLimitTerminalBypass(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.RateLimit = config.RateLimitConfig{
		UserRate:     0.001, // extremely slow
		UserBurst:    0,     // zero burst - would block everything
		ChannelRate:  100,   // high channel rate
		ChannelBurst: 100,
	}
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")

	// Terminal messages have empty AuthorID, so user rate limiting is skipped
	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "hello",
		Author:    "terminal-user",
		AuthorID:  "", // empty = terminal
		Source:    "terminal",
	}

	// isRateLimited should return false for terminal (no AuthorID)
	result := b.isRateLimited(mockProv, msg)
	if result {
		t.Error("terminal messages (empty AuthorID) should not be user-rate-limited")
	}
}

func TestBridge_RateLimitDifferentUsersIndependent(t *testing.T) {
	cfg := testConfig()
	cfg.Defaults.RateLimit = config.RateLimitConfig{
		UserRate:     0.1, // very slow refill
		UserBurst:    1,   // only 1 allowed
		ChannelRate:  100, // high channel limit
		ChannelBurst: 100,
	}
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	mockProv := provider.NewMockProvider("discord")

	b.repos["test-repo"] = &repoSession{
		name:     "test-repo",
		llm:      mockLLM,
		channels: []channelRef{{provider: mockProv, channelID: "channel-123"}},
		merger:   NewMerger(2 * time.Second),
	}

	ctx := context.Background()

	// User A sends a message - should pass
	msgA := provider.Message{
		ChannelID: "channel-123",
		Content:   "from user A",
		Author:    "userA",
		AuthorID:  "snowflake-A",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msgA)

	// User B sends a message - should also pass (independent bucket)
	msgB := provider.Message{
		ChannelID: "channel-123",
		Content:   "from user B",
		Author:    "userB",
		AuthorID:  "snowflake-B",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msgB)

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 2 {
		t.Errorf("different users should have independent limits, got %d LLM messages (want 2)", len(sentMsgs))
	}

	// User A sends again - should be rate limited
	msgA2 := provider.Message{
		ChannelID: "channel-123",
		Content:   "from user A again",
		Author:    "userA",
		AuthorID:  "snowflake-A",
		Source:    "discord",
	}
	b.processMessage(ctx, mockProv, msgA2)

	sentMsgs = mockLLM.getSentMessages()
	if len(sentMsgs) != 2 {
		t.Errorf("user A second message should be rate limited, got %d LLM messages (want 2)", len(sentMsgs))
	}
}

func TestBridge_IsRateLimited_NilLimiters(t *testing.T) {
	cfg := testConfig()
	disabled := false
	cfg.Defaults.RateLimit.Enabled = &disabled
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")
	msg := provider.Message{
		ChannelID: "channel-123",
		Content:   "test",
		AuthorID:  "user-123",
	}

	// With nil limiters, should always return false
	if b.isRateLimited(mockProv, msg) {
		t.Error("nil limiters should not rate limit")
	}
}

func TestBridge_GetStatus_RunningWithGitInfo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	b.repos["test-repo"] = &repoSession{
		name: "test-repo",
		llm:  mockLLM,
		gitInfo: &git.RepoInfo{
			Branch:     "main",
			IsWorktree: false,
		},
	}

	status := b.getStatus("channel-123")
	if !strings.Contains(status, "claude running") {
		t.Errorf("expected running status, got %q", status)
	}
	if !strings.Contains(status, "branch: main") {
		t.Errorf("expected branch in status, got %q", status)
	}
	if strings.Contains(status, "worktree") {
		t.Errorf("should not contain 'worktree' for non-worktree repo, got %q", status)
	}
}

func TestBridge_GetStatus_RunningWithWorktreeGitInfo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	b.repos["test-repo"] = &repoSession{
		name: "test-repo",
		llm:  mockLLM,
		gitInfo: &git.RepoInfo{
			Branch:     "feature/test",
			IsWorktree: true,
		},
	}

	status := b.getStatus("channel-123")
	if !strings.Contains(status, "claude running") {
		t.Errorf("expected running status, got %q", status)
	}
	if !strings.Contains(status, "branch: feature/test") {
		t.Errorf("expected branch in status, got %q", status)
	}
	if !strings.Contains(status, "worktree") {
		t.Errorf("expected 'worktree' in status for worktree repo, got %q", status)
	}
}

func TestBridge_GetStatus_RunningWithoutGitInfo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)

	b.repos["test-repo"] = &repoSession{
		name:    "test-repo",
		llm:     mockLLM,
		gitInfo: nil,
	}

	status := b.getStatus("channel-123")
	if !strings.Contains(status, "claude running") {
		t.Errorf("expected running status, got %q", status)
	}
	if !strings.Contains(status, "test-repo") {
		t.Errorf("expected repo name in status, got %q", status)
	}
	if strings.Contains(status, "branch:") {
		t.Errorf("should not contain branch info when gitInfo is nil, got %q", status)
	}
}

func TestBridge_GetOrCreateSession_DetectsGit(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return mockLLM, nil
	}

	detectedDir := ""
	b.gitDetector = func(dir string) (*git.RepoInfo, error) {
		detectedDir = dir
		return &git.RepoInfo{
			Branch:     "main",
			IsWorktree: false,
			Worktrees: []git.WorktreeInfo{
				{Path: "/tmp/test", Branch: "main", IsMain: true},
			},
		}, nil
	}

	mockProv := provider.NewMockProvider("discord")
	repo := cfg.Repos["test-repo"]

	session, err := b.getOrCreateSession(context.Background(), "test-repo", repo, mockProv)
	if err != nil {
		t.Fatalf("getOrCreateSession error = %v", err)
	}

	if detectedDir != "/tmp/test" {
		t.Errorf("gitDetector called with dir = %q, want %q", detectedDir, "/tmp/test")
	}

	if session.gitInfo == nil {
		t.Fatal("session.gitInfo should not be nil")
	}
	if session.gitInfo.Branch != "main" {
		t.Errorf("session.gitInfo.Branch = %q, want %q", session.gitInfo.Branch, "main")
	}
}

func TestBridge_GetOrCreateSession_GitDetectionFailure(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		return mockLLM, nil
	}

	b.gitDetector = func(dir string) (*git.RepoInfo, error) {
		return nil, fmt.Errorf("not a git repository")
	}

	mockProv := provider.NewMockProvider("discord")
	repo := cfg.Repos["test-repo"]

	session, err := b.getOrCreateSession(context.Background(), "test-repo", repo, mockProv)
	if err != nil {
		t.Fatalf("getOrCreateSession should succeed even if git detection fails, got error = %v", err)
	}

	if session.gitInfo != nil {
		t.Errorf("session.gitInfo should be nil when git detection fails, got %+v", session.gitInfo)
	}

	if session.name != "test-repo" {
		t.Errorf("session.name = %q, want %q", session.name, "test-repo")
	}
}

func TestBridge_HandleBridgeCommand_Worktrees_NoRepo(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	mockProv := provider.NewMockProvider("discord")

	route := router.Route{
		Type:    router.RouteToBridge,
		Command: "worktrees",
	}

	b.handleBridgeCommand(mockProv, "unknown-channel", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "No repo configured") {
		t.Errorf("expected 'No repo configured' in response, got %q", msgs[0].Content)
	}
}

func TestBridge_ListWorktrees_WithWorktrees(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	b.worktreeLister = func(dir string) ([]git.WorktreeInfo, error) {
		return []git.WorktreeInfo{
			{Path: "/tmp/test", Branch: "main", IsMain: true},
			{Path: "/tmp/test-feature", Branch: "feature/foo"},
		}, nil
	}

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "worktrees"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Worktrees for") {
		t.Errorf("expected worktree listing header, got %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "main") {
		t.Errorf("expected main branch in output, got %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "feature/foo") {
		t.Errorf("expected feature branch in output, got %q", msgs[0].Content)
	}
	// Current worktree should be marked with *
	if !strings.Contains(msgs[0].Content, "* ") {
		t.Errorf("expected current worktree marker, got %q", msgs[0].Content)
	}
}

func TestBridge_ListWorktrees_GitError(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	b.worktreeLister = func(dir string) ([]git.WorktreeInfo, error) {
		return nil, fmt.Errorf("not a git repository")
	}

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "worktrees"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "git error") {
		t.Errorf("expected git error message, got %q", msgs[0].Content)
	}
}

func TestBridge_ListWorktrees_NoLinkedWorktrees(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	b.worktreeLister = func(dir string) ([]git.WorktreeInfo, error) {
		return []git.WorktreeInfo{
			{Path: "/tmp/test", Branch: "main", IsMain: true},
		}, nil
	}

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "worktrees"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "No linked worktrees") {
		t.Errorf("expected no linked worktrees message, got %q", msgs[0].Content)
	}
}

func TestBridge_ListWorktrees_WithConfiguredRepo(t *testing.T) {
	cfg := testConfig()
	// Add a worktree repo with matching path
	cfg.Repos["test-repo/feature"] = config.RepoConfig{
		Provider:   "discord",
		ChannelID:  "channel-789",
		LLM:        "claude",
		WorkingDir: "/tmp/test-feature",
	}
	b := New(cfg)

	b.worktreeLister = func(dir string) ([]git.WorktreeInfo, error) {
		return []git.WorktreeInfo{
			{Path: "/tmp/test", Branch: "main", IsMain: true},
			{Path: "/tmp/test-feature", Branch: "feature/foo"},
		}, nil
	}

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "worktrees"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "repo: test-repo/feature") {
		t.Errorf("expected configured repo name in output, got %q", msgs[0].Content)
	}
}

func TestBridge_ListWorktrees_WithActiveSession(t *testing.T) {
	cfg := testConfig()
	cfg.Repos["test-repo/feature"] = config.RepoConfig{
		Provider:   "discord",
		ChannelID:  "channel-789",
		LLM:        "claude",
		WorkingDir: "/tmp/test-feature",
	}
	b := New(cfg)

	mockLLM := newMockLLM("claude")
	mockLLM.setRunning(true)
	b.repos["test-repo/feature"] = &repoSession{
		name: "test-repo/feature",
		llm:  mockLLM,
	}

	b.worktreeLister = func(dir string) ([]git.WorktreeInfo, error) {
		return []git.WorktreeInfo{
			{Path: "/tmp/test", Branch: "main", IsMain: true},
			{Path: "/tmp/test-feature", Branch: "feature/foo"},
		}, nil
	}

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "worktrees"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "[active]") {
		t.Errorf("expected [active] marker for running session, got %q", msgs[0].Content)
	}
}

func TestBridge_ListWorktrees_UnconfiguredWorktree(t *testing.T) {
	cfg := testConfig()
	b := New(cfg)

	b.worktreeLister = func(dir string) ([]git.WorktreeInfo, error) {
		return []git.WorktreeInfo{
			{Path: "/tmp/test", Branch: "main", IsMain: true},
			{Path: "/tmp/unmanaged", Branch: "experiment"},
		}, nil
	}

	mockProv := provider.NewMockProvider("discord")
	route := router.Route{Type: router.RouteToBridge, Command: "worktrees"}
	b.handleBridgeCommand(mockProv, "channel-123", route)

	msgs := mockProv.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "not configured") {
		t.Errorf("expected 'not configured' for unmanaged worktree, got %q", msgs[0].Content)
	}
}

func TestBridge_HandleBridgeCommand_Help_IncludesWorktrees(t *testing.T) {
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
	if !strings.Contains(msgs[0].Content, "/worktrees") {
		t.Errorf("expected '/worktrees' in help output, got %q", msgs[0].Content)
	}
}

func testConfigWithWorktrees() *config.Config {
	return &config.Config{
		Repos: map[string]config.RepoConfig{
			"myproject": {
				Provider:   "discord",
				ChannelID:  "channel-100",
				LLM:        "claude",
				WorkingDir: "/tmp/myproject",
			},
			"myproject/feature": {
				Provider:   "discord",
				ChannelID:  "channel-200",
				LLM:        "claude",
				WorkingDir: "/tmp/myproject-feature",
				GitRoot:    "/tmp/myproject",
			},
			"other-repo": {
				Provider:   "terminal",
				ChannelID:  "terminal",
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

func TestBridge_ChannelIDsForProvider_WithWorktreeRepos(t *testing.T) {
	cfg := testConfigWithWorktrees()
	b := New(cfg)

	discordIDs := b.channelIDsForProvider("discord")
	if len(discordIDs) != 2 {
		t.Errorf("expected 2 discord channel IDs, got %d", len(discordIDs))
	}

	found := make(map[string]bool)
	for _, id := range discordIDs {
		found[id] = true
	}
	if !found["channel-100"] || !found["channel-200"] {
		t.Errorf("missing expected discord channel IDs: %v", discordIDs)
	}

	terminalIDs := b.channelIDsForProvider("terminal")
	if len(terminalIDs) != 1 {
		t.Errorf("expected 1 terminal channel ID, got %d", len(terminalIDs))
	}
	if len(terminalIDs) == 1 && terminalIDs[0] != "terminal" {
		t.Errorf("expected terminal channel ID 'terminal', got %q", terminalIDs[0])
	}
}

func TestBridge_RepoForChannel_WorktreeChild(t *testing.T) {
	cfg := testConfigWithWorktrees()
	b := New(cfg)

	repo := b.repoForChannel("channel-100")
	if repo != "myproject" {
		t.Errorf("repoForChannel(channel-100) = %q, want myproject", repo)
	}

	repo = b.repoForChannel("channel-200")
	if repo != "myproject/feature" {
		t.Errorf("repoForChannel(channel-200) = %q, want myproject/feature", repo)
	}

	repo = b.repoForChannel("unknown")
	if repo != "" {
		t.Errorf("repoForChannel(unknown) = %q, want empty string", repo)
	}
}

func TestBridge_HandleLLMMessage_WorktreeChildChannel(t *testing.T) {
	cfg := testConfigWithWorktrees()
	b := New(cfg)

	var capturedWorkDir string
	mockLLM := newMockLLM("claude")
	b.llmFactory = func(backend, workDir, claudePath string, resume bool) (llm.LLM, error) {
		capturedWorkDir = workDir
		return mockLLM, nil
	}

	mockProv := provider.NewMockProvider("discord")
	msg := provider.Message{
		ChannelID: "channel-200",
		Content:   "work on feature",
		Source:    "discord",
	}
	route := router.Route{Type: router.RouteToLLM, Raw: "work on feature"}

	b.handleLLMMessage(context.Background(), mockProv, msg, route)

	// Verify session was created for the worktree child repo
	b.mu.Lock()
	_, hasWorktreeSession := b.repos["myproject/feature"]
	_, hasParentSession := b.repos["myproject"]
	b.mu.Unlock()

	if !hasWorktreeSession {
		t.Errorf("expected session for 'myproject/feature' to be created")
	}
	if hasParentSession {
		t.Errorf("did not expect session for 'myproject' to be created")
	}

	if capturedWorkDir != "/tmp/myproject-feature" {
		t.Errorf("llmFactory workDir = %q, want /tmp/myproject-feature", capturedWorkDir)
	}

	sentMsgs := mockLLM.getSentMessages()
	if len(sentMsgs) != 1 {
		t.Fatalf("expected 1 LLM message, got %d", len(sentMsgs))
	}
}

func TestBridge_Start_WithDiscord_WorktreeChannels(t *testing.T) {
	cfg := testConfigWithWorktrees()
	cfg.Providers.Discord.BotToken = "test-token"

	b := New(cfg)

	var capturedChannelIDs []string
	mockDiscord := provider.NewMockProvider("discord")
	b.discordFactory = func(token string, channelIDs []string) provider.Provider {
		capturedChannelIDs = channelIDs
		return mockDiscord
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := b.Start(ctx)
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}

	if len(capturedChannelIDs) != 2 {
		t.Fatalf("expected 2 channel IDs passed to discord factory, got %d", len(capturedChannelIDs))
	}

	found := make(map[string]bool)
	for _, id := range capturedChannelIDs {
		found[id] = true
	}
	if !found["channel-100"] || !found["channel-200"] {
		t.Errorf("discord factory missing expected channel IDs: %v", capturedChannelIDs)
	}
}
