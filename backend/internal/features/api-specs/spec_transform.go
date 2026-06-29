package mockapi

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"postman-transform/backend-golang/internal/constants"
	"postman-transform/backend-golang/internal/sanitize"
)

var openAPIHTTPMethods = map[string]bool{
	"delete":  true,
	"get":     true,
	"head":    true,
	"options": true,
	"patch":   true,
	"post":    true,
	"put":     true,
	"trace":   true,
}

var openAPIPathParameterPattern = regexp.MustCompile(constants.OpenAPIPathParameterPattern)

type openAPISpec struct {
	OpenAPI     string                   `json:"openapi" yaml:"openapi"`
	Swagger     string                   `json:"swagger" yaml:"swagger"`
	Info        openAPIInfo              `json:"info" yaml:"info"`
	Paths       map[string]openAPIPath   `json:"paths" yaml:"paths"`
	Components  openAPIComponents        `json:"components" yaml:"components"`
	Definitions map[string]openAPISchema `json:"definitions" yaml:"definitions"`
}

type openAPIInfo struct {
	Title   string `json:"title" yaml:"title"`
	Version string `json:"version" yaml:"version"`
}

type openAPIComponents struct {
	Schemas map[string]openAPISchema `json:"schemas" yaml:"schemas"`
}

type openAPIPath map[string]openAPIOperation

type openAPIOperation struct {
	OperationID string                     `json:"operationId" yaml:"operationId"`
	Summary     string                     `json:"summary" yaml:"summary"`
	Tags        []string                   `json:"tags" yaml:"tags"`
	Parameters  []openAPIParameter         `json:"parameters" yaml:"parameters"`
	RequestBody *openAPIRequestBody        `json:"requestBody" yaml:"requestBody"`
	Responses   map[string]openAPIResponse `json:"responses" yaml:"responses"`
}

type openAPIParameter struct {
	Name    string         `json:"name" yaml:"name"`
	In      string         `json:"in" yaml:"in"`
	Schema  *openAPISchema `json:"schema" yaml:"schema"`
	Example any            `json:"example" yaml:"example"`
}

type openAPIRequestBody struct {
	Content map[string]openAPIMediaType `json:"content" yaml:"content"`
}

type openAPIResponse struct {
	Description string                      `json:"description" yaml:"description"`
	Content     map[string]openAPIMediaType `json:"content" yaml:"content"`
}

type openAPIMediaType struct {
	Schema   *openAPISchema            `json:"schema" yaml:"schema"`
	Example  any                       `json:"example" yaml:"example"`
	Examples map[string]openAPIExample `json:"examples" yaml:"examples"`
}

type openAPIExample struct {
	Value any `json:"value" yaml:"value"`
}

type openAPISchema struct {
	Ref        string                   `json:"$ref" yaml:"$ref"`
	Type       string                   `json:"type" yaml:"type"`
	Format     string                   `json:"format" yaml:"format"`
	Properties map[string]openAPISchema `json:"properties" yaml:"properties"`
	Items      *openAPISchema           `json:"items" yaml:"items"`
	Example    any                      `json:"example" yaml:"example"`
	Enum       []any                    `json:"enum" yaml:"enum"`
}

func parseOpenAPISpec(payload []byte) ([]Definition, string, error) {
	var spec openAPISpec
	if err := yaml.Unmarshal(payload, &spec); err != nil {
		return nil, "", err
	}
	if spec.Paths == nil || (strings.TrimSpace(spec.OpenAPI) == "" && strings.TrimSpace(spec.Swagger) == "") {
		return nil, "", errors.New("Uploaded file is not a valid Swagger/OpenAPI spec")
	}

	specName, err := sanitize.Control(strings.TrimSpace(spec.Info.Title), "Swagger title")
	if err != nil {
		return nil, "", err
	}
	if specName == "" {
		specName = "Imported Swagger"
	}

	var definitions []Definition
	paths := sortedOpenAPIPaths(spec.Paths)
	for _, path := range paths {
		methods := spec.Paths[path]
		for _, method := range sortedOpenAPIMethods(methods) {
			operation := methods[method]
			definition, err := openAPIOperationToDefinition(spec, specName, path, method, operation)
			if err != nil {
				return nil, "", err
			}
			definitions = append(definitions, definition)
		}
	}
	if len(definitions) == 0 {
		return nil, "", errors.New("No request operations were found in the uploaded Swagger/OpenAPI spec")
	}
	return definitions, specName, nil
}

func openAPIOperationToDefinition(spec openAPISpec, specName, path, method string, operation openAPIOperation) (Definition, error) {
	routePath, err := sanitize.Control(openAPIPathToRoutePath(path), "Route path")
	if err != nil {
		return Definition{}, err
	}
	routeName := strings.TrimSpace(operation.Summary)
	if routeName == "" {
		routeName = strings.TrimSpace(operation.OperationID)
	}
	if routeName == "" {
		routeName = strings.ToUpper(method) + " " + routePath
	}
	routeName, err = sanitize.Control(routeName, "Route name")
	if err != nil {
		return Definition{}, err
	}

	requestKeys, requestTypes := openAPIRequestBodyKeys(spec, operation)
	requestBodyRaw, requestBodyType := openAPIRequestBodyExample(spec, operation)
	paramKeys, err := openAPIParamKeys(operation.Parameters)
	if err != nil {
		return Definition{}, err
	}
	examples, err := openAPIResponseExamples(spec, operation, strings.ToUpper(method), routePath)
	if err != nil {
		return Definition{}, err
	}

	return Definition{
		CollectionName:    specName,
		RouteName:         routeName,
		PostmanFolderPath: cleanStringSlice(operation.Tags),
		Method:            strings.ToUpper(method),
		RoutePath:         routePath,
		ResponseStatus:    examples[0].Status,
		ResponseBody:      examples[0].Body,
		ResponseBodyType:  examples[0].BodyType,
		ResponseExamples:  examples,
		RequestBodyRaw:    requestBodyRaw,
		RequestBodyType:   requestBodyType,
		RequestBodyKeys:   requestKeys,
		RequestBodyTypes:  requestTypes,
		RequestParamKeys:  paramKeys,
	}, nil
}

func openAPIRequestBodyExample(spec openAPISpec, operation openAPIOperation) (string, string) {
	media, ok := openAPIRequestMedia(operation)
	if !ok {
		return "", "none"
	}
	return openAPIMediaExampleBody(spec, media), "json"
}

func openAPIRequestBodyKeys(spec openAPISpec, operation openAPIOperation) ([]string, RequestBodyTypes) {
	media, ok := openAPIRequestMedia(operation)
	if !ok {
		return nil, RequestBodyTypes{}
	}
	keys := []string{}
	types := RequestBodyTypes{}
	if media.Schema != nil {
		schema := resolveOpenAPISchema(spec, *media.Schema)
		if strings.EqualFold(schema.Type, "array") && schema.Items != nil {
			schema = resolveOpenAPISchema(spec, *schema.Items)
		}
		for key, property := range schema.Properties {
			keys = append(keys, key)
			types[key] = openAPISchemaType(spec, property)
		}
	}

	exampleKeys, exampleTypes := openAPIExampleBodyKeys(spec, media)
	seen := map[string]bool{}
	for _, key := range keys {
		seen[key] = true
	}
	for _, key := range exampleKeys {
		if seen[key] {
			continue
		}
		keys = append(keys, key)
		types[key] = exampleTypes[key]
	}

	sort.Strings(keys)
	return keys, types
}

func openAPIRequestMedia(operation openAPIOperation) (openAPIMediaType, bool) {
	if operation.RequestBody != nil {
		if media, ok := preferredMediaType(operation.RequestBody.Content); ok {
			return media, true
		}
	}
	for _, parameter := range operation.Parameters {
		if strings.ToLower(strings.TrimSpace(parameter.In)) != "body" {
			continue
		}
		if parameter.Schema == nil && parameter.Example == nil {
			continue
		}
		return openAPIMediaType{Schema: parameter.Schema, Example: parameter.Example}, true
	}
	return openAPIMediaType{}, false
}

func openAPIExampleBodyKeys(spec openAPISpec, media openAPIMediaType) ([]string, RequestBodyTypes) {
	value, ok := openAPIMediaExampleValue(spec, media)
	if !ok {
		return nil, RequestBodyTypes{}
	}
	source := requestObjectSource(value)
	if source == nil {
		return nil, RequestBodyTypes{}
	}
	keys := make([]string, 0, len(source))
	types := RequestBodyTypes{}
	for key, value := range source {
		keys = append(keys, key)
		types[key] = inferAnyBodyType(value)
	}
	sort.Strings(keys)
	return keys, types
}

func openAPIParamKeys(parameters []openAPIParameter) ([]string, error) {
	keys := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		in := strings.ToLower(strings.TrimSpace(parameter.In))
		if in != "query" && in != "path" {
			continue
		}
		key := strings.TrimSpace(parameter.Name)
		if key == "" {
			continue
		}
		cleaned, err := sanitize.Control(key, "Request parameter key")
		if err != nil {
			return nil, err
		}
		keys = append(keys, cleaned)
	}
	sort.Strings(keys)
	return keys, nil
}

func openAPIResponseExamples(spec openAPISpec, operation openAPIOperation, method, routePath string) ([]ResponseExample, error) {
	statuses := sortedResponseStatuses(operation.Responses)
	examples := make([]ResponseExample, 0, len(statuses))
	seen := map[int]bool{}
	for _, statusKey := range statuses {
		status, ok := parseResponseStatus(statusKey)
		if !ok || seen[status] {
			continue
		}
		seen[status] = true
		response := operation.Responses[statusKey]
		body := "{}"
		if media, ok := preferredMediaType(response.Content); ok {
			body = openAPIMediaExampleBody(spec, media)
		}
		name := strings.TrimSpace(response.Description)
		if name == "" {
			name = "Response " + strconv.Itoa(status)
		}
		cleanName, err := sanitize.Control(name, "Response example name")
		if err != nil {
			return nil, err
		}
		cleanBody, err := sanitize.Payload(body, "Response example body")
		if err != nil {
			return nil, err
		}
		examples = append(examples, ResponseExample{Status: status, Name: cleanName, Body: cleanBody, BodyType: "json", ResponseHeaders: "{}"})
	}
	if len(examples) > 0 {
		return examples, nil
	}
	body, _ := json.MarshalIndent(map[string]any{
		"message": "Mock response generated from imported Swagger spec",
		"method":  method,
		"path":    routePath,
	}, "", "  ")
	return []ResponseExample{{Status: 200, Name: "Generated 200 response", Body: string(body), BodyType: "json", ResponseHeaders: "{}"}}, nil
}

func exportPostmanCollection(records []APIRecord) ([]byte, error) {
	root := &postmanFolderNode{}
	for _, record := range records {
		root.add(record.PostmanFolderPath, postmanItem(record))
	}
	collection := map[string]any{
		"info": map[string]any{
			"name":   "Collection Transform Export",
			"schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
		"item": root.itemsForJSON(),
	}
	return json.MarshalIndent(collection, "", "  ")
}

type postmanFolderNode struct {
	folders map[string]*postmanFolderNode
	items   []map[string]any
}

func (n *postmanFolderNode) add(path []string, item map[string]any) {
	if len(path) == 0 {
		n.items = append(n.items, item)
		return
	}
	if n.folders == nil {
		n.folders = map[string]*postmanFolderNode{}
	}
	name := path[0]
	if n.folders[name] == nil {
		n.folders[name] = &postmanFolderNode{}
	}
	n.folders[name].add(path[1:], item)
}

func (n *postmanFolderNode) itemsForJSON() []map[string]any {
	items := make([]map[string]any, 0, len(n.folders)+len(n.items))
	folderNames := make([]string, 0, len(n.folders))
	for name := range n.folders {
		folderNames = append(folderNames, name)
	}
	sort.Strings(folderNames)
	for _, name := range folderNames {
		items = append(items, map[string]any{"name": name, "item": n.folders[name].itemsForJSON()})
	}
	items = append(items, n.items...)
	return items
}

func postmanItem(record APIRecord) map[string]any {
	rawURL := "{{baseUrl}}" + record.RoutePath
	item := map[string]any{
		"name": record.RouteName,
		"request": map[string]any{
			"method": record.Method,
			"url":    postmanURL(rawURL, record),
		},
	}
	if len(record.ExpectedRequestKeys) > 0 {
		item["request"].(map[string]any)["body"] = postmanRequestBody(record)
	}
	responses := make([]map[string]any, 0, len(record.ResponseExamples))
	for _, example := range record.ResponseExamples {
		responses = append(responses, map[string]any{
			"name":            example.Name,
			"code":            example.Status,
			"body":            example.Body,
			"originalRequest": item["request"],
		})
	}
	item["response"] = responses
	return item
}

func postmanURL(rawURL string, record APIRecord) map[string]any {
	pathOnly, rawQuery, _ := strings.Cut(record.RoutePath, "?")
	pathSegments := splitRouteSegments(pathOnly)
	queryValues, _ := url.ParseQuery(rawQuery)
	queryItems := make([]map[string]any, 0, len(queryValues))
	for _, key := range sortedMapKeys(queryValues) {
		queryItems = append(queryItems, map[string]any{"key": key, "value": firstString(queryValues[key])})
	}
	return map[string]any{
		"raw":   rawURL,
		"host":  []string{"{{baseUrl}}"},
		"path":  pathSegments,
		"query": queryItems,
	}
}

func postmanRequestBody(record APIRecord) map[string]any {
	body := map[string]any{}
	for _, key := range record.ExpectedRequestKeys {
		body[key] = sampleForType(record.ExpectedRequestTypes[key])
	}
	payload, _ := json.MarshalIndent(body, "", "  ")
	return map[string]any{
		"mode": "raw",
		"raw":  string(payload),
		"options": map[string]any{
			"raw": map[string]any{"language": "json"},
		},
	}
}

func exportOpenAPISpec(records []APIRecord) ([]byte, error) {
	paths := map[string]map[string]any{}
	for _, record := range records {
		path := routePathToOpenAPIPath(record.RoutePath)
		method := strings.ToLower(record.Method)
		if paths[path] == nil {
			paths[path] = map[string]any{}
		}
		paths[path][method] = openAPIOperationForRecord(record)
	}
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Collection Transform Export",
			"version": "1.0.0",
		},
		"paths": paths,
	}
	return json.MarshalIndent(spec, "", "  ")
}

func openAPIOperationForRecord(record APIRecord) map[string]any {
	operation := map[string]any{
		"summary":   record.RouteName,
		"responses": openAPIResponsesForRecord(record),
	}
	if len(record.PostmanFolderPath) > 0 {
		operation["tags"] = record.PostmanFolderPath
	}
	parameters := openAPIParametersForRecord(record)
	if len(parameters) > 0 {
		operation["parameters"] = parameters
	}
	if len(record.ExpectedRequestKeys) > 0 {
		operation["requestBody"] = map[string]any{
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": openAPIRequestSchema(record),
				},
			},
		}
	}
	return operation
}

func openAPIParametersForRecord(record APIRecord) []map[string]any {
	pathParams := map[string]bool{}
	for _, key := range WildcardRouteParamKeys(record.RoutePath) {
		pathParams[key] = true
	}
	parameters := make([]map[string]any, 0, len(pathParams)+len(record.ExpectedParamKeys))
	for _, key := range sortedBoolKeys(pathParams) {
		parameters = append(parameters, map[string]any{
			"name":     key,
			"in":       "path",
			"required": true,
			"schema":   map[string]any{"type": "string"},
		})
	}
	for _, key := range record.ExpectedParamKeys {
		if pathParams[key] {
			continue
		}
		parameters = append(parameters, map[string]any{
			"name":   key,
			"in":     "query",
			"schema": map[string]any{"type": "string"},
		})
	}
	return parameters
}

func openAPIRequestSchema(record APIRecord) map[string]any {
	properties := map[string]any{}
	for _, key := range record.ExpectedRequestKeys {
		properties[key] = map[string]any{"type": openAPIType(record.ExpectedRequestTypes[key])}
	}
	return map[string]any{"type": "object", "properties": properties}
}

func openAPIResponsesForRecord(record APIRecord) map[string]any {
	responses := map[string]any{}
	for _, example := range record.ResponseExamples {
		body := any(nil)
		if json.Unmarshal([]byte(example.Body), &body) != nil {
			body = example.Body
		}
		responses[strconv.Itoa(example.Status)] = map[string]any{
			"description": example.Name,
			"content": map[string]any{
				"application/json": map[string]any{
					"example": body,
				},
			},
		}
	}
	if len(responses) == 0 {
		responses[strconv.Itoa(record.ResponseStatus)] = map[string]any{
			"description": "Imported response",
			"content": map[string]any{
				"application/json": map[string]any{
					"example": json.RawMessage(record.ResponseBody),
				},
			},
		}
	}
	return responses
}

func preferredMediaType(content map[string]openAPIMediaType) (openAPIMediaType, bool) {
	if len(content) == 0 {
		return openAPIMediaType{}, false
	}
	for _, key := range []string{"application/json", "application/*+json", "text/json"} {
		if media, ok := content[key]; ok {
			return media, true
		}
	}
	keys := make([]string, 0, len(content))
	for key := range content {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return content[keys[0]], true
}

func openAPIMediaExampleBody(spec openAPISpec, media openAPIMediaType) string {
	value, ok := openAPIMediaExampleValue(spec, media)
	if !ok {
		return "{}"
	}
	return marshalExample(value)
}

func openAPIMediaExampleValue(spec openAPISpec, media openAPIMediaType) (any, bool) {
	if media.Example != nil {
		return media.Example, true
	}
	exampleNames := make([]string, 0, len(media.Examples))
	for name := range media.Examples {
		exampleNames = append(exampleNames, name)
	}
	sort.Strings(exampleNames)
	for _, name := range exampleNames {
		if media.Examples[name].Value != nil {
			return media.Examples[name].Value, true
		}
	}
	if media.Schema != nil {
		sample := sampleForSchema(spec, *media.Schema)
		return sample, true
	}
	return nil, false
}

func marshalExample(value any) string {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(payload)
}

func sampleForSchema(spec openAPISpec, schema openAPISchema) any {
	schema = resolveOpenAPISchema(spec, schema)
	if schema.Example != nil {
		return schema.Example
	}
	switch strings.ToLower(schema.Type) {
	case "array":
		if schema.Items == nil {
			return []any{}
		}
		return []any{sampleForSchema(spec, *schema.Items)}
	case "object", "":
		if len(schema.Properties) == 0 {
			return map[string]any{}
		}
		value := map[string]any{}
		for _, key := range sortedSchemaKeys(schema.Properties) {
			value[key] = sampleForSchema(spec, schema.Properties[key])
		}
		return value
	case "integer", "number":
		return 0
	case "boolean":
		return false
	default:
		return ""
	}
}

func sampleForType(valueType string) any {
	switch openAPIType(valueType) {
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	case "number", "integer":
		return 0
	case "boolean":
		return false
	default:
		return ""
	}
}

func openAPISchemaType(spec openAPISpec, schema openAPISchema) string {
	schema = resolveOpenAPISchema(spec, schema)
	if schema.Type == "" && len(schema.Properties) > 0 {
		return "object"
	}
	if schema.Type == "integer" {
		return "number"
	}
	if schema.Type == "" {
		return "string"
	}
	return schema.Type
}

func resolveOpenAPISchema(spec openAPISpec, schema openAPISchema) openAPISchema {
	if schema.Ref == "" {
		return schema
	}
	name := strings.TrimPrefix(schema.Ref, "#/components/schemas/")
	if name != schema.Ref {
		if resolved, ok := spec.Components.Schemas[name]; ok {
			return resolved
		}
	}
	name = strings.TrimPrefix(schema.Ref, "#/definitions/")
	if name != schema.Ref {
		if resolved, ok := spec.Definitions[name]; ok {
			return resolved
		}
	}
	return schema
}

func openAPIType(valueType string) string {
	switch strings.ToLower(strings.TrimSpace(valueType)) {
	case "array":
		return "array"
	case "object":
		return "object"
	case "number":
		return "number"
	case "boolean":
		return "boolean"
	case "integer":
		return "integer"
	default:
		return "string"
	}
}

func openAPIPathToRoutePath(path string) string {
	converted := openAPIPathParameterPattern.ReplaceAllString(path, ":$1")
	if strings.TrimSpace(converted) == "" {
		return "/"
	}
	if !strings.HasPrefix(converted, "/") {
		return "/" + converted
	}
	return converted
}

func routePathToOpenAPIPath(path string) string {
	pathOnly, _, _ := strings.Cut(path, "?")
	segments := splitRouteSegments(pathOnly)
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") && len(segment) > 1 {
			segments[index] = "{" + segment[1:] + "}"
		}
	}
	converted := "/" + strings.Join(segments, "/")
	if converted == "/" && pathOnly != "/" {
		converted = pathOnly
	}
	return converted
}

func sortedOpenAPIPaths(paths map[string]openAPIPath) []string {
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedOpenAPIMethods(path openAPIPath) []string {
	methods := make([]string, 0, len(path))
	for method := range path {
		normalized := strings.ToLower(method)
		if openAPIHTTPMethods[normalized] {
			methods = append(methods, normalized)
		}
	}
	sort.Strings(methods)
	return methods
}

func sortedResponseStatuses(responses map[string]openAPIResponse) []string {
	keys := make([]string, 0, len(responses))
	for key := range responses {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left, leftOK := parseResponseStatus(keys[i])
		right, rightOK := parseResponseStatus(keys[j])
		if leftOK && rightOK {
			return left < right
		}
		if leftOK {
			return true
		}
		if rightOK {
			return false
		}
		return keys[i] < keys[j]
	})
	return keys
}

func parseResponseStatus(value string) (int, bool) {
	if strings.EqualFold(value, "default") {
		return 200, true
	}
	status, err := strconv.Atoi(value)
	return status, err == nil && status >= 100 && status <= 599
}

func sortedSchemaKeys(values map[string]openAPISchema) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedMapKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedBoolKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cleanStringSlice(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		cleaned, err := sanitize.Control(strings.TrimSpace(value), "Tag")
		if err == nil && cleaned != "" {
			items = append(items, cleaned)
		}
	}
	return items
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
