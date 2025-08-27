package web

import (
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/websocket"
	"github.com/kataras/neffos"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/auth"
	"netwatcher-controller/internal/probe"
)

type Router struct {
	App             *iris.Application
	DB              *gorm.DB
	Routes          []*Route
	WebSocketServer *neffos.Server
	ProbeDataChan   chan agent.ProbeData

	// NEW: services/repos
	AuthSvc    auth.Service
	AgentsRepo agent.Repository
	ProbesSvc  probe.Service
}

func NewRouter(db *gorm.DB, authSvc auth.Service, agentsRepo agent.Repository, probesSvc probe.Service) *Router {
	return &Router{
		App:        iris.New(),
		DB:         db,
		AuthSvc:    authSvc,
		AgentsRepo: agentsRepo,
		ProbesSvc:  probesSvc,
	}
}

func (r *Router) Init() {
	if err := addWebSocketServer(r); err != nil {
		log.Error(err)
	}
	r.App.Get("/agent_ws", websocket.Handler(r.WebSocketServer))
	log.Info("Loading Agent Websocket Route...")

	// Attach proxy-aware IP middleware
	r.App.Use(ProxyIPMiddleware)

	// Register routes (now that services are available on Router)
	r.Routes = append(r.Routes, addRouteAuth(r)...)
	r.Routes = append(r.Routes, addRouteAgents(r)...)
	r.Routes = append(r.Routes, addRouteSites(r)...)
	r.Routes = append(r.Routes, addRouteAgentAPI(r)...)
	// r.Routes = append(r.Routes, addRouteProbes(r)...)

	log.Info("Loading all routes...")
	log.Infof("Found %d route(s).", len(r.Routes))
	if len(r.Routes) == 0 {
		log.Error("No routes found.")
		return
	}

	log.Info("Skipping routes that require JWT...")
	r.LoadRoutes(false)

	log.Info("Enabling JWT Middleware...")
	// Keep your existing middleware (it can validate via Authorization header),
	// or swap to something that calls r.AuthSvc.Parse/Validate as needed.
	r.App.Use(VerifySession())

	log.Info("Loading JWT routes...")
	r.LoadRoutes(true)
}

func (r *Router) LoadRoutes(JWT bool) {
	for _, v := range r.Routes {
		if !v.JWT && JWT {
			log.Warnf("JWT route... SKIP... %s - %s", v.Name, v.Path)
			continue
		}
		if v.JWT && !JWT {
			log.Warnf("not JWT route... SKIP... %s - %s", v.Name, v.Path)
			continue
		}

		log.Infof("Loaded route: %s (%s) - %s", v.Name, v.Type, v.Path)
		switch v.Type {
		case RouteType_GET:
			r.App.Get(v.Path, func(ctx iris.Context) {
				if err := v.Func(ctx); err != nil {
					log.Error(err)
				}
			})
		case RouteType_POST:
			r.App.Post(v.Path, func(ctx iris.Context) {
				if err := v.Func(ctx); err != nil {
					log.Error(err)
				}
			})
		case RouteType_WEBSOCKET:
			// handled above with /agent_ws
		}
	}
}

func (r *Router) Listen(host string) {
	if err := r.App.Listen(host); err != nil {
		log.Error(err)
	}
}
