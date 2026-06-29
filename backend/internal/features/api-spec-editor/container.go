package apispeceditor

import (
	"errors"

	"github.com/gin-gonic/gin"

	apispecs "postman-transform/backend-golang/internal/features/api-specs"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const HandlerContainer = "apiSpecEditorHandler"

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
	return "api-spec-editor"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("api-spec-editor module is nil")
	}
	if m.Handler == nil {
		return errors.New("api-spec-editor handler is nil")
	}
	m.Handler.RegisterEditorRoutes(router.Group(""))
	return nil
}
