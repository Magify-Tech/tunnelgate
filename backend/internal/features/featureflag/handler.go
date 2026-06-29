package featureflag

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(router *gin.RouterGroup) {
	router.GET("/features/", h.List)
}

func (h *Handler) List(c *gin.Context) {
	result := h.service.Get()
	c.JSON(http.StatusOK, result)
}

func validationError(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message, "issues": []gin.H{{"message": message}}})
}
