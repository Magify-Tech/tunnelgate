package main

import (
	"postman-transform/backend-golang/internal/features/featureflag"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return featureflag.RegisterContainers(container)
}

var AdminModule = featureflag.Module

var Module = featureflag.Module

func main() {}
