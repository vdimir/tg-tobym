package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/kyokomi/emoji"

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
	TotalUsers       int
	CurrentUserTotal int
	Plus             int
	Minus            int
	Modified         bool
}

func arrowSign(score int) string {
	if score > 0 {
		return emoji.Sprintf(":right_arrow_curving_up:")
	}
	if score < 0 {
		return emoji.Sprintf(":right_arrow_curving_down:")
	}
	return ""
}

func (info voteAggregateMsgInfo) inlineKeyboardRow() []tgbotapi.InlineKeyboardButton {
	plusText := arrowSign(+1)
	if info.Plus > 0 {
		plusText = fmt.Sprintf("%s %d", plusText, info.Plus)
	}

	minusText := arrowSign(-1)
	if info.Minus > 0 {
		minusText = fmt.Sprintf("%s %d", minusText, info.Minus)
	}

	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(plusText, voteCallbackDataInc),
		tgbotapi.NewInlineKeyboardButtonData(minusText, voteCallbackDataDec),
	)
}

func (vapp *VoteApp) calcScore(votes *MsgVote, userID int) (info voteAggregateMsgInfo) {
	info.TotalUsers = len(votes.Users)

	for uid, score := range votes.Users {
		logit := 1.0 + math.Abs(float64(score))
		delta := int(math.Log2(logit))
		if uid == userID {
			oldDelta := int(math.Log2(logit - 1.0))
			info.Modified = delta != oldDelta

			info.CurrentUserTotal = score
		}
		if score > 0 {
			info.Plus += delta
		} else if score < 0 {
			info.Minus += delta
		}
	}
	return info
}

func (vapp *VoteApp) updateVote(msg *tgbotapi.CallbackQuery) (voteAggregateMsgInfo, error) {
	increment := 1
	if msg.Data == voteCallbackDataDec {
		increment = -1
	}

	msgID := MsgChatID{
		MessageID: msg.Message.ReplyToMessage.MessageID,
		ChatID:    msg.Message.Chat.ID,
	}

	userID := msg.From.ID
	storeModified, votedMsg, err := vapp.Store.AddVote(msg.Message.Time(), msgID, userID, increment)
	if err != nil {
		return voteAggregateMsgInfo{}, err
	}

	info := vapp.calcScore(votedMsg, userID)
	info.Modified = info.Modified && storeModified

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

		text := emoji.Sprintf("%+d / %d", voteAgg.CurrentUserTotal, voteAgg.TotalUsers)
		if voteAgg.Modified {
			text = emoji.Sprintf("%s %s", arrowSign(voteAgg.CurrentUserTotal), text)
		}

		_, err = vapp.Bot.AnswerCallbackQuery(tgbotapi.NewCallback(msg.ID, text))
		errs = multierror.Append(errs, err)

		if voteAgg.Modified {
			_, err = vapp.Bot.Send(replyMsg)
			errs = multierror.Append(errs, err)
		}
	}
	return errs.ErrorOrNil()
}

func (vapp *VoteApp) handleMessage(msg *tgbotapi.Message) (err error) {
	if msgID := vapp.isVotable(msg); msgID != 0 {
		msgChatID := MsgChatID{
			MessageID: msgID,
			ChatID:    msg.Chat.ID,
		}
		if has, err := vapp.Store.HasVote(msgChatID); has {
			respMsg := tgbotapi.NewMessage(msg.Chat.ID, "Already exists")
			_, err = vapp.Bot.Send(respMsg)
			return err
		} else if err != nil {
			return err
		}

		respMsg := tgbotapi.NewMessage(msg.Chat.ID, "let's vote it, guys")
		respMsg.ReplyToMessageID = msgID

		respMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			voteAggregateMsgInfo{}.inlineKeyboardRow())
		_, err = vapp.Bot.Send(respMsg)
	}
	return err
}
