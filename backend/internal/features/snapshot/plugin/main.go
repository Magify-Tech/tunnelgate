package main

import (
	"postman-transform/backend-golang/internal/features/snapshot"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return snapshot.RegisterContainers(container)
}

var AdminModule = snapshot.Module

var Module = snapshot.Module

func main() {}
