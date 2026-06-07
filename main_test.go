package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestExtractTableNames(t *testing.T) {
	tests := []struct {
		sql      string
		expected []string
	}{
		{"SELECT * FROM users", []string{"users"}},
		{"SELECT * FROM public.users JOIN roles ON users.role_id = roles.id", []string{"users", "roles"}},
		{"INSERT INTO logs (message) VALUES ('test')", []string{"logs"}},
		{"UPDATE accounts SET balance = 100 WHERE id = 1", []string{"accounts"}},
		{"DROP TABLE secrets", []string{"secrets"}},
		{"SELECT * FROM \"my_schema\".\"my_table\"", []string{"my_table"}},
		{"SELECT * FROM `db`.`transactions`", []string{"transactions"}},
	}

	for _, tc := range tests {
		got := extractTableNames(tc.sql)
		if len(got) != len(tc.expected) {
			t.Errorf("For %q, expected %v, got %v", tc.sql, tc.expected, got)
			continue
		}
		for i, v := range got {
			if v != tc.expected[i] {
				t.Errorf("For %q at index %d, expected %s, got %s", tc.sql, i, tc.expected[i], v)
			}
		}
	}
}

func TestValidateQuerySafety(t *testing.T) {
	// Setup global settings for test
	GlobalSettings.DeniedTables = []string{"secrets", "admin_users"}
	GlobalSettings.AllowedTables = []string{} // Empty means all allowed except denied

	// 1. Denylist check
	err := validateQuerySafety("SELECT * FROM users")
	if err != nil {
		t.Errorf("Expected users to be allowed, got error: %v", err)
	}

	err = validateQuerySafety("SELECT * FROM secrets")
	if err == nil {
		t.Error("Expected secrets to be blocked, but got no error")
	}

	err = validateQuerySafety("SELECT * FROM public.admin_users")
	if err == nil {
		t.Error("Expected admin_users to be blocked, but got no error")
	}

	// 2. Allowlist check
	GlobalSettings.AllowedTables = []string{"users", "posts"}
	err = validateQuerySafety("SELECT * FROM users JOIN posts ON users.id = posts.user_id")
	if err != nil {
		t.Errorf("Expected users/posts to be allowed, got error: %v", err)
	}

	err = validateQuerySafety("SELECT * FROM comments")
	if err == nil {
		t.Error("Expected comments to be blocked by allowlist, but got no error")
	}

	// Reset global settings
	GlobalSettings.AllowedTables = []string{}
	GlobalSettings.DeniedTables = []string{}
}

func TestValidateReadOnlySQL(t *testing.T) {
	GlobalSettings.AllowedTables = []string{}
	GlobalSettings.DeniedTables = []string{}
	GlobalSettings.AllowedSchemas = []string{}
	defer func() {
		GlobalSettings.AllowedTables = []string{}
		GlobalSettings.DeniedTables = []string{}
		GlobalSettings.AllowedSchemas = []string{}
	}()

	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{
			name:    "plain select allowed",
			sql:     "SELECT * FROM users",
			wantErr: false,
		},
		{
			name:    "writable cte blocked",
			sql:     "WITH deleted AS (DELETE FROM users RETURNING id) SELECT * FROM deleted",
			wantErr: true,
		},
		{
			name:    "multiple statements blocked",
			sql:     "SELECT * FROM users; SELECT * FROM posts",
			wantErr: true,
		},
		{
			name:    "keyword inside string ignored",
			sql:     "SELECT 'DELETE FROM users' AS example",
			wantErr: false,
		},
	}

	for _, tc := range tests {
		err := validateReadOnlySQL(tc.sql)
		if tc.wantErr && err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: expected no error, got %v", tc.name, err)
		}
	}
}

func TestIsSchemaAllowed(t *testing.T) {
	GlobalSettings.AllowedSchemas = []string{"public", "analytics"}

	if !isSchemaAllowed("public", "postgres") {
		t.Error("Expected schema public to be allowed")
	}

	if !isSchemaAllowed("analytics", "mysql") {
		t.Error("Expected schema analytics to be allowed")
	}

	if isSchemaAllowed("private", "postgres") {
		t.Error("Expected schema private to be blocked")
	}

	// Reset
	GlobalSettings.AllowedSchemas = []string{}
}

func TestValidateQuerySafetySchemaRestrictions(t *testing.T) {
	GlobalSettings.AllowedTables = []string{}
	GlobalSettings.DeniedTables = []string{}
	GlobalSettings.AllowedSchemas = []string{"public"}
	defer func() {
		GlobalSettings.AllowedTables = []string{}
		GlobalSettings.DeniedTables = []string{}
		GlobalSettings.AllowedSchemas = []string{}
	}()

	if err := validateQuerySafety("SELECT * FROM public.users"); err != nil {
		t.Errorf("expected public schema to be allowed, got %v", err)
	}

	if err := validateQuerySafety("SELECT * FROM private.users"); err == nil {
		t.Error("expected private schema to be blocked")
	}
}

func TestValidateSingleStatement(t *testing.T) {
	tests := []struct {
		sql     string
		wantErr bool
	}{
		{"SELECT * FROM users", false},
		{"SELECT * FROM users;", false},
		{"SELECT ';' AS semicolon", false},
		{"SELECT * FROM users; DELETE FROM users", true},
	}

	for _, tc := range tests {
		err := validateSingleStatement(tc.sql)
		if tc.wantErr && err == nil {
			t.Errorf("expected error for %q, got nil", tc.sql)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("expected no error for %q, got %v", tc.sql, err)
		}
	}
}

func TestExtractQuestionHints(t *testing.T) {
	hints := extractQuestionHints("quiero el reporte de cuantos items se vendieron para el service 133 este mes")

	if !slices.Contains(hints.IDValues, "133") {
		t.Fatalf("expected id 133 in hints, got %+v", hints)
	}
	if !slices.Contains(hints.TimeframeTerms, "este mes") && !slices.Contains(hints.TimeframeTerms, "mes") {
		t.Fatalf("expected timeframe hint, got %+v", hints)
	}
	if len(hints.MetricTerms) == 0 {
		t.Fatalf("expected metric hints, got %+v", hints)
	}
}

func TestScoreColumnMatch(t *testing.T) {
	hints := questionHints{
		TableTerms:  []string{"service"},
		MetricTerms: []string{"vend"},
	}
	match := ColumnMatch{
		Table:  "service_sales",
		Column: "items_sold",
	}
	score := scoreColumnMatch(match, hints)
	if score <= 0 {
		t.Fatalf("expected positive score, got %d", score)
	}
}

func TestSummarizeRelationships(t *testing.T) {
	relationships := []RelationshipInfo{
		{FromTable: "order_items", ToTable: "services"},
		{FromTable: "payments", ToTable: "orders"},
	}

	filtered := summarizeRelationships(relationships, []string{"services", "order_items"})
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered relationship, got %d", len(filtered))
	}
	if filtered[0].FromTable != "order_items" || filtered[0].ToTable != "services" {
		t.Fatalf("unexpected relationship: %+v", filtered[0])
	}
}

func TestLoadConfig(t *testing.T) {
	// Backup and clear relevant env variables
	backupEnv := make(map[string]string)
	keys := []string{
		"MCP_ENABLE_WRITE",
		"MCP_MAX_ROWS",
		"MCP_QUERY_TIMEOUT_SECONDS",
		"MCP_MAX_OPEN_CONNS",
		"MCP_MAX_IDLE_CONNS",
		"MCP_CONN_MAX_LIFETIME_SECONDS",
		"MCP_CONN_MAX_IDLE_TIME_SECONDS",
		"MCP_ALLOWED_SCHEMAS",
		"MCP_ALLOWED_TABLES",
		"MCP_DENIED_TABLES",
	}
	for _, k := range keys {
		if val, ok := os.LookupEnv(k); ok {
			backupEnv[k] = val
			os.Unsetenv(k)
		}
	}
	defer func() {
		for _, k := range keys {
			if val, ok := backupEnv[k]; ok {
				os.Setenv(k, val)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	// Create an empty temp env file to prevent loading the actual .env
	tmpEnv, err := os.CreateTemp("", "empty*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpEnv.Name())
	tmpEnv.Close()

	// Create a temporary YAML file
	yamlContent := `
databases:
  test_db:
    type: sqlite
    name: ":memory:"
settings:
  enable_write: true
  max_rows: 100
  query_timeout_seconds: 5
  max_open_conns: 12
  max_idle_conns: 6
  conn_max_lifetime_seconds: 60
  conn_max_idle_time_seconds: 30
  allowed_schemas:
    - public
  denied_tables:
    - secret_table
`
	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	// Reset GlobalSettings to default values before test runs
	GlobalSettings = defaultSettings

	// Load configuration using the empty env file
	dbConfigs, err := LoadConfig(tmpEnv.Name(), tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify database config
	cfg, ok := dbConfigs["test_db"]
	if !ok {
		t.Fatal("Expected test_db to be configured")
	}
	if cfg.Type != "sqlite" || cfg.Name != ":memory:" {
		t.Errorf("Unexpected database configuration: %+v", cfg)
	}

	// Verify global settings
	if !GlobalSettings.EnableWrite {
		t.Error("Expected EnableWrite to be true")
	}
	if GlobalSettings.MaxRows != 100 {
		t.Errorf("Expected MaxRows to be 100, got %d", GlobalSettings.MaxRows)
	}
	if GlobalSettings.QueryTimeoutSeconds != 5 {
		t.Errorf("Expected QueryTimeoutSeconds to be 5, got %d", GlobalSettings.QueryTimeoutSeconds)
	}
	if GlobalSettings.MaxOpenConns != 12 {
		t.Errorf("Expected MaxOpenConns to be 12, got %d", GlobalSettings.MaxOpenConns)
	}
	if GlobalSettings.MaxIdleConns != 6 {
		t.Errorf("Expected MaxIdleConns to be 6, got %d", GlobalSettings.MaxIdleConns)
	}
	if GlobalSettings.ConnMaxLifetimeSecs != 60 {
		t.Errorf("Expected ConnMaxLifetimeSecs to be 60, got %d", GlobalSettings.ConnMaxLifetimeSecs)
	}
	if GlobalSettings.ConnMaxIdleTimeSecs != 30 {
		t.Errorf("Expected ConnMaxIdleTimeSecs to be 30, got %d", GlobalSettings.ConnMaxIdleTimeSecs)
	}
	if len(GlobalSettings.AllowedSchemas) != 1 || GlobalSettings.AllowedSchemas[0] != "public" {
		t.Errorf("Unexpected AllowedSchemas: %v", GlobalSettings.AllowedSchemas)
	}
	if len(GlobalSettings.DeniedTables) != 1 || GlobalSettings.DeniedTables[0] != "secret_table" {
		t.Errorf("Unexpected DeniedTables: %v", GlobalSettings.DeniedTables)
	}
}

func TestLoadConfigNormalizesSettingsAndNames(t *testing.T) {
	tmpEnv, err := os.CreateTemp("", "empty*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpEnv.Name())
	tmpEnv.Close()

	yamlContent := `
databases:
  Analytics:
    type: POSTGRES
    host: localhost
    port: "5432"
    user: appuser
    password: secret
    name: analytics
    sslmode: disable
settings:
  allowed_schemas: [" Public ", "public", "ANALYTICS "]
  allowed_tables: [" Users ", "users", "Orders"]
  denied_tables: [" Secrets ", "secrets"]
  log_level: INFO
  log_format: JSON
`
	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	GlobalSettings = defaultSettings
	dbConfigs, err := LoadConfig(tmpEnv.Name(), tmpfile.Name())
	if err != nil {
		t.Fatalf("expected config to load, got %v", err)
	}

	if _, ok := dbConfigs["analytics"]; !ok {
		t.Fatalf("expected normalized database key 'analytics', got %v", mapsKeys(dbConfigs))
	}
	if GlobalSettings.LogLevel != "info" {
		t.Errorf("expected normalized log level info, got %s", GlobalSettings.LogLevel)
	}
	if GlobalSettings.LogFormat != "json" {
		t.Errorf("expected normalized log format json, got %s", GlobalSettings.LogFormat)
	}
	if !slices.Equal(GlobalSettings.AllowedSchemas, []string{"public", "analytics"}) {
		t.Errorf("unexpected normalized schemas: %v", GlobalSettings.AllowedSchemas)
	}
	if !slices.Equal(GlobalSettings.AllowedTables, []string{"users", "orders"}) {
		t.Errorf("unexpected normalized tables: %v", GlobalSettings.AllowedTables)
	}
	if !slices.Equal(GlobalSettings.DeniedTables, []string{"secrets"}) {
		t.Errorf("unexpected normalized denied tables: %v", GlobalSettings.DeniedTables)
	}
}

func TestLoadConfigRejectsInvalidSettings(t *testing.T) {
	tmpEnv, err := os.CreateTemp("", "empty*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpEnv.Name())
	tmpEnv.Close()

	yamlContent := `
databases:
  broken:
    type: postgres
    host: localhost
    port: "5432"
    user: appuser
    name: appdb
settings:
  max_rows: 0
  query_timeout_seconds: -1
  max_open_conns: 0
  max_idle_conns: 99
  conn_max_lifetime_seconds: -1
  conn_max_idle_time_seconds: -1
  log_level: verbose
  log_format: pretty
`
	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	GlobalSettings = defaultSettings
	_, err = LoadConfig(tmpEnv.Name(), tmpfile.Name())
	if err == nil {
		t.Fatal("expected config validation error, got nil")
	}

	wantSubstrings := []string{
		"settings.max_rows must be greater than 0",
		"settings.query_timeout_seconds must be greater than 0",
		"settings.max_open_conns must be greater than 0",
		"settings.max_idle_conns cannot be greater than settings.max_open_conns",
		"settings.conn_max_lifetime_seconds must be greater than or equal to 0",
		"settings.conn_max_idle_time_seconds must be greater than or equal to 0",
		"settings.log_level must be one of: debug, info, warn, error",
		"settings.log_format must be one of: text, json",
	}
	for _, substring := range wantSubstrings {
		if !strings.Contains(err.Error(), substring) {
			t.Errorf("expected error to contain %q, got %v", substring, err)
		}
	}
}

func TestLoadConfigRejectsIncompleteDBConfig(t *testing.T) {
	tmpEnv, err := os.CreateTemp("", "empty*.env")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpEnv.Name())
	tmpEnv.Close()

	yamlContent := `
databases:
  broken:
    type: mysql
    host: localhost
settings:
  max_rows: 10
`
	tmpfile, err := os.CreateTemp("", "config*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write([]byte(yamlContent)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	GlobalSettings = defaultSettings
	_, err = LoadConfig(tmpEnv.Name(), tmpfile.Name())
	if err == nil {
		t.Fatal("expected invalid db config error, got nil")
	}

	wantSubstrings := []string{
		"database 'broken': name is required",
		"database 'broken': port is required for mysql",
		"database 'broken': user is required for mysql",
	}
	for _, substring := range wantSubstrings {
		if !strings.Contains(err.Error(), substring) {
			t.Errorf("expected error to contain %q, got %v", substring, err)
		}
	}
}

func TestEnabledToolNames(t *testing.T) {
	GlobalSettings = defaultSettings
	tools := enabledToolNames()
	if slices.Contains(tools, "write_query") {
		t.Fatalf("write_query should be disabled by default, got %v", tools)
	}
	for _, required := range []string{"server_info", "list_views", "list_indexes", "find_columns", "list_relationships", "suggest_query_plan"} {
		if !slices.Contains(tools, required) {
			t.Fatalf("expected %s in enabled tools, got %v", required, tools)
		}
	}

	GlobalSettings.EnableWrite = true
	tools = enabledToolNames()
	if !slices.Contains(tools, "write_query") {
		t.Fatalf("write_query should be enabled when flag is true, got %v", tools)
	}
}

func TestConnectionFailureTolerance(t *testing.T) {
	// 1. Guardar el estado anterior
	originalState := appState

	// Limpiar / Setup mock data
	appState = NewServerState()

	// Mockear una base offline
	mockError := fmt.Errorf("connection timeout on port 5432")
	appState.AddConnectionError("offline_db", mockError)

	// Intentar obtener el cliente
	_, err := getClient("offline_db")
	if err == nil {
		t.Error("Expected error for offline_db, but got nil")
	} else if !strings.Contains(err.Error(), "offline") || !strings.Contains(err.Error(), "connection timeout") {
		t.Errorf("Unexpected error message: %v", err)
	}

	// Intentar obtener una base inexistente
	_, err = getClient("nonexistent_db")
	if err == nil {
		t.Error("Expected error for nonexistent_db, but got nil")
	} else if !strings.Contains(err.Error(), "not found in config") {
		t.Errorf("Unexpected error message for nonexistent_db: %v", err)
	}

	// Restaurar estado original
	appState = originalState
}

func TestEffectiveConfigMasksSecrets(t *testing.T) {
	dbConfigs := map[string]DBConfig{
		"default": {
			Type:     "postgres",
			Host:     "localhost",
			Port:     "5432",
			User:     "appuser",
			Password: "super-secret",
			Name:     "appdb",
			SSLMode:  "disable",
		},
	}

	config := EffectiveConfig(dbConfigs)
	databases := config["databases"].(map[string]map[string]any)
	if databases["default"]["password"] != "****" {
		t.Fatalf("expected masked password, got %v", databases["default"]["password"])
	}
}

func mapsKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestLoadConfigOctoDb(t *testing.T) {
	// Setup custom environment variables
	os.Setenv("OCTO_DB_DEFAULT", "main")
	os.Setenv("OCTO_DB_MAIN_DRIVER", "mysql")
	os.Setenv("OCTO_DB_MAIN_HOST", "localhost")
	os.Setenv("OCTO_DB_MAIN_PORT", "3306")
	os.Setenv("OCTO_DB_MAIN_DATABASE", "my_database")
	os.Setenv("OCTO_DB_MAIN_USER", "my_user")
	os.Setenv("OCTO_DB_MAIN_PASSWORD", "change_me")
	os.Setenv("OCTO_DB_ENABLE_WRITE", "true")
	os.Setenv("OCTO_DB_MAX_ROWS", "150")

	defer func() {
		os.Unsetenv("OCTO_DB_DEFAULT")
		os.Unsetenv("OCTO_DB_MAIN_DRIVER")
		os.Unsetenv("OCTO_DB_MAIN_HOST")
		os.Unsetenv("OCTO_DB_MAIN_PORT")
		os.Unsetenv("OCTO_DB_MAIN_DATABASE")
		os.Unsetenv("OCTO_DB_MAIN_USER")
		os.Unsetenv("OCTO_DB_MAIN_PASSWORD")
		os.Unsetenv("OCTO_DB_ENABLE_WRITE")
		os.Unsetenv("OCTO_DB_MAX_ROWS")
	}()

	// Load config without YAML file (it should load settings and databases from env only)
	dbConfigs, err := LoadConfig("", "")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	mainCfg, ok := dbConfigs["main"]
	if !ok {
		t.Fatal("Expected 'main' database to be configured")
	}

	if mainCfg.Type != "mysql" || mainCfg.Host != "localhost" || mainCfg.Port != "3306" || mainCfg.Name != "my_database" || mainCfg.User != "my_user" || mainCfg.Password != "change_me" {
		t.Errorf("Unexpected database configuration: %+v", mainCfg)
	}

	defaultCfg, ok := dbConfigs["default"]
	if !ok {
		t.Fatal("Expected 'default' database alias to be configured since it maps to default")
	}
	if defaultCfg.Name != "my_database" {
		t.Errorf("Expected default database to map to main database config, got %+v", defaultCfg)
	}

	if !GlobalSettings.EnableWrite {
		t.Error("Expected GlobalSettings.EnableWrite to be true")
	}

	if GlobalSettings.MaxRows != 150 {
		t.Errorf("Expected GlobalSettings.MaxRows to be 150, got %d", GlobalSettings.MaxRows)
	}
}

func TestAddLimitIfMissing(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		maxRows   int
		want      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "empty query remains unchanged",
			sql:     "",
			maxRows: 100,
			want:    "",
		},
		{
			name:    "whitespace query remains unchanged",
			sql:     "   ",
			maxRows: 100,
			want:    "   ",
		},
		{
			name:    "SHOW query remains unchanged",
			sql:     "SHOW TABLES",
			maxRows: 100,
			want:    "SHOW TABLES",
		},
		{
			name:    "SHOW query with semicolon remains unchanged",
			sql:     "SHOW TABLES;",
			maxRows: 100,
			want:    "SHOW TABLES;",
		},
		{
			name:    "SELECT without limit",
			sql:     "SELECT * FROM users",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT 100",
		},
		{
			name:    "SELECT with semicolon and without limit",
			sql:     "SELECT * FROM users;",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT 100;",
		},
		{
			name:    "SELECT with existing limit",
			sql:     "SELECT * FROM users LIMIT 5",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT 5",
		},
		{
			name:    "SELECT with existing limit and semicolon",
			sql:     "SELECT * FROM users LIMIT 5;",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT 5;",
		},
		{
			name:    "SELECT with placeholder limit (?)",
			sql:     "SELECT * FROM users LIMIT ?",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT ?",
		},
		{
			name:    "SELECT with placeholder limit ($1)",
			sql:     "SELECT * FROM users LIMIT $1",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT $1",
		},
		{
			name:    "SELECT with placeholder limit (:val)",
			sql:     "SELECT * FROM users LIMIT :val",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT :val",
		},
		{
			name:    "SELECT with parenthesized limit",
			sql:     "SELECT * FROM users LIMIT (10)",
			maxRows: 100,
			want:    "SELECT * FROM users LIMIT (10)",
		},
		{
			name:    "WITH query without limit",
			sql:     "WITH active_users AS (SELECT * FROM users WHERE status = 'active') SELECT * FROM active_users",
			maxRows: 100,
			want:    "WITH active_users AS (SELECT * FROM users WHERE status = 'active') SELECT * FROM active_users LIMIT 100",
		},
		{
			name:    "WITH query with limit",
			sql:     "WITH active_users AS (SELECT * FROM users) SELECT * FROM active_users LIMIT 10",
			maxRows: 100,
			want:    "WITH active_users AS (SELECT * FROM users) SELECT * FROM active_users LIMIT 10",
		},
		{
			name:    "Parenthesized SELECT without limit",
			sql:     "(SELECT * FROM users)",
			maxRows: 100,
			want:    "(SELECT * FROM users) LIMIT 100",
		},
		{
			name:      "Unbalanced parentheses (open)",
			sql:       "SELECT * FROM users WHERE id IN (SELECT user_id FROM posts",
			maxRows:   100,
			wantErr:   true,
			errSubstr: "unbalanced parentheses",
		},
		{
			name:      "Unbalanced parentheses (close)",
			sql:       "SELECT * FROM users WHERE id = 1)",
			maxRows:   100,
			wantErr:   true,
			errSubstr: "unbalanced parentheses",
		},
		{
			name:      "Unsafe ending (ends with WHERE)",
			sql:       "SELECT * FROM users WHERE",
			maxRows:   100,
			wantErr:   true,
			errSubstr: "ends with incomplete clause or unsafe keyword",
		},
		{
			name:      "Unsafe ending (ends with UNION)",
			sql:       "SELECT * FROM users UNION",
			maxRows:   100,
			wantErr:   true,
			errSubstr: "ends with incomplete clause or unsafe keyword",
		},
		{
			name:      "Unsafe keyword (INTO)",
			sql:       "SELECT * INTO new_table FROM users",
			maxRows:   100,
			wantErr:   true,
			errSubstr: "contains potentially unsafe keyword 'INTO'",
		},
		{
			name:      "Unsafe keyword (SHARE)",
			sql:       "SELECT * FROM users FOR SHARE",
			maxRows:   100,
			wantErr:   true,
			errSubstr: "contains potentially unsafe keyword 'SHARE'",
		},
		{
			name:      "Query length check",
			sql:       strings.Repeat("A", 70000),
			maxRows:   100,
			wantErr:   true,
			errSubstr: "exceeds the maximum allowed length",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := addLimitIfMissing(tc.sql, tc.maxRows)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
				}
				if !strings.Contains(err.Error(), tc.errSubstr) {
					t.Fatalf("expected error containing %q, got %q", tc.errSubstr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.want {
					t.Fatalf("expected %q, got %q", tc.want, got)
				}
			}
		})
	}
}
