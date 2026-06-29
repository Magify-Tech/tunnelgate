package main

import (
	shadowcompare "postman-transform/backend-golang/internal/features/shadow-compare"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return shadowcompare.RegisterContainers(container)
}

var AdminModule = shadowcompare.Module

var Module = shadowcompare.Module

func main() {}
