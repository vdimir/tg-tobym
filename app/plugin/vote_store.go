package plugin

import (
	"sync"
	"time"

	"github.com/asdine/storm/v3"
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

	if err == nil {
		if data.Users[userID]*increment >= 0 {
			data.Users[userID] += increment
			err = s.Bkt.Update(data)
			return true, data, nil
		}
	}
	return false, data, err
}
