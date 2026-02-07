package llm

import (
	"testing"
)

func TestNew_Claude(t *testing.T) {
	llm, err := New("claude", "/tmp/test", "/usr/bin/claude", "/usr/bin/codex", true)
	if err != nil {
		t.Fatalf("New(claude) error = %v", err)
	}
	if llm.Name() != "claude" {
		t.Errorf("Name() = %q, want claude", llm.Name())
	}
}

func TestNew_EmptyDefaultsToClaude(t *testing.T) {
	llm, err := New("", "/tmp/test", "", "", true)
	if err != nil {
		t.Fatalf("New('') error = %v", err)
	}
	if llm.Name() != "claude" {
		t.Errorf("Name() = %q, want claude", llm.Name())
	}
}

func TestNew_Codex(t *testing.T) {
	llm, err := New("codex", "/tmp/test", "/usr/bin/claude", "/usr/bin/codex", true)
	if err != nil {
		t.Fatalf("New(codex) error = %v", err)
	}
	if llm.Name() != "codex" {
		t.Errorf("Name() = %q, want codex", llm.Name())
	}
}

func TestNew_Unknown(t *testing.T) {
	_, err := New("gpt4", "/tmp/test", "", "", false)
	if err == nil {
		t.Error("New(gpt4) should return error for unknown backend")
	}
}
