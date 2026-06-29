package mockapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/internal/features/realtime"
)

func TestRoutePathsMatchSupportsWildcardSegmentsAndIgnoresQuery(t *testing.T) {
	if !RoutePathsMatch("/users/:id?expand=true", "/users/42?expand=false") {
		t.Fatal("expected wildcard route to match requested route")
	}
	if RoutePathsMatch("/users/:id/profile", "/users/42") {
		t.Fatal("expected routes with different segment counts not to match")
	}
}

func TestValidateRequestKeysReportsMissingUnexpectedAndTypeMismatches(t *testing.T) {
	mock := APIRecord{
		Method:              "POST",
		ExpectedRequestKeys: []string{"active", "name"},
		ExpectedRequestTypes: RequestBodyTypes{
			"active": "boolean",
			"name":   "string",
		},
	}
	result := ValidateRequestKeys(mock, map[string]any{"name": 42, "extra": true})
	if result.OK {
		t.Fatal("expected body validation to fail")
	}
	if len(result.MissingKeys) != 1 || result.MissingKeys[0] != "active" {
		t.Fatalf("unexpected missing keys: %#v", result.MissingKeys)
	}
	if len(result.UnexpectedKeys) != 1 || result.UnexpectedKeys[0] != "extra" {
		t.Fatalf("unexpected unexpected keys: %#v", result.UnexpectedKeys)
	}
	if len(result.TypeMismatches) != 1 || result.TypeMismatches[0].Key != "name" {
		t.Fatalf("unexpected type mismatches: %#v", result.TypeMismatches)
	}
}

func TestValidateRequestParamsIgnoresWildcardRouteParam(t *testing.T) {
	mock := APIRecord{
		Method:            "GET",
		ResolvedRoutePath: "/users/:id",
		ExpectedParamKeys: []string{"id", "expand"},
	}
	result := ValidateRequestParams(mock, map[string][]string{"expand": []string{"profile"}})
	if !result.OK {
		t.Fatalf("expected params to be valid: %#v", result)
	}
}

func TestListReturnsEmptyArraysForMissingSpecFields(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))

	err := service.repo.ImportDefinitions(context.Background(), []Definition{{
		CollectionName:   "Demo",
		RouteName:        "Ping",
		Method:           "GET",
		RoutePath:        "/ping",
		ResponseExamples: []ResponseExample{{Status: 200, Name: "OK", Body: "{}"}},
	}})
	if err != nil {
		t.Fatalf("ImportDefinitions returned error: %v", err)
	}

	result, err := service.List(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected one item, got %d", len(result.Items))
	}
	payload, err := json.Marshal(result.Items[0])
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if string(payload) == "" || jsonContainsNullSpecField(payload) {
		t.Fatalf("spec fields should serialize as empty arrays/maps, got %s", payload)
	}
}

func TestImportSwaggerImportsOpenAPIOperations(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	payload := []byte(`{
		"openapi": "3.0.3",
		"info": {"title": "Swagger Demo", "version": "1.0.0"},
		"paths": {
			"/users/{id}": {
				"get": {
					"summary": "Get user",
					"tags": ["Users"],
					"parameters": [{"name": "id", "in": "path"}, {"name": "expand", "in": "query"}],
					"responses": {
						"200": {
							"description": "OK",
							"content": {"application/json": {"example": {"id": 1, "name": "Ada"}}}
						}
					}
				}
			}
		}
	}`)

	name, count, err := service.ImportSwagger(context.Background(), payload)
	if err != nil {
		t.Fatalf("ImportSwagger returned error: %v", err)
	}
	if name != "Swagger Demo" || count != 1 {
		t.Fatalf("unexpected import summary: %s %d", name, count)
	}
	result, err := service.List(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	got := result.Items[0]
	if got.RoutePath != "/users/:id" || got.Method != "GET" || got.ResponseStatus != 200 {
		t.Fatalf("unexpected imported API: %+v", got)
	}
	if len(got.ExpectedParamKeys) != 2 || got.ExpectedParamKeys[0] != "expand" || got.ExpectedParamKeys[1] != "id" {
		t.Fatalf("unexpected parameter keys: %#v", got.ExpectedParamKeys)
	}
}

func TestImportSwaggerInfersRequestBodyKeysFromJSONExample(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	payload := []byte(`{
		"openapi": "3.0.3",
		"info": {"title": "Swagger JSON Body", "version": "1.0.0"},
		"paths": {
			"/users": {
				"post": {
					"summary": "Create user",
					"requestBody": {
						"content": {
							"application/json": {
								"example": {
									"active": true,
									"name": "Ada",
									"profile": {"tier": "admin"},
									"tags": ["admin"]
								}
							}
						}
					},
					"responses": {"201": {"description": "Created"}}
				}
			}
		}
	}`)

	_, _, err := service.ImportSwagger(context.Background(), payload)
	if err != nil {
		t.Fatalf("ImportSwagger returned error: %v", err)
	}
	result, err := service.List(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	got := result.Items[0]
	if len(got.ExpectedRequestKeys) != 4 || got.ExpectedRequestKeys[0] != "active" || got.ExpectedRequestKeys[1] != "name" || got.ExpectedRequestKeys[2] != "profile" || got.ExpectedRequestKeys[3] != "tags" {
		t.Fatalf("unexpected request body keys: %#v", got.ExpectedRequestKeys)
	}
	if got.ExpectedRequestTypes["active"] != "boolean" || got.ExpectedRequestTypes["name"] != "string" || got.ExpectedRequestTypes["profile"] != "object" || got.ExpectedRequestTypes["tags"] != "array" {
		t.Fatalf("unexpected request body types: %#v", got.ExpectedRequestTypes)
	}
}

func TestImportSwaggerInfersRequestBodyKeysFromBodyParameterSchema(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	payload := []byte(`{
		"swagger": "2.0",
		"info": {"title": "Swagger Body Parameter", "version": "1.0.0"},
		"paths": {
			"/case": {
				"post": {
					"summary": "Create case",
					"parameters": [{
						"name": "body",
						"in": "body",
						"schema": {
							"type": "object",
							"properties": {
								"device_serial": {"type": "string"},
								"operator_user_ids": {"type": "array", "items": {"type": "string"}},
								"active": {"type": "boolean"}
							}
						}
					}],
					"responses": {"200": {"description": "SUCCESS"}}
				}
			}
		}
	}`)

	_, _, err := service.ImportSwagger(context.Background(), payload)
	if err != nil {
		t.Fatalf("ImportSwagger returned error: %v", err)
	}
	result, err := service.List(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	got := result.Items[0]
	if len(got.ExpectedRequestKeys) != 3 || got.ExpectedRequestKeys[0] != "active" || got.ExpectedRequestKeys[1] != "device_serial" || got.ExpectedRequestKeys[2] != "operator_user_ids" {
		t.Fatalf("unexpected request body keys: %#v", got.ExpectedRequestKeys)
	}
	if got.ExpectedRequestTypes["device_serial"] != "string" || got.ExpectedRequestTypes["operator_user_ids"] != "array" || got.ExpectedRequestTypes["active"] != "boolean" {
		t.Fatalf("unexpected request body types: %#v", got.ExpectedRequestTypes)
	}
}

func TestManualSpecCreateAndUpdate(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))

	created, err := service.CreateManual(context.Background(), ManualSpecInput{
		Method:          "post",
		RoutePath:       "users/{{tenantId}}",
		RequestBodyKeys: []string{"name", "active"},
		RequestBodyTypes: RequestBodyTypes{
			"name":   "string",
			"active": "boolean",
		},
		RequestParamKeys: []string{"expand"},
		MockEnabled:      true,
		ProxyEnabled:     true,
		ResponseStatus:   201,
		ResponseExamples: []ResponseExample{
			{Status: 201, Name: "Created", Body: `{"id":1}`},
			{Status: 409, Name: "Conflict", Body: `{"message":"duplicate"}`},
		},
	})
	if err != nil {
		t.Fatalf("CreateManual returned error: %v", err)
	}
	if created.Method != "POST" || created.RoutePath != "/users/{{tenantId}}" || created.ResponseStatus != 201 {
		t.Fatalf("unexpected created API: %+v", created)
	}
	if len(created.ResponseExamples) != 2 || created.ExpectedRequestTypes["active"] != "boolean" {
		t.Fatalf("unexpected created spec fields: %+v", created)
	}

	updated, err := service.UpdateManual(context.Background(), created.ID, ManualSpecInput{
		CollectionName:   "Manual",
		RouteName:        "Update user",
		Method:           "PATCH",
		RoutePath:        "/users/:id",
		MockEnabled:      false,
		ProxyEnabled:     true,
		ResponseStatus:   204,
		ResponseExamples: []ResponseExample{{Status: 204, Name: "No Content", Body: ""}},
	})
	if err != nil {
		t.Fatalf("UpdateManual returned error: %v", err)
	}
	if updated.RouteName != "Update user" || updated.Method != "PATCH" || updated.RoutePath != "/users/:id" || updated.MockEnabled {
		t.Fatalf("unexpected updated API: %+v", updated)
	}
}

func TestManualSpecDuplicateRouteUpdatesExistingAndMergesExamples(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	first, err := service.CreateManual(context.Background(), ManualSpecInput{Method: "GET", RoutePath: "/ping", MockEnabled: true, ProxyEnabled: true, ResponseExamples: []ResponseExample{{Status: 200, Name: "OK", Body: "{}"}}})
	if err != nil {
		t.Fatalf("first CreateManual returned error: %v", err)
	}
	second, err := service.CreateManual(context.Background(), ManualSpecInput{Method: "GET", RoutePath: "/ping", MockEnabled: true, ProxyEnabled: true, ResponseExamples: []ResponseExample{{Status: 201, Name: "Created", Body: "{}"}}})
	if err != nil {
		t.Fatalf("second CreateManual returned error: %v", err)
	}

	reloadedFirst, err := service.Get(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if second.ID != first.ID || reloadedFirst.ID != first.ID {
		t.Fatalf("expected duplicate create to update existing API, first=%s second=%s reloaded=%s", first.ID, second.ID, reloadedFirst.ID)
	}
	if !reloadedFirst.MockEnabled || reloadedFirst.ProxyEnabled {
		t.Fatalf("expected updated existing mock to remain active as mock only, got %+v", reloadedFirst)
	}
	if reloadedFirst.ResponseStatus != 201 || len(reloadedFirst.ResponseExamples) != 2 {
		t.Fatalf("expected duplicate response status to be added as an example, got %+v", reloadedFirst)
	}

	updatedFirst, err := service.SetProxyEnabled(context.Background(), first.ID, true)
	if err != nil {
		t.Fatalf("SetProxyEnabled returned error: %v", err)
	}
	if updatedFirst.MockEnabled || !updatedFirst.ProxyEnabled {
		t.Fatalf("expected first duplicate proxy to be active, got %+v", updatedFirst)
	}
}

func TestCollectionsListsDistinctNames(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	for _, input := range []ManualSpecInput{
		{CollectionName: "Checkout", Method: "GET", RoutePath: "/orders", MockEnabled: true, ResponseExamples: []ResponseExample{{Status: 200, Name: "OK", Body: "{}"}}},
		{CollectionName: "Users", Method: "GET", RoutePath: "/users", MockEnabled: true, ResponseExamples: []ResponseExample{{Status: 200, Name: "OK", Body: "{}"}}},
		{CollectionName: "Checkout", Method: "POST", RoutePath: "/orders", MockEnabled: true, ResponseExamples: []ResponseExample{{Status: 201, Name: "Created", Body: "{}"}}},
	} {
		if _, err := service.CreateManual(context.Background(), input); err != nil {
			t.Fatalf("CreateManual returned error: %v", err)
		}
	}

	result, err := service.Collections(context.Background())
	if err != nil {
		t.Fatalf("Collections returned error: %v", err)
	}
	if strings.Join(result.Collections, ",") != "Checkout,Users" {
		t.Fatalf("unexpected collections: %#v", result.Collections)
	}
}

func TestDirectoryRejectsTraversalPathSegments(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))

	for _, path := range [][]string{
		{".."},
		{"."},
		{"api/../secret"},
		{"api\\secret"},
		{"api", ".."},
		{"api", "\x00secret"},
	} {
		if _, err := service.Directory(context.Background(), "path", path); !errors.Is(err, ErrInvalidDirectoryPath) {
			t.Fatalf("expected ErrInvalidDirectoryPath for %#v, got %v", path, err)
		}
	}
}

func TestDirectoryHandlerReturnsBadRequestForInvalidPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	router := gin.New()
	NewHandler(service, realtime.NewHub()).Register(router.Group(""))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/mock/apis/directory?mode=path&path="+url.QueryEscape(`["api",".."]`), nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRevertVersionRestoresSelectedSnapshot(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))

	if _, err := service.CreateManual(context.Background(), ManualSpecInput{CollectionName: "Versioned", RouteName: "One", Method: "GET", RoutePath: "/one", MockEnabled: true, ResponseExamples: []ResponseExample{{Status: 200, Name: "OK", Body: `{"one":true}`}}}); err != nil {
		t.Fatalf("CreateManual first returned error: %v", err)
	}
	firstVersion, err := service.CreateVersion(context.Background(), "first")
	if err != nil {
		t.Fatalf("CreateVersion returned error: %v", err)
	}
	if _, err := service.CreateManual(context.Background(), ManualSpecInput{CollectionName: "Versioned", RouteName: "Two", Method: "GET", RoutePath: "/two", MockEnabled: true, ResponseExamples: []ResponseExample{{Status: 200, Name: "OK", Body: `{"two":true}`}}}); err != nil {
		t.Fatalf("CreateManual second returned error: %v", err)
	}

	reverted, err := service.RevertVersion(context.Background(), firstVersion.ID)
	if err != nil {
		t.Fatalf("RevertVersion returned error: %v", err)
	}
	if reverted.APICount != 1 {
		t.Fatalf("expected reverted version to contain one API, got %d", reverted.APICount)
	}
	result, err := service.List(context.Background(), 1, 25)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Total != 1 || result.Items[0].RoutePath != "/one" {
		t.Fatalf("expected current APIs to equal selected snapshot, got %+v", result.Items)
	}
	record, err := service.GetVersion(context.Background(), reverted.ID)
	if err != nil {
		t.Fatalf("GetVersion returned error: %v", err)
	}
	if record == nil || len(record.Snapshot) != 1 || record.Snapshot[0].RoutePath != "/one" {
		t.Fatalf("expected new revert commit snapshot to equal selected snapshot, got %+v", record)
	}
}

func TestExportSpecSupportsPostmanAndSwagger(t *testing.T) {
	db := testDB(t)
	service := NewService(NewRepository(db), environment.NewService(db))
	err := service.repo.ImportDefinitions(context.Background(), []Definition{{
		CollectionName:    "Demo",
		RouteName:         "Get user",
		PostmanFolderPath: []string{"Users"},
		Method:            "GET",
		RoutePath:         "/users/:id",
		ResponseExamples:  []ResponseExample{{Status: 200, Name: "OK", Body: `{"id":1}`}},
		RequestParamKeys:  []string{"id"},
	}})
	if err != nil {
		t.Fatalf("ImportDefinitions returned error: %v", err)
	}

	postmanPayload, postmanFile, _, err := service.ExportSpec(context.Background(), "postman")
	if err != nil {
		t.Fatalf("ExportSpec postman returned error: %v", err)
	}
	if !regexp.MustCompile(`^collection-spec-\d{8}-\d{6}\.json$`).MatchString(postmanFile) || !json.Valid(postmanPayload) {
		t.Fatalf("unexpected collection export: %s %s", postmanFile, postmanPayload)
	}

	swaggerPayload, swaggerFile, _, err := service.ExportSpec(context.Background(), "swagger")
	if err != nil {
		t.Fatalf("ExportSpec swagger returned error: %v", err)
	}
	if !regexp.MustCompile(`^swagger-spec-\d{8}-\d{6}\.json$`).MatchString(swaggerFile) || !json.Valid(swaggerPayload) || !jsonContainsString(swaggerPayload, "/users/{id}") {
		t.Fatalf("unexpected Swagger export: %s %s", swaggerFile, swaggerPayload)
	}
}

func jsonContainsNullSpecField(payload []byte) bool {
	var record map[string]any
	if err := json.Unmarshal(payload, &record); err != nil {
		return true
	}
	for _, key := range []string{"postmanFolderPath", "responseExamples", "expectedRequestKeys", "expectedRequestTypes", "expectedParamKeys"} {
		if record[key] == nil {
			return true
		}
	}
	return false
}

func jsonContainsString(payload []byte, value string) bool {
	return strings.Contains(string(payload), value)
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
