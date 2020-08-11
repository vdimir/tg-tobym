package plugin

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type ShowVersion struct {
	NopPlugin
	Bot     *tgbotapi.BotAPI
	Version string
}

// HandleUpdate processes event
func (sapp *ShowVersion) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (caught bool, err error) {
	if upd.Message != nil && (sapp.Bot.IsMessageToMe(*upd.Message) || upd.Message.Chat.IsPrivate()) {
		if cmd := upd.Message.Command(); cmd == "version" {
			resp := tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("`%s`", sapp.Version))
			resp.ParseMode = tgbotapi.ModeMarkdownV2
			_, err = sapp.Bot.Send(resp)
			return true, err
		}
	}
	return false, err
}
