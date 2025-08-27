package workspace

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/datatypes"

	userpkg "netwatcher-controller/internal/users"
)

type Service interface {
	// Workspace lifecycle
	Create(ctx context.Context, in CreateInput) (*Workspace, error)
	// Convenience used by web layer: create from owner + struct
	CreateWithOwner(ctx context.Context, ownerUserID uint, ws *Workspace) (*Workspace, error)

	Get(ctx context.Context, id uint) (*Workspace, error)     // original
	GetByID(ctx context.Context, id uint) (*Workspace, error) // alias

	Update(ctx context.Context, id uint, in UpdateInput) (*Workspace, error)
	UpdateDetails(ctx context.Context, id uint, name, location, description string) (*Workspace, error)
	Delete(ctx context.Context, id uint) error
	ListForUser(ctx context.Context, userID uint, limit, offset int) ([]Workspace, int64, error)

	// Membership
	AddMemberByUserID(ctx context.Context, workspaceID, userID uint, role Role) (*WorkspaceMember, error)
	InviteByEmail(ctx context.Context, workspaceID uint, email string, role Role) (*WorkspaceMember, error)
	ListMembers(ctx context.Context, workspaceID uint) ([]WorkspaceMember, error)
	UpdateMemberRole(ctx context.Context, workspaceID, memberID uint, role Role) error
	RemoveMember(ctx context.Context, workspaceID, memberID uint) error
	TransferOwnership(ctx context.Context, workspaceID, newOwnerMemberID uint) error

	// Membership queries for authZ
	ListMemberships(ctx context.Context, userID uint) ([]WorkspaceMember, error)
	GetMemberRole(ctx context.Context, workspaceID, userID uint) (Role, error)
	RequireMember(ctx context.Context, workspaceID, userID uint) error
	RequireAdmin(ctx context.Context, workspaceID, userID uint) error
	RequireOwner(ctx context.Context, workspaceID, userID uint) error

	// Helpers used by routes
	LookupUserByEmail(ctx context.Context, email string) (*userpkg.User, error)
}

type service struct {
	repo      Repository
	usersRepo userpkg.Repository
}

func NewService(repo Repository, usersRepo userpkg.Repository) Service {
	return &service{repo: repo, usersRepo: usersRepo}
}

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

// CreateWithOwner is a convenience to match your web layer call pattern.
func (s *service) CreateWithOwner(ctx context.Context, ownerUserID uint, ws *Workspace) (*Workspace, error) {
	in := CreateInput{
		Name:        ws.Name,
		Slug:        ws.Slug,
		Description: ws.Description,
		Location:    ws.Location,
		OwnerUserID: ownerUserID,
		Labels:      ws.Labels,
		Metadata:    ws.Metadata,
	}
	return s.Create(ctx, in)
}

func (s *service) Get(ctx context.Context, id uint) (*Workspace, error) {
	return s.repo.GetWorkspace(ctx, id)
}
func (s *service) GetByID(ctx context.Context, id uint) (*Workspace, error) {
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

func (s *service) UpdateDetails(ctx context.Context, id uint, name, location, description string) (*Workspace, error) {
	ws, err := s.repo.GetWorkspace(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != "" {
		ws.Name = strings.TrimSpace(name)
	}
	if location != "" {
		ws.Location = strings.TrimSpace(location)
	}
	if description != "" {
		ws.Description = strings.TrimSpace(description)
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

// ----- Membership queries for authZ -----

func (s *service) ListMemberships(ctx context.Context, userID uint) ([]WorkspaceMember, error) {
	return s.repo.ListMembershipsForUser(ctx, userID)
}

func (s *service) GetMemberRole(ctx context.Context, workspaceID, userID uint) (Role, error) {
	return s.repo.GetMemberRole(ctx, workspaceID, userID)
}

func (s *service) RequireMember(ctx context.Context, workspaceID, userID uint) error {
	ok, err := s.repo.IsMember(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrMemberNotFound
	}
	return nil
}

func (s *service) RequireAdmin(ctx context.Context, workspaceID, userID uint) error {
	role, err := s.repo.GetMemberRole(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if role != RoleAdmin && role != RoleOwner {
		return errors.New("forbidden: requires ADMIN or OWNER")
	}
	return nil
}

func (s *service) RequireOwner(ctx context.Context, workspaceID, userID uint) error {
	role, err := s.repo.GetMemberRole(ctx, workspaceID, userID)
	if err != nil {
		return err
	}
	if role != RoleOwner {
		return errors.New("forbidden: requires OWNER")
	}
	return nil
}

// ----- Helpers -----

func (s *service) LookupUserByEmail(ctx context.Context, email string) (*userpkg.User, error) {
	em := strings.ToLower(strings.TrimSpace(email))
	if em == "" {
		return nil, errors.New("invalid email")
	}
	return s.usersRepo.GetByEmail(ctx, em)
}

func coalesceJSON(j datatypes.JSON) datatypes.JSON {
	if len(j) == 0 {
		return datatypes.JSON([]byte(`{}`))
	}
	return j
}
