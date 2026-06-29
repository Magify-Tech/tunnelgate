package proxyconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/constants"
	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/sanitize"
)

const (
	realServerBaseURLKey   = "real_server_base_url"
	shadowEndpointsKey     = "shadow_endpoints"
	captureEnabledKey      = "proxy_capture_enabled"
	auditLogEnabledKey     = "proxy_audit_log_enabled"
	headerMatchReplaceKey  = "proxy_header_match_replace_rules"
	publicProxySecurityKey = "public_proxy_security"
)

type ShadowEndpoint struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	Enabled bool   `json:"enabled"`
}

type HeaderMatchReplaceRule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Item    string `json:"item"`
	Match   string `json:"match"`
	Replace string `json:"replace"`
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

type PublicProxySecurity struct {
	Enabled            bool                     `json:"enabled"`
	XSS                bool                     `json:"xss"`
	SQLInjection       bool                     `json:"sqlInjection"`
	RateLimit          bool                     `json:"rateLimit"`
	RateLimitPerMinute int                      `json:"rateLimitPerMinute"`
	SecureHeaders      PublicProxySecureHeaders `json:"secureHeaders"`
}

type PublicProxySecureHeaders struct {
	Enabled bool                `json:"enabled"`
	Headers []PublicProxyHeader `json:"headers"`
}

type PublicProxyHeader struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type Config struct {
	RealServerBaseURL string                   `json:"realServerBaseUrl"`
	ShadowEndpoints   []ShadowEndpoint         `json:"shadowEndpoints"`
	CaptureEnabled    bool                     `json:"captureEnabled"`
	AuditLogEnabled   bool                     `json:"auditLogEnabled"`
	HeaderRules       []HeaderMatchReplaceRule `json:"headerRules"`
	Security          PublicProxySecurity      `json:"security"`
	UpdatedAt         *string                  `json:"updatedAt"`
}

type UpdateInput struct {
	RealServerBaseURL *string
	ShadowEndpoints   *[]ShadowEndpoint
	CaptureEnabled    *bool
	AuditLogEnabled   *bool
	HeaderRules       *[]HeaderMatchReplaceRule
	Security          *PublicProxySecurity
}

type Service struct {
	db      *database.Connection
	writeMu sync.Mutex
}

func NewService(db *database.Connection) *Service {
	return &Service{db: db}
}

func (s *Service) Get(ctx context.Context) (Config, error) {
	realValue, realUpdated, err := s.getConfig(ctx, realServerBaseURLKey)
	if err != nil {
		return Config{}, err
	}
	shadowValue, shadowUpdated, err := s.getConfig(ctx, shadowEndpointsKey)
	if err != nil {
		return Config{}, err
	}
	captureValue, captureUpdated, err := s.getConfig(ctx, captureEnabledKey)
	if err != nil {
		return Config{}, err
	}
	auditValue, auditUpdated, err := s.getConfig(ctx, auditLogEnabledKey)
	if err != nil {
		return Config{}, err
	}
	headerValue, headerUpdated, err := s.getConfig(ctx, headerMatchReplaceKey)
	if err != nil {
		return Config{}, err
	}
	securityValue, securityUpdated, err := s.getConfig(ctx, publicProxySecurityKey)
	if err != nil {
		return Config{}, err
	}

	var endpoints []ShadowEndpoint
	if shadowValue != "" {
		_ = json.Unmarshal([]byte(shadowValue), &endpoints)
	}
	for index := range endpoints {
		if endpoints[index].ID == "" {
			endpoints[index].ID = "shadow-" + strconv.Itoa(index+1)
		}
	}
	rules := []HeaderMatchReplaceRule{}
	if headerValue != "" {
		_ = json.Unmarshal([]byte(headerValue), &rules)
	}
	security := defaultPublicProxySecurity()
	if securityValue != "" {
		_ = json.Unmarshal([]byte(securityValue), &security)
		security = normalizeSecurityConfig(security)
	}
	updatedAt := latest(realUpdated, shadowUpdated, captureUpdated, auditUpdated, headerUpdated, securityUpdated)
	return Config{
		RealServerBaseURL: realValue,
		ShadowEndpoints:   endpoints,
		CaptureEnabled:    parseBoolConfig(captureValue, false),
		AuditLogEnabled:   parseBoolConfig(auditValue, true),
		HeaderRules:       rules,
		Security:          security,
		UpdatedAt:         updatedAt,
	}, nil
}

func (s *Service) Update(ctx context.Context, realServerBaseURL string, endpoints []ShadowEndpoint) (Config, error) {
	return s.UpdatePartial(ctx, UpdateInput{RealServerBaseURL: &realServerBaseURL, ShadowEndpoints: &endpoints})
}

func (s *Service) UpdatePartial(ctx context.Context, input UpdateInput) (Config, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	current, err := s.Get(ctx)
	if err != nil {
		return Config{}, err
	}
	if input.RealServerBaseURL != nil {
		current.RealServerBaseURL = *input.RealServerBaseURL
	}
	if input.ShadowEndpoints != nil {
		current.ShadowEndpoints = *input.ShadowEndpoints
	}
	if input.CaptureEnabled != nil {
		current.CaptureEnabled = *input.CaptureEnabled
	}
	if input.AuditLogEnabled != nil {
		current.AuditLogEnabled = *input.AuditLogEnabled
	}
	if input.HeaderRules != nil {
		current.HeaderRules = *input.HeaderRules
	}
	if input.Security != nil {
		current.Security = *input.Security
	}
	return s.save(ctx, current)
}

func (s *Service) Save(ctx context.Context, config Config) (Config, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	return s.save(ctx, config)
}

func (s *Service) save(ctx context.Context, config Config) (Config, error) {
	realServerBaseURL := config.RealServerBaseURL
	realServerBaseURL, err := normalizeURL(realServerBaseURL, "Real server endpoint", true)
	if err != nil {
		return Config{}, err
	}
	endpoints := config.ShadowEndpoints
	proxyDefaults := appconfig.DefaultAppConfig().Proxy
	if len(endpoints) > proxyDefaults.MaxShadowEndpoints {
		endpoints = endpoints[:proxyDefaults.MaxShadowEndpoints]
	}
	normalized := make([]ShadowEndpoint, 0, len(endpoints))
	for index, endpoint := range endpoints {
		name, err := sanitize.Control(strings.TrimSpace(endpoint.Name), "Shadow endpoint name")
		if err != nil {
			return Config{}, err
		}
		if name == "" {
			name = "Shadow " + strconv.Itoa(index+1)
		}
		baseURL, err := normalizeURL(endpoint.BaseURL, "Shadow endpoint", false)
		if err != nil {
			return Config{}, err
		}
		normalized = append(normalized, ShadowEndpoint{ID: "shadow-" + strconv.Itoa(index+1), Name: name, BaseURL: baseURL, Enabled: endpoint.Enabled})
	}
	rules, err := normalizeHeaderRules(config.HeaderRules)
	if err != nil {
		return Config{}, err
	}
	security := normalizeSecurityConfig(config.Security)

	shadowPayload, _ := json.Marshal(normalized)
	headerPayload, _ := json.Marshal(rules)
	securityPayload, _ := json.Marshal(security)
	if err := s.upsertConfig(ctx, realServerBaseURLKey, realServerBaseURL); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, shadowEndpointsKey, string(shadowPayload)); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, captureEnabledKey, strconv.FormatBool(config.CaptureEnabled)); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, auditLogEnabledKey, strconv.FormatBool(config.AuditLogEnabled)); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, headerMatchReplaceKey, string(headerPayload)); err != nil {
		return Config{}, err
	}
	if err := s.upsertConfig(ctx, publicProxySecurityKey, string(securityPayload)); err != nil {
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

func normalizeURL(value, fieldName string, allowEmpty bool) (string, error) {
	cleaned, err := sanitize.Control(strings.TrimSpace(value), fieldName)
	if err != nil {
		return "", err
	}
	cleaned = strings.TrimRight(cleaned, "/")
	if cleaned == "" && allowEmpty {
		return "", nil
	}
	parsed, err := url.Parse(cleaned)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != constants.URLSchemeHTTP && parsed.Scheme != constants.URLSchemeHTTPS) {
		return "", &url.Error{Op: "parse", URL: fieldName, Err: errInvalidURL(fieldName)}
	}
	return cleaned, nil
}

type invalidURLError string

func (e invalidURLError) Error() string { return string(e) }

func errInvalidURL(fieldName string) error {
	return invalidURLError(fieldName + " must be a valid URL")
}

func normalizeHeaderRules(rules []HeaderMatchReplaceRule) ([]HeaderMatchReplaceRule, error) {
	proxyDefaults := appconfig.DefaultAppConfig().Proxy
	if len(rules) > proxyDefaults.MaxHeaderRules {
		rules = rules[:proxyDefaults.MaxHeaderRules]
	}
	normalized := make([]HeaderMatchReplaceRule, 0, len(rules))
	for index, rule := range rules {
		item := normalizeHeaderRuleItem(rule.Item)
		match, err := sanitize.Control(strings.TrimSpace(rule.Match), "Header match")
		if err != nil {
			return nil, err
		}
		replace, err := sanitize.Control(strings.TrimSpace(rule.Replace), "Header replace")
		if err != nil {
			return nil, err
		}
		if match == "" && replace == "" {
			continue
		}
		ruleType := strings.ToLower(strings.TrimSpace(rule.Type))
		if ruleType == "" {
			ruleType = constants.HeaderRuleTypeRegex
		}
		if ruleType != constants.HeaderRuleTypeRegex && ruleType != constants.HeaderRuleTypeLiteral {
			return nil, errors.New("header rule type must be regex or literal")
		}
		if ruleType == constants.HeaderRuleTypeRegex {
			if _, err := regexp.Compile(match); err != nil {
				return nil, errors.New("header rule match must be a valid regex")
			}
		}
		name, err := sanitize.Control(strings.TrimSpace(rule.Name), "Header rule name")
		if err != nil {
			return nil, err
		}
		comment, err := sanitize.Control(strings.TrimSpace(rule.Comment), "Header rule comment")
		if err != nil {
			return nil, err
		}
		if name == "" {
			name = "Header rule " + strconv.Itoa(index+1)
		}
		normalized = append(normalized, HeaderMatchReplaceRule{
			ID:      "header-rule-" + strconv.Itoa(index+1),
			Name:    name,
			Enabled: rule.Enabled,
			Item:    item,
			Match:   match,
			Replace: replace,
			Type:    ruleType,
			Comment: comment,
		})
	}
	return normalized, nil
}

func normalizeHeaderRuleItem(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case constants.HeaderRuleItemResponse, "response header", "response":
		return constants.HeaderRuleItemResponse
	default:
		return constants.HeaderRuleItemRequest
	}
}

func defaultPublicProxySecurity() PublicProxySecurity {
	return PublicProxySecurity{
		RateLimitPerMinute: appconfig.DefaultAppConfig().Proxy.DefaultRateLimitPerMinute,
		SecureHeaders:      defaultPublicProxySecureHeaders(),
	}
}

func normalizeSecurityConfig(config PublicProxySecurity) PublicProxySecurity {
	proxyDefaults := appconfig.DefaultAppConfig().Proxy
	if config.RateLimitPerMinute <= 0 {
		config.RateLimitPerMinute = proxyDefaults.DefaultRateLimitPerMinute
	}
	if config.RateLimitPerMinute > proxyDefaults.MaxRateLimitPerMinute {
		config.RateLimitPerMinute = proxyDefaults.MaxRateLimitPerMinute
	}
	config.SecureHeaders = normalizeSecureHeaders(config.SecureHeaders)
	return config
}

func defaultPublicProxySecureHeaders() PublicProxySecureHeaders {
	return PublicProxySecureHeaders{
		Enabled: false,
		Headers: []PublicProxyHeader{
			{ID: "secure-header-1", Name: "Content-Security-Policy", Value: "default-src 'self'; base-uri 'self'; object-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'self'; upgrade-insecure-requests", Enabled: true},
			{ID: "secure-header-2", Name: "Cross-Origin-Opener-Policy", Value: "same-origin", Enabled: true},
			{ID: "secure-header-3", Name: "Cross-Origin-Resource-Policy", Value: "same-origin", Enabled: true},
			{ID: "secure-header-4", Name: "Origin-Agent-Cluster", Value: "?1", Enabled: true},
			{ID: "secure-header-5", Name: "Permissions-Policy", Value: "camera=(), geolocation=(), microphone=(), payment=(), usb=()", Enabled: true},
			{ID: "secure-header-6", Name: "Referrer-Policy", Value: "no-referrer", Enabled: true},
			{ID: "secure-header-7", Name: "Strict-Transport-Security", Value: "max-age=63072000; includeSubDomains; preload", Enabled: true},
			{ID: "secure-header-8", Name: "X-Content-Type-Options", Value: "nosniff", Enabled: true},
			{ID: "secure-header-9", Name: "X-DNS-Prefetch-Control", Value: "off", Enabled: true},
			{ID: "secure-header-10", Name: "X-Download-Options", Value: "noopen", Enabled: true},
			{ID: "secure-header-11", Name: "X-Frame-Options", Value: "DENY", Enabled: true},
			{ID: "secure-header-12", Name: "X-Permitted-Cross-Domain-Policies", Value: "none", Enabled: true},
			{ID: "secure-header-13", Name: "X-XSS-Protection", Value: "0", Enabled: true},
		},
	}
}

func normalizeSecureHeaders(config PublicProxySecureHeaders) PublicProxySecureHeaders {
	headers := config.Headers
	if len(headers) == 0 && !config.Enabled {
		headers = defaultPublicProxySecureHeaders().Headers
	}
	proxyDefaults := appconfig.DefaultAppConfig().Proxy
	if len(headers) > proxyDefaults.MaxHeaderRules {
		headers = headers[:proxyDefaults.MaxHeaderRules]
	}
	normalized := make([]PublicProxyHeader, 0, len(headers))
	for index, header := range headers {
		name, err := sanitize.Control(strings.TrimSpace(header.Name), "Secure header name")
		if err != nil || name == "" || !isValidHeaderName(name) {
			continue
		}
		value, err := sanitize.Control(strings.TrimSpace(header.Value), "Secure header value")
		if err != nil {
			continue
		}
		normalized = append(normalized, PublicProxyHeader{
			ID:      "secure-header-" + strconv.Itoa(index+1),
			Name:    name,
			Value:   value,
			Enabled: header.Enabled,
		})
	}
	return PublicProxySecureHeaders{Enabled: config.Enabled, Headers: normalized}
}

func isValidHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			continue
		}
		switch char {
		case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
			continue
		default:
			return false
		}
	}
	return true
}

func parseBoolConfig(value string, fallback bool) bool {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
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
