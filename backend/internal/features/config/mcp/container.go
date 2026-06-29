package mcp

import (
	"errors"

	"github.com/gin-gonic/gin"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/internal/features/realtime"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	ServiceContainer = "mcpService"
	HandlerContainer = "mcpHandler"
)

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	if err := realtime.RegisterContainers(container); err != nil {
		return err
	}
	if err := environment.RegisterContainers(container); err != nil {
		return err
	}
	if err := mockapi.RegisterContainers(container); err != nil {
		return err
	}
	if err := proxyconfig.RegisterContainers(container); err != nil {
		return err
	}
	if err := auditlog.RegisterContainers(container); err != nil {
		return err
	}

	service, err := ensureService(container)
	if err != nil {
		return err
	}
	return ensureHandler(container, service)
}

type RouteModule struct {
	Handler *Handler `container:""`
}

func (m *RouteModule) Name() string {
	return "mcp"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("mcp module is nil")
	}
	if m.Handler == nil {
		return errors.New("mcp handler is nil")
	}
	m.Handler.Register(router.Group(""))
	return nil
}

func ensureService(container pluginpkg.ServiceContainer) (*Service, error) {
	var service *Service
	ok, err := container.GetOptional(&service)
	if err != nil || ok {
		return service, err
	}
	var db *database.Connection
	if err := container.Get(&db); err != nil {
		return nil, err
	}
	var mockService *mockapi.Service
	if err := container.Get(&mockService); err != nil {
		return nil, err
	}
	var envService *environment.Service
	if err := container.Get(&envService); err != nil {
		return nil, err
	}
	var proxyService *proxyconfig.Service
	if err := container.Get(&proxyService); err != nil {
		return nil, err
	}
	var auditService *auditlog.Service
	if err := container.Get(&auditService); err != nil {
		return nil, err
	}
	var hub *realtime.Hub
	if _, err := container.GetOptional(&hub); err != nil {
		return nil, err
	}
	service = NewService(db, Dependencies{
		MockAPI:       mockService,
		AuditLog:      auditService,
		Environment:   envService,
		ProxyConfig:   proxyService,
		Hub:           hub,
		CORSAllowList: appconfig.Load().CORSAllowList,
	})
	if err := container.Register(service, pluginpkg.WithName(ServiceContainer)); err != nil {
		return nil, err
	}
	return service, nil
}

func ensureHandler(container pluginpkg.ServiceContainer, service *Service) error {
	var handler *Handler
	ok, err := container.GetOptional(&handler)
	if err != nil || ok {
		return err
	}
	var hub *realtime.Hub
	if _, err := container.GetOptional(&hub); err != nil {
		return err
	}
	return container.Register(NewHandler(service, hub), pluginpkg.WithName(HandlerContainer))
}
