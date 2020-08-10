package store

import (
	"path"

	"github.com/asdine/storm/v3"
)

// Storage stores data
type Storage struct {
	DB *storm.DB
}

// NewStorage creates new Stroage
func NewStorage(folderPath string) (*Storage, error) {
	db, err := storm.Open(path.Join(folderPath, "data.db"))
	if err != nil {
		return nil, err
	}

	if err != nil {
		db.Close()
		return nil, err
	}
	return &Storage{
		DB: db,
	}, nil
}

func (s *Storage) GetBucket(name string) storm.Node {
	return s.DB.From(name)
}

// Close storage
func (s *Storage) Close() error {
	return s.DB.Close()
}
