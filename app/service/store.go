package service

import (
	"log"

	"github.com/asdine/storm/v3"
)

// Storage stores data
type Storage struct {
	db *storm.DB
}

type MsgVote struct {
	ID int
	Users map[int]int
}

// NewStorage creates new Stroage
func NewStorage(path string) (*Storage, error)  {
	db, err := storm.Open(path)
	if err != nil {
		return nil, err
	}
	votesStore := db.From("votes")
	err = votesStore.Init(&MsgVote{})

	if err != nil {
		db.Close()
		return nil, err
	}
	return &Storage{
		db: db,
	}, nil
}

func (s *Storage) AddVote(userID int, messageID int, increment int) (*MsgVote,error) {
	votesStore, err := s.db.From("votes").Begin(true)
	log.Printf("[DEBUG] begin tx")
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

	log.Printf("[DEBUG] Commit tx")
	return data, votesStore.Commit()
}

// Close storage
func (s *Storage) Close() error {
	return s.db.Close()
}
