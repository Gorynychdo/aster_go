package store

import "database/sql"

type Store struct {
	db               *sql.DB
	userRepository   *UserRepository
	recordRepository *RecordRepository
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) User() *UserRepository {
	if s.userRepository == nil {
		s.userRepository = &UserRepository{store: s}
	}

	return s.userRepository
}

func (s *Store) Record() *RecordRepository {
	if s.recordRepository == nil {
		s.recordRepository = &RecordRepository{store: s}
	}

	return s.recordRepository
}
