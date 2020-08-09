package subapp

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// SubApp provides part of functionality for bot
type SubApp interface {
	Init() error
	HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error)
	Close() error
}
