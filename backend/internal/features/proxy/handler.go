package publicproxy

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/constants"
	"postman-transform/backend-golang/internal/features/api-specs"
	"postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/internal/features/config/proxyconfig"
	"postman-transform/backend-golang/internal/features/realtime"
)

var (
	publicProxyXSSPatterns          = compilePatterns(constants.PublicProxyXSSPatterns)
	publicProxySQLInjectionPatterns = compilePatterns(constants.PublicProxySQLInjectionPatterns)
)

type Handler struct {
	mockAPI     *mockapi.Service
	proxyConfig *proxyconfig.Service
	auditLog    *auditlog.Service
	hub         *realtime.Hub
	client      *http.Client
	limiter     *rateLimiter
}

func NewHandler(mockAPI *mockapi.Service, proxyConfig *proxyconfig.Service, auditLog *auditlog.Service, hub *realtime.Hub) *Handler {
	return &Handler{mockAPI: mockAPI, proxyConfig: proxyConfig, auditLog: auditLog, hub: hub, client: newProxyHTTPClient(), limiter: newRateLimiter()}
}

func newProxyHTTPClient() *http.Client {
	cfg := appconfig.DefaultAppConfig().Proxy.HTTPClient
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{
		Timeout:       cfg.DialTimeout,
		KeepAlive:     cfg.DialKeepAlive,
		FallbackDelay: cfg.DialFallbackDelay,
	}).DialContext
	transport.ExpectContinueTimeout = cfg.ExpectContinueTimeout
	transport.MaxIdleConns = cfg.MaxIdleConns
	transport.MaxIdleConnsPerHost = cfg.MaxIdleConnsPerHost

	return &http.Client{Timeout: cfg.Timeout, Transport: transport}
}

func (h *Handler) Middleware(c *gin.Context) {
	if isWebSocketUpgrade(c.Request) {
		config, err := h.proxyConfig.Get(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			c.Abort()
			return
		}
		h.websocketPassthrough(c, config)
		c.Abort()
		return
	}

	body, _ := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	path := c.Request.URL.Path
	if path == "" {
		path = "/"
	}
	config, err := h.proxyConfig.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		c.Abort()
		return
	}
	if blocked, status, message := h.securityBlockReason(c, body, config.Security); blocked {
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(status, gin.H{"message": message})
		c.Abort()
		return
	}
	if config.CaptureEnabled {
		matchedAPI, err := h.mockAPI.FindRoute(c.Request.Context(), c.Request.Method, path)
		if err != nil {
			applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			c.Abort()
			return
		}
		h.captureProxy(c, matchedAPI, body, config)
		c.Abort()
		return
	}

	matchedMock, err := h.mockAPI.FindActiveMock(c.Request.Context(), c.Request.Method, path)
	if err != nil {
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		c.Abort()
		return
	}
	if matchedMock == nil {
		matchedAPI, err := h.mockAPI.FindRoute(c.Request.Context(), c.Request.Method, path)
		if err != nil {
			applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			c.Abort()
			return
		}
		if matchedAPI != nil && !matchedAPI.MockEnabled && matchedAPI.ProxyEnabled {
			h.proxy(c, *matchedAPI, body, config)
			c.Abort()
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		c.Next()
		return
	}

	validationBody := mockapi.ParseValidationBody(c.Request, body)
	bodyValidation := mockapi.ValidateRequestKeys(*matchedMock, validationBody)
	if !bodyValidation.OK {
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusBadRequest, gin.H{
			"message":        "Request body keys do not match the uploaded collection",
			"expectedKeys":   matchedMock.ExpectedRequestKeys,
			"expectedTypes":  matchedMock.ExpectedRequestTypes,
			"missingKeys":    bodyValidation.MissingKeys,
			"unexpectedKeys": bodyValidation.UnexpectedKeys,
			"typeMismatches": bodyValidation.TypeMismatches,
		})
		c.Abort()
		return
	}

	paramValidation := mockapi.ValidateRequestParams(*matchedMock, c.Request.URL.Query())
	if !paramValidation.OK {
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusBadRequest, gin.H{
			"message":          "Request params do not match the uploaded collection",
			"expectedParams":   paramValidation.ExpectedKeys,
			"missingParams":    paramValidation.MissingKeys,
			"unexpectedParams": paramValidation.UnexpectedKeys,
		})
		c.Abort()
		return
	}

	responseBody, contentType := parseStoredResponse(matchedMock.ResponseBody)
	h.broadcastActivity(*matchedMock, "mock", &matchedMock.ResponseStatus, true)
	applyStoredResponseHeaders(c.Writer.Header(), matchedMock.ResponseHeaders)
	applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
	c.Data(matchedMock.ResponseStatus, contentType, responseBody)
	c.Abort()
}

func (h *Handler) websocketPassthrough(c *gin.Context, config proxyconfig.Config) {
	if config.RealServerBaseURL == "" {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Real server endpoint is not configured"})
		return
	}
	target, err := url.Parse(targetURL(config.RealServerBaseURL, c.Request.URL.RequestURI()))
	if err != nil || target.Scheme == "" || target.Host == "" || (target.Scheme != constants.URLSchemeHTTP && target.Scheme != constants.URLSchemeHTTPS) {
		c.JSON(http.StatusBadGateway, gin.H{"message": "Real server endpoint is invalid"})
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.Out.URL.Scheme = target.Scheme
			request.Out.URL.Host = target.Host
			request.Out.URL.Path = target.Path
			request.Out.URL.RawPath = target.RawPath
			request.Out.URL.RawQuery = target.RawQuery
			request.Out.Host = target.Host
		},
		ErrorHandler: func(w http.ResponseWriter, req *http.Request, err error) {
			http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) proxy(c *gin.Context, matchedAPI mockapi.APIRecord, body []byte, config proxyconfig.Config) {
	if config.RealServerBaseURL == "" {
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusBadGateway, gin.H{"message": "Real server endpoint is not configured"})
		return
	}

	enabledShadows := []proxyconfig.ShadowEndpoint{}
	if !config.CaptureEnabled {
		for _, endpoint := range config.ShadowEndpoints {
			if !endpoint.Enabled {
				continue
			}
			enabledShadows = append(enabledShadows, endpoint)
		}
	}
	shadowTargets := make([]auditlog.ShadowTarget, 0, len(enabledShadows))
	for _, endpoint := range enabledShadows {
		shadowTargets = append(shadowTargets, auditlog.ShadowTarget{ID: endpoint.ID, Name: endpoint.Name, BaseURL: endpoint.BaseURL, TargetURL: targetURL(endpoint.BaseURL, c.Request.URL.RequestURI())})
	}

	realResult := h.executeProxyFetch(c.Request, body, config.RealServerBaseURL, config.HeaderRules)
	apiID := matchedAPI.InternalID
	realRequestHeaders := auditRequestHeaders(c.Request, realResult)
	var auditRecord auditlog.Record
	var auditErr error
	if config.AuditLogEnabled {
		auditRecord, auditErr = h.auditLog.Create(c.Request.Context(), auditlog.CreateInput{
			APIMockDBID:     &apiID,
			APIMockID:       matchedAPI.ID,
			RouteName:       matchedAPI.RouteName,
			Method:          c.Request.Method,
			RoutePath:       matchedAPI.ResolvedRoutePath,
			TargetURL:       realResult.TargetURL,
			ResponseStatus:  realResult.ResponseStatus,
			DurationMS:      realResult.DurationMS,
			Success:         realResult.Success,
			ErrorMessage:    realResult.ErrorMessage,
			RequestHeaders:  realRequestHeaders,
			RequestBody:     requestBodyPreview(c.Request, body),
			ResponseHeaders: realResult.ResponseHeaders,
			ResponseBody:    responseBodyPreview(realResult.Body),
			ShadowTargets:   shadowTargets,
		})
	}
	if (!config.AuditLogEnabled || auditErr == nil) && h.hub != nil {
		if config.AuditLogEnabled {
			h.hub.Broadcast(realtime.EventProxyAuditLogCreated, auditRecord)
		}
		h.broadcastActivity(matchedAPI, "proxy", realResult.ResponseStatus, realResult.Success)
	}

	if config.AuditLogEnabled && auditErr == nil {
		baseShadowRequest := c.Request.Clone(context.Background())
		requestHeaders := headersJSON(c.Request.Header)
		requestPreview := requestBodyPreview(c.Request, body)
		for _, endpoint := range enabledShadows {
			go func(endpoint proxyconfig.ShadowEndpoint, auditID int, req *http.Request, requestHeaders, requestPreview string) {
				ctx, cancel := context.WithTimeout(context.Background(), appconfig.DefaultAppConfig().Proxy.ShadowRequestTimeout)
				result := h.executeProxyFetch(req.Clone(ctx), body, endpoint.BaseURL, nil)
				cancel()
				entry := auditlog.ShadowEntry{
					ID:              endpoint.ID,
					Name:            endpoint.Name,
					BaseURL:         endpoint.BaseURL,
					TargetURL:       result.TargetURL,
					ResponseStatus:  result.ResponseStatus,
					DurationMS:      result.DurationMS,
					Success:         result.Success,
					ErrorMessage:    result.ErrorMessage,
					RequestHeaders:  requestHeaders,
					RequestBody:     requestPreview,
					ResponseHeaders: result.ResponseHeaders,
					ResponseBody:    responseBodyPreview(result.Body),
				}
				if err := h.auditLog.CreateShadow(context.Background(), auditID, entry); err == nil && h.hub != nil {
					h.hub.Broadcast(realtime.EventProxyAuditLogShadowCreated, realtime.ProxyAuditLogShadowCreated{
						AuditLogID:  auditRecord.ID,
						ShadowEntry: entry,
					})
				}
			}(endpoint, auditRecord.InternalID, baseShadowRequest, requestHeaders, requestPreview)
		}
	} else if len(enabledShadows) > 0 {
		baseShadowRequest := c.Request.Clone(context.Background())
		for _, endpoint := range enabledShadows {
			go func(endpoint proxyconfig.ShadowEndpoint, req *http.Request) {
				ctx, cancel := context.WithTimeout(context.Background(), appconfig.DefaultAppConfig().Proxy.ShadowRequestTimeout)
				defer cancel()
				_ = h.executeProxyFetch(req.Clone(ctx), body, endpoint.BaseURL, nil)
			}(endpoint, baseShadowRequest)
		}
	}

	if !realResult.Success {
		message := "Proxy request failed"
		if realResult.ErrorMessage != nil {
			message = *realResult.ErrorMessage
		}
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusBadGateway, gin.H{"message": message})
		return
	}
	contentType := realResult.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	copyResponseHeaders(c.Writer.Header(), realResult.Headers)
	applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
	c.Data(statusOrDefault(realResult.ResponseStatus, 200), contentType, realResult.Body)
}

func (h *Handler) captureProxy(c *gin.Context, matchedAPI *mockapi.APIRecord, body []byte, config proxyconfig.Config) {
	if config.RealServerBaseURL == "" {
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusBadGateway, gin.H{"message": "Real server endpoint is not configured"})
		return
	}

	realResult := h.executeProxyFetch(c.Request, body, config.RealServerBaseURL, config.HeaderRules)
	if config.AuditLogEnabled {
		input := captureAuditInput(c.Request, body, realResult, matchedAPI)
		auditRecord, auditErr := h.auditLog.Create(c.Request.Context(), input)
		if auditErr == nil && h.hub != nil {
			h.hub.Broadcast(realtime.EventProxyAuditLogCreated, auditRecord)
		}
	}

	if realResult.Success && realResult.ResponseStatus != nil && matchedAPI != nil {
		h.broadcastActivity(*matchedAPI, "proxy", realResult.ResponseStatus, true)
	}

	if !realResult.Success {
		message := "Proxy request failed"
		if realResult.ErrorMessage != nil {
			message = *realResult.ErrorMessage
		}
		applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
		c.JSON(http.StatusBadGateway, gin.H{"message": message})
		return
	}
	contentType := realResult.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	copyResponseHeaders(c.Writer.Header(), realResult.Headers)
	applySecureHeaders(c.Writer.Header(), config.Security.SecureHeaders)
	c.Data(statusOrDefault(realResult.ResponseStatus, 200), contentType, realResult.Body)
}

func captureAuditInput(req *http.Request, body []byte, result proxyResult, matchedAPI *mockapi.APIRecord) auditlog.CreateInput {
	routePath := req.URL.Path
	if routePath == "" {
		routePath = "/"
	}
	routeName := req.Method + " " + routePath
	var apiDBID *int
	apiID := ""
	if matchedAPI != nil {
		apiDBID = &matchedAPI.InternalID
		apiID = matchedAPI.ID
		routeName = matchedAPI.RouteName
		routePath = matchedAPI.ResolvedRoutePath
	}
	return auditlog.CreateInput{
		APIMockDBID:     apiDBID,
		APIMockID:       apiID,
		RouteName:       routeName,
		Method:          req.Method,
		RoutePath:       routePath,
		TargetURL:       result.TargetURL,
		ResponseStatus:  result.ResponseStatus,
		DurationMS:      result.DurationMS,
		Success:         result.Success,
		ErrorMessage:    result.ErrorMessage,
		RequestHeaders:  auditRequestHeaders(req, result),
		RequestBody:     requestBodyPreview(req, body),
		ResponseHeaders: result.ResponseHeaders,
		ResponseBody:    responseBodyPreview(result.Body),
		ShadowTargets:   []auditlog.ShadowTarget{},
	}
}

func auditRequestHeaders(req *http.Request, result proxyResult) string {
	if result.RequestHeaders != "" {
		return result.RequestHeaders
	}
	return headersJSON(req.Header)
}

func (h *Handler) broadcastActivity(api mockapi.APIRecord, source string, responseStatus *int, success bool) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(realtime.EventImportedAPIActivity, realtime.ImportedAPIActivity{
		APIMockID:      api.ID,
		RouteName:      api.RouteName,
		Method:         api.Method,
		RoutePath:      api.RoutePath,
		ResponseStatus: responseStatus,
		Success:        success,
		Source:         source,
		CreatedAt:      realtime.Now(),
	})
}

func (h *Handler) securityBlockReason(c *gin.Context, body []byte, config proxyconfig.PublicProxySecurity) (bool, int, string) {
	if !config.Enabled {
		return false, 0, ""
	}
	if config.RateLimit && config.RateLimitPerMinute > 0 {
		key := c.Request.Method + " " + c.Request.URL.Path + " " + c.ClientIP()
		if !h.limiter.Allow(key, config.RateLimitPerMinute, time.Minute) {
			return true, http.StatusTooManyRequests, "Rate limit exceeded"
		}
	}
	values := securityInspectionValues(c.Request, body)
	if config.XSS && containsObviousXSS(values) {
		return true, http.StatusForbidden, "Request blocked by XSS filter"
	}
	if config.SQLInjection && containsObviousSQLInjection(values) {
		return true, http.StatusForbidden, "Request blocked by SQL injection filter"
	}
	return false, 0, ""
}

func securityInspectionValues(req *http.Request, body []byte) []string {
	values := []string{req.URL.RawQuery, req.URL.Path}
	for key, headerValues := range req.Header {
		values = append(values, key)
		values = append(values, headerValues...)
	}
	if len(body) > 0 && isTextBody(strings.ToLower(req.Header.Get("content-type"))) {
		values = append(values, string(body))
	}
	return values
}

func containsObviousXSS(values []string) bool {
	return containsAnyPattern(values, publicProxyXSSPatterns)
}

func containsObviousSQLInjection(values []string) bool {
	return containsAnyPattern(values, publicProxySQLInjectionPatterns)
}

func containsAnyPattern(values []string, patterns []*regexp.Regexp) bool {
	for _, value := range values {
		for _, pattern := range patterns {
			if pattern.MatchString(value) {
				return true
			}
		}
	}
	return false
}

func compilePatterns(patterns []string) []*regexp.Regexp {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, regexp.MustCompile(pattern))
	}
	return compiled
}

func isWebSocketUpgrade(req *http.Request) bool {
	return strings.EqualFold(req.Header.Get("Upgrade"), "websocket") && headerHasToken(req.Header, "Connection", "upgrade")
}

func headerHasToken(headers http.Header, name, token string) bool {
	for _, value := range headers.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

type rateLimitBucket struct {
	windowStart time.Time
	count       int
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateLimitBucket
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{buckets: map[string]rateLimitBucket{}}
}

func (r *rateLimiter) Allow(key string, limit int, window time.Duration) bool {
	if r == nil || limit <= 0 {
		return true
	}
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket := r.buckets[key]
	if bucket.windowStart.IsZero() || now.Sub(bucket.windowStart) >= window {
		r.buckets[key] = rateLimitBucket{windowStart: now, count: 1}
		r.prune(now, window)
		return true
	}
	if bucket.count >= limit {
		return false
	}
	bucket.count++
	r.buckets[key] = bucket
	return true
}

func (r *rateLimiter) prune(now time.Time, window time.Duration) {
	for key, bucket := range r.buckets {
		if now.Sub(bucket.windowStart) >= 2*window {
			delete(r.buckets, key)
		}
	}
}

type proxyResult struct {
	TargetURL       string
	ResponseStatus  *int
	DurationMS      int
	Success         bool
	ErrorMessage    *string
	RequestHeaders  string
	Headers         http.Header
	ResponseHeaders string
	Body            []byte
	ContentType     string
}

func (h *Handler) executeProxyFetch(req *http.Request, body []byte, baseURL string, headerRules []proxyconfig.HeaderMatchReplaceRule) proxyResult {
	started := time.Now()
	target := targetURL(baseURL, req.URL.RequestURI())
	proxyReq, err := http.NewRequestWithContext(req.Context(), req.Method, target, bytes.NewReader(body))
	if err != nil {
		message := err.Error()
		return proxyResult{TargetURL: target, DurationMS: int(time.Since(started).Milliseconds()), Success: false, ErrorMessage: &message, ResponseHeaders: "{}", Body: jsonErrorBody(message), ContentType: "application/json"}
	}
	copyProxyHeaders(proxyReq.Header, req.Header)
	applyHeaderRules(proxyReq.Header, headerRules, constants.HeaderRuleItemRequest)
	proxyReq.Header.Set("Accept-Encoding", "identity")
	requestHeaders := headersJSON(proxyReq.Header)
	resp, err := h.client.Do(proxyReq)
	if err != nil {
		message := err.Error()
		return proxyResult{TargetURL: target, DurationMS: int(time.Since(started).Milliseconds()), Success: false, ErrorMessage: &message, RequestHeaders: requestHeaders, ResponseHeaders: "{}", Body: jsonErrorBody(message), ContentType: "application/json"}
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	applyHeaderRules(resp.Header, headerRules, constants.HeaderRuleItemResponse)
	responseHeaders := resp.Header.Clone()
	status := resp.StatusCode
	return proxyResult{TargetURL: target, ResponseStatus: &status, DurationMS: int(time.Since(started).Milliseconds()), Success: true, RequestHeaders: requestHeaders, Headers: responseHeaders, ResponseHeaders: headersJSON(responseHeaders), Body: responseBody, ContentType: responseHeaders.Get("content-type")}
}

func jsonErrorBody(message string) []byte {
	body, err := json.Marshal(map[string]string{"message": message})
	if err != nil {
		return []byte(`{"message":"Proxy request failed"}`)
	}
	return body
}

func parseStoredResponse(responseBody string) ([]byte, string) {
	var parsed any
	if json.Unmarshal([]byte(responseBody), &parsed) == nil {
		return []byte(responseBody), "application/json"
	}
	trimmed := strings.ToLower(strings.TrimSpace(responseBody))
	if strings.HasPrefix(trimmed, "<soap") || strings.Contains(trimmed, "<soap:") || strings.Contains(trimmed, "<soapenv:") {
		return []byte(responseBody), "application/soap+xml; charset=utf-8"
	}
	if strings.HasPrefix(trimmed, "<") {
		return []byte(responseBody), "application/xml; charset=utf-8"
	}
	return []byte(responseBody), "text/plain; charset=utf-8"
}

func applyStoredResponseHeaders(headers http.Header, value string) {
	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return
	}
	for key, headerValue := range parsed {
		if strings.TrimSpace(key) != "" {
			headers.Set(key, headerValue)
		}
	}
}

func targetURL(baseURL, originalURI string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	parts := strings.SplitN(originalURI, "?", 2)
	basePath := strings.TrimRight(parsed.Path, "/")
	requestPath := strings.TrimLeft(parts[0], "/")
	parsed.Path = strings.ReplaceAll(basePath+"/"+requestPath, "//", "/")
	if len(parts) == 2 {
		parsed.RawQuery = parts[1]
	} else {
		parsed.RawQuery = ""
	}
	return parsed.String()
}

func copyProxyHeaders(target, source http.Header) {
	ignored := map[string]bool{"accept-encoding": true, "connection": true, "content-length": true, "expect": true, "host": true, "keep-alive": true, "proxy-authenticate": true, "proxy-authorization": true, "te": true, "trailer": true, "transfer-encoding": true, "upgrade": true}
	for key, values := range source {
		if ignored[strings.ToLower(key)] {
			continue
		}
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func copyResponseHeaders(target, source http.Header) {
	ignored := map[string]bool{"connection": true, "content-length": true, "keep-alive": true, "proxy-authenticate": true, "proxy-authorization": true, "te": true, "trailer": true, "transfer-encoding": true, "upgrade": true}
	for key, values := range source {
		if ignored[strings.ToLower(key)] {
			continue
		}
		target.Del(key)
		for _, value := range values {
			target.Add(key, value)
		}
	}
}

func applyHeaderRules(headers http.Header, rules []proxyconfig.HeaderMatchReplaceRule, item string) {
	for _, rule := range rules {
		if !rule.Enabled || rule.Item != item || strings.TrimSpace(rule.Match) == "" {
			continue
		}
		applyHeaderRule(headers, rule)
	}
}

func applyHeaderRule(headers http.Header, rule proxyconfig.HeaderMatchReplaceRule) {
	lines := []string{}
	for key, values := range headers {
		lines = append(lines, key+": "+strings.Join(values, ", "))
	}
	for _, line := range lines {
		next, ok := replaceHeaderLine(line, rule)
		if !ok {
			continue
		}
		name := strings.SplitN(line, ":", 2)[0]
		headers.Del(name)
		if strings.TrimSpace(next) == "" {
			continue
		}
		nextName, nextValue, found := strings.Cut(next, ":")
		if !found {
			headers.Set(name, strings.TrimSpace(next))
			continue
		}
		headers.Set(strings.TrimSpace(nextName), strings.TrimSpace(nextValue))
	}
}

func replaceHeaderLine(line string, rule proxyconfig.HeaderMatchReplaceRule) (string, bool) {
	if rule.Type == constants.HeaderRuleTypeLiteral {
		if !strings.Contains(line, rule.Match) {
			return "", false
		}
		return strings.ReplaceAll(line, rule.Match, rule.Replace), true
	}
	expr, err := regexp.Compile(rule.Match)
	if err != nil || !expr.MatchString(line) {
		return "", false
	}
	return expr.ReplaceAllString(line, rule.Replace), true
}

func applySecureHeaders(headers http.Header, config proxyconfig.PublicProxySecureHeaders) {
	if !config.Enabled {
		return
	}
	for _, header := range config.Headers {
		if !header.Enabled || strings.TrimSpace(header.Name) == "" {
			continue
		}
		headers.Set(header.Name, header.Value)
	}
}

func headersJSON(headers http.Header) string {
	values := map[string]string{}
	for key, value := range headers {
		values[strings.ToLower(key)] = strings.Join(value, ", ")
	}
	data, _ := json.MarshalIndent(values, "", "  ")
	return string(data)
}

func requestBodyPreview(req *http.Request, body []byte) string {
	if req.Method == http.MethodGet || req.Method == http.MethodHead || len(body) == 0 {
		return ""
	}
	proxyDefaults := appconfig.DefaultAppConfig().Proxy
	contentType := strings.ToLower(req.Header.Get("content-type"))
	if isTextBody(contentType) {
		return truncateString(string(body), proxyDefaults.RequestPreviewBytes)
	}
	maxBytes := len(body)
	if maxBytes > proxyDefaults.BinaryPreviewBytes {
		maxBytes = proxyDefaults.BinaryPreviewBytes
	}
	preview := proxyDefaults.HexBodyPrefix + hex.EncodeToString(body[:maxBytes])
	if len(body) > maxBytes {
		preview += "\n\n" + proxyDefaults.BinaryTruncateMarker + strconv.Itoa(len(body)-maxBytes) + " bytes]"
	}
	return preview
}

func bodyTypeFromContentType(contentType string, response bool) string {
	lower := strings.ToLower(contentType)
	switch {
	case hasContentTypeMarker(lower, constants.JSONContentTypeMarkers):
		return "json"
	case strings.Contains(lower, constants.ContentTypeXML) || strings.Contains(lower, constants.ContentTypeStructuredXML) || strings.Contains(lower, constants.ContentTypeSOAP):
		return "xml"
	case strings.Contains(lower, constants.ContentTypeHTML):
		return "html"
	case strings.Contains(lower, constants.ContentTypeYAML) || strings.Contains(lower, constants.ContentTypeYML):
		return "yaml"
	case strings.Contains(lower, constants.ContentTypeJavaScript):
		return "javascript"
	case strings.Contains(lower, constants.ContentTypeFormURLEncoded):
		return constants.RequestBodyTypeFormURLEncoded
	case strings.Contains(lower, constants.ContentTypeMultipartForm):
		return constants.RequestBodyTypeFormData
	}
	if response {
		return "raw"
	}
	return "raw"
}

func queryParamKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func queryParamTypes(values url.Values) mockapi.RequestBodyTypes {
	types := mockapi.RequestBodyTypes{}
	for key := range values {
		if strings.TrimSpace(key) != "" {
			types[key] = "string"
		}
	}
	return types
}

func responseBodyPreview(body []byte) string {
	return truncateString(string(body), appconfig.DefaultAppConfig().Proxy.ResponsePreviewBytes)
}

func truncateString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func isTextBody(contentType string) bool {
	return contentType == "" || hasContentTypeMarker(contentType, constants.TextBodyContentTypeMarkers)
}

func hasContentTypeMarker(contentType string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(contentType, marker) {
			return true
		}
	}
	return false
}

func statusOrDefault(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}
