package postman

import "testing"

func TestParseCollectionExtractsNestedRequestMetadata(t *testing.T) {
	payload := []byte(`{
		"info": {"name": "Demo"},
		"item": [{
			"name": "Users",
			"item": [{
				"name": "Create user",
				"request": {
					"method": "post",
					"url": {"raw": "https://api.example.test/users?tenant=acme"},
					"body": {
						"mode": "raw",
						"raw": "{\"name\":\"Ada\",\"active\":true}",
						"options": {"raw": {"language": "json"}}
					}
				},
				"response": [{"name": "Created", "code": 201, "body": "{\"ok\":true}"}]
			}]
		}]
	}`)

	definitions, err := ParseCollection(payload)
	if err != nil {
		t.Fatalf("ParseCollection returned error: %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(definitions))
	}

	got := definitions[0]
	if got.CollectionName != "Demo" || got.RouteName != "Create user" || got.Method != "POST" || got.RoutePath != "/users?tenant=acme" {
		t.Fatalf("unexpected definition: %+v", got)
	}
	if len(got.PostmanFolderPath) != 1 || got.PostmanFolderPath[0] != "Users" {
		t.Fatalf("unexpected folder path: %#v", got.PostmanFolderPath)
	}
	if got.ResponseStatus != 201 || got.ResponseExamples[0].Name != "Created" {
		t.Fatalf("unexpected response examples: %#v", got.ResponseExamples)
	}
	if got.RequestBodyTypes["name"] != "string" || got.RequestBodyTypes["active"] != "boolean" {
		t.Fatalf("unexpected request body types: %#v", got.RequestBodyTypes)
	}
	if len(got.RequestParamKeys) != 1 || got.RequestParamKeys[0] != "tenant" {
		t.Fatalf("unexpected params: %#v", got.RequestParamKeys)
	}
}

func TestExtractRequestBodyHandlesGraphQLVariables(t *testing.T) {
	body := RequestBody{Mode: "graphql"}
	body.GraphQL.Query = "query Users { users { id } }"
	body.GraphQL.Variables = `{"limit":10}`

	keys, types := ExtractRequestBody(body)
	if len(keys) != 2 || keys[0] != "query" || keys[1] != "variables" {
		t.Fatalf("unexpected GraphQL keys: %#v", keys)
	}
	if types["query"] != "string" || types["variables"] != "object" {
		t.Fatalf("unexpected GraphQL types: %#v", types)
	}
}

func TestExtractRequestBodyHandlesJSONLineComments(t *testing.T) {
	body := RequestBody{
		Mode: "raw",
		Raw: `{
			// "device_id": "ignored",
			"device_serial": "TEST-WATCH-002",
			"operator_user_ids": ["69637bc31cf69522d0f2396c"],
			"active": true
		}`,
	}
	body.Options.Raw.Language = "json"

	keys, types := ExtractRequestBody(body)
	if len(keys) != 3 || keys[0] != "active" || keys[1] != "device_serial" || keys[2] != "operator_user_ids" {
		t.Fatalf("unexpected request body keys: %#v", keys)
	}
	if types["device_serial"] != "string" || types["operator_user_ids"] != "array" || types["active"] != "boolean" {
		t.Fatalf("unexpected request body types: %#v", types)
	}
}
