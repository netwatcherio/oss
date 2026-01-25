// web/router.go
package web

import (
	"database/sql"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/limits"
	"netwatcher-controller/internal/oui"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RegisterRoutes mounts all Iris routes using your internal/* packages.
func RegisterRoutes(app *iris.Application, db *gorm.DB, ch *sql.DB, emailStore *email.QueueStore, geoStore *geoip.Store, ouiStore *oui.Store) {
	// Load limits configuration from environment
	limitsConfig := limits.LoadFromEnv()

	// ----- Public (no auth) -----
	registerAuthRoutes(app, db, emailStore) // /auth/*
	agentAuth(app, db)                      // /agent

	// Invite routes (public, no auth required)
	RegisterInviteRoutes(app, db, emailStore)

	// Agent API routes (PSK auth via headers)
	RegisterAgentAPI(app, db, ch, geoStore)

	err := addWebSocketServer(app, db, ch)
	if err != nil {
		log.Error(err)
	}

	// Raw WebSocket for panel (simpler than neffos, better browser compatibility)
	RegisterRawPanelWS(app, db)

	// Raw WebSocket for share links (share-token authenticated, no JWT required)
	RegisterRawShareWS(app, db)

	// ----- Protected (JWT) -----
	api := app.Party("/")
	api.Use(JWTMiddleware(db))

	panelWorkspaces(api, db, emailStore, limitsConfig) // /workspaces/*
	panelProbes(api, db, limitsConfig)                 // /workspaces/{id}/agents/{agentID}/probes/*
	panelAgents(api, db, ch, limitsConfig)
	panelProbeData(api, db, ch)
	panelSpeedtest(api, db, ch)    // /workspaces/{id}/agents/{agentID}/speedtest-*
	panelGeoIP(api, geoStore, ch)  // /geoip/*
	panelWhois(api, ch)            // /whois/*
	panelLookup(api, geoStore, ch) // /lookup/*
	panelOUI(api, ouiStore)        // /lookup/oui/*
	panelAlerts(api, db)           // /alerts/* and /workspaces/{id}/alert-rules/*
	panelShareLinks(api, db)       // /workspaces/{id}/agents/{agentID}/share-links/*

	// Admin panel routes (requires SITE_ADMIN role)
	RegisterAdminRoutes(api, db)

	// Public share access routes (no auth)
	RegisterShareRoutes(app, db, ch)

	// Health
	app.Get("/healthz", func(ctx iris.Context) { _ = ctx.JSON(iris.Map{"ok": true}) })
}
