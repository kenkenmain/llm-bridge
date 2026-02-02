package llm

import "testing"

func TestSanitizeSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal name",
			input:    "my-repo",
			expected: "llm-bridge-my-repo",
		},
		{
			name:     "dots replaced",
			input:    "my.repo.name",
			expected: "llm-bridge-my-repo-name",
		},
		{
			name:     "slashes replaced",
			input:    "org/repo/sub",
			expected: "llm-bridge-org-repo-sub",
		},
		{
			name:     "spaces replaced",
			input:    "my repo name",
			expected: "llm-bridge-my-repo-name",
		},
		{
			name:     "special characters replaced",
			input:    "repo@v2!#$%",
			expected: "llm-bridge-repo-v2",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "llm-bridge-",
		},
		{
			name:     "consecutive special chars collapsed",
			input:    "repo...name",
			expected: "llm-bridge-repo-name",
		},
		{
			name:     "trailing special chars trimmed",
			input:    "repo-name...",
			expected: "llm-bridge-repo-name",
		},
		{
			name:     "leading special chars trimmed",
			input:    "...repo-name",
			expected: "llm-bridge-repo-name",
		},
		{
			name:     "alphanumeric only",
			input:    "myrepo123",
			expected: "llm-bridge-myrepo123",
		},
		{
			name:     "hyphens preserved",
			input:    "my-cool-repo",
			expected: "llm-bridge-my-cool-repo",
		},
		{
			name:     "consecutive hyphens collapsed",
			input:    "my---repo",
			expected: "llm-bridge-my-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSessionName(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeSessionName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseSessionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid prefixed name",
			input:    "llm-bridge-my-repo",
			expected: "my-repo",
		},
		{
			name:     "unprefixed name returns empty",
			input:    "some-other-session",
			expected: "",
		},
		{
			name:     "empty string returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "prefix only returns empty repo name",
			input:    "llm-bridge-",
			expected: "",
		},
		{
			name:     "partial prefix does not match",
			input:    "llm-bridge",
			expected: "",
		},
		{
			name:     "complex repo name",
			input:    "llm-bridge-org-repo-sub",
			expected: "org-repo-sub",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSessionName(tt.input)
			if got != tt.expected {
				t.Errorf("ParseSessionName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSessionPrefix(t *testing.T) {
	if SessionPrefix != "llm-bridge-" {
		t.Errorf("SessionPrefix = %q, want %q", SessionPrefix, "llm-bridge-")
	}
}
