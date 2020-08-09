package subapp

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/vdimir/tg-tobym/app/store"
)

// SubApp provides part of functionality for bot
type SubApp interface {
	Init() error
	HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error)
	Close() error
}

// Factory allows to create subapps
type Factory interface {
	NewSubApp(bot *tgbotapi.BotAPI, store *store.Storage) (SubApp, error)
}
