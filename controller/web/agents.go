package web

import (
	"errors"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"

	"netwatcher-controller/internal"
	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/users"
)

// Mount workspace-scoped Agent routes using Iris parties (subpaths).
// We mirror the "workspaces" style: no PATCH/DELETE verbs; use POST .../update and POST .../delete paths.
func addRouteAgents(r *Router) []*Route {
	// Helpers shared by handlers
	parseUint := func(ctx iris.Context, name string) (uint, bool) {
		raw := ctx.Params().Get(name)
		id64, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, false
		}
		return uint(id64), true
	}
	currentUser := func(ctx iris.Context) (*users.User, error) {
		tok := bearer(ctx)
		if tok == "" {
			return nil, errors.New("missing token")
		}
		u, _, err := r.AuthSvc.GetUserFromJWT(ctx, tok, r.DB)
		return u, err
	}

	// ====== /workspaces/{id}/agents subtree ======
	ws := r.App.Party("/workspaces")
	ws.Use(VerifySession()) // protect subtree (remove if already globally applied)

	wsID := ws.Party("/{id:uint}")
	agents := wsID.Party("/agents")

	// GET /workspaces/{id}/agents -> list agents in a workspace
	agents.Get("/", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		wsID, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		// Must be a member to view agents
		if err := r.WorkspacesSvc.RequireMember(ctx, wsID, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		items, _, err := r.AgentsRepo.ListByWorkspace(ctx, wsID, 10_000, 0)
		if err != nil {
			log.WithError(err).Error("list agents by workspace")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		_ = ctx.JSON(items)
	})

	// POST /workspaces/{id}/agents -> create agent in workspace
	agents.Post("/", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		wsID, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		// Must be ADMIN+ to create agents
		if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		var body agent.Agent
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		now := time.Now()
		body.WorkspaceID = wsID
		body.CreatedAt = now
		body.UpdatedAt = now
		if body.Pin == "" {
			body.Pin = internal.GeneratePIN(6)
		}

		if err := r.AgentsRepo.Create(ctx, &body); err != nil {
			log.WithError(err).Error("create agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		_ = ctx.JSON(body)
	})

	// GET /workspaces/{id}/agents/{agentId} -> get one agent
	agents.Get("/{agentId:uint}", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		wsID, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		agentID, ok := parseUint(ctx, "agentId")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireMember(ctx, wsID, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		a, err := r.AgentsRepo.GetByID(ctx, agentID)
		if err != nil {
			log.WithError(err).Error("get agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		// Optional: ensure the agent belongs to this workspace
		if a.WorkspaceID != wsID {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}
		_ = ctx.JSON(a)
	})

	// POST /workspaces/{id}/agents/{agentId}/update -> update allowed fields
	agents.Post("/{agentId:uint}/update", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		wsID, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		agentID, ok := parseUint(ctx, "agentId")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		// Must be ADMIN+ to update
		if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		// confirm agent belongs to workspace
		cur, err := r.AgentsRepo.GetByID(ctx, agentID)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		if cur.WorkspaceID != wsID {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		var body agent.Agent
		if err := ctx.ReadJSON(&body); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		patch := map[string]any{
			"updated_at": time.Now(),
		}
		if body.Name != "" {
			patch["name"] = body.Name
		}
		if body.Location != "" {
			patch["location"] = body.Location
		}
		if body.PublicIPOverride != "" {
			patch["public_ip_override"] = body.PublicIPOverride
		}
		if body.Version != "" {
			patch["version"] = body.Version
		}
		// moving agents across workspaces should be rare; deny by default
		// if body.WorkspaceID != 0 && body.WorkspaceID != wsID { ... }

		if err := r.AgentsRepo.PatchFields(ctx, agentID, patch); err != nil {
			log.WithError(err).Error("update agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	// POST /workspaces/{id}/agents/{agentId}/deactivate -> soft disable
	agents.Post("/{agentId:uint}/deactivate", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		wsID, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		agentID, ok := parseUint(ctx, "agentId")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		cur, err := r.AgentsRepo.GetByID(ctx, agentID)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		if cur.WorkspaceID != wsID {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		if err := r.AgentsRepo.PatchFields(ctx, agentID, map[string]any{
			"status":     "DEACTIVATED",
			"updated_at": time.Now(),
		}); err != nil {
			log.WithError(err).Error("deactivate agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	// POST /workspaces/{id}/agents/{agentId}/delete -> delete agent (and clean probes)
	agents.Post("/{agentId:uint}/delete", func(ctx iris.Context) {
		u, err := currentUser(ctx)
		if err != nil {
			ctx.StatusCode(iris.StatusUnauthorized)
			return
		}
		wsID, ok := parseUint(ctx, "id")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		agentID, ok := parseUint(ctx, "agentId")
		if !ok {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}
		// ADMIN+ to delete
		if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		cur, err := r.AgentsRepo.GetByID(ctx, agentID)
		if err != nil {
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		if cur.WorkspaceID != wsID {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}

		// Best-effort: remove probes first
		if err := r.ProbesSvc.DeleteByAgent(ctx, agentID); err != nil {
			log.WithError(err).Warn("deleting probes for agent, continuing")
		}
		if err := r.AgentsRepo.Delete(ctx, agentID); err != nil {
			log.WithError(err).Error("delete agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	// Using parties, nothing to append to Router.Routes here.
	return nil
}
