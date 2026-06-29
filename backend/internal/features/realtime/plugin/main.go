package main

import (
	"postman-transform/backend-golang/internal/features/realtime"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return realtime.RegisterContainers(container)
}

var AdminModule = realtime.Module

var Module = realtime.Module

func main() {}
