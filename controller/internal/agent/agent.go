package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// -------------------- Errors --------------------

var (
	ErrNotFound        = errors.New("not found")
	ErrInvalidPIN      = errors.New("invalid pin")
	ErrPINExpired      = errors.New("pin expired")
	ErrPINMismatch     = errors.New("pin scope mismatch")
	ErrNonceRequired   = errors.New("signature and nonce required")
	ErrSignatureFailed = errors.New("signature verification failed")
	ErrInvalidPSK      = errors.New("invalid psk")
)

// -------------------- Agent (updated to your new struct) --------------------

type Agent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `gorm:"index" json:"updatedAt"`

	// Ownership / scoping
	WorkspaceID uint `gorm:"index:idx_ws_pin,priority:1" json:"workspaceId"`

	// Identity
	Name        string `gorm:"size:255;index" json:"name" form:"name"`
	Description string `gorm:"size:255;index" json:"description" form:"description"`

	// Network
	Location         string `gorm:"size:255" json:"location"`
	PublicIPOverride string `gorm:"size:64" json:"public_ip_override"`

	// Runtime / versioning
	Version string `gorm:"size:64;index" json:"version"`

	// Health
	LastSeenAt time.Time `gorm:"index" json:"lastSeenAt"`

	// Tags / labels
	Labels   datatypes.JSON `gorm:"type:jsonb" json:"labels"`
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata"`

	Initialized bool `gorm:"default:false" json:"initialized"`

	// Authentication (post-bootstrap)
	PSKHash string `gorm:"size:255" json:"-"` // bcrypt hash of server-generated PSK
}

// -------------------- Auth placeholders in separate tables --------------------

// Auth One-time bootstrap PINs (plaintext never stored)
type Auth struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"index" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID uint   `gorm:"index" json:"workspaceId"`
	AgentID     uint   `gorm:"index" json:"agentId"`
	PinHash     string `gorm:"size:255;index" json:"-"`
	// Unique across *active* (unconsumed) PINs
	PinIndex  string     `gorm:"size:64;uniqueIndex" json:"-"`
	Consumed  *time.Time `json:"-"`
	ExpiresAt *time.Time `json:"-"`
}

func (Auth) TableName() string { return "agent_pins" }

// -------------------- DTOs --------------------

type CreateInput struct {
	WorkspaceID      uint
	Name             string
	Description      string
	PinLength        int // default 9
	Location         string
	PublicIPOverride string
	Version          string
	Labels           datatypes.JSON
	Metadata         datatypes.JSON
	PINTTL           *time.Duration // optional expiry for bootstrap PIN
}

type CreateOutput struct {
	Agent *Agent
	PIN   string // plaintext, shown ONCE
}

type BootstrapWithPINInput struct {
	WorkspaceID uint
	AgentID     uint
	PIN         string

	// Optional: switch to key-based auth by registering an ed25519 key.
	PublicKey []byte // ed25519 public key
	Signature []byte // signature over Nonce using PublicKey
	Nonce     string // required when Signature is provided
}

type BootstrapOutput struct {
	Agent *Agent
	PSK   string // plaintext PSK, shown ONCE after successful bootstrap
}

// -------------------- Public API --------------------

// CreateAgent inserts Agent, default probes, and issues a bootstrap PIN (in agent_pins).
func CreateAgent(ctx context.Context, db *gorm.DB, in CreateInput, pinPepper string) (*CreateOutput, error) {
	pinLen := in.PinLength
	if pinLen <= 0 {
		pinLen = 9
	}

	now := time.Now()
	a := &Agent{
		WorkspaceID:      in.WorkspaceID,
		Name:             in.Name,
		Description:      in.Description,
		Location:         in.Location,
		PublicIPOverride: in.PublicIPOverride,
		Version:          in.Version,
		LastSeenAt:       time.Time{}, // zero until first heartbeat
		Labels:           coalesceJSON(in.Labels),
		Metadata:         coalesceJSON(in.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	var pinPlain string

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1) Create agent
		if err := tx.Create(a).Error; err != nil {
			return err
		}

		// 2) Default probes
		if err := createDefaultProbesTx(tx, a); err != nil {
			return err
		}

		// 3) Issue unique PIN
		var expiresAt *time.Time
		if in.PINTTL != nil && *in.PINTTL > 0 {
			t := now.Add(*in.PINTTL)
			expiresAt = &t
		}

		const maxAttempts = 10
		for attempt := 0; attempt < maxAttempts; attempt++ {
			p, err := generateNumericPIN(pinLen)
			if err != nil {
				return err
			}
			index := computePinIndex(pinPepper, p)

			// Fast uniqueness probe on active pins
			var dup Auth
			if err := tx.Where("pin_index = ? AND consumed IS NULL", index).First(&dup).Error; err == nil {
				continue // collision; try again
			}

			hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
			if err != nil {
				return err
			}

			ap := &Auth{
				WorkspaceID: a.WorkspaceID,
				AgentID:     a.ID,
				PinHash:     string(hash),
				PinIndex:    index,
				ExpiresAt:   expiresAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(ap).Error; err != nil {
				continue // handle race on unique index
			}

			pinPlain = p
			break
		}
		if pinPlain == "" {
			return errors.New("failed to generate unique PIN")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &CreateOutput{Agent: a, PIN: pinPlain}, nil
}

func GetAgentByID(ctx context.Context, db *gorm.DB, id uint) (*Agent, error) {
	var a Agent
	err := db.WithContext(ctx).First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &a, err
}

func GetAgentByWorkspaceAndID(ctx context.Context, db *gorm.DB, wsID, id uint) (*Agent, error) {
	var a Agent
	err := db.WithContext(ctx).Where("workspace_id = ? AND id = ?", wsID, id).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &a, err
}

func ListAgentsByWorkspace(ctx context.Context, db *gorm.DB, workspaceID uint, limit, offset int) ([]Agent, int64, error) {
	q := db.WithContext(ctx).Model(&Agent{}).Where("workspace_id = ?", workspaceID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 50
	}
	var items []Agent
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func UpdateAgent(ctx context.Context, db *gorm.DB, a *Agent) error {
	a.UpdatedAt = time.Now()
	return db.WithContext(ctx).Save(a).Error
}

func PatchAgentFields(ctx context.Context, db *gorm.DB, id uint, fields map[string]any) error {
	fields["updated_at"] = time.Now()
	res := db.WithContext(ctx).Model(&Agent{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAgentSeen sets LastSeenAt (lightweight heartbeat)
func UpdateAgentSeen(ctx context.Context, db *gorm.DB, id uint, seenAt time.Time) error {
	res := db.WithContext(ctx).Model(&Agent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_seen_at": seenAt,
			"updated_at":   time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAgent permanently deletes the row (no soft-delete on Agent)
func DeleteAgent(ctx context.Context, db *gorm.DB, id uint) error {
	res := db.WithContext(ctx).Delete(&Agent{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// -------------------- PIN operations --------------------

// IssuePIN creates a new one-time PIN row for an agent.
func IssuePIN(ctx context.Context, db *gorm.DB, workspaceID, agentID uint, pinLen int, pinPepper string, ttl *time.Duration) (plaintext string, err error) {
	if pinLen <= 0 {
		pinLen = 9
	}
	now := time.Now()
	var expiresAt *time.Time
	if ttl != nil && *ttl > 0 {
		t := now.Add(*ttl)
		expiresAt = &t
	}

	const maxAttempts = 10
	return plaintext, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for attempt := 0; attempt < maxAttempts; attempt++ {
			p, err := generateNumericPIN(pinLen)
			if err != nil {
				return err
			}
			index := computePinIndex(pinPepper, p)

			var dup Auth
			if err := tx.Where("pin_index = ? AND consumed IS NULL", index).First(&dup).Error; err == nil {
				continue
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			ap := &Auth{
				WorkspaceID: workspaceID,
				AgentID:     agentID,
				PinHash:     string(hash),
				PinIndex:    index,
				ExpiresAt:   expiresAt,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.Create(ap).Error; err != nil {
				continue
			}
			plaintext = p
			return nil
		}
		return errors.New("failed to issue unique PIN")
	})
}

// ConsumePIN validates and consumes a PIN for an agent.
func ConsumePIN(ctx context.Context, db *gorm.DB, workspaceID, agentID uint, pin, pinPepper string) (*Auth, error) {
	index := computePinIndex(pinPepper, pin)

	var ap Auth
	err := db.WithContext(ctx).
		Where("pin_index = ? AND consumed IS NULL", index).
		First(&ap).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Scope checks
	if ap.WorkspaceID != workspaceID || ap.AgentID != agentID {
		return nil, ErrPINMismatch
	}
	// Expiry
	if ap.ExpiresAt != nil && time.Now().After(*ap.ExpiresAt) {
		return nil, ErrPINExpired
	}
	// Hash compare
	if err := bcrypt.CompareHashAndPassword([]byte(ap.PinHash), []byte(pin)); err != nil {
		return nil, ErrInvalidPIN
	}
	// Mark consumed
	now := time.Now()
	if err := db.WithContext(ctx).Model(&ap).Update("consumed", &now).Error; err != nil {
		return nil, err
	}
	ap.Consumed = &now
	return &ap, nil
}

// -------------------- PSK operations --------------------

// AuthenticateWithPSK verifies the provided plaintext PSK against the stored bcrypt hash.
func AuthenticateWithPSK(ctx context.Context, db *gorm.DB, workspaceID, agentID uint, psk string) (*Agent, error) {
	a, err := GetAgentByWorkspaceAndID(ctx, db, workspaceID, agentID)
	if err != nil {
		return nil, err
	}
	if a.PSKHash == "" {
		return nil, ErrInvalidPSK
	}
	if err := bcrypt.CompareHashAndPassword([]byte(a.PSKHash), []byte(psk)); err != nil {
		return nil, ErrInvalidPSK
	}
	return a, nil
}

// RotatePSK creates a new PSK for an agent (admin/server action). Returns plaintext once.
func RotatePSK(ctx context.Context, db *gorm.DB, workspaceID, agentID uint) (string, error) {
	a, err := GetAgentByWorkspaceAndID(ctx, db, workspaceID, agentID)
	if err != nil {
		return "", err
	}
	newPSK, newHash, err := generatePSKAndHash()
	if err != nil {
		return "", err
	}
	a.PSKHash = newHash
	a.UpdatedAt = time.Now()
	if err := db.WithContext(ctx).Model(a).Select("psk_hash", "updated_at").Updates(a).Error; err != nil {
		return "", err
	}
	return newPSK, nil
}

// -------------------- Bootstrap (PIN -> PSK + optional key) --------------------

// BootstrapWithPIN consumes a PIN, generates & stores a server PSK (bcrypt), and
// (optionally) registers an ed25519 credential in agent_credentials. Returns the
// plaintext PSK exactly once.
func BootstrapWithPIN(ctx context.Context, db *gorm.DB, in BootstrapWithPINInput, pinPepper string) (*BootstrapOutput, error) {
	a, err := GetAgentByWorkspaceAndID(ctx, db, in.WorkspaceID, in.AgentID)
	if err != nil {
		return nil, err
	}

	// 1) Validate & consume PIN
	if _, err := ConsumePIN(ctx, db, in.WorkspaceID, in.AgentID, in.PIN, pinPepper); err != nil {
		return nil, fmt.Errorf("pin verification failed: %w", err)
	}

	// 2) Generate PSK and persist bcrypt hash on agents row
	pskPlain, pskHash, err := generatePSKAndHash()
	if err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&Agent{}).
		Where("workspace_id = ? AND id = ?", in.WorkspaceID, in.AgentID).
		Updates(map[string]any{
			"psk_hash":   pskHash,
			"updated_at": time.Now(),
		}).Error; err != nil {
		return nil, err
	}

	// 4) Return fresh agent + plaintext PSK (one-time)
	a, err = GetAgentByID(ctx, db, a.ID)
	if err != nil {
		return nil, err
	}
	return &BootstrapOutput{Agent: a, PSK: pskPlain}, nil
}

// -------------------- Helpers --------------------

func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}

func generateNumericPIN(n int) (string, error) {
	const digits = "0123456789"
	if n <= 0 {
		return "", errors.New("pin length must be > 0")
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		x, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		b[i] = digits[x.Int64()]
	}
	return string(b), nil
}

// generatePSKAndHash creates a cryptographically-strong PSK and its bcrypt hash.
// PSK is 32 bytes of entropy, hex-encoded (64 chars). Adjust if you prefer base32.
func generatePSKAndHash() (pskPlain string, pskHash string, err error) {
	raw := make([]byte, 32) // 256 bits
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	pskPlain = hex.EncodeToString(raw)
	hash, err := bcrypt.GenerateFromPassword([]byte(pskPlain), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return pskPlain, string(hash), nil
}

// computePinIndex = hex(sha256(pepper + ":" + pin))
func computePinIndex(pepper, pin string) string {
	sum := sha256.Sum256([]byte(pepper + ":" + pin))
	return hex.EncodeToString(sum[:])
}

func keyFP(pub []byte) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// -------------------- Default probe creation (local mirror) --------------------

type dbProbe struct {
	ID          uint           `gorm:"primaryKey;autoIncrement"`
	CreatedAt   time.Time      `gorm:"index"`
	UpdatedAt   time.Time      `gorm:"index"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
	WorkspaceID uint           `gorm:"index"`
	AgentID     uint           `gorm:"index"`
	Type        string         `gorm:"size:64;index"`
	Labels      datatypes.JSON `gorm:"type:jsonb"`
	Metadata    datatypes.JSON `gorm:"type:jsonb"`
}

func (dbProbe) TableName() string { return "probes" }

type dbTarget struct {
	ID        uint           `gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time      `gorm:"index"`
	UpdatedAt time.Time      `gorm:"index"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
	ProbeID   uint           `gorm:"index"`
	Target    string         `gorm:"size:255"`
}

func (dbTarget) TableName() string { return "probe_targets" }

// Inserts 4 default probes and SPEEDTEST target "ok"
func createDefaultProbesTx(tx *gorm.DB, a *Agent) error {
	now := time.Now()
	empty := datatypes.JSON([]byte(`{}`))

	p := []*dbProbe{
		{WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "NETINFO", CreatedAt: now, UpdatedAt: now, Labels: empty, Metadata: empty},
		{WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "SYSINFO", CreatedAt: now, UpdatedAt: now, Labels: empty, Metadata: empty},
		{WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "SPEEDTEST_SERVERS", CreatedAt: now, UpdatedAt: now, Labels: empty, Metadata: empty},
		{WorkspaceID: a.WorkspaceID, AgentID: a.ID, Type: "SPEEDTEST", CreatedAt: now, UpdatedAt: now, Labels: empty, Metadata: empty},
	}
	for _, row := range p {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
	}
	t := dbTarget{ProbeID: p[3].ID, Target: "ok", CreatedAt: now, UpdatedAt: now}
	return tx.Create(&t).Error
}
