package environment

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	appconfig "postman-transform/backend-golang/internal/config"
	"postman-transform/backend-golang/internal/features/realtime"
)

type Handler struct {
	service *Service
	hub     *realtime.Hub
}

type createRequest struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

type updateRequest struct {
	Key   *string `json:"key"`
	Value string  `json:"value"`
}

func NewHandler(service *Service, hub *realtime.Hub) *Handler {
	return &Handler{service: service, hub: hub}
}

func (h *Handler) Register(router *gin.RouterGroup) {
	router.POST("/engine/environment/upload", h.Upload)
	router.GET("/engine/environment/export", h.Export)
	router.GET("/engine/environment", h.List)
	router.POST("/engine/environment", h.Create)
	router.PATCH("/engine/environment/:key", h.Update)
	router.DELETE("/engine/environment/:key", h.Delete)
}

func (h *Handler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		validationError(c, `Please upload a file using the "file" field`)
		return
	}
	uploadMaxBytes := appconfig.DefaultAppConfig().Environment.UploadMaxBytes
	if file.Size > uploadMaxBytes {
		validationError(c, "Uploaded file must be "+formatBytes(uploadMaxBytes)+" or smaller")
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
	name, count, err := h.service.ImportPayload(c.Request.Context(), payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	variables, _ := h.service.List(c.Request.Context())
	h.broadcastChanged("upserted", "")
	c.JSON(http.StatusCreated, gin.H{"message": "Environment imported successfully", "fileName": file.Filename, "environmentName": name, "importedCount": count, "variables": variables})
}

func (h *Handler) Export(c *gin.Context) {
	payload, err := h.service.ExportPostmanEnvironment(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.Header("Content-Disposition", `attachment; filename="collection-transform-environment.json"`)
	c.Data(http.StatusOK, "application/json", payload)
}

func (h *Handler) List(c *gin.Context) {
	variables, err := h.service.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"variables": variables})
}

func (h *Handler) Create(c *gin.Context) {
	var body createRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	item, err := h.service.Upsert(c.Request.Context(), body.Key, body.Value)
	if err != nil {
		validationError(c, err.Error())
		return
	}
	variables, _ := h.service.List(c.Request.Context())
	h.broadcastChanged("upserted", item.Key)
	c.JSON(http.StatusCreated, gin.H{"message": "Environment variable saved successfully", "item": item, "variables": variables})
}

func (h *Handler) Update(c *gin.Context) {
	var body updateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	nextKey := c.Param("key")
	if body.Key != nil {
		nextKey = *body.Key
	}
	item, err := h.service.Update(c.Request.Context(), c.Param("key"), nextKey, body.Value)
	if err != nil {
		validationError(c, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Environment variable not found"})
		return
	}
	variables, _ := h.service.List(c.Request.Context())
	h.broadcastChanged("updated", item.Key)
	c.JSON(http.StatusOK, gin.H{"message": "Environment variable updated successfully", "item": item, "variables": variables})
}

func (h *Handler) Delete(c *gin.Context) {
	deleted, err := h.service.Delete(c.Request.Context(), c.Param("key"))
	if err != nil {
		validationError(c, err.Error())
		return
	}
	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{"message": "Environment variable not found"})
		return
	}
	variables, _ := h.service.List(c.Request.Context())
	h.broadcastChanged("deleted", c.Param("key"))
	c.JSON(http.StatusOK, gin.H{"message": "Environment variable deleted successfully", "key": c.Param("key"), "variables": variables})
}

func (h *Handler) broadcastChanged(action, key string) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(realtime.EventMCPChanged, realtime.MCPChanged{
		Resource:       "environment-variables",
		Action:         action,
		Source:         "admin-api",
		EnvironmentKey: key,
		CreatedAt:      realtime.Now(),
	})
}

func validationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message, "issues": []gin.H{{"message": message}}})
}

func formatBytes(value int64) string {
	const mb = 1024 * 1024
	if value > 0 && value%mb == 0 {
		return strconv.FormatInt(value/mb, 10) + " MB"
	}
	return strconv.FormatInt(value, 10) + " bytes"
}
