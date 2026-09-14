package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Feedback is one user-submitted feedback row: a required 1-5 star rating
// plus optional free text.
type Feedback struct {
	ID        int64
	Rating    int
	Text      string
	CreatedAt time.Time
}

type FeedbackStore struct {
	db *sql.DB
}

func NewFeedbackStore(db *sql.DB) *FeedbackStore {
	return &FeedbackStore{db: db}
}

func (s *FeedbackStore) Create(rating int, text string, createdAt time.Time) (*Feedback, error) {
	res, err := s.db.Exec(`
		INSERT INTO feedback (rating, text, created_at) VALUES (?, ?, ?)
	`, rating, text, createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert feedback: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("feedback last insert id: %w", err)
	}
	return &Feedback{ID: id, Rating: rating, Text: text, CreatedAt: createdAt}, nil
}
