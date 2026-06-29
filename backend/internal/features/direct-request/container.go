package directrequest

import (
	"errors"

	"github.com/gin-gonic/gin"

	auditlog "postman-transform/backend-golang/internal/features/audit-log"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "directRequestHandler"

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
	return "direct-request"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("direct-request module is nil")
	}
	if m.Handler == nil {
		return errors.New("direct-request handler is nil")
	}
	m.Handler.RegisterDirectRequestRoutes(router.Group(""))
	return nil
}
