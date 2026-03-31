// web/reports_api.go
package web

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/reports"
)

func panelReports(api fiber.Router, pg *gorm.DB, ch *sql.DB, emailStore *email.QueueStore) {
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
			return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}

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
