// web/router.go
package web

import (
	"database/sql"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/geoip"

	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RegisterRoutes mounts all Iris routes using your internal/* packages.
func RegisterRoutes(app *iris.Application, db *gorm.DB, ch *sql.DB, emailStore *email.QueueStore, geoStore *geoip.Store) {
	// ----- Public (no auth) -----
	registerAuthRoutes(app, db, emailStore) // /auth/*
	agentAuth(app, db)                      // /agent

	// Invite routes (public, no auth required)
	RegisterInviteRoutes(app, db, emailStore)

	err := addWebSocketServer(app, db, ch)
	if err != nil {
		log.Error(err)
	}

	// Raw WebSocket for panel (simpler than neffos, better browser compatibility)
	RegisterRawPanelWS(app, db)

	// ----- Protected (JWT) -----
	api := app.Party("/")
	api.Use(JWTMiddleware(db))

	panelWorkspaces(api, db, emailStore) // /workspaces/*
	panelProbes(api, db)                 // /workspaces/{id}/agents/{agentID}/probes/*
	panelAgents(api, db, ch)
	panelProbeData(api, db, ch)
	panelSpeedtest(api, db, ch) // /workspaces/{id}/agents/{agentID}/speedtest-*
	panelGeoIP(api, geoStore)   // /geoip/*
	panelWhois(api)             // /whois/*

	// Health
	app.Get("/healthz", func(ctx iris.Context) { _ = ctx.JSON(iris.Map{"ok": true}) })
}
