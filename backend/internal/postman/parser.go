package postman

import (
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"postman-transform/backend-golang/internal/constants"
	"postman-transform/backend-golang/internal/sanitize"
)

type ResponseExample struct {
	Status int
	Name   string
	Body   string
}

type RequestBodyTypes map[string]string

type Definition struct {
	CollectionName    string
	RouteName         string
	PostmanFolderPath []string
	Method            string
	RoutePath         string
	ResponseStatus    int
	ResponseBody      string
	ResponseExamples  []ResponseExample
	RequestBodyRaw    string
	RequestBodyType   string
	RequestBodyKeys   []string
	RequestBodyTypes  RequestBodyTypes
	RequestParamKeys  []string
}

type Collection struct {
	Info struct {
		Name string `json:"name"`
	} `json:"info"`
	Item []Item `json:"item"`
}

type Item struct {
	Name     string     `json:"name"`
	Item     []Item     `json:"item"`
	Request  *Request   `json:"request"`
	Response []Response `json:"response"`
}

type Request struct {
	Method string      `json:"method"`
	URL    URLValue    `json:"url"`
	Body   RequestBody `json:"body"`
}

type URLValue struct {
	Raw   string        `json:"raw"`
	Path  []string      `json:"path"`
	Query []QueryString `json:"query"`
}

func (u *URLValue) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err == nil {
		u.Raw = raw
		return nil
	}
	type alias URLValue
	var parsed alias
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	*u = URLValue(parsed)
	return nil
}

type QueryString struct {
	Key      string `json:"key"`
	Disabled bool   `json:"disabled"`
}

type RequestBody struct {
	Mode       string      `json:"mode"`
	Raw        string      `json:"raw"`
	URLEncoded []BodyEntry `json:"urlencoded"`
	FormData   []BodyEntry `json:"formdata"`
	GraphQL    struct {
		Query     string `json:"query"`
		Variables string `json:"variables"`
	} `json:"graphql"`
	Options struct {
		Raw struct {
			Language string `json:"language"`
		} `json:"raw"`
	} `json:"options"`
}

type BodyEntry struct {
	Key      string `json:"key"`
	Value    any    `json:"value"`
	Type     string `json:"type"`
	Disabled bool   `json:"disabled"`
}

type Response struct {
	Name string `json:"name"`
	Code int    `json:"code"`
	Body string `json:"body"`
}

func ParseCollection(payload []byte) ([]Definition, error) {
	var collection Collection
	if err := json.Unmarshal(payload, &collection); err != nil {
		return nil, err
	}
	if collection.Item == nil {
		return nil, errors.New("Uploaded file is not a valid collection")
	}

	collectionName, err := sanitize.Control(strings.TrimSpace(collection.Info.Name), "Collection name")
	if err != nil {
		return nil, err
	}
	if collectionName == "" {
		collectionName = "Imported Collection"
	}

	var definitions []Definition
	if err := collect(collection.Item, collectionName, nil, &definitions); err != nil {
		return nil, err
	}
	if len(definitions) == 0 {
		return nil, errors.New("No request items were found in the uploaded collection")
	}
	return definitions, nil
}

func collect(items []Item, collectionName string, folders []string, definitions *[]Definition) error {
	for _, item := range items {
		if len(item.Item) > 0 {
			folderName, err := sanitize.Control(strings.TrimSpace(item.Name), "Collection folder name")
			if err != nil {
				return err
			}
			nextFolders := folders
			if folderName != "" {
				nextFolders = append(append([]string{}, folders...), folderName)
			}
			if err := collect(item.Item, collectionName, nextFolders, definitions); err != nil {
				return err
			}
			continue
		}
		if item.Request == nil || strings.TrimSpace(item.Request.Method) == "" {
			continue
		}

		method, err := sanitize.Control(item.Request.Method, "HTTP method")
		if err != nil {
			return err
		}
		method = strings.ToUpper(method)
		routePath, err := sanitize.Control(extractPath(item.Request.URL), "Route path")
		if err != nil {
			return err
		}
		routeName, err := sanitize.Control(strings.TrimSpace(item.Name), "Route name")
		if err != nil {
			return err
		}
		if routeName == "" {
			routeName = method + " " + routePath
		}
		examples, err := responseExamples(item, method, routePath)
		if err != nil {
			return err
		}
		requestKeys, requestTypes := ExtractRequestBody(item.Request.Body)
		requestBodyRaw, requestBodyType := ExtractRequestBodyExample(item.Request.Body)
		paramKeys, err := ExtractRequestParamKeys(item.Request.URL)
		if err != nil {
			return err
		}

		*definitions = append(*definitions, Definition{
			CollectionName:    collectionName,
			RouteName:         routeName,
			PostmanFolderPath: folders,
			Method:            method,
			RoutePath:         routePath,
			ResponseStatus:    examples[0].Status,
			ResponseBody:      examples[0].Body,
			ResponseExamples:  examples,
			RequestBodyRaw:    requestBodyRaw,
			RequestBodyType:   requestBodyType,
			RequestBodyKeys:   requestKeys,
			RequestBodyTypes:  requestTypes,
			RequestParamKeys:  paramKeys,
		})
	}
	return nil
}

func ExtractRequestBodyExample(body RequestBody) (string, string) {
	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	switch mode {
	case constants.PostmanBodyModeURLEncoded:
		return bodyEntriesJSON(body.URLEncoded, false), constants.RequestBodyTypeFormURLEncoded
	case constants.PostmanBodyModeFormData:
		return bodyEntriesJSON(body.FormData, true), constants.RequestBodyTypeFormData
	case constants.PostmanBodyModeGraphQL:
		payload, _ := json.Marshal(map[string]any{"query": body.GraphQL.Query, "variables": body.GraphQL.Variables, "autoFetch": false})
		return string(payload), constants.RequestBodyTypeGraphQL
	case constants.PostmanBodyModeRaw:
		rawType := strings.ToLower(strings.TrimSpace(body.Options.Raw.Language))
		if rawType == "" {
			rawType = constants.RequestBodyTypeText
		}
		if rawType == constants.RequestBodyTypeXML || rawType == constants.RequestBodyTypeHTML || rawType == constants.RequestBodyTypeJavaScript || rawType == constants.RequestBodyTypeJSON {
			return body.Raw, rawType
		}
		return body.Raw, constants.RequestBodyTypeText
	default:
		return "", "none"
	}
}

func bodyEntriesJSON(entries []BodyEntry, includeDescription bool) string {
	rows := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Disabled || strings.TrimSpace(entry.Key) == "" {
			continue
		}
		row := map[string]string{
			"key":   entry.Key,
			"type":  InferValueType(entry.Value),
			"value": stringifyBodyEntryValue(entry.Value),
		}
		if strings.EqualFold(strings.TrimSpace(entry.Type), "file") {
			row["type"] = "file"
		}
		if includeDescription {
			row["description"] = ""
		}
		rows = append(rows, row)
	}
	payload, _ := json.Marshal(rows)
	return string(payload)
}

func stringifyBodyEntryValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	payload, _ := json.Marshal(value)
	return string(payload)
}

func responseExamples(item Item, method, routePath string) ([]ResponseExample, error) {
	seen := map[int]bool{}
	var examples []ResponseExample
	for index, response := range item.Response {
		status := response.Code
		if status < 100 || status > 599 {
			status = 200
		}
		if seen[status] {
			continue
		}
		seen[status] = true
		name := strings.TrimSpace(response.Name)
		if name == "" {
			name = "Example " + strconv.Itoa(index+1)
		}
		cleanName, err := sanitize.Control(name, "Response example name")
		if err != nil {
			return nil, err
		}
		body, err := sanitize.Payload(response.Body, "Response example body")
		if err != nil {
			return nil, err
		}
		examples = append(examples, ResponseExample{Status: status, Name: cleanName, Body: body})
	}
	if len(examples) > 0 {
		return examples, nil
	}
	body, _ := json.MarshalIndent(map[string]any{
		"message":   "Mock response generated from imported collection",
		"routeName": item.Name,
		"method":    method,
		"path":      routePath,
	}, "", "  ")
	return []ResponseExample{{Status: 200, Name: "Generated 200 response", Body: string(body)}}, nil
}

func extractPath(value URLValue) string {
	if len(value.Path) > 0 {
		return normalizePath("/" + strings.Join(value.Path, "/"))
	}
	return normalizePath(value.Raw)
}

func normalizePath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "/"
	}
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	if index := strings.Index(trimmed, "}}/"); index >= 0 {
		return normalizePath(trimmed[index+2:])
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Scheme != "" {
		path := parsed.EscapedPath()
		if path == "" {
			path = "/"
		}
		if parsed.RawQuery != "" {
			path += "?" + parsed.RawQuery
		}
		return path
	}
	if index := strings.Index(trimmed, "/"); index >= 0 {
		return trimmed[index:]
	}
	return "/" + trimmed
}

func ExtractRequestParamKeys(value URLValue) ([]string, error) {
	var keys []string
	if len(value.Query) > 0 {
		for _, query := range value.Query {
			if query.Disabled || strings.TrimSpace(query.Key) == "" {
				continue
			}
			key, err := sanitize.Control(strings.TrimSpace(query.Key), "Request query key")
			if err != nil {
				return nil, err
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return keys, nil
	}

	raw := value.Raw
	if raw == "" {
		return nil, nil
	}
	parsed, err := url.Parse(normalizePath(raw))
	if err != nil {
		return nil, nil
	}
	for key := range parsed.Query() {
		cleaned, err := sanitize.Control(key, "Request query key")
		if err != nil {
			return nil, err
		}
		keys = append(keys, cleaned)
	}
	sort.Strings(keys)
	return keys, nil
}

func ExtractRequestBody(body RequestBody) ([]string, RequestBodyTypes) {
	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	switch mode {
	case constants.PostmanBodyModeURLEncoded:
		return bodyEntryKeysAndTypes(body.URLEncoded)
	case constants.PostmanBodyModeFormData:
		return bodyEntryKeysAndTypes(body.FormData)
	case constants.PostmanBodyModeGraphQL:
		types := RequestBodyTypes{}
		var keys []string
		if strings.TrimSpace(body.GraphQL.Query) != "" {
			keys = append(keys, "query")
			types["query"] = "string"
		}
		if strings.TrimSpace(body.GraphQL.Variables) != "" {
			keys = append(keys, "variables")
			var parsed any
			if json.Unmarshal([]byte(body.GraphQL.Variables), &parsed) == nil {
				types["variables"] = InferValueType(parsed)
			} else {
				types["variables"] = "string"
			}
		}
		sort.Strings(keys)
		return keys, types
	default:
		if strings.TrimSpace(body.Raw) == "" || !isJSONRawBody(body) {
			return nil, RequestBodyTypes{}
		}
		var parsed any
		if parseJSONBody(body.Raw, &parsed) != nil {
			return nil, RequestBodyTypes{}
		}
		source := requestObjectSource(parsed)
		if source == nil {
			return nil, RequestBodyTypes{}
		}
		keys := make([]string, 0, len(source))
		types := RequestBodyTypes{}
		for key, value := range source {
			keys = append(keys, key)
			types[key] = InferValueType(value)
		}
		sort.Strings(keys)
		return keys, types
	}
}

func requestObjectSource(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}
	if object, ok := items[0].(map[string]any); ok {
		return object
	}
	return nil
}

func parseJSONBody(value string, target *any) error {
	if err := json.Unmarshal([]byte(value), target); err == nil {
		return nil
	}
	return json.Unmarshal([]byte(stripJSONLineComments(value)), target)
}

func stripJSONLineComments(value string) string {
	var builder strings.Builder
	inString := false
	escaped := false

	for index := 0; index < len(value); index++ {
		character := value[index]
		var nextCharacter byte
		if index+1 < len(value) {
			nextCharacter = value[index+1]
		}

		if inString {
			wasEscaped := escaped
			builder.WriteByte(character)
			if character == '"' && !wasEscaped {
				inString = false
			}
			escaped = character == '\\' && !wasEscaped
			continue
		}

		if character == '"' {
			inString = true
			builder.WriteByte(character)
			continue
		}

		if character == '/' && nextCharacter == '/' {
			for index < len(value) && value[index] != '\n' {
				index++
			}
			if index < len(value) {
				builder.WriteByte('\n')
			}
			continue
		}

		builder.WriteByte(character)
	}

	return builder.String()
}

func bodyEntryKeysAndTypes(entries []BodyEntry) ([]string, RequestBodyTypes) {
	var keys []string
	types := RequestBodyTypes{}
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if entry.Disabled || key == "" {
			continue
		}
		keys = append(keys, key)
		if strings.EqualFold(strings.TrimSpace(entry.Type), "file") {
			types[key] = "file"
		} else {
			types[key] = InferValueType(entry.Value)
		}
	}
	sort.Strings(keys)
	return keys, types
}

func isJSONRawBody(body RequestBody) bool {
	mode := strings.ToLower(strings.TrimSpace(body.Options.Raw.Language))
	if mode == "json" {
		return true
	}
	if mode != "" && mode != "raw" {
		return false
	}
	var parsed any
	return parseJSONBody(body.Raw, &parsed) == nil
}

func InferValueType(value any) string {
	if value == nil {
		return "null"
	}
	switch value.(type) {
	case []any:
		return "array"
	case map[string]any:
		return "object"
	case string:
		return "string"
	case float64, int, int64:
		return "number"
	case bool:
		return "boolean"
	default:
		return "string"
	}
}
