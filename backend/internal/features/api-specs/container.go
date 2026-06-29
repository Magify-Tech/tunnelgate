package mockapi

import (
	"errors"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/internal/features/realtime"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	RepositoryContainer = "mockAPIRepository"
	ServiceContainer    = "mockAPIService"
	HandlerContainer    = "mockAPIHandler"
)

var Module = RouteModule{}

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	if container == nil {
		return errors.New("container is nil")
	}
	if err := environment.RegisterContainers(container); err != nil {
		return err
	}

	repository, err := ensureRepository(container)
	if err != nil {
		return err
	}
	service, err := ensureService(container, repository)
	if err != nil {
		return err
	}
	return ensureHandler(container, service)
}

type RouteModule struct {
	Handler *Handler `container:""`
}

func (m *RouteModule) Name() string {
	return "api-specs"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("api-specs module is nil")
	}
	if m.Handler == nil {
		return errors.New("api-specs handler is nil")
	}
	m.Handler.RegisterAPISpecRoutes(router.Group(""))
	return nil
}

func ensureRepository(container pluginpkg.ServiceContainer) (*Repository, error) {
	var repository *Repository
	ok, err := container.GetOptional(&repository)
	if err != nil || ok {
		return repository, err
	}
	var db *database.Connection
	if err := container.Get(&db); err != nil {
		return nil, err
	}
	repository = NewRepository(db)
	if err := container.Register(repository, pluginpkg.WithName(RepositoryContainer)); err != nil {
		return nil, err
	}
	return repository, nil
}

func ensureService(container pluginpkg.ServiceContainer, repository *Repository) (*Service, error) {
	var service *Service
	ok, err := container.GetOptional(&service)
	if err != nil || ok {
		return service, err
	}
	var env *environment.Service
	if err := container.Get(&env); err != nil {
		return nil, err
	}
	service = NewService(repository, env)
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
