// web/router.go
package web

import (
	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// RegisterRoutes mounts all Iris routes using your internal/* packages.
func RegisterRoutes(app *iris.Application, db *gorm.DB) {
	// ----- Public (no auth) -----
	registerAuthRoutes(app, db) // /auth/*
	agentAuth(app, db)          // /agent

	err := addWebSocketServer(app, db)
	if err != nil {
		log.Error(err)
	}

	// ----- Protected (JWT) -----
	api := app.Party("/")
	api.Use(JWTMiddleware(db))

	panelWorkspaces(api, db) // /workspaces/*
	panelProbes(api, db)     // /workspaces/{id}/agents/{agentID}/probes/*
	panelAgents(api, db)

	// Health
	app.Get("/healthz", func(ctx iris.Context) { _ = ctx.JSON(iris.Map{"ok": true}) })
}
