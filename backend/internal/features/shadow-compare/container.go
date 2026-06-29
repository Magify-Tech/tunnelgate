package shadowcompare

import (
	"errors"

	"github.com/gin-gonic/gin"

	auditlog "postman-transform/backend-golang/internal/features/audit-log"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "shadowCompareHandler"

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
	return "shadow-compare"
}

func (m *RouteModule) ConfigureRoutes(_ gin.IRouter) error {
	if m == nil {
		return errors.New("shadow-compare module is nil")
	}
	if m.Handler == nil {
		return errors.New("shadow-compare handler is nil")
	}
	// Shadow compare reads audit-log records and shadow entries through
	// audit-log routes, so it has no separate backend route to mount.
	return nil
}
