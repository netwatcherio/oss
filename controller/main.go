// cmd/controller/main.go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"

	"netwatcher-controller/internal/admin"
	"netwatcher-controller/internal/database"
	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/llm"
	"netwatcher-controller/internal/logloki"
	"netwatcher-controller/internal/metrics"
	"netwatcher-controller/internal/oui"
	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/reports"
	"netwatcher-controller/internal/scheduler"
	"netwatcher-controller/web"
)

func main() {
	_ = godotenv.Load()

	// ---- DB ----
	db, err := database.OpenFromEnv()
	if err != nil {
		log.WithError(err).Fatal("db open failed")
	}

	err = database.CreateIndexes(db)
	if err != nil {
		log.WithError(err).Fatal("db index creation failed")
	}

	// ---- Admin Bootstrap ----
	adminCfg := admin.LoadConfigFromEnv()
	if err := admin.BootstrapDefaultAdmin(context.Background(), db, adminCfg); err != nil {
		log.WithError(err).Fatal("admin bootstrap failed")
	}

	ch, err := probe.OpenClickHouseFromEnv()
	if err != nil {
		log.WithError(err).Fatal("clickhouse open failed")
	}

	// ---- Data Retention Config ----
	retentionConfig := scheduler.LoadRetentionConfig()
	log.Infof("Data retention: %d days, soft-delete grace: %d days",
		retentionConfig.DataRetentionDays, retentionConfig.SoftDeleteGraceDays)

	if err := probe.MigrateCH(context.Background(), ch, retentionConfig.DataRetentionDays); err != nil {
		log.WithError(err).Fatal("clickhouse migrate failed")
	}
	if err := probe.MigrateCacheTablesCH(context.Background(), ch); err != nil {
		log.WithError(err).Fatal("clickhouse cache tables migrate failed")
	}

	probe.InitBatchWriter(ch)

	// ---- Email Worker ----
	smtpConfig := email.LoadSMTPConfigFromEnv()
	emailWorker := email.NewWorker(db, smtpConfig)
	if err := emailWorker.Start(); err != nil {
		log.WithError(err).Fatal("email worker start failed")
	}

	// ---- GeoIP ----
	var geoStore *geoip.Store
	geoConfig := geoip.LoadConfigFromEnv()
	if geoConfig.IsConfigured() {
		geoStore, err = geoip.NewStore(geoConfig)
		if err != nil {
			log.WithError(err).Warn("geoip init failed, lookups disabled")
		} else {
			log.Info("GeoIP databases loaded successfully")
		}
	} else {
		log.Info("GeoIP not configured, lookups disabled")
	}

	// ---- Prometheus Metrics ----
	m := metrics.New(db, ch)
	m.RegisterCollectors(db, ch)
	log.Info("Prometheus metrics initialized")

	// ---- Loki Log Shipping ----
	if lokiURL := os.Getenv("LOKI_URL"); lokiURL != "" {
		lokiHook := logloki.NewHook(lokiURL, "netwatcher-controller")
		if workspaceID := os.Getenv("LOKI_WORKSPACE_ID"); workspaceID != "" {
			lokiHook.SetLabel("workspace_id", workspaceID)
		}
		lokiHook.Start()
		log.AddHook(lokiHook)
		log.Infof("Loki log shipping enabled: %s", lokiURL)
	}

	// ---- OUI Store ----
	ouiConfig := oui.LoadConfigFromEnv()
	ouiStore := oui.NewStore(ouiConfig)
	if err := ouiStore.Load(); err != nil {
		log.WithError(err).Warn("OUI database load failed, vendor lookups disabled")
	}

	// ---- Cleanup Scheduler ----
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	cleanupScheduler := scheduler.NewCleanupScheduler(db, ch, retentionConfig)
	go cleanupScheduler.Start(cleanupCtx)

	go scheduler.EnsureClickHouseTTL(context.Background(), ch, retentionConfig.DataRetentionDays)

	// ---- Alert Scheduler ----
	alertConfig := scheduler.LoadAlertSchedulerConfig()
	alertScheduler := scheduler.NewAlertScheduler(db, alertConfig)
	go alertScheduler.Start(cleanupCtx)

	// ---- AI Analysis Loop ----
	analysisConfig := probe.LoadAnalysisLoopConfig()
	go probe.StartAnalysisLoop(cleanupCtx, ch, db, analysisConfig)

	// ---- Report Scheduler ----
	reportStore := reports.NewStore(db)
	reportGenerator := reports.NewGenerator(db, ch)
	reportScheduler := reports.NewScheduler(db, ch, reportStore, reportGenerator, emailWorker.GetStore())
	if err := reportScheduler.Start(cleanupCtx); err != nil {
		log.WithError(err).Warn("Report scheduler start failed")
	}

	// ---- Optional LLM Enrichment ----
	llmConfig := llm.LoadConfig()
	if llmP := llm.NewProvider(llmConfig); llmP != nil {
		probe.SetLLMProvider(llmP)
	}

	// ---- Fiber (REST routes only) ----
	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    10 * 1024 * 1024,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Authorization,Content-Type,X-Requested-With,Accept",
		MaxAge:       86400,
	}))

	probe.InitWorkers(ch, db)

	web.RegisterRoutes(app, db, ch, emailWorker.GetStore(), geoStore, ouiStore)

	// ---- Build unified HTTP mux ----
	// WebSocket routes are served by net/http (supports http.Hijacker).
	// All other routes go through Fiber via adaptor.FiberApp.
	mux := web.BuildHTTPMux(app, db, ch)

	listen := getenv("LISTEN", "0.0.0.0:8080")

	srv := &http.Server{
		Addr:         listen,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ---- Graceful Shutdown ----
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("Shutting down...")
		cleanupCancel()
		probe.StopBatchWriter()
		emailWorker.Stop()
		reportScheduler.Stop()
		if geoStore != nil {
			geoStore.Close()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	// ---- Listen ----
	log.Infof("HTTP listening on %s", listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.WithError(err).Fatal("server exited")
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
