package auditlog

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/constants"
)

type Handler struct {
	service *Service
	config  Config
	client  *http.Client
}

func NewHandler(service *Service, configs ...Config) *Handler {
	cfg := service.Config()
	if len(configs) > 0 {
		cfg = configs[0]
	}
	cfg = normalizeConfig(cfg)
	return &Handler{service: service, config: cfg, client: newReplayHTTPClient()}
}

func (h *Handler) Register(router *gin.RouterGroup) {
	h.RegisterAuditLogRoutes(router)
	h.RegisterProxyReplayRoutes(router)
	h.RegisterDirectRequestRoutes(router)
}

func (h *Handler) RegisterAuditLogRoutes(router *gin.RouterGroup) {
	router.GET("/engine/proxy-audit-logs", h.List)
	router.GET("/engine/proxy-audit-logs/:id", h.Get)
}

func (h *Handler) RegisterProxyReplayRoutes(router *gin.RouterGroup) {
	router.POST("/engine/proxy-audit-logs/:id/replay", h.Replay)
}

func (h *Handler) RegisterDirectRequestRoutes(router *gin.RouterGroup) {
	router.POST("/engine/direct-request", h.DirectRequest)
}

func (h *Handler) List(c *gin.Context) {
	page := queryInt(c, "page", 1)
	pageSize := queryInt(c, "pageSize", h.config.PageSizeDefault)
	if !h.config.HasPageSize(pageSize) {
		validationError(c, "pageSize must be one of "+h.config.PageSizeOptionsText())
		return
	}
	result, err := h.service.List(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Get(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		validationError(c, "id is required")
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"message": "Proxy audit log not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (h *Handler) Replay(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		validationError(c, "id is required")
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"message": "Proxy audit log not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	request := replayRequestFromRecord(item)
	if c.Request.ContentLength != 0 {
		var body ReplayRequest
		if err := c.ShouldBindJSON(&body); err != nil {
			validationError(c, "request body must be valid JSON")
			return
		}
		request = mergeReplayRequest(request, body)
	}
	if err := validateReplayRequest(request); err != nil {
		validationError(c, err.Error())
		return
	}
	result := h.replay(c.Request.Context(), request)
	c.JSON(http.StatusOK, gin.H{"item": result})
}

func (h *Handler) DirectRequest(c *gin.Context) {
	var body ReplayRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "request body must be valid JSON")
		return
	}
	request := mergeReplayRequest(ReplayRequest{Method: http.MethodGet, RequestHeaders: "{}"}, body)
	if strings.TrimSpace(request.TargetURL) == "" {
		validationError(c, "targetUrl is required")
		return
	}
	if err := validateReplayRequest(request); err != nil {
		validationError(c, err.Error())
		return
	}
	result := h.replay(c.Request.Context(), request)
	c.JSON(http.StatusOK, gin.H{"item": result})
}

func (h *Handler) replay(ctx context.Context, input ReplayRequest) ReplayResult {
	return executeReplayRequest(ctx, h.client, input)
}

func ValidateReplayRequest(request ReplayRequest) error {
	return validateReplayRequest(request)
}

func ExecuteReplayRequest(ctx context.Context, input ReplayRequest) ReplayResult {
	return executeReplayRequest(ctx, newReplayHTTPClient(), input)
}

func executeReplayRequest(ctx context.Context, client *http.Client, input ReplayRequest) ReplayResult {
	started := time.Now()
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if method == "" {
		method = http.MethodGet
	}
	targetURL := strings.TrimSpace(input.TargetURL)
	requestHeaders := strings.TrimSpace(input.RequestHeaders)
	if requestHeaders == "" {
		requestHeaders = "{}"
	}
	body := replayBodyBytes(input.RequestBody)
	result := ReplayResult{
		TargetURL:      targetURL,
		RequestHeaders: requestHeaders,
		RequestBody:    input.RequestBody,
	}
	req, err := http.NewRequestWithContext(ctx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		message := err.Error()
		result.DurationMS = int(time.Since(started).Milliseconds())
		result.Success = false
		result.ErrorMessage = &message
		result.ResponseHeaders = "{}"
		result.ResponseBody = `{"message":"` + message + `"}`
		return result
	}
	copyReplayHeaders(req.Header, replayHeadersFromJSON(requestHeaders))
	req.Header.Set("Accept-Encoding", "identity")

	resp, err := client.Do(req)
	if err != nil {
		message := err.Error()
		result.DurationMS = int(time.Since(started).Milliseconds())
		result.Success = false
		result.ErrorMessage = &message
		result.ResponseHeaders = "{}"
		result.ResponseBody = `{"message":"` + message + `"}`
		return result
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	status := resp.StatusCode
	result.ResponseStatus = &status
	result.DurationMS = int(time.Since(started).Milliseconds())
	result.Success = true
	result.ResponseHeaders = headersJSON(resp.Header)
	result.ResponseBody = truncateString(string(responseBody), appconfig.DefaultAppConfig().AuditLog.ResponsePreviewBytes)
	return result
}

func replayRequestFromRecord(item Record) ReplayRequest {
	return ReplayRequest{
		Method:         item.Method,
		TargetURL:      item.TargetURL,
		RequestHeaders: item.RequestHeaders,
		RequestBody:    item.RequestBody,
	}
}

func mergeReplayRequest(base ReplayRequest, override ReplayRequest) ReplayRequest {
	base.Method = override.Method
	base.TargetURL = override.TargetURL
	base.RequestHeaders = override.RequestHeaders
	base.RequestBody = override.RequestBody
	return base
}

func validateReplayRequest(request ReplayRequest) error {
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !validReplayMethod(method) {
		return errors.New("method must be a valid HTTP method")
	}
	if !validReplayURL(request.TargetURL) {
		return errors.New("targetUrl must be a valid URL")
	}
	if !validHeadersJSON(request.RequestHeaders) {
		return errors.New("requestHeaders must be a JSON object")
	}
	return nil
}

func validReplayMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func validReplayURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.Scheme != "" && parsed.Host != "" && (parsed.Scheme == constants.URLSchemeHTTP || parsed.Scheme == constants.URLSchemeHTTPS)
}

func validHeadersJSON(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	parsed := map[string]string{}
	return json.Unmarshal([]byte(trimmed), &parsed) == nil
}

func queryInt(c *gin.Context, key string, fallback int) int {
	parsed, err := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(fallback)))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func validationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message, "issues": []gin.H{{"message": message}}})
}

func newReplayHTTPClient() *http.Client {
	cfg := appconfig.DefaultAppConfig().AuditLog.ReplayHTTPClient
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

func replayHeadersFromJSON(value string) http.Header {
	headers := http.Header{}
	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return headers
	}
	for key, headerValue := range parsed {
		if strings.TrimSpace(key) != "" {
			headers.Set(key, headerValue)
		}
	}
	return headers
}

func copyReplayHeaders(target, source http.Header) {
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

func replayBodyBytes(value string) []byte {
	trimmed := strings.TrimSpace(value)
	auditDefaults := appconfig.DefaultAppConfig().AuditLog
	if strings.HasPrefix(trimmed, auditDefaults.HexBodyPrefix) && !strings.Contains(trimmed, auditDefaults.BinaryTruncateMarker) {
		if decoded, err := hex.DecodeString(strings.TrimPrefix(trimmed, auditDefaults.HexBodyPrefix)); err == nil {
			return decoded
		}
	}
	return []byte(value)
}

func headersJSON(headers http.Header) string {
	values := map[string]string{}
	for key, value := range headers {
		values[strings.ToLower(key)] = strings.Join(value, ", ")
	}
	data, _ := json.MarshalIndent(values, "", "  ")
	return string(data)
}

func truncateString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
