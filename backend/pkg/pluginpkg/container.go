// Package pluginpkg exposes the public plugin-facing service container API.
package pluginpkg

import (
	"errors"
	"reflect"
	"strings"
)

// ContainerOptions stores registration options for a service container entry.
type ContainerOptions struct {
	Name             string
	As               []reflect.Type
	RegisterConcrete bool
}

// ContainerOption customizes how an entry is registered in a ServiceContainer.
type ContainerOption func(*ContainerOptions) error

// BeanOptions is kept for compatibility with older plugins.
type BeanOptions = ContainerOptions

// BeanOption is kept for compatibility with older plugins.
type BeanOption = ContainerOption

// ServiceContainer is the public interface external modules use to register
// and resolve services without importing the backend's internal container.
type ServiceContainer interface {
	List() []string
	Register(container any, opts ...ContainerOption) error
	RegisterFactory(factory any, opts ...ContainerOption) (any, error)
	New(constructor any) (any, error)
	Invoke(fn any) ([]any, error)
	Get(target any) error
	GetOptional(target any) (bool, error)
	GetNamed(name string, target any) error
	GetNamedOptional(name string, target any) (bool, error)
	Autowire(target any) error
	ResolveType(typ reflect.Type) (reflect.Value, error)
	ResolveName(name string) (reflect.Value, error)
}

// ContainerRegistrar can be implemented by a plugin symbol to register additional
// dependencies before the module itself is built.
type ContainerRegistrar interface {
	RegisterContainers(ServiceContainer) error
}

// BeanRegistrar is kept for compatibility with older plugins.
type BeanRegistrar interface {
	RegisterBeans(ServiceContainer) error
}

// Initializer can be implemented by a module after its bean fields are wired.
type Initializer interface {
	Init(ServiceContainer) error
}

// DefaultContainerOptions returns the default registration options used by the
// built-in container implementation.
func DefaultContainerOptions() ContainerOptions {
	return ContainerOptions{RegisterConcrete: true}
}

// DefaultBeanOptions is kept for compatibility with older plugins.
func DefaultBeanOptions() ContainerOptions {
	return DefaultContainerOptions()
}

// WithName registers a container entry under a stable name for named injection.
func WithName(name string) ContainerOption {
	return func(opts *ContainerOptions) error {
		name = strings.TrimSpace(name)
		if name == "" {
			return errors.New("container name cannot be empty")
		}
		opts.Name = name
		return nil
	}
}

// WithNameOnly skips concrete type registration. Use it when multiple entries
// share a concrete type and should only be retrieved by name.
func WithNameOnly() ContainerOption {
	return func(opts *ContainerOptions) error {
		opts.RegisterConcrete = false
		return nil
	}
}

// AsType registers a container entry under another assignable type, usually an interface:
//
//	container.Register(sqlRepo, pluginpkg.AsType((*Repository)(nil)))
func AsType(typeToken any) ContainerOption {
	return func(opts *ContainerOptions) error {
		typ, err := TypeFromToken(typeToken)
		if err != nil {
			return err
		}
		opts.As = append(opts.As, typ)
		return nil
	}
}

// TypeFromToken converts an interface pointer or concrete value into a
// reflect.Type suitable for container registration.
func TypeFromToken(typeToken any) (reflect.Type, error) {
	if typeToken == nil {
		return nil, errors.New("type token cannot be nil")
	}
	typ := reflect.TypeOf(typeToken)
	if typ.Kind() == reflect.Pointer && typ.Elem().Kind() == reflect.Interface {
		return typ.Elem(), nil
	}
	return typ, nil
}
