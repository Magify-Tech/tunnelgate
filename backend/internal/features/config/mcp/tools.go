package mcp

func tools() []mcpTool {
	return []mcpTool{
		{
			Name:        "list_api_mocks",
			Title:       "List API mocks",
			Description: "List API mock specs with mock and proxy status.",
			InputSchema: objectSchema(map[string]any{
				"page":     map[string]any{"type": "integer", "minimum": 1, "default": 1},
				"pageSize": map[string]any{"type": "integer", "enum": []int{25, 50, 100}, "default": 25},
			}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "get_api_mock",
			Title:       "Get API mock",
			Description: "Read one API mock spec by ID.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema()}, []string{"id"}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "list_api_directory",
			Title:       "List API directory",
			Description: "Read API specs grouped by collection folder path or API route path.",
			InputSchema: objectSchema(map[string]any{
				"mode": map[string]any{"type": "string", "enum": []string{"postman", "api"}, "default": "postman"},
				"path": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "list_api_collections",
			Title:       "List API collections",
			Description: "Read imported API collection names.",
			InputSchema: objectSchema(map[string]any{}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "get_runtime_context",
			Title:       "Get runtime context",
			Description: "Read whether the API server is running in Docker or directly on the host.",
			InputSchema: objectSchema(map[string]any{}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "list_audit_logs",
			Title:       "List audit logs",
			Description: "Read proxy audit logs without replaying or mutating requests.",
			InputSchema: objectSchema(map[string]any{
				"page":     map[string]any{"type": "integer", "minimum": 1, "default": 1},
				"pageSize": map[string]any{"type": "integer", "enum": []int{25, 50, 100}, "default": 25},
			}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "get_audit_log",
			Title:       "Get audit log",
			Description: "Read one proxy audit log, including shadow entries and current API Spec examples.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema()}, []string{"id"}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "replay_audit_log",
			Title:       "Replay audit log",
			Description: "Replay an existing proxy audit log request with optional request overrides.",
			InputSchema: replayRequestSchema(map[string]any{"id": idSchema()}, []string{"id"}),
		},
		{
			Name:        "send_direct_request",
			Title:       "Send direct request",
			Description: "Send a direct HTTP request without reading or writing audit logs.",
			InputSchema: replayRequestSchema(map[string]any{}, []string{"targetUrl"}),
		},
		{
			Name:        "list_snapshots",
			Title:       "List snapshots",
			Description: "Read saved project API snapshots.",
			InputSchema: objectSchema(map[string]any{}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "get_snapshot",
			Title:       "Get snapshot",
			Description: "Read one saved project API snapshot.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema()}, []string{"id"}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "create_snapshot",
			Title:       "Create snapshot",
			Description: "Create a project API snapshot.",
			InputSchema: objectSchema(map[string]any{"message": map[string]any{"type": "string"}}),
		},
		{
			Name:        "restore_snapshot",
			Title:       "Restore snapshot",
			Description: "Restore current API specs from a snapshot.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema()}, []string{"id"}),
		},
		{
			Name:        "revert_snapshot",
			Title:       "Revert snapshot",
			Description: "Restore a snapshot and create a new snapshot for the reverted state.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema()}, []string{"id"}),
		},
		{
			Name:        "upsert_api_mock_spec",
			Title:       "Upsert API mock spec",
			Description: "Create or replace one API mock spec by HTTP method and route path.",
			InputSchema: apiMockSpecSchema([]string{"routeName", "method", "routePath", "responseExamples"}),
		},
		{
			Name:        "bulk_upsert_api_mock_specs",
			Title:       "Bulk upsert API mock specs",
			Description: "Create or replace multiple API mock specs by HTTP method and route path.",
			InputSchema: objectSchemaRequired(map[string]any{
				"items": map[string]any{"type": "array", "minItems": 1, "maxItems": 100, "items": apiMockSpecSchema([]string{"routeName", "method", "routePath", "responseExamples"})},
			}, []string{"items"}),
		},
		{
			Name:        "update_api_mock_spec",
			Title:       "Update API mock spec",
			Description: "Update fields on an existing API mock spec by ID.",
			InputSchema: apiMockSpecSchemaWithID(),
		},
		{
			Name:        "set_mock_enabled",
			Title:       "Set mock status",
			Description: "Enable or disable mock responses for an API mock.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema(), "enabled": map[string]any{"type": "boolean"}}, []string{"id", "enabled"}),
		},
		{
			Name:        "set_proxy_enabled",
			Title:       "Set proxy status",
			Description: "Enable or disable proxy forwarding for an API mock.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema(), "enabled": map[string]any{"type": "boolean"}}, []string{"id", "enabled"}),
		},
		{
			Name:        "select_mock_response",
			Title:       "Select mock response",
			Description: "Select the active response example status for an API mock.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema(), "status": map[string]any{"type": "integer", "minimum": 100, "maximum": 599}}, []string{"id", "status"}),
		},
		{
			Name:        "delete_api_mock",
			Title:       "Delete API mock",
			Description: "Delete an API mock spec.",
			InputSchema: objectSchemaRequired(map[string]any{"id": idSchema()}, []string{"id"}),
			Annotations: map[string]any{"destructiveHint": true},
		},
		{
			Name:        "get_proxy_config",
			Title:       "Get proxy config",
			Description: "Read real server and shadow endpoint proxy config.",
			InputSchema: objectSchema(map[string]any{}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "update_proxy_config",
			Title:       "Update proxy config",
			Description: "Update real server and shadow endpoint proxy config.",
			InputSchema: objectSchemaRequired(map[string]any{
				"realServerBaseUrl": map[string]any{"type": "string"},
				"shadowEndpoints": map[string]any{"type": "array", "items": objectSchemaRequired(map[string]any{
					"name":    map[string]any{"type": "string"},
					"baseUrl": map[string]any{"type": "string"},
					"enabled": map[string]any{"type": "boolean"},
				}, []string{"baseUrl"})},
			}, []string{"realServerBaseUrl"}),
		},
		{
			Name:        "update_proxy_real_server",
			Title:       "Update proxy real server",
			Description: "Update the real server endpoint.",
			InputSchema: objectSchemaRequired(map[string]any{"realServerBaseUrl": map[string]any{"type": "string"}}, []string{"realServerBaseUrl"}),
		},
		{
			Name:        "update_shadow_endpoints",
			Title:       "Update shadow endpoints",
			Description: "Replace configured shadow endpoints.",
			InputSchema: objectSchemaRequired(map[string]any{
				"shadowEndpoints": shadowEndpointArraySchema(),
			}, []string{"shadowEndpoints"}),
		},
		{
			Name:        "update_proxy_header_rules",
			Title:       "Update proxy header rules",
			Description: "Replace HTTP match and replace rules.",
			InputSchema: objectSchemaRequired(map[string]any{
				"headerRules": map[string]any{"type": "array", "items": headerRuleSchema()},
			}, []string{"headerRules"}),
		},
		{
			Name:        "update_proxy_security",
			Title:       "Update proxy security",
			Description: "Replace public proxy security settings.",
			InputSchema: objectSchemaRequired(map[string]any{
				"security": securitySchema(),
			}, []string{"security"}),
		},
		{
			Name:        "set_proxy_capture_enabled",
			Title:       "Set proxy capture status",
			Description: "Enable or disable proxy capture mode.",
			InputSchema: objectSchemaRequired(map[string]any{"enabled": map[string]any{"type": "boolean"}}, []string{"enabled"}),
		},
		{
			Name:        "set_proxy_audit_log_enabled",
			Title:       "Set proxy audit log status",
			Description: "Enable or disable proxy audit-log persistence.",
			InputSchema: objectSchemaRequired(map[string]any{"enabled": map[string]any{"type": "boolean"}}, []string{"enabled"}),
		},
		{
			Name:        "list_environment_variables",
			Title:       "List environment variables",
			Description: "List stored collection environment variables.",
			InputSchema: objectSchema(map[string]any{}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
		{
			Name:        "upsert_environment_variable",
			Title:       "Upsert environment variable",
			Description: "Create or replace an environment variable.",
			InputSchema: objectSchemaRequired(map[string]any{"key": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}}, []string{"key", "value"}),
		},
		{
			Name:        "update_environment_variable",
			Title:       "Update environment variable",
			Description: "Rename or update an existing environment variable.",
			InputSchema: objectSchemaRequired(map[string]any{"currentKey": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}}, []string{"currentKey", "value"}),
		},
		{
			Name:        "delete_environment_variable",
			Title:       "Delete environment variable",
			Description: "Delete an environment variable.",
			InputSchema: objectSchemaRequired(map[string]any{"key": map[string]any{"type": "string"}}, []string{"key"}),
			Annotations: map[string]any{"destructiveHint": true},
		},
		{
			Name:        "get_mcp_config",
			Title:       "Get MCP config",
			Description: "Read MCP enablement, token, and origin settings.",
			InputSchema: objectSchema(map[string]any{}),
			Annotations: map[string]any{"readOnlyHint": true},
		},
	}
}

func objectSchema(properties map[string]any) map[string]any {
	return objectSchemaRequired(properties, []string{})
}

func objectSchemaRequired(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func responseExampleSchema() map[string]any {
	return objectSchemaRequired(map[string]any{
		"status": map[string]any{"type": "integer", "minimum": 100, "maximum": 599},
		"name":   map[string]any{"type": "string"},
		"body":   map[string]any{"type": "string"},
	}, []string{"status", "body"})
}

func replayRequestSchema(properties map[string]any, required []string) map[string]any {
	for key, value := range map[string]any{
		"method":         map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}, "default": "GET"},
		"targetUrl":      map[string]any{"type": "string"},
		"requestHeaders": map[string]any{"type": "string", "default": "{}"},
		"requestBody":    map[string]any{"type": "string"},
	} {
		properties[key] = value
	}
	return objectSchemaRequired(properties, required)
}

func apiMockSpecSchema(required []string) map[string]any {
	return objectSchemaRequired(map[string]any{
		"collectionName":    map[string]any{"type": "string"},
		"routeName":         map[string]any{"type": "string"},
		"postmanFolderPath": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"method":            map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}},
		"routePath":         map[string]any{"type": "string"},
		"responseExamples":  map[string]any{"type": "array", "items": responseExampleSchema()},
		"requestBodyKeys":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"requestBodyTypes":  map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string", "enum": []string{"string", "number", "boolean", "array", "object", "null", "file"}}},
		"requestParamKeys":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"mockEnabled":       map[string]any{"type": "boolean"},
		"proxyEnabled":      map[string]any{"type": "boolean"},
	}, required)
}

func apiMockSpecSchemaWithID() map[string]any {
	schema := apiMockSpecSchema([]string{"id"})
	properties := schema["properties"].(map[string]any)
	properties["id"] = idSchema()
	return schema
}

func idSchema() map[string]any {
	return map[string]any{"oneOf": []map[string]any{{"type": "string"}, {"type": "integer", "minimum": 1}}}
}

func shadowEndpointArraySchema() map[string]any {
	return map[string]any{"type": "array", "items": objectSchemaRequired(map[string]any{
		"id":      map[string]any{"type": "string"},
		"name":    map[string]any{"type": "string"},
		"baseUrl": map[string]any{"type": "string"},
		"enabled": map[string]any{"type": "boolean"},
	}, []string{"baseUrl"})}
}

func headerRuleSchema() map[string]any {
	return objectSchemaRequired(map[string]any{
		"id":      map[string]any{"type": "string"},
		"name":    map[string]any{"type": "string"},
		"enabled": map[string]any{"type": "boolean"},
		"item":    map[string]any{"type": "string"},
		"match":   map[string]any{"type": "string"},
		"replace": map[string]any{"type": "string"},
		"type":    map[string]any{"type": "string", "enum": []string{"regex", "text"}},
		"comment": map[string]any{"type": "string"},
	}, []string{"item", "match", "replace", "type"})
}

func securitySchema() map[string]any {
	return objectSchemaRequired(map[string]any{
		"enabled":            map[string]any{"type": "boolean"},
		"xss":                map[string]any{"type": "boolean"},
		"sqlInjection":       map[string]any{"type": "boolean"},
		"rateLimit":          map[string]any{"type": "boolean"},
		"rateLimitPerMinute": map[string]any{"type": "integer", "minimum": 1},
		"secureHeaders": objectSchemaRequired(map[string]any{
			"enabled": map[string]any{"type": "boolean"},
			"headers": map[string]any{"type": "array", "items": objectSchemaRequired(map[string]any{
				"id":      map[string]any{"type": "string"},
				"name":    map[string]any{"type": "string"},
				"value":   map[string]any{"type": "string"},
				"enabled": map[string]any{"type": "boolean"},
			}, []string{"name", "value"})},
		}, []string{"enabled", "headers"}),
	}, []string{"enabled", "xss", "sqlInjection", "rateLimit", "rateLimitPerMinute", "secureHeaders"})
}
