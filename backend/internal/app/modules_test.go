package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/container"
	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/features/featureflag"
)

func TestConfigureFeatureModulesUsesDynamicBuiltInModules(t *testing.T) {
	gin.SetMode(gin.TestMode)

	container := container.NewContainer()
	db := testDB(t)
	if err := registerInfrastructureContainers(container, db, db); err != nil {
		t.Fatalf("register infrastructure containers: %v", err)
	}

	adminEngine := gin.New()
	adminGroup := adminEngine.Group("/api/v1")
	publicEngine := gin.New()
	if err := configureFeatureModules(container, config.Config{}, adminGroup, publicEngine); err != nil {
		t.Fatalf("configure feature modules: %v", err)
	}

	assertStatus(t, adminEngine, http.MethodGet, "/api/v1/healthz", http.StatusOK)
	assertStatus(t, adminEngine, http.MethodGet, "/api/v1/features/", http.StatusOK)
	assertStatus(t, adminEngine, http.MethodGet, "/api/v1/engine/environment", http.StatusOK)
	assertStatus(t, publicEngine, http.MethodGet, "/", http.StatusOK)
}

func TestPublishLoadedFeaturesSkipsMissingService(t *testing.T) {
	c := container.NewContainer()

	if err := publishLoadedFeatures(c); err != nil {
		t.Fatalf("publish loaded features: %v", err)
	}
}

func TestPublishLoadedFeaturesWritesContainerBeans(t *testing.T) {
	c := container.NewContainer()
	service := featureflag.NewService()
	if err := c.Register(service, container.WithName(featureflag.ServiceContainer)); err != nil {
		t.Fatalf("register feature flag service: %v", err)
	}

	if err := publishLoadedFeatures(c); err != nil {
		t.Fatalf("publish loaded features: %v", err)
	}

	if !containsString(service.Get(), "container") {
		t.Fatalf("expected published features to include container, got %v", service.Get())
	}
	if !containsString(service.Get(), featureflag.ServiceContainer) {
		t.Fatalf("expected published features to include %q, got %v", featureflag.ServiceContainer, service.Get())
	}
}

func TestConfigureFeatureModulesUsesConfiguredPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go plugins are not supported on windows")
	}
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	pluginPath := filepath.Join(tempDir, "health.so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", pluginPath, "../features/health/plugin")
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(tempDir, "gocache"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build health plugin: %v\n%s", err, output)
	}

	container := container.NewContainer()
	db := testDB(t)
	if err := registerInfrastructureContainers(container, db, db); err != nil {
		t.Fatalf("register infrastructure containers: %v", err)
	}

	adminEngine := gin.New()
	publicEngine := gin.New()
	cfg := config.Config{
		AdminFeaturePlugins:  []string{pluginPath},
		PublicFeaturePlugins: []string{pluginPath},
	}
	if err := configureFeatureModules(container, cfg, adminEngine.Group("/api/v1"), publicEngine); err != nil {
		t.Fatalf("configure plugin modules: %v", err)
	}

	assertStatus(t, adminEngine, http.MethodGet, "/api/v1/healthz", http.StatusOK)
	assertStatus(t, publicEngine, http.MethodGet, "/", http.StatusOK)
}

func TestConfigureFeatureModulesUsesWildcardPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go plugins are not supported on windows")
	}
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	buildFeaturePlugin(t, tempDir, "environment-variables")

	container := container.NewContainer()
	db := testDB(t)
	if err := registerInfrastructureContainers(container, db, db); err != nil {
		t.Fatalf("register infrastructure containers: %v", err)
	}

	pattern := filepath.Join(tempDir, "*.so")
	adminEngine := gin.New()
	publicEngine := gin.New()
	cfg := config.Config{
		AdminFeaturePlugins: []string{pattern},
	}
	if err := configureFeatureModules(container, cfg, adminEngine.Group("/api/v1"), publicEngine); err != nil {
		t.Fatalf("configure wildcard plugin modules: %v", err)
	}

	assertStatus(t, adminEngine, http.MethodGet, "/api/v1/engine/environment", http.StatusOK)
	assertStatus(t, publicEngine, http.MethodGet, "/", http.StatusOK)
}

func TestConfigureFeatureModulesLoadsSplitAdminAndPublicPlugins(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go plugins are not supported on windows")
	}
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	adminDir := filepath.Join(tempDir, "admin")
	publicDir := filepath.Join(tempDir, "public")
	buildFeaturePluginPackage(t, adminDir, "health", "../features/health/adminplugin")
	buildFeaturePluginPackage(t, publicDir, "health", "../features/health/publicplugin")

	container := container.NewContainer()
	db := testDB(t)
	if err := registerInfrastructureContainers(container, db, db); err != nil {
		t.Fatalf("register infrastructure containers: %v", err)
	}

	adminEngine := gin.New()
	publicEngine := gin.New()
	cfg := config.Config{
		AdminFeaturePlugins:  []string{filepath.Join(adminDir, "*.so")},
		PublicFeaturePlugins: []string{filepath.Join(publicDir, "*.so")},
	}
	if err := configureFeatureModules(container, cfg, adminEngine.Group("/api/v1"), publicEngine); err != nil {
		t.Fatalf("configure split plugin modules: %v", err)
	}

	assertStatus(t, adminEngine, http.MethodGet, "/api/v1/healthz", http.StatusOK)
	assertStatus(t, publicEngine, http.MethodGet, "/", http.StatusOK)
}

func TestExpandPluginPathsReportsUnmatchedWildcard(t *testing.T) {
	_, err := expandPluginPaths([]string{filepath.Join(t.TempDir(), "*.so")})
	if err == nil {
		t.Fatalf("expected unmatched wildcard to return an error")
	}
}

func assertStatus(t *testing.T, engine *gin.Engine, method string, path string, want int) {
	t.Helper()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	engine.ServeHTTP(response, request)
	if response.Code != want {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, response.Code, want, response.Body.String())
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func buildFeaturePlugin(t *testing.T, outputDir string, feature string) string {
	t.Helper()
	return buildFeaturePluginPackage(t, outputDir, feature, "../features/"+feature+"/plugin")
}

func buildFeaturePluginPackage(t *testing.T, outputDir string, feature string, pkg string) string {
	t.Helper()
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("create plugin output dir: %v", err)
	}
	pluginPath := filepath.Join(outputDir, feature+".so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", pluginPath, pkg)
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(outputDir, "gocache"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build %s plugin: %v\n%s", feature, err, output)
	}
	return pluginPath
}

func testDB(t *testing.T) *database.Connection {
	t.Helper()
	db, err := database.OpenSQLite("file:" + t.TempDir() + "/test.sqlite")
	if err != nil {
		t.Fatalf("OpenSQLite returned error: %v", err)
	}
	if err := database.EnsureSchema(db); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
