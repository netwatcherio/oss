package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	agentpkg "netwatcher-controller/internal/agent"
	userpkg "netwatcher-controller/internal/users"
)

// DTOs
type Login struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Register struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	Company   string `json:"company"`
	Phone     string `json:"phoneNumber"`
}

type AgentLogin struct {
	PIN          string `json:"pin"`
	ID           uint   `json:"id"`      // Agent PK (gorm uint)
	AgentVersion string `json:"version"` // optional
	IP           string `json:"-"`
}

type Service interface {
	// users
	Login(ctx context.Context, in Login, ip string) (token string, u *userpkg.User, err error)
	Register(ctx context.Context, in Register, ip string) (token string, u *userpkg.User, err error)

	// agents
	AgentLogin(ctx context.Context, in AgentLogin) (token string, a *agentpkg.Agent, err error)

	// sessions
	GetSession(ctx context.Context, sessionID uint) (*Session, error)
	UpdateWSConn(ctx context.Context, sessionID uint, ws string) error
	GetSessionFromWSConn(ctx context.Context, ws string) (*Session, error)

	// helpers
	GetUserFromJWT(ctx context.Context, tokenString string, db *gorm.DB) (*userpkg.User, *Session, error)
	GetAgentFromJWT(ctx context.Context, tokenString string, db *gorm.DB) (*agentpkg.Agent, *Session, error)
}

type service struct {
	db          *gorm.DB
	usersRepo   userpkg.Repository
	usersSvc    userpkg.Service
	agentsRepo  agentpkg.Repository
	sessionRepo SessionRepository
}

func NewService(db *gorm.DB, usersRepo userpkg.Repository, usersSvc userpkg.Service, agentsRepo agentpkg.Repository) Service {
	return &service{
		db:          db,
		usersRepo:   usersRepo,
		usersSvc:    usersSvc,
		agentsRepo:  agentsRepo,
		sessionRepo: NewSessionRepository(db),
	}
}

// ----- Users -----

func (s *service) Login(ctx context.Context, in Login, ip string) (string, *userpkg.User, error) {
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return "", nil, errors.New("invalid credentials")
	}

	u, err := s.usersRepo.GetByEmail(ctx, email)
	if err != nil {
		return "", nil, errors.New("invalid credentials")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(in.Password)) != nil {
		return "", nil, errors.New("invalid credentials")
	}

	// make session
	sess := &Session{
		ID:      u.ID,
		IsAgent: false,
		IP:      ip,
		Expiry:  time.Now().Add(24 * time.Hour),
		Created: time.Now(),
	}
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return "", nil, err
	}

	// record login time (best effort)
	_ = s.usersRepo.RecordLogin(ctx, u.ID, time.Now())

	// jwt
	tok, err := IssueJWT(u.ID, sess.SessionID, false, 24*time.Hour)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}

func (s *service) Register(ctx context.Context, in Register, ip string) (string, *userpkg.User, error) {
	if strings.TrimSpace(in.FirstName) == "" || strings.TrimSpace(in.LastName) == "" {
		return "", nil, errors.New("invalid name")
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" || in.Password == "" {
		return "", nil, errors.New("invalid credentials")
	}
	u, err := s.usersSvc.Register(ctx, userpkg.RegisterInput{
		Email:       email,
		FirstName:   in.FirstName,
		LastName:    in.LastName,
		Company:     in.Company,
		PhoneNumber: in.Phone,
		Password:    in.Password, // service hashes it
	})
	if err != nil {
		return "", nil, err
	}

	// session
	sess := &Session{
		ID:      u.ID,
		IsAgent: false,
		IP:      ip,
		Expiry:  time.Now().Add(24 * time.Hour),
		Created: time.Now(),
	}
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return "", nil, err
	}
	tok, err := IssueJWT(u.ID, sess.SessionID, false, 24*time.Hour)
	if err != nil {
		return "", nil, err
	}
	return tok, u, nil
}

// ----- Agents -----

func (s *service) AgentLogin(ctx context.Context, in AgentLogin) (string, *agentpkg.Agent, error) {
	if in.PIN == "" || in.ID == 0 {
		return "", nil, errors.New("invalid agent credentials")
	}
	a, err := s.agentsRepo.GetByID(ctx, in.ID)
	if err != nil {
		return "", nil, errors.New("agent not found")
	}
	if a.Pin != in.PIN {
		return "", nil, errors.New("invalid pin")
	}

	// Create session
	sess := &Session{
		ID:      a.ID,
		IsAgent: true,
		IP:      in.IP,
		Expiry:  time.Now().Add(24 * time.Hour),
		Created: time.Now(),
	}
	if err := s.sessionRepo.Create(ctx, sess); err != nil {
		return "", nil, err
	}

	// Best-effort: bump version / heartbeat
	_ = s.agentsRepo.PatchFields(ctx, a.ID, map[string]any{
		"version":    strings.TrimSpace(in.AgentVersion),
		"updated_at": time.Now(),
	})

	// JWT
	tok, err := IssueJWT(a.ID, sess.SessionID, true, 24*time.Hour)
	if err != nil {
		return "", nil, err
	}
	return tok, a, nil
}

// ----- Sessions -----

func (s *service) GetSession(ctx context.Context, sessionID uint) (*Session, error) {
	return s.sessionRepo.GetBySessionID(ctx, sessionID)
}

func (s *service) UpdateWSConn(ctx context.Context, sessionID uint, ws string) error {
	return s.sessionRepo.UpdateWSConn(ctx, sessionID, ws)
}

func (s *service) GetSessionFromWSConn(ctx context.Context, ws string) (*Session, error) {
	return s.sessionRepo.GetByWSConn(ctx, ws)
}

// ----- Helpers (token → user/agent) -----

func (s *service) GetUserFromJWT(ctx context.Context, tokenString string, db *gorm.DB) (*userpkg.User, *Session, error) {
	claims, err := ParseJWT(tokenString)
	if err != nil {
		return nil, nil, err
	}
	sess, err := s.sessionRepo.GetBySessionID(ctx, claims.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if time.Now().After(sess.Expiry) {
		return nil, nil, ErrExpiredToken
	}
	if sess.IsAgent {
		return nil, nil, errors.New("session belongs to an agent")
	}
	if sess.ID != claims.ItemID {
		return nil, nil, errors.New("id mismatch")
	}
	u, err := s.usersRepo.GetByID(ctx, sess.ID)
	if err != nil {
		return nil, nil, err
	}
	return u, sess, nil
}

func (s *service) GetAgentFromJWT(ctx context.Context, tokenString string, db *gorm.DB) (*agentpkg.Agent, *Session, error) {
	claims, err := ParseJWT(tokenString)
	if err != nil {
		return nil, nil, err
	}
	sess, err := s.sessionRepo.GetBySessionID(ctx, claims.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if time.Now().After(sess.Expiry) {
		return nil, nil, ErrExpiredToken
	}
	if !sess.IsAgent {
		return nil, nil, errors.New("session is not an agent")
	}
	if sess.ID != claims.ItemID {
		return nil, nil, errors.New("id mismatch")
	}
	// you likely have agent repo wired already
	a, err := s.agentsRepo.GetByID(ctx, sess.ID)
	if err != nil {
		return nil, nil, err
	}
	return a, sess, nil
}
