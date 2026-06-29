package mockapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"postman-transform/backend-golang/internal/features/environment-variables"
	"postman-transform/backend-golang/internal/postman"
)

var ErrDuplicateRoute = errors.New("API spec with this method and path already exists")
var ErrInvalidDirectoryPath = errors.New("directory path must be absolute path segments without traversal")

type Service struct {
	repo    *Repository
	env     *environment.Service
	config  Config
	writeMu sync.Mutex
}

func NewService(repo *Repository, env *environment.Service, configs ...Config) *Service {
	cfg := DefaultConfig()
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return &Service{repo: repo, env: env, config: normalizeConfig(cfg)}
}

func (s *Service) Config() Config {
	if s == nil {
		return DefaultConfig()
	}
	return normalizeConfig(s.config)
}

func (s *Service) ImportCollection(ctx context.Context, payload []byte) (string, int, error) {
	parsedDefinitions, err := postman.ParseCollection(payload)
	if err != nil {
		return "", 0, err
	}
	definitions := make([]Definition, 0, len(parsedDefinitions))
	for _, parsed := range parsedDefinitions {
		examples := make([]ResponseExample, 0, len(parsed.ResponseExamples))
		for _, example := range parsed.ResponseExamples {
			examples = append(examples, ResponseExample{Status: example.Status, Name: example.Name, Body: example.Body, BodyType: "json", ResponseHeaders: "{}"})
		}
		definitions = append(definitions, Definition{
			CollectionName:    parsed.CollectionName,
			RouteName:         parsed.RouteName,
			PostmanFolderPath: parsed.PostmanFolderPath,
			Method:            parsed.Method,
			RoutePath:         parsed.RoutePath,
			ResponseStatus:    parsed.ResponseStatus,
			ResponseBody:      parsed.ResponseBody,
			ResponseBodyType:  "json",
			RequestHeaders:    "{}",
			ResponseHeaders:   "{}",
			ResponseExamples:  examples,
			RequestBodyRaw:    parsed.RequestBodyRaw,
			RequestBodyType:   parsed.RequestBodyType,
			RequestBodyKeys:   parsed.RequestBodyKeys,
			RequestBodyTypes:  RequestBodyTypes(parsed.RequestBodyTypes),
			RequestParamKeys:  parsed.RequestParamKeys,
		})
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.repo.ImportDefinitions(ctx, definitions); err != nil {
		return "", 0, err
	}
	return definitions[0].CollectionName, len(definitions), nil
}

func (s *Service) ImportSwagger(ctx context.Context, payload []byte) (string, int, error) {
	definitions, specName, err := parseOpenAPISpec(payload)
	if err != nil {
		return "", 0, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	if err := s.repo.ImportDefinitions(ctx, definitions); err != nil {
		return "", 0, err
	}
	return specName, len(definitions), nil
}

func (s *Service) ExportSpec(ctx context.Context, format string) ([]byte, string, string, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, "", "", err
	}
	records, err := s.toRecords(ctx, rows)
	if err != nil {
		return nil, "", "", err
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "postman", "collection":
		payload, err := exportPostmanCollection(records)
		return payload, specExportFileName("collection"), "application/json", err
	case "swagger", "openapi":
		payload, err := exportOpenAPISpec(records)
		return payload, specExportFileName("swagger"), "application/json", err
	default:
		return nil, "", "", errors.New("format must be collection or swagger")
	}
}

func specExportFileName(format string) string {
	return format + "-spec-" + time.Now().UTC().Format("20060102-150405") + ".json"
}

func revertVersionMessage(version ProjectVersion) string {
	hash := strings.ReplaceAll(version.ID, "-", "")
	if len(hash) > 12 {
		hash = hash[:12]
	}
	message := strings.TrimSpace(version.Message)
	if message == "" {
		return "Revert " + hash
	}
	return "Revert " + hash + " " + message
}

func (s *Service) List(ctx context.Context, page, pageSize int) (ListResult, error) {
	total, err := s.repo.Count(ctx)
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
	rows, err := s.repo.List(ctx, pageSize, (page-1)*pageSize)
	if err != nil {
		return ListResult{}, err
	}
	items, err := s.toRecords(ctx, rows)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

func (s *Service) Directory(ctx context.Context, mode string, path []string) (DirectoryResult, error) {
	if mode != "postman" {
		mode = "path"
	}
	normalizedPath, err := normalizeDirectoryPath(path)
	if err != nil {
		return DirectoryResult{}, err
	}
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return DirectoryResult{}, err
	}
	apis, err := s.toRecords(ctx, rows)
	if err != nil {
		return DirectoryResult{}, err
	}

	childCounts := map[string]int{}
	currentAPIs := []APIRecord{}
	for _, api := range apis {
		segments := directorySegments(api, mode)
		if !isDescendant(segments, normalizedPath) {
			continue
		}
		if len(segments) == len(normalizedPath) {
			currentAPIs = append(currentAPIs, api)
			continue
		}
		childCounts[segments[len(normalizedPath)]]++
	}

	children := make([]DirectoryChild, 0, len(childCounts))
	for name, count := range childCounts {
		children = append(children, DirectoryChild{Name: name, Path: append(append([]string{}, normalizedPath...), name), APICount: count})
	}
	sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
	sort.Slice(currentAPIs, func(i, j int) bool {
		return currentAPIs[i].ResolvedRoutePath+" "+currentAPIs[i].Method < currentAPIs[j].ResolvedRoutePath+" "+currentAPIs[j].Method
	})
	return DirectoryResult{Mode: mode, Path: normalizedPath, Children: children, APIs: currentAPIs, TotalAPICount: len(apis)}, nil
}

func (s *Service) Collections(ctx context.Context) (CollectionsResult, error) {
	collections, err := s.repo.ListCollections(ctx)
	if err != nil {
		return CollectionsResult{}, err
	}
	return CollectionsResult{Collections: collections}, nil
}

func (s *Service) ListVersions(ctx context.Context) ([]ProjectVersion, error) {
	return s.repo.ListVersions(ctx)
}

func (s *Service) CreateVersion(ctx context.Context, message string) (*ProjectVersion, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	records, err := s.toRecords(ctx, rows)
	if err != nil {
		return nil, err
	}
	cleanMessage := strings.TrimSpace(message)
	if cleanMessage == "" {
		cleanMessage = "Project version"
	}
	return s.repo.CreateVersion(ctx, cleanMessage, records)
}

func (s *Service) GetVersion(ctx context.Context, id string) (*ProjectVersionRecord, error) {
	return s.repo.GetVersion(ctx, id)
}

func (s *Service) RestoreVersion(ctx context.Context, id string) (*ProjectVersionRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	version, err := s.repo.GetVersion(ctx, id)
	if err != nil || version == nil {
		return version, err
	}
	if err := s.repo.ReplaceAll(ctx, version.Snapshot); err != nil {
		return nil, err
	}
	return version, nil
}

func (s *Service) RevertVersion(ctx context.Context, id string) (*ProjectVersion, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	target, err := s.repo.GetVersion(ctx, id)
	if err != nil || target == nil {
		return nil, err
	}

	if err := s.repo.ReplaceAll(ctx, target.Snapshot); err != nil {
		return nil, err
	}
	return s.repo.CreateVersion(ctx, revertVersionMessage(target.ProjectVersion), target.Snapshot)
}

func (s *Service) Get(ctx context.Context, id string) (*APIRecord, error) {
	row, err := s.repo.FindByID(ctx, id)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) CreateManual(ctx context.Context, input ManualSpecInput) (*APIRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	definition, mockEnabled, proxyEnabled, err := normalizeManualSpec(input)
	if err != nil {
		return nil, err
	}
	id, err := s.manualCreateTarget(ctx, &definition)
	if err != nil {
		return nil, err
	}
	row, err := s.repo.SaveManual(ctx, id, definition, mockEnabled, proxyEnabled)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) UpsertManual(ctx context.Context, input ManualSpecInput) (*APIRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	definition, mockEnabled, proxyEnabled, err := normalizeManualSpec(input)
	if err != nil {
		return nil, err
	}
	var id *string
	existing, err := s.repo.FindByMethodAndRoute(ctx, definition.Method, definition.RoutePath)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		id = &existing.ID
	}
	row, err := s.repo.SaveManual(ctx, id, definition, mockEnabled, proxyEnabled)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) UpdateManual(ctx context.Context, id string, input ManualSpecInput) (*APIRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	definition, mockEnabled, proxyEnabled, err := normalizeManualSpec(input)
	if err != nil {
		return nil, err
	}
	row, err := s.repo.SaveManual(ctx, &id, definition, mockEnabled, proxyEnabled)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) SetMockEnabled(ctx context.Context, id string, enabled bool) (*APIRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	row, err := s.repo.SetMockEnabled(ctx, id, enabled)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) SetProxyEnabled(ctx context.Context, id string, enabled bool) (*APIRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	row, err := s.repo.SetProxyEnabled(ctx, id, enabled)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) SelectResponse(ctx context.Context, id string, status int) (*APIRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	row, err := s.repo.SetSelectedResponse(ctx, id, status)
	if err != nil || row == nil {
		return nil, err
	}
	record, err := s.toRecord(ctx, *row)
	return &record, err
}

func (s *Service) Delete(ctx context.Context, id string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	return s.repo.Delete(ctx, id)
}

func (s *Service) manualCreateTarget(ctx context.Context, definition *Definition) (*string, error) {
	existing, err := s.repo.FindByMethodAndRoute(ctx, definition.Method, definition.RoutePath)
	if err != nil || existing == nil {
		return nil, err
	}
	existingExamples, err := s.repo.ResponseExamples(ctx, existing.InternalID)
	if err != nil {
		return nil, err
	}
	definition.ResponseExamples = mergeResponseExamples(existingExamples, definition.ResponseExamples)
	return &existing.ID, nil
}

func mergeResponseExamples(existing, incoming []ResponseExample) []ResponseExample {
	merged := make([]ResponseExample, 0, len(existing)+len(incoming))
	statusIndex := map[int]int{}
	for _, example := range existing {
		if !validHTTPStatus(example.Status) {
			continue
		}
		statusIndex[example.Status] = len(merged)
		merged = append(merged, example)
	}
	for _, example := range incoming {
		if index, ok := statusIndex[example.Status]; ok {
			merged[index] = example
			continue
		}
		statusIndex[example.Status] = len(merged)
		merged = append(merged, example)
	}
	if len(merged) == 0 {
		return incoming
	}
	return merged
}

func (s *Service) FindActiveMock(ctx context.Context, method, routePath string) (*APIRecord, error) {
	return s.findRoute(ctx, method, routePath, true)
}

func (s *Service) FindRoute(ctx context.Context, method, routePath string) (*APIRecord, error) {
	return s.findRoute(ctx, method, routePath, false)
}

func (s *Service) findRoute(ctx context.Context, method, routePath string, onlyMockEnabled bool) (*APIRecord, error) {
	rows, err := s.repo.FindByMethod(ctx, strings.ToUpper(method), onlyMockEnabled)
	if err != nil {
		return nil, err
	}
	envVars, err := s.env.List(ctx)
	if err != nil {
		return nil, err
	}
	requestedPath := ResolveTemplateVariables(routePath, envVars)
	var fallback *APIRecord
	for _, row := range rows {
		resolved := ResolveTemplateVariables(row.RoutePath, envVars)
		if RoutePathsMatch(resolved, requestedPath) {
			record, err := s.toRecordWithEnv(ctx, row, envVars)
			if err != nil {
				return nil, err
			}
			if onlyMockEnabled || record.ProxyEnabled {
				return &record, nil
			}
			if fallback == nil {
				fallback = &record
			}
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, nil
}

func (s *Service) toRecords(ctx context.Context, rows []StoredAPI) ([]APIRecord, error) {
	envVars, err := s.env.List(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]APIRecord, 0, len(rows))
	for _, row := range rows {
		record, err := s.toRecordWithEnv(ctx, row, envVars)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *Service) toRecord(ctx context.Context, row StoredAPI) (APIRecord, error) {
	envVars, err := s.env.List(ctx)
	if err != nil {
		return APIRecord{}, err
	}
	return s.toRecordWithEnv(ctx, row, envVars)
}

func (s *Service) toRecordWithEnv(ctx context.Context, row StoredAPI, envVars map[string]string) (APIRecord, error) {
	examples, err := s.repo.ResponseExamples(ctx, row.InternalID)
	if err != nil {
		return APIRecord{}, err
	}
	if len(examples) == 0 {
		examples = []ResponseExample{{Status: row.ResponseStatus, Name: "Imported response", Body: row.ResponseBody, BodyType: row.ResponseBodyType, ResponseHeaders: row.ResponseHeaders}}
	}

	folders := parseStringSlice(row.PostmanFolderPath)
	keys := parseStringSlice(row.RequestBodyKeys)
	params := parseStringSlice(row.RequestParamKeys)
	paramTypes := parseRequestBodyTypes(row.RequestParamTypes)
	types := parseRequestBodyTypes(row.RequestBodyTypes)

	return APIRecord{
		InternalID:           row.InternalID,
		ID:                   row.ID,
		CollectionName:       row.CollectionName,
		RouteName:            row.RouteName,
		PostmanFolderPath:    folders,
		Method:               row.Method,
		RoutePath:            row.RoutePath,
		ResolvedRoutePath:    ResolveTemplateVariables(row.RoutePath, envVars),
		MockEnabled:          row.MockEnabled,
		ProxyEnabled:         row.ProxyEnabled,
		ResponseStatus:       row.ResponseStatus,
		ResponseBody:         row.ResponseBody,
		ResponseBodyType:     row.ResponseBodyType,
		RequestHeaders:       row.RequestHeaders,
		ResponseHeaders:      row.ResponseHeaders,
		ResponseExamples:     examples,
		RequestBodyRaw:       row.RequestBodyRaw,
		RequestBodyType:      row.RequestBodyType,
		ExpectedRequestKeys:  keys,
		ExpectedRequestTypes: types,
		ExpectedParamKeys:    params,
		ExpectedParamTypes:   paramTypes,
		UpdatedAt:            row.UpdatedAt,
	}, nil
}

func normalizeManualSpec(input ManualSpecInput) (Definition, bool, bool, error) {
	method := strings.ToUpper(strings.TrimSpace(input.Method))
	if !validHTTPMethod(method) {
		return Definition{}, false, false, errors.New("method must be a valid HTTP method")
	}
	routePath := normalizeManualRoutePath(input.RoutePath)
	if routePath == "" {
		return Definition{}, false, false, errors.New("path is required")
	}

	examples, selectedStatus, selectedBody, err := normalizeResponseExamples(input.ResponseExamples, input.ResponseStatus)
	if err != nil {
		return Definition{}, false, false, err
	}
	requestHeaders, err := normalizeHeadersJSON(input.RequestHeaders)
	if err != nil {
		return Definition{}, false, false, err
	}
	bodyKeys := uniqueTrimmed(input.RequestBodyKeys)
	inferredBodyTypes := RequestBodyTypes{}
	if len(bodyKeys) == 0 && strings.EqualFold(strings.TrimSpace(input.RequestBodyType), "json") {
		var rawBody any
		if json.Unmarshal([]byte(input.RequestBodyRaw), &rawBody) == nil {
			source := requestObjectSource(rawBody)
			for key, value := range source {
				bodyKeys = append(bodyKeys, key)
				inferredBodyTypes[key] = inferAnyBodyType(value)
			}
			sort.Strings(bodyKeys)
		}
	}
	paramKeys := uniqueTrimmed(input.RequestParamKeys)
	bodyTypes := RequestBodyTypes{}
	for _, key := range bodyKeys {
		if inferredBodyTypes[key] != "" {
			bodyTypes[key] = inferredBodyTypes[key]
			continue
		}
		bodyTypes[key] = normalizeBodyType(input.RequestBodyTypes[key])
	}
	paramTypes := RequestBodyTypes{}
	for _, key := range paramKeys {
		paramTypes[key] = normalizeBodyType(input.RequestParamTypes[key])
	}

	collectionName := strings.TrimSpace(input.CollectionName)
	if collectionName == "" {
		collectionName = "Manual Specs"
	}
	routeName := strings.TrimSpace(input.RouteName)
	if routeName == "" {
		routeName = method + " " + routePath
	}

	return Definition{
		CollectionName:    collectionName,
		RouteName:         routeName,
		PostmanFolderPath: uniqueTrimmed(input.PostmanFolderPath),
		Method:            method,
		RoutePath:         routePath,
		ResponseStatus:    selectedStatus,
		ResponseBody:      selectedBody,
		ResponseBodyType:  selectedExampleBodyType(examples, selectedStatus),
		RequestHeaders:    requestHeaders,
		ResponseHeaders:   selectedExampleHeaders(examples, selectedStatus),
		ResponseExamples:  examples,
		RequestBodyRaw:    input.RequestBodyRaw,
		RequestBodyType:   strings.TrimSpace(input.RequestBodyType),
		RequestBodyKeys:   bodyKeys,
		RequestBodyTypes:  bodyTypes,
		RequestParamKeys:  paramKeys,
		RequestParamTypes: paramTypes,
	}, input.MockEnabled, input.ProxyEnabled, nil
}

func inferAnyBodyType(value any) string {
	if value == nil {
		return "null"
	}
	switch value.(type) {
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case float64, int, int64:
		return "number"
	case bool:
		return "boolean"
	default:
		return "string"
	}
}

func requestObjectSource(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return map[string]any{}
	}
	if object, ok := items[0].(map[string]any); ok {
		return object
	}
	return map[string]any{}
}

func validHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
		return true
	default:
		return false
	}
}

func normalizeManualRoutePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return trimmed
}

func normalizeResponseExamples(input []ResponseExample, selected int) ([]ResponseExample, int, string, error) {
	if len(input) == 0 {
		input = []ResponseExample{{Status: 200, Name: "OK", Body: "{}", BodyType: "json"}}
	}
	examples := make([]ResponseExample, 0, len(input))
	seen := map[int]bool{}
	for _, example := range input {
		if !validHTTPStatus(example.Status) {
			return nil, 0, "", errors.New("response status must be a valid HTTP status code")
		}
		if seen[example.Status] {
			return nil, 0, "", errors.New("response statuses must be unique")
		}
		seen[example.Status] = true
		name := strings.TrimSpace(example.Name)
		if name == "" {
			name = "Response"
		}
		headers, err := normalizeHeadersJSON(example.ResponseHeaders)
		if err != nil {
			return nil, 0, "", err
		}
		examples = append(examples, ResponseExample{Status: example.Status, Name: name, Body: example.Body, BodyType: normalizeResponseBodyType(example.BodyType), ResponseHeaders: headers})
	}
	selectedExample := examples[0]
	for _, example := range examples {
		if example.Status == selected {
			selectedExample = example
			break
		}
	}
	return examples, selectedExample.Status, selectedExample.Body, nil
}

func selectedExampleBodyType(examples []ResponseExample, selected int) string {
	for _, example := range examples {
		if example.Status == selected {
			return example.BodyType
		}
	}
	if len(examples) > 0 {
		return examples[0].BodyType
	}
	return "json"
}

func selectedExampleHeaders(examples []ResponseExample, selected int) string {
	for _, example := range examples {
		if example.Status == selected {
			return example.ResponseHeaders
		}
	}
	if len(examples) > 0 {
		return examples[0].ResponseHeaders
	}
	return "{}"
}

func normalizeResponseBodyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "json", "xml", "html", "yaml", "javascript", "raw":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "json"
	}
}

func normalizeHeadersJSON(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "{}", nil
	}
	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return "", errors.New("headers must be a JSON object")
	}
	normalized := map[string]string{}
	for key, headerValue := range parsed {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = strings.TrimSpace(headerValue)
	}
	data, _ := json.Marshal(normalized)
	return string(data), nil
}

func validHTTPStatus(status int) bool {
	switch status {
	case 100, 101, 102, 103,
		200, 201, 202, 203, 204, 205, 206, 207, 208, 226,
		300, 301, 302, 303, 304, 305, 307, 308,
		400, 401, 402, 403, 404, 405, 406, 407, 408, 409, 410, 411, 412, 413, 414, 415, 416, 417, 418, 421, 422, 423, 424, 425, 426, 428, 429, 431, 451,
		500, 501, 502, 503, 504, 505, 506, 507, 508, 510, 511:
		return true
	default:
		return false
	}
}

func uniqueTrimmed(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func normalizeBodyType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "string", "number", "boolean", "object", "array", "null", "file":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func parseStringSlice(value string) []string {
	var items []string
	if err := json.Unmarshal([]byte(value), &items); err != nil || items == nil {
		return []string{}
	}
	return items
}

func parseRequestBodyTypes(value string) RequestBodyTypes {
	types := RequestBodyTypes{}
	if err := json.Unmarshal([]byte(value), &types); err != nil || types == nil {
		return RequestBodyTypes{}
	}
	return types
}

func ResolveTemplateVariables(template string, variables map[string]string) string {
	result := template
	for key, value := range variables {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return result
}

func RoutePathsMatch(candidateRoutePath, requestedRoutePath string) bool {
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

func WildcardRouteParamKeys(routePath string) []string {
	segments := splitRouteSegments(comparableRoutePath(routePath))
	keys := []string{}
	for _, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			keys = append(keys, segment[1:])
		}
	}
	return keys
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

func directorySegments(api APIRecord, mode string) []string {
	if mode == "postman" {
		segments := []string{api.CollectionName}
		segments = append(segments, api.PostmanFolderPath...)
		return nonEmptyOrRoot(segments)
	}
	pathOnly := strings.Split(api.ResolvedRoutePath, "?")[0]
	segments := strings.Split(strings.Trim(pathOnly, "/"), "/")
	if len(segments) > 1 {
		return nonEmptyOrRoot(segments[:len(segments)-1])
	}
	return []string{"Root"}
}

func nonEmptyOrRoot(values []string) []string {
	result := []string{}
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	if len(result) == 0 {
		return []string{"Root"}
	}
	return result
}

func normalizeDirectoryPath(values []string) ([]string, error) {
	result := []string{}
	for _, value := range values {
		segment := strings.TrimSpace(value)
		if segment == "" {
			continue
		}
		if segment == "." || segment == ".." || strings.ContainsAny(segment, `/\`) || strings.ContainsRune(segment, 0) {
			return nil, ErrInvalidDirectoryPath
		}
		result = append(result, segment)
	}
	return result, nil
}

func isDescendant(segments, path []string) bool {
	if len(path) > len(segments) {
		return false
	}
	for index, segment := range path {
		if segments[index] != segment {
			return false
		}
	}
	return true
}
