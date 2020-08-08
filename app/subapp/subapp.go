package subapp

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// SubApp provides part of functionality for bot
type SubApp interface {
	HandleUpdate(upd *tgbotapi.Update) (bool, error)
}
