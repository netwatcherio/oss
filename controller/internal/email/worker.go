// internal/email/worker.go
package email

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultBatchSize    = 10
)

// DeliveryMethod specifies how emails are sent
type DeliveryMethod string

const (
	DeliverySMTP    DeliveryMethod = "smtp"
	DeliveryWebhook DeliveryMethod = "webhook"
	DeliveryNone    DeliveryMethod = "none" // Queue only, no delivery
)

// Worker processes the email queue in the background
type Worker struct {
	store         *QueueStore
	smtpSender    *Sender
	webhookSender *WebhookSender
	delivery      DeliveryMethod

	pollInterval time.Duration
	batchSize    int

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewWorker creates a new email worker
func NewWorker(db *gorm.DB, smtpConfig *SMTPConfig) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

	// Determine delivery method
	webhookConfig := LoadWebhookConfigFromEnv()
	delivery := DeliveryNone

	if webhookConfig.Enabled {
		delivery = DeliveryWebhook
	} else if smtpConfig.IsConfigured() {
		delivery = DeliverySMTP
	}

	return &Worker{
		store:         NewQueueStore(db),
		smtpSender:    NewSender(smtpConfig),
		webhookSender: NewWebhookSender(webhookConfig),
		delivery:      delivery,
		pollInterval:  defaultPollInterval,
		batchSize:     defaultBatchSize,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins processing the email queue
func (w *Worker) Start() error {
	// Migrate table
	if err := w.store.AutoMigrate(context.Background()); err != nil {
		return err
	}

	switch w.delivery {
	case DeliveryWebhook:
		log.WithField("url", w.webhookSender.config.URL).Info("Email worker started (webhook mode)")
	case DeliverySMTP:
		log.WithFields(log.Fields{
			"host": w.smtpSender.config.Host,
			"port": w.smtpSender.config.Port,
			"from": w.smtpSender.config.FromEmail,
		}).Info("Email worker started (SMTP mode)")
	default:
		log.Warn("Email worker started (queue-only mode - no delivery configured)")
	}

	w.wg.Add(1)
	go w.run()

	return nil
}

// Stop gracefully stops the worker
func (w *Worker) Stop() {
	w.cancel()
	w.wg.Wait()
	log.Info("Email worker stopped")
}

// run is the main worker loop
func (w *Worker) run() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.processBatch()
		}
	}
}

// processBatch processes a batch of pending emails
func (w *Worker) processBatch() {
	if w.delivery == DeliveryNone {
		return
	}

	emails, err := w.store.GetPendingEmails(w.ctx, w.batchSize)
	if err != nil {
		log.WithError(err).Error("Failed to get pending emails")
		return
	}

	for _, email := range emails {
		select {
		case <-w.ctx.Done():
			return
		default:
			w.processEmail(email)
		}
	}
}

// processEmail sends a single email
func (w *Worker) processEmail(email EmailQueue) {
	logger := log.WithFields(log.Fields{
		"email_id": email.ID,
		"type":     email.Type,
		"to":       email.ToEmail,
		"method":   w.delivery,
	})

	// Mark as processing
	if err := w.store.MarkProcessing(w.ctx, email.ID); err != nil {
		logger.WithError(err).Error("Failed to mark email as processing")
		return
	}

	// Build template vars for webhook
	vars := TemplateVars{
		ToEmail:       email.ToEmail,
		ToName:        email.ToName,
		PanelEndpoint: GetPanelEndpoint(),
	}

	// Send via appropriate method
	var sendErr error
	switch w.delivery {
	case DeliveryWebhook:
		sendErr = w.webhookSender.Send(&email, vars)
	case DeliverySMTP:
		sendErr = w.smtpSender.Send(&email)
	}

	if sendErr != nil {
		logger.WithError(sendErr).Error("Failed to send email")
		_ = w.store.MarkFailed(w.ctx, email.ID, sendErr)
		return
	}

	// Mark as sent
	if err := w.store.MarkSent(w.ctx, email.ID); err != nil {
		logger.WithError(err).Error("Failed to mark email as sent")
		return
	}

	logger.Info("Email sent successfully")
}

// GetStore returns the queue store for direct access
func (w *Worker) GetStore() *QueueStore {
	return w.store
}

// GetDeliveryMethod returns the current delivery method
func (w *Worker) GetDeliveryMethod() DeliveryMethod {
	return w.delivery
}
