package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/email"
)

type Scheduler struct {
	db         *gorm.DB
	ch         *sql.DB
	store      *Store
	generator  *Generator
	emailStore *email.QueueStore
	cron       *cron.Cron
	jobEntries map[uint]cron.EntryID
}

func NewScheduler(db *gorm.DB, ch *sql.DB, store *Store, generator *Generator, emailStore *email.QueueStore) *Scheduler {
	return &Scheduler{
		db:         db,
		ch:         ch,
		store:      store,
		generator:  generator,
		emailStore: emailStore,
		cron:       cron.New(),
		jobEntries: make(map[uint]cron.EntryID),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if err := s.store.AutoMigrate(ctx); err != nil {
		return fmt.Errorf("failed to migrate report configs: %w", err)
	}

	configs, err := s.store.GetScheduled(ctx)
	if err != nil {
		return fmt.Errorf("failed to load scheduled reports: %w", err)
	}

	for _, cfg := range configs {
		if cfg.Schedule == "" {
			continue
		}
		s.addJob(cfg)
	}

	s.cron.Start()
	log.Infof("[reports] scheduler started with %d scheduled reports", len(configs))
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Info("[reports] scheduler stopped")
}

func (s *Scheduler) addJob(cfg ReportConfig) {
	entryID, err := s.cron.AddFunc(cfg.Schedule, func() {
		s.runReport(cfg.ID)
	})
	if err != nil {
		log.Errorf("[reports] failed to add cron job for report %d: %v", cfg.ID, err)
	} else {
		s.jobEntries[cfg.ID] = entryID
		log.Infof("[reports] scheduled report %d with cron: %s (entry %d)", cfg.ID, cfg.Schedule, entryID)
	}
}

func (s *Scheduler) RemoveJob(reportID uint) {
	if entryID, ok := s.jobEntries[reportID]; ok {
		s.cron.Remove(entryID)
		delete(s.jobEntries, reportID)
		log.Infof("[reports] removed cron job for report %d", reportID)
	}
}

func (s *Scheduler) RescheduleJob(cfg ReportConfig) {
	s.RemoveJob(cfg.ID)
	if cfg.Schedule != "" {
		s.addJob(cfg)
	}
}

func (s *Scheduler) runReport(reportID uint) {
	ctx := context.Background()

	cfg, err := s.store.GetByID(ctx, reportID)
	if err != nil {
		log.Errorf("[reports] failed to load report %d: %v", reportID, err)
		return
	}

	var configJSON ReportConfigJSON
	if cfg.Description != "" {
		json.Unmarshal([]byte(cfg.Description), &configJSON)
	}
	if configJSON.TimeRangeDays == 0 {
		configJSON.TimeRangeDays = 7
	}

	pdfData, err := s.generator.GenerateWorkspacePDF(ctx, cfg, configJSON)
	if err != nil {
		log.Errorf("[reports] failed to generate PDF for report %d: %v", reportID, err)
		s.store.UpdateLastRun(ctx, reportID, err.Error())
		return
	}

	if cfg.EmailEnabled && len(cfg.EmailRecipients) > 0 {
		recipients := ParseEmailRecipients(cfg.EmailRecipients)
		if len(recipients) > 0 {
			if err := s.sendReportEmail(ctx, cfg, recipients, pdfData); err != nil {
				log.Errorf("[reports] failed to send report email for %d: %v", reportID, err)
				s.store.UpdateLastRun(ctx, reportID, err.Error())
				return
			}
		}
	}

	s.store.UpdateLastRun(ctx, reportID, "")
	log.Infof("[reports] successfully ran report %d", reportID)
}

func (s *Scheduler) sendReportEmail(ctx context.Context, cfg *ReportConfig, recipients []string, pdfData []byte) error {
	subject := fmt.Sprintf("NetWatcher Report: %s", cfg.Name)
	panelLink := getPanelEndpoint()
	bodyHTML := fmt.Sprintf(`<html><body style="font-family: Arial, sans-serif; color: #333;">
<h2 style="color: #1a365d;">NetWatcher Report</h2>
<p>Your scheduled report <strong>%s</strong> is attached.</p>
<p>Report Type: %s<br>Generated: %s</p>
<p><a href="%s">View in NetWatcher</a></p>
<hr style="border: none; border-top: 1px solid #eee;">
<p style="color: #666; font-size: 12px;">NetWatcher - Network Monitoring Platform</p>
</body></html>`,
		cfg.Name, cfg.ReportType, time.Now().Format("Jan 2, 2006 15:04 UTC"), panelLink)

	attachmentName := fmt.Sprintf("%s.pdf", cfg.Name)

	var lastErr error
	for _, recipient := range recipients {
		recipient = trimEmail(recipient)
		if recipient == "" {
			continue
		}

		emailQueue := &email.EmailQueue{
			Type:              email.TypeReport,
			ToEmail:           recipient,
			Subject:           subject,
			Body:              fmt.Sprintf("NetWatcher Report: %s is attached.", cfg.Name),
			BodyHTML:          bodyHTML,
			AttachmentName:    attachmentName,
			AttachmentContent: pdfData,
		}

		if err := s.emailStore.Enqueue(ctx, emailQueue); err != nil {
			log.Errorf("[reports] failed to enqueue email for %s: %v", recipient, err)
			lastErr = err
			continue
		}
		log.Infof("[reports] enqueued report email for %s", recipient)
	}

	if lastErr != nil && len(recipients) == 0 {
		return fmt.Errorf("failed to enqueue any email: %w", lastErr)
	}

	return nil
}

func trimEmail(email string) string {
	email = strings.TrimSpace(email)
	email = strings.Trim(email, "<>")
	return strings.TrimSpace(email)
}

func getPanelEndpoint() string {
	if ep := getEnv("PANEL_ENDPOINT", ""); ep != "" {
		return ep
	}
	if ep := getEnv("PANEL_URL", ""); ep != "" {
		return ep
	}
	if ep := getEnv("APP_DOMAIN", ""); ep != "" {
		return ep
	}
	return "http://localhost:3000"
}

func getEnv(key, def string) string {
	if v := getEnvValue(key); v != "" {
		return v
	}
	return def
}

func getEnvValue(key string) string {
	return os.Getenv(key)
}
