package web

import (
	"net/http"
	"netwatcher-controller/internal"
	"strconv"
	"time"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"

	"netwatcher-controller/internal/agent"
)

// NOTE: Using r.AuthSvc.GetUserFromJWT for authN/authZ bootstrap.
// If you have workspace membership checks, insert them after fetching the user.

func addRouteAgents(r *Router) []*Route {
	var routes []*Route

	// GET /agents/site/{siteid}  -> list agents in a workspace/site
	routes = append(routes, &Route{
		Name: "Get Agents for Workspace",
		Path: "/agents/site/{siteid}",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			// Validate + get user (also validates session/expiry/id match)
			user, _, err := r.AuthSvc.GetUserFromJWT(ctx, token, r.DB)
			if err != nil || user == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			// TODO: optional: validate user is member of this workspace/site

			wsParam := ctx.Params().Get("siteid")
			wsID64, err := strconv.ParseUint(wsParam, 10, 64)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			wsID := uint(wsID64)

			agents, _, err := r.AgentsRepo.ListByWorkspace(ctx, wsID, 10000, 0)
			if err != nil {
				log.WithError(err).Error("failed to list agents by workspace")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			return ctx.JSON(agents)
		},
	})

	// GET /agents/delete/{agentid} -> delete agent (and its probes)
	routes = append(routes, &Route{
		Name: "Delete Agent",
		Path: "/agents/delete/{agentid}",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			user, _, err := r.AuthSvc.GetUserFromJWT(ctx, token, r.DB)
			if err != nil || user == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			// TODO: optional: verify user can delete this agent (workspace role)

			aidParam := ctx.Params().Get("agentid")
			aid64, err := strconv.ParseUint(aidParam, 10, 64)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			aid := uint(aid64)

			// cleanup probes first (FKs may also handle this)
			if err := r.ProbesSvc.DeleteByAgent(ctx, aid); err != nil {
				log.WithError(err).Warn("failed deleting probes for agent (continuing)")
			}
			if err := r.AgentsRepo.Delete(ctx, aid); err != nil {
				log.WithError(err).Error("failed deleting agent")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// POST /agents/new/{siteid} -> create agent inside workspace/site
	routes = append(routes, &Route{
		Name: "New Agent for Workspace",
		Path: "/agents/new/{siteid}",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			user, _, err := r.AuthSvc.GetUserFromJWT(ctx, token, r.DB)
			if err != nil || user == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			// TODO: optional: verify user can create agents in this workspace/site

			wsParam := ctx.Params().Get("siteid")
			wsID64, err := strconv.ParseUint(wsParam, 10, 64)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			wsID := uint(wsID64)

			var body agent.Agent
			if err := ctx.ReadJSON(&body); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			body.WorkspaceID = wsID
			now := time.Now()
			body.CreatedAt = now
			body.UpdatedAt = now
			if body.Pin == "" {
				if gen, ok := interface{}(internal.GeneratePIN(6)).(func() string); ok {
					body.Pin = gen()
				}
			}

			if err := r.AgentsRepo.Create(ctx, &body); err != nil {
				log.WithError(err).Error("failed to create agent")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			return ctx.JSON(body)
		},
	})

	// POST /agents/update/{agentid} -> patch allowed fields
	routes = append(routes, &Route{
		Name: "Update Agent",
		Path: "/agents/update/{agentid}",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			user, _, err := r.AuthSvc.GetUserFromJWT(ctx, token, r.DB)
			if err != nil || user == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			// TODO: optional: check workspace role

			aidParam := ctx.Params().Get("agentid")
			aid64, err := strconv.ParseUint(aidParam, 10, 64)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			aid := uint(aid64)

			var body agent.Agent
			if err := ctx.ReadJSON(&body); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
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
			if body.SiteID != 0 {
				patch["site_id"] = body.SiteID
			}
			if body.WorkspaceID != 0 {
				patch["workspace_id"] = body.WorkspaceID
			}

			if err := r.AgentsRepo.PatchFields(ctx, aid, patch); err != nil {
				log.WithError(err).Error("failed to update agent")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// GET /agents/deactivate/{agentid} -> deactivate
	routes = append(routes, &Route{
		Name: "Deactivate an agent",
		Path: "/agents/deactivate/{agentid}",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			user, _, err := r.AuthSvc.GetUserFromJWT(ctx, token, r.DB)
			if err != nil || user == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			// TODO: optional: check workspace role

			aidParam := ctx.Params().Get("agentid")
			aid64, err := strconv.ParseUint(aidParam, 10, 64)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			aid := uint(aid64)

			if err := r.AgentsRepo.PatchFields(ctx, aid, map[string]any{
				"active":      false,         // adjust for your schema
				"status":      "DEACTIVATED", // optional
				"updated_at":  time.Now(),
				"disabled_at": time.Now(), // optional
			}); err != nil {
				log.WithError(err).Error("failed to deactivate agent")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// GET /agents/{agentid} -> fetch one
	routes = append(routes, &Route{
		Name: "Get Agent",
		Path: "/agents/{agentid}",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			token := bearer(ctx)
			if token == "" {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			user, _, err := r.AuthSvc.GetUserFromJWT(ctx, token, r.DB)
			if err != nil || user == nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			aidParam := ctx.Params().Get("agentid")
			aid64, err := strconv.ParseUint(aidParam, 10, 64)
			if err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			aid := uint(aid64)

			ag, err := r.AgentsRepo.GetByID(ctx, aid)
			if err != nil {
				log.WithError(err).Error("failed to get agent")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			return ctx.JSON(ag)
		},
	})

	return routes
}
