// cmd/controller/main.go
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"

	"netwatcher-controller/internal/database"
	"netwatcher-controller/internal/email"
	"netwatcher-controller/internal/geoip"
	"netwatcher-controller/internal/probe"
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

	ch, err := probe.OpenClickHouseFromEnv()
	if err != nil {
		log.WithError(err).Fatal("clickhouse open failed")
	}
	if err := probe.MigrateCH(context.Background(), ch); err != nil {
		log.WithError(err).Fatal("clickhouse migrate failed")
	}

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

	// ---- Iris ----
	app := iris.New()
	app.Configure(iris.WithConfiguration(iris.Configuration{
		DisableStartupLog: false,
		Charset:           "UTF-8",
		TimeFormat:        time.RFC3339,
	}))

	crs := func(ctx iris.Context) {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Credentials", "true")

		if ctx.Method() == iris.MethodOptions {
			ctx.Header("Access-Control-Allow-Methods",
				"GET, POST, PUT, PATCH, DELETE, OPTIONS")
			ctx.Header("Access-Control-Allow-Headers",
				"Authorization, Content-Type, X-Requested-With, Accept")
			ctx.Header("Access-Control-Max-Age", "86400")
			ctx.StatusCode(iris.StatusNoContent)
			return
		}

		ctx.Next()
	}
	app.UseRouter(crs)

	probe.InitWorkers(ch)

	// Routes (public + protected)
	web.RegisterRoutes(app, db, ch, emailWorker.GetStore(), geoStore)

	// Health
	app.Get("/healthz", func(ctx iris.Context) { _ = ctx.JSON(iris.Map{"ok": true}) })

	// ---- Graceful Shutdown ----
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("Shutting down...")
		emailWorker.Stop()
		if geoStore != nil {
			geoStore.Close()
		}
		_ = app.Shutdown(context.Background())
	}()

	// ---- Listen ----
	listen := getenv("LISTEN", "0.0.0.0:8080")
	log.Infof("HTTP listening on %s", listen)
	if err := app.Listen(listen,
		iris.WithoutServerError(iris.ErrServerClosed),
		iris.WithOptimizations,
	); err != nil {
		log.WithError(err).Fatal("server exited")
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
