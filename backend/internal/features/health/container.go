package health

import (
	"errors"

	"github.com/gin-gonic/gin"

	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	AdminHandlerContainer  = "healthAdminHandler"
	PublicHandlerContainer = "healthPublicHandler"
)

var Module = AdminModule

var AdminModule = AdminRouteModule{}

var PublicModule = PublicRouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	var admin *AdminHandler
	adminExists, err := container.GetOptional(&admin)
	if err != nil {
		return err
	}
	if !adminExists {
		if err := container.Register(NewAdminHandler(), pluginpkg.WithName(AdminHandlerContainer)); err != nil {
			return err
		}
	}

	var public *PublicHandler
	publicExists, err := container.GetOptional(&public)
	if err != nil {
		return err
	}
	if !publicExists {
		if err := container.Register(NewPublicHandler(), pluginpkg.WithName(PublicHandlerContainer)); err != nil {
			return err
		}
	}
	return nil
}

type AdminRouteModule struct {
	Handler *AdminHandler `container:""`
}

func (m *AdminRouteModule) Name() string {
	return "health-admin"
}

func (m *AdminRouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("health admin module is nil")
	}
	if m.Handler == nil {
		return errors.New("health admin handler is nil")
	}
	m.Handler.Register(router.Group(""))
	return nil
}

type PublicRouteModule struct {
	Handler *PublicHandler `container:""`
}

func (m *PublicRouteModule) Name() string {
	return "health-public"
}

func (m *PublicRouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("health public module is nil")
	}
	if m.Handler == nil {
		return errors.New("health public handler is nil")
	}
	m.Handler.Register(router)
	return nil
}
