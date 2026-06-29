package main

import (
	publicproxy "postman-transform/backend-golang/internal/features/proxy"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return publicproxy.RegisterContainers(container)
}

var PublicModule = publicproxy.Module

var Module = publicproxy.Module

func main() {}
