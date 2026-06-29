package featureflag

import (
	"errors"

	"github.com/gin-gonic/gin"

	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	ServiceContainer = "featureFlagService"
	HandlerContainer = "featureFlagHandler"
)

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
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
	return "featureflag"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("featureflag module is nil")
	}
	if m.Handler == nil {
		return errors.New("featureflag handler is nil")
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
	service = NewService()
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
	return container.Register(NewHandler(service), pluginpkg.WithName(HandlerContainer))
}
