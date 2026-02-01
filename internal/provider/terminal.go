package provider

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// Terminal provides local stdin/stdout as a provider
type Terminal struct {
	channelID string
	reader    io.Reader
	writer    io.Writer

	mu       sync.Mutex
	messages chan Message
	stopped  bool
}

func NewTerminal(channelID string) *Terminal {
	return &Terminal{
		channelID: channelID,
		reader:    os.Stdin,
		writer:    os.Stdout,
		messages:  make(chan Message, 100),
	}
}

func (t *Terminal) Name() string {
	return "terminal"
}

func (t *Terminal) Start(ctx context.Context) error {
	go t.readLoop(ctx)
	return nil
}

func (t *Terminal) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(t.reader)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg := Message{
			ChannelID: t.channelID,
			Content:   scanner.Text(),
			Author:    "terminal",
			Source:    "terminal",
		}

		t.mu.Lock()
		if t.stopped {
			t.mu.Unlock()
			return
		}

		select {
		case t.messages <- msg:
		default:
			// Channel full or closed, drop message
		}
		t.mu.Unlock()
	}
}

func (t *Terminal) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped {
		return nil
	}
	t.stopped = true
	close(t.messages)
	return nil
}

func (t *Terminal) Send(channelID string, content string) error {
	_, err := fmt.Fprintln(t.writer, content)
	return err
}

func (t *Terminal) SendFile(channelID string, filename string, content []byte) error {
	_, err := fmt.Fprintf(t.writer, "--- %s ---\n%s\n--- end ---\n", filename, string(content))
	return err
}

func (t *Terminal) Messages() <-chan Message {
	return t.messages
}

func (t *Terminal) ChannelID() string {
	return t.channelID
}
