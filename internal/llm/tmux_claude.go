package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// TmuxClaude implements the LLM interface using tmux for session persistence.
// Sessions survive bridge crashes and can be reattached.
type TmuxClaude struct {
	repoName      string
	sessionName   string
	workingDir    string
	resumeSession bool
	claudePath    string
	executor      TmuxExecutor

	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	fifoPath     string
	fifoFile     *os.File
	running      bool
	lastActivity time.Time
}

// TmuxClaudeOption configures a TmuxClaude instance.
type TmuxClaudeOption func(*TmuxClaude)

// WithTmuxWorkingDir sets the working directory for the tmux session.
func WithTmuxWorkingDir(dir string) TmuxClaudeOption {
	return func(tc *TmuxClaude) {
		tc.workingDir = dir
	}
}

// WithTmuxResume sets whether to resume an existing Claude session.
func WithTmuxResume(resume bool) TmuxClaudeOption {
	return func(tc *TmuxClaude) {
		tc.resumeSession = resume
	}
}

// WithTmuxClaudePath sets the path to the Claude CLI binary.
func WithTmuxClaudePath(path string) TmuxClaudeOption {
	return func(tc *TmuxClaude) {
		if path != "" {
			tc.claudePath = path
		}
	}
}

// WithTmuxExecutor sets the TmuxExecutor implementation (for testing).
func WithTmuxExecutor(executor TmuxExecutor) TmuxClaudeOption {
	return func(tc *TmuxClaude) {
		tc.executor = executor
	}
}

// NewTmuxClaude creates a new TmuxClaude instance.
func NewTmuxClaude(repoName string, opts ...TmuxClaudeOption) *TmuxClaude {
	tc := &TmuxClaude{
		repoName:      repoName,
		sessionName:   SanitizeSessionName(repoName),
		workingDir:    ".",
		resumeSession: true,
		claudePath:    "claude",
		executor:      &RealTmuxExecutor{},
		lastActivity:  time.Now(),
	}
	for _, opt := range opts {
		opt(tc)
	}
	return tc
}

// Name returns the LLM backend name.
func (tc *TmuxClaude) Name() string {
	return "claude-tmux"
}

// Running returns true if the tmux session is active.
func (tc *TmuxClaude) Running() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.running
}

// LastActivity returns the time of last input/output.
func (tc *TmuxClaude) LastActivity() time.Time {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.lastActivity
}

// UpdateActivity updates the last activity timestamp.
func (tc *TmuxClaude) UpdateActivity() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.lastActivity = time.Now()
}

// Start creates a new tmux session running Claude CLI.
func (tc *TmuxClaude) Start(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.running {
		return nil
	}

	// Check that tmux is installed
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux binary not found: tmux_enabled requires tmux to be installed")
	}

	tc.ctx, tc.cancel = context.WithCancel(ctx)

	// Build Claude command with --resume flag if needed
	claudeCmd := tc.claudePath
	if tc.resumeSession {
		claudeCmd += " --resume"
	}

	// Create tmux session
	if err := tc.executor.NewSession(tc.ctx, tc.sessionName, tc.workingDir, claudeCmd); err != nil {
		tc.cancel()
		return fmt.Errorf("create tmux session: %w", err)
	}

	// Create secure FIFO directory and file
	uid := os.Getuid()
	fifoDir := fmt.Sprintf("/tmp/llm-bridge-%d", uid)
	if err := os.MkdirAll(fifoDir, 0700); err != nil {
		_ = tc.executor.KillSession(tc.sessionName) // best-effort cleanup
		tc.cancel()
		return fmt.Errorf("create FIFO directory: %w", err)
	}

	tc.fifoPath = filepath.Join(fifoDir, tc.sessionName+".fifo")

	// Remove stale FIFO if it exists (defense against crashes)
	os.Remove(tc.fifoPath)

	if err := syscall.Mkfifo(tc.fifoPath, 0600); err != nil {
		_ = tc.executor.KillSession(tc.sessionName) // best-effort cleanup
		tc.cancel()
		return fmt.Errorf("create FIFO: %w", err)
	}

	// Set up pipe-pane to stream output to FIFO
	pipePaneCmd := fmt.Sprintf("cat >> %s", tc.fifoPath)
	if err := tc.executor.PipePane(tc.sessionName, pipePaneCmd); err != nil {
		os.Remove(tc.fifoPath)
		_ = tc.executor.KillSession(tc.sessionName) // best-effort cleanup
		tc.cancel()
		return fmt.Errorf("setup pipe-pane: %w", err)
	}

	// Open FIFO for reading. Opening a FIFO for read blocks until a writer opens it.
	// We release the lock so monitorSession/other methods aren't blocked, then use a
	// goroutine with a timeout. The goroutine sets tc.fifoFile directly since we
	// control its lifecycle via the select below.
	tc.mu.Unlock()

	fifoOpenCh := make(chan error, 1)
	var fifoFile *os.File
	go func() {
		var err error
		fifoFile, err = os.OpenFile(tc.fifoPath, os.O_RDONLY, 0)
		fifoOpenCh <- err
	}()

	var fifoErr error
	select {
	case err := <-fifoOpenCh:
		fifoErr = err
	case <-time.After(10 * time.Second):
		fifoErr = fmt.Errorf("timeout opening FIFO: pipe-pane may not be writing")
	}

	tc.mu.Lock()

	if fifoErr != nil {
		os.Remove(tc.fifoPath)
		_ = tc.executor.KillSession(tc.sessionName) // best-effort cleanup
		tc.cancel()
		return fifoErr
	}

	tc.fifoFile = fifoFile
	tc.running = true
	tc.lastActivity = time.Now()

	// Monitor session health
	go tc.monitorSession()

	return nil
}

// monitorSession watches the tmux session and marks as not running when it exits.
func (tc *TmuxClaude) monitorSession() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tc.ctx.Done():
			return
		case <-ticker.C:
			exists, err := tc.executor.HasSession(tc.sessionName)
			if err != nil || !exists {
				tc.mu.Lock()
				tc.running = false
				tc.mu.Unlock()
				return
			}
		}
	}
}

// Stop gracefully terminates the tmux session.
func (tc *TmuxClaude) Stop() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if !tc.running {
		return nil
	}

	// Try graceful exit first
	_ = tc.executor.SendKeys(tc.sessionName, "/exit", true)

	// Give Claude a moment to exit gracefully
	time.Sleep(2 * time.Second)

	// Check if session still exists
	exists, _ := tc.executor.HasSession(tc.sessionName)
	if exists {
		_ = tc.executor.KillSession(tc.sessionName)
	}

	// Cleanup FIFO
	if tc.fifoFile != nil {
		_ = tc.fifoFile.Close()
		tc.fifoFile = nil
	}
	if tc.fifoPath != "" {
		os.Remove(tc.fifoPath)
	}

	if tc.cancel != nil {
		tc.cancel()
	}

	tc.running = false
	return nil
}

// Send writes a message to the tmux session via send-keys.
func (tc *TmuxClaude) Send(msg Message) error {
	tc.mu.Lock()
	if !tc.running {
		tc.mu.Unlock()
		return fmt.Errorf("tmux claude not running")
	}
	sessionName := tc.sessionName
	tc.lastActivity = time.Now()
	tc.mu.Unlock()

	// Send the message content as literal keys followed by Enter
	if err := tc.executor.SendKeys(sessionName, msg.Content, true); err != nil {
		return fmt.Errorf("send keys: %w", err)
	}
	return tc.executor.SendKeys(sessionName, "Enter", false)
}

// Cancel sends Ctrl-C to the tmux session.
func (tc *TmuxClaude) Cancel() error {
	tc.mu.Lock()
	if !tc.running {
		tc.mu.Unlock()
		return nil
	}
	sessionName := tc.sessionName
	tc.mu.Unlock()

	return tc.executor.SendKeys(sessionName, "C-c", false)
}

// Output returns the FIFO file reader for streaming output from pipe-pane.
func (tc *TmuxClaude) Output() io.Reader {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.fifoFile
}

// Reconnect attaches to an existing tmux session (used for session recovery).
// Unlike Start, it does not create a new session -- it verifies the session exists
// and sets up a new FIFO + pipe-pane for output streaming.
func (tc *TmuxClaude) Reconnect(ctx context.Context) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.running {
		return nil
	}

	// Verify session exists
	exists, err := tc.executor.HasSession(tc.sessionName)
	if err != nil {
		return fmt.Errorf("check session: %w", err)
	}
	if !exists {
		return fmt.Errorf("tmux session %q not found", tc.sessionName)
	}

	tc.ctx, tc.cancel = context.WithCancel(ctx)

	// Create secure FIFO directory and file
	uid := os.Getuid()
	fifoDir := fmt.Sprintf("/tmp/llm-bridge-%d", uid)
	if err := os.MkdirAll(fifoDir, 0700); err != nil {
		tc.cancel()
		return fmt.Errorf("create FIFO directory: %w", err)
	}

	tc.fifoPath = filepath.Join(fifoDir, tc.sessionName+".fifo")

	// Remove stale FIFO
	os.Remove(tc.fifoPath)

	if err := syscall.Mkfifo(tc.fifoPath, 0600); err != nil {
		tc.cancel()
		return fmt.Errorf("create FIFO: %w", err)
	}

	// Set up pipe-pane
	pipePaneCmd := fmt.Sprintf("cat >> %s", tc.fifoPath)
	if err := tc.executor.PipePane(tc.sessionName, pipePaneCmd); err != nil {
		os.Remove(tc.fifoPath)
		tc.cancel()
		return fmt.Errorf("setup pipe-pane: %w", err)
	}

	// Open FIFO for reading - release lock to avoid blocking other operations.
	tc.mu.Unlock()

	fifoOpenCh := make(chan error, 1)
	var fifoFile *os.File
	go func() {
		var err error
		fifoFile, err = os.OpenFile(tc.fifoPath, os.O_RDONLY, 0)
		fifoOpenCh <- err
	}()

	var fifoErr error
	select {
	case err := <-fifoOpenCh:
		fifoErr = err
	case <-time.After(10 * time.Second):
		fifoErr = fmt.Errorf("timeout opening FIFO")
	}

	tc.mu.Lock()

	if fifoErr != nil {
		os.Remove(tc.fifoPath)
		tc.cancel()
		return fifoErr
	}

	tc.fifoFile = fifoFile
	tc.running = true
	tc.lastActivity = time.Now()

	go tc.monitorSession()

	return nil
}
