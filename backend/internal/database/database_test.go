package database

import "testing"

func TestRebindUsesProviderPlaceholders(t *testing.T) {
	query := `SELECT * FROM api_mocks WHERE method = ? AND route_path = ?`

	if got := Rebind(query, ProviderPostgres); got != `SELECT * FROM api_mocks WHERE method = $1 AND route_path = $2` {
		t.Fatalf("unexpected postgres query: %s", got)
	}
	if got := Rebind(query, ProviderSQLServer); got != `SELECT * FROM api_mocks WHERE method = @p1 AND route_path = @p2` {
		t.Fatalf("unexpected sqlserver query: %s", got)
	}
	if got := Rebind(query, ProviderMySQL); got != query {
		t.Fatalf("mysql placeholders should be unchanged: %s", got)
	}
}

func TestInferProviderFromDatabaseURL(t *testing.T) {
	cases := map[string]Provider{
		"file:./data/mock-engine.sqlite":              ProviderSQLite,
		"postgresql://user:pass@localhost:5432/app":   ProviderPostgres,
		"mysql://user:pass@localhost:3306/app":        ProviderMySQL,
		"mariadb://user:pass@localhost:3306/app":      ProviderMariaDB,
		"sqlserver://user:pass@localhost:1433/app":    ProviderSQLServer,
		"cockroachdb://user:pass@localhost:26257/app": ProviderCockroach,
	}
	for input, expected := range cases {
		if got := InferProvider(input); got != expected {
			t.Fatalf("expected %s for %s, got %s", expected, input, got)
		}
	}
}
