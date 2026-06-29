package configfeature

import (
	"errors"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/features/config/mcp"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "configHandler"

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	if err := proxyconfig.RegisterContainers(container); err != nil {
		return err
	}
	return mcp.RegisterContainers(container)
}

type RouteModule struct {
	Proxy *proxyconfig.Handler `container:""`
	MCP   *mcp.Handler         `container:""`
}

func (m *RouteModule) Name() string {
	return "config"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("config module is nil")
	}
	if m.Proxy == nil {
		return errors.New("config proxy handler is nil")
	}
	if m.MCP == nil {
		return errors.New("config mcp handler is nil")
	}
	group := router.Group("")
	m.Proxy.Register(group)
	m.MCP.Register(group)
	return nil
}
