package plugin

import (
	"log"
	"sync"
	"time"

	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/q"
	"github.com/pkg/errors"
)

type MsgChatID struct {
	MessageID int
	ChatID    int64
}

type VoteStore struct {
	Bkt        storm.Node
	perChatMtx sync.Map
}

type MsgVote struct {
	ID        MsgChatID `storm:"id"`
	Timestamp int64     `storm:"index"`
	Author    int
	Users     map[int]int
}

func NewVoteStore(bkt storm.Node) *VoteStore {
	err := bkt.ReIndex(&MsgVote{})
	log.Printf("[WARN]  Reindex vote store, Err: %v", err)

	return &VoteStore{
		Bkt: bkt,
	}
}

func (s *VoteStore) lockChat(msg MsgChatID) *sync.RWMutex {
	lk, ok := s.perChatMtx.Load(msg.ChatID)
	if !ok {
		lk, _ = s.perChatMtx.LoadOrStore(msg.ChatID, new(sync.RWMutex))
	}
	return lk.(*sync.RWMutex)
}

func (s *VoteStore) HasVote(msg MsgChatID) (bool, error) {
	lk := s.lockChat(msg)
	lk.RLock()
	defer lk.RUnlock()

	data := &MsgVote{ID: msg}
	err := s.Bkt.One("ID", data.ID, data)

	if err == nil {
		return true, nil
	} else if err == storm.ErrNotFound {
		return false, nil
	}
	return false, err
}

func (s *VoteStore) NewVote(ts time.Time, msg MsgChatID, userID int) (*MsgVote, error) {
	lk := s.lockChat(msg)
	lk.Lock()
	defer lk.Unlock()

	data := &MsgVote{
		ID:        msg,
		Users:     map[int]int{},
		Author:    userID,
		Timestamp: ts.Unix(),
	}
	err := s.Bkt.Save(data)
	return data, err
}

func (s *VoteStore) AddVote(ts time.Time, msg MsgChatID, userID int, increment int) (bool, *MsgVote, error) {
	lk := s.lockChat(msg)
	lk.Lock()
	defer lk.Unlock()

	data := &MsgVote{ID: msg}
	err := s.Bkt.One("ID", data.ID, data)

	log.Printf("[TRACE] >> %v %v %v %v", msg, userID, increment, err)
	log.Printf("[TRACE] > %v", data)

	if err == nil {
		log.Printf("[TRACE] > data.Users[userID]*increment %v", data.Users[userID]*increment)

		if data.Users[userID]*increment >= 0 {
			data.Users[userID] += increment
			log.Printf("[TRACE] > data.Users[userID] %v", data.Users[userID])

			err = s.Bkt.Update(data)
			return true, data, err
		}
	}
	return false, data, err
}

func (s *VoteStore) Stat(ts time.Time, msg MsgChatID, fn func(*MsgVote)) error {
	lk := s.lockChat(msg)
	lk.RLock()
	defer lk.RUnlock()
	query := s.Bkt.Select(q.Eq("ID", msg), q.Gt("Timestamp", ts))

	err := query.Each(&MsgVote{}, func(val interface{}) error {
		if vote, ok := val.(*MsgVote); ok {
			fn(vote)
		}
		return errors.Errorf("`val` is mot MsgVote, but %v", val)
	})
	return err
}
