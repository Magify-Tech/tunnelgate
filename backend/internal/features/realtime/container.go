package realtime

import (
	"errors"

	"github.com/gin-gonic/gin"

	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HubContainer = "realtimeHub"

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	var hub *Hub
	ok, err := container.GetOptional(&hub)
	if err != nil || ok {
		return err
	}
	return container.Register(NewHub(), pluginpkg.WithName(HubContainer))
}

type RouteModule struct {
	Hub *Hub `container:""`
}

func (m *RouteModule) Name() string {
	return "realtime"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("realtime module is nil")
	}
	if m.Hub == nil {
		return errors.New("realtime hub is nil")
	}
	m.Hub.Register(router.Group(""))
	return nil
}
