package plugin

import (
	"context"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// PlugIn provides part of functionality for bot
type PlugIn interface {
	Init() error
	HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error)
	Close() error
}

// WebApp is a PlugIn that shoould serve http
type WebApp interface {
	PlugIn
	Routes() http.Handler
}

// NopPlugin does nothing
type NopPlugin struct{}

func (sapp *NopPlugin) Init() (err error) {
	return nil
}

func (sapp *NopPlugin) HandleUpdate(_ context.Context, _ *tgbotapi.Update) (bool, error) {
	return true, nil
}

func (sapp *NopPlugin) Close() (err error) {
	return nil
}
