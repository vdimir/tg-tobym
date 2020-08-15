package plugin

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/vdimir/tg-tobym/app/common"
)

// Help sends usage info
type Help struct {
	NopPlugin
	Bot  *tgbotapi.BotAPI
	Cmds []CommandDescription
}

func (plg *Help) formatHelpText(short bool, botName string) string {
	lines := []string{}
	for _, cmd := range plg.Cmds {
		lines = append(lines, fmt.Sprintf("/%s%s - %s", cmd.Cmd, botName, cmd.Help))
		if !short && cmd.Details != "" {
			lines = append(lines, cmd.Details)
		}
	}
	log.Printf("[TRACE] help %v", strings.Join(lines, "\n"))
	return strings.Join(lines, "\n")
}

func (plg *Help) Commands() []CommandDescription {
	return []CommandDescription{{
		Cmd:     "help",
		Help:    "Show usage",
		Details: "",
	}}
}

func (plg *Help) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (bool, error) {
	if upd.Message == nil {
		return false, nil
	}

	msgToMe := plg.Bot.IsMessageToMe(*upd.Message) || upd.Message.Chat.IsPrivate()
	if cmd := upd.Message.Command(); msgToMe && cmd == "help" || cmd == "help_short" {
		botName := ""
		if !upd.Message.Chat.IsPrivate() {
			botMe, err := plg.Bot.GetMe()
			if err != nil {
				return true, err
			}
			botName = "@" + botMe.UserName
		}
		args := upd.Message.CommandArguments()
		text := plg.formatHelpText(args == "short" || cmd == "help_short", botName)
		err := common.ReplyWithText(plg.Bot, upd.Message, text, "")
		return true, err
	}
	return false, nil
}
