// web/analysis_api.go
// REST API endpoints for AI health analysis
package web

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/kataras/iris/v12"
	"gorm.io/gorm"

	"netwatcher-controller/internal/probe"
)

func panelAnalysis(api iris.Party, pg *gorm.DB, ch *sql.DB) {
	// ------------------------------------------
	// GET /workspaces/{id}/analysis
	// Workspace health overview with per-agent health vectors
	// Query: lookback=<minutes, default 60>
	// ------------------------------------------
	api.Get("/workspaces/{id:uint}/analysis", func(ctx iris.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] PANIC: %v", r)
				ctx.StatusCode(http.StatusInternalServerError)
				_ = ctx.JSON(iris.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(ctx, "id")
		lookback := intOrDefault(ctx.URLParam("lookback"), 60)

		analysis, err := probe.ComputeWorkspaceAnalysis(ctx.Request().Context(), ch, pg, wID, lookback)
		if err != nil {
			log.Printf("[analysis] workspace=%d error: %v", wID, err)
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		jsonBytes, err := json.Marshal(analysis)
		if err != nil {
			log.Printf("[analysis] JSON marshal error: %v", err)
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "json serialization failed"})
			return
		}

		ctx.ContentType("application/json")
		_, _ = ctx.Write(jsonBytes)
	})

	// ------------------------------------------
	// GET /workspaces/{id}/analysis/probes/{probeId}
	// Detailed probe analysis with bidirectional data
	// Query: lookback=<minutes, default 60>
	// ------------------------------------------
	api.Get("/workspaces/{id:uint}/analysis/probes/{probeId:uint}", func(ctx iris.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] probe PANIC: %v", r)
				ctx.StatusCode(http.StatusInternalServerError)
				_ = ctx.JSON(iris.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(ctx, "id")
		probeID := uintParam(ctx, "probeId")
		lookback := intOrDefault(ctx.URLParam("lookback"), 60)

		analysis, err := probe.ComputeProbeAnalysis(ctx.Request().Context(), ch, pg, wID, probeID, lookback)
		if err != nil {
			log.Printf("[analysis] workspace=%d probe=%d error: %v", wID, probeID, err)
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		jsonBytes, err := json.Marshal(analysis)
		if err != nil {
			log.Printf("[analysis] probe JSON marshal error: %v", err)
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "json serialization failed"})
			return
		}

		ctx.ContentType("application/json")
		_, _ = ctx.Write(jsonBytes)
	})
}
