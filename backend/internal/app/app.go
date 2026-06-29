package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/config"
	Container "postman-transform/backend-golang/internal/container"
	"postman-transform/backend-golang/internal/database"
	auditlog "postman-transform/backend-golang/internal/features/audit-log"
	"postman-transform/backend-golang/internal/features/featureflag"
)

func Run(ctx context.Context) error {
	cfg := config.Load()
	db, err := database.Open(cfg.DatabaseURL, cfg.SQLProvider)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.EnsureConnectionSchema(db); err != nil {
		return err
	}
	auditDB := db
	if cfg.AuditDatabaseURL != cfg.DatabaseURL || database.NormalizeProvider(cfg.AuditSQLProvider) != database.NormalizeProvider(cfg.SQLProvider) {
		auditDB, err = database.Open(cfg.AuditDatabaseURL, cfg.AuditSQLProvider)
		if err != nil {
			return err
		}
		defer auditDB.Close()
		if err := database.EnsureConnectionSchema(auditDB); err != nil {
			return err
		}
	}

	serviceContainer := Container.NewContainer()
	if err := registerInfrastructureContainers(serviceContainer, db, auditDB); err != nil {
		return err
	}

	adminEngine := gin.New()
	adminEngine.Use(gin.Logger(), gin.Recovery(), cors.New(cors.Config{
		AllowOrigins:     cfg.CORSAllowList,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "x-mcp-token"},
		AllowCredentials: cfg.CORSAllowCredentials,
		MaxAge:           12 * time.Hour,
	}))
	api := adminEngine.Group("/api/v1")

	publicEngine := gin.New()
	publicEngine.Use(gin.Logger(), gin.Recovery())
	if err := configureFeatureModules(serviceContainer, cfg, api, publicEngine); err != nil {
		return err
	}

	if err := publishLoadedFeatures(serviceContainer); err != nil {
		return err
	}

	logLoadedFeatureModules(cfg)

	adminServer := &http.Server{Addr: fmt.Sprintf(":%d", cfg.AdminPort), Handler: adminEngine}
	publicServer := &http.Server{Addr: fmt.Sprintf(":%d", cfg.PublicPort), Handler: publicEngine}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("Admin server listening on http://localhost:%d", cfg.AdminPort)
		errCh <- adminServer.ListenAndServe()
	}()
	go func() {
		log.Printf("Public server listening on http://localhost:%d", cfg.PublicPort)
		errCh <- publicServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = adminServer.Shutdown(shutdownCtx)
		_ = publicServer.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func publishLoadedFeatures(beans *Container.Container) error {
	var featureFlagService *featureflag.Service
	ok, err := beans.GetOptional(&featureFlagService)
	if err != nil {
		return err
	}
	if !ok || featureFlagService == nil {
		return nil
	}
	featureFlagService.Put(beans.List())
	return nil
}

func startAuditRetention(ctx context.Context, service *auditlog.Service, retentionDays, intervalMinutes int) {
	if retentionDays <= 0 || intervalMinutes <= 0 {
		return
	}
	prune := func() {
		deleted, err := service.PruneBefore(ctx, time.Now().AddDate(0, 0, -retentionDays))
		if err != nil {
			log.Printf("Audit log retention failed: %v", err)
			return
		}
		if deleted > 0 {
			log.Printf("Audit log retention deleted %d records", deleted)
		}
	}
	prune()
	go func() {
		ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				prune()
			}
		}
	}()
}
