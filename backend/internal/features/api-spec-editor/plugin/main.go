package main

import (
	apispeceditor "postman-transform/backend-golang/internal/features/api-spec-editor"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return apispeceditor.RegisterContainers(container)
}

var AdminModule = apispeceditor.Module

var Module = apispeceditor.Module

func main() {}
