package main

import (
	configfeature "postman-transform/backend-golang/internal/features/config"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return configfeature.RegisterContainers(container)
}

var AdminModule = configfeature.Module

var Module = configfeature.Module

func main() {}
