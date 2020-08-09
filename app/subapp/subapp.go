package subapp

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"github.com/vdimir/tg-tobym/app/store"
)

// SubApp provides part of functionality for bot
type SubApp interface {
	Init() error
	HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error)
	Close() error
}

// NewSubApp creates new subapp based on config
func NewSubApp(bot *tgbotapi.BotAPI, store *store.Storage, cfg interface{}) (SubApp, error) {
	switch cfg := cfg.(type) {
	case *VoteAppConfig:
		return NewVoteApp(bot, store, cfg)
	case *ShowVersionConfig:
		return NewShowVersionApp(bot, store, cfg)
	}
	return nil, errors.Errorf("unknown subapp config")
}
