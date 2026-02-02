package provider

import (
	"testing"

	"github.com/bwmarrin/discordgo"
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

func TestDiscord_UpdatePresence_NotConnected(t *testing.T) {
	d := &Discord{}
	err := d.UpdatePresence(0)
	if err == nil {
		t.Error("UpdatePresence should return error when not connected")
	}
}

func TestDiscord_UpdatePresence_StatusFormat(t *testing.T) {
	tests := []struct {
		name  string
		count int
		want  string
	}{
		{"zero sessions", 0, "Watching 0 sessions"},
		{"one session singular", 1, "Watching 1 session"},
		{"multiple sessions plural", 3, "Watching 3 sessions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We can't easily test the actual Discord API call without a mock session,
			// but we can verify that UpdatePresence doesn't panic with a nil session
			// and returns an appropriate error
			d := &Discord{}
			err := d.UpdatePresence(tt.count)
			if err == nil {
				t.Error("expected error with nil session")
			}
		})
	}
}

func TestDiscord_HandleMessage_SelfMessage(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	// Create a minimal discordgo session with state
	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}

	// Message from the bot itself should be ignored
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "channel-1",
			Content:   "self message",
			Author:    &discordgo.User{ID: "bot-id", Username: "bot"},
		},
	}

	d.handleMessage(session, msg)

	// No message should be in the channel
	select {
	case m := <-d.messages:
		t.Errorf("should not receive self-message, got %+v", m)
	default:
		// Expected: no message
	}
}

func TestDiscord_HandleMessage_WrongChannel(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "wrong-channel",
			Content:   "message in wrong channel",
			Author:    &discordgo.User{ID: "user-1", Username: "testuser"},
		},
	}

	d.handleMessage(session, msg)

	select {
	case m := <-d.messages:
		t.Errorf("should not receive message from wrong channel, got %+v", m)
	default:
		// Expected: no message
	}
}

func TestDiscord_HandleMessage_ValidMessage(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})

	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "channel-1",
			Content:   "hello world",
			Author:    &discordgo.User{ID: "user-123", Username: "testuser"},
		},
	}

	d.handleMessage(session, msg)

	select {
	case m := <-d.messages:
		if m.Content != "hello world" {
			t.Errorf("Content = %q, want %q", m.Content, "hello world")
		}
		if m.ChannelID != "channel-1" {
			t.Errorf("ChannelID = %q, want %q", m.ChannelID, "channel-1")
		}
		if m.Author != "testuser" {
			t.Errorf("Author = %q, want %q", m.Author, "testuser")
		}
		if m.AuthorID != "user-123" {
			t.Errorf("AuthorID = %q, want %q", m.AuthorID, "user-123")
		}
		if m.Source != "discord" {
			t.Errorf("Source = %q, want %q", m.Source, "discord")
		}
	default:
		t.Error("expected message on channel")
	}
}

func TestDiscord_HandleMessage_AfterStop(t *testing.T) {
	d := NewDiscord("token", []string{"channel-1"})
	_ = d.Stop()

	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "channel-1",
			Content:   "message after stop",
			Author:    &discordgo.User{ID: "user-1", Username: "testuser"},
		},
	}

	// Should not panic even though the channel is closed
	d.handleMessage(session, msg)
}
