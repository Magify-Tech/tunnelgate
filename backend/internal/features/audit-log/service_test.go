package auditlog

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"postman-transform/backend-golang/internal/database"
)

func TestCreateAndListAuditLogWithShadow(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	status := 200
	apiID := 7

	created, err := service.Create(context.Background(), CreateInput{
		APIMockDBID:     &apiID,
		RouteName:       "Get users",
		Method:          "GET",
		RoutePath:       "/users",
		TargetURL:       "http://real.test/users",
		ResponseStatus:  &status,
		DurationMS:      12,
		Success:         true,
		RequestHeaders:  "{}",
		RequestBody:     "",
		ResponseHeaders: "{}",
		ResponseBody:    "[]",
		ShadowTargets:   []ShadowTarget{{ID: "shadow-1", Name: "Shadow", BaseURL: "http://shadow.test", TargetURL: "http://shadow.test/users"}},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.ID == "" || created.InternalID == 0 || created.APIMockDBID == nil || *created.APIMockDBID != apiID {
		t.Fatalf("unexpected created record: %#v", created)
	}

	if err := service.CreateShadow(context.Background(), created.InternalID, ShadowEntry{ID: "shadow-1", Name: "Shadow", BaseURL: "http://shadow.test", TargetURL: "http://shadow.test/users", ResponseStatus: &status, DurationMS: 10, Success: true, RequestHeaders: "{}", ResponseHeaders: "{}", ResponseBody: "[]"}); err != nil {
		t.Fatalf("CreateShadow returned error: %v", err)
	}

	list, err := service.List(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || len(list.Items[0].ShadowEntries) != 1 {
		t.Fatalf("unexpected list result: %#v", list)
	}
}

func TestGetAuditLogIncludesPostmanExamples(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	ctx := context.Background()
	apiID, publicID := insertMockAPI(t, db)
	status := 201
	insertProjectVersion(t, db, "snapshot-with-users", []snapshotAPIRecord{
		{
			ID:             publicID,
			Method:         "POST",
			RoutePath:      "/users",
			ResponseStatus: 201,
			ResponseExamples: []snapshotResponseExample{
				{Status: 200, Name: "OK", Body: `{"snapshotOk":true}`},
				{Status: 201, Name: "Created", Body: `{"snapshotId":1}`},
			},
		},
	})

	created, err := service.Create(ctx, CreateInput{
		APIMockDBID:     &apiID,
		APIMockID:       publicID,
		RouteName:       "Create user",
		Method:          "POST",
		RoutePath:       "/users",
		TargetURL:       "http://real.test/users",
		ResponseStatus:  &status,
		DurationMS:      18,
		Success:         true,
		RequestHeaders:  "{}",
		RequestBody:     `{"name":"Ada"}`,
		ResponseHeaders: "{}",
		ResponseBody:    `{"id":1}`,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	got, err := service.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.PostmanExample == nil || got.PostmanExample.Status != 201 || got.PostmanExample.ResponseBody != `{"id":1}` {
		t.Fatalf("unexpected selected postman example: %#v", got.PostmanExample)
	}
	if len(got.PostmanExamples) != 2 || got.PostmanExamples[0].Status != 200 || got.PostmanExamples[1].Status != 201 {
		t.Fatalf("unexpected postman examples: %#v", got.PostmanExamples)
	}
	if got.APIMockID == nil || *got.APIMockID != publicID {
		t.Fatalf("unexpected public api mock id: %#v", got.APIMockID)
	}
}

func TestGetAuditLogUsesCurrentSpecWhenLatestSnapshotDeletedSpec(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	ctx := context.Background()
	apiID, publicID := insertMockAPI(t, db)
	status := 200
	insertProjectVersion(t, db, "snapshot-with-users", []snapshotAPIRecord{
		{
			ID:             publicID,
			Method:         "POST",
			RoutePath:      "/users",
			ResponseStatus: 200,
			ResponseExamples: []snapshotResponseExample{
				{Status: 200, Name: "OK", Body: `{"ok":true}`},
			},
		},
	})

	created, err := service.Create(ctx, CreateInput{
		APIMockDBID:     &apiID,
		APIMockID:       publicID,
		RouteName:       "Create user",
		Method:          "POST",
		RoutePath:       "/users",
		TargetURL:       "http://real.test/users",
		ResponseStatus:  &status,
		DurationMS:      18,
		Success:         true,
		RequestHeaders:  "{}",
		RequestBody:     `{"name":"Ada"}`,
		ResponseHeaders: "{}",
		ResponseBody:    `{"ok":true}`,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	insertProjectVersion(t, db, "snapshot-after-delete", []snapshotAPIRecord{})

	got, err := service.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.PostmanExample == nil || len(got.PostmanExamples) != 2 {
		t.Fatalf("expected current spec examples despite latest snapshot delete, got selected=%#v examples=%#v", got.PostmanExample, got.PostmanExamples)
	}
}

func TestGetAuditLogDoesNotUseSnapshotRouteWithoutCurrentSpec(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	ctx := context.Background()
	apiID, publicID := insertMockAPI(t, db)
	status := 201
	insertProjectVersion(t, db, "snapshot-with-users", []snapshotAPIRecord{
		{
			ID:                publicID,
			Method:            "POST",
			RoutePath:         "/users/{{tenantId}}",
			ResolvedRoutePath: "/users/acme",
			ResponseStatus:    201,
			ResponseExamples: []snapshotResponseExample{
				{Status: 201, Name: "Created", Body: `{"snapshotId":1}`},
			},
		},
	})

	created, err := service.Create(ctx, CreateInput{
		APIMockDBID:     &apiID,
		RouteName:       "Create user",
		Method:          "POST",
		RoutePath:       "/users/acme",
		TargetURL:       "http://real.test/users",
		ResponseStatus:  &status,
		DurationMS:      18,
		Success:         true,
		RequestHeaders:  "{}",
		RequestBody:     `{"name":"Ada"}`,
		ResponseHeaders: "{}",
		ResponseBody:    `{"id":1}`,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM api_mocks WHERE id = ?`, apiID); err != nil {
		t.Fatalf("delete api mock returned error: %v", err)
	}

	got, err := service.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if len(got.PostmanExamples) != 0 || got.APIMockID != nil {
		t.Fatalf("expected no snapshot fallback without current spec, got examples=%#v apiMockID=%#v", got.PostmanExamples, got.APIMockID)
	}
}

func TestPruneBeforeDeletesOldAuditLogs(t *testing.T) {
	db := testDB(t)
	service := NewService(db)
	ctx := context.Background()
	status := 200

	oldRecord, err := service.Create(ctx, CreateInput{
		RouteName:       "Old",
		Method:          "GET",
		RoutePath:       "/old",
		TargetURL:       "http://real.test/old",
		ResponseStatus:  &status,
		DurationMS:      1,
		Success:         true,
		RequestHeaders:  "{}",
		RequestBody:     "",
		ResponseHeaders: "{}",
		ResponseBody:    "{}",
	})
	if err != nil {
		t.Fatalf("Create old returned error: %v", err)
	}
	if _, err := service.Create(ctx, CreateInput{
		RouteName:       "New",
		Method:          "GET",
		RoutePath:       "/new",
		TargetURL:       "http://real.test/new",
		ResponseStatus:  &status,
		DurationMS:      1,
		Success:         true,
		RequestHeaders:  "{}",
		RequestBody:     "",
		ResponseHeaders: "{}",
		ResponseBody:    "{}",
	}); err != nil {
		t.Fatalf("Create new returned error: %v", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE proxy_audit_logs SET created_at = ? WHERE id = ?`, formatTimestamp(time.Now().AddDate(0, 0, -10)), oldRecord.InternalID)
	if err != nil {
		t.Fatalf("update old record returned error: %v", err)
	}

	deleted, err := service.PruneBefore(ctx, time.Now().AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("PruneBefore returned error: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one deleted record, got %d", deleted)
	}
	count, err := service.Count(ctx)
	if err != nil {
		t.Fatalf("Count returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one remaining record, got %d", count)
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

func insertMockAPI(t *testing.T, db *database.Connection) (int, string) {
	t.Helper()
	publicID := "018f25a0-0000-7000-8000-000000000001"
	result, err := db.DB.Exec(`
		INSERT INTO api_mocks (
			public_id, collection_name, route_name, postman_folder_path, method, route_path,
			response_status, response_body, request_body_keys, request_body_types, request_param_keys, request_param_types
		)
		VALUES (?, 'Users', 'Create user', '[]', 'POST', '/users', 201, '{"id":1}', '[]', '{}', '[]', '{}')
	`, publicID)
	if err != nil {
		t.Fatalf("insert api mock returned error: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId returned error: %v", err)
	}
	_, err = db.DB.Exec(`
		INSERT INTO api_mock_responses (api_mock_id, response_status, response_name, response_body, sort_order)
		VALUES
			(?, 200, 'OK', '{"ok":true}', 0),
			(?, 201, 'Created', '{"id":1}', 1)
	`, id, id)
	if err != nil {
		t.Fatalf("insert api mock responses returned error: %v", err)
	}
	return int(id), publicID
}

func insertProjectVersion(t *testing.T, db *database.Connection, publicID string, snapshot []snapshotAPIRecord) {
	t.Helper()
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot returned error: %v", err)
	}
	_, err = db.DB.Exec(`
		INSERT INTO api_mock_versions (public_id, message, snapshot_json, api_count, created_at)
		VALUES (?, 'Snapshot', ?, ?, ?)
	`, publicID, string(payload), len(snapshot), formatTimestamp(time.Now()))
	if err != nil {
		t.Fatalf("insert project version returned error: %v", err)
	}
}
