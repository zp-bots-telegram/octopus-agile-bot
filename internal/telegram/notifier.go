package telegram

import (
	"context"

	"github.com/go-telegram/bot"
)

// Notifier implements service.Notifier by sending Telegram messages.
type Notifier struct {
	b *bot.Bot
}

func NewNotifier(b *bot.Bot) *Notifier { return &Notifier{b: b} }

func (n *Notifier) Notify(ctx context.Context, chatID int64, text string) error {
	_, err := n.b.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text})
	return err
}
