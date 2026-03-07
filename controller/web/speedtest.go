// web/speedtest.go
package web

import (
	"database/sql"
	"net/http"
	"netwatcher-controller/internal/speedtest"
	"netwatcher-controller/internal/workspace"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// panelSpeedtest registers speedtest queue and server endpoints under agents.
// Routes: /workspaces/:id/agents/:agentID/speedtest-*
func panelSpeedtest(api fiber.Router, db *gorm.DB, ch *sql.DB) {
	ws := api.Group("/workspaces/:id")
	wsStore := workspace.NewStore(db)

	// Speedtest routes under individual agents
	st := ws.Group("/agents/:agentID")
	st.Use(RequireWorkspaceAccess(wsStore))

	// -------------------- Queue Endpoints --------------------

	// GET /workspaces/:id/agents/:agentID/speedtest-queue
	// List queue items for an agent
	st.Get("/speedtest-queue", func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		var status *speedtest.QueueStatus
		if s := c.Query("status"); s != "" {
			st := speedtest.QueueStatus(s)
			status = &st
		}

		items, err := speedtest.ListForAgent(c.UserContext(), db, aID, status)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": items, "total": len(items)})
	})

	// POST /workspaces/:id/agents/:agentID/speedtest-queue
	// Add a new speedtest to the queue - requires CanEdit (USER+)
	st.Post("/speedtest-queue", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")
		aID := uintParam(c, "agentID")
		userID := getUserID(c)

		var body struct {
			ServerID   string `json:"server_id"`
			ServerName string `json:"server_name"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		item, err := speedtest.CreateQueueItem(c.UserContext(), db, speedtest.CreateQueueInput{
			WorkspaceID: wsID,
			AgentID:     aID,
			ServerID:    body.ServerID,
			ServerName:  body.ServerName,
			RequestedBy: userID,
		})
		if err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}

		return c.Status(http.StatusCreated).JSON(item)
	})

	// GET /workspaces/:id/agents/:agentID/speedtest-queue/:queueID
	// Get a specific queue item
	st.Get("/speedtest-queue/:queueID", func(c *fiber.Ctx) error {
		qID := uintParam(c, "queueID")
		item, err := speedtest.GetQueueItem(c.UserContext(), db, qID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "queue item not found"})
		}
		return c.JSON(item)
	})

	// DELETE /workspaces/:id/agents/:agentID/speedtest-queue/:queueID
	// Cancel a pending queue item - requires CanEdit (USER+)
	st.Delete("/speedtest-queue/:queueID", RequireRole(wsStore, CanEdit), func(c *fiber.Ctx) error {
		qID := uintParam(c, "queueID")
		if err := speedtest.CancelQueueItem(c.UserContext(), db, qID); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// -------------------- Server Cache Endpoints --------------------

	// GET /workspaces/:id/agents/:agentID/speedtest-servers
	// Get cached speedtest servers for an agent
	st.Get("/speedtest-servers", func(c *fiber.Ctx) error {
		aID := uintParam(c, "agentID")
		servers, err := speedtest.ListServersForAgent(c.UserContext(), db, aID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": servers, "total": len(servers)})
	})
}
