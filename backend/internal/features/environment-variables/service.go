package environment

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"postman-transform/backend-golang/internal/constants"
	"postman-transform/backend-golang/internal/database"
	"postman-transform/backend-golang/internal/sanitize"
)

var environmentVariableKeyPattern = regexp.MustCompile(constants.EnvironmentVariableKeyPattern)

type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Service struct {
	db      *database.Connection
	writeMu sync.Mutex
}

func NewService(db *database.Connection) *Service {
	return &Service{db: db}
}

func (s *Service) ImportPayload(ctx context.Context, payload []byte) (string, int, error) {
	var parsed struct {
		Name   string `json:"name"`
		Values []struct {
			Key     string `json:"key"`
			Value   any    `json:"value"`
			Enabled *bool  `json:"enabled"`
		} `json:"values"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return "", 0, err
	}
	if parsed.Values == nil {
		return "", 0, errors.New("Uploaded file is not a valid collection environment")
	}

	name, err := sanitize.Control(strings.TrimSpace(parsed.Name), "Environment name")
	if err != nil {
		return "", 0, err
	}
	if name == "" {
		name = "Imported Environment"
	}

	imported := 0
	for _, item := range parsed.Values {
		if item.Enabled != nil && !*item.Enabled {
			continue
		}
		key := strings.TrimSpace(item.Key)
		if key == "" {
			continue
		}
		value := strings.TrimSpace(toString(item.Value))
		if _, err := s.Upsert(ctx, key, value); err != nil {
			return "", 0, err
		}
		imported++
	}
	if imported == 0 {
		return "", 0, errors.New("No enabled variables were found in the uploaded environment")
	}
	return name, imported, nil
}

func (s *Service) Upsert(ctx context.Context, key, value string) (Variable, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	key, value, err := normalizeVariable(key, value)
	if err != nil {
		return Variable{}, err
	}
	err = s.db.UpsertKeyValue(ctx, "postman_environment_variables", key, value, timestamp())
	if err != nil {
		return Variable{}, err
	}
	return Variable{Key: key, Value: value}, nil
}

func (s *Service) List(ctx context.Context) (map[string]string, error) {
	keyColumn := database.KeyColumn(s.db.Provider)
	rows, err := s.db.QueryContext(ctx, `SELECT `+keyColumn+`, value FROM postman_environment_variables ORDER BY `+keyColumn+` ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := map[string]string{}
	for rows.Next() {
		var item Variable
		if err := rows.Scan(&item.Key, &item.Value); err != nil {
			return nil, err
		}
		items[item.Key] = item.Value
	}
	return items, rows.Err()
}

func (s *Service) ListItems(ctx context.Context) ([]Variable, error) {
	values, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]Variable, 0, len(keys))
	for _, key := range keys {
		items = append(items, Variable{Key: key, Value: values[key]})
	}
	return items, nil
}

func (s *Service) ExportPostmanEnvironment(ctx context.Context) ([]byte, error) {
	items, err := s.ListItems(ctx)
	if err != nil {
		return nil, err
	}
	values := make([]map[string]any, 0, len(items))
	for _, item := range items {
		values = append(values, map[string]any{
			"key":     item.Key,
			"value":   item.Value,
			"type":    "default",
			"enabled": true,
		})
	}
	return json.MarshalIndent(map[string]any{
		"name":   "Collection Transform Environment",
		"values": values,
	}, "", "  ")
}

func (s *Service) Update(ctx context.Context, key, nextKey, value string) (*Variable, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	key, _, err := normalizeVariable(key, "")
	if err != nil {
		return nil, err
	}
	nextKey, value, err = normalizeVariable(nextKey, value)
	if err != nil {
		return nil, err
	}

	keyColumn := database.KeyColumn(s.db.Provider)
	result, err := s.db.ExecContext(ctx, `UPDATE postman_environment_variables SET `+keyColumn+` = ?, value = ?, updated_at = ? WHERE `+keyColumn+` = ?`, nextKey, value, timestamp(), key)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, nil
	}
	return &Variable{Key: nextKey, Value: value}, nil
}

func (s *Service) Delete(ctx context.Context, key string) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	key, _, err := normalizeVariable(key, "")
	if err != nil {
		return false, err
	}
	keyColumn := database.KeyColumn(s.db.Provider)
	result, err := s.db.ExecContext(ctx, `DELETE FROM postman_environment_variables WHERE `+keyColumn+` = ?`, key)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func normalizeVariable(key, value string) (string, string, error) {
	cleanKey, err := sanitize.Control(strings.TrimSpace(key), "Environment variable key")
	if err != nil {
		return "", "", err
	}
	if !environmentVariableKeyPattern.MatchString(cleanKey) {
		return "", "", errors.New("Environment variable key can contain only letters, numbers, underscore, dash, or dot")
	}
	cleanValue, err := sanitize.Text(value, sanitize.Options{FieldName: "Environment variable value", RejectSQLInjection: true, StripHTMLTags: false})
	if err != nil {
		return "", "", err
	}
	return cleanKey, cleanValue, nil
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if raw, ok := value.(string); ok {
		return raw
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func timestamp() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}
