package agent

import (
	"context"
	"time"

	"gorm.io/datatypes"
)

// Service offers business-logic level helpers (compose your HTTP/RPC handlers atop this).
type Service interface {
	Register(ctx context.Context, in RegisterInput) (*Agent, error)
	Get(ctx context.Context, id uint) (*Agent, error)
	List(ctx context.Context, workspaceID uint, limit, offset int) ([]Agent, int64, error)
	Update(ctx context.Context, id uint, in UpdateInput) (*Agent, error)
	Patch(ctx context.Context, id uint, fields map[string]any) error
	Beat(ctx context.Context, id uint, status Status) error
	RotatePin(ctx context.Context, id uint, newPin string) error
	Delete(ctx context.Context, id uint) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

// ----- DTOs -----

type RegisterInput struct {
	WorkspaceID          uint
	SiteID               uint
	Name                 string
	Hostname             string
	Pin                  string
	PublicKey            string
	Location             string
	PublicIPOverride     string
	DetectedPublicIP     string
	PrivateIP            string
	MACAddress           string
	Version              string
	Platform             string
	Arch                 string
	HeartbeatIntervalSec int
	Labels               datatypes.JSON
	Metadata             datatypes.JSON
}

type UpdateInput struct {
	Name                 *string
	Hostname             *string
	Location             *string
	PublicIPOverride     *string
	DetectedPublicIP     *string
	PrivateIP            *string
	MACAddress           *string
	Version              *string
	Platform             *string
	Arch                 *string
	Status               *Status
	HeartbeatIntervalSec *int
	Labels               *datatypes.JSON
	Metadata             *datatypes.JSON
}

// ----- Implementations -----

func (s *service) Register(ctx context.Context, in RegisterInput) (*Agent, error) {
	now := time.Now()
	a := &Agent{
		WorkspaceID:          in.WorkspaceID,
		SiteID:               in.SiteID,
		Name:                 in.Name,
		Hostname:             in.Hostname,
		Pin:                  in.Pin,
		PublicKey:            in.PublicKey,
		Location:             in.Location,
		PublicIPOverride:     in.PublicIPOverride,
		DetectedPublicIP:     in.DetectedPublicIP,
		PrivateIP:            in.PrivateIP,
		MACAddress:           in.MACAddress,
		Version:              in.Version,
		Platform:             in.Platform,
		Arch:                 in.Arch,
		Status:               StatusUnknown,
		LastSeenAt:           now,
		HeartbeatIntervalSec: ifZeroInt(in.HeartbeatIntervalSec, 60),
		Labels:               coalesceJSON(in.Labels),
		Metadata:             coalesceJSON(in.Metadata),
		Initialized:          true,
	}
	if err := s.repo.Create(ctx, a); err != nil {
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
	// Apply only provided fields
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
	if in.DetectedPublicIP != nil {
		a.DetectedPublicIP = *in.DetectedPublicIP
	}
	if in.PrivateIP != nil {
		a.PrivateIP = *in.PrivateIP
	}
	if in.MACAddress != nil {
		a.MACAddress = *in.MACAddress
	}
	if in.Version != nil {
		a.Version = *in.Version
	}
	if in.Platform != nil {
		a.Platform = *in.Platform
	}
	if in.Arch != nil {
		a.Arch = *in.Arch
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
	// Always touch updated_at for patches unless caller provided explicitly
	if _, ok := fields["updated_at"]; !ok {
		fields["updated_at"] = time.Now()
	}
	return s.repo.PatchFields(ctx, id, fields)
}

// Beat updates timestamps/status for heartbeats (quick, contention-safe).
func (s *service) Beat(ctx context.Context, id uint, status Status) error {
	return s.repo.UpdateHeartbeat(ctx, id, time.Now(), status)
}

func (s *service) RotatePin(ctx context.Context, id uint, newPin string) error {
	return s.repo.RotatePin(ctx, id, newPin)
}

func (s *service) Delete(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}

// ---- helpers ----

func ifZeroInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

// coalesceJSON ensures non-nil empty JSON rather than null if desired
func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}
