package mockapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/uuidv7"
)

type Repository struct {
	db *database.Connection
}

func NewRepository(db *database.Connection) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ImportDefinitions(ctx context.Context, definitions []Definition) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, definition := range definitions {
		if err := upsertDefinition(ctx, tx, definition); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) SaveManual(ctx context.Context, id *string, definition Definition, mockEnabled, proxyEnabled bool) (*StoredAPI, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	timestamp := databaseTimestamp(time.Now())
	response := definition.ResponseExamples[0]
	for _, example := range definition.ResponseExamples {
		if example.Status == definition.ResponseStatus {
			response = example
			break
		}
	}

	folderJSON, _ := json.Marshal(nonNilStringSlice(definition.PostmanFolderPath))
	keysJSON, _ := json.Marshal(nonNilStringSlice(definition.RequestBodyKeys))
	typesJSON, _ := json.Marshal(nonNilRequestBodyTypes(definition.RequestBodyTypes))
	paramsJSON, _ := json.Marshal(nonNilStringSlice(definition.RequestParamKeys))
	paramTypesJSON, _ := json.Marshal(nonNilRequestBodyTypes(definition.RequestParamTypes))

	var apiID int
	var publicID string
	if id != nil {
		result, err := tx.ExecContext(ctx, `
			UPDATE api_mocks
			SET collection_name = ?, route_name = ?, postman_folder_path = ?, method = ?,
				route_path = ?, mock_enabled = ?, proxy_enabled = ?, response_status = ?,
				response_body = ?, response_body_type = ?, request_headers = ?, response_headers = ?, request_body_keys = ?, request_body_types = ?,
				request_body_raw = ?, request_body_type = ?, request_param_keys = ?, request_param_types = ?, updated_at = ?
			WHERE public_id = ?
		`, definition.CollectionName, definition.RouteName, string(folderJSON), definition.Method, definition.RoutePath, database.BoolValue(mockEnabled), database.BoolValue(proxyEnabled), response.Status, response.Body, response.BodyType, definition.RequestHeaders, response.ResponseHeaders, string(keysJSON), string(typesJSON), definition.RequestBodyRaw, definition.RequestBodyType, string(paramsJSON), string(paramTypesJSON), timestamp, *id)
		if err != nil {
			return nil, err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil, nil
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM api_mocks WHERE public_id = ?`, *id).Scan(&apiID); err != nil {
			return nil, err
		}
		publicID = *id
	} else {
		generatedID, err := uuidv7.New()
		if err != nil {
			return nil, err
		}
		publicID = generatedID
		_, err = tx.ExecContext(ctx, `
			INSERT INTO api_mocks (
				public_id, collection_name, route_name, postman_folder_path, method, route_path,
				mock_enabled, proxy_enabled, response_status, response_body, response_body_type,
				request_headers, response_headers, request_body_keys, request_body_types, request_body_raw, request_body_type, request_param_keys, request_param_types, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, publicID, definition.CollectionName, definition.RouteName, string(folderJSON), definition.Method, definition.RoutePath, database.BoolValue(mockEnabled), database.BoolValue(proxyEnabled), response.Status, response.Body, response.BodyType, definition.RequestHeaders, response.ResponseHeaders, string(keysJSON), string(typesJSON), definition.RequestBodyRaw, definition.RequestBodyType, string(paramsJSON), string(paramTypesJSON), timestamp)
		if err != nil {
			return nil, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM api_mocks WHERE public_id = ?`, publicID).Scan(&apiID); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM api_mock_responses WHERE api_mock_id = ?`, apiID); err != nil {
		return nil, err
	}
	for index, example := range definition.ResponseExamples {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO api_mock_responses (api_mock_id, response_status, response_name, response_body, response_body_type, response_headers, sort_order, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, apiID, example.Status, example.Name, example.Body, example.BodyType, example.ResponseHeaders, index, timestamp); err != nil {
			return nil, err
		}
	}
	if err := deactivateDuplicateRoutes(ctx, tx, apiID, definition.Method, definition.RoutePath, mockEnabled, proxyEnabled, timestamp); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.FindByID(ctx, publicID)
}

func upsertDefinition(ctx context.Context, tx *database.Tx, definition Definition) error {
	timestamp := databaseTimestamp(time.Now())
	response := definition.ResponseExamples[0]

	var existingStatus sql.NullInt64
	var existingID int64
	err := tx.QueryRowContext(ctx, `SELECT id, response_status FROM api_mocks WHERE method = ? AND route_path = ?`, definition.Method, definition.RoutePath).Scan(&existingID, &existingStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if existingStatus.Valid {
		for _, example := range definition.ResponseExamples {
			if example.Status == int(existingStatus.Int64) {
				response = example
				break
			}
		}
	}

	folderJSON, _ := json.Marshal(nonNilStringSlice(definition.PostmanFolderPath))
	keysJSON, _ := json.Marshal(nonNilStringSlice(definition.RequestBodyKeys))
	typesJSON, _ := json.Marshal(nonNilRequestBodyTypes(definition.RequestBodyTypes))
	paramsJSON, _ := json.Marshal(nonNilStringSlice(definition.RequestParamKeys))
	paramTypesJSON, _ := json.Marshal(nonNilRequestBodyTypes(definition.RequestParamTypes))

	var apiID int64
	if existingID > 0 {
		apiID = existingID
		_, err = tx.ExecContext(ctx, `
			UPDATE api_mocks
				SET collection_name = ?, route_name = ?, postman_folder_path = ?, mock_enabled = 1,
				proxy_enabled = 1, response_status = ?, response_body = ?, response_body_type = ?, request_headers = ?, response_headers = ?, request_body_keys = ?,
				request_body_types = ?, request_body_raw = ?, request_body_type = ?, request_param_keys = ?, request_param_types = ?, updated_at = ?
			WHERE id = ?
		`, definition.CollectionName, definition.RouteName, string(folderJSON), response.Status, response.Body, response.BodyType, definition.RequestHeaders, response.ResponseHeaders, string(keysJSON), string(typesJSON), definition.RequestBodyRaw, definition.RequestBodyType, string(paramsJSON), string(paramTypesJSON), timestamp, apiID)
		if err != nil {
			return err
		}
	} else {
		publicID, err := uuidv7.New()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO api_mocks (
				public_id, collection_name, route_name, postman_folder_path, method, route_path,
				mock_enabled, proxy_enabled, response_status, response_body, response_body_type,
				request_headers, response_headers, request_body_keys, request_body_types, request_body_raw, request_body_type, request_param_keys, request_param_types, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, 1, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, publicID, definition.CollectionName, definition.RouteName, string(folderJSON), definition.Method, definition.RoutePath, response.Status, response.Body, response.BodyType, definition.RequestHeaders, response.ResponseHeaders, string(keysJSON), string(typesJSON), definition.RequestBodyRaw, definition.RequestBodyType, string(paramsJSON), string(paramTypesJSON), timestamp)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM api_mocks WHERE method = ? AND route_path = ?`, definition.Method, definition.RoutePath).Scan(&apiID); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM api_mock_responses WHERE api_mock_id = ?`, apiID); err != nil {
		return err
	}
	for index, example := range definition.ResponseExamples {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO api_mock_responses (api_mock_id, response_status, response_name, response_body, response_body_type, response_headers, sort_order, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, apiID, example.Status, example.Name, example.Body, example.BodyType, example.ResponseHeaders, index, timestamp); err != nil {
			return err
		}
	}
	return nil
}

func deactivateDuplicateRoutes(ctx context.Context, tx *database.Tx, apiID int, method, routePath string, mockEnabled, proxyEnabled bool, timestamp string) error {
	if !mockEnabled && !proxyEnabled {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE api_mocks SET mock_enabled = 0, proxy_enabled = 0, updated_at = ? WHERE method = ? AND route_path = ? AND id <> ?`, timestamp, method, routePath, apiID); err != nil {
		return err
	}
	if mockEnabled {
		_, err := tx.ExecContext(ctx, `UPDATE api_mocks SET proxy_enabled = 0 WHERE id = ?`, apiID)
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE api_mocks SET mock_enabled = 0 WHERE id = ?`, apiID)
	return err
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]StoredAPI, error) {
	if err := r.ensurePublicIDs(ctx); err != nil {
		return nil, err
	}
	paging, args := database.LimitOffset(r.db.Provider, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, COALESCE(response_body_type, 'json'),
			COALESCE(request_headers, '{}'), COALESCE(response_headers, '{}'), request_body_keys, request_body_types, COALESCE(request_body_raw, ''), COALESCE(request_body_type, ''),
			request_param_keys, COALESCE(request_param_types, '{}'), updated_at
		FROM api_mocks
		ORDER BY route_path ASC, method ASC
	`+paging, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAPIs(rows)
}

func (r *Repository) ListAll(ctx context.Context) ([]StoredAPI, error) {
	if err := r.ensurePublicIDs(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, COALESCE(response_body_type, 'json'),
			COALESCE(request_headers, '{}'), COALESCE(response_headers, '{}'), request_body_keys, request_body_types, COALESCE(request_body_raw, ''), COALESCE(request_body_type, ''),
			request_param_keys, COALESCE(request_param_types, '{}'), updated_at
		FROM api_mocks
		ORDER BY route_path ASC, method ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAPIs(rows)
}

func (r *Repository) Count(ctx context.Context) (int, error) {
	var total int
	return total, r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_mocks`).Scan(&total)
}

func (r *Repository) ListCollections(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT collection_name
		FROM api_mocks
		WHERE TRIM(collection_name) <> ''
		ORDER BY collection_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	collections := []string{}
	for rows.Next() {
		var collection string
		if err := rows.Scan(&collection); err != nil {
			return nil, err
		}
		collections = append(collections, collection)
	}
	return collections, rows.Err()
}

func (r *Repository) ListVersions(ctx context.Context) ([]ProjectVersion, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT public_id, message, api_count, created_at
		FROM api_mock_versions
		ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := []ProjectVersion{}
	for rows.Next() {
		var item ProjectVersion
		if err := rows.Scan(&item.ID, &item.Message, &item.APICount, &item.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, item)
	}
	return versions, rows.Err()
}

func (r *Repository) CreateVersion(ctx context.Context, message string, snapshot []APIRecord) (*ProjectVersion, error) {
	publicID, err := uuidv7.New()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(nonNilAPIRecords(snapshot))
	if err != nil {
		return nil, err
	}
	timestamp := databaseTimestamp(time.Now())
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO api_mock_versions (public_id, message, snapshot_json, api_count, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, publicID, message, string(payload), len(snapshot), timestamp); err != nil {
		return nil, err
	}
	return &ProjectVersion{ID: publicID, Message: message, APICount: len(snapshot), CreatedAt: timestamp}, nil
}

func (r *Repository) GetVersion(ctx context.Context, id string) (*ProjectVersionRecord, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT public_id, message, snapshot_json, api_count, created_at
		FROM api_mock_versions
		WHERE public_id = ?
	`, id)
	var publicID string
	var message string
	var snapshotJSON string
	var apiCount int
	var createdAt string
	if err := row.Scan(&publicID, &message, &snapshotJSON, &apiCount, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	snapshot := []APIRecord{}
	if err := json.Unmarshal([]byte(snapshotJSON), &snapshot); err != nil {
		return nil, err
	}
	return &ProjectVersionRecord{
		ProjectVersion: ProjectVersion{ID: publicID, Message: message, APICount: apiCount, CreatedAt: createdAt},
		Snapshot:       snapshot,
	}, nil
}

func (r *Repository) ReplaceAll(ctx context.Context, records []APIRecord) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM api_mocks`); err != nil {
		return err
	}

	timestamp := databaseTimestamp(time.Now())
	for _, record := range records {
		publicID := strings.TrimSpace(record.ID)
		if publicID == "" {
			generatedID, err := uuidv7.New()
			if err != nil {
				return err
			}
			publicID = generatedID
		}

		folderJSON, _ := json.Marshal(nonNilStringSlice(record.PostmanFolderPath))
		keysJSON, _ := json.Marshal(nonNilStringSlice(record.ExpectedRequestKeys))
		typesJSON, _ := json.Marshal(nonNilRequestBodyTypes(record.ExpectedRequestTypes))
		paramsJSON, _ := json.Marshal(nonNilStringSlice(record.ExpectedParamKeys))
		paramTypesJSON, _ := json.Marshal(nonNilRequestBodyTypes(record.ExpectedParamTypes))
		response := selectedRecordResponse(record)

		_, err = tx.ExecContext(ctx, `
			INSERT INTO api_mocks (
				public_id, collection_name, route_name, postman_folder_path, method, route_path,
				mock_enabled, proxy_enabled, response_status, response_body, response_body_type,
				request_headers, response_headers, request_body_keys, request_body_types, request_body_raw, request_body_type, request_param_keys, request_param_types, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, publicID, record.CollectionName, record.RouteName, string(folderJSON), record.Method, record.RoutePath, database.BoolValue(record.MockEnabled), database.BoolValue(record.ProxyEnabled), response.Status, response.Body, response.BodyType, record.RequestHeaders, response.ResponseHeaders, string(keysJSON), string(typesJSON), record.RequestBodyRaw, record.RequestBodyType, string(paramsJSON), string(paramTypesJSON), timestamp)
		if err != nil {
			return err
		}

		var apiID int
		if err := tx.QueryRowContext(ctx, `SELECT id FROM api_mocks WHERE public_id = ?`, publicID).Scan(&apiID); err != nil {
			return err
		}
		for index, example := range record.ResponseExamples {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO api_mock_responses (api_mock_id, response_status, response_name, response_body, response_body_type, response_headers, sort_order, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`, apiID, example.Status, example.Name, example.Body, example.BodyType, example.ResponseHeaders, index, timestamp); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func (r *Repository) FindByID(ctx context.Context, id string) (*StoredAPI, error) {
	if err := r.ensurePublicIDs(ctx); err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, COALESCE(response_body_type, 'json'),
			COALESCE(request_headers, '{}'), COALESCE(response_headers, '{}'), request_body_keys, request_body_types, COALESCE(request_body_raw, ''), COALESCE(request_body_type, ''),
			request_param_keys, COALESCE(request_param_types, '{}'), updated_at
		FROM api_mocks WHERE public_id = ?
	`, id)
	return scanAPI(row)
}

func (r *Repository) FindByMethodAndRoute(ctx context.Context, method, routePath string) (*StoredAPI, error) {
	if err := r.ensurePublicIDs(ctx); err != nil {
		return nil, err
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, COALESCE(response_body_type, 'json'),
			COALESCE(request_headers, '{}'), COALESCE(response_headers, '{}'), request_body_keys, request_body_types, COALESCE(request_body_raw, ''), COALESCE(request_body_type, ''),
			request_param_keys, COALESCE(request_param_types, '{}'), updated_at
		FROM api_mocks WHERE method = ? AND route_path = ?
	`, strings.ToUpper(method), routePath)
	return scanAPI(row)
}

func (r *Repository) FindByMethod(ctx context.Context, method string, onlyMockEnabled bool) ([]StoredAPI, error) {
	if err := r.ensurePublicIDs(ctx); err != nil {
		return nil, err
	}
	query := `
		SELECT id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, COALESCE(response_body_type, 'json'),
			COALESCE(request_headers, '{}'), COALESCE(response_headers, '{}'), request_body_keys, request_body_types, COALESCE(request_body_raw, ''), COALESCE(request_body_type, ''),
			request_param_keys, COALESCE(request_param_types, '{}'), updated_at
		FROM api_mocks WHERE method = ?
	`
	args := []any{strings.ToUpper(method)}
	if onlyMockEnabled {
		query += ` AND mock_enabled = 1`
	}
	query += ` ORDER BY route_path ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAPIs(rows)
}

func (r *Repository) SetMockEnabled(ctx context.Context, id string, enabled bool) (*StoredAPI, error) {
	return r.updateBoolean(ctx, id, "mock_enabled", enabled)
}

func (r *Repository) SetProxyEnabled(ctx context.Context, id string, enabled bool) (*StoredAPI, error) {
	return r.updateBoolean(ctx, id, "proxy_enabled", enabled)
}

func (r *Repository) updateBoolean(ctx context.Context, id string, column string, enabled bool) (*StoredAPI, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var apiID int
	var method string
	var routePath string
	err = tx.QueryRowContext(ctx, `SELECT id, method, route_path FROM api_mocks WHERE public_id = ?`, id).Scan(&apiID, &method, &routePath)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	value := 0
	if enabled {
		value = 1
	}
	timestamp := databaseTimestamp(time.Now())
	if enabled {
		if _, err := tx.ExecContext(ctx, `UPDATE api_mocks SET mock_enabled = 0, proxy_enabled = 0, updated_at = ? WHERE method = ? AND route_path = ? AND id <> ?`, timestamp, method, routePath, apiID); err != nil {
			return nil, err
		}
		if column == "mock_enabled" {
			if _, err := tx.ExecContext(ctx, `UPDATE api_mocks SET proxy_enabled = 0 WHERE id = ?`, apiID); err != nil {
				return nil, err
			}
		}
		if column == "proxy_enabled" {
			if _, err := tx.ExecContext(ctx, `UPDATE api_mocks SET mock_enabled = 0 WHERE id = ?`, apiID); err != nil {
				return nil, err
			}
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE api_mocks SET `+column+` = ?, updated_at = ? WHERE public_id = ?`, value, timestamp, id)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.FindByID(ctx, id)
}

func (r *Repository) SetSelectedResponse(ctx context.Context, id string, status int) (*StoredAPI, error) {
	internalID, err := r.internalID(ctx, id)
	if err != nil || internalID == 0 {
		return nil, err
	}
	var body string
	var bodyType string
	var responseHeaders string
	err = r.db.QueryRowContext(ctx, `SELECT response_body, COALESCE(response_body_type, 'json'), COALESCE(response_headers, '{}') FROM api_mock_responses WHERE api_mock_id = ? AND response_status = ?`, internalID, status).Scan(&body, &bodyType, &responseHeaders)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	result, err := r.db.ExecContext(ctx, `UPDATE api_mocks SET response_status = ?, response_body = ?, response_body_type = ?, response_headers = ?, updated_at = ? WHERE public_id = ?`, status, body, bodyType, responseHeaders, databaseTimestamp(time.Now()), id)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, nil
	}
	return r.FindByID(ctx, id)
}

func (r *Repository) Delete(ctx context.Context, id string) (bool, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM api_mocks WHERE public_id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) ResponseExamples(ctx context.Context, apiMockID int) ([]ResponseExample, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT response_status, response_name, response_body, COALESCE(response_body_type, 'json'), COALESCE(response_headers, '{}')
		FROM api_mock_responses
		WHERE api_mock_id = ?
		ORDER BY sort_order ASC, response_status ASC
	`, apiMockID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var examples []ResponseExample
	for rows.Next() {
		var item ResponseExample
		if err := rows.Scan(&item.Status, &item.Name, &item.Body, &item.BodyType, &item.ResponseHeaders); err != nil {
			return nil, err
		}
		if strings.TrimSpace(item.ResponseHeaders) == "" {
			item.ResponseHeaders = "{}"
		}
		examples = append(examples, item)
	}
	return examples, rows.Err()
}

func (r *Repository) internalID(ctx context.Context, publicID string) (int, error) {
	if err := r.ensurePublicIDs(ctx); err != nil {
		return 0, err
	}
	var id int
	err := r.db.QueryRowContext(ctx, `SELECT id FROM api_mocks WHERE public_id = ?`, publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (r *Repository) ensurePublicIDs(ctx context.Context) error {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM api_mocks WHERE public_id IS NULL OR public_id = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, id := range ids {
		publicID, err := uuidv7.New()
		if err != nil {
			return err
		}
		if _, err := r.db.ExecContext(ctx, `UPDATE api_mocks SET public_id = ? WHERE id = ?`, publicID, id); err != nil {
			return err
		}
	}
	return nil
}

func scanAPIs(rows *sql.Rows) ([]StoredAPI, error) {
	var apis []StoredAPI
	for rows.Next() {
		api, err := scanAPI(rows)
		if err != nil {
			return nil, err
		}
		apis = append(apis, *api)
	}
	return apis, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAPI(row scanner) (*StoredAPI, error) {
	var api StoredAPI
	var mockEnabled, proxyEnabled int
	err := row.Scan(
		&api.InternalID,
		&api.ID,
		&api.CollectionName,
		&api.RouteName,
		&api.PostmanFolderPath,
		&api.Method,
		&api.RoutePath,
		&mockEnabled,
		&proxyEnabled,
		&api.ResponseStatus,
		&api.ResponseBody,
		&api.ResponseBodyType,
		&api.RequestHeaders,
		&api.ResponseHeaders,
		&api.RequestBodyKeys,
		&api.RequestBodyTypes,
		&api.RequestBodyRaw,
		&api.RequestBodyType,
		&api.RequestParamKeys,
		&api.RequestParamTypes,
		&api.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	api.MockEnabled = mockEnabled == 1
	api.ProxyEnabled = proxyEnabled == 1
	if strings.TrimSpace(api.RequestHeaders) == "" {
		api.RequestHeaders = "{}"
	}
	if strings.TrimSpace(api.ResponseHeaders) == "" {
		api.ResponseHeaders = "{}"
	}
	return &api, nil
}

func databaseTimestamp(now time.Time) string {
	return now.UTC().Format("2006-01-02 15:04:05")
}

func nonNilStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilRequestBodyTypes(values RequestBodyTypes) RequestBodyTypes {
	if values == nil {
		return RequestBodyTypes{}
	}
	return values
}

func nonNilAPIRecords(values []APIRecord) []APIRecord {
	if values == nil {
		return []APIRecord{}
	}
	return values
}

func selectedRecordResponse(record APIRecord) ResponseExample {
	if len(record.ResponseExamples) == 0 {
		return ResponseExample{Status: record.ResponseStatus, Name: "Imported response", Body: record.ResponseBody, BodyType: record.ResponseBodyType, ResponseHeaders: record.ResponseHeaders}
	}
	for _, example := range record.ResponseExamples {
		if example.Status == record.ResponseStatus {
			return example
		}
	}
	return record.ResponseExamples[0]
}
