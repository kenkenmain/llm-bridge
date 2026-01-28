package provider

import (
	"context"
	"sync"
)

// MockProvider implements Provider for testing
type MockProvider struct {
	name      string
	channelID string

	mu          sync.Mutex
	messages    chan Message
	sentMsgs    []SentMessage
	sentFiles   []SentFile
	startCalled bool
	stopCalled  bool
	startErr    error
	sendErr     error
	stopped     bool
}

type SentMessage struct {
	ChannelID string
	Content   string
}

type SentFile struct {
	ChannelID string
	Filename  string
	Content   []byte
}

func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		name:      name,
		channelID: "mock-channel",
		messages:  make(chan Message, 100),
	}
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	return m.startErr
}

func (m *MockProvider) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return nil
	}
	m.stopCalled = true
	m.stopped = true
	close(m.messages)
	return nil
}

func (m *MockProvider) Send(channelID string, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMsgs = append(m.sentMsgs, SentMessage{ChannelID: channelID, Content: content})
	return nil
}

func (m *MockProvider) SendFile(channelID string, filename string, content []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentFiles = append(m.sentFiles, SentFile{ChannelID: channelID, Filename: filename, Content: content})
	return nil
}

func (m *MockProvider) Messages() <-chan Message {
	return m.messages
}

// Test helpers

func (m *MockProvider) SimulateMessage(msg Message) {
	m.mu.Lock()
	stopped := m.stopped
	m.mu.Unlock()
	if stopped {
		return
	}
	select {
	case m.messages <- msg:
	default:
	}
}

func (m *MockProvider) GetSentMessages() []SentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]SentMessage, len(m.sentMsgs))
	copy(result, m.sentMsgs)
	return result
}

func (m *MockProvider) GetSentFiles() []SentFile {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]SentFile, len(m.sentFiles))
	copy(result, m.sentFiles)
	return result
}

func (m *MockProvider) SetStartError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startErr = err
}

func (m *MockProvider) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendErr = err
}

func (m *MockProvider) WasStartCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startCalled
}

func (m *MockProvider) WasStopCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopCalled
}
