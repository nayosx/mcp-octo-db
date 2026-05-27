package main

import (
	"os"
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

func TestLoadConfig(t *testing.T) {
	// Backup and clear relevant env variables
	backupEnv := make(map[string]string)
	keys := []string{
		"MCP_ENABLE_WRITE",
		"MCP_MAX_ROWS",
		"MCP_QUERY_TIMEOUT_SECONDS",
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
	GlobalSettings = Settings{
		EnableWrite:         false,
		MaxRows:             500,
		QueryTimeoutSeconds: 10,
		AllowedSchemas:      []string{},
		AllowedTables:       []string{},
		DeniedTables:        []string{},
		LogLevel:            "info",
		LogFormat:           "text",
		AuditLog:            false,
	}

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
	if len(GlobalSettings.AllowedSchemas) != 1 || GlobalSettings.AllowedSchemas[0] != "public" {
		t.Errorf("Unexpected AllowedSchemas: %v", GlobalSettings.AllowedSchemas)
	}
	if len(GlobalSettings.DeniedTables) != 1 || GlobalSettings.DeniedTables[0] != "secret_table" {
		t.Errorf("Unexpected DeniedTables: %v", GlobalSettings.DeniedTables)
	}
}
