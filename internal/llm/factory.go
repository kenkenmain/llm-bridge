package llm

import "fmt"

// New creates an LLM instance based on the backend name
func New(backend, workingDir, claudePath string, resume bool) (LLM, error) {
	switch backend {
	case "claude", "":
		return NewClaude(
			WithWorkingDir(workingDir),
			WithResume(resume),
			WithClaudePath(claudePath),
		), nil
	case "codex":
		return NewCodex(workingDir), nil
	default:
		return nil, fmt.Errorf("unknown LLM backend: %s", backend)
	}
}
