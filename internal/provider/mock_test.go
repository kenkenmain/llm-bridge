package provider

import (
	"context"
	"testing"
)

func TestNewMockProvider(t *testing.T) {
	m := NewMockProvider("test")
	if m.Name() != "test" {
		t.Errorf("Name() = %q, want test", m.Name())
	}
}

func TestMockProvider_Start(t *testing.T) {
	m := NewMockProvider("test")
	err := m.Start(context.Background())
	if err != nil {
		t.Errorf("Start() error = %v", err)
	}
	if !m.WasStartCalled() {
		t.Error("Start was not recorded")
	}
}

func TestMockProvider_StartWithError(t *testing.T) {
	m := NewMockProvider("test")
	m.SetStartError(context.DeadlineExceeded)
	err := m.Start(context.Background())
	if err != context.DeadlineExceeded {
		t.Errorf("Start() error = %v, want DeadlineExceeded", err)
	}
}

func TestMockProvider_Stop(t *testing.T) {
	m := NewMockProvider("test")
	err := m.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
	if !m.WasStopCalled() {
		t.Error("Stop was not recorded")
	}
}

func TestMockProvider_StopIdempotent(t *testing.T) {
	m := NewMockProvider("test")
	m.Stop()
	err := m.Stop()
	if err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestMockProvider_Send(t *testing.T) {
	m := NewMockProvider("test")
	err := m.Send("channel-1", "hello")
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	msgs := m.GetSentMessages()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ChannelID != "channel-1" || msgs[0].Content != "hello" {
		t.Errorf("message = %+v", msgs[0])
	}
}

func TestMockProvider_SendWithError(t *testing.T) {
	m := NewMockProvider("test")
	m.SetSendError(context.DeadlineExceeded)
	err := m.Send("channel-1", "hello")
	if err != context.DeadlineExceeded {
		t.Errorf("Send() error = %v, want DeadlineExceeded", err)
	}
}

func TestMockProvider_SendFile(t *testing.T) {
	m := NewMockProvider("test")
	err := m.SendFile("channel-1", "test.md", []byte("content"))
	if err != nil {
		t.Errorf("SendFile() error = %v", err)
	}

	files := m.GetSentFiles()
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Filename != "test.md" || string(files[0].Content) != "content" {
		t.Errorf("file = %+v", files[0])
	}
}

func TestMockProvider_SimulateMessage(t *testing.T) {
	m := NewMockProvider("test")

	m.SimulateMessage(Message{
		ChannelID: "chan-1",
		Content:   "test message",
		Author:    "user",
		Source:    "test",
	})

	select {
	case msg := <-m.Messages():
		if msg.Content != "test message" {
			t.Errorf("message content = %q", msg.Content)
		}
	default:
		t.Error("expected message on channel")
	}
}

func TestMockProvider_SimulateMessageAfterStop(t *testing.T) {
	m := NewMockProvider("test")
	m.Stop()

	// Should not panic
	m.SimulateMessage(Message{Content: "test"})
}
