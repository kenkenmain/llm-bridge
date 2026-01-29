package bridge

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/anthropics/llm-bridge/internal/llm"
)

// mockLLM implements llm.LLM for testing
type mockLLM struct {
	name string

	mu           sync.Mutex
	running      bool
	lastActivity time.Time
	sentMsgs     []llm.Message
	output       io.Reader

	sendErr   error
	cancelErr error
}

func newMockLLM(name string) *mockLLM {
	return &mockLLM{
		name:         name,
		lastActivity: time.Now(),
	}
}

func (m *mockLLM) Name() string { return m.name }
func (m *mockLLM) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = true
	return nil
}
func (m *mockLLM) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = false
	return nil
}

func (m *mockLLM) Send(msg llm.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	if !m.running {
		return io.ErrClosedPipe
	}
	m.sentMsgs = append(m.sentMsgs, msg)
	m.lastActivity = time.Now()
	return nil
}

func (m *mockLLM) Output() io.Reader {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.output
}

func (m *mockLLM) SetOutput(r io.Reader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.output = r
}

func (m *mockLLM) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *mockLLM) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cancelErr
}

func (m *mockLLM) LastActivity() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastActivity
}

func (m *mockLLM) UpdateActivity() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActivity = time.Now()
}

// Test helpers

func (m *mockLLM) getSentMessages() []llm.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]llm.Message, len(m.sentMsgs))
	copy(result, m.sentMsgs)
	return result
}

func (m *mockLLM) setRunning(running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = running
}

func (m *mockLLM) setSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

func (m *mockLLM) setLastActivity(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActivity = t
}
