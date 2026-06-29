package auditlog

import (
	"reflect"
	"testing"

	appconfig "postman-transform/backend-golang/internal/config"
)

func TestDefaultConfigUsesAppDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PageSizeDefault != 25 || !reflect.DeepEqual(cfg.PageSizeOptions, []int{25, 50, 100}) {
		t.Fatalf("unexpected default config: %#v", cfg)
	}
}

func TestConfigFromAppUsesCentralValues(t *testing.T) {
	cfg := ConfigFromApp(appconfig.AppConfig{
		AuditLog: appconfig.AuditLogConfig{
			PageSizeDefault: 40,
			PageSizeOptions: []int{10, 40, 10, -1},
		},
	})
	if cfg.PageSizeDefault != 40 || !reflect.DeepEqual(cfg.PageSizeOptions, []int{10, 40}) {
		t.Fatalf("unexpected overridden config: %#v", cfg)
	}
}
