package auditlog

import (
	"sort"
	"strconv"

	appconfig "postman-transform/backend-golang/internal/config"
)

type Config struct {
	PageSizeDefault int
	PageSizeOptions []int
}

func DefaultConfig() Config {
	return ConfigFromApp(appconfig.DefaultAppConfig())
}

func ConfigFromApp(app appconfig.AppConfig) Config {
	return normalizeConfig(configFromApp(app))
}

func configFromApp(app appconfig.AppConfig) Config {
	return Config{
		PageSizeDefault: app.AuditLog.PageSizeDefault,
		PageSizeOptions: append([]int{}, app.AuditLog.PageSizeOptions...),
	}
}

func (c Config) HasPageSize(pageSize int) bool {
	for _, option := range c.PageSizeOptions {
		if option == pageSize {
			return true
		}
	}
	return false
}

func (c Config) PageSizeOptionsText() string {
	if len(c.PageSizeOptions) == 0 {
		return ""
	}
	result := strconv.Itoa(c.PageSizeOptions[0])
	for _, option := range c.PageSizeOptions[1:] {
		result += ", " + strconv.Itoa(option)
	}
	return result
}

func normalizeConfig(cfg Config) Config {
	defaults := configFromApp(appconfig.DefaultAppConfig())
	if cfg.PageSizeDefault <= 0 {
		cfg.PageSizeDefault = defaults.PageSizeDefault
	}
	if len(cfg.PageSizeOptions) == 0 {
		cfg.PageSizeOptions = append([]int{}, defaults.PageSizeOptions...)
	}
	cfg.PageSizeOptions = normalizePageSizeOptions(cfg.PageSizeOptions, cfg.PageSizeDefault)
	return cfg
}

func normalizePageSizeOptions(options []int, defaultPageSize int) []int {
	seen := map[int]bool{}
	normalized := make([]int, 0, len(options)+1)
	for _, option := range options {
		if option > 0 && !seen[option] {
			normalized = append(normalized, option)
			seen[option] = true
		}
	}
	if defaultPageSize > 0 && !seen[defaultPageSize] {
		normalized = append(normalized, defaultPageSize)
	}
	if len(normalized) == 0 {
		normalized = append(normalized, DefaultConfig().PageSizeDefault)
	}
	sort.Ints(normalized)
	return normalized
}
