package environment

import (
	"errors"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/features/realtime"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	ServiceContainer = "environmentService"
	HandlerContainer = "environmentHandler"
)

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	if err := realtime.RegisterContainers(container); err != nil {
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
	return "environment-variables"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("environment-variables module is nil")
	}
	if m.Handler == nil {
		return errors.New("environment-variables handler is nil")
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
	service = NewService(db)
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
