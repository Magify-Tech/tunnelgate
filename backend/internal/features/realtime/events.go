package realtime

import "time"

const (
	EventProxyAuditLogCreated       = "proxy-audit-log:created"
	EventProxyAuditLogShadowCreated = "proxy-audit-log:shadow-created"
	EventImportedAPIActivity        = "imported-api:activity"
	EventMCPChanged                 = "mcp:changed"
)

type ImportedAPIActivity struct {
	APIMockID      string `json:"apiMockId"`
	RouteName      string `json:"routeName"`
	Method         string `json:"method"`
	RoutePath      string `json:"routePath"`
	ResponseStatus *int   `json:"responseStatus"`
	Success        bool   `json:"success"`
	Source         string `json:"source"`
	CreatedAt      string `json:"createdAt"`
}

type MCPChanged struct {
	Resource       string  `json:"resource"`
	Action         string  `json:"action"`
	Source         string  `json:"source"`
	APIMockID      *string `json:"apiMockId,omitempty"`
	EnvironmentKey string  `json:"environmentKey,omitempty"`
	CreatedAt      string  `json:"createdAt"`
}

type ProxyAuditLogShadowCreated struct {
	AuditLogID  string `json:"auditLogId"`
	ShadowEntry any    `json:"shadowEntry"`
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}
