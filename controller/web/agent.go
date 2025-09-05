package web

import (
	"net/http"
	"netwatcher-controller/internal/agent"
	"os"
	"time"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ---- Request / Response payloads ----

type agentLoginRequest struct {
	// Optional body duplicates of path params; path takes precedence if present.
	WorkspaceID uint   `json:"workspaceId,omitempty"`
	AgentID     uint   `json:"agentId,omitempty"`
	PSK         string `json:"psk,omitempty"`
	PIN         string `json:"pin,omitempty"`
}

type agentLoginResponse struct {
	Status string       `json:"status"`          // "ok" | "bootstrapped"
	PSK    string       `json:"psk,omitempty"`   // only on bootstrap
	Agent  *agent.Agent `json:"agent,omitempty"` // convenience
	Error  string       `json:"error,omitempty"` // on failure
}

// ---- Route registration ----

// Call this from your router setup, e.g.:
//
//	api := app.Party("/api")
//	agentAuth(api, r.DB)
func agentAuth(api iris.Party, db *gorm.DB) {
	// Base: /workspaces/{workspaceID}/agents/{agentID}
	base := api.Party("/agent")

	// POST /workspaces/{workspaceID}/agents/{agentID}/login
	base.Post("/", func(ctx iris.Context) {
		ctx.ContentType("application/json")

		var req agentLoginRequest
		_ = ctx.ReadJSON(&req) // ignore error; fields are optional

		/*		if workspaceID == 0 || agentID == 0 {
				ctx.StatusCode(http.StatusBadRequest)
				_ = ctx.JSON(agentLoginResponse{Error: "workspaceId_and_agentId_required"})
				return
			}*/

		// 1) Prefer PSK if provided
		if req.PSK != "" {
			a, err := agent.AuthenticateWithPSK(ctx, db, req.WorkspaceID, req.AgentID, req.PSK)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				_ = ctx.JSON(agentLoginResponse{Error: "invalid_psk"})
				return
			}
			// Lightweight heartbeat
			if err := agent.UpdateAgentSeen(ctx, db, a.ID, time.Now()); err != nil {
				log.WithError(err).Warn("update last seen failed (psk login)")
			}
			ctx.StatusCode(http.StatusOK)
			_ = ctx.JSON(agentLoginResponse{
				Status: "ok",
				Agent:  a,
			})
			return
		}

		// 2) No PSK → try PIN bootstrap, if provided
		if req.PIN != "" {
			pepper := os.Getenv("PIN_PEPPER")
			out, err := agent.BootstrapWithPIN(ctx, db, agent.BootstrapWithPINInput{
				WorkspaceID: req.WorkspaceID,
				AgentID:     req.AgentID,
				PIN:         req.PIN,
			}, pepper)
			if err != nil {
				ctx.StatusCode(http.StatusUnauthorized)
				_ = ctx.JSON(agentLoginResponse{Error: "pin_verification_failed"})
				return
			}
			if err := agent.UpdateAgentSeen(ctx, db, out.Agent.ID, time.Now()); err != nil {
				log.WithError(err).Warn("update last seen failed (pin bootstrap)")
			}
			ctx.StatusCode(http.StatusOK)
			_ = ctx.JSON(agentLoginResponse{
				Status: "bootstrapped",
				PSK:    out.PSK, // <-- show once
				Agent:  out.Agent,
			})
			return
		}

		// 3) Neither PSK nor PIN
		ctx.StatusCode(http.StatusBadRequest)
		_ = ctx.JSON(agentLoginResponse{Error: "psk_or_pin_required"})
	})
}
