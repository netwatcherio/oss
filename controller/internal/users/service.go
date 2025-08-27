package users

import (
	"context"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
)

type Service interface {
	Register(ctx context.Context, in RegisterInput) (*User, error)
	Get(ctx context.Context, id uint) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context, limit, offset int, q string) ([]User, int64, error)

	Update(ctx context.Context, id uint, in UpdateInput) (*User, error)
	Patch(ctx context.Context, id uint, fields map[string]any) error
	Delete(ctx context.Context, id uint) error
	HardDelete(ctx context.Context, id uint) error

	VerifyEmail(ctx context.Context, id uint) error
	SetPassword(ctx context.Context, id uint, plain string) error
	CheckPassword(ctx context.Context, id uint, plain string) (bool, error)
	RecordLogin(ctx context.Context, id uint) error

	SetAdmin(ctx context.Context, id uint, admin bool) error
	SetRole(ctx context.Context, id uint, role string) error
	SetStatus(ctx context.Context, id uint, status Status) error
}

type service struct{ repo Repository }

func NewService(repo Repository) Service { return &service{repo: repo} }

// ----- DTOs -----

type RegisterInput struct {
	Email       string
	FirstName   string
	LastName    string
	Company     string
	PhoneNumber string
	Role        string
	Password    string // plain text; will be bcrypt-hashed
	Admin       bool

	TZ        string
	AvatarURL string

	Labels   datatypes.JSON
	Metadata datatypes.JSON
}

type UpdateInput struct {
	Email       *string
	FirstName   *string
	LastName    *string
	Company     *string
	PhoneNumber *string
	Role        *string
	Admin       *bool
	TZ          *string
	AvatarURL   *string
	Labels      *datatypes.JSON
	Metadata    *datatypes.JSON
	Status      *Status // allow suspend/activate via Update
}

func (s *service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &User{
		Email:       strings.ToLower(strings.TrimSpace(in.Email)),
		FirstName:   strings.TrimSpace(in.FirstName),
		LastName:    strings.TrimSpace(in.LastName),
		Company:     strings.TrimSpace(in.Company),
		PhoneNumber: strings.TrimSpace(in.PhoneNumber),
		Role:        strings.TrimSpace(in.Role),
		Admin:       in.Admin,
		Password:    string(hash),
		Verified:    false,
		TZ:          strings.TrimSpace(in.TZ),
		AvatarURL:   strings.TrimSpace(in.AvatarURL),
		Labels:      coalesceJSON(in.Labels),
		Metadata:    coalesceJSON(in.Metadata),
		Status:      StatusActive,
	}
	if err := s.repo.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *service) Get(ctx context.Context, id uint) (*User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) GetByEmail(ctx context.Context, email string) (*User, error) {
	return s.repo.GetByEmail(ctx, email)
}

func (s *service) List(ctx context.Context, limit, offset int, q string) ([]User, int64, error) {
	return s.repo.List(ctx, limit, offset, q)
}

func (s *service) Update(ctx context.Context, id uint, in UpdateInput) (*User, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Email != nil {
		u.Email = strings.ToLower(strings.TrimSpace(*in.Email))
	}
	if in.FirstName != nil {
		u.FirstName = strings.TrimSpace(*in.FirstName)
	}
	if in.LastName != nil {
		u.LastName = strings.TrimSpace(*in.LastName)
	}
	if in.Company != nil {
		u.Company = strings.TrimSpace(*in.Company)
	}
	if in.PhoneNumber != nil {
		u.PhoneNumber = strings.TrimSpace(*in.PhoneNumber)
	}
	if in.Role != nil {
		u.Role = strings.TrimSpace(*in.Role)
	}
	if in.Admin != nil {
		u.Admin = *in.Admin
	}
	if in.TZ != nil {
		u.TZ = strings.TrimSpace(*in.TZ)
	}
	if in.AvatarURL != nil {
		u.AvatarURL = strings.TrimSpace(*in.AvatarURL)
	}
	if in.Labels != nil {
		u.Labels = *in.Labels
	}
	if in.Metadata != nil {
		u.Metadata = *in.Metadata
	}
	if in.Status != nil {
		u.Status = *in.Status
	}
	u.UpdatedAt = time.Now()
	if err := s.repo.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *service) Patch(ctx context.Context, id uint, fields map[string]any) error {
	return s.repo.PatchFields(ctx, id, fields)
}

func (s *service) Delete(ctx context.Context, id uint) error     { return s.repo.Delete(ctx, id) }
func (s *service) HardDelete(ctx context.Context, id uint) error { return s.repo.HardDelete(ctx, id) }

func (s *service) VerifyEmail(ctx context.Context, id uint) error {
	return s.repo.MarkVerified(ctx, id)
}

func (s *service) SetPassword(ctx context.Context, id uint, plain string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.PatchFields(ctx, id, map[string]any{"password": string(hash)})
}

func (s *service) CheckPassword(ctx context.Context, id uint, plain string) (bool, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	err = bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(plain))
	return err == nil, nil
}

func (s *service) RecordLogin(ctx context.Context, id uint) error {
	return s.repo.RecordLogin(ctx, id, time.Now())
}

func (s *service) SetAdmin(ctx context.Context, id uint, admin bool) error {
	return s.repo.PatchFields(ctx, id, map[string]any{"admin": admin})
}

func (s *service) SetRole(ctx context.Context, id uint, role string) error {
	return s.repo.PatchFields(ctx, id, map[string]any{"role": strings.TrimSpace(role)})
}

func (s *service) SetStatus(ctx context.Context, id uint, status Status) error {
	return s.repo.PatchFields(ctx, id, map[string]any{"status": status})
}

// ---- helpers ----
func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}
