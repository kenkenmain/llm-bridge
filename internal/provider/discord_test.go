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

// TestDiscord_MessageWithoutAuthorID verifies messages without AuthorID
// still work correctly.
func TestDiscord_MessageWithoutAuthorID(t *testing.T) {
	msg := Message{
		ChannelID: "channel-123",
		Content:   "test",
		Author:    "user",
		AuthorID:  "",
		Source:    "discord",
	}

	if msg.AuthorID != "" {
		t.Error("messages without author ID should have empty AuthorID")
	}
}

func TestDiscord_MultipleChannels(t *testing.T) {
	d := NewDiscord("token", []string{"ch1", "ch2", "ch3"})
	if len(d.channels) != 3 {
		t.Errorf("expected 3 channels, got %d", len(d.channels))
	}
	for _, ch := range []string{"ch1", "ch2", "ch3"} {
		if !d.channels[ch] {
			t.Errorf("channel %q should be in map", ch)
		}
	}
}

func TestDiscord_DuplicateChannels(t *testing.T) {
	// Duplicate channel IDs should be deduplicated by the map
	d := NewDiscord("token", []string{"ch1", "ch1", "ch2"})
	if len(d.channels) != 2 {
		t.Errorf("expected 2 unique channels, got %d", len(d.channels))
	}
}

func TestDiscord_MessagesChannel_Buffered(t *testing.T) {
	d := NewDiscord("token", []string{"ch1"})
	// Verify the messages channel has buffer capacity
	ch := d.Messages()
	if cap(ch) != 100 {
		t.Errorf("messages channel capacity = %d, want 100", cap(ch))
	}
}

func TestDiscord_Stop_ClosesMessagesChannel(t *testing.T) {
	d := NewDiscord("token", []string{"ch1"})
	ch := d.Messages()

	_ = d.Stop()

	// After stop, channel should be closed
	_, open := <-ch
	if open {
		t.Error("messages channel should be closed after Stop()")
	}
}

func TestDiscord_Send_ErrorMessage(t *testing.T) {
	d := NewDiscord("token", []string{"ch1"})
	// session is nil, so Send should fail with specific error
	err := d.Send("ch1", "test")
	if err == nil || err.Error() != "discord not connected" {
		t.Errorf("Send() error = %v, want 'discord not connected'", err)
	}
}

func TestDiscord_SendFile_ErrorMessage(t *testing.T) {
	d := NewDiscord("token", []string{"ch1"})
	// session is nil, so SendFile should fail with specific error
	err := d.SendFile("ch1", "file.txt", []byte("data"))
	if err == nil || err.Error() != "discord not connected" {
		t.Errorf("SendFile() error = %v, want 'discord not connected'", err)
	}
}

func TestDiscord_StoppedFlag(t *testing.T) {
	d := NewDiscord("token", []string{"ch1"})

	if d.stopped {
		t.Error("stopped should be false initially")
	}

	_ = d.Stop()

	if !d.stopped {
		t.Error("stopped should be true after Stop()")
	}
}

func TestDiscord_Stop_WithSession(t *testing.T) {
	d := NewDiscord("token", []string{"ch1"})

	// Set a session directly to test the session.Close() path
	// The session will fail to close (no connection) but should not panic
	d.session = &discordgo.Session{}

	err := d.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if !d.stopped {
		t.Error("stopped should be true after Stop()")
	}
}

func TestDiscord_Token(t *testing.T) {
	d := NewDiscord("my-secret-token", []string{"ch1"})
	if d.token != "my-secret-token" {
		t.Errorf("token = %q, want %q", d.token, "my-secret-token")
	}
}

func TestDiscord_HandleMessage_BotMessage(t *testing.T) {
	d := NewDiscord("token", []string{"allowed-channel"})

	// Create a session with a bot user
	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}
	d.session = session

	// Create a message from the bot itself - should be ignored
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "allowed-channel",
			Content:   "bot message",
			Author:    &discordgo.User{ID: "bot-id", Username: "BotUser"},
		},
	}

	d.handleMessage(session, msg)

	// Should not receive any message (bot's own message is ignored)
	select {
	case <-d.Messages():
		t.Error("should not receive bot's own message")
	default:
		// Expected
	}
}

func TestDiscord_HandleMessage_DisallowedChannel(t *testing.T) {
	d := NewDiscord("token", []string{"allowed-channel"})

	// Create a session with a bot user
	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}
	d.session = session

	// Create a message from another user in a disallowed channel
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "other-channel", // Not in allowed channels
			Content:   "hello",
			Author:    &discordgo.User{ID: "user-id", Username: "TestUser"},
		},
	}

	d.handleMessage(session, msg)

	// Should not receive message from disallowed channel
	select {
	case <-d.Messages():
		t.Error("should not receive message from disallowed channel")
	default:
		// Expected
	}
}

func TestDiscord_HandleMessage_Success(t *testing.T) {
	d := NewDiscord("token", []string{"allowed-channel"})

	// Create a session with a bot user
	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}
	d.session = session

	// Create a valid message from another user in an allowed channel
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "allowed-channel",
			Content:   "hello world",
			Author:    &discordgo.User{ID: "user-123", Username: "TestUser"},
		},
	}

	d.handleMessage(session, msg)

	// Should receive the message
	select {
	case received := <-d.Messages():
		if received.ChannelID != "allowed-channel" {
			t.Errorf("ChannelID = %q, want %q", received.ChannelID, "allowed-channel")
		}
		if received.Content != "hello world" {
			t.Errorf("Content = %q, want %q", received.Content, "hello world")
		}
		if received.Author != "TestUser" {
			t.Errorf("Author = %q, want %q", received.Author, "TestUser")
		}
		if received.AuthorID != "user-123" {
			t.Errorf("AuthorID = %q, want %q", received.AuthorID, "user-123")
		}
		if received.Source != "discord" {
			t.Errorf("Source = %q, want %q", received.Source, "discord")
		}
	default:
		t.Error("expected to receive message")
	}
}

func TestDiscord_HandleMessage_Stopped(t *testing.T) {
	d := NewDiscord("token", []string{"allowed-channel"})

	// Create a session with a bot user
	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}
	d.session = session

	// Stop the discord first
	_ = d.Stop()

	// Try to send a message after stopping
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "allowed-channel",
			Content:   "hello",
			Author:    &discordgo.User{ID: "user-id", Username: "TestUser"},
		},
	}

	// This should not panic even though messages channel is closed
	d.handleMessage(session, msg)
}

func TestDiscord_HandleMessage_ChannelFull(t *testing.T) {
	// Create discord with a tiny buffer
	d := &Discord{
		token:    "token",
		channels: map[string]bool{"allowed-channel": true},
		messages: make(chan Message, 1), // Only 1 slot
	}

	// Create a session with a bot user
	session := &discordgo.Session{
		State: discordgo.NewState(),
	}
	session.State.User = &discordgo.User{ID: "bot-id"}
	d.session = session

	// Fill the channel
	msg1 := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "allowed-channel",
			Content:   "first",
			Author:    &discordgo.User{ID: "user-id", Username: "TestUser"},
		},
	}
	d.handleMessage(session, msg1)

	// Second message should be dropped (channel full) but not block
	msg2 := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "allowed-channel",
			Content:   "second",
			Author:    &discordgo.User{ID: "user-id", Username: "TestUser"},
		},
	}
	d.handleMessage(session, msg2)

	// Only first message should be in channel
	select {
	case received := <-d.Messages():
		if received.Content != "first" {
			t.Errorf("expected first message, got %q", received.Content)
		}
	default:
		t.Error("expected at least one message")
	}

	// Channel should be empty now (second message was dropped)
	select {
	case <-d.Messages():
		t.Error("channel should be empty after draining")
	default:
		// Expected
	}
}
