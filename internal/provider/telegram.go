package provider

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
)

type Telegram struct {
	token    string
	channels map[string]bool

	mu       sync.Mutex
	bot      *tele.Bot
	messages chan Message
	stopped  bool
}

func NewTelegram(token string, channelIDs []string) *Telegram {
	channels := make(map[string]bool)
	for _, id := range channelIDs {
		channels[id] = true
	}
	return &Telegram{
		token:    token,
		channels: channels,
		messages: make(chan Message, 100),
	}
}

func (t *Telegram) Name() string {
	return "telegram"
}

func (t *Telegram) Start(ctx context.Context) error {
	pref := tele.Settings{
		Token:  t.token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	var err error
	t.bot, err = tele.NewBot(pref)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	t.bot.Handle(tele.OnText, func(c tele.Context) error {
		chatID := strconv.FormatInt(c.Chat().ID, 10)

		if !t.channels[chatID] {
			return nil
		}

		t.mu.Lock()
		stopped := t.stopped
		t.mu.Unlock()

		if stopped {
			return nil
		}

		t.messages <- Message{
			ChannelID: chatID,
			Content:   c.Text(),
			Author:    c.Sender().Username,
			Source:    "telegram",
		}
		return nil
	})

	go t.bot.Start()
	return nil
}

func (t *Telegram) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stopped {
		return nil
	}
	t.stopped = true

	if t.bot != nil {
		t.bot.Stop()
	}
	close(t.messages)
	return nil
}

func (t *Telegram) Send(channelID string, content string) error {
	if t.bot == nil {
		return fmt.Errorf("telegram not connected")
	}

	chatID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	_, err = t.bot.Send(&tele.Chat{ID: chatID}, content)
	return err
}

func (t *Telegram) SendFile(channelID string, filename string, content []byte) error {
	if t.bot == nil {
		return fmt.Errorf("telegram not connected")
	}

	chatID, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	doc := &tele.Document{
		File:     tele.FromReader(bytes.NewReader(content)),
		FileName: filename,
	}
	_, err = t.bot.Send(&tele.Chat{ID: chatID}, doc)
	return err
}

func (t *Telegram) Messages() <-chan Message {
	return t.messages
}
