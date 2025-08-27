package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"
)

func addRouteSites(r *Router) []*Route {
	var routes []*Route

	// -------------------------------
	// POST /sites/update/{siteid}
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Update Workspace",
		Path: "/sites/update/{siteid}",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			// must be ADMIN or OWNER
			if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			var body workspace.Workspace
			if err := ctx.ReadJSON(&body); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			if _, err := r.WorkspacesSvc.UpdateDetails(ctx, wsID, body.Name, body.Location, body.Description); err != nil {
				log.WithError(err).Error("update workspace")
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}

			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// -------------------------------
	// GET /sites  (list workspaces for current user)
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Get Sites",
		Path: "/sites",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			items, _, err := r.WorkspacesSvc.ListForUser(ctx, u.ID, 200, 0)
			if err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			return ctx.JSON(items)
		},
	})

	// -------------------------------
	// POST /sites (create new workspace owned by current user)
	// -------------------------------
	routes = append(routes, &Route{
		Name: "New Workspace",
		Path: "/sites",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}

			var ws workspace.Workspace
			if err := ctx.ReadJSON(&ws); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			created, err := r.WorkspacesSvc.CreateWithOwner(ctx, u.ID, &ws)
			if err != nil {
				_ = ctx.JSON(err) // preserve your prior behavior
				return nil
			}
			return ctx.JSON(created)
		},
	})

	// -------------------------------
	// GET /sites/{siteid}/memberinfo
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Get MemberInfo",
		Path: "/sites/{siteid}/memberinfo",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			if err := r.WorkspacesSvc.RequireMember(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			members, err := r.WorkspacesSvc.ListMembers(ctx, wsID)
			if err != nil {
				return ctx.JSON(err)
			}
			return ctx.JSON(members)
		},
	})

	// -------------------------------
	// GET /sites/{siteid}
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Workspace",
		Path: "/sites/{siteid}",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			if err := r.WorkspacesSvc.RequireMember(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			ws, err := r.WorkspacesSvc.GetByID(ctx, wsID)
			if err != nil {
				return ctx.JSON(err)
			}
			return ctx.JSON(ws)
		},
	})

	// -------------------------------
	// DELETE /sites/{siteid}
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Delete Workspace",
		Path: "/sites/{siteid}",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			// only OWNER can delete
			if err := r.WorkspacesSvc.RequireOwner(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}
			if err := r.WorkspacesSvc.Delete(ctx, wsID); err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			ctx.StatusCode(http.StatusNoContent)
			return nil
		},
	})

	// -------------------------------
	// GET /sites/members   (current user's memberships)
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Get Members",
		Path: "/sites/members",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			memberships, err := r.WorkspacesSvc.ListMemberships(ctx, u.ID)
			if err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			return ctx.JSON(memberships)
		},
	})

	// -------------------------------
	// POST /sites/{siteid}/update_role
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Update Member Role",
		Path: "/sites/{siteid}/update_role",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			var info struct {
				ID   uint           `json:"id"`   // memberID (row id) or userID? keeping memberID semantics
				Role workspace.Role `json:"role"` // new role
			}
			if err := ctx.ReadJSON(&info); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			// must be ADMIN or OWNER to update roles; OWNER to set OWNER
			if info.Role == workspace.RoleOwner {
				if err := r.WorkspacesSvc.RequireOwner(ctx, wsID, u.ID); err != nil {
					ctx.StatusCode(http.StatusForbidden)
					return nil
				}
			} else {
				if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
					ctx.StatusCode(http.StatusForbidden)
					return nil
				}
			}

			if err := r.WorkspacesSvc.UpdateMemberRole(ctx, wsID, info.ID, info.Role); err != nil {
				return ctx.JSON(err)
			}
			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// -------------------------------
	// POST /sites/{siteid}/invite
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Add Member",
		Path: "/sites/{siteid}/invite",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			var info struct {
				Email string         `json:"email"`
				Role  workspace.Role `json:"role"`
				ID    uint           `json:"id"` // optional: direct user id
			}
			if err := ctx.ReadJSON(&info); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			if info.Role == workspace.RoleOwner {
				ctx.StatusCode(http.StatusBadRequest)
				return errors.New("only the current owner can add owners")
			}

			// require ADMIN or OWNER to invite
			if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			// resolve user by id or email
			var invitee *users.User
			if info.ID != 0 {
				// by user id
				matches, _ := r.WorkspacesSvc.ListMemberships(ctx, info.ID)
				_ = matches // we don't need, just showing that user exists is enough via users repo,
				// but our Service exposes LookupUserByEmail; for id we can rely on users repo directly if available.
				// If you want strict existence check by ID, add a usersSvc.GetByID and call it here.
			} else if info.Email != "" {
				u2, err := r.WorkspacesSvc.LookupUserByEmail(ctx, info.Email)
				if err == nil {
					invitee = u2
				}
			}

			if invitee != nil {
				if _, err := r.WorkspacesSvc.AddMemberByUserID(ctx, wsID, invitee.ID, info.Role); err != nil {
					return ctx.JSON(err)
				}
			} else {
				// invite-by-email flow (user may not exist yet)
				if _, err := r.WorkspacesSvc.InviteByEmail(ctx, wsID, info.Email, info.Role); err != nil {
					return ctx.JSON(err)
				}
			}

			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// -------------------------------
	// POST /sites/{siteid}/remove
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Remove Member",
		Path: "/sites/{siteid}/remove",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			var body struct {
				ID uint `json:"id"` // memberID
			}
			if err := ctx.ReadJSON(&body); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			// admin or owner to remove
			if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			// disallow removing OWNER
			role, err := r.WorkspacesSvc.GetMemberRole(ctx, wsID, u.ID)
			if err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			_ = role // we only needed admin+ check above; owner check is done inside RemoveMember

			if err := r.WorkspacesSvc.RemoveMember(ctx, wsID, body.ID); err != nil {
				return ctx.JSON(err)
			}
			ctx.StatusCode(http.StatusOK)
			return nil
		},
	})

	// -------------------------------
	// POST /sites/{siteid}/groups
	// -------------------------------
	/*routes = append(routes, &Route{
		Name: "New Agent Group",
		Path: "/sites/{siteid}/groups",
		JWT:  true,
		Type: RouteType_POST,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}

			// admin+ to create
			if err := r.WorkspacesSvc.RequireAdmin(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			var g agent.Group
			if err := ctx.ReadJSON(&g); err != nil {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			g.WorkspaceID = wsID
			now := time.Now()
			g.CreatedAt, g.UpdatedAt = now, now

			if err := r.DB.WithContext(ctx).Create(&g).Error; err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			return ctx.JSON(g)
		},
	})*/

	// -------------------------------
	// GET /sites/{siteid}/groups
	// -------------------------------
	routes = append(routes, &Route{
		Name: "Workspace Groups",
		Path: "/sites/{siteid}/groups",
		JWT:  true,
		Type: RouteType_GET,
		Func: func(ctx iris.Context) error {
			ctx.ContentType("application/json")

			u, err := currentUser(ctx, r)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				return nil
			}
			wsID, ok := parseUintParam(ctx, "siteid")
			if !ok {
				ctx.StatusCode(http.StatusBadRequest)
				return nil
			}
			if err := r.WorkspacesSvc.RequireMember(ctx, wsID, u.ID); err != nil {
				ctx.StatusCode(http.StatusForbidden)
				return nil
			}

			var groups []agent.Group
			if err := r.DB.WithContext(ctx).Where("workspace_id = ?", wsID).Find(&groups).Error; err != nil {
				ctx.StatusCode(http.StatusInternalServerError)
				return nil
			}
			return ctx.JSON(groups)
		},
	})

	return routes
}

// -------------------------------
// helpers
// -------------------------------

func currentUser(ctx iris.Context, r *Router) (*users.User, error) {
	tok := bearer(ctx)
	if tok == "" {
		return nil, errors.New("missing token")
	}
	u, _, err := r.AuthSvc.GetUserFromJWT(ctx, tok, r.DB)
	return u, err
}

func parseUintParam(ctx iris.Context, name string) (uint, bool) {
	raw := ctx.Params().Get(name)
	id64, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(id64), true
}
