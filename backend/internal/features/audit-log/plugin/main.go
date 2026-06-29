package main

import (
	auditlog "postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

func RegisterContainers(container pluginpkg.ServiceContainer) error {
	return auditlog.RegisterContainers(container)
}

var AdminModule = auditlog.Module

var Module = auditlog.Module

func main() {}
