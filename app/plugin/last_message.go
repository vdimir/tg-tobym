package plugin

import (
	"container/ring"
	"context"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// LastMessage tracks last message in chat
type LastMessage struct {
	lastMsg map[int64]*ring.Ring
	mtx     sync.RWMutex
}

func (plg *LastMessage) Init() (err error) {
	plg.lastMsg = map[int64]*ring.Ring{}
	return nil
}

func (plg *LastMessage) Commands() []CommandDescription {
	return []CommandDescription{}
}

func (plg *LastMessage) HandleUpdate(_ context.Context, upd *tgbotapi.Update) (bool, error) {
	if upd.Message == nil {
		return false, nil
	}

	fromRealUser := upd.Message.From != nil && !upd.Message.From.IsBot
	if !fromRealUser {
		return false, nil
	}

	chatID := upd.Message.Chat.ID

	plg.mtx.Lock()
	defer plg.mtx.Unlock()

	msgList, ok := plg.lastMsg[chatID]
	if !ok {
		msgList = ring.New(5)
	} else {
		msgList = plg.lastMsg[chatID].Next()
	}
	msgList.Value = upd.Message
	plg.lastMsg[chatID] = msgList

	return false, nil
}

func (plg *LastMessage) Close() (err error) {
	return nil
}

func (plg *LastMessage) LastMessageID(chatID int64) int {
	plg.mtx.RLock()
	defer plg.mtx.RUnlock()

	msgList, ok := plg.lastMsg[chatID]
	if !ok {
		return 0
	}
	return msgList.Value.(*tgbotapi.Message).MessageID
}

func (plg *LastMessage) Select(chatID int64, pred func(*tgbotapi.Message) bool) *tgbotapi.Message {
	plg.mtx.RLock()
	defer plg.mtx.RUnlock()

	msgList, ok := plg.lastMsg[chatID]
	if !ok {
		return nil
	}

	var found *tgbotapi.Message
	msgList.Do(func(val interface{}) {
		if val == nil {
			return
		}
		currentMsg := val.(*tgbotapi.Message)
		if pred(currentMsg) && (found == nil || found.MessageID < currentMsg.MessageID) {
			found = currentMsg
		}
	})
	return found
}
