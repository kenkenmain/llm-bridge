package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestNewTmuxClaude_Defaults(t *testing.T) {
	tc := NewTmuxClaude("my-repo")

	if tc.repoName != "my-repo" {
		t.Errorf("repoName = %q, want %q", tc.repoName, "my-repo")
	}
	if tc.sessionName != "llm-bridge-my-repo" {
		t.Errorf("sessionName = %q, want %q", tc.sessionName, "llm-bridge-my-repo")
	}
	if tc.workingDir != "." {
		t.Errorf("workingDir = %q, want %q", tc.workingDir, ".")
	}
	if tc.resumeSession != true {
		t.Errorf("resumeSession = %v, want true", tc.resumeSession)
	}
	if tc.claudePath != "claude" {
		t.Errorf("claudePath = %q, want %q", tc.claudePath, "claude")
	}
	if tc.executor == nil {
		t.Error("executor should not be nil")
	}
	if tc.running != false {
		t.Errorf("running = %v, want false", tc.running)
	}
}

func TestNewTmuxClaude_SessionNameSanitized(t *testing.T) {
	tests := []struct {
		repoName    string
		wantSession string
	}{
		{"my-repo", "llm-bridge-my-repo"},
		{"org/repo", "llm-bridge-org-repo"},
		{"my.repo.name", "llm-bridge-my-repo-name"},
		{"repo@v2", "llm-bridge-repo-v2"},
	}

	for _, tt := range tests {
		t.Run(tt.repoName, func(t *testing.T) {
			tc := NewTmuxClaude(tt.repoName)
			if tc.sessionName != tt.wantSession {
				t.Errorf("sessionName = %q, want %q", tc.sessionName, tt.wantSession)
			}
		})
	}
}

func TestNewTmuxClaude_WithOptions(t *testing.T) {
	mock := &MockTmuxExecutor{}
	tc := NewTmuxClaude("test-repo",
		WithTmuxWorkingDir("/tmp/test"),
		WithTmuxResume(false),
		WithTmuxClaudePath("/usr/local/bin/claude"),
		WithTmuxExecutor(mock),
	)

	if tc.workingDir != "/tmp/test" {
		t.Errorf("workingDir = %q, want %q", tc.workingDir, "/tmp/test")
	}
	if tc.resumeSession != false {
		t.Errorf("resumeSession = %v, want false", tc.resumeSession)
	}
	if tc.claudePath != "/usr/local/bin/claude" {
		t.Errorf("claudePath = %q, want %q", tc.claudePath, "/usr/local/bin/claude")
	}
	if tc.executor != mock {
		t.Error("executor should be the mock")
	}
}

func TestWithTmuxClaudePath_EmptyIgnored(t *testing.T) {
	tc := NewTmuxClaude("repo", WithTmuxClaudePath(""))
	if tc.claudePath != "claude" {
		t.Errorf("claudePath = %q, want %q (default)", tc.claudePath, "claude")
	}
}

func TestTmuxClaude_Name(t *testing.T) {
	tc := NewTmuxClaude("repo")
	if name := tc.Name(); name != "claude-tmux" {
		t.Errorf("Name() = %q, want %q", name, "claude-tmux")
	}
}

func TestTmuxClaude_Running_NotStarted(t *testing.T) {
	tc := NewTmuxClaude("repo")
	if tc.Running() {
		t.Error("Running() should be false when not started")
	}
}

func TestTmuxClaude_LastActivity(t *testing.T) {
	before := time.Now()
	tc := NewTmuxClaude("repo")
	after := time.Now()

	activity := tc.LastActivity()
	if activity.Before(before) || activity.After(after) {
		t.Errorf("LastActivity() = %v, should be between %v and %v", activity, before, after)
	}
}

func TestTmuxClaude_UpdateActivity(t *testing.T) {
	tc := NewTmuxClaude("repo")
	initial := tc.LastActivity()

	time.Sleep(10 * time.Millisecond)
	tc.UpdateActivity()

	updated := tc.LastActivity()
	if !updated.After(initial) {
		t.Errorf("UpdateActivity() should advance timestamp, got %v <= %v", updated, initial)
	}
}

func TestTmuxClaude_DefaultExecutorIsReal(t *testing.T) {
	tc := NewTmuxClaude("repo")
	if _, ok := tc.executor.(*RealTmuxExecutor); !ok {
		t.Errorf("default executor should be *RealTmuxExecutor, got %T", tc.executor)
	}
}

// --- Start method tests ---

func TestTmuxClaude_Start_AlreadyRunning(t *testing.T) {
	mock := &MockTmuxExecutor{}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true

	err := tc.Start(context.Background())
	if err != nil {
		t.Errorf("Start() when already running should return nil, got %v", err)
	}
}

func TestTmuxClaude_Start_NewSessionError(t *testing.T) {
	mock := &MockTmuxExecutor{
		NewSessionFn: func(ctx context.Context, sessionName, workingDir, command string) error {
			return fmt.Errorf("tmux not available")
		},
	}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Start(context.Background())
	if err == nil {
		t.Fatal("Start() should return error when NewSession fails")
	}
	if !strings.Contains(err.Error(), "create tmux session") {
		t.Errorf("error should mention 'create tmux session', got %q", err.Error())
	}
	if tc.Running() {
		t.Error("Running() should be false after failed Start")
	}
}

func TestTmuxClaude_Start_PipePaneError(t *testing.T) {
	killCalled := false
	mock := &MockTmuxExecutor{
		PipePaneFn: func(sessionName, command string) error {
			return fmt.Errorf("pipe-pane failed")
		},
		KillSessionFn: func(sessionName string) error {
			killCalled = true
			return nil
		},
	}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Start(context.Background())
	if err == nil {
		t.Fatal("Start() should return error when PipePane fails")
	}
	if !strings.Contains(err.Error(), "setup pipe-pane") {
		t.Errorf("error should mention 'setup pipe-pane', got %q", err.Error())
	}
	if !killCalled {
		t.Error("KillSession should be called to clean up on PipePane failure")
	}
}

func TestTmuxClaude_Start_ResumeFlag(t *testing.T) {
	var capturedCmd string
	mock := &MockTmuxExecutor{
		NewSessionFn: func(ctx context.Context, sessionName, workingDir, command string) error {
			capturedCmd = command
			return nil
		},
		PipePaneFn: func(sessionName, command string) error {
			return fmt.Errorf("stop here") // stop before FIFO
		},
	}

	// With resume
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock), WithTmuxResume(true))
	_ = tc.Start(context.Background())
	if !strings.Contains(capturedCmd, "--resume") {
		t.Errorf("command should contain --resume when resumeSession=true, got %q", capturedCmd)
	}

	// Without resume
	tc = NewTmuxClaude("repo", WithTmuxExecutor(mock), WithTmuxResume(false))
	_ = tc.Start(context.Background())
	if strings.Contains(capturedCmd, "--resume") {
		t.Errorf("command should not contain --resume when resumeSession=false, got %q", capturedCmd)
	}
}


func TestTmuxClaude_Start_Success(t *testing.T) {
	// This test verifies the full Start flow using a mock executor.
	// The mock PipePane triggers a goroutine that opens the FIFO for writing
	// (unblocking the read-open in Start).
	var capturedSessionName string
	var capturedPipeCmd string
	mock := &MockTmuxExecutor{
		NewSessionFn: func(ctx context.Context, sessionName, workingDir, command string) error {
			capturedSessionName = sessionName
			return nil
		},
		PipePaneFn: func(sessionName, command string) error {
			capturedPipeCmd = command
			// Open the FIFO for writing in a goroutine to unblock the read side.
			// We need to extract the FIFO path from the command.
			parts := strings.Fields(command)
			if len(parts) >= 3 {
				fifoPath := parts[2]
				go func() {
					// Small delay to simulate real pipe-pane behavior
					time.Sleep(50 * time.Millisecond)
					f, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
					if err == nil {
						_ = f.Close()
					}
				}()
			}
			return nil
		},
		HasSessionFn: func(sessionName string) (bool, error) {
			return true, nil
		},
	}

	tc := NewTmuxClaude("start-test", WithTmuxExecutor(mock), WithTmuxWorkingDir("/tmp"))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := tc.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() { _ = tc.Stop() }()

	if !tc.Running() {
		t.Error("Running() should be true after successful Start")
	}
	if capturedSessionName == "" {
		t.Error("NewSession should have been called")
	}
	if capturedPipeCmd == "" {
		t.Error("PipePane should have been called")
	}
	if !strings.Contains(capturedPipeCmd, ".fifo") {
		t.Errorf("PipePane command should reference a .fifo file, got %q", capturedPipeCmd)
	}
	if tc.Output() == nil {
		t.Error("Output() should not be nil after Start")
	}
}

// --- Stop method tests ---

func TestTmuxClaude_Stop_NotRunning(t *testing.T) {
	mock := &MockTmuxExecutor{}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Stop()
	if err != nil {
		t.Errorf("Stop() when not running should return nil, got %v", err)
	}
}

func TestTmuxClaude_Stop_Running(t *testing.T) {
	sendKeysCalls := 0
	hasSessionCalled := false
	killCalled := false

	mock := &MockTmuxExecutor{
		SendKeysFn: func(sessionName, keys string, literal bool) error {
			sendKeysCalls++
			if sendKeysCalls == 1 {
				// First call should be /exit
				if keys != "/exit" || !literal {
					t.Errorf("first SendKeys should be literal /exit, got keys=%q literal=%v", keys, literal)
				}
			}
			return nil
		},
		HasSessionFn: func(sessionName string) (bool, error) {
			hasSessionCalled = true
			return true, nil // session still exists
		},
		KillSessionFn: func(sessionName string) error {
			killCalled = true
			return nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	// Manually set running state with a cancel func
	tc.running = true
	ctx, cancelFn := context.WithCancel(context.Background())
	tc.cancel = cancelFn
	tc.ctx = ctx

	err := tc.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	if tc.Running() {
		t.Error("Running() should be false after Stop")
	}
	if !hasSessionCalled {
		t.Error("HasSession should be called to check if session still exists")
	}
	if !killCalled {
		t.Error("KillSession should be called when session still exists")
	}
}

func TestTmuxClaude_Stop_SessionAlreadyGone(t *testing.T) {
	killCalled := false
	mock := &MockTmuxExecutor{
		SendKeysFn: func(sessionName, keys string, literal bool) error {
			return nil
		},
		HasSessionFn: func(sessionName string) (bool, error) {
			return false, nil // session already gone
		},
		KillSessionFn: func(sessionName string) error {
			killCalled = true
			return nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true
	ctx, cancelFn := context.WithCancel(context.Background())
	tc.cancel = cancelFn
	tc.ctx = ctx

	err := tc.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	if killCalled {
		t.Error("KillSession should NOT be called when session is already gone")
	}
}

func TestTmuxClaude_Stop_CleansFIFO(t *testing.T) {
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			return false, nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true
	ctx, cancelFn := context.WithCancel(context.Background())
	tc.cancel = cancelFn
	tc.ctx = ctx

	// Create a temp FIFO to verify cleanup
	tmpDir := t.TempDir()
	fifoPath := tmpDir + "/test.fifo"
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatalf("failed to create test FIFO: %v", err)
	}
	tc.fifoPath = fifoPath

	err := tc.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// FIFO should be removed
	if _, err := os.Stat(fifoPath); !os.IsNotExist(err) {
		t.Error("FIFO should be removed after Stop")
	}
}

// --- Send method tests ---

func TestTmuxClaude_Send_NotRunning(t *testing.T) {
	mock := &MockTmuxExecutor{}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Send(Message{Content: "hello"})
	if err == nil {
		t.Fatal("Send() should return error when not running")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Errorf("error should mention 'not running', got %q", err.Error())
	}
}

func TestTmuxClaude_Send_Success(t *testing.T) {
	var calls []struct {
		keys    string
		literal bool
	}
	mock := &MockTmuxExecutor{
		SendKeysFn: func(sessionName, keys string, literal bool) error {
			calls = append(calls, struct {
				keys    string
				literal bool
			}{keys, literal})
			return nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true

	before := tc.LastActivity()
	time.Sleep(10 * time.Millisecond)

	err := tc.Send(Message{Content: "test message"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Should have two SendKeys calls: literal content + Enter
	if len(calls) != 2 {
		t.Fatalf("expected 2 SendKeys calls, got %d", len(calls))
	}
	if calls[0].keys != "test message" || !calls[0].literal {
		t.Errorf("first call should be literal 'test message', got keys=%q literal=%v",
			calls[0].keys, calls[0].literal)
	}
	if calls[1].keys != "Enter" || calls[1].literal {
		t.Errorf("second call should be non-literal 'Enter', got keys=%q literal=%v",
			calls[1].keys, calls[1].literal)
	}

	// Activity should be updated
	after := tc.LastActivity()
	if !after.After(before) {
		t.Error("Send() should update LastActivity")
	}
}

func TestTmuxClaude_Send_SendKeysError(t *testing.T) {
	mock := &MockTmuxExecutor{
		SendKeysFn: func(sessionName, keys string, literal bool) error {
			if literal {
				return fmt.Errorf("connection lost")
			}
			return nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true

	err := tc.Send(Message{Content: "hello"})
	if err == nil {
		t.Fatal("Send() should return error when SendKeys fails")
	}
	if !strings.Contains(err.Error(), "send keys") {
		t.Errorf("error should mention 'send keys', got %q", err.Error())
	}
}

// --- Cancel method tests ---

func TestTmuxClaude_Cancel_NotRunning(t *testing.T) {
	mock := &MockTmuxExecutor{}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Cancel()
	if err != nil {
		t.Errorf("Cancel() when not running should return nil, got %v", err)
	}
}

func TestTmuxClaude_Cancel_Success(t *testing.T) {
	var capturedKeys string
	var capturedLiteral bool
	mock := &MockTmuxExecutor{
		SendKeysFn: func(sessionName, keys string, literal bool) error {
			capturedKeys = keys
			capturedLiteral = literal
			return nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true

	err := tc.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if capturedKeys != "C-c" {
		t.Errorf("Cancel should send 'C-c', got %q", capturedKeys)
	}
	if capturedLiteral {
		t.Error("Cancel should send C-c as non-literal (tmux key binding)")
	}
}

// --- Output method tests ---

func TestTmuxClaude_Output_NilFileWhenNotStarted(t *testing.T) {
	tc := NewTmuxClaude("repo")
	// When not started, fifoFile is nil (*os.File)(nil).
	// Due to Go's interface semantics, (*os.File)(nil) cast to io.Reader
	// produces a non-nil interface with a nil concrete value.
	// This is expected behavior, same as Claude.Output().
	_ = tc.Output() // Just verify it doesn't panic
}

func TestTmuxClaude_Output_ReturnsFile(t *testing.T) {
	tc := NewTmuxClaude("repo")
	// Create a temporary file to simulate the FIFO
	tmpFile, err := os.CreateTemp("", "tmux-output-test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	tc.fifoFile = tmpFile
	out := tc.Output()
	if out == nil {
		t.Error("Output() should return non-nil when fifoFile is set")
	}
	if out != tmpFile {
		t.Error("Output() should return the fifoFile")
	}
}

// --- Reconnect method tests ---

func TestTmuxClaude_Reconnect_AlreadyRunning(t *testing.T) {
	mock := &MockTmuxExecutor{}
	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true

	err := tc.Reconnect(context.Background())
	if err != nil {
		t.Errorf("Reconnect() when already running should return nil, got %v", err)
	}
}

func TestTmuxClaude_Reconnect_SessionNotFound(t *testing.T) {
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			return false, nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Reconnect(context.Background())
	if err == nil {
		t.Fatal("Reconnect() should return error when session not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got %q", err.Error())
	}
}

func TestTmuxClaude_Reconnect_HasSessionError(t *testing.T) {
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			return false, fmt.Errorf("tmux server unavailable")
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Reconnect(context.Background())
	if err == nil {
		t.Fatal("Reconnect() should return error when HasSession fails")
	}
	if !strings.Contains(err.Error(), "check session") {
		t.Errorf("error should mention 'check session', got %q", err.Error())
	}
}

func TestTmuxClaude_Reconnect_PipePaneError(t *testing.T) {
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			return true, nil
		},
		PipePaneFn: func(sessionName, command string) error {
			return fmt.Errorf("pipe-pane failed")
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))

	err := tc.Reconnect(context.Background())
	if err == nil {
		t.Fatal("Reconnect() should return error when PipePane fails")
	}
	if !strings.Contains(err.Error(), "setup pipe-pane") {
		t.Errorf("error should mention 'setup pipe-pane', got %q", err.Error())
	}
}

func TestTmuxClaude_Reconnect_Success(t *testing.T) {
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			return true, nil
		},
		PipePaneFn: func(sessionName, command string) error {
			// Open the FIFO for writing to unblock the read side
			parts := strings.Fields(command)
			if len(parts) >= 3 {
				fifoPath := parts[2]
				go func() {
					time.Sleep(50 * time.Millisecond)
					f, err := os.OpenFile(fifoPath, os.O_WRONLY, 0)
					if err == nil {
						_ = f.Close()
					}
				}()
			}
			return nil
		},
	}

	tc := NewTmuxClaude("reconnect-test", WithTmuxExecutor(mock))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := tc.Reconnect(ctx)
	if err != nil {
		t.Fatalf("Reconnect() error = %v", err)
	}
	defer func() { _ = tc.Stop() }()

	if !tc.Running() {
		t.Error("Running() should be true after successful Reconnect")
	}
	if tc.Output() == nil {
		t.Error("Output() should not be nil after Reconnect")
	}
}

// --- monitorSession tests ---

func TestTmuxClaude_MonitorSession_DetectsExit(t *testing.T) {
	callCount := 0
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			callCount++
			if callCount >= 2 {
				return false, nil // session gone on second check
			}
			return true, nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true
	ctx, cancelFn := context.WithCancel(context.Background())
	tc.ctx = ctx
	tc.cancel = cancelFn

	// Run monitor in background - it will detect session gone and set running=false
	go tc.monitorSession()

	// Wait for monitor to detect the session exit (at least two 5s ticks)
	// Use a shorter polling loop to check
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !tc.Running() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if tc.Running() {
		t.Error("monitorSession should set running=false when session exits")
	}

	cancelFn() // cleanup
}

func TestTmuxClaude_MonitorSession_StopsOnCancel(t *testing.T) {
	mock := &MockTmuxExecutor{
		HasSessionFn: func(sessionName string) (bool, error) {
			return true, nil
		},
	}

	tc := NewTmuxClaude("repo", WithTmuxExecutor(mock))
	tc.running = true
	ctx, cancelFn := context.WithCancel(context.Background())
	tc.ctx = ctx
	tc.cancel = cancelFn

	done := make(chan struct{})
	go func() {
		tc.monitorSession()
		close(done)
	}()

	// Cancel context to stop monitor
	cancelFn()

	select {
	case <-done:
		// monitorSession returned - good
	case <-time.After(10 * time.Second):
		t.Fatal("monitorSession should stop when context is cancelled")
	}

	// running should still be true since we cancelled, not because session died
	if !tc.Running() {
		t.Error("Running() should still be true - monitor stopped due to cancel, not session exit")
	}
}
