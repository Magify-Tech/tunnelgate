package auditlog

import (
	"errors"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/database"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

const (
	DatabaseContainer = "auditDatabase"
	ServiceContainer  = "auditLogService"
	HandlerContainer  = "auditLogHandler"
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
	return "audit-log"
}

func (m *RouteModule) ConfigureRoutes(router gin.IRouter) error {
	if m == nil {
		return errors.New("audit-log module is nil")
	}
	if m.Handler == nil {
		return errors.New("audit-log handler is nil")
	}
	m.Handler.RegisterAuditLogRoutes(router.Group(""))
	return nil
}

func ensureService(container pluginpkg.ServiceContainer) (*Service, error) {
	var service *Service
	ok, err := container.GetOptional(&service)
	if err != nil || ok {
		return service, err
	}
	db, err := resolveDatabase(container)
	if err != nil {
		return nil, err
	}
	var specDB *database.Connection
	if err := container.Get(&specDB); err != nil {
		return nil, err
	}
	service = NewService(db, specDB)
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

func resolveDatabase(container pluginpkg.ServiceContainer) (*database.Connection, error) {
	var db *database.Connection
	if ok, err := container.GetNamedOptional(DatabaseContainer, &db); err != nil || ok {
		return db, err
	}
	if err := container.Get(&db); err != nil {
		return nil, err
	}
	return db, nil
}
