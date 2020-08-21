package plugin

import (
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/kyokomi/emoji"
	"github.com/pkg/errors"
	"github.com/vdimir/tg-tobym/app/common"

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
	Stat  *LastMessage
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

func (_ *VoteApp) Commands() []CommandDescription {
	return []CommandDescription{{
		Cmd:  "vote_stat",
		Help: "Show statisitcs",
	}}
}

func (vapp *VoteApp) isVotable(msg *tgbotapi.Message) int {
	if msg.From == nil {
		return 0
	}
	isReply := msg.ReplyToMessage != nil

	isTriggerToOther := msg.Text == "#vote" && isReply && !msg.ReplyToMessage.From.IsBot

	if isTriggerToOther {
		return msg.ReplyToMessage.MessageID
	}

	if msg.Text == "#vote" && !isReply {
		msg := vapp.Stat.Select(msg.Chat.ID, func(msg *tgbotapi.Message) bool {
			return !strings.Contains(msg.Text, "#vote") && !msg.IsCommand()
		})
		if msg != nil {
			return msg.MessageID
		}
	}

	containsHash := strings.Contains(msg.Text, "#vote") && len(msg.Text) > 5 && msg.ReplyToMessage == nil
	if containsHash {
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

type voteStat struct {
	TopVoters     []int
	TopCreators   []int
	WorstCreators []int
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

func smoothScore(score int) int {
	logit := 1.0 + math.Abs(float64(score))
	return int(math.Log2(logit))
}

func calcScore(votes *MsgVote, userID int) (info voteAggregateMsgInfo) {
	info.TotalUsers = len(votes.Users)

	for uid, score := range votes.Users {
		delta := smoothScore(score)
		if uid == userID {
			oldDelta := smoothScore(score - 1)
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

	info := calcScore(votedMsg, userID)
	log.Printf("[TRACE] calcScore: %v ", info)
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

type userScorePair struct {
	User  int
	Score int
}

type userScoreList []userScorePair

func (p userScoreList) Len() int           { return len(p) }
func (p userScoreList) Less(i, j int) bool { return p[i].Score > p[j].Score }
func (p userScoreList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func mapToUserScoreList(m map[int]int) userScoreList {
	res := make(userScoreList, len(m))
	for user, score := range m {
		res = append(res, userScorePair{User: user, Score: score})
	}
	return res
}

func (vapp *VoteApp) calcStat(startTs time.Time, msgChatID MsgChatID) (voteStat, error) {
	authorScores := map[int]int{}
	votersStat := map[int]int{}

	vapp.Store.Stat(startTs, msgChatID, func(votedMsg *MsgVote) {
		info := calcScore(votedMsg, 0)
		authorScores[votedMsg.Author] += info.Plus - info.Minus

		for user, logit := range votedMsg.Users {
			votersStat[user] += smoothScore(logit)
		}
	})

	stat := voteStat{}

	authorScoresArr := mapToUserScoreList(authorScores)

	n := len(authorScoresArr)
	for i := 0; i < len(authorScoresArr); i++ {
		if authorScoresArr[i].Score == authorScoresArr[0].Score {
			stat.TopCreators = append(stat.TopCreators, authorScoresArr[i].User)
		}
		if authorScoresArr[n-i-1].Score == authorScoresArr[n-1].Score {
			stat.WorstCreators = append(stat.TopCreators, authorScoresArr[n-i-1].User)
		}
	}

	votersStatArr := mapToUserScoreList(votersStat)
	for i := 0; i < len(votersStatArr); i++ {
		if votersStatArr[i].Score == votersStatArr[0].Score {
			stat.TopVoters = append(stat.TopVoters, votersStatArr[i].User)
		}
	}
	return stat, nil
}

func (vapp *VoteApp) handleMessage(msg *tgbotapi.Message) (err error) {
	if msgID := vapp.isVotable(msg); msgID != 0 {
		msgChatID := MsgChatID{
			MessageID: msgID,
			ChatID:    msg.Chat.ID,
		}
		if has, err := vapp.Store.HasVote(msgChatID); has {
			return nil
		} else if err == nil {
			_, err = vapp.Store.NewVote(msg.Time(), msgChatID, msg.From.ID)
		}
		if err != nil {
			return err
		}

		respMsg := tgbotapi.NewMessage(msg.Chat.ID, "let's score it, guys")
		respMsg.ReplyToMessageID = msgID

		respMsg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			voteAggregateMsgInfo{}.inlineKeyboardRow())
		_, err = vapp.Bot.Send(respMsg)
		return err
	}

	if common.CommandToBot(vapp.Bot, msg) == "vote_stat" {
		msgChatID := MsgChatID{
			MessageID: msg.MessageID,
			ChatID:    msg.Chat.ID,
		}
		startTs := msg.Time().AddDate(0, 0, -15)
		stat, err := vapp.calcStat(startTs, msgChatID)
		if err != nil {
			return errors.Wrapf(err, "calc stat error")
		}

		text := fmt.Sprintf("[TRACE] stat: %v", stat)

		err = common.SentTextMessage(vapp.Bot, msg.Chat.ID, text, "")
		if err != nil {
			return errors.Wrapf(err, "message send error")
		}
		return err
	}
	return nil
}
