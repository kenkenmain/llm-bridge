package output

import (
	"strings"
	"testing"
)

func TestNewHandler_DefaultThreshold(t *testing.T) {
	h := NewHandler(0)
	if h.threshold != 1500 {
		t.Errorf("NewHandler(0).threshold = %d, want 1500", h.threshold)
	}

	h = NewHandler(-100)
	if h.threshold != 1500 {
		t.Errorf("NewHandler(-100).threshold = %d, want 1500", h.threshold)
	}
}

func TestNewHandler_CustomThreshold(t *testing.T) {
	h := NewHandler(2000)
	if h.threshold != 2000 {
		t.Errorf("NewHandler(2000).threshold = %d, want 2000", h.threshold)
	}
}

func TestShouldAttach(t *testing.T) {
	h := NewHandler(100)

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{"below threshold", strings.Repeat("a", 50), false},
		{"at threshold", strings.Repeat("a", 100), false},
		{"above threshold", strings.Repeat("a", 101), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := h.ShouldAttach(tt.content)
			if got != tt.want {
				t.Errorf("ShouldAttach(%d chars) = %v, want %v", len(tt.content), got, tt.want)
			}
		})
	}
}

func TestFormatFile(t *testing.T) {
	h := NewHandler(100)
	content := "test content"

	filename, data := h.FormatFile(content)

	if !strings.HasPrefix(filename, "response-") {
		t.Errorf("filename %q should start with 'response-'", filename)
	}
	if !strings.HasSuffix(filename, ".md") {
		t.Errorf("filename %q should end with '.md'", filename)
	}
	if string(data) != content {
		t.Errorf("data = %q, want %q", string(data), content)
	}
}
