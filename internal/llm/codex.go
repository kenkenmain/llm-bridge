package llm

import (
	"context"
	"fmt"
	"io"
	"time"
)

// Codex implements the LLM interface for Codex CLI (stub for future)
type Codex struct {
	workingDir string
}

func NewCodex(workingDir string) *Codex {
	return &Codex{workingDir: workingDir}
}

func (c *Codex) Name() string {
	return "codex"
}

func (c *Codex) Start(ctx context.Context) error {
	return fmt.Errorf("codex support not yet implemented")
}

func (c *Codex) Stop() error {
	return nil
}

func (c *Codex) Send(msg Message) error {
	return fmt.Errorf("codex not running")
}

func (c *Codex) Output() io.Reader {
	return nil
}

func (c *Codex) Running() bool {
	return false
}

func (c *Codex) Cancel() error {
	return nil
}

func (c *Codex) LastActivity() time.Time {
	return time.Time{}
}

func (c *Codex) UpdateActivity() {
	// No-op for stub
}
