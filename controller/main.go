package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"github.com/kataras/iris/v12"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"netwatcher-controller/internal/database"
	"netwatcher-controller/internal/workspace"
	"netwatcher-controller/web"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	// modules for wiring
	agentpkg "netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/auth"
	probepkg "netwatcher-controller/internal/probe"
	userspkg "netwatcher-controller/internal/users"
)

func main() {
	runtime.GOMAXPROCS(4)
	log.SetFormatter(&log.TextFormatter{FullTimestamp: true})

	_ = godotenv.Load()

	// ---- DB (agnostic) ----
	db, err := database.OpenFromEnv()
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	// ---- Automigrate + indexes ----
	if err := db.AutoMigrate(
		&userspkg.User{},
		&agentpkg.Agent{},
		&probepkg.Probe{},
		&probepkg.Target{},
		&auth.Session{},
	); err != nil {
		log.Fatalf("automigrate failed: %v", err)
	}
	if err := database.RunMigrations(db); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	// ---- Build services ----
	authSvc, agentsRepo, probesSvc, workspaceSvc := buildServices(db)

	// ---- Router ----
	r := web.NewRouter(db, authSvc, agentsRepo, probesSvc, workspaceSvc)
	// r.ProbeDataChan = make(chan agentpkg.ProbeData, 1024)

	// Example CORS (unchanged)
	crs := func(ctx iris.Context) {
		ctx.Header("Access-Control-Allow-Origin", "*")
		ctx.Header("Access-Control-Allow-Credentials", "true")
		if ctx.Method() == iris.MethodOptions {
			ctx.Header("Access-Control-Methods", "POST, PUT, PATCH, DELETE, OPTIONS")
			ctx.Header("Access-Control-Allow-Headers", "Access-Control-Allow-Origin,Content-Type,*")
			ctx.Header("Access-Control-Max-Age", "86400")
			ctx.StatusCode(iris.StatusNoContent)
			return
		}
		ctx.Next()
	}
	r.App.UseRouter(crs)

	// Workers that need DB/router
	// workers.CreateProbeDataWorker(r.ProbeDataChan, r.DB)

	handleSignals()

	// Routes + serve
	r.Init()
	r.Listen(os.Getenv("LISTEN"))
}

// ---- wiring helpers ----

func buildServices(db *gorm.DB) (auth.Service, agentpkg.Repository, probepkg.Service, workspace.Service) {
	// Users repo/svc
	usersRepo := userspkg.NewRepository(db)
	usersSvc := userspkg.NewService(usersRepo)

	// Agents repo (and optionally a service if you added one)
	agentsRepo := agentpkg.NewRepository(db)

	// Probes repo/svc
	probeRepo := probepkg.NewRepository(db)
	probesSvc := probepkg.NewService(probeRepo, agentsRepo)

	// Auth service
	authSvc := auth.NewService(db, usersRepo, usersSvc, agentsRepo)

	workspaceSvc := workspace.NewService(workspace.NewRepository(db), usersRepo)

	return authSvc, agentsRepo, probesSvc, workspaceSvc
}

func handleSignals() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL)
	go func() {
		for range signals {
			shutdown()
		}
	}()
}

func shutdown() {
	fmt.Println()
	log.Warnf("%d goroutines at exit.", runtime.NumGoroutine())
	log.Warn("Shutting down NetWatcher Controller...")
	time.Sleep(100 * time.Millisecond)
	os.Exit(0)
}
