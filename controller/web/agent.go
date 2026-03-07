package web

import (
	"errors"
	"net/http"
	"netwatcher-controller/internal/agent"
	"time"

	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ---- Request / Response payloads ----

type agentLoginRequest struct {
	// Optional body duplicates of path params; path takes precedence if present.
	WorkspaceID uint   `json:"workspace_id,omitempty"`
	AgentID     uint   `json:"agent_id,omitempty"`
	PSK         string `json:"psk,omitempty"`
	PIN         string `json:"pin,omitempty"`
}

type agentLoginResponse struct {
	Status string       `json:"status"`          // "ok" | "bootstrapped" | "deleted"
	PSK    string       `json:"psk,omitempty"`   // only on bootstrap
	Agent  *agent.Agent `json:"agent,omitempty"` // convenience
	Error  string       `json:"error,omitempty"` // on failure
}

// ---- Route registration ----

// Call this from your router setup, e.g.:
//
//	api := app.Group("/api")
//	agentAuth(api, r.DB)
func agentAuth(api fiber.Router, db *gorm.DB) {
	// Base: /agent
	base := api.Group("/agent")

	// POST /agent or /agent/login - both work for agent authentication
	loginHandler := func(c *fiber.Ctx) error {
		c.Set("Content-Type", "application/json")

		var req agentLoginRequest
		_ = c.BodyParser(&req) // ignore error; fields are optional

		// 1) Prefer PSK if provided
		if req.PSK != "" {
			a, err := agent.AuthenticateWithPSK(c.UserContext(), db, req.WorkspaceID, req.AgentID, req.PSK)
			if err != nil {
				// Check if agent was deleted - return 410 Gone to signal permanent removal
				if errors.Is(err, agent.ErrAgentDeleted) {
					log.Infof("Agent %d/%d attempted login after deletion - returning 410", req.WorkspaceID, req.AgentID)
					return c.Status(http.StatusGone).JSON(agentLoginResponse{Status: "deleted", Error: "agent_deleted"})
				}
				// Check for transient server errors (DB down, storage full, etc.)
				// Return 503 so agents know to retry instead of deactivating
				if errors.Is(err, agent.ErrServerError) {
					log.Warnf("Agent %d/%d login failed due to server error: %v", req.WorkspaceID, req.AgentID, err)
					return c.Status(http.StatusServiceUnavailable).JSON(agentLoginResponse{Error: "server_error"})
				}
				return c.Status(http.StatusUnauthorized).JSON(agentLoginResponse{Error: "invalid_psk"})
			}
			// Lightweight heartbeat
			if err := agent.UpdateAgentSeen(c.UserContext(), db, a.ID, time.Now()); err != nil {
				log.WithError(err).Warn("update last seen failed (psk login)")
			}
			return c.Status(http.StatusOK).JSON(agentLoginResponse{
				Status: "ok",
				Agent:  a,
			})
		}

		// 2) No PSK → try PIN bootstrap, if provided
		if req.PIN != "" {
			out, err := agent.BootstrapWithPIN(c.UserContext(), db, agent.BootstrapWithPINInput{
				WorkspaceID: req.WorkspaceID,
				AgentID:     req.AgentID,
				PIN:         req.PIN,
			})
			if err != nil {
				return c.Status(http.StatusUnauthorized).JSON(agentLoginResponse{Error: "pin_verification_failed"})
			}
			if err := agent.UpdateAgentSeen(c.UserContext(), db, out.Agent.ID, time.Now()); err != nil {
				log.WithError(err).Warn("update last seen failed (pin bootstrap)")
			}
			return c.Status(http.StatusOK).JSON(agentLoginResponse{
				Status: "success",
				PSK:    out.PSK, // <-- show once
				Agent:  out.Agent,
			})
		}

		// 3) Neither PSK nor PIN
		return c.Status(http.StatusBadRequest).JSON(agentLoginResponse{Error: "psk_or_pin_required"})
	}

	// Register handler on both routes for compatibility
	base.Post("/", loginHandler)
	base.Post("/login", loginHandler)
}
