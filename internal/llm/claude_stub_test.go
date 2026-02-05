package llm

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestClaude_WithStub tests Claude using a stub script instead of real CLI.
// Requires tmux to be installed.
func TestClaude_WithStub(t *testing.T) {
	// Check tmux is available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available, skipping stub test")
	}

	// Create stub script in temp dir
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	stubScript := `#!/bin/bash
# Simple stub that echoes input
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    echo "[STUB] $line"
done
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithWorkingDir(tmpDir),
		WithClaudePath(stubPath),
		WithResume(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := c.Stop(); err != nil {
			t.Errorf("Stop() error = %v", err)
		}
	})

	if !c.Running() {
		t.Fatal("expected Running() to be true after Start()")
	}

	// Send a message
	err := c.Send(Message{Source: "test", Content: "hello world"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Read output
	output := c.Output()
	buf := make([]byte, 1024)
	
	// Give stub time to respond
	time.Sleep(200 * time.Millisecond)
	
	n, err := output.Read(buf)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, "[STUB]") {
		t.Errorf("expected stub response, got: %q", response)
	}
	if !strings.Contains(response, "hello world") {
		t.Errorf("expected echoed input in response, got: %q", response)
	}
}
