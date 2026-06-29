package main

import (
	proxyreplay "postman-transform/backend-golang/internal/features/proxy-replay"
	pluginpkg "postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return proxyreplay.RegisterContainers(container)
}

var AdminModule = proxyreplay.Module

var Module = proxyreplay.Module

func main() {}
