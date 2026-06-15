// web/reports_api.go
package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/reports"
)

func panelReports(api fiber.Router, pg *gorm.DB, ch *sql.DB, emailStore *email.QueueStore, scheduler *reports.Scheduler) {
	reportStore := reports.NewStore(pg)
	generator := reports.NewGenerator(pg, ch)

	api.Get("/workspaces/:id/reports", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")
		configs, err := reportStore.GetByWorkspace(c.UserContext(), wID)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		details := make([]reports.ReportConfigDetails, len(configs))
		for i, cfg := range configs {
			details[i] = toReportDetails(cfg)
		}

		return c.JSON(fiber.Map{
			"reports": details,
			"count":   len(details),
		})
	})

	api.Post("/workspaces/:id/reports", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")

		var req CreateReportRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		cfg := &reports.ReportConfig{
			WorkspaceID:     wID,
			Name:            req.Name,
			Description:     req.Description,
			ReportType:      reports.ReportType(req.ReportType),
			Schedule:        req.Schedule,
			EmailEnabled:    req.EmailEnabled,
			EmailRecipients: reports.SerializeEmailRecipients(req.EmailRecipients),
		}

		if err := reportStore.Create(c.UserContext(), cfg); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		if scheduler != nil && cfg.Schedule != "" {
			scheduler.RescheduleJob(*cfg)
		}

		return c.Status(http.StatusCreated).JSON(toReportDetails(*cfg))
	})

	api.Get("/workspaces/:id/reports/:reportId", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")
		reportID := uintParam(c, "reportId")

		cfg, err := reportStore.GetByID(c.UserContext(), reportID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		if cfg.WorkspaceID != wID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}

		return c.JSON(toReportDetails(*cfg))
	})

	api.Put("/workspaces/:id/reports/:reportId", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")
		reportID := uintParam(c, "reportId")

		cfg, err := reportStore.GetByID(c.UserContext(), reportID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		if cfg.WorkspaceID != wID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}

		var req UpdateReportRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
		}

		oldSchedule := cfg.Schedule
		if req.Name != nil && *req.Name != "" {
			cfg.Name = *req.Name
		}
		if req.Description != nil && *req.Description != "" {
			cfg.Description = *req.Description
		}
		if req.ReportType != nil && *req.ReportType != "" {
			cfg.ReportType = reports.ReportType(*req.ReportType)
		}
		if req.Schedule != nil {
			cfg.Schedule = *req.Schedule
		}
		if req.EmailEnabled != nil {
			cfg.EmailEnabled = *req.EmailEnabled
		}
		if req.EmailRecipients != nil {
			cfg.EmailRecipients = reports.SerializeEmailRecipients(req.EmailRecipients)
		}

		if err := reportStore.Update(c.UserContext(), cfg); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		if scheduler != nil && oldSchedule != cfg.Schedule {
			scheduler.RescheduleJob(*cfg)
		}

		return c.JSON(toReportDetails(*cfg))
	})

	api.Delete("/workspaces/:id/reports/:reportId", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")
		reportID := uintParam(c, "reportId")

		cfg, err := reportStore.GetByID(c.UserContext(), reportID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		if cfg.WorkspaceID != wID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}

		if scheduler != nil {
			scheduler.RemoveJob(reportID)
		}

		if err := reportStore.Delete(c.UserContext(), reportID); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		return c.SendStatus(http.StatusNoContent)
	})

	api.Get("/workspaces/:id/reports/:reportId/run", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")
		reportID := uintParam(c, "reportId")

		cfg, err := reportStore.GetByID(c.UserContext(), reportID)
		if err != nil {
			return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "report not found"})
		}
		if cfg.WorkspaceID != wID {
			return c.Status(http.StatusForbidden).JSON(fiber.Map{"error": "forbidden"})
		}

		var configJSON reports.ReportConfigJSON
		if cfg.Description != "" {
			json.Unmarshal([]byte(cfg.Description), &configJSON)
		}
		if configJSON.TimeRangeDays == 0 {
			configJSON.TimeRangeDays = 7
		}

		pdfData, err := generator.GenerateWorkspacePDF(c.UserContext(), cfg, configJSON)
		if err != nil {
			reportStore.UpdateLastRun(c.UserContext(), reportID, err.Error())
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		reportStore.UpdateLastRun(c.UserContext(), reportID, "")

		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", "attachment; filename="+cfg.Name+".pdf")
		return c.Send(pdfData)
	})

	api.Get("/workspaces/:id/reports/preview", func(c *fiber.Ctx) error {
		wID := uintParam(c, "id")

		timeRangeDays := int64(7)
		if tr := c.Query("time_range_days"); tr != "" {
			fmt.Sscanf(tr, "%d", &timeRangeDays)
		}

		configJSON := reports.ReportConfigJSON{
			TimeRangeDays: timeRangeDays,
			IncludeAlerts: true,
		}

		cfg := &reports.ReportConfig{
			WorkspaceID: wID,
			Name:        "Preview",
			ReportType:  reports.ReportTypeWorkspaceSummary,
		}

		pdfData, err := generator.GenerateWorkspacePDF(c.UserContext(), cfg, configJSON)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", "inline; filename=report-preview.pdf")
		return c.Send(pdfData)
	})
}

func agentReports(api fiber.Router, pg *gorm.DB, ch *sql.DB) {
	generator := reports.NewGenerator(pg, ch)

	api.Get("/agents/:id/reports/agent_detail/run", func(c *fiber.Ctx) error {		agentID := uintParam(c, "id")

		var days int64 = 7
		var from, to *time.Time

		if c.Query("from") != "" && c.Query("to") != "" {
			fromStr := c.Query("from")
			toStr := c.Query("to")
			fromTime, err1 := time.Parse(time.RFC3339, fromStr)
			toTime, err2 := time.Parse(time.RFC3339, toStr)
			if err1 == nil && err2 == nil {
				from = &fromTime
				to = &toTime
				days = 0
			}
		} else if tr := c.Query("time_range_days"); tr != "" {
			fmt.Sscanf(tr, "%d", &days)
		}

		// `sections` is a CSV of optional report sections (summary,
		// timeline, aggregate, probes, issues, correlation, appendix,
		// raw). The empty / missing value yields the default preset;
		// "all" turns on everything. See reports.ParseAgentReportSections.
		sectionsCSV := c.Query("sections")
		opts := reports.ParseAgentReportSections(sectionsCSV)

		var pdfData []byte
		var err error
		if from != nil && to != nil {
			pdfData, err = generator.GenerateAgentPDFWithOptions(c.UserContext(), agentID, 0, *from, *to, opts)
		} else {
			if days <= 0 {
				days = 7
			}
			pdfData, err = generator.GenerateAgentPDFWithOptions(c.UserContext(), agentID, days, time.Time{}, time.Time{}, opts)
		}
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=agent-%d-voice-quality.pdf", agentID))
		return c.Send(pdfData)
	})
}

// workspaceVoiceReport wires the workspace-wide voice report PDF
// endpoint. The endpoint accepts either a time range in days
// (`time_range_days=`) or a custom range (`from=` + `to=` in
// RFC3339). It is the workspace-level analog of the per-agent
// agent_detail/run endpoint, useful for daily/weekly digests
// delivered via the existing scheduled-report infrastructure.
func workspaceVoiceReport(api fiber.Router, pg *gorm.DB, ch *sql.DB) {
	generator := reports.NewGenerator(pg, ch)

	api.Get("/workspaces/:id/reports/voice/run", func(c *fiber.Ctx) error {
		wsID := uintParam(c, "id")

		var days int64 = 7
		var from, to *time.Time

		if c.Query("from") != "" && c.Query("to") != "" {
			fromStr := c.Query("from")
			toStr := c.Query("to")
			fromTime, err1 := time.Parse(time.RFC3339, fromStr)
			toTime, err2 := time.Parse(time.RFC3339, toStr)
			if err1 == nil && err2 == nil {
				from = &fromTime
				to = &toTime
				days = 0
			}
		} else if tr := c.Query("time_range_days"); tr != "" {
			fmt.Sscanf(tr, "%d", &days)
		}

		var pdfData []byte
		var err error
		if from != nil && to != nil {
			// For now the workspace voice report uses the same
			// windowed-from-days path. Custom-range support can
			// extend the generator; the API shape stays stable.
			_ = from
			_ = to
		}
		if days <= 0 {
			days = 7
		}
		pdfData, err = generator.GenerateWorkspaceVoicePDF(c.UserContext(), wsID, days)
		if err != nil {
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=workspace-%d-voice-quality.pdf", wsID))
		return c.Send(pdfData)
	})
}

type CreateReportRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	ReportType      string   `json:"report_type"`
	Schedule        string   `json:"schedule,omitempty"`
	EmailEnabled    bool     `json:"email_enabled"`
	EmailRecipients []string `json:"email_recipients"`
}

type UpdateReportRequest struct {
	Name            *string  `json:"name,omitempty"`
	Description     *string  `json:"description,omitempty"`
	ReportType      *string  `json:"report_type,omitempty"`
	Schedule        *string  `json:"schedule,omitempty"`
	EmailEnabled    *bool    `json:"email_enabled,omitempty"`
	EmailRecipients []string `json:"email_recipients,omitempty"`
}

func toReportDetails(cfg reports.ReportConfig) reports.ReportConfigDetails {
	var configJSON reports.ReportConfigJSON
	if cfg.Description != "" {
		json.Unmarshal([]byte(cfg.Description), &configJSON)
	}

	return reports.ReportConfigDetails{
		ID:              cfg.ID,
		WorkspaceID:     cfg.WorkspaceID,
		Name:            cfg.Name,
		Description:     cfg.Description,
		ReportType:      cfg.ReportType,
		Schedule:        cfg.Schedule,
		EmailEnabled:    cfg.EmailEnabled,
		EmailRecipients: reports.ParseEmailRecipients(cfg.EmailRecipients),
		LastRunAt:       cfg.LastRunAt,
		LastError:       cfg.LastError,
		Config:          configJSON,
		CreatedAt:       cfg.CreatedAt,
		UpdatedAt:       cfg.UpdatedAt,
	}
}
