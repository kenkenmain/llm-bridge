//go:build integration

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func getTestToken(t *testing.T) string {
	t.Helper()
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" {
		t.Skip("DISCORD_BOT_TOKEN not set")
	}
	return token
}

func getTestChannelID(t *testing.T) string {
	t.Helper()
	if id := os.Getenv("DISCORD_TEST_CHANNEL_ID"); id != "" {
		return id
	}
	return config.DefaultDiscordTestChannelID
}

func startTestDiscord(t *testing.T) *Discord {
	t.Helper()
	token := getTestToken(t)
	channelID := getTestChannelID(t)

	d := NewDiscord(token, []string{channelID})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	if err := d.Start(ctx); err != nil {
		t.Fatalf("failed to start Discord: %v", err)
	}
	t.Cleanup(func() { _ = d.Stop() })

	// Allow Gateway connection to establish
	time.Sleep(2 * time.Second)
	return d
}

func TestDiscordIntegration_Connect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	token := getTestToken(t)
	channelID := getTestChannelID(t)

	d := NewDiscord(token, []string{channelID})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify session is connected
	if d.session == nil {
		t.Fatal("session should not be nil after Start")
	}
	if d.session.State.User == nil {
		t.Fatal("session user should not be nil after Start")
	}

	t.Logf("connected as %s#%s", d.session.State.User.Username, d.session.State.User.Discriminator)

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestDiscordIntegration_SendMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	d := startTestDiscord(t)
	channelID := getTestChannelID(t)

	msg := fmt.Sprintf("[TEST] integration test message at %s", time.Now().Format(time.RFC3339))
	if err := d.Send(channelID, msg); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestDiscordIntegration_SendFile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	d := startTestDiscord(t)
	channelID := getTestChannelID(t)

	content := []byte(fmt.Sprintf("[TEST] file content at %s", time.Now().Format(time.RFC3339)))
	if err := d.SendFile(channelID, "test-output.txt", content); err != nil {
		t.Fatalf("SendFile() error = %v", err)
	}
}

func TestDiscordIntegration_ChannelExists(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	d := startTestDiscord(t)
	channelID := getTestChannelID(t)

	ch, err := d.session.Channel(channelID)
	if err != nil {
		t.Fatalf("Channel() error = %v", err)
	}
	if ch.ID != channelID {
		t.Errorf("Channel ID = %q, want %q", ch.ID, channelID)
	}
	t.Logf("channel: #%s (type: %d)", ch.Name, ch.Type)
}

func TestDiscordIntegration_MessageRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	d := startTestDiscord(t)
	channelID := getTestChannelID(t)

	nonce := fmt.Sprintf("test-nonce-%d", time.Now().UnixNano())
	msg := fmt.Sprintf("[TEST] round-trip %s", nonce)

	if err := d.Send(channelID, msg); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	// Brief delay for message delivery
	time.Sleep(2 * time.Second)

	// Read recent messages and verify ours is present
	messages, err := d.session.ChannelMessages(channelID, 10, "", "", "")
	if err != nil {
		t.Fatalf("ChannelMessages() error = %v", err)
	}

	var found *discordgo.Message
	for _, m := range messages {
		if m.Content == msg {
			found = m
			break
		}
	}

	if found == nil {
		t.Errorf("sent message not found in recent channel messages")
	} else {
		t.Logf("round-trip verified: message ID %s", found.ID)
	}
}
