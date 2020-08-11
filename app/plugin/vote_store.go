package plugin

import (
	"github.com/asdine/storm/v3"
)

type VoteStore struct {
	Bkt storm.Node
}

type msgID struct {
	messageID int
	chatID    int64
}

type MsgVote struct {
	ID    msgID `storm:"id"`
	Users map[int]int
}

func (s *VoteStore) AddVote(userID int, chatID int64, messageID int, increment int) (*MsgVote, error) {
	votesStore, err := s.Bkt.Begin(true)

	if err != nil {
		return nil, err
	}
	data := &MsgVote{
		ID: msgID{messageID: messageID, chatID: chatID},
	}

	err = votesStore.One("ID", data.ID, data)
	if err == storm.ErrNotFound {
		data = &MsgVote{
			ID: msgID{messageID: messageID, chatID: chatID},
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
		data.Users[userID] += increment
		err = votesStore.Update(data)
		if err != nil {
			votesStore.Rollback()
			return nil, err
		}
	} else {
		votesStore.Rollback()
		return nil, err
	}

	return data, votesStore.Commit()
}
