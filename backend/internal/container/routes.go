package container

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/gin-gonic/gin"
)

var ErrNoRouteConfigurer = errors.New("module has no route configurer")

// RouteConfigurer is the preferred route contract for dynamic modules. Both
// *gin.Engine and *gin.RouterGroup satisfy gin.IRouter.
type RouteConfigurer interface {
	ConfigureRoutes(gin.IRouter) error
}

// ConfigureRoutes mounts routes for a module onto either *gin.Engine or
// *gin.RouterGroup. The module may implement ConfigureRoutes, RegisterRoutes,
// or MountRoutes with gin.IRouter, *gin.Engine, or *gin.RouterGroup.
func ConfigureRoutes(module any, router any) error {
	if module == nil {
		return errors.New("module cannot be nil")
	}
	routeTarget, err := normalizeRouter(router)
	if err != nil {
		return err
	}
	if configurer, ok := module.(RouteConfigurer); ok {
		return configurer.ConfigureRoutes(routeTarget.router)
	}

	moduleValue := reflect.ValueOf(module)
	for _, methodName := range []string{"ConfigureRoutes", "RegisterRoutes", "MountRoutes"} {
		method := moduleValue.MethodByName(methodName)
		if !method.IsValid() {
			continue
		}
		if err := callRouteMethod(methodName, method, routeTarget); err == nil {
			return nil
		} else if !errors.Is(err, ErrNoRouteConfigurer) {
			return err
		}
	}
	return ErrNoRouteConfigurer
}

type routeTarget struct {
	source reflect.Value
	router gin.IRouter
	engine *gin.Engine
	group  *gin.RouterGroup
}

func normalizeRouter(router any) (routeTarget, error) {
	if router == nil {
		return routeTarget{}, errors.New("router cannot be nil")
	}
	target := routeTarget{source: reflect.ValueOf(router)}
	switch typed := router.(type) {
	case *gin.Engine:
		if typed == nil {
			return routeTarget{}, errors.New("router cannot be nil")
		}
		target.router = typed
		target.engine = typed
	case *gin.RouterGroup:
		if typed == nil {
			return routeTarget{}, errors.New("router cannot be nil")
		}
		target.router = typed
		target.group = typed
	case gin.IRouter:
		target.router = typed
	default:
		return routeTarget{}, fmt.Errorf("router must be *gin.Engine, *gin.RouterGroup, or gin.IRouter; got %T", router)
	}
	return target, nil
}

func callRouteMethod(name string, method reflect.Value, target routeTarget) error {
	methodType := method.Type()
	if methodType.NumIn() != 1 {
		return ErrNoRouteConfigurer
	}
	arg, ok := routeArgument(methodType.In(0), target)
	if !ok {
		return ErrNoRouteConfigurer
	}
	results := method.Call([]reflect.Value{arg})
	if len(results) == 0 {
		return nil
	}
	if len(results) == 1 && typeAssignableTo(results[0].Type(), errorType) {
		if isNilValue(results[0]) {
			return nil
		}
		return results[0].Interface().(error)
	}
	return fmt.Errorf("%s must return nothing or error", name)
}

func routeArgument(param reflect.Type, target routeTarget) (reflect.Value, bool) {
	if target.source.IsValid() && valueAssignableTo(target.source, param) {
		return target.source, true
	}
	if target.engine != nil {
		engineValue := reflect.ValueOf(target.engine)
		if valueAssignableTo(engineValue, param) {
			return engineValue, true
		}
	}
	if target.group != nil {
		groupValue := reflect.ValueOf(target.group)
		if valueAssignableTo(groupValue, param) {
			return groupValue, true
		}
	}
	routerValue := reflect.ValueOf(target.router)
	if routerValue.IsValid() && valueAssignableTo(routerValue, param) {
		return routerValue, true
	}
	return reflect.Value{}, false
}

func valueAssignableTo(value reflect.Value, target reflect.Type) bool {
	if !value.IsValid() {
		return false
	}
	return typeAssignableTo(value.Type(), target)
}
