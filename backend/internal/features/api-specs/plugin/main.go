package main

import (
	mockapi "postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return mockapi.RegisterContainers(container)
}

var AdminModule = mockapi.Module

var Module = mockapi.Module

func main() {}
