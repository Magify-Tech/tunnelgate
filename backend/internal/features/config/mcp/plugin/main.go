package main

import (
	"postman-transform/backend-golang/internal/features/config/mcp"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return mcp.RegisterContainers(container)
}

var AdminModule = mcp.Module

var Module = mcp.Module
