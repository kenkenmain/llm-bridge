package provider

import (
	"context"
)

// Message represents a chat message
type Message struct {
	ChannelID string
	Content   string
	Author    string
	Source    string // provider name
}

// Provider defines the interface for chat providers
type Provider interface {
	// Name returns the provider name ("discord", "telegram", "terminal")
	Name() string

	// Start connects to the chat service
	Start(ctx context.Context) error

	// Stop disconnects from the chat service
	Stop() error

	// Send sends a message to a channel
	Send(channelID string, content string) error

	// SendFile sends a file to a channel
	SendFile(channelID string, filename string, content []byte) error

	// Messages returns a channel of incoming messages
	Messages() <-chan Message
}
