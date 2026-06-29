package app

import (
	"fmt"
	"log"
	"path/filepath"
	goplugin "plugin"
	"strings"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/container"
	"postman-transform/backend-golang/internal/database"
	apispeceditor "postman-transform/backend-golang/internal/features/api-spec-editor"
	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/audit-log"
	configfeature "postman-transform/backend-golang/internal/features/config"
	directrequest "postman-transform/backend-golang/internal/features/direct-request"
	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/internal/features/featureflag"
	"postman-transform/backend-golang/internal/features/health"
	"postman-transform/backend-golang/internal/features/proxy"
	proxyreplay "postman-transform/backend-golang/internal/features/proxy-replay"
	"postman-transform/backend-golang/internal/features/realtime"
	shadowcompare "postman-transform/backend-golang/internal/features/shadow-compare"
	"postman-transform/backend-golang/internal/features/snapshot"
	"postman-transform/backend-golang/pkg/pluginpkg"
)

const mainDatabaseContainer = "database"

type builtInModule struct {
	name               string
	registerContainers func(pluginpkg.ServiceContainer) error
	module             any
	routeName          string
}

type pluginPath struct {
	path         string
	fromWildcard bool
}

func registerInfrastructureContainers(serviceContainer *container.Container, db *database.Connection, auditDB *database.Connection) error {
	if err := serviceContainer.Register(db, container.WithName(mainDatabaseContainer)); err != nil {
		return err
	}
	if auditDB == nil {
		auditDB = db
	}
	return serviceContainer.Register(auditDB, container.WithName(auditlog.DatabaseContainer), container.WithNameOnly())
}

func configureFeatureModules(serviceContainer *container.Container, cfg config.Config, adminRouter *gin.RouterGroup, publicRouter gin.IRouter) error {
	if len(cfg.AdminFeaturePlugins) > 0 {
		if err := loadPluginModules(serviceContainer, cfg.AdminFeaturePlugins, adminRouter, []string{"AdminModule"}, []string{"Module"}); err != nil {
			return err
		}
	} else {
		if err := loadBuiltInModules(serviceContainer, defaultAdminModules(), adminRouter); err != nil {
			return err
		}
	}

	if len(cfg.PublicFeaturePlugins) > 0 {
		if err := loadPluginModules(serviceContainer, cfg.PublicFeaturePlugins, publicRouter, []string{"PublicModule"}, []string{"Module"}); err != nil {
			return err
		}
	} else {
		if err := loadBuiltInModules(serviceContainer, defaultPublicModules(), publicRouter); err != nil {
			return err
		}
	}

	return nil
}

func loadPluginModules(serviceContainer *container.Container, paths []string, router any, moduleSymbols []string, explicitFallbackSymbols []string) error {
	expanded, err := expandPluginPaths(paths)
	if err != nil {
		return err
	}
	loader := container.NewLoader(serviceContainer)
	for _, plugin := range expanded {
		symbols := moduleSymbols
		if plugin.fromWildcard {
			hasSymbol, err := pluginHasAnySymbol(plugin.path, moduleSymbols)
			if err != nil {
				return fmt.Errorf("inspect feature module plugin %q: %w", plugin.path, err)
			}
			if !hasSymbol {
				log.Printf("Skipping feature module plugin %s: no compatible module symbol %s", plugin.path, strings.Join(moduleSymbols, " or "))
				continue
			}
		} else if len(explicitFallbackSymbols) > 0 {
			symbols = append(append([]string{}, moduleSymbols...), explicitFallbackSymbols...)
		}
		loader.ModuleSymbols = symbols
		log.Printf("Loading feature module plugin %s", plugin.path)
		if _, err := loader.LoadAndConfigure(plugin.path, router); err != nil {
			return fmt.Errorf("load feature module plugin %q: %w", plugin.path, err)
		}
		log.Printf("Loaded feature module plugin %s", plugin.path)
	}
	return nil
}

func loadBuiltInModules(serviceContainer *container.Container, modules []builtInModule, router any) error {
	loader := container.NewLoader(serviceContainer)
	for _, module := range modules {
		if module.registerContainers != nil {
			if err := module.registerContainers(serviceContainer); err != nil {
				return fmt.Errorf("register %s containers: %w", module.name, err)
			}
		}
		if _, err := loader.LoadInstanceAndConfigure(module.routeName, module.module, router); err != nil {
			return fmt.Errorf("configure %s module: %w", module.name, err)
		}
	}
	return nil
}

func defaultAdminModules() []builtInModule {
	return []builtInModule{
		{name: "featureflag", registerContainers: featureflag.RegisterContainers, module: &featureflag.RouteModule{}, routeName: "featureflag"},
		{name: "realtime", registerContainers: realtime.RegisterContainers, module: &realtime.RouteModule{}, routeName: "realtime"},
		{name: "health", registerContainers: health.RegisterContainers, module: &health.AdminRouteModule{}, routeName: "health-admin"},
		{name: "environment-variables", registerContainers: environment.RegisterContainers, module: &environment.RouteModule{}, routeName: "environment-variables"},
		{name: "api-specs", registerContainers: mockapi.RegisterContainers, module: &mockapi.RouteModule{}, routeName: "api-specs"},
		{name: "api-spec-editor", registerContainers: apispeceditor.RegisterContainers, module: &apispeceditor.RouteModule{}, routeName: "api-spec-editor"},
		{name: "snapshot", registerContainers: snapshot.RegisterContainers, module: &snapshot.RouteModule{}, routeName: "snapshot"},
		{name: "config", registerContainers: configfeature.RegisterContainers, module: &configfeature.RouteModule{}, routeName: "config"},
		{name: "audit-log", registerContainers: auditlog.RegisterContainers, module: &auditlog.RouteModule{}, routeName: "audit-log"},
		{name: "direct-request", registerContainers: directrequest.RegisterContainers, module: &directrequest.RouteModule{}, routeName: "direct-request"},
		{name: "proxy-replay", registerContainers: proxyreplay.RegisterContainers, module: &proxyreplay.RouteModule{}, routeName: "proxy-replay"},
		{name: "shadow-compare", registerContainers: shadowcompare.RegisterContainers, module: &shadowcompare.RouteModule{}, routeName: "shadow-compare"},
	}
}

func defaultPublicModules() []builtInModule {
	return []builtInModule{
		{name: "proxy", registerContainers: publicproxy.RegisterContainers, module: &publicproxy.RouteModule{}, routeName: "proxy"},
		{name: "health", registerContainers: health.RegisterContainers, module: &health.PublicRouteModule{}, routeName: "health-public"},
	}
}

func logLoadedFeatureModules(cfg config.Config) {
	if len(cfg.AdminFeaturePlugins) > 0 {
		log.Printf("Loaded admin feature plugins: %s", strings.Join(pluginPathsForLog(cfg.AdminFeaturePlugins), ", "))
	} else {
		log.Printf("Loaded built-in admin feature modules: %s", strings.Join(moduleNames(defaultAdminModules()), ", "))
	}

	if len(cfg.PublicFeaturePlugins) > 0 {
		log.Printf("Loaded public feature plugins: %s", strings.Join(pluginPathsForLog(cfg.PublicFeaturePlugins), ", "))
	} else {
		log.Printf("Loaded built-in public feature modules: %s", strings.Join(moduleNames(defaultPublicModules()), ", "))
	}
}

func expandPluginPaths(paths []string) ([]pluginPath, error) {
	expanded := make([]pluginPath, 0, len(paths))
	seen := map[string]struct{}{}
	for _, rawPath := range paths {
		rawPath = strings.TrimSpace(rawPath)
		if rawPath == "" {
			continue
		}
		if !hasWildcard(rawPath) {
			if _, ok := seen[rawPath]; ok {
				continue
			}
			expanded = append(expanded, pluginPath{path: rawPath})
			seen[rawPath] = struct{}{}
			continue
		}

		matches, err := filepath.Glob(rawPath)
		if err != nil {
			return nil, fmt.Errorf("expand plugin wildcard %q: %w", rawPath, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("plugin wildcard %q matched no files", rawPath)
		}
		for _, match := range matches {
			if _, ok := seen[match]; ok {
				continue
			}
			expanded = append(expanded, pluginPath{path: match, fromWildcard: true})
			seen[match] = struct{}{}
		}
	}
	return expanded, nil
}

func pluginHasAnySymbol(path string, symbolNames []string) (bool, error) {
	plugin, err := goplugin.Open(path)
	if err != nil {
		return false, err
	}
	for _, symbolName := range symbolNames {
		if _, err := plugin.Lookup(symbolName); err == nil {
			return true, nil
		}
	}
	return false, nil
}

func hasWildcard(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func pluginPathsForLog(paths []string) []string {
	expanded, err := expandPluginPaths(paths)
	if err != nil {
		return paths
	}
	result := make([]string, 0, len(expanded))
	for _, path := range expanded {
		result = append(result, path.path)
	}
	return result
}

func moduleNames(modules []builtInModule) []string {
	names := make([]string, 0, len(modules))
	for _, module := range modules {
		names = append(names, module.name)
	}
	return names
}
