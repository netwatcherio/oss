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
	"netwatcher-controller/internal/workspace"
)

/************* Route types & model *************/

type Router struct {
	App             *iris.Application
	DB              *gorm.DB
	Routes          []*Route
	WebSocketServer *neffos.Server

	// Services/repos
	AuthSvc       auth.Service
	AgentsRepo    agent.Repository
	ProbesSvc     probe.Service
	WorkspacesSvc workspace.Service
	AgentsSvc     agent.Service
}

func NewRouter(
	db *gorm.DB,
	authSvc auth.Service,
	agentsRepo agent.Repository,
	probesSvc probe.Service,
	workspaceSvc workspace.Service,
	agentsSvc agent.Service,
) *Router {
	return &Router{
		App:           iris.New(),
		DB:            db,
		AuthSvc:       authSvc,
		AgentsRepo:    agentsRepo,
		ProbesSvc:     probesSvc,
		WorkspacesSvc: workspaceSvc,
		AgentsSvc:     agentsSvc,
	}
}

func (r *Router) Init() {
	// WebSocket
	if err := addWebSocketServer(r); err != nil {
		log.Error(err)
	}
	r.App.Get("/agent_ws", websocket.Handler(r.WebSocketServer))
	log.Info("Loading Agent Websocket Route...")

	// Global middleware that should apply to ALL requests
	r.App.Use(ProxyIPMiddleware)

	// Register routes now that services are available on Router
	addRouteAuth(r)
	addRouteAgents(r)
	addRouteWorkspaces(r)
	addRouteAgentAPI(r)

	log.Info("Loading all routes with per-route middleware...")
	if err := r.LoadRoutes(); err != nil {
		log.WithError(err).Error("failed to load routes")
		return
	}
	log.Infof("Loaded %d route(s).", len(r.Routes))
}

func (r *Router) LoadRoutes() error {
	for _, v := range r.Routes {
		handlers := make([]iris.Handler, 0, len(v.Middlewares)+2)

		// Inject JWT/session verification ONLY when route requires it
		/*if v.JWT {
			handlers = append(handlers, VerifySession())
		}*/

		// Route-specific middlewares in declared order
		if len(v.Middlewares) > 0 {
			handlers = append(handlers, v.Middlewares...)
		}

		// The actual handler wrapper (error logging)
		handlers = append(handlers, func(ctx iris.Context) {
			if err := v.Func(ctx); err != nil {
				log.WithError(err).Errorf("route %s (%s %s) failed", v.Name, v.Type, v.Path)
			}
		})

		// Register with Iris
		switch v.Type {
		case RouteType_GET:
			r.App.Get(v.Path, handlers...)
		case RouteType_POST:
			r.App.Post(v.Path, handlers...)
		case RouteType_DELETE:
			r.App.Delete(v.Path, handlers...)
		case RouteType_PATCH:
			r.App.Patch(v.Path, handlers...)
		case RouteType_WEBSOCKET:
			// handled above with /agent_ws
		default:
			log.Warnf("unknown route type %q for %s (%s)", v.Type, v.Name, v.Path)
		}

		log.Infof("Mounted route: %-30s %-6s %s (mw:%d)",
			v.Name, v.Type, v.Path, len(v.Middlewares))
	}
	return nil
}

func (r *Router) Listen(host string) {
	if err := r.App.Listen(host); err != nil {
		log.Error(err)
	}
}
