package proxyreplay

import (
	"errors"

	"github.com/gin-gonic/gin"

	auditlog "postman-transform/backend-golang/internal/features/audit-log"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "proxyReplayHandler"

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	return auditlog.RegisterContainers(container)
}

type RouteModule struct {
	Handler *auditlog.Handler `container:""`
}

func (m *RouteModule) Name() string {
	return "proxy-replay"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("proxy-replay module is nil")
	}
	if m.Handler == nil {
		return errors.New("proxy-replay handler is nil")
	}
	m.Handler.RegisterProxyReplayRoutes(router.Group(""))
	return nil
}
