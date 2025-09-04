package web

import (
	"github.com/kataras/iris/v12"
)

type RouteType string

const (
	RouteType_GET       RouteType = "GET"
	RouteType_POST      RouteType = "POST"
	RouteType_DELETE    RouteType = "DELETE"
	RouteType_PATCH     RouteType = "PATCH"
	RouteType_WEBSOCKET RouteType = "WEBSOCKET"
)

// Route now supports per-route middlewares.
type Route struct {
	Name        string
	Path        string
	Type        RouteType
	Middlewares []iris.Handler // optional, in-order; run before the handler
	Func        func(ctx iris.Context) error
}
