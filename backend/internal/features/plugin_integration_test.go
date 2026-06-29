package features

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"

	Container "postman-transform/backend-golang/internal/container"
	"postman-transform/backend-golang/internal/database"
)

func TestEnvironmentVariablesFeaturePluginLoadsIndependently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Go plugins are not supported on windows")
	}
	gin.SetMode(gin.TestMode)

	tempDir := t.TempDir()
	pluginPath := filepath.Join(tempDir, "environment.so")
	cmd := exec.Command("go", "build", "-buildmode=plugin", "-o", pluginPath, "./environment-variables/plugin")
	cmd.Env = append(os.Environ(), "GOCACHE="+filepath.Join(tempDir, "gocache"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build environment-variables plugin: %v\n%s", err, output)
	}

	container := Container.NewContainer()
	db := testDB(t)
	if err := container.Register(db, Container.WithName("database")); err != nil {
		t.Fatalf("register database: %v", err)
	}

	engine := gin.New()
	loader := Container.NewLoader(container)
	if _, err := loader.LoadAndConfigure(pluginPath, engine.Group("/api/v1")); err != nil {
		t.Fatalf("load environment plugin: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/engine/environment", nil)
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
}

func testDB(t *testing.T) *database.Connection {
	t.Helper()
	db, err := database.Open("file:"+t.TempDir()+"/test.sqlite", "sqlite")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := database.EnsureConnectionSchema(db); err != nil {
		t.Fatalf("EnsureConnectionSchema returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
