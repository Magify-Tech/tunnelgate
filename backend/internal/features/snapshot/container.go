package snapshot

import (
	"errors"

	"github.com/gin-gonic/gin"

	apispecs "postman-transform/backend-golang/internal/features/api-specs"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "snapshotHandler"

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	return apispecs.RegisterContainers(container)
}

type RouteModule struct {
	Handler *apispecs.Handler `container:""`
}

func (m *RouteModule) Name() string {
	return "snapshot"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("snapshot module is nil")
	}
	if m.Handler == nil {
		return errors.New("snapshot handler is nil")
	}
	m.Handler.RegisterSnapshotRoutes(router.Group(""))
	return nil
}
