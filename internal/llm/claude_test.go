package llm

import (
	"context"
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
	// When not running, ptmx is nil
	// Output() returns it wrapped in io.Reader interface
	// Due to Go's interface semantics, (*os.File)(nil) as io.Reader is non-nil
	// but has nil concrete value - this is expected behavior
	_ = c.Output() // Just verify it doesn't panic
}

func TestClaude_Stop_NotRunning(t *testing.T) {
	c := NewClaude()
	// Stop should be safe when not running
	if err := c.Stop(); err != nil {
		t.Errorf("Stop() when not running should not error, got %v", err)
	}
}

func TestClaude_Cancel_NotRunning(t *testing.T) {
	c := NewClaude()
	// Cancel should be safe when not running
	if err := c.Cancel(); err != nil {
		t.Errorf("Cancel() when not running should not error, got %v", err)
	}
}

func TestClaude_Start_WithRealProcess(t *testing.T) {
	// Use 'cat' as a simple process that reads stdin and writes to stdout
	c := NewClaude(
		WithClaudePath("cat"),
		WithWorkingDir("/tmp"),
		WithResume(false), // cat doesn't support --resume
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if !c.Running() {
		t.Error("Running() should be true after Start")
	}

	// Send a message
	err := c.Send(Message{Content: "hello"})
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	// Output should be readable
	out := c.Output()
	if out == nil {
		t.Error("Output() should not be nil when running")
	}
}

func TestClaude_Start_AlreadyRunning(t *testing.T) {
	c := NewClaude(WithClaudePath("cat"), WithResume(false))

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

func TestClaude_Start_InvalidCommand(t *testing.T) {
	c := NewClaude(WithClaudePath("/nonexistent/command"), WithResume(false))

	ctx := context.Background()
	err := c.Start(ctx)
	if err == nil {
		t.Error("Start() should error for invalid command")
		_ = c.Stop()
	}
}

func TestClaude_Stop_Running(t *testing.T) {
	c := NewClaude(WithClaudePath("cat"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if c.Running() {
		t.Error("Running() should be false after Stop")
	}
}

func TestClaude_Send_Running(t *testing.T) {
	c := NewClaude(WithClaudePath("cat"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	// Send should update activity
	before := c.LastActivity()
	time.Sleep(10 * time.Millisecond)

	if err := c.Send(Message{Content: "test message"}); err != nil {
		t.Errorf("Send() error = %v", err)
	}

	after := c.LastActivity()
	if !after.After(before) {
		t.Error("Send should update LastActivity")
	}
}

func TestClaude_Cancel_Running(t *testing.T) {
	// Use 'cat' which responds to signals
	c := NewClaude(WithClaudePath("cat"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	// Cancel should send SIGINT
	if err := c.Cancel(); err != nil {
		t.Errorf("Cancel() error = %v", err)
	}
}

func TestClaude_WithResumeFlag(t *testing.T) {
	// Test that --resume flag is added when resumeSession is true
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
	c := NewClaude(WithClaudePath("cat"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// First stop
	if err := c.Stop(); err != nil {
		t.Errorf("first Stop() error = %v", err)
	}

	// Second stop should not panic due to sync.Once
	if err := c.Stop(); err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestClaude_ProcessExitsCleanup(t *testing.T) {
	// Use 'echo' which exits immediately after printing
	c := NewClaude(WithClaudePath("echo"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for process to exit naturally
	time.Sleep(100 * time.Millisecond)

	// Running should be false after process exits
	if c.Running() {
		t.Error("Running() should be false after process exits")
	}

	// Stop should still be safe to call (tests closeOnce)
	if err := c.Stop(); err != nil {
		t.Errorf("Stop() after process exit error = %v", err)
	}
}

func TestClaude_RestartAfterStop(t *testing.T) {
	c := NewClaude(WithClaudePath("cat"), WithResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// First start
	if err := c.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	// Stop
	if err := c.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Second start should work (closeOnce should be reset)
	if err := c.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if !c.Running() {
		t.Error("Running() should be true after restart")
	}
}
