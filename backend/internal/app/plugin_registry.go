package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	pluginTargetAdmin  = "admin"
	pluginTargetPublic = "public"
)

type FeaturePluginBinding struct {
	Name       string `json:"name"`
	Target     string `json:"target"`
	PortNumber int    `json:"portNumber,omitempty"`
	Source     string `json:"source"`
	Path       string `json:"path,omitempty"`
	Symbol     string `json:"symbol,omitempty"`
}

type FeaturePluginRegistry struct {
	adminPort  int
	publicPort int
	items      map[string][]FeaturePluginBinding
}

func NewFeaturePluginRegistry(adminPort, publicPort int) *FeaturePluginRegistry {
	return &FeaturePluginRegistry{
		adminPort:  adminPort,
		publicPort: publicPort,
		items: map[string][]FeaturePluginBinding{
			pluginTargetAdmin:  {},
			pluginTargetPublic: {},
		},
	}
}

func (r *FeaturePluginRegistry) RegisterAdminRoutes(router gin.IRouter) {
	router.GET("/engine/plugins", r.ListAdmin)
}

func (r *FeaturePluginRegistry) Set(target string, bindings []FeaturePluginBinding) {
	if r == nil {
		return
	}
	copied := append([]FeaturePluginBinding(nil), bindings...)
	r.items[target] = copied
}

func (r *FeaturePluginRegistry) ListAdmin(c *gin.Context) {
	if r == nil {
		c.JSON(http.StatusOK, gin.H{"target": pluginTargetAdmin, "portNumber": 0, "items": []FeaturePluginBinding{}, "total": 0})
		return
	}
	items := r.Bindings(pluginTargetAdmin)
	c.JSON(http.StatusOK, gin.H{
		"target":     pluginTargetAdmin,
		"portNumber": r.adminPort,
		"items":      items,
		"total":      len(items),
	})
}

func (r *FeaturePluginRegistry) Bindings(target string) []FeaturePluginBinding {
	if r == nil {
		return nil
	}
	return append([]FeaturePluginBinding(nil), r.items[target]...)
}

func portNumberForTarget(target string, registry *FeaturePluginRegistry) int {
	if registry == nil {
		return 0
	}
	if target == pluginTargetPublic {
		return registry.publicPort
	}
	return registry.adminPort
}
