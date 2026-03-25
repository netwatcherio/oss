// web/router.go
package web

import (
	"database/sql"
	"net/http"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/limits"
	"netwatcher-controller/internal/oui"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RegisterRoutes mounts all REST / Fiber routes (no WebSocket routes here).
func RegisterRoutes(app *fiber.App, db *gorm.DB, ch *sql.DB, emailStore *email.QueueStore, geoStore *geoip.Store, ouiStore *oui.Store) {
	limitsConfig := limits.LoadFromEnv()

	// ----- Public (no auth) -----
	registerAuthRoutes(app, db, emailStore)
	agentAuth(app, db)
	RegisterInviteRoutes(app, db, emailStore)
	RegisterAgentAPI(app, db, ch, geoStore)

	// Public share access routes — MUST be registered before the JWT group,
	// because app.Group("/") applies its middleware to all routes declared after it.
	RegisterShareRoutes(app, db, ch)

	// ----- Protected (JWT) -----
	api := app.Group("/")
	api.Use(JWTMiddleware(db))

	panelWorkspaces(api, db, emailStore, limitsConfig)
	panelProbes(api, db, limitsConfig)
	panelAgents(api, db, ch, limitsConfig)
	panelProbeData(api, db, ch)
	panelSpeedtest(api, db, ch)
	panelGeoIP(api, geoStore, ch)
	panelWhois(api, ch)
	panelLookup(api, geoStore, ch)
	panelOUI(api, ouiStore)
	panelAlerts(api, db)
	panelShareLinks(api, db)
	panelAnalysis(api, db, ch)
	RegisterAdminRoutes(api, db)

	// Health
	app.Get("/healthz", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })
}

// BuildHTTPMux creates a net/http.ServeMux that routes:
//   - /ws/*  paths → native net/http WebSocket handlers (supports http.Hijacker)
//   - everything else → Fiber app via adaptor.FiberApp
//
// This is necessary because Fiber uses fasthttp under the hood, and fasthttp's
// response writer does not implement http.Hijacker, which gorilla/websocket
// (and therefore neffos) requires for WebSocket upgrades.
func BuildHTTPMux(app *fiber.App, db *gorm.DB, ch *sql.DB) http.Handler {
	mux := http.NewServeMux()

	// --- WebSocket routes (served by net/http for Hijacker support) ---
	agentNeffos, panelNeffos := setupNeffosServers(db, ch)
	rawPanelHandler := setupRawPanelWS(db)
	rawShareHandler := setupRawShareWS(db)

	mux.Handle("/ws/agent", agentNeffos)
	mux.Handle("/ws/panel/raw", rawPanelHandler)
	mux.Handle("/ws/panel", panelNeffos)
	mux.Handle("/ws/share/raw", rawShareHandler)
	mux.Handle("/ws", agentNeffos) // legacy backwards compat

	log.Info("WebSocket routes registered on net/http mux")

	// --- Everything else → Fiber ---
	mux.Handle("/", adaptor.FiberApp(app))

	return mux
}
