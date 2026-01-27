package bridge

import (
	"fmt"
	"sync"
	"time"
)

// Merger handles input conflict detection and prefixing.
// When multiple sources send messages within the conflict window,
// ALL messages get prefixed (not just the second one).
type Merger struct {
	mu          sync.Mutex
	sources     map[string]time.Time // track recent activity per source
	conflictWin time.Duration
}

func NewMerger(conflictWindow time.Duration) *Merger {
	if conflictWindow <= 0 {
		conflictWindow = 2 * time.Second
	}
	return &Merger{
		sources:     make(map[string]time.Time),
		conflictWin: conflictWindow,
	}
}

// FormatMessage adds source prefix when multiple sources are active
// within the conflict window. This ensures BOTH/ALL messages get prefixed,
// not just the second one.
func (m *Merger) FormatMessage(source, content string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Clean up stale sources
	for src, t := range m.sources {
		if now.Sub(t) >= m.conflictWin {
			delete(m.sources, src)
		}
	}

	// Check if we're in a conflict period (other sources recently active)
	inConflict := false
	for src := range m.sources {
		if src != source {
			inConflict = true
			break
		}
	}

	// Record this source's activity
	m.sources[source] = now

	// Prefix if in conflict period
	if inConflict {
		return fmt.Sprintf("[%s] %s", source, content)
	}
	return content
}
