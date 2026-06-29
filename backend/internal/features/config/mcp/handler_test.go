package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/internal/features/environment-variables"
)

func TestGetConfigReturnsMCPEndpoint(t *testing.T) {
	t.Setenv(appconfig.DefaultTunnelGateRuntimeEnv, "docker")
	router := testRouter(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/engine/mcp-config", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	assertEndpoint(t, recorder.Body.Bytes())
	assertRuntime(t, recorder.Body.Bytes(), "docker", true)
}

func TestUpdateConfigReturnsMCPEndpoint(t *testing.T) {
	router := testRouter(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/engine/mcp-config", bytes.NewBufferString(`{"enabled":true,"clientToken":"","allowedOrigins":[]}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	assertEndpoint(t, recorder.Body.Bytes())
}

func TestPostMCPInitialize(t *testing.T) {
	router := testFullRouter(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Mcp-Session-Id") == "" {
		t.Fatal("expected Mcp-Session-Id header")
	}
	var payload struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unable to decode response: %v", err)
	}
	if payload.Result.ProtocolVersion != supportedProtocolVersion {
		t.Fatalf("unexpected protocol version: %q", payload.Result.ProtocolVersion)
	}
}

func TestMCPToolUpsertAndListAPIMocks(t *testing.T) {
	router := testFullRouter(t)

	upsertBody := `{
		"jsonrpc":"2.0",
		"id":"upsert-1",
		"method":"tools/call",
		"params":{
			"name":"upsert_api_mock_spec",
			"arguments":{
				"collectionName":"MCP",
				"routeName":"Get user",
				"method":"GET",
				"routePath":"/users/1",
				"responseExamples":[{"status":200,"name":"OK","body":"{\"id\":1}"}]
			}
		}
	}`
	upsert := httptest.NewRecorder()
	router.ServeHTTP(upsert, httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewBufferString(upsertBody)))
	if upsert.Code != http.StatusOK {
		t.Fatalf("unexpected upsert status: %d body=%s", upsert.Code, upsert.Body.String())
	}

	listBody := `{"jsonrpc":"2.0","id":"list-1","method":"tools/call","params":{"name":"list_api_mocks","arguments":{"page":1,"pageSize":25}}}`
	list := httptest.NewRecorder()
	router.ServeHTTP(list, httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewBufferString(listBody)))
	if list.Code != http.StatusOK {
		t.Fatalf("unexpected list status: %d body=%s", list.Code, list.Body.String())
	}
	var payload struct {
		Result struct {
			StructuredContent struct {
				Total int `json:"total"`
				Items []struct {
					RoutePath string `json:"routePath"`
				} `json:"items"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unable to decode list response: %v", err)
	}
	if payload.Result.StructuredContent.Total != 1 || len(payload.Result.StructuredContent.Items) != 1 || payload.Result.StructuredContent.Items[0].RoutePath != "/users/1" {
		t.Fatalf("unexpected list payload: %s", list.Body.String())
	}
}

func TestMCPToolSendDirectRequest(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(target.Close)

	router := testFullRouter(t)
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      "direct-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "send_direct_request",
			"arguments": map[string]any{
				"method":         http.MethodPost,
				"targetUrl":      target.URL,
				"requestHeaders": "{}",
				"requestBody":    `{"hello":"mcp"}`,
			},
		},
	}
	payload, _ := json.Marshal(body)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected direct request status: %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Result struct {
			StructuredContent struct {
				Item struct {
					ResponseStatus *int   `json:"responseStatus"`
					ResponseBody   string `json:"responseBody"`
				} `json:"item"`
			} `json:"structuredContent"`
		} `json:"result"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unable to decode direct response: %v", err)
	}
	if response.Result.StructuredContent.Item.ResponseStatus == nil || *response.Result.StructuredContent.Item.ResponseStatus != http.StatusCreated {
		t.Fatalf("unexpected direct request payload: %s", recorder.Body.String())
	}
	if response.Result.StructuredContent.Item.ResponseBody != `{"ok":true}` {
		t.Fatalf("unexpected direct request body: %q", response.Result.StructuredContent.Item.ResponseBody)
	}
}

func testRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(NewService(testDB(t)), nil).Register(router.Group("/api/v1"))
	return router
}

func testFullRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db := testDB(t)
	envService := environment.NewService(db)
	service := NewService(db, Dependencies{
		MockAPI:       mockapi.NewService(mockapi.NewRepository(db), envService),
		Environment:   envService,
		ProxyConfig:   proxyconfig.NewService(db),
		CORSAllowList: []string{"http://localhost:5173"},
	})
	if _, err := service.Update(context.Background(), true, "", []string{}); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(service, nil).Register(router.Group("/api/v1"))
	return router
}

func assertEndpoint(t *testing.T, body []byte) {
	t.Helper()
	var payload struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unable to decode response: %v", err)
	}
	if payload.Endpoint != endpoint {
		t.Fatalf("unexpected endpoint: %q", payload.Endpoint)
	}
}

func assertRuntime(t *testing.T, body []byte, runtime string, isDocker bool) {
	t.Helper()
	var payload struct {
		RuntimeContext struct {
			Runtime  string `json:"runtime"`
			IsDocker bool   `json:"isDocker"`
		} `json:"runtimeContext"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unable to decode response: %v", err)
	}
	if payload.RuntimeContext.Runtime != runtime || payload.RuntimeContext.IsDocker != isDocker {
		t.Fatalf("unexpected runtime context: %s", body)
	}
}
