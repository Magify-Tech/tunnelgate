package main

import (
	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return environment.RegisterContainers(container)
}

var AdminModule = environment.Module

var Module = environment.Module

func main() {}
