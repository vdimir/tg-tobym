package plugin

import (
	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/q"
	"github.com/pkg/errors"
)

type NotifierStore struct {
	Bkt storm.Node
}

type chatToken struct {
	Token  string `storm:"id"`
	ChatID int64
}

func (s *NotifierStore) SaveToken(chatID int64, token string) error {
	if token == "" {
		return errors.Errorf("empty token")
	}
	err := s.Bkt.Save(&chatToken{Token: token, ChatID: chatID})
	return err
}

func (s *NotifierStore) RemoveTokens(chatID int64, token string) (int, error) {
	if token == "" {
		err := s.Bkt.Select(q.Eq("ChatID", chatID)).Delete(&chatToken{})
		if err == storm.ErrNotFound {
			return 0, nil
		}
		// do not count number of deleted entries actually, say "more than one"
		return 2, err
	}
	err := s.Bkt.DeleteStruct(&chatToken{Token: token, ChatID: chatID})
	if err == storm.ErrNotFound {
		return 0, nil
	}
	return 1, err
}

func (s *NotifierStore) FindToken(token string) int64 {
	if token == "" {
		return 0
	}

	res := &chatToken{}
	err := s.Bkt.One("Token", token, res)
	if err == storm.ErrNotFound {
		return 0
	}
	return res.ChatID
}
