package plugin

import (
	"context"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type CommandDescription struct {
	Cmd     string
	Help    string
	Details string
}

// PlugIn provides part of functionality for bot
type PlugIn interface {
	Init() error
	Commands() []CommandDescription
	HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (caught bool, err error)
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

func (sapp *NopPlugin) Commands() []CommandDescription {
	return []CommandDescription{}
}

func (sapp *NopPlugin) HandleUpdate(_ context.Context, _ *tgbotapi.Update) (bool, error) {
	return false, nil
}

func (sapp *NopPlugin) Close() (err error) {
	return nil
}
