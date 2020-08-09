package subapp

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/vdimir/tg-tobym/app/store"
)

type ShowVersion struct {
	bot     *tgbotapi.BotAPI
	Version string
}

type ShowVersionConfig struct {
	Version string
}

func NewShowVersionApp(bot *tgbotapi.BotAPI, store *store.Storage, cfg *ShowVersionConfig) (*ShowVersion, error) {
	return &ShowVersion{
		bot:     bot,
		Version: cfg.Version,
	}, nil
}

// Init setup ShowVersion
func (sapp *ShowVersion) Init() (err error) {
	return nil
}

// HandleUpdate processes event
func (sapp *ShowVersion) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (cont bool, err error) {
	if upd.Message != nil && (sapp.bot.IsMessageToMe(*upd.Message) || upd.Message.Chat.IsPrivate()) {
		log.Printf("[DEBUG] cmd %s", upd.Message.Command())

		if cmd := upd.Message.Command(); cmd == "version" {
			resp := tgbotapi.NewMessage(upd.Message.Chat.ID, fmt.Sprintf("`%s`", sapp.Version))
			resp.ParseMode = tgbotapi.ModeMarkdownV2
			_, err = sapp.bot.Send(resp)
			return false, err
		}
	}
	return true, err
}

// Close shutdown ShowVersion
func (sapp *ShowVersion) Close() (err error) {
	return nil
}
