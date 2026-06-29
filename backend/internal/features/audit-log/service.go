package auditlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/uuidv7"
)

type Service struct {
	db      *database.Connection
	specDB  *database.Connection
	config  Config
	writeMu sync.Mutex
}

func NewService(db *database.Connection, specDB ...*database.Connection) *Service {
	return NewServiceWithConfig(db, DefaultConfig(), specDB...)
}

func NewServiceWithConfig(db *database.Connection, cfg Config, specDB ...*database.Connection) *Service {
	sourceDB := db
	if len(specDB) > 0 && specDB[0] != nil {
		sourceDB = specDB[0]
	}
	return &Service{db: db, specDB: sourceDB, config: normalizeConfig(cfg)}
}

func (s *Service) Config() Config {
	if s == nil {
		return DefaultConfig()
	}
	return normalizeConfig(s.config)
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Record, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	targets, _ := json.Marshal(input.ShadowTargets)
	success := 0
	if input.Success {
		success = 1
	}
	publicID, err := uuidv7.New()
	if err != nil {
		return Record{}, err
	}
	insertQuery := `
		INSERT INTO proxy_audit_logs (
			public_id, api_mock_id, api_mock_public_id, route_name, method, route_path, target_url, response_status,
			duration_ms, success, error_message, request_headers, request_body,
			response_headers, response_body, shadow_targets, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	args := []any{publicID, input.APIMockDBID, nullableString(input.APIMockID), input.RouteName, input.Method, input.RoutePath, input.TargetURL, input.ResponseStatus, input.DurationMS, success, input.ErrorMessage, input.RequestHeaders, input.RequestBody, input.ResponseHeaders, input.ResponseBody, string(targets), timestamp()}
	id, err := s.insertAuditLog(ctx, insertQuery, args...)
	if err != nil {
		return Record{}, err
	}
	return s.GetByInternalID(ctx, int(id))
}

func (s *Service) CreateShadow(ctx context.Context, auditLogID int, entry ShadowEntry) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	success := 0
	if entry.Success {
		success = 1
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE proxy_audit_log_shadows
		SET name = ?, base_url = ?, target_url = ?, response_status = ?, duration_ms = ?,
			success = ?, error_message = ?, request_headers = ?, request_body = ?,
			response_headers = ?, response_body = ?, created_at = ?
		WHERE proxy_audit_log_id = ? AND shadow_endpoint_id = ?
	`, entry.Name, entry.BaseURL, entry.TargetURL, entry.ResponseStatus, entry.DurationMS, success, entry.ErrorMessage, entry.RequestHeaders, entry.RequestBody, entry.ResponseHeaders, entry.ResponseBody, timestamp(), auditLogID, entry.ID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO proxy_audit_log_shadows (
			proxy_audit_log_id, shadow_endpoint_id, name, base_url, target_url,
			response_status, duration_ms, success, error_message, request_headers,
			request_body, response_headers, response_body, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, auditLogID, entry.ID, entry.Name, entry.BaseURL, entry.TargetURL, entry.ResponseStatus, entry.DurationMS, success, entry.ErrorMessage, entry.RequestHeaders, entry.RequestBody, entry.ResponseHeaders, entry.ResponseBody, timestamp())
	return err
}

func (s *Service) Get(ctx context.Context, id string) (Record, error) {
	if err := s.ensurePublicIDs(ctx); err != nil {
		return Record{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, public_id, api_mock_id, api_mock_public_id, route_name, method, route_path, target_url,
			response_status, duration_ms, success, error_message, request_headers,
			request_body, response_headers, response_body, shadow_targets, created_at
		FROM proxy_audit_logs WHERE public_id = ?
	`, id)
	record, err := scanRecord(row)
	if err == nil {
		if err := s.hydrateRecord(ctx, &record); err != nil {
			return Record{}, err
		}
		return record, nil
	}
	if !errors.Is(err, sql.ErrNoRows) || !isPositiveInteger(id) {
		return Record{}, err
	}
	internalID, _ := strconv.Atoi(id)
	return s.GetByInternalID(ctx, internalID)
}

func (s *Service) GetByInternalID(ctx context.Context, id int) (Record, error) {
	if err := s.ensurePublicIDs(ctx); err != nil {
		return Record{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, public_id, api_mock_id, api_mock_public_id, route_name, method, route_path, target_url,
			response_status, duration_ms, success, error_message, request_headers,
			request_body, response_headers, response_body, shadow_targets, created_at
		FROM proxy_audit_logs WHERE id = ?
	`, id)
	record, err := scanRecord(row)
	if err != nil {
		return Record{}, err
	}
	if err := s.hydrateRecord(ctx, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Service) List(ctx context.Context, page, pageSize int) (ListResult, error) {
	if err := s.ensurePublicIDs(ctx); err != nil {
		return ListResult{}, err
	}
	total, err := s.Count(ctx)
	if err != nil {
		return ListResult{}, err
	}
	totalPages := int(math.Max(1, math.Ceil(float64(total)/float64(pageSize))))
	if page < 1 {
		page = 1
	}
	if page > totalPages {
		page = totalPages
	}
	paging, args := database.LimitOffset(s.db.Provider, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, public_id, api_mock_id, api_mock_public_id, route_name, method, route_path, target_url,
			response_status, duration_ms, success, error_message, request_headers,
			request_body, response_headers, response_body, shadow_targets, created_at
		FROM proxy_audit_logs
		ORDER BY id DESC
	`+paging, args...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()

	items := []Record{}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return ListResult{}, err
		}
		record.ShadowEntries, err = s.ListShadows(ctx, record.InternalID)
		if err != nil {
			return ListResult{}, err
		}
		items = append(items, record)
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, rows.Err()
}

func (s *Service) Count(ctx context.Context) (int, error) {
	var total int
	return total, s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_audit_logs`).Scan(&total)
}

func (s *Service) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM proxy_audit_logs WHERE created_at < ?`, formatTimestamp(cutoff))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Service) insertAuditLog(ctx context.Context, query string, args ...any) (int64, error) {
	switch {
	case database.IsPostgresFamily(s.db.Provider):
		var id int64
		err := s.db.QueryRowContext(ctx, query+` RETURNING id`, args...).Scan(&id)
		return id, err
	case s.db.Provider == database.ProviderSQLServer:
		sqlServerQuery := strings.Replace(query, "\n\t\tVALUES", "\n\t\tOUTPUT INSERTED.id\n\t\tVALUES", 1)
		var id int64
		err := s.db.QueryRowContext(ctx, sqlServerQuery, args...).Scan(&id)
		return id, err
	default:
		result, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			return 0, err
		}
		return result.LastInsertId()
	}
}

func (s *Service) ListShadows(ctx context.Context, auditLogID int) ([]ShadowEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, proxy_audit_log_id, shadow_endpoint_id, name, base_url, target_url,
			response_status, duration_ms, success, error_message, request_headers,
			request_body, response_headers, response_body
		FROM proxy_audit_log_shadows WHERE proxy_audit_log_id = ?
		ORDER BY id ASC
	`, auditLogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := []ShadowEntry{}
	for rows.Next() {
		var entry ShadowEntry
		var responseStatus sql.NullInt64
		var errorMessage sql.NullString
		var success int
		err := rows.Scan(&entry.ShadowAuditLogID, &entry.AuditLogID, &entry.ID, &entry.Name, &entry.BaseURL, &entry.TargetURL, &responseStatus, &entry.DurationMS, &success, &errorMessage, &entry.RequestHeaders, &entry.RequestBody, &entry.ResponseHeaders, &entry.ResponseBody)
		if err != nil {
			return nil, err
		}
		if responseStatus.Valid {
			value := int(responseStatus.Int64)
			entry.ResponseStatus = &value
		}
		if errorMessage.Valid {
			entry.ErrorMessage = &errorMessage.String
		}
		entry.Success = success == 1
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Service) ensurePublicIDs(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM proxy_audit_logs WHERE public_id IS NULL OR public_id = ''`)
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
		if _, err := s.db.ExecContext(ctx, `UPDATE proxy_audit_logs SET public_id = ? WHERE id = ?`, publicID, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) hydrateRecord(ctx context.Context, record *Record) error {
	shadowEntries, err := s.ListShadows(ctx, record.InternalID)
	if err != nil {
		return err
	}
	record.ShadowEntries = shadowEntries

	return s.hydratePostmanExamples(ctx, record)
}

func (s *Service) hydratePostmanExamples(ctx context.Context, record *Record) error {
	record.PostmanExamples = []PostmanExample{}
	record.PostmanExample = nil

	publicID := stringValue(record.APIMockID)
	if publicID == "" && record.APIMockDBID != nil {
		currentPublicID, err := s.apiMockPublicID(ctx, *record.APIMockDBID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			publicID = currentPublicID
			if publicID != "" {
				record.APIMockID = &publicID
			}
		}
	}

	examples, selectedStatus, matchedPublicID, err := s.currentSpecPostmanExamples(ctx, publicID, record.Method, record.RoutePath)
	if err != nil {
		return err
	}
	record.PostmanExamples = examples
	if len(examples) == 0 {
		return nil
	}
	if matchedPublicID != "" {
		record.APIMockID = &matchedPublicID
	}
	record.PostmanExample = preferredPostmanExample(examples, selectedStatus, record.ResponseStatus)
	return nil
}

func (s *Service) apiMockPublicID(ctx context.Context, apiMockID int) (string, error) {
	var publicID string
	err := s.specDB.QueryRowContext(ctx, `SELECT COALESCE(public_id, '') FROM api_mocks WHERE id = ?`, apiMockID).Scan(&publicID)
	return publicID, err
}

func (s *Service) currentSpecPostmanExamples(ctx context.Context, publicID, method, routePath string) ([]PostmanExample, *int, string, error) {
	record, err := s.currentSpecRecord(ctx, publicID, method, routePath)
	if err != nil || record == nil {
		return []PostmanExample{}, nil, "", err
	}

	rows, err := s.specDB.QueryContext(ctx, `
		SELECT response_status, response_name, response_body
		FROM api_mock_responses
		WHERE api_mock_id = ?
		ORDER BY sort_order ASC, response_status ASC
	`, record.InternalID)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()

	examples := []PostmanExample{}
	for rows.Next() {
		var example PostmanExample
		example.APIMockID = record.ID
		if err := rows.Scan(&example.Status, &example.Name, &example.ResponseBody); err != nil {
			return nil, nil, "", err
		}
		examples = append(examples, example)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, "", err
	}
	if len(examples) == 0 {
		examples = append(examples, PostmanExample{
			APIMockID:    record.ID,
			Status:       record.ResponseStatus,
			Name:         "Imported response",
			ResponseBody: record.ResponseBody,
		})
	}
	selectedStatus := record.ResponseStatus
	return examples, &selectedStatus, record.ID, nil
}

func (s *Service) currentSpecRecord(ctx context.Context, publicID, method, routePath string) (*currentAPIRecord, error) {
	if publicID != "" {
		record, err := s.queryCurrentSpecByPublicID(ctx, publicID)
		if err != nil || record != nil {
			return record, err
		}
	}

	rows, err := s.specDB.QueryContext(ctx, `
		SELECT id, public_id, method, route_path, response_status, response_body
		FROM api_mocks
		WHERE method = ?
		ORDER BY route_path ASC, id ASC
	`, strings.ToUpper(method))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		record, err := scanCurrentAPIRecord(rows)
		if err != nil {
			return nil, err
		}
		if routePathsMatch(record.RoutePath, routePath) {
			return &record, nil
		}
	}
	return nil, rows.Err()
}

func (s *Service) queryCurrentSpecByPublicID(ctx context.Context, publicID string) (*currentAPIRecord, error) {
	row := s.specDB.QueryRowContext(ctx, `
		SELECT id, public_id, method, route_path, response_status, response_body
		FROM api_mocks
		WHERE public_id = ?
	`, publicID)
	record, err := scanCurrentAPIRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func routePathsMatch(candidateRoutePath, requestedRoutePath string) bool {
	candidate := comparableRoutePath(candidateRoutePath)
	requested := comparableRoutePath(requestedRoutePath)
	if candidate == requested {
		return true
	}
	candidateSegments := splitRouteSegments(candidate)
	requestedSegments := splitRouteSegments(requested)
	if len(candidateSegments) != len(requestedSegments) {
		return false
	}
	for index, segment := range candidateSegments {
		if strings.HasPrefix(segment, ":") {
			if requestedSegments[index] == "" {
				return false
			}
			continue
		}
		if segment != requestedSegments[index] {
			return false
		}
	}
	return true
}

func comparableRoutePath(routePath string) string {
	pathOnly := strings.Split(routePath, "?")[0]
	trimmed := strings.TrimSpace(pathOnly)
	if trimmed == "" {
		return "/"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

func splitRouteSegments(routePath string) []string {
	if routePath == "/" {
		return nil
	}
	return strings.Split(strings.TrimPrefix(routePath, "/"), "/")
}

type currentAPIRecord struct {
	InternalID     int
	ID             string
	Method         string
	RoutePath      string
	ResponseStatus int
	ResponseBody   string
}

func scanCurrentAPIRecord(row scanner) (currentAPIRecord, error) {
	var record currentAPIRecord
	err := row.Scan(&record.InternalID, &record.ID, &record.Method, &record.RoutePath, &record.ResponseStatus, &record.ResponseBody)
	return record, err
}

func findSnapshotAPI(snapshot []snapshotAPIRecord, publicID, method, routePath string) *snapshotAPIRecord {
	normalizedMethod := strings.ToUpper(method)
	if publicID != "" {
		for index := range snapshot {
			if snapshot[index].ID == publicID {
				return &snapshot[index]
			}
		}
		return nil
	}
	for index := range snapshot {
		record := &snapshot[index]
		if strings.ToUpper(record.Method) != normalizedMethod {
			continue
		}
		if record.RoutePath == routePath || record.ResolvedRoutePath == routePath {
			return record
		}
	}
	return nil
}

type snapshotAPIRecord struct {
	ID                string                    `json:"id"`
	Method            string                    `json:"method"`
	RoutePath         string                    `json:"routePath"`
	ResolvedRoutePath string                    `json:"resolvedRoutePath"`
	ResponseStatus    int                       `json:"responseStatus"`
	ResponseBody      string                    `json:"responseBody"`
	ResponseExamples  []snapshotResponseExample `json:"responseExamples"`
}

type snapshotResponseExample struct {
	Status int    `json:"status"`
	Name   string `json:"name"`
	Body   string `json:"body"`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRecord(row scanner) (Record, error) {
	var record Record
	var publicID sql.NullString
	var apiMockID sql.NullInt64
	var apiMockPublicID sql.NullString
	var responseStatus sql.NullInt64
	var errorMessage sql.NullString
	var success int
	var shadowTargets string
	err := row.Scan(&record.InternalID, &publicID, &apiMockID, &apiMockPublicID, &record.RouteName, &record.Method, &record.RoutePath, &record.TargetURL, &responseStatus, &record.DurationMS, &success, &errorMessage, &record.RequestHeaders, &record.RequestBody, &record.ResponseHeaders, &record.ResponseBody, &shadowTargets, &record.CreatedAt)
	if err != nil {
		return Record{}, err
	}
	if publicID.Valid && strings.TrimSpace(publicID.String) != "" {
		record.ID = publicID.String
	} else {
		record.ID = strconv.Itoa(record.InternalID)
	}
	if apiMockID.Valid {
		value := int(apiMockID.Int64)
		record.APIMockDBID = &value
	}
	if apiMockPublicID.Valid && strings.TrimSpace(apiMockPublicID.String) != "" {
		value := apiMockPublicID.String
		record.APIMockID = &value
	}
	if responseStatus.Valid {
		value := int(responseStatus.Int64)
		record.ResponseStatus = &value
	}
	if errorMessage.Valid {
		record.ErrorMessage = &errorMessage.String
	}
	record.Success = success == 1
	_ = json.Unmarshal([]byte(shadowTargets), &record.ShadowTargets)
	if record.ShadowTargets == nil {
		record.ShadowTargets = []ShadowTarget{}
	}
	if record.ShadowEntries == nil {
		record.ShadowEntries = []ShadowEntry{}
	}
	if record.PostmanExamples == nil {
		record.PostmanExamples = []PostmanExample{}
	}
	return record, nil
}

func isPositiveInteger(value string) bool {
	parsed, err := strconv.Atoi(value)
	return err == nil && parsed > 0
}

func preferredPostmanExample(examples []PostmanExample, selectedStatus, responseStatus *int) *PostmanExample {
	if len(examples) == 0 {
		return nil
	}
	if selectedStatus != nil {
		for _, example := range examples {
			if example.Status == *selectedStatus {
				selected := example
				return &selected
			}
		}
	}
	if responseStatus != nil {
		for _, example := range examples {
			if example.Status == *responseStatus {
				selected := example
				return &selected
			}
		}
	}
	selected := examples[0]
	return &selected
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timestamp() string {
	return formatTimestamp(time.Now())
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05")
}
