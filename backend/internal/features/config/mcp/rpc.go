package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/internal/features/realtime"
	"postman-transform/backend-golang/internal/uuidv7"
)

const supportedProtocolVersion = "2025-06-18"

type jsonRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *jsonRPCError `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type apiMockSpecInput struct {
	CollectionName    string                    `json:"collectionName"`
	RouteName         string                    `json:"routeName"`
	PostmanFolderPath []string                  `json:"postmanFolderPath"`
	Method            string                    `json:"method"`
	RoutePath         string                    `json:"routePath"`
	ResponseExamples  []mockapi.ResponseExample `json:"responseExamples"`
	RequestBodyKeys   []string                  `json:"requestBodyKeys"`
	RequestBodyTypes  mockapi.RequestBodyTypes  `json:"requestBodyTypes"`
	RequestParamKeys  []string                  `json:"requestParamKeys"`
	MockEnabled       *bool                     `json:"mockEnabled"`
	ProxyEnabled      *bool                     `json:"proxyEnabled"`
}

type bulkAPIInput struct {
	Items []apiMockSpecInput `json:"items"`
}

type updateAPIInput struct {
	ID string `json:"id"`
	apiMockSpecInput
	fields map[string]bool
}

type idInput struct {
	ID string `json:"id"`
}

type pathInput struct {
	Mode string   `json:"mode"`
	Path []string `json:"path"`
}

type messageInput struct {
	Message string `json:"message"`
}

type toggleInput struct {
	ID      string `json:"id"`
	Enabled *bool  `json:"enabled"`
}

type enabledInput struct {
	Enabled *bool `json:"enabled"`
}

type selectResponseInput struct {
	ID     string `json:"id"`
	Status int    `json:"status"`
}

type environmentUpsertInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type environmentUpdateInput struct {
	CurrentKey string  `json:"currentKey"`
	Key        *string `json:"key"`
	Value      string  `json:"value"`
}

type proxyConfigInput struct {
	RealServerBaseURL string                     `json:"realServerBaseUrl"`
	ShadowEndpoints   []proxyShadowEndpointInput `json:"shadowEndpoints"`
}

type proxyShadowEndpointInput struct {
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Enabled *bool  `json:"enabled"`
}

type shadowEndpointsInput struct {
	ShadowEndpoints []proxyconfig.ShadowEndpoint `json:"shadowEndpoints"`
}

type headerRulesInput struct {
	HeaderRules []proxyconfig.HeaderMatchReplaceRule `json:"headerRules"`
}

type securityInput struct {
	Security *proxyconfig.PublicProxySecurity `json:"security"`
}

func (s *Service) HandleJSONRPC(ctx context.Context, message any) (*jsonRPCResponse, error) {
	request, ok := message.(map[string]any)
	if !ok {
		return createMCPError(nil, -32600, "Invalid JSON-RPC request", []map[string]string{{"message": "request must be an object"}}), nil
	}

	id := requestID(request["id"])
	jsonrpc, ok := request["jsonrpc"].(string)
	if !ok || jsonrpc != "2.0" {
		return createMCPError(id, -32600, "Invalid JSON-RPC request", []map[string]string{{"message": "jsonrpc must be 2.0"}}), nil
	}
	method, ok := request["method"].(string)
	if !ok || strings.TrimSpace(method) == "" {
		return createMCPError(id, -32600, "Invalid JSON-RPC request", []map[string]string{{"message": "method is required"}}), nil
	}

	switch method {
	case "initialize":
		return createResult(id, map[string]any{
			"protocolVersion": supportedProtocolVersion,
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "postman-transform",
				"version": "1.0.0",
			},
		}), nil
	case "notifications/initialized":
		return nil, nil
	case "ping":
		return createResult(id, map[string]any{}), nil
	case "tools/list":
		return createResult(id, map[string]any{"tools": tools()}), nil
	case "tools/call":
		return s.handleToolCall(ctx, id, request["params"]), nil
	default:
		return createMCPError(id, -32601, "Method not found: "+method), nil
	}
}

func (s *Service) handleToolCall(ctx context.Context, id any, params any) *jsonRPCResponse {
	var parsed toolCallParams
	if params == nil {
		params = map[string]any{}
	}
	if err := decode(params, &parsed); err != nil || strings.TrimSpace(parsed.Name) == "" {
		return createMCPError(id, -32602, "Invalid params", errData(err))
	}
	if parsed.Arguments == nil {
		parsed.Arguments = map[string]any{}
	}

	result, err := s.callTool(ctx, parsed.Name, parsed.Arguments)
	if err != nil {
		var validation paramError
		if errors.As(err, &validation) {
			return createMCPError(id, -32602, validation.Error(), []map[string]string{{"message": validation.Error()}})
		}
		if errors.Is(err, errUnknownTool) {
			return createMCPError(id, -32602, "Unknown tool: "+parsed.Name)
		}
		return createMCPError(id, -32000, err.Error())
	}
	return createResult(id, toolCallResult(result))
}

func (s *Service) callTool(ctx context.Context, name string, input map[string]any) (any, error) {
	switch name {
	case "list_api_mocks":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		page := intFromMap(input, "page", 1)
		pageSize := intFromMap(input, "pageSize", 25)
		if page < 1 {
			page = 1
		}
		if pageSize < 1 || pageSize > 100 {
			return nil, paramError("pageSize must be between 1 and 100")
		}
		return s.mockAPI.List(ctx, page, pageSize)
	case "get_api_mock":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.Get(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("API not found")
		}
		return map[string]any{"item": item}, nil
	case "list_api_directory":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		var parsed pathInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		mode := strings.TrimSpace(parsed.Mode)
		if mode == "" {
			mode = "postman"
		}
		result, err := s.mockAPI.Directory(ctx, mode, parsed.Path)
		return map[string]any{"directory": result}, err
	case "list_api_collections":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		result, err := s.mockAPI.Collections(ctx)
		return map[string]any{"collections": result.Collections}, err
	case "get_runtime_context":
		return map[string]any{"runtimeContext": runtimeContext()}, nil
	case "list_audit_logs":
		if s.auditLog == nil {
			return nil, errors.New("audit log service is unavailable")
		}
		page := intFromMap(input, "page", 1)
		pageSize := intFromMap(input, "pageSize", 25)
		if page < 1 {
			page = 1
		}
		if pageSize != 25 && pageSize != 50 && pageSize != 100 {
			return nil, paramError("pageSize must be one of 25, 50, 100")
		}
		return s.auditLog.List(ctx, page, pageSize)
	case "get_audit_log":
		if s.auditLog == nil {
			return nil, errors.New("audit log service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		return s.auditLog.Get(ctx, parsed.ID)
	case "replay_audit_log":
		if s.auditLog == nil {
			return nil, errors.New("audit log service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.auditLog.Get(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		request := mergeMCPReplayRequest(auditlog.ReplayRequest{
			Method:         item.Method,
			TargetURL:      item.TargetURL,
			RequestHeaders: item.RequestHeaders,
			RequestBody:    item.RequestBody,
		}, input)
		if err := auditlog.ValidateReplayRequest(request); err != nil {
			return nil, paramError(err.Error())
		}
		return map[string]any{"item": auditlog.ExecuteReplayRequest(ctx, request)}, nil
	case "send_direct_request":
		request := mergeMCPReplayRequest(auditlog.ReplayRequest{Method: "GET", RequestHeaders: "{}"}, input)
		if strings.TrimSpace(request.TargetURL) == "" {
			return nil, paramError("targetUrl is required")
		}
		if err := auditlog.ValidateReplayRequest(request); err != nil {
			return nil, paramError(err.Error())
		}
		return map[string]any{"item": auditlog.ExecuteReplayRequest(ctx, request)}, nil
	case "list_snapshots":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		items, err := s.mockAPI.ListVersions(ctx)
		return map[string]any{"items": items}, err
	case "get_snapshot":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.GetVersion(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("Snapshot not found")
		}
		return map[string]any{"item": item}, nil
	case "create_snapshot":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		var parsed messageInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		item, err := s.mockAPI.CreateVersion(ctx, parsed.Message)
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-snapshots", Action: "created", Source: "mcp"})
		return map[string]any{"item": item}, nil
	case "restore_snapshot":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.RestoreVersion(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("Snapshot not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "restored", Source: "mcp"})
		return map[string]any{"item": item}, nil
	case "revert_snapshot":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.RevertVersion(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("Snapshot not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "reverted", Source: "mcp"})
		return map[string]any{"item": item}, nil
	case "upsert_api_mock_spec":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		spec, err := parseAPISpec(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.UpsertManual(ctx, manualInput(spec))
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "upserted", Source: "mcp", APIMockID: &item.ID})
		return map[string]any{"item": item}, nil
	case "bulk_upsert_api_mock_specs":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		var bulk bulkAPIInput
		if err := decode(input, &bulk); err != nil {
			return nil, paramError("Invalid params")
		}
		if len(bulk.Items) == 0 {
			return nil, paramError("At least one API mock spec is required")
		}
		if len(bulk.Items) > 100 {
			return nil, paramError("Bulk API mock specs are limited to 100")
		}
		seen := map[string]bool{}
		items := make([]mockapi.APIRecord, 0, len(bulk.Items))
		for _, raw := range bulk.Items {
			spec, err := validateAPISpec(raw)
			if err != nil {
				return nil, err
			}
			key := spec.Method + ":" + spec.RoutePath
			if seen[key] {
				return nil, paramError("Bulk API mock specs must have unique method and route path pairs")
			}
			seen[key] = true
			item, err := s.mockAPI.UpsertManual(ctx, manualInput(spec))
			if err != nil {
				return nil, err
			}
			items = append(items, *item)
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "upserted", Source: "mcp"})
		return map[string]any{"upsertedCount": len(items), "items": items}, nil
	case "update_api_mock_spec":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseUpdateAPIInput(input)
		if err != nil {
			return nil, err
		}
		existing, err := s.mockAPI.Get(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, errors.New("API not found")
		}
		next, err := mergeAPIUpdate(*existing, parsed)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.UpdateManual(ctx, parsed.ID, manualInput(next))
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("API not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "updated", Source: "mcp", APIMockID: &item.ID})
		return map[string]any{"item": item}, nil
	case "set_mock_enabled":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseToggleInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.SetMockEnabled(ctx, parsed.ID, *parsed.Enabled)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("API not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "updated", Source: "mcp", APIMockID: &item.ID})
		return map[string]any{"item": item}, nil
	case "set_proxy_enabled":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseToggleInput(input)
		if err != nil {
			return nil, err
		}
		item, err := s.mockAPI.SetProxyEnabled(ctx, parsed.ID, *parsed.Enabled)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("API not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "updated", Source: "mcp", APIMockID: &item.ID})
		return map[string]any{"item": item}, nil
	case "select_mock_response":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		var parsed selectResponseInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		parsed.ID = normalizeID(parsed.ID)
		if parsed.ID == "" {
			return nil, paramError("id is required")
		}
		if parsed.Status < 100 || parsed.Status > 599 {
			return nil, paramError("status must be a valid HTTP status code")
		}
		item, err := s.mockAPI.SelectResponse(ctx, parsed.ID, parsed.Status)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("API response example not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "updated", Source: "mcp", APIMockID: &item.ID})
		return map[string]any{"item": item}, nil
	case "delete_api_mock":
		if s.mockAPI == nil {
			return nil, errors.New("mock API service is unavailable")
		}
		parsed, err := parseIDInput(input)
		if err != nil {
			return nil, err
		}
		deleted, err := s.mockAPI.Delete(ctx, parsed.ID)
		if err != nil {
			return nil, err
		}
		if !deleted {
			return nil, errors.New("API not found")
		}
		s.emitChanged(realtime.MCPChanged{Resource: "api-mocks", Action: "deleted", Source: "mcp", APIMockID: &parsed.ID})
		return map[string]any{"id": parsed.ID, "deleted": deleted}, nil
	case "get_proxy_config":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		config, err := s.proxyConfig.Get(ctx)
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, err
	case "update_proxy_config":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		var parsed proxyConfigInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		endpoints := make([]proxyconfig.ShadowEndpoint, 0, len(parsed.ShadowEndpoints))
		for _, endpoint := range parsed.ShadowEndpoints {
			enabled := true
			if endpoint.Enabled != nil {
				enabled = *endpoint.Enabled
			}
			endpoints = append(endpoints, proxyconfig.ShadowEndpoint{Name: endpoint.Name, BaseURL: endpoint.BaseURL, Enabled: enabled})
		}
		config, err := s.proxyConfig.Update(ctx, parsed.RealServerBaseURL, endpoints)
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "update_proxy_real_server":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		var parsed struct {
			RealServerBaseURL string `json:"realServerBaseUrl"`
		}
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		config, err := s.proxyConfig.UpdatePartial(ctx, proxyconfig.UpdateInput{RealServerBaseURL: &parsed.RealServerBaseURL})
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "update_shadow_endpoints":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		var parsed shadowEndpointsInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		config, err := s.proxyConfig.UpdatePartial(ctx, proxyconfig.UpdateInput{ShadowEndpoints: &parsed.ShadowEndpoints})
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "update_proxy_header_rules":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		var parsed headerRulesInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		config, err := s.proxyConfig.UpdatePartial(ctx, proxyconfig.UpdateInput{HeaderRules: &parsed.HeaderRules})
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "update_proxy_security":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		var parsed securityInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		if parsed.Security == nil {
			return nil, paramError("security is required")
		}
		config, err := s.proxyConfig.UpdatePartial(ctx, proxyconfig.UpdateInput{Security: parsed.Security})
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "set_proxy_capture_enabled":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		parsed, err := parseEnabledInput(input)
		if err != nil {
			return nil, err
		}
		config, err := s.proxyConfig.UpdatePartial(ctx, proxyconfig.UpdateInput{CaptureEnabled: parsed.Enabled})
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "set_proxy_audit_log_enabled":
		if s.proxyConfig == nil {
			return nil, errors.New("proxy config service is unavailable")
		}
		parsed, err := parseEnabledInput(input)
		if err != nil {
			return nil, err
		}
		config, err := s.proxyConfig.UpdatePartial(ctx, proxyconfig.UpdateInput{AuditLogEnabled: parsed.Enabled})
		if err != nil {
			return nil, err
		}
		s.emitChanged(realtime.MCPChanged{Resource: "proxy-config", Action: "updated", Source: "mcp"})
		return map[string]any{"config": config, "runtimeContext": runtimeContext()}, nil
	case "list_environment_variables":
		if s.env == nil {
			return nil, errors.New("environment service is unavailable")
		}
		variables, err := s.env.List(ctx)
		return map[string]any{"variables": variables}, err
	case "upsert_environment_variable":
		if s.env == nil {
			return nil, errors.New("environment service is unavailable")
		}
		var parsed environmentUpsertInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		item, err := s.env.Upsert(ctx, parsed.Key, parsed.Value)
		if err != nil {
			return nil, err
		}
		variables, _ := s.env.List(ctx)
		s.emitChanged(realtime.MCPChanged{Resource: "environment-variables", Action: "upserted", Source: "mcp", EnvironmentKey: item.Key})
		return map[string]any{"item": item, "variables": variables}, nil
	case "update_environment_variable":
		if s.env == nil {
			return nil, errors.New("environment service is unavailable")
		}
		var parsed environmentUpdateInput
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		parsed.CurrentKey = strings.TrimSpace(parsed.CurrentKey)
		if parsed.CurrentKey == "" {
			return nil, paramError("currentKey is required")
		}
		nextKey := parsed.CurrentKey
		if parsed.Key != nil {
			nextKey = *parsed.Key
		}
		item, err := s.env.Update(ctx, parsed.CurrentKey, nextKey, parsed.Value)
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, errors.New("Environment variable not found")
		}
		variables, _ := s.env.List(ctx)
		s.emitChanged(realtime.MCPChanged{Resource: "environment-variables", Action: "updated", Source: "mcp", EnvironmentKey: item.Key})
		return map[string]any{"item": item, "variables": variables}, nil
	case "delete_environment_variable":
		if s.env == nil {
			return nil, errors.New("environment service is unavailable")
		}
		var parsed struct {
			Key string `json:"key"`
		}
		if err := decode(input, &parsed); err != nil {
			return nil, paramError("Invalid params")
		}
		parsed.Key = strings.TrimSpace(parsed.Key)
		if parsed.Key == "" {
			return nil, paramError("key is required")
		}
		deleted, err := s.env.Delete(ctx, parsed.Key)
		if err != nil {
			return nil, err
		}
		if !deleted {
			return nil, errors.New("Environment variable not found")
		}
		variables, _ := s.env.List(ctx)
		s.emitChanged(realtime.MCPChanged{Resource: "environment-variables", Action: "deleted", Source: "mcp", EnvironmentKey: parsed.Key})
		return map[string]any{"key": parsed.Key, "deleted": deleted, "variables": variables}, nil
	case "get_mcp_config":
		config, err := s.Get(ctx)
		return map[string]any{"config": config}, err
	default:
		return nil, errUnknownTool
	}
}

func createResult(id any, result any) *jsonRPCResponse {
	return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func createMCPError(id any, code int, message string, data ...any) *jsonRPCResponse {
	err := &jsonRPCError{Code: code, Message: message}
	if len(data) > 0 {
		err.Data = data[0]
	}
	return &jsonRPCResponse{JSONRPC: "2.0", ID: id, Error: err}
}

func createMCPSessionID() string {
	id, err := uuidv7.New()
	if err != nil {
		return ""
	}
	return id
}

func toolCallResult(data any) map[string]any {
	payload, _ := json.MarshalIndent(data, "", "  ")
	return map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": string(payload)},
		},
		"structuredContent": data,
	}
}

func decode(input any, target any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func requestID(value any) any {
	switch id := value.(type) {
	case nil:
		return nil
	case string:
		return id
	case json.Number:
		if integer, err := id.Int64(); err == nil {
			return integer
		}
		if number, err := id.Float64(); err == nil {
			return number
		}
	case float64:
		if math.Trunc(id) == id {
			return int64(id)
		}
		return id
	}
	return nil
}

func parseIDInput(input map[string]any) (idInput, error) {
	var parsed idInput
	if err := decode(input, &parsed); err != nil {
		return parsed, paramError("Invalid params")
	}
	parsed.ID = idFromAny(input["id"])
	if parsed.ID == "" {
		return parsed, paramError("id is required")
	}
	return parsed, nil
}

func parseToggleInput(input map[string]any) (toggleInput, error) {
	var parsed toggleInput
	if err := decode(input, &parsed); err != nil {
		return parsed, paramError("Invalid params")
	}
	parsed.ID = idFromAny(input["id"])
	if parsed.ID == "" {
		return parsed, paramError("id is required")
	}
	if parsed.Enabled == nil {
		return parsed, paramError("enabled is required")
	}
	return parsed, nil
}

func parseEnabledInput(input map[string]any) (enabledInput, error) {
	var parsed enabledInput
	if err := decode(input, &parsed); err != nil {
		return parsed, paramError("Invalid params")
	}
	if parsed.Enabled == nil {
		return parsed, paramError("enabled is required")
	}
	return parsed, nil
}

func parseAPISpec(input map[string]any) (apiMockSpecInput, error) {
	var parsed apiMockSpecInput
	if err := decode(input, &parsed); err != nil {
		return parsed, paramError("Invalid params")
	}
	return validateAPISpec(parsed)
}

func parseUpdateAPIInput(input map[string]any) (updateAPIInput, error) {
	var parsed updateAPIInput
	if err := decode(input, &parsed); err != nil {
		return parsed, paramError("Invalid params")
	}
	parsed.ID = idFromAny(input["id"])
	if parsed.ID == "" {
		return parsed, paramError("id is required")
	}
	fields := map[string]bool{}
	for key := range input {
		if key != "id" {
			fields[key] = true
		}
	}
	if len(fields) == 0 {
		return parsed, paramError("At least one API mock field is required")
	}
	parsed.fields = fields
	return parsed, nil
}

func validateAPISpec(input apiMockSpecInput) (apiMockSpecInput, error) {
	input.CollectionName = strings.TrimSpace(input.CollectionName)
	if input.CollectionName == "" {
		input.CollectionName = "MCP"
	}
	if len(input.CollectionName) > 160 {
		return input, paramError("Collection name must be 160 characters or less")
	}
	input.RouteName = strings.TrimSpace(input.RouteName)
	if input.RouteName == "" {
		return input, paramError("Route name is required")
	}
	if len(input.RouteName) > 160 {
		return input, paramError("Route name must be 160 characters or less")
	}
	if len(input.PostmanFolderPath) > 12 {
		return input, paramError("Collection folder path can contain up to 12 segments")
	}
	for _, segment := range input.PostmanFolderPath {
		if len(strings.TrimSpace(segment)) > 120 {
			return input, paramError("Collection folder name must be 120 characters or less")
		}
	}
	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	if !validHTTPMethod(input.Method) {
		return input, paramError("method must be a valid HTTP method")
	}
	input.RoutePath = strings.TrimSpace(input.RoutePath)
	if input.RoutePath == "" {
		return input, paramError("Route path is required")
	}
	if len(input.RoutePath) > 512 {
		return input, paramError("Route path must be 512 characters or less")
	}
	if !strings.HasPrefix(input.RoutePath, "/") {
		return input, paramError("Route path must start with /")
	}
	if len(input.ResponseExamples) == 0 {
		return input, paramError("At least one response example is required")
	}
	if len(input.ResponseExamples) > 10 {
		return input, paramError("Response examples are limited to 10")
	}
	seenStatus := map[int]bool{}
	for index := range input.ResponseExamples {
		example := &input.ResponseExamples[index]
		if example.Status < 100 || example.Status > 599 {
			return input, paramError("response status must be a valid HTTP status code")
		}
		if seenStatus[example.Status] {
			return input, paramError("Response example statuses must be unique")
		}
		seenStatus[example.Status] = true
		if len(strings.TrimSpace(example.Name)) > 120 {
			return input, paramError("Response example name must be 120 characters or less")
		}
		if example.BodyType == "" {
			example.BodyType = "json"
		}
	}
	if len(input.RequestBodyKeys) > 100 {
		return input, paramError("Request body keys are limited to 100")
	}
	if len(input.RequestParamKeys) > 100 {
		return input, paramError("Request query keys are limited to 100")
	}
	if input.RequestBodyTypes == nil {
		input.RequestBodyTypes = mockapi.RequestBodyTypes{}
	}
	for _, value := range input.RequestBodyTypes {
		if !validBodyType(value) {
			return input, paramError("Request body type is invalid")
		}
	}
	return input, nil
}

func mergeAPIUpdate(existing mockapi.APIRecord, update updateAPIInput) (apiMockSpecInput, error) {
	mockEnabled := existing.MockEnabled
	proxyEnabled := existing.ProxyEnabled
	next := apiMockSpecInput{
		CollectionName:    existing.CollectionName,
		RouteName:         existing.RouteName,
		PostmanFolderPath: existing.PostmanFolderPath,
		Method:            existing.Method,
		RoutePath:         existing.RoutePath,
		ResponseExamples:  existing.ResponseExamples,
		RequestBodyKeys:   existing.ExpectedRequestKeys,
		RequestBodyTypes:  existing.ExpectedRequestTypes,
		RequestParamKeys:  existing.ExpectedParamKeys,
		MockEnabled:       &mockEnabled,
		ProxyEnabled:      &proxyEnabled,
	}
	if update.fields["collectionName"] {
		next.CollectionName = update.CollectionName
	}
	if update.fields["routeName"] {
		next.RouteName = update.RouteName
	}
	if update.fields["postmanFolderPath"] {
		next.PostmanFolderPath = update.PostmanFolderPath
	}
	if update.fields["method"] {
		next.Method = update.Method
	}
	if update.fields["routePath"] {
		next.RoutePath = update.RoutePath
	}
	if update.fields["responseExamples"] {
		next.ResponseExamples = update.ResponseExamples
	}
	if update.fields["requestBodyKeys"] {
		next.RequestBodyKeys = update.RequestBodyKeys
	}
	if update.fields["requestBodyTypes"] {
		next.RequestBodyTypes = update.RequestBodyTypes
	}
	if update.fields["requestParamKeys"] {
		next.RequestParamKeys = update.RequestParamKeys
	}
	if update.fields["mockEnabled"] {
		next.MockEnabled = update.MockEnabled
	}
	if update.fields["proxyEnabled"] {
		next.ProxyEnabled = update.ProxyEnabled
	}
	return validateAPISpec(next)
}

func manualInput(input apiMockSpecInput) mockapi.ManualSpecInput {
	mockEnabled := true
	if input.MockEnabled != nil {
		mockEnabled = *input.MockEnabled
	}
	proxyEnabled := true
	if input.ProxyEnabled != nil {
		proxyEnabled = *input.ProxyEnabled
	}
	responseStatus := 200
	if len(input.ResponseExamples) > 0 {
		responseStatus = input.ResponseExamples[0].Status
	}
	return mockapi.ManualSpecInput{
		CollectionName:    input.CollectionName,
		RouteName:         input.RouteName,
		PostmanFolderPath: input.PostmanFolderPath,
		Method:            input.Method,
		RoutePath:         input.RoutePath,
		MockEnabled:       mockEnabled,
		ProxyEnabled:      proxyEnabled,
		ResponseStatus:    responseStatus,
		ResponseExamples:  input.ResponseExamples,
		RequestBodyKeys:   input.RequestBodyKeys,
		RequestBodyTypes:  input.RequestBodyTypes,
		RequestParamKeys:  input.RequestParamKeys,
	}
}

func mergeMCPReplayRequest(base auditlog.ReplayRequest, input map[string]any) auditlog.ReplayRequest {
	if value, ok := input["method"].(string); ok {
		base.Method = value
	}
	if value, ok := input["targetUrl"].(string); ok {
		base.TargetURL = value
	}
	if value, ok := input["requestHeaders"].(string); ok {
		base.RequestHeaders = value
	}
	if value, ok := input["requestBody"].(string); ok {
		base.RequestBody = value
	}
	return base
}

func runtimeContext() map[string]any {
	configured := strings.ToLower(strings.TrimSpace(os.Getenv(appconfig.DefaultTunnelGateRuntimeEnv)))
	runtime := ""
	switch configured {
	case "docker", "container", "1", "true":
		runtime = "docker"
	case "host", "local", "0", "false":
		runtime = "host"
	}
	_, dockerFileErr := os.Stat("/.dockerenv")
	isDocker := runtime == "docker" || (runtime == "" && dockerFileErr == nil)
	if runtime == "" {
		if isDocker {
			runtime = "docker"
		} else {
			runtime = "host"
		}
	}
	loopbackTarget := "host"
	hostMachineHostname := "127.0.0.1"
	if isDocker {
		loopbackTarget = "container"
		hostMachineHostname = "host.docker.internal"
	}
	return map[string]any{
		"runtime":  runtime,
		"isDocker": isDocker,
		"proxyAddressing": map[string]any{
			"loopbackTarget":      loopbackTarget,
			"hostMachineHostname": hostMachineHostname,
		},
	}
}

func intFromMap(input map[string]any, key string, fallback int) int {
	value, ok := input[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		if err == nil {
			return int(parsed)
		}
	case float64:
		return int(typed)
	case int:
		return typed
	case string:
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

func idFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		if math.Trunc(typed) == typed {
			return fmt.Sprintf("%.0f", typed)
		}
		return fmt.Sprintf("%v", typed)
	case int:
		return fmt.Sprintf("%d", typed)
	}
	return ""
}

func normalizeID(id string) string {
	return strings.TrimSpace(id)
}

func validHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func validBodyType(value string) bool {
	switch value {
	case "string", "number", "boolean", "array", "object", "null", "file":
		return true
	default:
		return false
	}
}

func errData(err error) any {
	if err == nil {
		return nil
	}
	return []map[string]string{{"message": err.Error()}}
}

var errUnknownTool = errors.New("unknown tool")

type paramError string

func (e paramError) Error() string { return string(e) }
