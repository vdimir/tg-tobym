package subapp

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type ShowVersion struct {
	Bot     *tgbotapi.BotAPI
	Version string
}

// Init setup ShowVersion
func (sapp *ShowVersion) Init() (err error) {
	return nil
}

// HandleUpdate processes event
func (sapp *ShowVersion) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (cont bool, err error) {
	if upd.Message != nil && (sapp.Bot.IsMessageToMe(*upd.Message) || upd.Message.Chat.IsPrivate()) {
		if cmd := upd.Message.Command(); cmd == "version" {
			resp := tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("`%s`", sapp.Version))
			resp.ParseMode = tgbotapi.ModeMarkdownV2
			_, err = sapp.Bot.Send(resp)
			return false, err
		}
	}
	return true, err
}

// Close shutdown ShowVersion
func (sapp *ShowVersion) Close() (err error) {
	return nil
}
