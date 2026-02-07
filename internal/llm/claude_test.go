package llm

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewClaude_Defaults(t *testing.T) {
	c := NewClaude()

	if c.workingDir != "." {
		t.Errorf("workingDir = %q, want %q", c.workingDir, ".")
	}
	if c.resumeSession != true {
		t.Errorf("resumeSession = %v, want true", c.resumeSession)
	}
	if c.claudePath != "claude" {
		t.Errorf("claudePath = %q, want %q", c.claudePath, "claude")
	}
	if c.running != false {
		t.Errorf("running = %v, want false", c.running)
	}
	if c.pipeReader != nil {
		t.Error("pipeReader should be nil before Start()")
	}
	if c.pipeWriter != nil {
		t.Error("pipeWriter should be nil before Start()")
	}
	if c.sessionID != "" {
		t.Errorf("sessionID = %q, want empty", c.sessionID)
	}
}

func TestNewClaude_WithOptions(t *testing.T) {
	c := NewClaude(
		WithWorkingDir("/tmp/test"),
		WithResume(false),
		WithClaudePath("/usr/local/bin/claude"),
	)

	if c.workingDir != "/tmp/test" {
		t.Errorf("workingDir = %q, want %q", c.workingDir, "/tmp/test")
	}
	if c.resumeSession != false {
		t.Errorf("resumeSession = %v, want false", c.resumeSession)
	}
	if c.claudePath != "/usr/local/bin/claude" {
		t.Errorf("claudePath = %q, want %q", c.claudePath, "/usr/local/bin/claude")
	}
}

func TestWithClaudePath_EmptyIgnored(t *testing.T) {
	c := NewClaude(WithClaudePath(""))
	if c.claudePath != "claude" {
		t.Errorf("claudePath = %q, want %q (default)", c.claudePath, "claude")
	}
}

func TestClaude_Name(t *testing.T) {
	c := NewClaude()
	if name := c.Name(); name != "claude" {
		t.Errorf("Name() = %q, want %q", name, "claude")
	}
}

func TestClaude_Running_NotStarted(t *testing.T) {
	c := NewClaude()
	if c.Running() != false {
		t.Error("Running() should be false when not started")
	}
}

func TestClaude_LastActivity(t *testing.T) {
	before := time.Now()
	c := NewClaude()
	after := time.Now()

	activity := c.LastActivity()
	if activity.Before(before) || activity.After(after) {
		t.Errorf("LastActivity() = %v, should be between %v and %v", activity, before, after)
	}
}

func TestClaude_UpdateActivity(t *testing.T) {
	c := NewClaude()
	initial := c.LastActivity()

	time.Sleep(10 * time.Millisecond)
	c.UpdateActivity()

	updated := c.LastActivity()
	if !updated.After(initial) {
		t.Errorf("UpdateActivity() should advance timestamp, got %v <= %v", updated, initial)
	}
}

func TestClaude_Send_NotRunning(t *testing.T) {
	c := NewClaude()
	err := c.Send(Message{Content: "test"})
	if err == nil {
		t.Error("Send() should return error when not running")
	}
}

func TestClaude_Output_WhenNotRunning(t *testing.T) {
	c := NewClaude()
	// When not running, pipeReader is nil. Output() returns it wrapped in
	// io.Reader interface (non-nil interface with nil concrete value).
	out := c.Output()
	_ = out // Just verify it doesn't panic
}

func TestClaude_Stop_NotRunning(t *testing.T) {
	c := NewClaude()
	if err := c.Stop(); err != nil {
		t.Errorf("Stop() when not running should not error, got %v", err)
	}
}

func TestClaude_Cancel_NotRunning(t *testing.T) {
	c := NewClaude()
	if err := c.Cancel(); err != nil {
		t.Errorf("Cancel() when not running should not error, got %v", err)
	}
}

func TestClaude_Start_CreatesPipe(t *testing.T) {
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if !c.Running() {
		t.Error("Running() should be true after Start()")
	}

	if c.pipeReader == nil {
		t.Error("pipeReader should be non-nil after Start()")
	}
	if c.pipeWriter == nil {
		t.Error("pipeWriter should be non-nil after Start()")
	}
	if c.sessionID != "" {
		t.Errorf("sessionID should be empty after Start(), got %q", c.sessionID)
	}

	out := c.Output()
	if out == nil {
		t.Error("Output() should not be nil when running")
	}
}

func TestClaude_Start_AlreadyRunning(t *testing.T) {
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	// Second start should be no-op
	if err := c.Start(ctx); err != nil {
		t.Errorf("second Start() error = %v", err)
	}
}

func TestClaude_Start_AlwaysSucceeds(t *testing.T) {
	// With process-per-message model, Start() only creates a pipe
	// and never fails, even for invalid commands (failure happens at Send time)
	c := NewClaude(WithClaudePath("/nonexistent/command"), WithResume(false))

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Errorf("Start() should always succeed since it only creates a pipe, got %v", err)
	}
	defer func() { _ = c.Stop() }()

	if !c.Running() {
		t.Error("Running() should be true after Start()")
	}
}

func TestClaude_Send_InvalidCommand(t *testing.T) {
	// With process-per-message model, invalid commands fail at Send() time
	c := NewClaude(WithClaudePath("/nonexistent/command"), WithResume(false))

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	err := c.Send(Message{Content: "test"})
	if err == nil {
		t.Error("Send() should error for invalid command")
	}
}

func TestClaude_Stop_Running(t *testing.T) {
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if c.Running() {
		t.Error("Running() should be false after Stop()")
	}
}

func TestClaude_Send_SpawnsSubprocess(t *testing.T) {
	// Use a stub that outputs NDJSON to stdout
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	stubScript := `#!/bin/bash
# Read stdin (the message) and echo it as NDJSON
MSG=$(cat)
echo "{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"echo: $MSG\"}}"
echo "{\"type\":\"result\",\"session_id\":\"test-sess-001\"}"
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithClaudePath(stubPath),
		WithWorkingDir(tmpDir),
		WithResume(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	before := c.LastActivity()
	time.Sleep(10 * time.Millisecond)

	if err := c.Send(Message{Content: "hello"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	after := c.LastActivity()
	if !after.After(before) {
		t.Error("Send() should update LastActivity")
	}

	// Read output from the pipe
	output := c.Output()
	buf := make([]byte, 4096)

	// Wait for subprocess to complete and data to flow
	time.Sleep(300 * time.Millisecond)

	n, err := output.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, "content_block_delta") {
		t.Errorf("expected NDJSON output, got: %q", response)
	}
	if !strings.Contains(response, "echo: hello") {
		t.Errorf("expected echoed message in output, got: %q", response)
	}
}

func TestClaude_SessionID_Extraction(t *testing.T) {
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	stubScript := `#!/bin/bash
MSG=$(cat)
echo "{\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"response\"}}"
echo "{\"type\":\"result\",\"session_id\":\"extracted-session-42\"}"
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithClaudePath(stubPath),
		WithWorkingDir(tmpDir),
		WithResume(true),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if err := c.Send(Message{Content: "test"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Must read from the pipe to unblock the forwarding goroutine
	// (io.Pipe has no buffer — writes block until read)
	output := c.Output()
	buf := make([]byte, 4096)
	readDone := make(chan struct{})
	go func() {
		for {
			n, err := output.Read(buf)
			if n > 0 && strings.Contains(string(buf[:n]), "extracted-session-42") {
				close(readDone)
				return
			}
			if err != nil {
				close(readDone)
				return
			}
		}
	}()

	select {
	case <-readDone:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out reading output")
	}

	// Give the goroutine a moment to store the session ID
	time.Sleep(100 * time.Millisecond)

	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()

	if sid != "extracted-session-42" {
		t.Errorf("sessionID = %q, want %q", sid, "extracted-session-42")
	}
}

func TestClaude_Cancel_Running(t *testing.T) {
	// Use sleep as a long-running subprocess
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	stubScript := `#!/bin/bash
cat > /dev/null  # Read stdin
sleep 60
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithClaudePath(stubPath),
		WithWorkingDir(tmpDir),
		WithResume(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if err := c.Send(Message{Content: "test"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Give subprocess time to start
	time.Sleep(100 * time.Millisecond)

	if err := c.Cancel(); err != nil {
		t.Errorf("Cancel() error = %v", err)
	}
}

func TestClaude_WithResumeFlag(t *testing.T) {
	c := NewClaude(WithResume(true))
	if !c.resumeSession {
		t.Error("resumeSession should be true")
	}

	c = NewClaude(WithResume(false))
	if c.resumeSession {
		t.Error("resumeSession should be false")
	}
}

func TestClaude_Stop_DoubleStop(t *testing.T) {
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("first Stop() error = %v", err)
	}

	// Second stop should be a no-op because running is already false
	if err := c.Stop(); err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestClaude_Stop_ClosesReader(t *testing.T) {
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	out := c.Output()

	if err := c.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// Reading from Output() should return EOF after Stop()
	buf := make([]byte, 128)
	_, err := out.Read(buf)
	if err != io.EOF {
		t.Errorf("Read() after Stop() should return io.EOF, got %v", err)
	}
}

func TestClaude_Send_RejectsConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	stubScript := `#!/bin/bash
cat > /dev/null
sleep 60
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithClaudePath(stubPath),
		WithWorkingDir(tmpDir),
		WithResume(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	// First Send starts the subprocess
	if err := c.Send(Message{Content: "first"}); err != nil {
		t.Fatalf("first Send() error = %v", err)
	}

	// Give subprocess time to start
	time.Sleep(100 * time.Millisecond)

	// Second Send should be rejected while first is still processing
	err := c.Send(Message{Content: "second"})
	if err == nil {
		t.Error("Send() should reject concurrent calls")
	}
	if err != nil && !strings.Contains(err.Error(), "previous message still processing") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClaude_RestartAfterStop(t *testing.T) {
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Second start should work because Start() creates a fresh io.Pipe
	if err := c.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if !c.Running() {
		t.Error("Running() should be true after restart")
	}
}
