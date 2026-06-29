package database

import (
	"context"
	"strings"
)

func EnsureSchema(db *Connection) error {
	return EnsureConnectionSchema(db)
}

func EnsureConnectionSchema(conn *Connection) error {
	statements := schemaStatements(conn.Provider)
	for _, statement := range statements {
		if _, err := conn.ExecContext(context.Background(), statement); err != nil && !isDuplicateIndexError(err) {
			return err
		}
	}
	if err := ensureAPIMocksAllowsDuplicateRoutes(conn); err != nil {
		return err
	}
	return nil
}

func ensureAPIMocksAllowsDuplicateRoutes(conn *Connection) error {
	if conn.Provider != ProviderSQLite {
		return nil
	}
	hasUnique, err := sqliteHasAPIMocksRouteUnique(conn)
	if err != nil || !hasUnique {
		return err
	}
	for _, statement := range []string{
		`PRAGMA foreign_keys = OFF`,
		`CREATE TABLE api_mocks_without_route_unique (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			public_id TEXT,
			collection_name TEXT NOT NULL,
			route_name TEXT NOT NULL,
			postman_folder_path TEXT NOT NULL DEFAULT '[]',
			method TEXT NOT NULL,
			route_path TEXT NOT NULL,
			mock_enabled INTEGER NOT NULL DEFAULT 1,
			proxy_enabled INTEGER NOT NULL DEFAULT 1,
			response_status INTEGER NOT NULL DEFAULT 200,
			response_body TEXT NOT NULL,
			response_body_type TEXT NOT NULL DEFAULT 'json',
			request_headers TEXT NOT NULL DEFAULT '{}',
			response_headers TEXT NOT NULL DEFAULT '{}',
			request_body_keys TEXT NOT NULL DEFAULT '[]',
			request_body_types TEXT NOT NULL DEFAULT '{}',
			request_body_raw TEXT NOT NULL DEFAULT '',
			request_body_type TEXT NOT NULL DEFAULT '',
			request_param_keys TEXT NOT NULL DEFAULT '[]',
			request_param_types TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO api_mocks_without_route_unique (
			id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, response_body_type,
			request_headers, response_headers, request_body_keys, request_body_types, request_body_raw, request_body_type, request_param_keys, request_param_types, updated_at
		)
		SELECT id, public_id, collection_name, route_name, postman_folder_path, method, route_path,
			mock_enabled, proxy_enabled, response_status, response_body, COALESCE(response_body_type, 'json'),
			COALESCE(request_headers, '{}'), COALESCE(response_headers, '{}'), request_body_keys, request_body_types, COALESCE(request_body_raw, ''), COALESCE(request_body_type, ''), request_param_keys, COALESCE(request_param_types, '{}'), updated_at
		FROM api_mocks`,
		`DROP TABLE api_mocks`,
		`ALTER TABLE api_mocks_without_route_unique RENAME TO api_mocks`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_mocks_public_id ON api_mocks(public_id)`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := conn.ExecContext(context.Background(), statement); err != nil {
			return err
		}
	}
	return nil
}

func sqliteHasAPIMocksRouteUnique(conn *Connection) (bool, error) {
	rows, err := conn.QueryContext(context.Background(), `PRAGMA index_list(api_mocks)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var seq int
		var name string
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return false, err
		}
		if unique != 1 {
			continue
		}
		match, err := sqliteIndexColumnsMatch(conn, name, []string{"method", "route_path"})
		if err != nil || match {
			return match, err
		}
	}
	return false, rows.Err()
}

func sqliteIndexColumnsMatch(conn *Connection, name string, expected []string) (bool, error) {
	rows, err := conn.QueryContext(context.Background(), `PRAGMA index_info(`+name+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	columns := []string{}
	for rows.Next() {
		var seqno int
		var cid int
		var column string
		if err := rows.Scan(&seqno, &cid, &column); err != nil {
			return false, err
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(columns) != len(expected) {
		return false, nil
	}
	for index, column := range columns {
		if column != expected[index] {
			return false, nil
		}
	}
	return true, nil
}

func schemaStatements(provider Provider) []string {
	switch provider {
	case ProviderPostgres, ProviderCockroach:
		return postgresSchema()
	case ProviderMySQL, ProviderMariaDB:
		return mysqlSchema()
	case ProviderSQLServer:
		return sqlServerSchema()
	default:
		return sqliteSchema()
	}
}

func sqliteSchema() []string {
	return []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS api_mocks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			public_id TEXT,
			collection_name TEXT NOT NULL,
			route_name TEXT NOT NULL,
			postman_folder_path TEXT NOT NULL DEFAULT '[]',
			method TEXT NOT NULL,
			route_path TEXT NOT NULL,
			mock_enabled INTEGER NOT NULL DEFAULT 1,
			proxy_enabled INTEGER NOT NULL DEFAULT 1,
			response_status INTEGER NOT NULL DEFAULT 200,
			response_body TEXT NOT NULL,
			response_body_type TEXT NOT NULL DEFAULT 'json',
			request_headers TEXT NOT NULL DEFAULT '{}',
			response_headers TEXT NOT NULL DEFAULT '{}',
			request_body_keys TEXT NOT NULL DEFAULT '[]',
			request_body_types TEXT NOT NULL DEFAULT '{}',
			request_param_keys TEXT NOT NULL DEFAULT '[]',
			request_param_types TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`ALTER TABLE api_mocks ADD COLUMN public_id TEXT`,
		`ALTER TABLE api_mocks ADD COLUMN response_body_type TEXT NOT NULL DEFAULT 'json'`,
		`ALTER TABLE api_mocks ADD COLUMN request_headers TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE api_mocks ADD COLUMN response_headers TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE api_mocks ADD COLUMN request_body_raw TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE api_mocks ADD COLUMN request_body_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE api_mocks ADD COLUMN request_param_types TEXT NOT NULL DEFAULT '{}'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_mocks_public_id ON api_mocks(public_id)`,
		`CREATE TABLE IF NOT EXISTS api_mock_responses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			api_mock_id INTEGER NOT NULL,
			response_status INTEGER NOT NULL,
			response_name TEXT NOT NULL DEFAULT '',
			response_body TEXT NOT NULL,
			response_body_type TEXT NOT NULL DEFAULT 'json',
			response_headers TEXT NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(api_mock_id, response_status),
			FOREIGN KEY(api_mock_id) REFERENCES api_mocks(id) ON DELETE CASCADE
		)`,
		`ALTER TABLE api_mock_responses ADD COLUMN response_body_type TEXT NOT NULL DEFAULT 'json'`,
		`ALTER TABLE api_mock_responses ADD COLUMN response_headers TEXT NOT NULL DEFAULT '{}'`,
		`CREATE INDEX IF NOT EXISTS idx_api_mock_responses_api_mock_id ON api_mock_responses(api_mock_id)`,
		`CREATE TABLE IF NOT EXISTS engine_config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS postman_environment_variables (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			public_id TEXT,
			api_mock_id INTEGER,
			api_mock_public_id TEXT,
			route_name TEXT NOT NULL,
			method TEXT NOT NULL,
			route_path TEXT NOT NULL,
			target_url TEXT NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			request_headers TEXT NOT NULL,
			request_body TEXT NOT NULL,
			response_headers TEXT NOT NULL,
			response_body TEXT NOT NULL,
			shadow_targets TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`ALTER TABLE proxy_audit_logs ADD COLUMN public_id TEXT`,
		`ALTER TABLE proxy_audit_logs ADD COLUMN api_mock_public_id TEXT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_proxy_audit_logs_public_id ON proxy_audit_logs(public_id)`,
		`CREATE TABLE IF NOT EXISTS proxy_audit_log_shadows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			proxy_audit_log_id INTEGER NOT NULL,
			shadow_endpoint_id TEXT NOT NULL,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			target_url TEXT NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			request_headers TEXT NOT NULL,
			request_body TEXT NOT NULL,
			response_headers TEXT NOT NULL,
			response_body TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (proxy_audit_log_id) REFERENCES proxy_audit_logs(id) ON DELETE CASCADE,
			UNIQUE(proxy_audit_log_id, shadow_endpoint_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_proxy_audit_log_shadows_audit_log_id ON proxy_audit_log_shadows(proxy_audit_log_id)`,
		`CREATE TABLE IF NOT EXISTS api_mock_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			public_id TEXT NOT NULL,
			message TEXT NOT NULL,
			snapshot_json TEXT NOT NULL,
			api_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_mock_versions_public_id ON api_mock_versions(public_id)`,
	}
}

func postgresSchema() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS api_mocks (
			id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			public_id TEXT,
			collection_name TEXT NOT NULL,
			route_name TEXT NOT NULL,
			postman_folder_path TEXT NOT NULL DEFAULT '[]',
			method TEXT NOT NULL,
			route_path TEXT NOT NULL,
			mock_enabled INTEGER NOT NULL DEFAULT 1,
			proxy_enabled INTEGER NOT NULL DEFAULT 1,
			response_status INTEGER NOT NULL DEFAULT 200,
			response_body TEXT NOT NULL,
			response_body_type TEXT NOT NULL DEFAULT 'json',
			request_headers TEXT NOT NULL DEFAULT '{}',
			response_headers TEXT NOT NULL DEFAULT '{}',
			request_body_keys TEXT NOT NULL DEFAULT '[]',
			request_body_types TEXT NOT NULL DEFAULT '{}',
			request_body_raw TEXT NOT NULL DEFAULT '',
			request_body_type TEXT NOT NULL DEFAULT '',
			request_param_keys TEXT NOT NULL DEFAULT '[]',
			request_param_types TEXT NOT NULL DEFAULT '{}',
			updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS public_id TEXT`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS response_body_type TEXT NOT NULL DEFAULT 'json'`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS request_headers TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS response_headers TEXT NOT NULL DEFAULT '{}'`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS request_body_raw TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS request_body_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE api_mocks ADD COLUMN IF NOT EXISTS request_param_types TEXT NOT NULL DEFAULT '{}'`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_mocks_public_id ON api_mocks(public_id)`,
		`CREATE TABLE IF NOT EXISTS api_mock_responses (
			id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			api_mock_id BIGINT NOT NULL REFERENCES api_mocks(id) ON DELETE CASCADE,
			response_status INTEGER NOT NULL,
			response_name TEXT NOT NULL DEFAULT '',
			response_body TEXT NOT NULL,
			response_body_type TEXT NOT NULL DEFAULT 'json',
			response_headers TEXT NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			UNIQUE(api_mock_id, response_status)
		)`,
		`ALTER TABLE api_mock_responses ADD COLUMN IF NOT EXISTS response_body_type TEXT NOT NULL DEFAULT 'json'`,
		`ALTER TABLE api_mock_responses ADD COLUMN IF NOT EXISTS response_headers TEXT NOT NULL DEFAULT '{}'`,
		`CREATE INDEX IF NOT EXISTS idx_api_mock_responses_api_mock_id ON api_mock_responses(api_mock_id)`,
		`CREATE TABLE IF NOT EXISTS engine_config (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00')`,
		`CREATE TABLE IF NOT EXISTS postman_environment_variables (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00')`,
		`CREATE TABLE IF NOT EXISTS proxy_audit_logs (
			id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			public_id TEXT,
			api_mock_id BIGINT,
			api_mock_public_id TEXT,
			route_name TEXT NOT NULL,
			method TEXT NOT NULL,
			route_path TEXT NOT NULL,
			target_url TEXT NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			request_headers TEXT NOT NULL,
			request_body TEXT NOT NULL,
			response_headers TEXT NOT NULL,
			response_body TEXT NOT NULL,
			shadow_targets TEXT NOT NULL,
			created_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`ALTER TABLE proxy_audit_logs ADD COLUMN IF NOT EXISTS public_id TEXT`,
		`ALTER TABLE proxy_audit_logs ADD COLUMN IF NOT EXISTS api_mock_public_id TEXT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_proxy_audit_logs_public_id ON proxy_audit_logs(public_id)`,
		`CREATE TABLE IF NOT EXISTS proxy_audit_log_shadows (
			id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			proxy_audit_log_id BIGINT NOT NULL REFERENCES proxy_audit_logs(id) ON DELETE CASCADE,
			shadow_endpoint_id TEXT NOT NULL,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			target_url TEXT NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			request_headers TEXT NOT NULL,
			request_body TEXT NOT NULL,
			response_headers TEXT NOT NULL,
			response_body TEXT NOT NULL,
			created_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			UNIQUE(proxy_audit_log_id, shadow_endpoint_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_proxy_audit_log_shadows_audit_log_id ON proxy_audit_log_shadows(proxy_audit_log_id)`,
		`CREATE TABLE IF NOT EXISTS api_mock_versions (
			id BIGINT GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
			public_id TEXT NOT NULL,
			message TEXT NOT NULL,
			snapshot_json TEXT NOT NULL,
			api_count INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_mock_versions_public_id ON api_mock_versions(public_id)`,
	}
}

func mysqlSchema() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS api_mocks (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			public_id VARCHAR(36),
			collection_name TEXT NOT NULL,
			route_name TEXT NOT NULL,
			postman_folder_path LONGTEXT NOT NULL,
			method VARCHAR(16) NOT NULL,
			route_path VARCHAR(768) NOT NULL,
			mock_enabled INTEGER NOT NULL DEFAULT 1,
			proxy_enabled INTEGER NOT NULL DEFAULT 1,
			response_status INTEGER NOT NULL DEFAULT 200,
			response_body LONGTEXT NOT NULL,
			response_body_type VARCHAR(24) NOT NULL,
			request_headers LONGTEXT NOT NULL,
			response_headers LONGTEXT NOT NULL,
			request_body_keys LONGTEXT NOT NULL,
			request_body_types LONGTEXT NOT NULL,
			request_body_raw LONGTEXT NOT NULL,
			request_body_type VARCHAR(32) NOT NULL,
			request_param_keys LONGTEXT NOT NULL,
			request_param_types LONGTEXT NOT NULL,
			updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`ALTER TABLE api_mocks ADD COLUMN public_id VARCHAR(36)`,
		`ALTER TABLE api_mocks ADD COLUMN response_body_type VARCHAR(24) NULL`,
		`ALTER TABLE api_mocks ADD COLUMN request_headers LONGTEXT NULL`,
		`ALTER TABLE api_mocks ADD COLUMN response_headers LONGTEXT NULL`,
		`ALTER TABLE api_mocks ADD COLUMN request_body_raw LONGTEXT NULL`,
		`ALTER TABLE api_mocks ADD COLUMN request_body_type VARCHAR(32) NULL`,
		`ALTER TABLE api_mocks ADD COLUMN request_param_types LONGTEXT NULL`,
		`CREATE UNIQUE INDEX idx_api_mocks_public_id ON api_mocks(public_id)`,
		`CREATE TABLE IF NOT EXISTS api_mock_responses (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			api_mock_id BIGINT NOT NULL,
			response_status INTEGER NOT NULL,
			response_name TEXT NOT NULL,
			response_body LONGTEXT NOT NULL,
			response_body_type VARCHAR(24) NOT NULL,
			response_headers LONGTEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			UNIQUE KEY uq_api_mock_responses_api_status (api_mock_id, response_status),
			INDEX idx_api_mock_responses_api_mock_id (api_mock_id),
			CONSTRAINT fk_api_mock_responses_api FOREIGN KEY (api_mock_id) REFERENCES api_mocks(id) ON DELETE CASCADE
		)`,
		`ALTER TABLE api_mock_responses ADD COLUMN response_body_type VARCHAR(24) NULL`,
		`ALTER TABLE api_mock_responses ADD COLUMN response_headers LONGTEXT NULL`,
		`CREATE TABLE IF NOT EXISTS engine_config (` + "`key`" + ` VARCHAR(255) PRIMARY KEY, value LONGTEXT NOT NULL, updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00')`,
		`CREATE TABLE IF NOT EXISTS postman_environment_variables (` + "`key`" + ` VARCHAR(255) PRIMARY KEY, value LONGTEXT NOT NULL, updated_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00')`,
		`CREATE TABLE IF NOT EXISTS proxy_audit_logs (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			public_id VARCHAR(36),
			api_mock_id BIGINT,
			api_mock_public_id VARCHAR(36),
			route_name TEXT NOT NULL,
			method VARCHAR(16) NOT NULL,
			route_path TEXT NOT NULL,
			target_url TEXT NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			request_headers LONGTEXT NOT NULL,
			request_body LONGTEXT NOT NULL,
			response_headers LONGTEXT NOT NULL,
			response_body LONGTEXT NOT NULL,
			shadow_targets LONGTEXT NOT NULL,
			created_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`ALTER TABLE proxy_audit_logs ADD COLUMN public_id VARCHAR(36)`,
		`ALTER TABLE proxy_audit_logs ADD COLUMN api_mock_public_id VARCHAR(36)`,
		`CREATE UNIQUE INDEX idx_proxy_audit_logs_public_id ON proxy_audit_logs(public_id)`,
		`CREATE TABLE IF NOT EXISTS proxy_audit_log_shadows (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			proxy_audit_log_id BIGINT NOT NULL,
			shadow_endpoint_id VARCHAR(255) NOT NULL,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			target_url TEXT NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message TEXT,
			request_headers LONGTEXT NOT NULL,
			request_body LONGTEXT NOT NULL,
			response_headers LONGTEXT NOT NULL,
			response_body LONGTEXT NOT NULL,
			created_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			UNIQUE KEY uq_proxy_audit_log_shadows_audit_shadow (proxy_audit_log_id, shadow_endpoint_id),
			INDEX idx_proxy_audit_log_shadows_audit_log_id (proxy_audit_log_id),
			CONSTRAINT fk_proxy_audit_log_shadows_audit FOREIGN KEY (proxy_audit_log_id) REFERENCES proxy_audit_logs(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS api_mock_versions (
			id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			public_id VARCHAR(36) NOT NULL,
			message TEXT NOT NULL,
			snapshot_json LONGTEXT NOT NULL,
			api_count INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			UNIQUE KEY idx_api_mock_versions_public_id (public_id)
		)`,
	}
}

func sqlServerSchema() []string {
	return []string{
		`IF OBJECT_ID('api_mocks', 'U') IS NULL CREATE TABLE api_mocks (
			id BIGINT IDENTITY(1,1) PRIMARY KEY,
			public_id NVARCHAR(36),
			collection_name NVARCHAR(MAX) NOT NULL,
			route_name NVARCHAR(MAX) NOT NULL,
			postman_folder_path NVARCHAR(MAX) NOT NULL,
			method NVARCHAR(16) NOT NULL,
			route_path NVARCHAR(450) NOT NULL,
			mock_enabled INTEGER NOT NULL DEFAULT 1,
			proxy_enabled INTEGER NOT NULL DEFAULT 1,
			response_status INTEGER NOT NULL DEFAULT 200,
			response_body NVARCHAR(MAX) NOT NULL,
			response_body_type NVARCHAR(24) NOT NULL,
			request_headers NVARCHAR(MAX) NOT NULL,
			response_headers NVARCHAR(MAX) NOT NULL,
			request_body_keys NVARCHAR(MAX) NOT NULL,
			request_body_types NVARCHAR(MAX) NOT NULL,
			request_body_raw NVARCHAR(MAX) NOT NULL,
			request_body_type NVARCHAR(32) NOT NULL,
			request_param_keys NVARCHAR(MAX) NOT NULL,
			request_param_types NVARCHAR(MAX) NOT NULL,
			updated_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`IF COL_LENGTH('api_mocks', 'public_id') IS NULL ALTER TABLE api_mocks ADD public_id NVARCHAR(36)`,
		`IF COL_LENGTH('api_mocks', 'response_body_type') IS NULL ALTER TABLE api_mocks ADD response_body_type NVARCHAR(24) NOT NULL DEFAULT 'json'`,
		`IF COL_LENGTH('api_mocks', 'request_headers') IS NULL ALTER TABLE api_mocks ADD request_headers NVARCHAR(MAX) NOT NULL DEFAULT '{}'`,
		`IF COL_LENGTH('api_mocks', 'response_headers') IS NULL ALTER TABLE api_mocks ADD response_headers NVARCHAR(MAX) NOT NULL DEFAULT '{}'`,
		`IF COL_LENGTH('api_mocks', 'request_body_raw') IS NULL ALTER TABLE api_mocks ADD request_body_raw NVARCHAR(MAX) NOT NULL DEFAULT ''`,
		`IF COL_LENGTH('api_mocks', 'request_body_type') IS NULL ALTER TABLE api_mocks ADD request_body_type NVARCHAR(32) NOT NULL DEFAULT ''`,
		`IF COL_LENGTH('api_mocks', 'request_param_types') IS NULL ALTER TABLE api_mocks ADD request_param_types NVARCHAR(MAX) NOT NULL DEFAULT '{}'`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_api_mocks_public_id') CREATE UNIQUE INDEX idx_api_mocks_public_id ON api_mocks(public_id) WHERE public_id IS NOT NULL`,
		`IF OBJECT_ID('api_mock_responses', 'U') IS NULL CREATE TABLE api_mock_responses (
			id BIGINT IDENTITY(1,1) PRIMARY KEY,
			api_mock_id BIGINT NOT NULL,
			response_status INTEGER NOT NULL,
			response_name NVARCHAR(MAX) NOT NULL DEFAULT '',
			response_body NVARCHAR(MAX) NOT NULL,
			response_body_type NVARCHAR(24) NOT NULL,
			response_headers NVARCHAR(MAX) NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			CONSTRAINT uq_api_mock_responses_api_status UNIQUE(api_mock_id, response_status),
			CONSTRAINT fk_api_mock_responses_api FOREIGN KEY(api_mock_id) REFERENCES api_mocks(id) ON DELETE CASCADE
		)`,
		`IF COL_LENGTH('api_mock_responses', 'response_body_type') IS NULL ALTER TABLE api_mock_responses ADD response_body_type NVARCHAR(24) NOT NULL DEFAULT 'json'`,
		`IF COL_LENGTH('api_mock_responses', 'response_headers') IS NULL ALTER TABLE api_mock_responses ADD response_headers NVARCHAR(MAX) NOT NULL DEFAULT '{}'`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_api_mock_responses_api_mock_id') CREATE INDEX idx_api_mock_responses_api_mock_id ON api_mock_responses(api_mock_id)`,
		`IF OBJECT_ID('engine_config', 'U') IS NULL CREATE TABLE engine_config ([key] NVARCHAR(255) PRIMARY KEY, value NVARCHAR(MAX) NOT NULL, updated_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00')`,
		`IF OBJECT_ID('postman_environment_variables', 'U') IS NULL CREATE TABLE postman_environment_variables ([key] NVARCHAR(255) PRIMARY KEY, value NVARCHAR(MAX) NOT NULL, updated_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00')`,
		`IF OBJECT_ID('proxy_audit_logs', 'U') IS NULL CREATE TABLE proxy_audit_logs (
			id BIGINT IDENTITY(1,1) PRIMARY KEY,
			public_id NVARCHAR(36),
			api_mock_id BIGINT,
			api_mock_public_id NVARCHAR(36),
			route_name NVARCHAR(MAX) NOT NULL,
			method NVARCHAR(16) NOT NULL,
			route_path NVARCHAR(MAX) NOT NULL,
			target_url NVARCHAR(MAX) NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message NVARCHAR(MAX),
			request_headers NVARCHAR(MAX) NOT NULL,
			request_body NVARCHAR(MAX) NOT NULL,
			response_headers NVARCHAR(MAX) NOT NULL,
			response_body NVARCHAR(MAX) NOT NULL,
			shadow_targets NVARCHAR(MAX) NOT NULL,
			created_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`IF COL_LENGTH('proxy_audit_logs', 'public_id') IS NULL ALTER TABLE proxy_audit_logs ADD public_id NVARCHAR(36)`,
		`IF COL_LENGTH('proxy_audit_logs', 'api_mock_public_id') IS NULL ALTER TABLE proxy_audit_logs ADD api_mock_public_id NVARCHAR(36)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_proxy_audit_logs_public_id') CREATE UNIQUE INDEX idx_proxy_audit_logs_public_id ON proxy_audit_logs(public_id) WHERE public_id IS NOT NULL`,
		`IF OBJECT_ID('proxy_audit_log_shadows', 'U') IS NULL CREATE TABLE proxy_audit_log_shadows (
			id BIGINT IDENTITY(1,1) PRIMARY KEY,
			proxy_audit_log_id BIGINT NOT NULL,
			shadow_endpoint_id NVARCHAR(255) NOT NULL,
			name NVARCHAR(MAX) NOT NULL,
			base_url NVARCHAR(MAX) NOT NULL,
			target_url NVARCHAR(MAX) NOT NULL,
			response_status INTEGER,
			duration_ms INTEGER NOT NULL,
			success INTEGER NOT NULL,
			error_message NVARCHAR(MAX),
			request_headers NVARCHAR(MAX) NOT NULL,
			request_body NVARCHAR(MAX) NOT NULL,
			response_headers NVARCHAR(MAX) NOT NULL,
			response_body NVARCHAR(MAX) NOT NULL,
			created_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00',
			CONSTRAINT uq_proxy_audit_log_shadows_audit_shadow UNIQUE(proxy_audit_log_id, shadow_endpoint_id),
			CONSTRAINT fk_proxy_audit_log_shadows_audit FOREIGN KEY(proxy_audit_log_id) REFERENCES proxy_audit_logs(id) ON DELETE CASCADE
		)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_proxy_audit_log_shadows_audit_log_id') CREATE INDEX idx_proxy_audit_log_shadows_audit_log_id ON proxy_audit_log_shadows(proxy_audit_log_id)`,
		`IF OBJECT_ID('api_mock_versions', 'U') IS NULL CREATE TABLE api_mock_versions (
			id BIGINT IDENTITY(1,1) PRIMARY KEY,
			public_id NVARCHAR(36) NOT NULL,
			message NVARCHAR(MAX) NOT NULL,
			snapshot_json NVARCHAR(MAX) NOT NULL,
			api_count INTEGER NOT NULL DEFAULT 0,
			created_at NVARCHAR(32) NOT NULL DEFAULT '1970-01-01 00:00:00'
		)`,
		`IF NOT EXISTS (SELECT 1 FROM sys.indexes WHERE name = 'idx_api_mock_versions_public_id') CREATE UNIQUE INDEX idx_api_mock_versions_public_id ON api_mock_versions(public_id)`,
	}
}

func isDuplicateIndexError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already exists") || strings.Contains(message, "duplicate") || strings.Contains(message, "exists")
}
