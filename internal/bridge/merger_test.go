package bridge

import (
	"strings"
	"testing"
	"time"
)

func TestNewMerger_DefaultWindow(t *testing.T) {
	m := NewMerger(0)
	if m.conflictWin != 2*time.Second {
		t.Errorf("conflictWin = %v, want 2s", m.conflictWin)
	}
}

func TestNewMerger_CustomWindow(t *testing.T) {
	m := NewMerger(5 * time.Second)
	if m.conflictWin != 5*time.Second {
		t.Errorf("conflictWin = %v, want 5s", m.conflictWin)
	}
}

func TestMerger_SingleSource_NoPrefix(t *testing.T) {
	m := NewMerger(100 * time.Millisecond)

	msg := m.FormatMessage("discord", "hello world")
	if msg != "hello world" {
		t.Errorf("single source should not have prefix, got %q", msg)
	}

	// Same source again
	msg2 := m.FormatMessage("discord", "second message")
	if msg2 != "second message" {
		t.Errorf("same source should not have prefix, got %q", msg2)
	}
}

func TestMerger_MultipleSources_Prefix(t *testing.T) {
	m := NewMerger(100 * time.Millisecond)

	// First message from discord - no prefix
	msg1 := m.FormatMessage("discord", "from discord")
	if msg1 != "from discord" {
		t.Errorf("first message should not have prefix, got %q", msg1)
	}

	// Second message from terminal within window - gets prefix
	msg2 := m.FormatMessage("terminal", "from terminal")
	if !strings.HasPrefix(msg2, "[terminal]") {
		t.Errorf("second source should have prefix, got %q", msg2)
	}

	// Third message from discord within window - also gets prefix now
	msg3 := m.FormatMessage("discord", "another discord")
	if !strings.HasPrefix(msg3, "[discord]") {
		t.Errorf("during conflict, all sources should have prefix, got %q", msg3)
	}
}

func TestMerger_ConflictExpires(t *testing.T) {
	m := NewMerger(50 * time.Millisecond)

	// First from discord
	m.FormatMessage("discord", "msg1")

	// Second from terminal
	msg2 := m.FormatMessage("terminal", "msg2")
	if !strings.HasPrefix(msg2, "[terminal]") {
		t.Errorf("expected prefix during conflict, got %q", msg2)
	}

	// Wait for conflict window to expire
	time.Sleep(60 * time.Millisecond)

	// Now single source should work without prefix
	msg3 := m.FormatMessage("discord", "after expiry")
	if msg3 != "after expiry" {
		t.Errorf("after conflict window, no prefix expected, got %q", msg3)
	}
}

func TestMerger_ThreeSources(t *testing.T) {
	m := NewMerger(100 * time.Millisecond)

	// All three sources send within window
	m.FormatMessage("discord", "1")
	msg2 := m.FormatMessage("terminal", "2")
	msg3 := m.FormatMessage("web", "3")

	if !strings.HasPrefix(msg2, "[terminal]") {
		t.Errorf("terminal should have prefix, got %q", msg2)
	}
	if !strings.HasPrefix(msg3, "[web]") {
		t.Errorf("web should have prefix, got %q", msg3)
	}
}

func TestMerger_ConcurrentAccess(t *testing.T) {
	m := NewMerger(100 * time.Millisecond)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(source string) {
			for j := 0; j < 100; j++ {
				m.FormatMessage(source, "test")
			}
			done <- true
		}(string(rune('a' + i)))
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// Test passes if no race condition panic occurs
}
