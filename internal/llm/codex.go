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

type Codex struct {
	workingDir    string
	resumeSession bool
	codexPath     string

	mu           sync.Mutex
	cmd          *exec.Cmd
	ptmx         *os.File
	running      bool
	lastActivity time.Time
	closeOnce    *sync.Once
}

type CodexOption func(*Codex)

func WithCodexWorkingDir(dir string) CodexOption {
	return func(c *Codex) {
		c.workingDir = dir
	}
}

func WithCodexResume(resume bool) CodexOption {
	return func(c *Codex) {
		c.resumeSession = resume
	}
}

func WithCodexPath(path string) CodexOption {
	return func(c *Codex) {
		if path != "" {
			c.codexPath = path
		}
	}
}

func NewCodex(opts ...CodexOption) *Codex {
	c := &Codex{
		workingDir:    ".",
		resumeSession: true,
		codexPath:     "codex",
		lastActivity:  time.Now(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Codex) Name() string {
	return "codex"
}

func (c *Codex) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	args := []string{}
	if c.resumeSession {
		args = append(args, "resume", "--last")
	}

	c.cmd = exec.CommandContext(ctx, c.codexPath, args...)
	c.cmd.Dir = c.workingDir
	c.cmd.Env = os.Environ()

	var err error
	c.ptmx, err = pty.Start(c.cmd)
	if err != nil {
		return fmt.Errorf("start pty: %w", err)
	}

	c.running = true
	c.lastActivity = time.Now()
	c.closeOnce = &sync.Once{}

	currentPtmx := c.ptmx
	currentOnce := c.closeOnce
	currentCmd := c.cmd

	go func() {
		_ = currentCmd.Wait()
		c.mu.Lock()
		if c.cmd == currentCmd {
			c.running = false
		}
		currentOnce.Do(func() {
			if currentPtmx != nil {
				_ = currentPtmx.Close()
			}
		})
		c.mu.Unlock()
	}()

	return nil
}

func (c *Codex) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = c.cmd.Process.Kill()
	}

	if c.closeOnce != nil {
		c.closeOnce.Do(func() {
			if c.ptmx != nil {
				_ = c.ptmx.Close()
				c.ptmx = nil
			}
		})
	}

	c.running = false
	return nil
}

func (c *Codex) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.ptmx == nil {
		return fmt.Errorf("codex not running")
	}

	c.lastActivity = time.Now()
	_, err := c.ptmx.WriteString(msg.Content + "\n")
	return err
}

func (c *Codex) Output() io.Reader {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ptmx
}

func (c *Codex) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *Codex) Cancel() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running || c.cmd == nil || c.cmd.Process == nil {
		return nil
	}

	return c.cmd.Process.Signal(syscall.SIGINT)
}

func (c *Codex) LastActivity() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastActivity
}

func (c *Codex) UpdateActivity() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastActivity = time.Now()
}
