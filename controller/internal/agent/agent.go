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
	ErrAgentDeleted    = errors.New("agent deleted") // Agent was soft-deleted from panel
)

// -------------------- Agent (updated to your new struct) --------------------

type Agent struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Ownership / scoping
	WorkspaceID uint `gorm:"index:idx_ws_pin,priority:1" json:"workspace_id"`

	// Identity
	Name        string `gorm:"size:255;index" json:"name" form:"name"`
	Description string `gorm:"size:255;index" json:"description" form:"description"`

	// Network
	Location         string `gorm:"size:255" json:"location"`
	PublicIPOverride string `gorm:"size:64" json:"public_ip_override"`

	// Runtime / versioning
	Version string `gorm:"size:64;index" json:"version"`

	// Health
	LastSeenAt time.Time `gorm:"index" json:"last_seen_at"`

	// Tags / labels
	Labels   datatypes.JSON `gorm:"type:jsonb" json:"labels"`
	Metadata datatypes.JSON `gorm:"type:jsonb" json:"metadata"`

	Initialized bool `gorm:"default:false" json:"initialized"`

	// Authentication (post-bootstrap)
	PSKHash string `gorm:"size:255" json:"-"` // bcrypt hash of server-generated PSK

	// TrafficSim server configuration (per-agent, not per-probe)
	TrafficSimEnabled bool   `gorm:"column:trafficsim_enabled;default:false" json:"trafficsim_enabled"`
	TrafficSimHost    string `gorm:"column:trafficsim_host;size:64;default:0.0.0.0" json:"trafficsim_host"`
	TrafficSimPort    int    `gorm:"column:trafficsim_port;default:5000" json:"trafficsim_port"`
}

// -------------------- Auth placeholders in separate tables --------------------

// Auth One-time bootstrap PINs (plaintext stored until consumed for reviewability)
type Auth struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID  uint       `gorm:"index:idx_agent_pins_scope" json:"workspace_id"`
	AgentID      uint       `gorm:"index:idx_agent_pins_scope" json:"agent_id"`
	PinHash      string     `gorm:"size:255" json:"-"`
	PinPlaintext string     `gorm:"size:255" json:"-"` // Stored until consumed, then cleared
	Consumed     *time.Time `gorm:"index" json:"-"`
	ExpiresAt    *time.Time `json:"-"`
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
	Agent *Agent `json:"agent,omitempty"`
	PIN   string `json:"pin,omitempty"` // plaintext, shown ONCE
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
func CreateAgent(ctx context.Context, db *gorm.DB, in CreateInput) (*CreateOutput, error) {
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

		// 2) Issue PIN (simplified - no pepper/index needed)
		var expiresAt *time.Time
		if in.PINTTL != nil && *in.PINTTL > 0 {
			t := now.Add(*in.PINTTL)
			expiresAt = &t
		}

		p, err := generateNumericPIN(pinLen)
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		ap := &Auth{
			WorkspaceID:  a.WorkspaceID,
			AgentID:      a.ID,
			PinHash:      string(hash),
			PinPlaintext: p, // Stored for reviewability until consumed
			ExpiresAt:    expiresAt,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(ap).Error; err != nil {
			return err
		}
		pinPlain = p
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

// UpdateAgentVersion sets the last seen version (handled in ws events)
func UpdateAgentVersion(ctx context.Context, db *gorm.DB, id uint, version string) error {
	res := db.WithContext(ctx).Model(&Agent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"version": version,
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
// Simplified: no longer requires pinPepper - uses workspace+agent scoped lookup.
func IssuePIN(ctx context.Context, db *gorm.DB, workspaceID, agentID uint, pinLen int, ttl *time.Duration) (plaintext string, err error) {
	if pinLen <= 0 {
		pinLen = 9
	}
	now := time.Now()
	var expiresAt *time.Time
	if ttl != nil && *ttl > 0 {
		t := now.Add(*ttl)
		expiresAt = &t
	}

	return plaintext, db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := generateNumericPIN(pinLen)
		if err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		ap := &Auth{
			WorkspaceID:  workspaceID,
			AgentID:      agentID,
			PinHash:      string(hash),
			PinPlaintext: p, // Stored for reviewability until consumed
			ExpiresAt:    expiresAt,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(ap).Error; err != nil {
			return err
		}
		plaintext = p
		return nil
	})
}

// ConsumePIN validates and consumes a PIN for an agent.
// Simplified: uses workspace+agent scoped lookup with bcrypt verification.
func ConsumePIN(ctx context.Context, db *gorm.DB, workspaceID, agentID uint, pin string) (*Auth, error) {
	// Find all unconsumed PINs for this agent, ordered by newest first
	var pins []Auth
	err := db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND consumed IS NULL", workspaceID, agentID).
		Order("created_at DESC").
		Find(&pins).Error
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		return nil, ErrNotFound
	}

	// Try to match against each unconsumed PIN
	for _, ap := range pins {
		// Check expiry first
		if ap.ExpiresAt != nil && time.Now().After(*ap.ExpiresAt) {
			continue // Try next PIN
		}
		// bcrypt compare
		if err := bcrypt.CompareHashAndPassword([]byte(ap.PinHash), []byte(pin)); err != nil {
			continue // Wrong PIN, try next
		}
		// Match! Mark consumed and clear plaintext
		now := time.Now()
		if err := db.WithContext(ctx).Model(&ap).Updates(map[string]any{
			"consumed":      &now,
			"pin_plaintext": "", // Clear plaintext on consumption
		}).Error; err != nil {
			return nil, err
		}
		ap.Consumed = &now
		ap.PinPlaintext = ""
		return &ap, nil
	}

	return nil, ErrInvalidPIN
}

// GetPendingPIN returns the plaintext PIN for an agent if an unconsumed, non-expired PIN exists.
// Returns empty string if no pending PIN exists.
func GetPendingPIN(ctx context.Context, db *gorm.DB, workspaceID, agentID uint) (string, error) {
	var pin Auth
	err := db.WithContext(ctx).
		Where("workspace_id = ? AND agent_id = ? AND consumed IS NULL AND pin_plaintext != ''", workspaceID, agentID).
		Order("created_at DESC").
		First(&pin).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil // No pending PIN
		}
		return "", err
	}
	// Check if expired
	if pin.ExpiresAt != nil && time.Now().After(*pin.ExpiresAt) {
		return "", nil
	}
	return pin.PinPlaintext, nil
}

// -------------------- PSK operations --------------------

// AuthenticateWithPSK verifies the provided plaintext PSK against the stored bcrypt hash.
// Returns ErrAgentDeleted if the agent was soft-deleted (should trigger 410 Gone response).
func AuthenticateWithPSK(ctx context.Context, db *gorm.DB, workspaceID, agentID uint, psk string) (*Agent, error) {
	// First check if agent exists but is soft-deleted
	if deleted, err := IsAgentSoftDeleted(ctx, db, workspaceID, agentID); err == nil && deleted {
		return nil, ErrAgentDeleted
	}

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

// IsAgentSoftDeleted checks if an agent exists but has been soft-deleted.
// Uses Unscoped to find agents that would normally be hidden by GORM's soft-delete filter.
func IsAgentSoftDeleted(ctx context.Context, db *gorm.DB, workspaceID, agentID uint) (bool, error) {
	var a Agent
	err := db.WithContext(ctx).
		Unscoped().
		Where("workspace_id = ? AND id = ? AND deleted_at IS NOT NULL", workspaceID, agentID).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil // Not soft-deleted (or doesn't exist at all)
	}
	if err != nil {
		return false, err
	}
	return true, nil // Found a soft-deleted record
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
func BootstrapWithPIN(ctx context.Context, db *gorm.DB, in BootstrapWithPINInput) (*BootstrapOutput, error) {
	a, err := GetAgentByWorkspaceAndID(ctx, db, in.WorkspaceID, in.AgentID)
	if err != nil {
		return nil, err
	}

	// 1) Validate & consume PIN (simplified - no pepper needed)
	if _, err := ConsumePIN(ctx, db, in.WorkspaceID, in.AgentID, in.PIN); err != nil {
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
			"psk_hash":    pskHash,
			"updated_at":  time.Now(),
			"initialized": true,
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
