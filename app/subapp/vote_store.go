package subapp

import (
	"github.com/asdine/storm/v3"
	"github.com/vdimir/tg-tobym/app/store"
)

type VoteStore struct {
	Store *store.Storage
}

type MsgVote struct {
	ID    int
	Users map[int]int
}

func (s *VoteStore) AddVote(userID int, messageID int, increment int) (*MsgVote, error) {
	votesStore, err := s.Store.DB.From("votes").Begin(true)

	if err != nil {
		return nil, err
	}
	data := &MsgVote{}

	err = votesStore.One("ID", messageID, data)
	if err == storm.ErrNotFound {
		data = &MsgVote{
			ID: messageID,
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
