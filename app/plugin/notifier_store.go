package plugin

import (
	"github.com/asdine/storm/v3"
	"github.com/vdimir/tg-tobym/app/store"
)

const notifierBucketName = "notifier"

type NotifierStore struct {
	Store *store.Storage
}

type chatToken struct {
	ChatID int64  `storm:"id"`
	Token  string `storm:"unique"`
}

func (s *NotifierStore) SaveToken(chatID int64, token string) error {
	bkt := s.Store.GetBucket(notifierBucketName)
	err := bkt.Save(&chatToken{ChatID: chatID, Token: token})
	return err
}

func (s *NotifierStore) FindToken(token string) int64 {
	if token == "" {
		return 0
	}

	bkt := s.Store.GetBucket(notifierBucketName)
	res := &chatToken{}
	err := bkt.One("Token", token, res)
	if err == storm.ErrNotFound {
		return 0
	}
	return res.ChatID
}
