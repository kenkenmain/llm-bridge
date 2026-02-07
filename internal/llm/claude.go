package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// streamResult is a minimal struct for extracting session IDs from NDJSON result events.
type streamResult struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
}

type Claude struct {
	workingDir    string
	resumeSession bool
	claudePath    string

	mu           sync.Mutex
	ctx          context.Context    // Stored from Start() for subprocess spawning
	activeCmd    *exec.Cmd          // Currently-running subprocess (nil between messages)
	pipeReader   *io.PipeReader     // Stable reader returned by Output()
	pipeWriter   *io.PipeWriter     // Write end; subprocess stdout is copied here
	running      bool               // True from Start() to Stop()
	sessionID    string             // Captured from first response for -r flag
	lastActivity time.Time
}

type ClaudeOption func(*Claude)

func WithWorkingDir(dir string) ClaudeOption {
	return func(c *Claude) {
		c.workingDir = dir
	}
}

func WithResume(resume bool) ClaudeOption {
	return func(c *Claude) {
		c.resumeSession = resume
	}
}

func WithClaudePath(path string) ClaudeOption {
	return func(c *Claude) {
		if path != "" {
			c.claudePath = path
		}
	}
}

func NewClaude(opts ...ClaudeOption) *Claude {
	c := &Claude{
		workingDir:    ".",
		resumeSession: true,
		claudePath:    "claude",
		lastActivity:  time.Now(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Claude) Name() string {
	return "claude"
}

func (c *Claude) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	c.pipeReader, c.pipeWriter = io.Pipe()
	c.ctx = ctx
	c.running = true
	c.lastActivity = time.Now()
	c.sessionID = ""

	return nil
}

func (c *Claude) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	// Kill active subprocess if present
	if c.activeCmd != nil && c.activeCmd.Process != nil {
		if err := syscall.Kill(-c.activeCmd.Process.Pid, syscall.SIGTERM); err != nil {
			_ = c.activeCmd.Process.Kill()
		}
	}

	// Close pipeWriter to signal EOF to readers
	if c.pipeWriter != nil {
		_ = c.pipeWriter.Close()
	}

	c.running = false
	return nil
}

func (c *Claude) Send(msg Message) error {
	c.mu.Lock()

	if !c.running || c.pipeWriter == nil {
		c.mu.Unlock()
		return fmt.Errorf("claude not running")
	}

	// Reject if a previous subprocess is still running
	if c.activeCmd != nil {
		c.mu.Unlock()
		return fmt.Errorf("previous message still processing")
	}

	args := []string{"-p", "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"}
	if c.resumeSession && c.sessionID != "" {
		args = append(args, "-r", c.sessionID)
	}

	cmd := exec.CommandContext(c.ctx, c.claudePath, args...)
	cmd.Dir = c.workingDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Pass message via stdin to avoid ARG_MAX risk
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("start claude: %w", err)
	}

	// Write message to stdin and close to signal EOF
	go func() {
		_, _ = io.WriteString(stdinPipe, msg.Content)
		_ = stdinPipe.Close()
	}()

	c.activeCmd = cmd
	c.lastActivity = time.Now()

	pw := c.pipeWriter
	c.mu.Unlock()

	// Goroutine: read subprocess stdout, extract session ID, forward to pipe
	go func() {
		scanner := bufio.NewScanner(stdout)
		// Increase buffer size to 1MB to handle large NDJSON lines
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()

			// Extract session ID from result events
			var event streamResult
			if json.Unmarshal([]byte(line), &event) == nil && event.Type == "result" && event.SessionID != "" {
				c.mu.Lock()
				c.sessionID = event.SessionID
				c.mu.Unlock()
			}

			// Forward raw JSON line to pipeWriter
			_, _ = pw.Write([]byte(line + "\n"))
		}

		// Reap the process
		_ = cmd.Wait()

		// Clear activeCmd
		c.mu.Lock()
		if c.activeCmd == cmd {
			c.activeCmd = nil
		}
		c.mu.Unlock()
	}()

	return nil
}

func (c *Claude) Output() io.Reader {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pipeReader
}

func (c *Claude) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Claude) Cancel() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.activeCmd == nil || c.activeCmd.Process == nil {
		return nil
	}

	return syscall.Kill(-c.activeCmd.Process.Pid, syscall.SIGINT)
}

func (c *Claude) LastActivity() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastActivity
}

func (c *Claude) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastActivity = time.Now()
}
