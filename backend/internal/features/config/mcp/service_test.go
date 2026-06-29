package mcp

import (
	"context"
	"testing"

	"postman-transform/backend-golang/internal/database"
)

func TestUpdateNormalizesMCPOrigins(t *testing.T) {
	db := testDB(t)
	service := NewService(db)

	config, err := service.Update(context.Background(), true, "token", []string{"https://client.test/path", "https://client.test/other"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !config.Enabled || config.ClientToken != "token" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if len(config.AllowedOrigins) != 1 || config.AllowedOrigins[0] != "https://client.test" {
		t.Fatalf("unexpected origins: %#v", config.AllowedOrigins)
	}
}

func testDB(t *testing.T) *database.Connection {
	t.Helper()
	db, err := database.Open("file:"+t.TempDir()+"/test.sqlite", "sqlite")
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if err := database.EnsureConnectionSchema(db); err != nil {
		t.Fatalf("EnsureSchema returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
