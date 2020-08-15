package common

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
)

// SentTextMessage is a shortcut for sending text message
func SentTextMessage(bot *tgbotapi.BotAPI, chatID int64, text string, parseMode string) error {
	resp := tgbotapi.NewMessage(chatID, emoji.Sprint(text))
	if parseMode != "" {
		resp.ParseMode = parseMode
	}

	_, err := bot.Send(resp)
	return errors.Wrapf(err, "cannot send message")
}

// ReplyWithText send message to chat
func ReplyWithText(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, text string, parseMode string) error {
	if msg == nil {
		return errors.Errorf("message is nil")
	}
	return SentTextMessage(bot, msg.Chat.ID, text, parseMode)
}
