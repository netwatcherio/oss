package web

import (
	"gorm.io/datatypes"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/users"
)

// Mount workspace-scoped Agent routes using Iris parties (subpaths).
// Style: GET/POST with action endpoints for mutations.
func addRouteAgents(r *Router) {
	currentUser := func(ctx iris.Context) (*users.User, error) {
		// GetClaims(ctx) should return your *auth.Session claims
		sess := GetClaims(ctx)
		u, _, err := r.AuthSvc.GetUserFromJWT(ctx, sess, r.DB)
		return u, err
	}

	// Helpers shared by handlers
	parseUint := func(ctx iris.Context, name string) (uint, bool) {
		raw := ctx.Params().Get(name)
		id64, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return 0, false
		}
		return uint(id64), true
	}

	type createAgentReq struct {
		Name                 string         `json:"name"`
		Hostname             string         `json:"hostname"`
		Location             string         `json:"location"`
		PublicIPOverride     string         `json:"publicIpOverride"`
		Version              string         `json:"version"`
		HeartbeatIntervalSec int            `json:"heartbeatIntervalSec"`
		PinLength            int            `json:"pinLength"` // optional; default 9 if 0
		Labels               datatypes.JSON `json:"labels"`
		Metadata             datatypes.JSON `json:"metadata"`
	}

	type createAgentResp struct {
		Agent *agent.Agent `json:"agent"`
		PIN   string       `json:"pin"` // plaintext PIN shown once
	}

	// ====== /workspaces/{id}/agents subtree ======
	ws := r.App.Party("/workspaces")
	ws.Use(VerifySession()) // protect this subtree (remove if already globally applied)

	wsID := ws.Party("/{id:uint}")
	agentsParty := wsID.Party("/agents")

	// GET /workspaces/{id}/agents -> list agents in a workspace
	agentsParty.Get("/", func(ctx iris.Context) {
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

	// POST /workspaces/{id}/agents -> create agent in workspace (returns plaintext PIN once)
	agentsParty.Post("/", func(ctx iris.Context) {
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

		var in createAgentReq
		if err := ctx.ReadJSON(&in); err != nil {
			ctx.StatusCode(iris.StatusBadRequest)
			return
		}

		out, err := r.AgentsSvc.Create(ctx, agent.CreateInput{
			WorkspaceID:          wsID,
			Name:                 in.Name,
			Hostname:             in.Hostname,
			PinLength:            in.PinLength, // defaults to 9 inside service if 0
			Location:             in.Location,
			PublicIPOverride:     in.PublicIPOverride,
			Version:              in.Version,
			HeartbeatIntervalSec: in.HeartbeatIntervalSec,
			Labels:               in.Labels,
			Metadata:             in.Metadata,
		})
		if err != nil {
			log.WithError(err).Error("create agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}

		ctx.StatusCode(iris.StatusCreated)
		_ = ctx.JSON(createAgentResp{
			Agent: out.Agent,
			PIN:   out.PIN,
		})
	})

	// GET /workspaces/{id}/agents/{agentId} -> get one agent
	agentsParty.Get("/{agentId:uint}", func(ctx iris.Context) {
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
		if a.WorkspaceID != wsID {
			ctx.StatusCode(iris.StatusForbidden)
			return
		}
		_ = ctx.JSON(a)
	})

	// POST /workspaces/{id}/agents/{agentId}/update -> update allowed fields
	agentsParty.Post("/{agentId:uint}/update", func(ctx iris.Context) {
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

		if err := r.AgentsRepo.PatchFields(ctx, agentID, patch); err != nil {
			log.WithError(err).Error("update agent")
			ctx.StatusCode(iris.StatusInternalServerError)
			return
		}
		ctx.StatusCode(iris.StatusOK)
	})

	// POST /workspaces/{id}/agents/{agentId}/deactivate -> soft disable
	agentsParty.Post("/{agentId:uint}/deactivate", func(ctx iris.Context) {
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
	agentsParty.Post("/{agentId:uint}/delete", func(ctx iris.Context) {
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
}
