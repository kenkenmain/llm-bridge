package bridge

import (
	"fmt"
	"sync"
	"time"
)

// Merger handles input conflict detection and prefixing
type Merger struct {
	mu          sync.Mutex
	lastSource  string
	lastTime    time.Time
	conflictWin time.Duration
}

func NewMerger(conflictWindow time.Duration) *Merger {
	if conflictWindow <= 0 {
		conflictWindow = 2 * time.Second
	}
	return &Merger{
		conflictWin: conflictWindow,
	}
}

// FormatMessage adds source prefix only if there's a conflict
// (multiple sources sending within the conflict window)
func (m *Merger) FormatMessage(source, content string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	needsPrefix := false

	// Check if different source within conflict window
	if m.lastSource != "" && m.lastSource != source {
		if now.Sub(m.lastTime) < m.conflictWin {
			needsPrefix = true
		}
	}

	m.lastSource = source
	m.lastTime = now

	if needsPrefix {
		return fmt.Sprintf("[%s] %s", source, content)
	}
	return content
}
