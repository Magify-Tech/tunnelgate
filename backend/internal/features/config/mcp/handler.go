package mcp

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/features/realtime"
)

const endpoint = "/api/v1/mcp"

type Handler struct {
	service *Service
	hub     *realtime.Hub
}

type configUpdateRequest struct {
	Enabled        bool     `json:"enabled"`
	ClientToken    string   `json:"clientToken"`
	AllowedOrigins []string `json:"allowedOrigins"`
}

func NewHandler(service *Service, hub *realtime.Hub) *Handler {
	return &Handler{service: service, hub: hub}
}

func (h *Handler) Register(router *gin.RouterGroup) {
	router.GET("/engine/mcp-config", h.GetConfig)
	router.PUT("/engine/mcp-config", h.UpdateConfig)
	router.GET("/mcp", h.GetMCP)
	router.POST("/mcp", h.PostMCP)
}

func (h *Handler) GetConfig(c *gin.Context) {
	config, err := h.service.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": config, "endpoint": endpoint, "runtimeContext": runtimeContext()})
}

func (h *Handler) UpdateConfig(c *gin.Context) {
	var body configUpdateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.Update(c.Request.Context(), body.Enabled, body.ClientToken, body.AllowedOrigins)
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "MCP config updated successfully", "config": config, "endpoint": endpoint, "runtimeContext": runtimeContext()})
}

func (h *Handler) broadcastChanged() {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(realtime.EventMCPChanged, realtime.MCPChanged{
		Resource:  "mcp-config",
		Action:    "updated",
		Source:    "admin-api",
		CreatedAt: realtime.Now(),
	})
}

func (h *Handler) GetMCP(c *gin.Context) {
	c.JSON(http.StatusMethodNotAllowed, createMCPError(nil, -32000, "MCP streaming is not supported"))
}

func (h *Handler) PostMCP(c *gin.Context) {
	config, err := h.service.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, createMCPError(nil, -32000, err.Error()))
		return
	}
	if !config.Enabled {
		c.JSON(http.StatusForbidden, createMCPError(nil, -32000, "MCP is disabled"))
		return
	}
	if !h.service.isOriginAllowed(c.GetHeader("Origin"), config.AllowedOrigins) {
		c.JSON(http.StatusForbidden, createMCPError(nil, -32000, "MCP origin is not allowed"))
		return
	}
	if !isTokenAllowed(c.GetHeader("Authorization"), c.GetHeader("X-MCP-Token"), config.ClientToken) {
		c.JSON(http.StatusUnauthorized, createMCPError(nil, -32000, "MCP client token is required"))
		return
	}

	var message any
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&message); err != nil {
		c.JSON(http.StatusOK, createMCPError(nil, -32600, "Invalid JSON-RPC request", []gin.H{{"message": err.Error()}}))
		return
	}
	response, err := h.service.HandleJSONRPC(c.Request.Context(), message)
	if err != nil {
		c.JSON(http.StatusOK, createMCPError(nil, -32000, err.Error()))
		return
	}
	if response == nil {
		c.Status(http.StatusAccepted)
		return
	}
	if request, ok := message.(map[string]any); ok && request["method"] == "initialize" {
		if sessionID := createMCPSessionID(); sessionID != "" {
			c.Header("Mcp-Session-Id", sessionID)
		}
	}
	c.JSON(http.StatusOK, response)
}

func validationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message, "issues": []gin.H{{"message": message}}})
}
