// cmd/controller/main.go
package main

import (
	"context"
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
	"netwatcher-controller/internal/oui"
	"netwatcher-controller/internal/probe"
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

	// Start the ClickHouse batch writer (reduces data-part fragmentation)
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

	// Update ClickHouse TTL on startup (in case config changed)
	go scheduler.EnsureClickHouseTTL(context.Background(), ch, retentionConfig.DataRetentionDays)

	// ---- Alert Scheduler (offline checks) ----
	alertConfig := scheduler.LoadAlertSchedulerConfig()
	alertScheduler := scheduler.NewAlertScheduler(db, alertConfig)
	go alertScheduler.Start(cleanupCtx) // reuse same context for shutdown

	// ---- AI Analysis Loop (background incident detection + alerting) ----
	analysisConfig := probe.LoadAnalysisLoopConfig()
	go probe.StartAnalysisLoop(cleanupCtx, ch, db, analysisConfig)

	// ---- Optional LLM Enrichment ----
	llmConfig := llm.LoadConfig()
	if llmP := llm.NewProvider(llmConfig); llmP != nil {
		probe.SetLLMProvider(llmP)
	}

	// ---- Fiber ----
	app := fiber.New(fiber.Config{
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		BodyLimit:    10 * 1024 * 1024, // 10 MB
	})

	// CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders:     "Authorization,Content-Type,X-Requested-With,Accept",
		AllowCredentials: true,
		MaxAge:           86400,
	}))

	probe.InitWorkers(ch, db)

	// Routes (public + protected)
	web.RegisterRoutes(app, db, ch, emailWorker.GetStore(), geoStore, ouiStore)

	// Health (also registered in router.go, but kept here for main visibility)
	// app.Get("/healthz", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"ok": true}) })

	// ---- Graceful Shutdown ----
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("Shutting down...")
		cleanupCancel()
		probe.StopBatchWriter() // flush remaining CH records
		emailWorker.Stop()
		if geoStore != nil {
			geoStore.Close()
		}
		_ = app.Shutdown()
	}()

	// ---- Listen ----
	listen := getenv("LISTEN", "0.0.0.0:8080")
	log.Infof("HTTP listening on %s", listen)
	if err := app.Listen(listen); err != nil {
		log.WithError(err).Fatal("server exited")
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
