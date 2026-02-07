package llm

import (
	"context"
	"io"
	"time"
)

// Message represents input from a source
type Message struct {
	Source  string // "discord"
	Content string
}

// LLM defines the interface for LLM backends
type LLM interface {
	// Start spawns the LLM process
	Start(ctx context.Context) error

	// Stop terminates the LLM process
	Stop() error

	// Send writes a message to the LLM's stdin
	Send(msg Message) error

	// Output returns the reader for LLM output
	Output() io.Reader

	// Running returns true if the LLM process is active
	Running() bool

	// Cancel sends interrupt signal (SIGINT)
	Cancel() error

	// LastActivity returns time of last input/output
	LastActivity() time.Time

	// UpdateActivity updates the last activity timestamp
	UpdateActivity()

	// Name returns the LLM backend name (e.g. "claude")
	Name() string
}
