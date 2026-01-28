package llm

import (
	"bytes"
	"context"
	"io"
	"sync"
	"time"
)

// MockLLM implements LLM for testing
type MockLLM struct {
	name string

	mu           sync.Mutex
	running      bool
	lastActivity time.Time
	outputBuf    *bytes.Buffer
	sentMsgs     []Message

	startErr  error
	stopErr   error
	sendErr   error
	cancelErr error
}

func NewMockLLM(name string) *MockLLM {
	return &MockLLM{
		name:         name,
		lastActivity: time.Now(),
		outputBuf:    &bytes.Buffer{},
	}
}

func (m *MockLLM) Name() string {
	return m.name
}

func (m *MockLLM) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startErr != nil {
		return m.startErr
	}
	m.running = true
	m.lastActivity = time.Now()
	return nil
}

func (m *MockLLM) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopErr != nil {
		return m.stopErr
	}
	m.running = false
	return nil
}

func (m *MockLLM) Send(msg Message) error {
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

func (m *MockLLM) Output() io.Reader {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.outputBuf
}

func (m *MockLLM) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

func (m *MockLLM) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancelErr != nil {
		return m.cancelErr
	}
	return nil
}

func (m *MockLLM) LastActivity() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastActivity
}

func (m *MockLLM) UpdateActivity() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActivity = time.Now()
}

// Test helpers

func (m *MockLLM) WriteOutput(data string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputBuf.WriteString(data)
}

func (m *MockLLM) GetSentMessages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]Message, len(m.sentMsgs))
	copy(result, m.sentMsgs)
	return result
}

func (m *MockLLM) SetStartError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startErr = err
}

func (m *MockLLM) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

func (m *MockLLM) SetCancelError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancelErr = err
}

func (m *MockLLM) SetRunning(running bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running = running
}

func (m *MockLLM) SetLastActivity(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastActivity = t
}
