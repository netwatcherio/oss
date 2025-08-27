package auth

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var (
	ErrSessionNotFound = errors.New("session not found")
)

type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	GetBySessionID(ctx context.Context, sid uint) (*Session, error)
	UpdateWSConn(ctx context.Context, sid uint, ws string) error
	GetByWSConn(ctx context.Context, ws string) (*Session, error)
	Delete(ctx context.Context, sid uint) error
}

type gormSessionRepo struct{ db *gorm.DB }

func NewSessionRepository(db *gorm.DB) SessionRepository { return &gormSessionRepo{db: db} }

func (r *gormSessionRepo) Create(ctx context.Context, s *Session) error {
	now := time.Now()
	if s.Created.IsZero() {
		s.Created = now
	}
	if s.Expiry.IsZero() {
		s.Expiry = now.Add(24 * time.Hour)
	}
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *gormSessionRepo) GetBySessionID(ctx context.Context, sid uint) (*Session, error) {
	var out Session
	err := r.db.WithContext(ctx).First(&out, "session_id = ?", sid).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	return &out, err
}

func (r *gormSessionRepo) UpdateWSConn(ctx context.Context, sid uint, ws string) error {
	res := r.db.WithContext(ctx).Model(&Session{}).Where("session_id = ?", sid).Update("ws_conn", ws)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (r *gormSessionRepo) GetByWSConn(ctx context.Context, ws string) (*Session, error) {
	var out Session
	err := r.db.WithContext(ctx).First(&out, "ws_conn = ?", ws).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	return &out, err
}

func (r *gormSessionRepo) Delete(ctx context.Context, sid uint) error {
	res := r.db.WithContext(ctx).Delete(&Session{}, "session_id = ?", sid)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrSessionNotFound
	}
	return nil
}
