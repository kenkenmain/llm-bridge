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

	d.Stop()
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
