SHELL := /bin/sh

ADMIN_FEATURES := api-specs api-spec-editor audit-log config direct-request environment-variables health proxy-replay realtime shadow-compare snapshot featureflag
PUBLIC_FEATURES := health proxy
FEATURES := api-specs api-spec-editor audit-log config direct-request environment-variables health proxy proxy-replay realtime shadow-compare snapshot featureflag

BUILD_DIR ?= build/features
ADMIN_BUILD_DIR ?= $(BUILD_DIR)/admin
PUBLIC_BUILD_DIR ?= $(BUILD_DIR)/public
SINGLE_BUILD_DIR ?= $(BUILD_DIR)/single
GO ?= go
GOFLAGS ?=

ADMIN_PLUGIN_PKG_health := ./internal/features/health/adminplugin
PUBLIC_PLUGIN_PKG_health := ./internal/features/health/publicplugin

admin_plugin_pkg = $(if $(ADMIN_PLUGIN_PKG_$(1)),$(ADMIN_PLUGIN_PKG_$(1)),./internal/features/$(1)/plugin)
public_plugin_pkg = $(if $(PUBLIC_PLUGIN_PKG_$(1)),$(PUBLIC_PLUGIN_PKG_$(1)),./internal/features/$(1)/plugin)

.PHONY: help features admin-features public-features feature-% admin-feature-% public-feature-% list-features list-admin-features list-public-features print-plugin-env test clean

help:
	@echo "Feature plugin build targets:"
	@echo "  make features                 Build admin and public feature plugins"
	@echo "  make admin-features           Build admin plugins into $(ADMIN_BUILD_DIR)"
	@echo "  make public-features          Build public plugins into $(PUBLIC_BUILD_DIR)"
	@echo "  make admin-feature-<name>     Build one admin plugin, e.g. make admin-feature-api-specs"
	@echo "  make public-feature-<name>    Build one public plugin, e.g. make public-feature-proxy"
	@echo "  make feature-<name>           Build one generic plugin into $(SINGLE_BUILD_DIR)"
	@echo "  make print-plugin-env         Print env vars for loading built plugins"
	@echo "  make list-features            Print buildable feature names"
	@echo "  make test                     Run the Go test suite"
	@echo "  make clean                    Remove built feature plugins"
	@echo ""
	@echo "Options:"
	@echo "  BUILD_DIR=path                Override plugin output root"
	@echo "  ADMIN_BUILD_DIR=path          Override admin plugin output directory"
	@echo "  PUBLIC_BUILD_DIR=path         Override public plugin output directory"
	@echo "  SINGLE_BUILD_DIR=path         Override generic plugin output directory"
	@echo "  GOFLAGS='...'                 Extra flags passed to go build"

features: admin-features public-features

admin-features: $(ADMIN_FEATURES:%=$(ADMIN_BUILD_DIR)/%.so)

public-features: $(PUBLIC_FEATURES:%=$(PUBLIC_BUILD_DIR)/%.so)

feature-%:
	$(MAKE) $(SINGLE_BUILD_DIR)/$*.so

admin-feature-%:
	$(MAKE) $(ADMIN_BUILD_DIR)/$*.so

public-feature-%:
	$(MAKE) $(PUBLIC_BUILD_DIR)/$*.so


PLUGIN_GOFLAGS ?= -buildvcs=false
PLUGIN_LDFLAGS ?= -s -w -buildid=
PLUGIN_DYNLINK_DIR := $(shell printf '%s/pkg/%s_%s_dynlink' "$$($(GO) env GOROOT 2>/dev/null)" "$$($(GO) env GOOS 2>/dev/null)" "$$($(GO) env GOARCH 2>/dev/null)")
PLUGIN_LINKSHARED := $(shell test -d "$(PLUGIN_DYNLINK_DIR)" && echo -linkshared)
PLUGIN_BUILDFLAGS ?= -buildmode=plugin $(PLUGIN_LINKSHARED)

$(SINGLE_BUILD_DIR)/%.so:
	@mkdir -p $(SINGLE_BUILD_DIR)
	$(GO) build $(GOFLAGS) $(PLUGIN_GOFLAGS) $(PLUGIN_BUILDFLAGS) -ldflags="$(PLUGIN_LDFLAGS)" -o $@ ./internal/features/$*/plugin

$(ADMIN_BUILD_DIR)/%.so:
	@mkdir -p $(ADMIN_BUILD_DIR)
	$(GO) build $(GOFLAGS) $(PLUGIN_GOFLAGS) $(PLUGIN_BUILDFLAGS) -ldflags="$(PLUGIN_LDFLAGS)" -o $@ $(call admin_plugin_pkg,$*)

$(PUBLIC_BUILD_DIR)/%.so:
	@mkdir -p $(PUBLIC_BUILD_DIR)
	$(GO) build $(GOFLAGS) $(PLUGIN_GOFLAGS) $(PLUGIN_BUILDFLAGS) -ldflags="$(PLUGIN_LDFLAGS)" -o $@ $(call public_plugin_pkg,$*)

list-features:
	@printf '%s\n' $(FEATURES)

list-admin-features:
	@printf '%s\n' $(ADMIN_FEATURES)

list-public-features:
	@printf '%s\n' $(PUBLIC_FEATURES)

print-plugin-env:
	@echo "ADMIN_FEATURE_PLUGINS=$(ADMIN_BUILD_DIR)/*.so"
	@echo "PUBLIC_FEATURE_PLUGINS=$(PUBLIC_BUILD_DIR)/*.so"

test:
	$(GO) test ./...

clean:
	rm -rf $(BUILD_DIR)
