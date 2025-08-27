package users

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

var (
	ErrNotFound       = errors.New("user not found")
	ErrDuplicateEmail = errors.New("email already in use")
)

type Repository interface {
	Create(ctx context.Context, u *User) error
	GetByID(ctx context.Context, id uint) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, limit, offset int, q string) ([]User, int64, error)
	Update(ctx context.Context, u *User) error
	PatchFields(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error     // soft delete
	HardDelete(ctx context.Context, id uint) error // permanent delete
	MarkVerified(ctx context.Context, id uint) error
	RecordLogin(ctx context.Context, id uint, when time.Time) error
}

type gormRepo struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) Repository { return &gormRepo{db: db} }

func (r *gormRepo) Create(ctx context.Context, u *User) error {
	// enforce case-insensitive uniqueness on email in code-path
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&User{}).
		Where("LOWER(email) = LOWER(?)", u.Email).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return ErrDuplicateEmail
	}
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *gormRepo) GetByID(ctx context.Context, id uint) (*User, error) {
	var out User
	err := r.db.WithContext(ctx).First(&out, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &out, err
}

func (r *gormRepo) GetByEmail(ctx context.Context, email string) (*User, error) {
	var out User
	err := r.db.WithContext(ctx).
		Where("LOWER(email) = LOWER(?)", email).
		First(&out).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &out, err
}

func (r *gormRepo) List(ctx context.Context, limit, offset int, q string) ([]User, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	dbq := r.db.WithContext(ctx).Model(&User{})
	if q != "" {
		like := "%" + q + "%"
		dbq = dbq.Where("email ILIKE ? OR first_name ILIKE ? OR last_name ILIKE ?", like, like, like)
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []User
	if err := dbq.Order("id DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *gormRepo) Update(ctx context.Context, u *User) error {
	// Protect uniqueness on email
	var cnt int64
	if err := r.db.WithContext(ctx).Model(&User{}).
		Where("LOWER(email) = LOWER(?) AND id <> ?", u.Email, u.ID).
		Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return ErrDuplicateEmail
	}
	return r.db.WithContext(ctx).Save(u).Error
}

func (r *gormRepo) PatchFields(ctx context.Context, id uint, fields map[string]any) error {
	if _, ok := fields["updated_at"]; !ok {
		fields["updated_at"] = time.Now()
	}
	res := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&User{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepo) HardDelete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Unscoped().Delete(&User{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepo) MarkVerified(ctx context.Context, id uint) error {
	return r.PatchFields(ctx, id, map[string]any{"verified": true})
}

func (r *gormRepo) RecordLogin(ctx context.Context, id uint, when time.Time) error {
	return r.PatchFields(ctx, id, map[string]any{"last_login_at": when})
}
