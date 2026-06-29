package environment

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"postman-transform/backend-golang/internal/database"
)

func TestImportPayloadSkipsDisabledVariables(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	payload := []byte(`{"name":"Local","values":[{"key":"API_URL","value":"http://localhost","enabled":true},{"key":"SECRET","value":"skip","enabled":false}]}`)

	name, count, err := service.ImportPayload(context.Background(), payload)
	if err != nil {
		t.Fatalf("ImportPayload returned error: %v", err)
	}
	if name != "Local" || count != 1 {
		t.Fatalf("unexpected import summary: %s %d", name, count)
	}
	variables, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if variables["API_URL"] != "http://localhost" {
		t.Fatalf("expected API_URL to be imported, got %#v", variables)
	}
	if _, exists := variables["SECRET"]; exists {
		t.Fatalf("disabled variable should not be imported: %#v", variables)
	}
}

func TestExportPostmanEnvironment(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	if _, err := service.Upsert(context.Background(), "API_URL", "http://localhost"); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	payload, err := service.ExportPostmanEnvironment(context.Background())
	if err != nil {
		t.Fatalf("ExportPostmanEnvironment returned error: %v", err)
	}
	if !json.Valid(payload) || !strings.Contains(string(payload), `"key": "API_URL"`) {
		t.Fatalf("unexpected export payload: %s", payload)
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
