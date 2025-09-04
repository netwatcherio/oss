package agent

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Service offers business-logic level helpers (compose your HTTP/RPC handlers atop this).
type Service interface {
	// Admin create: returns PIN plaintext ONCE; also creates default probes.
	Create(ctx context.Context, in CreateInput) (*CreateOutput, error)

	// Deprecated: prefer Create(); kept for compatibility (returns Agent only)
	Register(ctx context.Context, in RegisterInput) (*Agent, error)

	Get(ctx context.Context, id uint) (*Agent, error)
	List(ctx context.Context, workspaceID uint, limit, offset int) ([]Agent, int64, error)
	Update(ctx context.Context, id uint, in UpdateInput) (*Agent, error)
	Patch(ctx context.Context, id uint, fields map[string]any) error
	Beat(ctx context.Context, id uint, status Status) error

	// RotatePin: legacy (caller supplies plaintext; does NOT return it)
	RotatePin(ctx context.Context, id uint, newPin string) error
	// RotatePinAndReturn: generates + returns new PIN once
	RotatePinAndReturn(ctx context.Context, id uint, length int) (*RotatePinOutput, error)

	// Key bootstrap
	CreateChallenge(ctx context.Context, in ChallengeInput) (*ChallengeOutput, error)
	RegisterKey(ctx context.Context, in RegisterKeyInput) (*Agent, error)
}

type service struct {
	repo      Repository
	db        *gorm.DB
	pinPepper []byte
}

func NewService(repo Repository, db *gorm.DB, pinPepper string) Service {
	return &service{repo: repo, db: db, pinPepper: []byte(pinPepper)}
}

// ----- DTOs -----

// CreateInput generates a PIN server-side and stores only its hash.
type CreateInput struct {
	WorkspaceID          uint
	SiteID               uint
	Name                 string
	Hostname             string
	PinLength            int // default 9 if 0 (as you preferred 9)
	Location             string
	PublicIPOverride     string
	Version              string
	HeartbeatIntervalSec int
	Labels               datatypes.JSON
	Metadata             datatypes.JSON
}
type CreateOutput struct {
	Agent *Agent
	PIN   string // plaintext, shown ONCE
}

// Back-compat with your original code paths. (Deprecated)
type RegisterInput struct {
	WorkspaceID          uint
	SiteID               uint
	Name                 string
	Hostname             string
	Pin                  string // if provided, we will hash it; otherwise we autogenerate
	Location             string
	PublicIPOverride     string
	Version              string
	HeartbeatIntervalSec int
	Labels               datatypes.JSON
	Metadata             datatypes.JSON
}

type UpdateInput struct {
	Name                 *string
	Hostname             *string
	Location             *string
	PublicIPOverride     *string
	Version              *string
	Status               *Status
	HeartbeatIntervalSec *int
	Labels               *datatypes.JSON
	Metadata             *datatypes.JSON
}

// PIN challenge & key registration
type ChallengeInput struct {
	WorkspaceID uint
	AgentID     uint
	PIN         string
}
type ChallengeOutput struct {
	AgentID   uint
	Nonce     string
	ExpiresAt time.Time
}
type RegisterKeyInput struct {
	WorkspaceID uint
	AgentID     uint
	PIN         string
	PublicKey   []byte // raw 32B Ed25519 public key
	Nonce       string
	Signature   []byte // signature over nonce
}

type RotatePinOutput struct{ PIN string }

// ----- Implementations -----

// Create creates an agent, generates a unique (non-duplicated) PIN, stores only a hash+index,
// creates default probes, and returns the plaintext PIN once.
func (s *service) Create(ctx context.Context, in CreateInput) (*CreateOutput, error) {
	pinLen := in.PinLength
	if pinLen <= 0 {
		pinLen = 9
	}

	// Generate a unique PIN by checking PinIndex collisions (race-safe due to DB unique index).
	var pin, pinIndex string
	var hash []byte
	const maxAttempts = 10
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var err error
		pin, err = generateNumericPIN(pinLen)
		if err != nil {
			return nil, err
		}
		pinIndex = s.pinIndex(pin)
		// Optional pre-check to avoid obvious collisions (unique index still the final arbiter)
		if _, err := s.repo.GetUnclaimedByPinIndex(ctx, pinIndex); err == nil {
			continue // collision, try again
		}
		hash, err = bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		break
	}
	if pin == "" {
		return nil, fmt.Errorf("failed to generate unique PIN after %d attempts", maxAttempts)
	}

	now := time.Now()
	idxPtr := &pinIndex
	a := &Agent{
		WorkspaceID:          in.WorkspaceID,
		Name:                 in.Name,
		Hostname:             in.Hostname,
		PinHash:              string(hash),
		PinIndex:             idxPtr, // non-nil until consumed
		PublicKey:            nil,    // blank until agent claims
		Location:             in.Location,
		PublicIPOverride:     in.PublicIPOverride,
		Version:              in.Version,
		Status:               StatusUnknown,
		LastSeenAt:           now,
		HeartbeatIntervalSec: ifZeroInt(in.HeartbeatIntervalSec, 60),
		Labels:               coalesceJSON(in.Labels),
		Metadata:             coalesceJSON(in.Metadata),
		Initialized:          false,
	}

	// Create agent + default probes in one transaction to keep things tidy.
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(a).Error; err != nil {
			return err
		}
		// Default probes for the new agent
		return s.createDefaultProbesTx(tx, a)
	})
	if err != nil {
		return nil, err
	}

	return &CreateOutput{Agent: a, PIN: pin}, nil
}

// Register (deprecated): create agent without returning PIN.
// Prefer Create() in admin APIs to return the PIN to the user.
func (s *service) Register(ctx context.Context, in RegisterInput) (*Agent, error) {
	var pin string
	if in.Pin != "" {
		pin = in.Pin
	} else {
		var err error
		pin, err = generateNumericPIN(9)
		if err != nil {
			return nil, err
		}
	}
	idx := s.pinIndex(pin)
	hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	a := &Agent{
		WorkspaceID:          in.WorkspaceID,
		Name:                 in.Name,
		Hostname:             in.Hostname,
		PinHash:              string(hash),
		PinIndex:             &idx,
		PublicKey:            nil,
		Location:             in.Location,
		PublicIPOverride:     in.PublicIPOverride,
		Version:              in.Version,
		Status:               StatusUnknown,
		LastSeenAt:           now,
		HeartbeatIntervalSec: ifZeroInt(in.HeartbeatIntervalSec, 60),
		Labels:               coalesceJSON(in.Labels),
		Metadata:             coalesceJSON(in.Metadata),
		Initialized:          false,
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(a).Error; err != nil {
			return err
		}
		return s.createDefaultProbesTx(tx, a)
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *service) Get(ctx context.Context, id uint) (*Agent, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, workspaceID uint, limit, offset int) ([]Agent, int64, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID, limit, offset)
}

func (s *service) Update(ctx context.Context, id uint, in UpdateInput) (*Agent, error) {
	a, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	// Apply only provided fields (note: removed Detected/Private/MAC/Platform/Arch)
	if in.Name != nil {
		a.Name = *in.Name
	}
	if in.Hostname != nil {
		a.Hostname = *in.Hostname
	}
	if in.Location != nil {
		a.Location = *in.Location
	}
	if in.PublicIPOverride != nil {
		a.PublicIPOverride = *in.PublicIPOverride
	}
	if in.Version != nil {
		a.Version = *in.Version
	}
	if in.Status != nil {
		a.Status = *in.Status
	}
	if in.HeartbeatIntervalSec != nil && *in.HeartbeatIntervalSec > 0 {
		a.HeartbeatIntervalSec = *in.HeartbeatIntervalSec
	}
	if in.Labels != nil {
		a.Labels = *in.Labels
	}
	if in.Metadata != nil {
		a.Metadata = *in.Metadata
	}
	a.UpdatedAt = time.Now()

	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Patch lets you update arbitrary columns without constructing UpdateInput.
func (s *service) Patch(ctx context.Context, id uint, fields map[string]any) error {
	if _, ok := fields["updated_at"]; !ok {
		fields["updated_at"] = time.Now()
	}
	return s.repo.PatchFields(ctx, id, fields)
}

// Beat updates timestamps/status for heartbeats (quick, contention-safe).
func (s *service) Beat(ctx context.Context, id uint, status Status) error {
	return s.repo.UpdateHeartbeat(ctx, id, time.Now(), status)
}

// RotatePin (legacy): caller supplies plaintext; we hash, compute new index, and store.
func (s *service) RotatePin(ctx context.Context, id uint, newPin string) error {
	if newPin == "" {
		return errors.New("pin cannot be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPin), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.RotatePinHash(ctx, id, string(hash), s.pinIndex(newPin))
}

// RotatePinAndReturn generates a unique PIN, stores hash+index, and returns plaintext once.
func (s *service) RotatePinAndReturn(ctx context.Context, id uint, length int) (*RotatePinOutput, error) {
	if length <= 0 {
		length = 9
	}
	const maxAttempts = 10
	var pin string
	for attempt := 0; attempt < maxAttempts; attempt++ {
		var err error
		pin, err = generateNumericPIN(length)
		if err != nil {
			return nil, err
		}
		idx := s.pinIndex(pin)
		if _, err := s.repo.GetUnclaimedByPinIndex(ctx, idx); err == nil {
			continue // collision; try again
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.DefaultCost)
		if err != nil {
			return nil, err
		}
		if err := s.repo.RotatePinHash(ctx, id, string(hash), idx); err != nil {
			return nil, err
		}
		return &RotatePinOutput{PIN: pin}, nil
	}
	return nil, fmt.Errorf("failed to generate unique PIN after %d attempts", maxAttempts)
}

// ---- Key bootstrap flow ----

func (s *service) cmpPIN(hash, pin string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pin))
}
func fp(pub []byte) string {
	sum := sha256.Sum256(pub)
	return hex.EncodeToString(sum[:])
}

// Challenge: verify PIN against unclaimed agent; return a short-lived nonce.
func (s *service) CreateChallenge(ctx context.Context, in ChallengeInput) (*ChallengeOutput, error) {
	a, err := s.repo.GetByWorkspaceAndID(ctx, in.WorkspaceID, in.AgentID)
	if err != nil {
		return nil, err
	}
	if a.PinConsumedAt != nil || len(a.PublicKey) > 0 {
		return nil, errors.New("agent already has a key")
	}
	if err := s.cmpPIN(a.PinHash, in.PIN); err != nil {
		return nil, errors.New("bad pin")
	}
	n := randomBase64URL(32)
	exp := time.Now().Add(5 * time.Minute)
	if err := s.repo.CreateNonce(ctx, a.ID, n, exp); err != nil {
		return nil, err
	}
	return &ChallengeOutput{AgentID: a.ID, Nonce: n, ExpiresAt: exp}, nil
}

// RegisterKey: consume nonce, verify signature over nonce with provided pubkey, store key.
func (s *service) RegisterKey(ctx context.Context, in RegisterKeyInput) (*Agent, error) {
	a, err := s.repo.GetByWorkspaceAndID(ctx, in.WorkspaceID, in.AgentID)
	if err != nil {
		return nil, err
	}
	if a.PinConsumedAt != nil || len(a.PublicKey) > 0 {
		return nil, errors.New("agent already has a key")
	}
	if err := s.cmpPIN(a.PinHash, in.PIN); err != nil {
		return nil, errors.New("bad pin")
	}
	agentID, err := s.repo.UseNonce(ctx, in.Nonce)
	if err != nil || agentID != a.ID {
		return nil, errors.New("invalid nonce")
	}
	if !ed25519.Verify(ed25519.PublicKey(in.PublicKey), []byte(in.Nonce), in.Signature) {
		return nil, errors.New("signature verify failed")
	}
	if err := s.repo.MarkPinConsumedAndStoreKey(ctx, a.ID, in.PublicKey, fp(in.PublicKey)); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, a.ID)
}

// ---- helpers ----

func ifZeroInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}

func randomBase64URL(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func generateNumericPIN(n int) (string, error) {
	const digits = "0123456789"
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

func (s *service) pinIndex(pin string) string {
	// sha256(pepper + ":" + pin)
	h := sha256.Sum256(append(append([]byte{}, s.pinPepper...), []byte(":"+pin)...))
	return hex.EncodeToString(h[:])
}
