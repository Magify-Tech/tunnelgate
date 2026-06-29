package main

import (
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return proxyconfig.RegisterContainers(container)
}

var AdminModule = proxyconfig.Module

var Module = proxyconfig.Module
