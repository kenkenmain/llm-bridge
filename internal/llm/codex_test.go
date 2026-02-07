package llm

import (
	"context"
	"testing"
	"time"
)

func TestNewCodex_Defaults(t *testing.T) {
	c := NewCodex()

	if c.workingDir != "." {
		t.Errorf("workingDir = %q, want %q", c.workingDir, ".")
	}
	if c.resumeSession != true {
		t.Errorf("resumeSession = %v, want true", c.resumeSession)
	}
	if c.codexPath != "codex" {
		t.Errorf("codexPath = %q, want %q", c.codexPath, "codex")
	}
	if c.running != false {
		t.Errorf("running = %v, want false", c.running)
	}
}

func TestNewCodex_WithOptions(t *testing.T) {
	c := NewCodex(
		WithCodexWorkingDir("/tmp/test"),
		WithCodexResume(false),
		WithCodexPath("/usr/local/bin/codex"),
	)

	if c.workingDir != "/tmp/test" {
		t.Errorf("workingDir = %q, want %q", c.workingDir, "/tmp/test")
	}
	if c.resumeSession != false {
		t.Errorf("resumeSession = %v, want false", c.resumeSession)
	}
	if c.codexPath != "/usr/local/bin/codex" {
		t.Errorf("codexPath = %q, want %q", c.codexPath, "/usr/local/bin/codex")
	}
}

func TestWithCodexPath_EmptyIgnored(t *testing.T) {
	c := NewCodex(WithCodexPath(""))
	if c.codexPath != "codex" {
		t.Errorf("codexPath = %q, want %q (default)", c.codexPath, "codex")
	}
}

func TestCodex_Name(t *testing.T) {
	c := NewCodex()
	if name := c.Name(); name != "codex" {
		t.Errorf("Name() = %q, want %q", name, "codex")
	}
}

func TestCodex_Start_WithRealProcess(t *testing.T) {
	c := NewCodex(
		WithCodexPath("cat"),
		WithCodexWorkingDir("/tmp"),
		WithCodexResume(false),
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

	err := c.Send(Message{Content: "hello"})
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	out := c.Output()
	if out == nil {
		t.Error("Output() should not be nil when running")
	}
}

func TestCodex_Start_InvalidCommand(t *testing.T) {
	c := NewCodex(WithCodexPath("/nonexistent/command"), WithCodexResume(false))

	ctx := context.Background()
	err := c.Start(ctx)
	if err == nil {
		t.Error("Start() should error for invalid command")
		_ = c.Stop()
	}
}

func TestCodex_Stop_DoubleStop(t *testing.T) {
	c := NewCodex(WithCodexPath("cat"), WithCodexResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("first Stop() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestCodex_Send_NotRunning(t *testing.T) {
	c := NewCodex()
	err := c.Send(Message{Content: "test"})
	if err == nil {
		t.Error("Send() should return error when not running")
	}
}

func TestCodex_Running_NotStarted(t *testing.T) {
	c := NewCodex()
	if c.Running() != false {
		t.Error("Running() should be false when not started")
	}
}

func TestCodex_LastActivity(t *testing.T) {
	before := time.Now()
	c := NewCodex()
	after := time.Now()

	activity := c.LastActivity()
	if activity.Before(before) || activity.After(after) {
		t.Errorf("LastActivity() = %v, should be between %v and %v", activity, before, after)
	}
}

func TestCodex_UpdateActivity(t *testing.T) {
	c := NewCodex()
	initial := c.LastActivity()

	time.Sleep(10 * time.Millisecond)
	c.UpdateActivity()

	updated := c.LastActivity()
	if !updated.After(initial) {
		t.Errorf("UpdateActivity() should advance timestamp, got %v <= %v", updated, initial)
	}
}

func TestCodex_Output_WhenNotRunning(t *testing.T) {
	c := NewCodex()
	_ = c.Output() // Just verify it doesn't panic
}

func TestCodex_Stop_NotRunning(t *testing.T) {
	c := NewCodex()
	if err := c.Stop(); err != nil {
		t.Errorf("Stop() when not running should not error, got %v", err)
	}
}

func TestCodex_Cancel_NotRunning(t *testing.T) {
	c := NewCodex()
	if err := c.Cancel(); err != nil {
		t.Errorf("Cancel() when not running should not error, got %v", err)
	}
}

func TestCodex_Start_AlreadyRunning(t *testing.T) {
	c := NewCodex(WithCodexPath("cat"), WithCodexResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if err := c.Start(ctx); err != nil {
		t.Errorf("second Start() error = %v", err)
	}
}

func TestCodex_Stop_Running(t *testing.T) {
	c := NewCodex(WithCodexPath("cat"), WithCodexResume(false))

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

func TestCodex_Send_Running(t *testing.T) {
	c := NewCodex(WithCodexPath("cat"), WithCodexResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

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

func TestCodex_Cancel_Running(t *testing.T) {
	c := NewCodex(WithCodexPath("cat"), WithCodexResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if err := c.Cancel(); err != nil {
		t.Errorf("Cancel() error = %v", err)
	}
}

func TestCodex_WithResumeFlag(t *testing.T) {
	c := NewCodex(WithCodexResume(true))
	if !c.resumeSession {
		t.Error("resumeSession should be true")
	}

	c = NewCodex(WithCodexResume(false))
	if c.resumeSession {
		t.Error("resumeSession should be false")
	}
}

func TestCodex_ProcessExitsCleanup(t *testing.T) {
	c := NewCodex(WithCodexPath("echo"), WithCodexResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if c.Running() {
		t.Error("Running() should be false after process exits")
	}

	if err := c.Stop(); err != nil {
		t.Errorf("Stop() after process exit error = %v", err)
	}
}

func TestCodex_RestartAfterStop(t *testing.T) {
	c := NewCodex(WithCodexPath("cat"), WithCodexResume(false))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}

	if err := c.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if err := c.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	if !c.Running() {
		t.Error("Running() should be true after restart")
	}
}
