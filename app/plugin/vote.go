package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/hashicorp/go-multierror"
)

const voteCallbackDataPrefix = "vote"
const voteCallbackDataInc = voteCallbackDataPrefix + "+"
const voteCallbackDataDec = voteCallbackDataPrefix + "-"

// VoteApp sends vote form for particular messages in chats
type VoteApp struct {
	NopPlugin
	Bot   *tgbotapi.BotAPI
	Store *VoteStore
}

// HandleUpdate processes event
func (vapp *VoteApp) HandleUpdate(ctx context.Context, upd *tgbotapi.Update) (caught bool, err error) {
	if upd.Message != nil {
		err = vapp.handleMessage(upd.Message)
	}
	if upd.CallbackQuery != nil {
		err = vapp.handleCallcackQuery(upd.CallbackQuery)
	}
	return false, err
}

func (vapp *VoteApp) isVotable(msg *tgbotapi.Message) int {
	isTriggerToOther := msg.Text == "#vote" && msg.ReplyToMessage != nil
	if isTriggerToOther {
		return msg.ReplyToMessage.MessageID
	}

	isForward := msg.ForwardFromChat != nil
	if isForward {
		return msg.MessageID
	}
	return 0
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

	votedMsg, err := vapp.Store.AddVote(msg.Message.Chat.ID, voteMsgID, msg.From.ID, increment)

	if err != nil {
		return info, err
	}

	for _, inc := range votedMsg.Users {
		logit := 1.0 + math.Abs(float64(inc))
		if inc > 0 {
			info.Plus += int(math.Log2(logit))
		}
		if inc < 0 {
			info.Minus += int(math.Log2(logit))
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
	if msgID := vapp.isVotable(msg); msgID != 0 {
		respMsg := tgbotapi.NewMessage(msg.Chat.ID, "let's vote it, guys")
		respMsg.ReplyToMessageID = msgID

		respMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			voteAggregateMsgInfo{}.inlineKeyboardRow())
		_, err = vapp.Bot.Send(respMsg)
	}
	return err
}
