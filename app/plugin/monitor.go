package plugin

import (
	"context"
	"log"

	"github.com/asdine/storm/v3"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/vdimir/tg-tobym/app/common"
)

// Monitor notifies subscribers on service update
type Monitor struct {
	NopPlugin
	Bot           *tgbotapi.BotAPI
	Store         storm.Node
	closeNotifier chan (struct{})
}

type subscriberData struct {
	ChatID     int64 `storm:"id"`
	Subscribed bool
}

func (plg *Monitor) Commands() []CommandDescription {
	return []CommandDescription{{
		Cmd:     "subscibe_to_service",
		Help:    "Subscribe to service events (e.g. startup)",
		Details: "To unsubscibe add 'off' argument",
	}}
}

func (plg *Monitor) subsctibeUser(chatID int64, enable bool) error {
	err := plg.Store.Save(&subscriberData{
		ChatID:     chatID,
		Subscribed: enable,
	})
	return err
}

func (plg *Monitor) notifySubscibersOnStartup() {
	subscribers := []subscriberData{}
	err := plg.Store.All(&subscribers)
	if err != nil {
		log.Printf("[ERROR] cannot get subscribers: %v", err)
		return
	}

	for _, s := range subscribers {
		select {
		case <-plg.closeNotifier:
			break
		default:
		}

		if s.Subscribed && s.ChatID != 0 {
			err := common.SentTextMessage(plg.Bot, s.ChatID, "Hello! I'm awake! You may send /version to me.", "")
			if err != nil {
				log.Printf("[ERROR] cannot send message to subscriber: %v", err)
			}
		}
	}
}

func (plg *Monitor) Init() error {
	plg.closeNotifier = make(chan struct{})
	go plg.notifySubscibersOnStartup()
	return nil
}

func (plg *Monitor) Close() error {
	close(plg.closeNotifier)
	return nil
}

func (plg *Monitor) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error) {
	if upd.Message == nil {
		return false, nil
	}

	msgToMe := plg.Bot.IsMessageToMe(*upd.Message) || upd.Message.Chat.IsPrivate()
	if cmd := upd.Message.Command(); msgToMe && cmd == "subscibe_to_service" {
		args := upd.Message.CommandArguments()
		enable := args != "off"

		err := plg.subsctibeUser(upd.Message.Chat.ID, enable)

		text := "Subscibed :ok_hand:"
		if err != nil {
			text = "Can't subscribe, internal error :("
			return true, err
		}
		if !enable {
			text = "Unsubscibed :ok_hand:"
		}
		err = common.ReplyWithText(plg.Bot, upd.Message, text, tgbotapi.ModeMarkdown)
		return true, err
	}
	return false, nil
}
