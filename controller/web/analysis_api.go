// web/analysis_api.go
// REST API endpoints for AI health analysis
package web

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

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

	// ------------------------------------------
	// GET /workspaces/{id}/analysis/history
	// Historical analysis snapshots for trend analysis
	// Query: from=<RFC3339>, to=<RFC3339>, limit=<int, default 288>
	// ------------------------------------------
	api.Get("/workspaces/{id:uint}/analysis/history", func(ctx iris.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[analysis] history PANIC: %v", r)
				ctx.StatusCode(http.StatusInternalServerError)
				_ = ctx.JSON(iris.Map{"error": "internal error"})
			}
		}()

		wID := uintParam(ctx, "id")
		limit := intOrDefault(ctx.URLParam("limit"), 288)

		var from, to time.Time
		if v := ctx.URLParam("from"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				from = t
			}
		}
		if v := ctx.URLParam("to"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				to = t
			}
		}

		// Default to last 24 hours if no from specified
		if from.IsZero() {
			from = time.Now().UTC().Add(-24 * time.Hour)
		}

		snapshots, err := probe.GetAnalysisSnapshots(ctx.Request().Context(), ch, wID, from, to, limit)
		if err != nil {
			log.Printf("[analysis] history workspace=%d error: %v", wID, err)
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": err.Error()})
			return
		}

		jsonBytes, err := json.Marshal(iris.Map{
			"workspace_id": wID,
			"snapshots":    snapshots,
			"count":        len(snapshots),
		})
		if err != nil {
			ctx.StatusCode(http.StatusInternalServerError)
			_ = ctx.JSON(iris.Map{"error": "json serialization failed"})
			return
		}

		ctx.ContentType("application/json")
		_, _ = ctx.Write(jsonBytes)
	})
}
