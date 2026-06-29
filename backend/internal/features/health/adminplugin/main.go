package main

import (
	"postman-transform/backend-golang/internal/features/health"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return health.RegisterContainers(container)
}

var AdminModule = health.AdminModule

var Module = health.AdminModule

func main() {}
