package proxyconfig

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/features/realtime"
)

type Handler struct {
	service *Service
	hub     *realtime.Hub
}

type updateRequest struct {
	RealServerBaseURL *string                   `json:"realServerBaseUrl"`
	ShadowEndpoints   *[]ShadowEndpoint         `json:"shadowEndpoints"`
	CaptureEnabled    *bool                     `json:"captureEnabled"`
	AuditLogEnabled   *bool                     `json:"auditLogEnabled"`
	HeaderRules       *[]HeaderMatchReplaceRule `json:"headerRules"`
	Security          *PublicProxySecurity      `json:"security"`
}

type realServerRequest struct {
	RealServerBaseURL string `json:"realServerBaseUrl"`
}

type shadowEndpointsRequest struct {
	ShadowEndpoints []ShadowEndpoint `json:"shadowEndpoints" binding:"required"`
}

type enabledRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type headerRulesRequest struct {
	HeaderRules []HeaderMatchReplaceRule `json:"headerRules" binding:"required"`
}

type securityRequest struct {
	Security *PublicProxySecurity `json:"security" binding:"required"`
}

func NewHandler(service *Service, hub *realtime.Hub) *Handler {
	return &Handler{service: service, hub: hub}
}

func (h *Handler) Register(router *gin.RouterGroup) {
	router.GET("/engine/proxy-config", h.Get)
	router.PUT("/engine/proxy-config", h.Update)
	router.PATCH("/engine/proxy-config/real-server", h.PatchRealServer)
	router.PATCH("/engine/proxy-config/shadow-endpoints", h.PatchShadowEndpoints)
	router.PATCH("/engine/proxy-config/audit-log", h.PatchAuditLog)
	router.PATCH("/engine/proxy-config/header-rules", h.PatchHeaderRules)
	router.PATCH("/engine/proxy-config/security", h.PatchSecurity)
	router.PATCH("/engine/proxy-config/capture", h.PatchCapture)
}

func (h *Handler) Get(c *gin.Context) {
	config, err := h.service.Get(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"config": config})
}

func (h *Handler) Update(c *gin.Context) {
	var body updateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{
		RealServerBaseURL: body.RealServerBaseURL,
		ShadowEndpoints:   body.ShadowEndpoints,
		CaptureEnabled:    body.CaptureEnabled,
		AuditLogEnabled:   body.AuditLogEnabled,
		HeaderRules:       body.HeaderRules,
		Security:          body.Security,
	})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Proxy config updated successfully", "config": config})
}

func (h *Handler) PatchRealServer(c *gin.Context) {
	var body realServerRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{RealServerBaseURL: &body.RealServerBaseURL})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Real server endpoint updated successfully", "config": config})
}

func (h *Handler) PatchShadowEndpoints(c *gin.Context) {
	var body shadowEndpointsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{ShadowEndpoints: &body.ShadowEndpoints})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Shadow endpoints updated successfully", "config": config})
}

func (h *Handler) PatchAuditLog(c *gin.Context) {
	var body enabledRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{AuditLogEnabled: body.Enabled})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Proxy audit log updated successfully", "config": config})
}

func (h *Handler) PatchHeaderRules(c *gin.Context) {
	var body headerRulesRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{HeaderRules: &body.HeaderRules})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Header rules updated successfully", "config": config})
}

func (h *Handler) PatchSecurity(c *gin.Context) {
	var body securityRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.Security == nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{Security: body.Security})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Public proxy security updated successfully", "config": config})
}

func (h *Handler) PatchCapture(c *gin.Context) {
	var body enabledRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		validationError(c, "Invalid request")
		return
	}
	config, err := h.service.UpdatePartial(c.Request.Context(), UpdateInput{CaptureEnabled: body.Enabled})
	if err != nil {
		validationError(c, err.Error())
		return
	}
	h.broadcastChanged()
	c.JSON(http.StatusOK, gin.H{"message": "Proxy capture mode updated successfully", "config": config})
}

func (h *Handler) broadcastChanged() {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast(realtime.EventMCPChanged, realtime.MCPChanged{
		Resource:  "proxy-config",
		Action:    "updated",
		Source:    "admin-api",
		CreatedAt: realtime.Now(),
	})
}

func validationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message, "issues": []gin.H{{"message": message}}})
}
