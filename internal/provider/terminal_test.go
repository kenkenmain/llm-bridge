package provider

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestNewTerminal(t *testing.T) {
	term := NewTerminal("test-channel")
	if term.channelID != "test-channel" {
		t.Errorf("channelID = %q, want %q", term.channelID, "test-channel")
	}
	if term.messages == nil {
		t.Error("messages channel should be initialized")
	}
}

func TestTerminal_Name(t *testing.T) {
	term := NewTerminal("test")
	if name := term.Name(); name != "terminal" {
		t.Errorf("Name() = %q, want %q", name, "terminal")
	}
}

func TestTerminal_ChannelID(t *testing.T) {
	term := NewTerminal("my-channel")
	if id := term.ChannelID(); id != "my-channel" {
		t.Errorf("ChannelID() = %q, want %q", id, "my-channel")
	}
}

func TestTerminal_Send(t *testing.T) {
	var buf bytes.Buffer
	term := &Terminal{
		channelID: "test",
		writer:    &buf,
		messages:  make(chan Message, 10),
	}

	if err := term.Send("test-channel", "hello world"); err != nil {
		t.Errorf("Send() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "hello world") {
		t.Errorf("Send() output = %q, want to contain %q", got, "hello world")
	}
}

func TestTerminal_SendFile(t *testing.T) {
	var buf bytes.Buffer
	term := &Terminal{
		channelID: "test",
		writer:    &buf,
		messages:  make(chan Message, 10),
	}

	if err := term.SendFile("test-channel", "test.md", []byte("file content")); err != nil {
		t.Errorf("SendFile() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "test.md") {
		t.Errorf("SendFile() output should contain filename, got %q", got)
	}
	if !strings.Contains(got, "file content") {
		t.Errorf("SendFile() output should contain content, got %q", got)
	}
}

func TestTerminal_ReadLoop(t *testing.T) {
	input := "line1\nline2\n"
	reader := strings.NewReader(input)

	term := &Terminal{
		channelID: "test",
		reader:    reader,
		writer:    &bytes.Buffer{},
		messages:  make(chan Message, 10),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := term.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait for messages
	time.Sleep(50 * time.Millisecond)

	// Check messages received
	select {
	case msg := <-term.Messages():
		if msg.Content != "line1" {
			t.Errorf("first message = %q, want %q", msg.Content, "line1")
		}
		if msg.Source != "terminal" {
			t.Errorf("source = %q, want %q", msg.Source, "terminal")
		}
	default:
		t.Error("expected message on channel")
	}
}

func TestTerminal_Stop(t *testing.T) {
	term := &Terminal{
		channelID: "test",
		reader:    strings.NewReader(""),
		writer:    &bytes.Buffer{},
		messages:  make(chan Message, 10),
	}

	// Stop should be idempotent
	if err := term.Stop(); err != nil {
		t.Errorf("first Stop() error = %v", err)
	}
	if err := term.Stop(); err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestTerminal_StopPreventsSend(t *testing.T) {
	// Use a pipe so we can control when data is available
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	term := &Terminal{
		channelID: "test",
		reader:    pr,
		writer:    &bytes.Buffer{},
		messages:  make(chan Message, 10),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := term.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Send a message
	pw.Write([]byte("line1\n"))
	time.Sleep(20 * time.Millisecond)

	// Stop the terminal
	if err := term.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify we can drain messages without blocking
	timeout := time.After(50 * time.Millisecond)
	for {
		select {
		case <-term.Messages():
			continue
		case <-timeout:
			return
		}
	}
}
