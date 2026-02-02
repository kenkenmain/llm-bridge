package llm

import "context"

// MockTmuxExecutor is a test double for TmuxExecutor.
type MockTmuxExecutor struct {
	NewSessionFn   func(ctx context.Context, sessionName, workingDir, command string) error
	HasSessionFn   func(sessionName string) (bool, error)
	KillSessionFn  func(sessionName string) error
	SendKeysFn     func(sessionName, keys string, literal bool) error
	PipePaneFn     func(sessionName, command string) error
	ListSessionsFn func() ([]string, error)
}

func (m *MockTmuxExecutor) NewSession(ctx context.Context, sessionName, workingDir, command string) error {
	if m.NewSessionFn != nil {
		return m.NewSessionFn(ctx, sessionName, workingDir, command)
	}
	return nil
}

func (m *MockTmuxExecutor) HasSession(sessionName string) (bool, error) {
	if m.HasSessionFn != nil {
		return m.HasSessionFn(sessionName)
	}
	return false, nil
}

func (m *MockTmuxExecutor) KillSession(sessionName string) error {
	if m.KillSessionFn != nil {
		return m.KillSessionFn(sessionName)
	}
	return nil
}

func (m *MockTmuxExecutor) SendKeys(sessionName, keys string, literal bool) error {
	if m.SendKeysFn != nil {
		return m.SendKeysFn(sessionName, keys, literal)
	}
	return nil
}

func (m *MockTmuxExecutor) PipePane(sessionName, command string) error {
	if m.PipePaneFn != nil {
		return m.PipePaneFn(sessionName, command)
	}
	return nil
}

func (m *MockTmuxExecutor) ListSessions() ([]string, error) {
	if m.ListSessionsFn != nil {
		return m.ListSessionsFn()
	}
	return nil, nil
}
