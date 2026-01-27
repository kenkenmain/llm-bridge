package llm

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Claude struct {
	workingDir    string
	resumeSession bool
	claudePath    string

	mu           sync.Mutex
	cmd          *exec.Cmd
	ptmx         *os.File
	running      bool
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

	args := []string{}
	if c.resumeSession {
		args = append(args, "--resume")
	}

	c.cmd = exec.CommandContext(ctx, c.claudePath, args...)
	c.cmd.Dir = c.workingDir
	c.cmd.Env = os.Environ()

	var err error
	c.ptmx, err = pty.Start(c.cmd)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}

	c.running = true
	c.lastActivity = time.Now()

	go func() {
		c.cmd.Wait()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	return nil
}

func (c *Claude) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		c.cmd.Process.Kill()
	}

	if c.ptmx != nil {
		c.ptmx.Close()
	}

	c.running = false
	return nil
}

func (c *Claude) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.ptmx == nil {
		return fmt.Errorf("claude not running")
	}

	c.lastActivity = time.Now()
	_, err := c.ptmx.WriteString(msg.Content + "\n")
	return err
}

func (c *Claude) Output() io.Reader {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ptmx
}

func (c *Claude) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Claude) Cancel() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	return c.cmd.Process.Signal(syscall.SIGINT)
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
