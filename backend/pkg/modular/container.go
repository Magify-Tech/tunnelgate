// Package modular is kept for compatibility with plugins that imported the
// earlier public package path. New plugins should import pkg/pluginpkg.
package modular

import (
	"reflect"

	"postman-transform/backend-golang/pkg/pluginpkg"
)

type ContainerOptions = pluginpkg.ContainerOptions
type ContainerOption = pluginpkg.ContainerOption
type BeanOptions = pluginpkg.BeanOptions
type BeanOption = pluginpkg.BeanOption
type ServiceContainer = pluginpkg.ServiceContainer
type ContainerRegistrar = pluginpkg.ContainerRegistrar
type BeanRegistrar = pluginpkg.BeanRegistrar
type Initializer = pluginpkg.Initializer

func DefaultContainerOptions() ContainerOptions {
	return pluginpkg.DefaultContainerOptions()
}

func DefaultBeanOptions() ContainerOptions {
	return pluginpkg.DefaultBeanOptions()
}

func WithName(name string) ContainerOption {
	return pluginpkg.WithName(name)
}

func WithNameOnly() ContainerOption {
	return pluginpkg.WithNameOnly()
}

func AsType(typeToken any) ContainerOption {
	return pluginpkg.AsType(typeToken)
}

func TypeFromToken(typeToken any) (reflect.Type, error) {
	return pluginpkg.TypeFromToken(typeToken)
}
