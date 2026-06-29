package publicproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"postman-transform/backend-golang/internal/features/config/proxyconfig"
)

func TestTargetURLPreservesBasePathAndQuery(t *testing.T) {
	got := targetURL("http://real.test/api", "/users?active=true")
	want := "http://real.test/api/users?active=true"
	if got != want {
		t.Fatalf("targetURL = %q, want %q", got, want)
	}
}

func TestParseStoredResponseInfersContentType(t *testing.T) {
	_, contentType := parseStoredResponse(`<soapenv:Envelope></soapenv:Envelope>`)
	if contentType != "application/soap+xml; charset=utf-8" {
		t.Fatalf("unexpected content type: %s", contentType)
	}

	_, contentType = parseStoredResponse(`{"ok":true}`)
	if contentType != "application/json" {
		t.Fatalf("unexpected JSON content type: %s", contentType)
	}
}

func TestCopyProxyHeadersDropsAcceptEncoding(t *testing.T) {
	source := http.Header{
		"Accept-Encoding": {"gzip, deflate, br"},
		"Expect":          {"100-continue"},
		"Authorization":   {"Bearer token"},
	}
	target := http.Header{}

	copyProxyHeaders(target, source)

	if target.Get("Accept-Encoding") != "" {
		t.Fatalf("Accept-Encoding should not be forwarded, got %q", target.Get("Accept-Encoding"))
	}
	if target.Get("Expect") != "" {
		t.Fatalf("Expect should not be forwarded, got %q", target.Get("Expect"))
	}
	if target.Get("Authorization") != "Bearer token" {
		t.Fatalf("Authorization header was not forwarded")
	}
}

func TestIsWebSocketUpgradeRequiresUpgradeHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://proxy.test/socket", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Connection", "keep-alive, Upgrade")
	req.Header.Set("Upgrade", "websocket")

	if !isWebSocketUpgrade(req) {
		t.Fatal("expected WebSocket upgrade request")
	}

	req.Header.Del("Connection")
	if isWebSocketUpgrade(req) {
		t.Fatal("request without Connection upgrade token should not be treated as WebSocket")
	}
}

func TestApplySecureHeadersKeepsPassthroughWhenDisabled(t *testing.T) {
	headers := http.Header{
		"X-Request-Id": {"abc"},
	}

	applySecureHeaders(headers, proxyconfig.PublicProxySecureHeaders{})

	if got := headers.Get("X-Request-Id"); got != "abc" {
		t.Fatalf("X-Request-Id = %q, want abc", got)
	}
}

func TestApplySecureHeadersOnlyReplacesConfiguredHeaders(t *testing.T) {
	headers := http.Header{
		"Strict-Transport-Security": {"max-age=1"},
		"X-Frame-Options":           {"SAMEORIGIN"},
		"X-Request-Id":              {"abc"},
	}

	applySecureHeaders(headers, proxyconfig.PublicProxySecureHeaders{
		Enabled: true,
		Headers: []proxyconfig.PublicProxyHeader{
			{Name: "Strict-Transport-Security", Value: "max-age=63072000", Enabled: true},
			{Name: "X-Frame-Options", Value: "DENY", Enabled: false},
		},
	})

	if got := headers.Values("Strict-Transport-Security"); len(got) != 1 || got[0] != "max-age=63072000" {
		t.Fatalf("unexpected Strict-Transport-Security values: %v", got)
	}
	if got := headers.Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Fatalf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
	if got := headers.Get("X-Request-Id"); got != "abc" {
		t.Fatalf("X-Request-Id = %q, want abc", got)
	}
}

func TestApplyHeaderRulesRewritesRequestHeaders(t *testing.T) {
	headers := http.Header{"User-Agent": {"Original"}, "Authorization": {"Bearer token"}}
	rules := []proxyconfig.HeaderMatchReplaceRule{{
		Enabled: true,
		Item:    "request_header",
		Match:   "^User-Agent:.*$",
		Replace: "User-Agent: Capture",
		Type:    "regex",
	}}

	applyHeaderRules(headers, rules, "request_header")

	if got := headers.Get("User-Agent"); got != "Capture" {
		t.Fatalf("User-Agent = %q, want Capture", got)
	}
	if got := headers.Get("Authorization"); got != "Bearer token" {
		t.Fatalf("Authorization should be unchanged, got %q", got)
	}
}

func TestExecuteProxyFetchCapturesRewrittenRequestHeaders(t *testing.T) {
	seenUserAgent := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, "http://proxy.test/users", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("User-Agent", "Original")
	req.Header.Set("Authorization", "Bearer token")
	rules := []proxyconfig.HeaderMatchReplaceRule{{
		Enabled: true,
		Item:    "request_header",
		Match:   "^User-Agent:.*$",
		Replace: "User-Agent: Capture",
		Type:    "regex",
	}}

	handler := &Handler{client: server.Client()}
	result := handler.executeProxyFetch(req, nil, server.URL, rules)

	if !result.Success {
		t.Fatalf("executeProxyFetch failed: %v", result.ErrorMessage)
	}
	if seenUserAgent != "Capture" {
		t.Fatalf("real server User-Agent = %q, want Capture", seenUserAgent)
	}

	var captured map[string]string
	if err := json.Unmarshal([]byte(result.RequestHeaders), &captured); err != nil {
		t.Fatalf("RequestHeaders is not valid JSON: %v", err)
	}
	if got := captured["user-agent"]; got != "Capture" {
		t.Fatalf("captured user-agent = %q, want Capture", got)
	}
	if got := captured["authorization"]; got != "Bearer token" {
		t.Fatalf("captured authorization = %q, want Bearer token", got)
	}
}

func TestCaptureAuditInputUsesRewrittenProxyRequestHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://proxy.test/users", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("User-Agent", "Original")
	result := proxyResult{TargetURL: "http://real.test/users", RequestHeaders: `{"user-agent":"Capture"}`}

	input := captureAuditInput(req, nil, result, nil)

	if input.RequestHeaders != result.RequestHeaders {
		t.Fatalf("RequestHeaders = %q, want rewritten headers %q", input.RequestHeaders, result.RequestHeaders)
	}
}

func TestJSONErrorBodyEscapesMessage(t *testing.T) {
	body := jsonErrorBody(`bad "target"`)
	var parsed map[string]string
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("jsonErrorBody returned invalid JSON: %s", body)
	}
	if parsed["message"] != `bad "target"` {
		t.Fatalf("message = %q, want quoted message", parsed["message"])
	}
}

func TestSecurityFiltersDetectObviousPayloads(t *testing.T) {
	if !containsObviousXSS([]string{`q=<script>alert(1)</script>`}) {
		t.Fatal("expected XSS payload to be detected")
	}
	if !containsObviousSQLInjection([]string{`id=1 UNION SELECT password FROM users`}) {
		t.Fatal("expected SQL injection payload to be detected")
	}
	if containsObviousSQLInjection([]string{`name=ordinary search text`}) {
		t.Fatal("ordinary search text should not be detected as SQL injection")
	}
}

func TestSecurityInspectionSkipsBinaryBodies(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "http://proxy.test/upload", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	values := securityInspectionValues(req, []byte(`<script>alert(1)</script>`))

	if containsObviousXSS(values) {
		t.Fatal("binary request body should not be inspected by text security filters")
	}
}

func TestRateLimiterAllowsPerWindowLimit(t *testing.T) {
	limiter := newRateLimiter()

	if !limiter.Allow("GET /users 127.0.0.1", 2, 10_000_000_000) {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("GET /users 127.0.0.1", 2, 10_000_000_000) {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("GET /users 127.0.0.1", 2, 10_000_000_000) {
		t.Fatal("third request should be blocked")
	}
}
