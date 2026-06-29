package config

import "time"

const (
	DefaultLogLevel                     = "info"
	DefaultAdminPort                    = 9200
	DefaultPublicPort                   = 9280
	DefaultCORSAllowCredentials         = false
	DefaultUpgradeInsecureRequests      = false
	DefaultHSTSPreload                  = false
	DefaultDatabaseURL                  = "file:./data/mock-engine.sqlite"
	DefaultSQLProvider                  = "sqlite"
	DefaultAuditLogRetentionDays        = 7
	DefaultAuditLogRetentionIntervalMin = 60
	DefaultTunnelGateRuntimeEnv         = "TUNNEL_GATE"
)

var DefaultCORSAllowList = []string{"http://localhost:5173"}

type Config struct {
	LogLevel                     string
	AdminPort                    int
	PublicPort                   int
	CORSAllowList                []string
	CORSAllowCredentials         bool
	UpgradeInsecureRequests      bool
	HSTSPreload                  bool
	DatabaseURL                  string
	SQLProvider                  string
	AuditDatabaseURL             string
	AuditSQLProvider             string
	AdminFeaturePlugins          []string
	PublicFeaturePlugins         []string
	AuditLogRetentionDays        int
	AuditLogRetentionIntervalMin int
}

type AppConfig struct {
	MockAPI     MockAPIConfig
	AuditLog    AuditLogConfig
	Environment EnvironmentConfig
	Proxy       ProxyConfig
}

type MockAPIConfig struct {
	UploadMaxBytes  int64
	PageSizeDefault int
	PageSizeOptions []int
}

type AuditLogConfig struct {
	PageSizeDefault      int
	PageSizeOptions      []int
	ReplayHTTPClient     HTTPClientConfig
	ResponsePreviewBytes int
	BinaryPreviewBytes   int
	BinaryTruncateMarker string
	HexBodyPrefix        string
}

type EnvironmentConfig struct {
	UploadMaxBytes int64
}

type ProxyConfig struct {
	DefaultRateLimitPerMinute int
	MaxRateLimitPerMinute     int
	MaxShadowEndpoints        int
	MaxHeaderRules            int
	ShadowRequestTimeout      time.Duration
	HTTPClient                HTTPClientConfig
	RequestPreviewBytes       int
	BinaryPreviewBytes        int
	ResponsePreviewBytes      int
	BinaryTruncateMarker      string
	HexBodyPrefix             string
}

type HTTPClientConfig struct {
	Timeout               time.Duration
	DialTimeout           time.Duration
	DialKeepAlive         time.Duration
	DialFallbackDelay     time.Duration
	ExpectContinueTimeout time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
}

func Load() Config {
	databaseURL := env("DATABASE_URL", DefaultDatabaseURL)
	auditDatabaseURL := env("AUDIT_DATABASE_URL", databaseURL)
	sqlProvider := env("SQL_PROVIDER", DefaultSQLProvider)
	auditSQLProvider := env("AUDIT_SQL_PROVIDER", sqlProvider)

	return Config{
		LogLevel:                     env("LOG_LEVEL", DefaultLogLevel),
		AdminPort:                    envInt("ADMIN_PORT", DefaultAdminPort),
		PublicPort:                   envInt("APP_PORT", DefaultPublicPort),
		CORSAllowList:                envCSV("CORS", DefaultCORSAllowList),
		CORSAllowCredentials:         envBool("CORS_ALLOW_CREDENTIALS", DefaultCORSAllowCredentials),
		UpgradeInsecureRequests:      envBool("UPGRADE_INSECURE_REQUESTS", DefaultUpgradeInsecureRequests),
		HSTSPreload:                  envBool("HSTS_PRELOAD", DefaultHSTSPreload),
		DatabaseURL:                  databaseURL,
		SQLProvider:                  sqlProvider,
		AuditDatabaseURL:             auditDatabaseURL,
		AdminFeaturePlugins:          envCSV("ADMIN_FEATURE_PLUGINS", nil),
		PublicFeaturePlugins:         envCSV("PUBLIC_FEATURE_PLUGINS", nil),
		AuditSQLProvider:             auditSQLProvider,
		AuditLogRetentionDays:        envInt("AUDIT_LOG_RETENTION_DAYS", DefaultAuditLogRetentionDays),
		AuditLogRetentionIntervalMin: envInt("AUDIT_LOG_RETENTION_INTERVAL_MINUTES", DefaultAuditLogRetentionIntervalMin),
	}
}

func DefaultAppConfig() AppConfig {
	pageSizeOptions := []int{25, 50, 100}
	return AppConfig{
		MockAPI: MockAPIConfig{
			UploadMaxBytes:  16 * 1024 * 1024,
			PageSizeDefault: 25,
			PageSizeOptions: append([]int{}, pageSizeOptions...),
		},
		AuditLog: AuditLogConfig{
			PageSizeDefault:      25,
			PageSizeOptions:      append([]int{}, pageSizeOptions...),
			ReplayHTTPClient:     HTTPClientConfig{Timeout: 30 * time.Second, DialTimeout: 10 * time.Second, DialKeepAlive: 30 * time.Second, DialFallbackDelay: 50 * time.Millisecond, MaxIdleConns: 128, MaxIdleConnsPerHost: 32},
			ResponsePreviewBytes: 64 * 1024,
			BinaryPreviewBytes:   8 * 1024,
			BinaryTruncateMarker: "[truncated ",
			HexBodyPrefix:        "0x",
		},
		Environment: EnvironmentConfig{
			UploadMaxBytes: 16 * 1024 * 1024,
		},
		Proxy: ProxyConfig{
			DefaultRateLimitPerMinute: 120,
			MaxRateLimitPerMinute:     10000,
			MaxShadowEndpoints:        4,
			MaxHeaderRules:            50,
			ShadowRequestTimeout:      10 * time.Second,
			HTTPClient:                HTTPClientConfig{Timeout: 30 * time.Second, DialTimeout: 10 * time.Second, DialKeepAlive: 30 * time.Second, DialFallbackDelay: 50 * time.Millisecond, MaxIdleConns: 256, MaxIdleConnsPerHost: 64},
			RequestPreviewBytes:       64 * 1024,
			BinaryPreviewBytes:        8 * 1024,
			ResponsePreviewBytes:      64 * 1024,
			BinaryTruncateMarker:      "[truncated ",
			HexBodyPrefix:             "0x",
		},
	}
}
