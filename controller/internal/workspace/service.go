package workspace

import (
	"context"
	"strings"
	"time"

	"gorm.io/datatypes"
)

type Service interface {
	// Workspace lifecycle
	Create(ctx context.Context, in CreateInput) (*Workspace, error)
	Get(ctx context.Context, id uint) (*Workspace, error)
	Update(ctx context.Context, id uint, in UpdateInput) (*Workspace, error)
	Delete(ctx context.Context, id uint) error
	ListForUser(ctx context.Context, userID uint, limit, offset int) ([]Workspace, int64, error)

	// Membership
	AddMemberByUserID(ctx context.Context, workspaceID, userID uint, role Role) (*WorkspaceMember, error)
	InviteByEmail(ctx context.Context, workspaceID uint, email string, role Role) (*WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID uint) ([]WorkspaceMember, error)
	UpdateMemberRole(ctx context.Context, workspaceID, memberID uint, role Role) error
	RemoveMember(ctx context.Context, workspaceID, memberID uint) error
	TransferOwnership(ctx context.Context, workspaceID, newOwnerMemberID uint) error
}

type service struct {
	repo Repository
}

func NewService(repo Repository) Service { return &service{repo: repo} }

type CreateInput struct {
	Name        string
	Slug        string
	Description string
	Location    string
	OwnerUserID uint
	Labels      datatypes.JSON
	Metadata    datatypes.JSON
}

type UpdateInput struct {
	Name        *string
	Slug        *string
	Description *string
	Location    *string
	Labels      *datatypes.JSON
	Metadata    *datatypes.JSON
}

func (s *service) Create(ctx context.Context, in CreateInput) (*Workspace, error) {
	ws := &Workspace{
		Name:        strings.TrimSpace(in.Name),
		Slug:        strings.TrimSpace(in.Slug),
		Description: strings.TrimSpace(in.Description),
		Location:    strings.TrimSpace(in.Location),
		OwnerUserID: in.OwnerUserID,
		Labels:      coalesceJSON(in.Labels),
		Metadata:    coalesceJSON(in.Metadata),
	}
	if err := s.repo.CreateWorkspace(ctx, ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (s *service) Get(ctx context.Context, id uint) (*Workspace, error) {
	return s.repo.GetWorkspace(ctx, id)
}

func (s *service) Update(ctx context.Context, id uint, in UpdateInput) (*Workspace, error) {
	ws, err := s.repo.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		ws.Name = strings.TrimSpace(*in.Name)
	}
	if in.Slug != nil {
		ws.Slug = strings.TrimSpace(*in.Slug)
	}
	if in.Description != nil {
		ws.Description = strings.TrimSpace(*in.Description)
	}
	if in.Location != nil {
		ws.Location = strings.TrimSpace(*in.Location)
	}
	if in.Labels != nil {
		ws.Labels = *in.Labels
	}
	if in.Metadata != nil {
		ws.Metadata = *in.Metadata
	}
	ws.UpdatedAt = time.Now()
	if err := s.repo.UpdateWorkspace(ctx, ws); err != nil {
		return nil, err
	}
	return ws, nil
}

func (s *service) Delete(ctx context.Context, id uint) error {
	return s.repo.DeleteWorkspace(ctx, id)
}

func (s *service) ListForUser(ctx context.Context, userID uint, limit, offset int) ([]Workspace, int64, error) {
	return s.repo.ListWorkspacesForUser(ctx, userID, limit, offset)
}

// ----- Membership -----

func (s *service) AddMemberByUserID(ctx context.Context, workspaceID, userID uint, role Role) (*WorkspaceMember, error) {
	return s.repo.AddMemberByUserID(ctx, workspaceID, userID, role)
}

func (s *service) InviteByEmail(ctx context.Context, workspaceID uint, email string, role Role) (*WorkspaceMember, error) {
	return s.repo.InviteMemberByEmail(ctx, workspaceID, strings.ToLower(strings.TrimSpace(email)), role)
}

func (s *service) ListMembers(ctx context.Context, workspaceID uint) ([]WorkspaceMember, error) {
	return s.repo.ListMembers(ctx, workspaceID)
}

func (s *service) UpdateMemberRole(ctx context.Context, workspaceID, memberID uint, role Role) error {
	return s.repo.UpdateMemberRole(ctx, workspaceID, memberID, role)
}

func (s *service) RemoveMember(ctx context.Context, workspaceID, memberID uint) error {
	return s.repo.RemoveMember(ctx, workspaceID, memberID)
}

func (s *service) TransferOwnership(ctx context.Context, workspaceID, newOwnerMemberID uint) error {
	return s.repo.TransferOwnership(ctx, workspaceID, newOwnerMemberID)
}

// ----- helpers -----

func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}
