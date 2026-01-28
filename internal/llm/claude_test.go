package llm

import (
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
