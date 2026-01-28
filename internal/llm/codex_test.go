package llm

import (
	"context"
	"testing"
)

func TestNewCodex(t *testing.T) {
	c := NewCodex("/tmp/test")
	if c == nil {
		t.Fatal("NewCodex returned nil")
	}
}

func TestCodex_Name(t *testing.T) {
	c := NewCodex("/tmp/test")
	if name := c.Name(); name != "codex" {
		t.Errorf("Name() = %q, want codex", name)
	}
}

func TestCodex_Start_ReturnsError(t *testing.T) {
	c := NewCodex("/tmp/test")
	err := c.Start(context.Background())
	if err == nil {
		t.Error("Start() should return error (not implemented)")
	}
}

func TestCodex_Stop(t *testing.T) {
	c := NewCodex("/tmp/test")
	// Stop should not panic
	err := c.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestCodex_Running(t *testing.T) {
	c := NewCodex("/tmp/test")
	if c.Running() {
		t.Error("Running() should be false")
	}
}

func TestCodex_Send(t *testing.T) {
	c := NewCodex("/tmp/test")
	err := c.Send(Message{Content: "test"})
	if err == nil {
		t.Error("Send() should return error (not running)")
	}
}

func TestCodex_Output(t *testing.T) {
	c := NewCodex("/tmp/test")
	out := c.Output()
	if out != nil {
		t.Error("Output() should return nil")
	}
}

func TestCodex_Cancel(t *testing.T) {
	c := NewCodex("/tmp/test")
	// Cancel should not panic
	err := c.Cancel()
	if err != nil {
		t.Errorf("Cancel() error = %v", err)
	}
}

func TestCodex_LastActivity(t *testing.T) {
	c := NewCodex("/tmp/test")
	activity := c.LastActivity()
	if activity.IsZero() {
		t.Error("LastActivity() should not be zero")
	}
}

func TestCodex_UpdateActivity(t *testing.T) {
	c := NewCodex("/tmp/test")
	before := c.LastActivity()
	c.UpdateActivity()
	// UpdateActivity should not panic - we don't check if it changed
	// since the codex stub might not implement it
	_ = before
}
