package subapp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/hashicorp/go-multierror"
)

const voteCallbackDataPrefix = "vote"
const voteCallbackDataInc = voteCallbackDataPrefix + "+"
const voteCallbackDataDec = voteCallbackDataPrefix + "-"

// VoteApp sends vote form for particular messages in chats
type VoteApp struct {
	NopSubapp
	Bot   *tgbotapi.BotAPI
	Store *VoteStore
}

// HandleUpdate processes event
func (vapp *VoteApp) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (cont bool, err error) {
	if upd.Message != nil {
		err = vapp.handleMessage(upd.Message)
	}
	if upd.CallbackQuery != nil {
		err = vapp.handleCallcackQuery(upd.CallbackQuery)
	}
	return true, err
}

func (vapp *VoteApp) isVotable(msg *tgbotapi.Message) bool {
	return msg.Photo != nil || (msg.Text == "#vote" && msg.ReplyToMessage != nil)
}

type voteAggregateMsgInfo struct {
	Plus  int
	Minus int
}

func (info voteAggregateMsgInfo) inlineKeyboardRow() []tgbotapi.InlineKeyboardButton {
	plusText := "+"
	if info.Plus > 0 {
		plusText = fmt.Sprintf("+%d", info.Plus)
	}

	minusText := "-"
	if info.Minus > 0 {
		minusText = fmt.Sprintf("-%d", info.Minus)
	}

	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(plusText, voteCallbackDataInc),
		tgbotapi.NewInlineKeyboardButtonData(minusText, voteCallbackDataDec),
	)
}

func (vapp *VoteApp) updateVote(msg *tgbotapi.CallbackQuery) (info voteAggregateMsgInfo, err error) {
	voteMsgID := msg.Message.ReplyToMessage.MessageID
	increment := 0
	if msg.Data == voteCallbackDataInc {
		increment = 1
	} else {
		increment = -1
	}

	votedMsg, err := vapp.Store.AddVote(msg.From.ID, voteMsgID, increment)

	if err != nil {
		return info, err
	}

	for _, inc := range votedMsg.Users {
		if inc > 0 {
			info.Plus += inc
		}
		if inc < 0 {
			info.Minus -= inc
		}
	}
	return info, nil
}

func (vapp *VoteApp) handleCallcackQuery(msg *tgbotapi.CallbackQuery) error {
	errs := &multierror.Error{}
	if strings.HasPrefix(msg.Data, voteCallbackDataPrefix) {
		if msg.Message.ReplyToMessage == nil {
			return errors.New("vote callback is not a reply")
		}
		voteAgg, err := vapp.updateVote(msg)
		if err != nil {
			return err
		}

		replyMsg := tgbotapi.NewEditMessageReplyMarkup(
			msg.Message.Chat.ID, msg.Message.MessageID,
			tgbotapi.NewInlineKeyboardMarkup(voteAgg.inlineKeyboardRow()),
		)
		_, err = vapp.Bot.AnswerCallbackQuery(tgbotapi.NewCallback(msg.ID, "ok"))
		errs = multierror.Append(errs, err)

		_, err = vapp.Bot.Send(replyMsg)
		errs = multierror.Append(errs, err)
	}
	return errs.ErrorOrNil()
}

func (vapp *VoteApp) handleMessage(msg *tgbotapi.Message) (err error) {
	if vapp.isVotable(msg) {
		respMsg := tgbotapi.NewMessage(msg.Chat.ID, "let's vote it, guys")

		respMsg.ReplyToMessageID = msg.MessageID
		if msg.ReplyToMessage != nil {
			respMsg.ReplyToMessageID = msg.ReplyToMessage.MessageID
		}

		respMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			voteAggregateMsgInfo{}.inlineKeyboardRow())
		_, err = vapp.Bot.Send(respMsg)
	}
	return err
}
