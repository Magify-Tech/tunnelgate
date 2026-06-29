package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
	_ "github.com/microsoft/go-mssqldb"
)

type Provider string

const (
	ProviderSQLite      Provider = "sqlite"
	ProviderPostgres    Provider = "postgresql"
	ProviderMySQL       Provider = "mysql"
	ProviderMariaDB     Provider = "mariadb"
	ProviderSQLServer   Provider = "sqlserver"
	ProviderCockroach   Provider = "cockroachdb"
	defaultSQLiteURL             = "file:./data/mock-engine.sqlite"
	sqliteDriverName             = "sqlite3"
	postgresDriverName           = "pgx"
	mysqlDriverName              = "mysql"
	sqlServerDriverName          = "sqlserver"
)

type Connection struct {
	DB       *sql.DB
	Provider Provider
}

type Tx struct {
	tx       *sql.Tx
	Provider Provider
}

func Open(databaseURL, provider string) (*Connection, error) {
	resolvedProvider := NormalizeProvider(provider)
	if resolvedProvider == "" {
		resolvedProvider = InferProvider(databaseURL)
	}
	driver, dsn, err := driverConfig(databaseURL, resolvedProvider)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Connection{DB: db, Provider: resolvedProvider}, nil
}

func OpenSQLite(databaseURL string) (*Connection, error) {
	return Open(databaseURL, string(ProviderSQLite))
}

func SQLitePath(databaseURL string) (string, error) {
	value := strings.TrimSpace(databaseURL)
	if value == "" {
		value = defaultSQLiteURL
	}
	if strings.HasPrefix(value, "file:") {
		parsed, err := url.Parse(value)
		if err == nil && parsed.Path != "" {
			if parsed.Host != "" {
				return filepath.Join(string(filepath.Separator), parsed.Host, parsed.Path), nil
			}
			return parsed.Path, nil
		}
		return strings.TrimPrefix(value, "file:"), nil
	}
	if strings.HasPrefix(value, "sqlite:") {
		return strings.TrimPrefix(value, "sqlite:"), nil
	}
	if strings.Contains(value, "://") {
		return "", fmt.Errorf("sqlite provider requires file: or sqlite: database URL")
	}
	return value, nil
}

func NormalizeProvider(provider string) Provider {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "auto":
		return ""
	case "sqlite", "sqlite3":
		return ProviderSQLite
	case "postgres", "postgresql", "pg":
		return ProviderPostgres
	case "mysql":
		return ProviderMySQL
	case "mariadb", "maria":
		return ProviderMariaDB
	case "mssql", "sqlserver", "sql_server":
		return ProviderSQLServer
	case "cockroach", "cockroachdb":
		return ProviderCockroach
	default:
		return Provider(provider)
	}
}

func InferProvider(databaseURL string) Provider {
	value := strings.ToLower(strings.TrimSpace(databaseURL))
	switch {
	case value == "", strings.HasPrefix(value, "file:"), strings.HasPrefix(value, "sqlite:"), !strings.Contains(value, "://"):
		return ProviderSQLite
	case strings.HasPrefix(value, "postgres://"), strings.HasPrefix(value, "postgresql://"):
		return ProviderPostgres
	case strings.HasPrefix(value, "mysql://"):
		return ProviderMySQL
	case strings.HasPrefix(value, "mariadb://"):
		return ProviderMariaDB
	case strings.HasPrefix(value, "sqlserver://"), strings.HasPrefix(value, "mssql://"):
		return ProviderSQLServer
	case strings.HasPrefix(value, "cockroach://"), strings.HasPrefix(value, "cockroachdb://"):
		return ProviderCockroach
	default:
		return ProviderSQLite
	}
}

func (c *Connection) Close() error {
	if c == nil || c.DB == nil {
		return nil
	}
	return c.DB.Close()
}

func (c *Connection) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := c.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx, Provider: c.Provider}, nil
}

func (c *Connection) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return c.DB.ExecContext(ctx, Rebind(query, c.Provider), args...)
}

func (c *Connection) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return c.DB.QueryContext(ctx, Rebind(query, c.Provider), args...)
}

func (c *Connection) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return c.DB.QueryRowContext(ctx, Rebind(query, c.Provider), args...)
}

func (c *Connection) UpsertKeyValue(ctx context.Context, table, key, value, updatedAt string) error {
	return upsertKeyValue(ctx, c, table, key, value, updatedAt)
}

func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.tx.ExecContext(ctx, Rebind(query, tx.Provider), args...)
}

func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.tx.QueryContext(ctx, Rebind(query, tx.Provider), args...)
}

func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.tx.QueryRowContext(ctx, Rebind(query, tx.Provider), args...)
}

func (tx *Tx) Commit() error {
	return tx.tx.Commit()
}

func (tx *Tx) Rollback() error {
	return tx.tx.Rollback()
}

func Rebind(query string, provider Provider) string {
	switch provider {
	case ProviderPostgres, ProviderCockroach:
		return numberedPlaceholders(query, "$")
	case ProviderSQLServer:
		return numberedPlaceholders(query, "@p")
	default:
		return query
	}
}

func LimitOffset(provider Provider, limit, offset int) (string, []any) {
	if provider == ProviderSQLServer {
		return " OFFSET ? ROWS FETCH NEXT ? ROWS ONLY", []any{offset, limit}
	}
	return " LIMIT ? OFFSET ?", []any{limit, offset}
}

func BoolValue(value bool) int {
	if value {
		return 1
	}
	return 0
}

func IsPostgresFamily(provider Provider) bool {
	return provider == ProviderPostgres || provider == ProviderCockroach
}

func IsMySQLFamily(provider Provider) bool {
	return provider == ProviderMySQL || provider == ProviderMariaDB
}

func KeyColumn(provider Provider) string {
	if provider == ProviderSQLServer {
		return "[key]"
	}
	if IsMySQLFamily(provider) {
		return "`key`"
	}
	return "key"
}

func driverConfig(databaseURL string, provider Provider) (string, string, error) {
	switch provider {
	case ProviderSQLite:
		path, err := SQLitePath(databaseURL)
		if err != nil {
			return "", "", err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", "", err
		}
		return sqliteDriverName, path + "?_foreign_keys=on", nil
	case ProviderPostgres, ProviderCockroach:
		return postgresDriverName, strings.TrimSpace(databaseURL), nil
	case ProviderMySQL, ProviderMariaDB:
		dsn, err := mysqlDSN(databaseURL)
		return mysqlDriverName, dsn, err
	case ProviderSQLServer:
		return sqlServerDriverName, strings.TrimSpace(databaseURL), nil
	default:
		return "", "", fmt.Errorf("unsupported SQL_PROVIDER %q", provider)
	}
}

func mysqlDSN(databaseURL string) (string, error) {
	value := strings.TrimSpace(databaseURL)
	if !strings.Contains(value, "://") {
		return value, nil
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	auth := username
	if password != "" {
		auth += ":" + password
	}
	host := parsed.Host
	if !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, "3306")
	}
	params := parsed.Query()
	if !params.Has("parseTime") {
		params.Set("parseTime", "true")
	}
	if !params.Has("multiStatements") {
		params.Set("multiStatements", "true")
	}
	return fmt.Sprintf("%s@tcp(%s)%s?%s", auth, host, parsed.EscapedPath(), params.Encode()), nil
}

func numberedPlaceholders(query, prefix string) string {
	var builder strings.Builder
	index := 1
	for _, char := range query {
		if char == '?' {
			builder.WriteString(prefix)
			builder.WriteString(fmt.Sprint(index))
			index++
			continue
		}
		builder.WriteRune(char)
	}
	return builder.String()
}

type keyValueStore interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	ProviderName() Provider
}

func upsertKeyValue(ctx context.Context, store keyValueStore, table, key, value, updatedAt string) error {
	keyColumn := KeyColumn(store.ProviderName())
	switch store.ProviderName() {
	case ProviderSQLite, ProviderPostgres, ProviderCockroach:
		_, err := store.ExecContext(ctx, `
			INSERT INTO `+table+` (`+keyColumn+`, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(`+keyColumn+`) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, updatedAt)
		return err
	case ProviderMySQL, ProviderMariaDB:
		_, err := store.ExecContext(ctx, `
			INSERT INTO `+table+` (`+keyColumn+`, value, updated_at)
			VALUES (?, ?, ?)
			ON DUPLICATE KEY UPDATE value = VALUES(value), updated_at = VALUES(updated_at)
		`, key, value, updatedAt)
		return err
	case ProviderSQLServer:
		result, err := store.ExecContext(ctx, `UPDATE `+table+` SET value = ?, updated_at = ? WHERE `+keyColumn+` = ?`, value, updatedAt, key)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		if affected > 0 {
			return nil
		}
		_, err = store.ExecContext(ctx, `INSERT INTO `+table+` (`+keyColumn+`, value, updated_at) VALUES (?, ?, ?)`, key, value, updatedAt)
		return err
	default:
		return fmt.Errorf("unsupported SQL provider %q", store.ProviderName())
	}
}

func (c *Connection) ProviderName() Provider { return c.Provider }

func (tx *Tx) ProviderName() Provider { return tx.Provider }
