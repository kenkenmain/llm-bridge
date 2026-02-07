package bridge

import "encoding/json"

// StreamEvent is the top-level envelope for all stream-json lines.
type StreamEvent struct {
	Type string `json:"type"`
	// For content_block_delta events:
	Delta *DeltaPayload `json:"delta,omitempty"`
	// For result events:
	Result *ResultPayload `json:"result,omitempty"`
	// For message_start events:
	Message *MessagePayload `json:"message,omitempty"`
}

// DeltaPayload holds the delta content for content_block_delta events.
type DeltaPayload struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ResultPayload holds the result data including session ID for continuity.
type ResultPayload struct {
	SessionID string `json:"session_id"`
}

// MessagePayload holds message metadata from message_start events.
type MessagePayload struct {
	ID string `json:"id"`
}

// extractText parses a single NDJSON line and returns the text content
// if it is a text delta event. Returns empty string for non-text events.
func extractText(line string) string {
	var event StreamEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		return ""
	}
	if event.Type == "content_block_delta" && event.Delta != nil && event.Delta.Type == "text_delta" {
		return event.Delta.Text
	}
	return ""
}
