package subapp

import (
	"context"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// SubApp provides part of functionality for bot
type SubApp interface {
	Init() error
	HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error)
	Close() error
}

// WebApp is a SubApp that shoould serve http
type WebApp interface {
	SubApp
	Routes() http.Handler
}