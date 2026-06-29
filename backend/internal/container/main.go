// Package container provides a small runtime module system for this backend.
//
// It is intentionally built around Go's standard plugin package. A dynamic
// module can be compiled with:
//
//	go build -buildmode=plugin -o module.so ./path/to/module
//
// The plugin can export any of these symbols:
//
//   - RegisterContainers: func(pluginpkg.ServiceContainer) error
//   - NewModule: a constructor whose arguments are resolved from Container
//   - Module: an exported module instance variable
//
// Route configuration is handled by ConfigureRoutes. Modules may expose
// ConfigureRoutes, RegisterRoutes, or MountRoutes with one of these argument
// types: gin.IRouter, *gin.Engine, or *gin.RouterGroup.
package container
