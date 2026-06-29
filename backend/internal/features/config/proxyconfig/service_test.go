package proxyconfig

import (
	"context"
	"testing"

	"postman-transform/backend-golang/internal/database"
)

func TestUpdateNormalizesProxyConfig(t *testing.T) {
	db := testDB(t)
	service := NewService(db)

	config, err := service.Update(context.Background(), "http://example.test/base///", []ShadowEndpoint{{Name: "", BaseURL: "https://shadow.test///", Enabled: true}})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if config.RealServerBaseURL != "http://example.test/base" {
		t.Fatalf("unexpected real server URL: %s", config.RealServerBaseURL)
	}
	if len(config.ShadowEndpoints) != 1 || config.ShadowEndpoints[0].ID != "shadow-1" || config.ShadowEndpoints[0].Name != "Shadow 1" || config.ShadowEndpoints[0].BaseURL != "https://shadow.test" {
		t.Fatalf("unexpected shadow endpoint: %#v", config.ShadowEndpoints)
	}
}

func TestUpdatePartialPreservesAdvancedProxyConfig(t *testing.T) {
	db := testDB(t)
	service := NewService(db)

	captureEnabled := true
	auditEnabled := false
	security := PublicProxySecurity{
		Enabled:            true,
		XSS:                true,
		SQLInjection:       true,
		RateLimit:          true,
		RateLimitPerMinute: 10,
		SecureHeaders: PublicProxySecureHeaders{
			Enabled: true,
			Headers: []PublicProxyHeader{
				{Name: "X-Frame-Options", Value: "DENY", Enabled: true},
			},
		},
	}
	rules := []HeaderMatchReplaceRule{{
		Enabled: true,
		Item:    "Request header",
		Match:   "^User-Agent:.*$",
		Replace: "User-Agent: Capture",
		Type:    "Regex",
	}}

	_, err := service.UpdatePartial(context.Background(), UpdateInput{
		CaptureEnabled:  &captureEnabled,
		AuditLogEnabled: &auditEnabled,
		HeaderRules:     &rules,
		Security:        &security,
	})
	if err != nil {
		t.Fatalf("UpdatePartial returned error: %v", err)
	}

	realServer := "http://example.test"
	_, err = service.UpdatePartial(context.Background(), UpdateInput{RealServerBaseURL: &realServer})
	if err != nil {
		t.Fatalf("UpdatePartial real server returned error: %v", err)
	}

	config, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !config.CaptureEnabled || config.AuditLogEnabled {
		t.Fatalf("advanced booleans were not preserved: %#v", config)
	}
	if len(config.HeaderRules) != 1 || config.HeaderRules[0].Item != "request_header" || config.HeaderRules[0].Type != "regex" {
		t.Fatalf("header rule was not normalized and preserved: %#v", config.HeaderRules)
	}
	if !config.Security.Enabled || !config.Security.XSS || !config.Security.SQLInjection || !config.Security.RateLimit || config.Security.RateLimitPerMinute != 10 {
		t.Fatalf("security config was not preserved: %#v", config.Security)
	}
	if !config.Security.SecureHeaders.Enabled || len(config.Security.SecureHeaders.Headers) != 1 || config.Security.SecureHeaders.Headers[0].Name != "X-Frame-Options" {
		t.Fatalf("secure headers config was not preserved: %#v", config.Security.SecureHeaders)
	}
}

func TestDefaultSecureHeadersAreDisabled(t *testing.T) {
	db := testDB(t)
	service := NewService(db)

	config, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if config.Security.SecureHeaders.Enabled {
		t.Fatal("secure headers should be disabled by default")
	}
	if len(config.Security.SecureHeaders.Headers) == 0 {
		t.Fatal("default secure headers should be visible in config")
	}
}

func TestUpdatePartialRejectsInvalidHeaderRegex(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	rules := []HeaderMatchReplaceRule{{Enabled: true, Match: "[", Replace: "x", Type: "regex"}}

	if _, err := service.UpdatePartial(context.Background(), UpdateInput{HeaderRules: &rules}); err == nil {
		t.Fatal("expected invalid regex error")
	}
}

func testDB(t *testing.T) *database.Connection {
	t.Helper()
	db, err := database.Open("file:"+t.TempDir()+"/test.sqlite", "sqlite")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := database.EnsureConnectionSchema(db); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
