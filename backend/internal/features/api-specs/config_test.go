package mockapi

import (
	"reflect"
	"testing"

	appconfig "postman-transform/backend-golang/internal/config"
)

func TestDefaultConfigUsesAppDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.UploadMaxBytes != appconfig.DefaultAppConfig().MockAPI.UploadMaxBytes {
		t.Fatalf("unexpected upload max: %d", cfg.UploadMaxBytes)
	}
	if cfg.PageSizeDefault != 25 || !reflect.DeepEqual(cfg.PageSizeOptions, []int{25, 50, 100}) {
		t.Fatalf("unexpected page config: %#v", cfg)
	}
}

func TestConfigFromAppUsesCentralValues(t *testing.T) {
	cfg := ConfigFromApp(appconfig.AppConfig{
		MockAPI: appconfig.MockAPIConfig{
			UploadMaxBytes:  8 * 1024 * 1024,
			PageSizeDefault: 30,
			PageSizeOptions: []int{10, 30, 10, -1},
		},
	})
	if cfg.UploadMaxBytes != 8*1024*1024 {
		t.Fatalf("unexpected upload max: %d", cfg.UploadMaxBytes)
	}
	if cfg.PageSizeDefault != 30 || !reflect.DeepEqual(cfg.PageSizeOptions, []int{10, 30}) {
		t.Fatalf("unexpected page config: %#v", cfg)
	}
}
