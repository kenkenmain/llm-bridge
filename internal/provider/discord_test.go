package provider

import (
	"testing"
)

func TestNewDiscord(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1", "channel-2"})
	if d == nil {
		t.Fatal("NewDiscord returned nil")
	}
}

func TestDiscord_Name(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})
	if name := d.Name(); name != "discord" {
		t.Errorf("Name() = %q, want discord", name)
	}
}

func TestDiscord_ChannelFiltering(t *testing.T) {
	d := NewDiscord("token", []string{"allowed-1", "allowed-2"})

	// Verify channels map was built correctly
	if !d.channels["allowed-1"] {
		t.Error("allowed-1 should be in channels map")
	}
	if !d.channels["allowed-2"] {
		t.Error("allowed-2 should be in channels map")
	}
	if d.channels["not-allowed"] {
		t.Error("not-allowed should not be in channels map")
	}
}

func TestDiscord_Messages(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})
	ch := d.Messages()
	if ch == nil {
		t.Error("Messages() should return non-nil channel")
	}
}

func TestDiscord_Stop_BeforeStart(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	// Stop before start should not panic
	err := d.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestDiscord_Stop_Idempotent(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	_ = d.Stop()
	err := d.Stop()
	if err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestDiscord_Send_NotConnected(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	err := d.Send("channel-1", "test")
	if err == nil {
		t.Error("Send() should error when not connected")
	}
}

func TestDiscord_SendFile_NotConnected(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	err := d.SendFile("channel-1", "test.md", []byte("content"))
	if err == nil {
		t.Error("SendFile() should error when not connected")
	}
}

func TestDiscord_EmptyChannelList(t *testing.T) {
	d := NewDiscord("token", []string{})
	if len(d.channels) != 0 {
		t.Errorf("expected empty channels map, got %d", len(d.channels))
	}
}

// TestDiscord_MessageIncludesAuthorID verifies that the Message struct
// includes the AuthorID field. Since handleMessage requires a live Discord
// session with discordgo objects, we verify the struct field exists and
// can be set correctly by constructing a Message directly as the handler would.
func TestDiscord_MessageIncludesAuthorID(t *testing.T) {
	// Simulate what handleMessage constructs
	msg := Message{
		ChannelID: "channel-123",
		Content:   "test message",
		Author:    "testuser",
		AuthorID:  "123456789012345678", // Discord snowflake ID
		Source:    "discord",
	}

	if msg.AuthorID == "" {
		t.Error("AuthorID should be populated")
	}
	if msg.AuthorID != "123456789012345678" {
		t.Errorf("AuthorID = %q, want %q", msg.AuthorID, "123456789012345678")
	}

	// Verify AuthorID is distinct from Author (display name)
	if msg.AuthorID == msg.Author {
		t.Error("AuthorID should be the snowflake ID, not the display name")
	}
}

// TestDiscord_MessageWithoutAuthorID verifies terminal-like messages
// (no AuthorID) still work correctly.
func TestDiscord_MessageWithoutAuthorID(t *testing.T) {
	msg := Message{
		ChannelID: "terminal",
		Content:   "test",
		Author:    "localuser",
		AuthorID:  "", // Terminal messages have no AuthorID
		Source:    "terminal",
	}

	if msg.AuthorID != "" {
		t.Error("terminal messages should have empty AuthorID")
	}
}
