package plugin

import (
	"github.com/asdine/storm/v3"
)

type VoteStore struct {
	Bkt storm.Node
}

type msgID struct {
	MessageID int
	ChatID    int64
}

type MsgVote struct {
	ID    msgID `storm:"id"`
	Users map[int]int
}

func (s *VoteStore) AddVote(chatID int64, messageID int, userID int, increment int) (*MsgVote, error) {
	votesStore, err := s.Bkt.Begin(true)

	if err != nil {
		return nil, err
	}
	data := &MsgVote{
		ID: msgID{MessageID: messageID, ChatID: chatID},
	}

	err = votesStore.One("ID", data.ID, data)
	if err == storm.ErrNotFound {
		data = &MsgVote{
			ID: msgID{MessageID: messageID, ChatID: chatID},
			Users: map[int]int{
				userID: increment,
			},
		}
		err = votesStore.Save(data)
		if err != nil {
			votesStore.Rollback()
			return nil, err
		}
	} else if err == nil {
		if data.Users[userID]*increment >= 0 {
			data.Users[userID] += increment
			err = votesStore.Update(data)
			if err != nil {
				votesStore.Rollback()
				return nil, err
			}
		}
	} else {
		votesStore.Rollback()
		return nil, err
	}

	return data, votesStore.Commit()
}
