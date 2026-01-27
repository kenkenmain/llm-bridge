package output

import (
	"fmt"
	"time"
)

type Handler struct {
	threshold int
}

func NewHandler(threshold int) *Handler {
	if threshold <= 0 {
		threshold = 1500
	}
	return &Handler{threshold: threshold}
}

func (h *Handler) ShouldAttach(content string) bool {
	return len(content) > h.threshold
}

func (h *Handler) FormatFile(content string) (filename string, data []byte) {
	filename = fmt.Sprintf("response-%s.md", time.Now().Format("150405"))
	data = []byte(content)
	return
}

func (h *Handler) Truncate(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen-3] + "..."
}
