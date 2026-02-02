package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TmuxExecutor defines the interface for interacting with tmux.
// This abstraction allows testing without a real tmux binary.
type TmuxExecutor interface {
	// NewSession creates a new tmux session with the given name, working directory, and command.
	NewSession(ctx context.Context, sessionName, workingDir, command string) error

	// HasSession checks whether a tmux session with the given name exists.
	HasSession(sessionName string) (bool, error)

	// KillSession terminates the tmux session with the given name.
	KillSession(sessionName string) error

	// SendKeys sends keystrokes to the specified tmux session.
	// If literal is true, the -l flag is used to send keys literally.
	SendKeys(sessionName, keys string, literal bool) error

	// PipePane pipes the output of the specified tmux session to a shell command.
	PipePane(sessionName, command string) error

	// ListSessions returns the names of all active tmux sessions.
	ListSessions() ([]string, error)
}

// tmuxCommandTimeout is the default timeout for tmux commands.
const tmuxCommandTimeout = 10 * time.Second

// RealTmuxExecutor implements TmuxExecutor by shelling out to the tmux binary.
type RealTmuxExecutor struct {
	// TmuxPath is the path to the tmux binary. Defaults to "tmux".
	TmuxPath string
}

// NewRealTmuxExecutor creates a RealTmuxExecutor with the default tmux path.
func NewRealTmuxExecutor() *RealTmuxExecutor {
	return &RealTmuxExecutor{TmuxPath: "tmux"}
}

func (r *RealTmuxExecutor) tmuxPath() string {
	if r.TmuxPath == "" {
		return "tmux"
	}
	return r.TmuxPath
}

func (r *RealTmuxExecutor) runCommand(ctx context.Context, args ...string) ([]byte, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, tmuxCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, r.tmuxPath(), args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("tmux %s: %w (output: %s)", args[0], err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

// NewSession creates a new detached tmux session.
func (r *RealTmuxExecutor) NewSession(ctx context.Context, sessionName, workingDir, command string) error {
	args := []string{"new-session", "-d", "-s", sessionName, "-c", workingDir, command}
	_, err := r.runCommand(ctx, args...)
	if err != nil {
		return fmt.Errorf("create tmux session %q: %w", sessionName, err)
	}
	return nil
}

// HasSession checks if a tmux session exists. Returns true if exit code is 0.
func (r *RealTmuxExecutor) HasSession(sessionName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.tmuxPath(), "has-session", "-t", sessionName)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	// tmux has-session returns exit code 1 when the session does not exist.
	// This is expected behavior, not an error.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}

	return false, fmt.Errorf("check tmux session %q: %w", sessionName, err)
}

// KillSession terminates the specified tmux session.
func (r *RealTmuxExecutor) KillSession(sessionName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	_, err := r.runCommand(ctx, "kill-session", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("kill tmux session %q: %w", sessionName, err)
	}
	return nil
}

// SendKeys sends keystrokes to the specified tmux session.
func (r *RealTmuxExecutor) SendKeys(sessionName, keys string, literal bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	args := []string{"send-keys", "-t", sessionName}
	if literal {
		args = append(args, "-l")
	}
	args = append(args, keys)

	_, err := r.runCommand(ctx, args...)
	if err != nil {
		return fmt.Errorf("send keys to tmux session %q: %w", sessionName, err)
	}
	return nil
}

// PipePane pipes the output of the tmux pane to a shell command.
func (r *RealTmuxExecutor) PipePane(sessionName, command string) error {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	_, err := r.runCommand(ctx, "pipe-pane", "-o", "-t", sessionName, command)
	if err != nil {
		return fmt.Errorf("pipe-pane for tmux session %q: %w", sessionName, err)
	}
	return nil
}

// ListSessions returns the names of all active tmux sessions.
func (r *RealTmuxExecutor) ListSessions() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxCommandTimeout)
	defer cancel()

	output, err := r.runCommand(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		// tmux list-sessions returns error when no server is running (no sessions).
		// Treat this as an empty list rather than an error.
		if strings.Contains(string(output), "no server running") || strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, fmt.Errorf("list tmux sessions: %w", err)
	}

	raw := strings.TrimSpace(string(output))
	if raw == "" {
		return nil, nil
	}

	sessions := strings.Split(raw, "\n")
	result := make([]string, 0, len(sessions))
	for _, s := range sessions {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result, nil
}
