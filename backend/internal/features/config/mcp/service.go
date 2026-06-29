package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
	"time"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/constants"
	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/internal/features/realtime"
	"postman-transform/backend-golang/internal/sanitize"
)

const (
	enabledKey        = "mcp_enabled"
	clientTokenKey    = "mcp_client_token"
	allowedOriginsKey = "mcp_allowed_origins"
)

type Config struct {
	Enabled        bool     `json:"enabled"`
	ClientToken    string   `json:"clientToken"`
	AllowedOrigins []string `json:"allowedOrigins"`
	UpdatedAt      *string  `json:"updatedAt"`
}

type Service struct {
	db            *database.Connection
	mockAPI       *mockapi.Service
	auditLog      *auditlog.Service
	env           *environment.Service
	proxyConfig   *proxyconfig.Service
	hub           *realtime.Hub
	corsAllowList []string
	writeMu       sync.Mutex
}

type Dependencies struct {
	MockAPI       *mockapi.Service
	AuditLog      *auditlog.Service
	Environment   *environment.Service
	ProxyConfig   *proxyconfig.Service
	Hub           *realtime.Hub
	CORSAllowList []string
}

func NewService(db *database.Connection, dependencies ...Dependencies) *Service {
	deps := Dependencies{}
	if len(dependencies) > 0 {
		deps = dependencies[0]
	}
	corsAllowList := deps.CORSAllowList
	if len(corsAllowList) == 0 {
		corsAllowList = appconfig.Load().CORSAllowList
	}
	return &Service{
		db:            db,
		mockAPI:       deps.MockAPI,
		auditLog:      deps.AuditLog,
		env:           deps.Environment,
		proxyConfig:   deps.ProxyConfig,
		hub:           deps.Hub,
		corsAllowList: corsAllowList,
	}
}

func (s *Service) Get(ctx context.Context) (Config, error) {
	enabled, enabledUpdated, err := s.getConfig(ctx, enabledKey)
	if err != nil {
		return Config{}, err
	}
	token, tokenUpdated, err := s.getConfig(ctx, clientTokenKey)
	if err != nil {
		return Config{}, err
	}
	originsRaw, originsUpdated, err := s.getConfig(ctx, allowedOriginsKey)
	if err != nil {
		return Config{}, err
	}
	origins := []string{}
	_ = json.Unmarshal([]byte(originsRaw), &origins)
	return Config{Enabled: enabled == "true", ClientToken: token, AllowedOrigins: origins, UpdatedAt: latest(enabledUpdated, tokenUpdated, originsUpdated)}, nil
}

func (s *Service) Update(ctx context.Context, enabled bool, token string, origins []string) (Config, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	cleanToken, err := sanitize.Control(strings.TrimSpace(token), "MCP client token")
	if err != nil {
		return Config{}, err
	}
	if len(cleanToken) > 256 {
		return Config{}, invalid("MCP client token must be 256 characters or less")
	}
	unique := map[string]bool{}
	normalizedOrigins := []string{}
	for _, origin := range origins {
		cleaned, err := sanitize.Control(strings.TrimSpace(origin), "MCP allowed origin")
		if err != nil {
			return Config{}, err
		}
		parsed, err := url.Parse(cleaned)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != constants.URLSchemeHTTP && parsed.Scheme != constants.URLSchemeHTTPS) {
			return Config{}, invalid("MCP allowed origin must be a valid URL")
		}
		normalized := parsed.Scheme + "://" + parsed.Host
		if !unique[normalized] {
			unique[normalized] = true
			normalizedOrigins = append(normalizedOrigins, normalized)
		}
	}
	payload, _ := json.Marshal(normalizedOrigins)
	if err := s.upsertConfig(ctx, enabledKey, map[bool]string{true: "true", false: "false"}[enabled]); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, clientTokenKey, cleanToken); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, allowedOriginsKey, string(payload)); err != nil {
		return Config{}, err
	}
	return s.Get(ctx)
}

func (s *Service) getConfig(ctx context.Context, key string) (string, *string, error) {
	var value string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT value, updated_at FROM engine_config WHERE `+database.KeyColumn(s.db.Provider)+` = ?`, key).Scan(&value, &updatedAt)
	if err == sql.ErrNoRows {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return value, &updatedAt, nil
}

func (s *Service) upsertConfig(ctx context.Context, key, value string) error {
	return s.db.UpsertKeyValue(ctx, "engine_config", key, value, time.Now().UTC().Format("2006-01-02 15:04:05"))
}

type invalid string

func (e invalid) Error() string { return string(e) }

func (s *Service) isOriginAllowed(origin string, allowedOrigins []string) bool {
	if origin == "" {
		return true
	}
	configuredOrigins := allowedOrigins
	if len(configuredOrigins) == 0 {
		configuredOrigins = s.corsAllowList
	}
	for _, configured := range configuredOrigins {
		if origin == configured {
			return true
		}
	}
	return false
}

func isTokenAllowed(authorization, mcpToken, configuredToken string) bool {
	if configuredToken == "" {
		return true
	}
	return authorization == "Bearer "+configuredToken || mcpToken == configuredToken
}

func (s *Service) emitChanged(event realtime.MCPChanged) {
	if s == nil || s.hub == nil {
		return
	}
	event.CreatedAt = realtime.Now()
	s.hub.Broadcast(realtime.EventMCPChanged, event)
}

func latest(values ...*string) *string {
	var latestValue *string
	for _, value := range values {
		if value == nil {
			continue
		}
		if latestValue == nil || *value > *latestValue {
			copyValue := *value
			latestValue = &copyValue
		}
	}
	return latestValue
}
