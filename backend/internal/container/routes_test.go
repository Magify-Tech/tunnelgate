package container

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type interfaceRouteModule struct{}

func (interfaceRouteModule) ConfigureRoutes(router gin.IRouter) error {
	router.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})
	return nil
}

type groupRouteModule struct{}

func (groupRouteModule) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/group", func(c *gin.Context) {
		c.String(http.StatusOK, "group")
	})
}

type engineRouteModule struct{}

func (engineRouteModule) MountRoutes(router *gin.Engine) error {
	router.GET("/engine", func(c *gin.Context) {
		c.String(http.StatusOK, "engine")
	})
	return nil
}

func TestConfigureRoutesWithRouterGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api")

	if err := ConfigureRoutes(interfaceRouteModule{}, group); err != nil {
		t.Fatalf("configure routes: %v", err)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != "pong" {
		t.Fatalf("unexpected response %d %q", response.Code, response.Body.String())
	}
}

func TestConfigureRoutesWithConcreteRouterTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api")

	if err := ConfigureRoutes(groupRouteModule{}, group); err != nil {
		t.Fatalf("configure group routes: %v", err)
	}
	if err := ConfigureRoutes(engineRouteModule{}, engine); err != nil {
		t.Fatalf("configure engine routes: %v", err)
	}

	groupResponse := httptest.NewRecorder()
	groupRequest := httptest.NewRequest(http.MethodGet, "/api/group", nil)
	engine.ServeHTTP(groupResponse, groupRequest)
	if groupResponse.Code != http.StatusOK || groupResponse.Body.String() != "group" {
		t.Fatalf("unexpected group response %d %q", groupResponse.Code, groupResponse.Body.String())
	}

	engineResponse := httptest.NewRecorder()
	engineRequest := httptest.NewRequest(http.MethodGet, "/engine", nil)
	engine.ServeHTTP(engineResponse, engineRequest)
	if engineResponse.Code != http.StatusOK || engineResponse.Body.String() != "engine" {
		t.Fatalf("unexpected engine response %d %q", engineResponse.Code, engineResponse.Body.String())
	}
}
