package provider

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type Discord struct {
	token    string
	channels map[string]bool

	mu       sync.Mutex
	session  *discordgo.Session
	messages chan Message
	stopped  bool
}

func NewDiscord(token string, channelIDs []string) *Discord {
	channels := make(map[string]bool)
	for _, id := range channelIDs {
		channels[id] = true
	}
	return &Discord{
		token:    token,
		channels: channels,
		messages: make(chan Message, 100),
	}
}

func (d *Discord) Name() string {
	return "discord"
}

func (d *Discord) Start(ctx context.Context) error {
	var err error
	d.session, err = discordgo.New("Bot " + d.token)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	d.session.AddHandler(d.handleMessage)
	d.session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages | discordgo.IntentMessageContent

	if err := d.session.Open(); err != nil {
		return fmt.Errorf("open session: %w", err)
	}

	return nil
}

func (d *Discord) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return nil
	}
	d.stopped = true

	if d.session != nil {
		d.session.Close()
	}
	close(d.messages)
	return nil
}

func (d *Discord) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !d.channels[m.ChannelID] {
		return
	}

	msg := Message{
		ChannelID: m.ChannelID,
		Content:   m.Content,
		Author:    m.Author.Username,
		AuthorID:  m.Author.ID,
		Source:    "discord",
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	select {
	case d.messages <- msg:
	default:
		// Channel full or closed, drop message
	}
}

func (d *Discord) Send(channelID string, content string) error {
	if d.session == nil {
		return fmt.Errorf("discord not connected")
	}
	_, err := d.session.ChannelMessageSend(channelID, content)
	return err
}

func (d *Discord) SendFile(channelID string, filename string, content []byte) error {
	if d.session == nil {
		return fmt.Errorf("discord not connected")
	}
	_, err := d.session.ChannelFileSend(channelID, filename, bytes.NewReader(content))
	return err
}

func (d *Discord) Messages() <-chan Message {
	return d.messages
}
