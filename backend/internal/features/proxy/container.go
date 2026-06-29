package publicproxy

import (
	"errors"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/internal/features/realtime"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "publicProxyHandler"

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	for _, register := range []func(pluginpkg.ServiceContainer) error{
		mockapi.RegisterContainers,
		proxyconfig.RegisterContainers,
		auditlog.RegisterContainers,
		realtime.RegisterContainers,
	} {
		if err := register(container); err != nil {
			return err
		}
	}
	return ensureHandler(container)
}

type RouteModule struct {
	Handler *Handler `container:""`
}

func (m *RouteModule) Name() string {
	return "proxy"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("proxy module is nil")
	}
	if m.Handler == nil {
		return errors.New("proxy handler is nil")
	}
	router.Use(m.Handler.Middleware)
	return nil
}

func ensureHandler(container pluginpkg.ServiceContainer) error {
	var handler *Handler
	ok, err := container.GetOptional(&handler)
	if err != nil || ok {
		return err
	}
	var mockAPI *mockapi.Service
	if err := container.Get(&mockAPI); err != nil {
		return err
	}
	var proxyConfig *proxyconfig.Service
	if err := container.Get(&proxyConfig); err != nil {
		return err
	}
	var auditLog *auditlog.Service
	if err := container.Get(&auditLog); err != nil {
		return err
	}
	var hub *realtime.Hub
	if _, err := container.GetOptional(&hub); err != nil {
		return err
	}
	return container.Register(NewHandler(mockAPI, proxyConfig, auditLog, hub), pluginpkg.WithName(HandlerContainer))
}
