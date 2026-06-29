package main

import (
	directrequest "postman-transform/backend-golang/internal/features/direct-request"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return directrequest.RegisterContainers(container)
}

var AdminModule = directrequest.Module

var Module = directrequest.Module

func main() {}
