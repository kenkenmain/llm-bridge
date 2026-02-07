package llm

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// drainPipeUntil reads from the pipe in a goroutine, collecting output
// until the collected text contains the expected substring or timeout.
func drainPipeUntil(c *Claude, contains string, timeout time.Duration) (string, bool) {
	output := c.Output()
	buf := make([]byte, 8192)
	var collected string
	done := make(chan struct{})

	go func() {
		for {
			n, err := output.Read(buf)
			if n > 0 {
				collected += string(buf[:n])
			}
			if strings.Contains(collected, contains) {
				close(done)
				return
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		return collected, true
	case <-time.After(timeout):
		return collected, false
	}
}

// TestClaude_WithStub tests Claude using a stub script that emits NDJSON
// matching `claude -p --output-format stream-json` behavior.
func TestClaude_WithStub(t *testing.T) {
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	stubScript := `#!/bin/bash
# Simulate claude -p --output-format stream-json
# Read message from stdin
MSG=$(cat)

# Emit NDJSON stream events
echo '{"type":"message_start","message":{"id":"msg_stub_001"}}'
echo '{"type":"content_block_start","index":0}'
echo "{\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"[STUB] $MSG\"}}"
echo '{"type":"content_block_stop","index":0}'
echo '{"type":"message_delta","delta":{"stop_reason":"end_turn"}}'
echo '{"type":"message_stop"}'
echo '{"type":"result","session_id":"stub-session-001"}'
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithWorkingDir(tmpDir),
		WithClaudePath(stubPath),
		WithResume(true),
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

	// Read output — must drain pipe concurrently to unblock the forwarding goroutine
	// Wait for the result event which is the last line
	response, ok := drainPipeUntil(c, "stub-session-001", 3*time.Second)
	if !ok {
		t.Fatalf("timed out reading output, got so far: %q", response)
	}

	// Verify NDJSON output contains expected events
	if !strings.Contains(response, "content_block_delta") {
		t.Errorf("expected content_block_delta event, got: %q", response)
	}
	if !strings.Contains(response, "[STUB] hello world") {
		t.Errorf("expected echoed input in response, got: %q", response)
	}

	// Verify session ID was extracted (it's in the last line we waited for)
	// Give the goroutine a moment to process the session ID
	time.Sleep(100 * time.Millisecond)
	c.mu.Lock()
	sid := c.sessionID
	c.mu.Unlock()

	if sid != "stub-session-001" {
		t.Errorf("sessionID = %q, want %q", sid, "stub-session-001")
	}
}

// TestClaude_WithStub_SessionResume verifies that the -r flag is passed
// when a session ID has been extracted from a previous response.
func TestClaude_WithStub_SessionResume(t *testing.T) {
	tmpDir := t.TempDir()
	stubPath := filepath.Join(tmpDir, "claude-stub")
	// This stub echoes all args to stdout as NDJSON, so we can verify -r was passed
	stubScript := `#!/bin/bash
MSG=$(cat)
ARGS="$@"
echo "{\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"args: $ARGS\"}}"
echo '{"type":"result","session_id":"stub-session-002"}'
`
	if err := os.WriteFile(stubPath, []byte(stubScript), 0755); err != nil {
		t.Fatalf("failed to write stub: %v", err)
	}

	c := NewClaude(
		WithWorkingDir(tmpDir),
		WithClaudePath(stubPath),
		WithResume(true),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = c.Stop() }()

	// First message — no session ID yet
	if err := c.Send(Message{Content: "first"}); err != nil {
		t.Fatalf("first Send() error = %v", err)
	}

	// Drain first response — must read to unblock the pipe
	response1, ok := drainPipeUntil(c, "stub-session-002", 3*time.Second)
	if !ok {
		t.Fatalf("timed out reading first response, got: %q", response1)
	}

	// Wait for activeCmd to be cleared after subprocess exits
	deadline := time.After(3 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("activeCmd not cleared after first subprocess")
		default:
		}
		c.mu.Lock()
		done := c.activeCmd == nil
		c.mu.Unlock()
		if done {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Second message — should include -r stub-session-002
	if err := c.Send(Message{Content: "second"}); err != nil {
		t.Fatalf("second Send() error = %v", err)
	}

	// Read second response
	response2, ok := drainPipeUntil(c, "stub-session-002", 3*time.Second)
	if !ok {
		t.Fatalf("timed out reading second response, got: %q", response2)
	}

	if !strings.Contains(response2, "-r") {
		t.Errorf("expected -r flag in args for resumed session, got: %q", response2)
	}
	if !strings.Contains(response2, "stub-session-002") {
		t.Errorf("expected session ID in args, got: %q", response2)
	}
}
