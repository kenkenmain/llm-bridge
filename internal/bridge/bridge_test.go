package bridge

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/llm-bridge/internal/config"
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

func TestBridge_ProcessMessage_ExclamationCommand(t *testing.T) {
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
		Content:   "!commit",
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
