package mockapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"postman-transform/backend-golang/internal/constants"
)

type BodyValidationResult struct {
	OK             bool           `json:"ok"`
	MissingKeys    []string       `json:"missingKeys"`
	UnexpectedKeys []string       `json:"unexpectedKeys"`
	TypeMismatches []TypeMismatch `json:"typeMismatches"`
}

type TypeMismatch struct {
	Key          string `json:"key"`
	ExpectedType string `json:"expectedType"`
	ActualType   string `json:"actualType"`
}

type ParamValidationResult struct {
	OK             bool     `json:"ok"`
	ExpectedKeys   []string `json:"expectedKeys"`
	MissingKeys    []string `json:"missingKeys"`
	UnexpectedKeys []string `json:"unexpectedKeys"`
}

func ValidateRequestKeys(mock APIRecord, body any) BodyValidationResult {
	if !hasBodyValidationMethod(mock.Method) || len(mock.ExpectedRequestKeys) == 0 {
		return BodyValidationResult{OK: true, MissingKeys: []string{}, UnexpectedKeys: []string{}, TypeMismatches: []TypeMismatch{}}
	}

	incoming, ok := body.(map[string]any)
	if !ok || incoming == nil {
		return BodyValidationResult{OK: false, MissingKeys: mock.ExpectedRequestKeys, UnexpectedKeys: []string{}, TypeMismatches: []TypeMismatch{}}
	}

	incomingKeys := sortedKeys(incoming)
	expectedKeys := append([]string{}, mock.ExpectedRequestKeys...)
	sort.Strings(expectedKeys)
	missing := difference(expectedKeys, incomingKeys)
	unexpected := difference(incomingKeys, expectedKeys)
	mismatches := []TypeMismatch{}
	for _, key := range expectedKeys {
		expectedType := mock.ExpectedRequestTypes[key]
		if expectedType == "" {
			continue
		}
		value, exists := incoming[key]
		if !exists {
			continue
		}
		actualType := InferJSONType(value)
		if actualType != expectedType {
			mismatches = append(mismatches, TypeMismatch{Key: key, ExpectedType: expectedType, ActualType: actualType})
		}
	}
	return BodyValidationResult{OK: len(missing) == 0 && len(unexpected) == 0 && len(mismatches) == 0, MissingKeys: missing, UnexpectedKeys: unexpected, TypeMismatches: mismatches}
}

func ValidateRequestParams(mock APIRecord, query map[string][]string) ParamValidationResult {
	if mock.Method != http.MethodGet && mock.Method != http.MethodDelete {
		return ParamValidationResult{OK: true, ExpectedKeys: []string{}, MissingKeys: []string{}, UnexpectedKeys: []string{}}
	}

	wildcards := map[string]bool{}
	for _, key := range WildcardRouteParamKeys(mock.ResolvedRoutePath) {
		wildcards[key] = true
	}
	expected := []string{}
	for _, key := range mock.ExpectedParamKeys {
		if !wildcards[key] {
			expected = append(expected, key)
		}
	}
	sort.Strings(expected)
	if len(expected) == 0 {
		return ParamValidationResult{OK: true, ExpectedKeys: expected, MissingKeys: []string{}, UnexpectedKeys: []string{}}
	}

	incoming := make([]string, 0, len(query))
	for key := range query {
		incoming = append(incoming, key)
	}
	sort.Strings(incoming)
	missing := difference(expected, incoming)
	unexpected := difference(incoming, expected)
	return ParamValidationResult{OK: len(missing) == 0 && len(unexpected) == 0, ExpectedKeys: expected, MissingKeys: missing, UnexpectedKeys: unexpected}
}

func ParseValidationBody(req *http.Request, body []byte) any {
	if len(body) == 0 {
		return nil
	}
	contentType := strings.ToLower(req.Header.Get("content-type"))
	if hasContentTypeMarker(contentType, constants.JSONContentTypeMarkers) {
		var parsed any
		if json.Unmarshal(body, &parsed) == nil {
			return parsed
		}
		return string(body)
	}
	if strings.Contains(contentType, constants.ContentTypeFormURLEncoded) {
		values, _ := url.ParseQuery(string(body))
		result := map[string]any{}
		for key, items := range values {
			if len(items) == 1 {
				result[key] = items[0]
			} else {
				valuesAny := make([]any, len(items))
				for index, value := range items {
					valuesAny[index] = value
				}
				result[key] = valuesAny
			}
		}
		return result
	}
	if strings.Contains(contentType, constants.ContentTypeGraphQL) {
		return map[string]any{"query": string(body)}
	}
	return body
}

func hasContentTypeMarker(contentType string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(contentType, marker) {
			return true
		}
	}
	return false
}

func InferJSONType(value any) string {
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

func hasBodyValidationMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

func sortedKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func difference(left, right []string) []string {
	rightSet := map[string]bool{}
	for _, item := range right {
		rightSet[item] = true
	}
	result := []string{}
	for _, item := range left {
		if !rightSet[item] {
			result = append(result, item)
		}
	}
	return result
}
