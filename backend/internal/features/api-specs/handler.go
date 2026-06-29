package mockapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/features/realtime"
)

type Handler struct {
	service *Service
	hub     *realtime.Hub
	config  Config
}

func NewHandler(service *Service, hub *realtime.Hub, configs ...Config) *Handler {
	cfg := service.Config()
	if len(configs) > 0 {
		cfg = configs[0]
	}
	cfg = normalizeConfig(cfg)
	return &Handler{service: service, hub: hub, config: cfg}
}

func (h *Handler) Register(router *gin.RouterGroup) {
	h.RegisterAPISpecRoutes(router)
	h.RegisterEditorRoutes(router)
	h.RegisterSnapshotRoutes(router)
}

func (h *Handler) RegisterAPISpecRoutes(router *gin.RouterGroup) {
	router.POST("/engine/upload", h.Upload)
	router.POST("/engine/swagger/upload", h.UploadSwagger)
	router.GET("/engine/export/spec", h.ExportSpec)
	router.GET("/mock/apis", h.List)
	router.GET("/mock/apis/directory", h.Directory)
	router.GET("/mock/apis/collections", h.Collections)
	router.PATCH("/mock/apis/:id", h.ToggleMock)
	router.PATCH("/mock/apis/:id/proxy", h.ToggleProxy)
	router.PATCH("/mock/apis/:id/response", h.SelectResponse)
	router.DELETE("/mock/apis/:id", h.Delete)
}

func (h *Handler) RegisterEditorRoutes(router *gin.RouterGroup) {
	router.POST("/mock/apis", h.CreateManual)
	router.GET("/mock/apis/:id", h.Get)
	router.PUT("/mock/apis/:id", h.UpdateManual)
}

func (h *Handler) RegisterSnapshotRoutes(router *gin.RouterGroup) {
	router.GET("/mock/api-versions", h.ListVersions)
	router.POST("/mock/api-versions", h.CreateVersion)
	router.GET("/mock/api-versions/:id", h.GetVersion)
	router.POST("/mock/api-versions/:id/restore", h.RestoreVersion)
	router.POST("/mock/api-versions/:id/revert", h.RevertVersion)
}

func (h *Handler) ListVersions(c *gin.Context) {
	result, err := h.service.ListVersions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": result})
}

func (h *Handler) CreateVersion(c *gin.Context) {
	var body VersionCreateInput
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "invalid version payload")
		return
	}
	item, err := h.service.CreateVersion(c.Request.Context(), body.Message)
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged("api-mocks", "upserted", nil)
	c.JSON(http.StatusCreated, gin.H{"message": "Project version created", "item": item})
}

func (h *Handler) GetVersion(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	item, err := h.service.GetVersion(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Project version not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (h *Handler) RestoreVersion(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	item, err := h.service.RestoreVersion(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Project version not found"})
		return
	}
	h.broadcastChanged("api-mocks", "upserted", nil)
	c.JSON(http.StatusOK, gin.H{"message": "Project version restored", "item": item})
}

func (h *Handler) RevertVersion(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	item, err := h.service.RevertVersion(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Project version not found"})
		return
	}
	h.broadcastChanged("api-mocks", "upserted", nil)
	c.JSON(http.StatusOK, gin.H{"message": "Project version reverted", "item": item})
}

func (h *Handler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		validationError(c, `Please upload a file using the "file" field`)
		return
	}
	if file.Size > h.config.UploadMaxBytes {
		validationError(c, "Uploaded file must be "+formatBytes(h.config.UploadMaxBytes)+" or smaller")
		return
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	defer opened.Close()
	payload, err := io.ReadAll(opened)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	collectionName, count, err := h.service.ImportCollection(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	h.broadcastChanged("api-mocks", "upserted", nil)
	c.JSON(http.StatusCreated, gin.H{"message": "Collection imported successfully", "fileName": file.Filename, "collectionName": collectionName, "importedCount": count})
}

func (h *Handler) UploadSwagger(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		validationError(c, `Please upload a file using the "file" field`)
		return
	}
	if file.Size > h.config.UploadMaxBytes {
		validationError(c, "Uploaded file must be "+formatBytes(h.config.UploadMaxBytes)+" or smaller")
		return
	}
	opened, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	defer opened.Close()
	payload, err := io.ReadAll(opened)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	specName, count, err := h.service.ImportSwagger(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	h.broadcastChanged("api-mocks", "upserted", nil)
	c.JSON(http.StatusCreated, gin.H{"message": "Swagger imported successfully", "fileName": file.Filename, "collectionName": specName, "importedCount": count})
}

func (h *Handler) ExportSpec(c *gin.Context) {
	payload, fileName, contentType, err := h.service.ExportSpec(c.Request.Context(), c.DefaultQuery("format", "postman"))
	if err != nil {
		validationError(c, err.Error())
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+fileName+`"`)
	c.Data(http.StatusOK, contentType, payload)
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

func (h *Handler) Directory(c *gin.Context) {
	mode := c.DefaultQuery("mode", "path")
	var path []string
	if err := json.Unmarshal([]byte(c.DefaultQuery("path", "[]")), &path); err != nil {
		validationError(c, "path must be a JSON string array")
		return
	}
	result, err := h.service.Directory(c.Request.Context(), mode, path)
	if err != nil {
		if errors.Is(err, ErrInvalidDirectoryPath) {
			validationError(c, err.Error())
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Collections(c *gin.Context) {
	result, err := h.service.Collections(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) Get(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (h *Handler) CreateManual(c *gin.Context) {
	var body ManualSpecInput
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "invalid API spec payload")
		return
	}
	item, err := h.service.CreateManual(c.Request.Context(), body)
	if err != nil {
		h.writeManualSpecError(c, err)
		return
	}
	h.broadcastChanged("api-mocks", "created", &item.ID)
	c.JSON(http.StatusCreated, gin.H{"message": "API spec created", "item": item})
}

func (h *Handler) UpdateManual(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var body ManualSpecInput
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "invalid API spec payload")
		return
	}
	item, err := h.service.UpdateManual(c.Request.Context(), id, body)
	if err != nil {
		h.writeManualSpecError(c, err)
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
		return
	}
	h.broadcastChanged("api-mocks", "updated", &id)
	c.JSON(http.StatusOK, gin.H{"message": "API spec updated", "item": item})
}

func (h *Handler) writeManualSpecError(c *gin.Context, err error) {
	if errors.Is(err, ErrDuplicateRoute) {
		c.JSON(http.StatusConflict, gin.H{"message": err.Error()})
		return
	}
	validationError(c, err.Error())
}

func (h *Handler) ToggleMock(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	enabled, ok := enabledBody(c)
	if !ok {
		return
	}
	item, err := h.service.SetMockEnabled(c.Request.Context(), id, enabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
		return
	}
	h.broadcastChanged("api-mocks", "updated", &id)
	c.JSON(http.StatusOK, gin.H{"message": "Mock status updated", "item": item})
}

func (h *Handler) ToggleProxy(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	enabled, ok := enabledBody(c)
	if !ok {
		return
	}
	item, err := h.service.SetProxyEnabled(c.Request.Context(), id, enabled)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
		return
	}
	h.broadcastChanged("api-mocks", "updated", &id)
	c.JSON(http.StatusOK, gin.H{"message": "Proxy status updated", "item": item})
}

func (h *Handler) SelectResponse(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	var body SelectResponseInput
	if err := c.ShouldBindJSON(&body); err != nil || !validHTTPStatus(body.Status) {
		validationError(c, "status must be a valid HTTP status code")
		return
	}
	item, err := h.service.SelectResponse(c.Request.Context(), id, body.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "API response example not found"})
		return
	}
	h.broadcastChanged("api-mocks", "updated", &id)
	c.JSON(http.StatusOK, gin.H{"message": "Mock response status updated", "item": item})
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := idParam(c)
	if !ok {
		return
	}
	deleted, err := h.service.Delete(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"message": "API not found"})
		return
	}
	h.broadcastChanged("api-mocks", "deleted", &id)
	c.JSON(http.StatusOK, gin.H{"message": "API deleted successfully", "id": id})
}

func (h *Handler) broadcastChanged(resource, action string, apiMockID *string) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(realtime.EventMCPChanged, realtime.MCPChanged{
		Resource:  resource,
		Action:    action,
		Source:    "admin-api",
		APIMockID: apiMockID,
		CreatedAt: realtime.Now(),
	})
}

func idParam(c *gin.Context) (string, bool) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		validationError(c, "id is required")
		return "", false
	}
	return id, true
}

func enabledBody(c *gin.Context) (bool, bool) {
	var body ToggleInput
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		validationError(c, "enabled is required")
		return false, false
	}
	return *body.Enabled, true
}

func queryInt(c *gin.Context, key string, fallback int) int {
	parsed, err := strconv.Atoi(c.DefaultQuery(key, strconv.Itoa(fallback)))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func formatBytes(value int64) string {
	const mb = 1024 * 1024
	if value > 0 && value%mb == 0 {
		return strconv.FormatInt(value/mb, 10) + " MB"
	}
	return strconv.FormatInt(value, 10) + " bytes"
}

func validationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message, "issues": []gin.H{{"message": message}}})
}
