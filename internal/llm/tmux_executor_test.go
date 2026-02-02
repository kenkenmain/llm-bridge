package llm

import (
	"context"
	"strings"
	"testing"
)

func TestNewRealTmuxExecutor(t *testing.T) {
	exec := NewRealTmuxExecutor()
	if exec.TmuxPath != "tmux" {
		t.Errorf("TmuxPath = %q, want %q", exec.TmuxPath, "tmux")
	}
}

func TestRealTmuxExecutor_tmuxPath(t *testing.T) {
	t.Run("empty defaults to tmux", func(t *testing.T) {
		exec := &RealTmuxExecutor{}
		if got := exec.tmuxPath(); got != "tmux" {
			t.Errorf("tmuxPath() = %q, want %q", got, "tmux")
		}
	})

	t.Run("custom path", func(t *testing.T) {
		exec := &RealTmuxExecutor{TmuxPath: "/usr/local/bin/tmux"}
		if got := exec.tmuxPath(); got != "/usr/local/bin/tmux" {
			t.Errorf("tmuxPath() = %q, want %q", got, "/usr/local/bin/tmux")
		}
	})
}

func TestRealTmuxExecutor_runCommand_InvalidBinary(t *testing.T) {
	exec := &RealTmuxExecutor{TmuxPath: "/nonexistent/tmux"}
	_, err := exec.runCommand(context.Background(), "list-sessions")
	if err == nil {
		t.Fatal("runCommand should fail with nonexistent binary")
	}
}

func TestRealTmuxExecutor_NewSession_InvalidBinary(t *testing.T) {
	exec := &RealTmuxExecutor{TmuxPath: "/nonexistent/tmux"}
	err := exec.NewSession(context.Background(), "test-session", "/tmp", "echo hello")
	if err == nil {
		t.Fatal("NewSession should fail with nonexistent binary")
	}
	if !strings.Contains(err.Error(), "create tmux session") {
		t.Errorf("error should mention 'create tmux session', got %q", err.Error())
	}
}

func TestRealTmuxExecutor_HasSession_NonExistent(t *testing.T) {
	exec := NewRealTmuxExecutor()
	exists, err := exec.HasSession("llm-bridge-nonexistent-test-session-xyz")
	if err != nil {
		t.Fatalf("HasSession should not error for non-existent session, got %v", err)
	}
	if exists {
		t.Error("HasSession should return false for non-existent session")
	}
}

func TestRealTmuxExecutor_HasSession_InvalidBinary(t *testing.T) {
	exec := &RealTmuxExecutor{TmuxPath: "/nonexistent/tmux"}
	_, err := exec.HasSession("test-session")
	if err == nil {
		t.Fatal("HasSession should fail with nonexistent binary")
	}
	if !strings.Contains(err.Error(), "check tmux session") {
		t.Errorf("error should mention 'check tmux session', got %q", err.Error())
	}
}

func TestRealTmuxExecutor_KillSession_NonExistent(t *testing.T) {
	exec := NewRealTmuxExecutor()
	err := exec.KillSession("llm-bridge-nonexistent-test-session-xyz")
	if err == nil {
		t.Fatal("KillSession should fail for non-existent session")
	}
	if !strings.Contains(err.Error(), "kill tmux session") {
		t.Errorf("error should mention 'kill tmux session', got %q", err.Error())
	}
}

func TestRealTmuxExecutor_SendKeys_NonExistent(t *testing.T) {
	exec := NewRealTmuxExecutor()
	err := exec.SendKeys("llm-bridge-nonexistent-test-session-xyz", "hello", true)
	if err == nil {
		t.Fatal("SendKeys should fail for non-existent session")
	}
	if !strings.Contains(err.Error(), "send keys") {
		t.Errorf("error should mention 'send keys', got %q", err.Error())
	}
}

func TestRealTmuxExecutor_PipePane_NonExistent(t *testing.T) {
	exec := NewRealTmuxExecutor()
	err := exec.PipePane("llm-bridge-nonexistent-test-session-xyz", "cat >> /dev/null")
	if err == nil {
		t.Fatal("PipePane should fail for non-existent session")
	}
	if !strings.Contains(err.Error(), "pipe-pane") {
		t.Errorf("error should mention 'pipe-pane', got %q", err.Error())
	}
}

func TestRealTmuxExecutor_ListSessions(t *testing.T) {
	exec := NewRealTmuxExecutor()
	// ListSessions should not error even when no tmux server is running.
	// It returns nil for "no server running" responses.
	sessions, err := exec.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions should not error: %v", err)
	}
	// sessions may be nil or contain existing sessions — just verify no crash
	_ = sessions
}

func TestRealTmuxExecutor_Integration(t *testing.T) {
	exec := NewRealTmuxExecutor()
	sessionName := "llm-bridge-test-executor-integration"

	// Clean up any leftover session
	_ = exec.KillSession(sessionName)

	// Create a session running a simple command
	err := exec.NewSession(context.Background(), sessionName, "/tmp", "cat")
	if err != nil {
		t.Fatalf("NewSession error = %v", err)
	}
	defer func() { _ = exec.KillSession(sessionName) }()

	// Verify session exists
	exists, err := exec.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession error = %v", err)
	}
	if !exists {
		t.Fatal("session should exist after creation")
	}

	// Session should appear in ListSessions
	sessions, err := exec.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions error = %v", err)
	}
	found := false
	for _, s := range sessions {
		if s == sessionName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListSessions should contain %q, got %v", sessionName, sessions)
	}

	// SendKeys should work (literal)
	if err := exec.SendKeys(sessionName, "hello", true); err != nil {
		t.Errorf("SendKeys literal error = %v", err)
	}

	// SendKeys should work (non-literal, e.g. Enter)
	if err := exec.SendKeys(sessionName, "Enter", false); err != nil {
		t.Errorf("SendKeys non-literal error = %v", err)
	}

	// PipePane should work
	if err := exec.PipePane(sessionName, "cat >> /dev/null"); err != nil {
		t.Errorf("PipePane error = %v", err)
	}

	// Kill session
	if err := exec.KillSession(sessionName); err != nil {
		t.Errorf("KillSession error = %v", err)
	}

	// Verify session is gone
	exists, err = exec.HasSession(sessionName)
	if err != nil {
		t.Fatalf("HasSession after kill error = %v", err)
	}
	if exists {
		t.Error("session should not exist after kill")
	}
}
