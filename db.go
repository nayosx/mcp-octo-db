package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// ColumnInfo representa la información de una columna de una tabla
type ColumnInfo struct {
	Name       string  `json:"name"`
	Type       string  `json:"type"`
	Nullable   bool    `json:"nullable"`
	Default    *string `json:"default,omitempty"`
	PrimaryKey bool    `json:"primary_key"`
}

type IndexInfo struct {
	Name    string `json:"name"`
	Unique  bool   `json:"unique"`
	Columns string `json:"columns,omitempty"`
}

type ColumnMatch struct {
	Schema   string `json:"schema,omitempty"`
	Table    string `json:"table"`
	Column   string `json:"column"`
	DataType string `json:"data_type,omitempty"`
}

type RelationshipInfo struct {
	FromSchema string `json:"from_schema,omitempty"`
	FromTable  string `json:"from_table"`
	FromColumn string `json:"from_column"`
	ToSchema   string `json:"to_schema,omitempty"`
	ToTable    string `json:"to_table"`
	ToColumn   string `json:"to_column"`
}

// DBClient define los métodos comunes para interactuar con Postgres y MySQL/MariaDB
type DBClient interface {
	ListTables(ctx context.Context, schema string) ([]string, error)
	ListSchemas(ctx context.Context) ([]string, error)
	ListViews(ctx context.Context, schema string) ([]string, error)
	ListIndexes(ctx context.Context, schema, table string) ([]IndexInfo, error)
	FindColumns(ctx context.Context, schema, search string, limit int) ([]ColumnMatch, error)
	ListRelationships(ctx context.Context, schema, table string) ([]RelationshipInfo, error)
	DescribeTable(ctx context.Context, schema, table string) ([]ColumnInfo, error)
	ExecuteQuery(ctx context.Context, query string) ([]map[string]any, error)
	ExecuteReadOnlyQuery(ctx context.Context, query string) ([]map[string]any, error)
	ExecuteWrite(ctx context.Context, query string) (int64, error)
	Close() error
}

// DBConfig contiene los parámetros para construir el DSN de conexión
type DBConfig struct {
	Type     string
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
}

// DiscoverDBs escanea las variables de entorno buscando patrones de conexión
func DiscoverDBs() (map[string]DBConfig, error) {
	configs := make(map[string]DBConfig)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		if key == "DB_NAME" {
			configs["default"] = buildConfigForSuffix("")
		} else if strings.HasPrefix(key, "DB_NAME_") {
			suffix := strings.TrimPrefix(key, "DB_NAME_")
			if suffix != "" {
				configs[strings.ToLower(suffix)] = buildConfigForSuffix(suffix)
			}
		}
	}

	return configs, nil
}

func buildConfigForSuffix(suffix string) DBConfig {
	var s string
	if suffix != "" {
		s = "_" + suffix
	}

	cfg := DBConfig{
		Type:     getEnv(fmt.Sprintf("DB_TYPE%s", s), "postgres"),
		Host:     getEnv(fmt.Sprintf("DB_HOST%s", s), "localhost"),
		Port:     getEnv(fmt.Sprintf("DB_PORT%s", s), ""),
		User:     getEnv(fmt.Sprintf("DB_USER%s", s), ""),
		Password: getEnv(fmt.Sprintf("DB_PASSWORD%s", s), ""),
		Name:     getEnv(fmt.Sprintf("DB_NAME%s", s), ""),
		SSLMode:  getEnv(fmt.Sprintf("DB_SSLMODE%s", s), "disable"),
	}

	if cfg.Port == "" {
		if strings.ToLower(cfg.Type) == "postgres" || strings.ToLower(cfg.Type) == "postgresql" {
			cfg.Port = "5432"
		} else {
			cfg.Port = "3306"
		}
	}

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return defaultVal
}

// NewDBClient instancia el cliente correspondiente según el tipo configurado
func NewDBClient(cfg DBConfig) (DBClient, error) {
	connectTimeout := time.Duration(GlobalSettings.QueryTimeoutSeconds) * time.Second
	if connectTimeout <= 0 {
		connectTimeout = 10 * time.Second
	}

	switch strings.ToLower(cfg.Type) {
	case "postgres", "postgresql":
		dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name, cfg.SSLMode)
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres connection: %w", err)
		}
		configureDBPool(db)
		pingCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to ping postgres: %w", err)
		}
		return &PostgresClient{db: db}, nil

	case "mysql", "mariadb":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open mysql connection: %w", err)
		}
		configureDBPool(db)
		pingCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to ping mysql: %w", err)
		}
		return &MySQLClient{db: db}, nil

	case "sqlite", "sqlite3":
		db, err := sql.Open("sqlite", cfg.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to open sqlite connection: %w", err)
		}
		configureDBPool(db)
		pingCtx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to ping sqlite: %w", err)
		}
		return &SQLiteClient{db: db}, nil

	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Type)
	}
}

func configureDBPool(db *sql.DB) {
	db.SetMaxOpenConns(GlobalSettings.MaxOpenConns)
	db.SetMaxIdleConns(GlobalSettings.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(GlobalSettings.ConnMaxLifetimeSecs) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(GlobalSettings.ConnMaxIdleTimeSecs) * time.Second)
}

// ==========================================
// CLIENTE POSTGRES
// ==========================================

type PostgresClient struct {
	db *sql.DB
}

func (c *PostgresClient) Close() error {
	return c.db.Close()
}

func (c *PostgresClient) ListSchemas(ctx context.Context) ([]string, error) {
	query := `
		SELECT schema_name 
		FROM information_schema.schemata 
		WHERE schema_name NOT IN ('pg_catalog', 'information_schema')
		ORDER BY schema_name;`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

func (c *PostgresClient) ListTables(ctx context.Context, schema string) ([]string, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY table_name;`

	rows, err := c.db.QueryContext(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func (c *PostgresClient) ListViews(ctx context.Context, schema string) ([]string, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT table_name
		FROM information_schema.views
		WHERE table_schema = $1
		ORDER BY table_name;`

	rows, err := c.db.QueryContext(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var view string
		if err := rows.Scan(&view); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *PostgresClient) ListIndexes(ctx context.Context, schema, table string) ([]IndexInfo, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = $1 AND tablename = $2
		ORDER BY indexname;`

	rows, err := c.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		var indexDef string
		if err := rows.Scan(&idx.Name, &indexDef); err != nil {
			return nil, err
		}
		idx.Unique = strings.Contains(strings.ToUpper(indexDef), "UNIQUE INDEX")
		idx.Columns = parseIndexColumns(indexDef)
		indexes = append(indexes, idx)
	}
	return indexes, nil
}

func (c *PostgresClient) FindColumns(ctx context.Context, schema, search string, limit int) ([]ColumnMatch, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT table_schema, table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND ($2 = '' OR column_name ILIKE $2 OR table_name ILIKE $2)
		ORDER BY table_name, ordinal_position
		LIMIT $3;`

	like := ""
	if search != "" {
		like = "%" + search + "%"
	}

	rows, err := c.db.QueryContext(ctx, query, schema, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []ColumnMatch
	for rows.Next() {
		var match ColumnMatch
		if err := rows.Scan(&match.Schema, &match.Table, &match.Column, &match.DataType); err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, nil
}

func (c *PostgresClient) ListRelationships(ctx context.Context, schema, table string) ([]RelationshipInfo, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT
			tc.table_schema,
			tc.table_name,
			kcu.column_name,
			ccu.table_schema,
			ccu.table_name,
			ccu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name
		 AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON ccu.constraint_name = tc.constraint_name
		 AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema = $1
		  AND ($2 = '' OR tc.table_name = $2 OR ccu.table_name = $2)
		ORDER BY tc.table_name, kcu.column_name;`

	rows, err := c.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []RelationshipInfo
	for rows.Next() {
		var rel RelationshipInfo
		if err := rows.Scan(&rel.FromSchema, &rel.FromTable, &rel.FromColumn, &rel.ToSchema, &rel.ToTable, &rel.ToColumn); err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}
	return relationships, nil
}

func (c *PostgresClient) DescribeTable(ctx context.Context, schema, table string) ([]ColumnInfo, error) {
	if schema == "" {
		schema = "public"
	}
	query := `
		SELECT 
			c.column_name, 
			c.data_type, 
			c.is_nullable, 
			c.column_default,
			COALESCE(
				(SELECT true 
				 FROM information_schema.table_constraints tc
				 JOIN information_schema.key_column_usage kcu 
				   ON tc.constraint_name = kcu.constraint_name 
				   AND tc.table_schema = kcu.table_schema
				 WHERE tc.constraint_type = 'PRIMARY KEY' 
				   AND tc.table_schema = c.table_schema 
				   AND tc.table_name = c.table_name 
				   AND kcu.column_name = c.column_name), 
				false
			) AS is_primary
		FROM information_schema.columns c
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position;`

	rows, err := c.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var nullableStr string
		var defaultStr sql.NullString
		if err := rows.Scan(&col.Name, &col.Type, &nullableStr, &defaultStr, &col.PrimaryKey); err != nil {
			return nil, err
		}
		col.Nullable = strings.ToUpper(nullableStr) == "YES"
		if defaultStr.Valid {
			col.Default = &defaultStr.String
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (c *PostgresClient) ExecuteQuery(ctx context.Context, query string) ([]map[string]any, error) {
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (c *PostgresClient) ExecuteReadOnlyQuery(ctx context.Context, query string) ([]map[string]any, error) {
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (c *PostgresClient) ExecuteWrite(ctx context.Context, query string) (int64, error) {
	res, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==========================================
// CLIENTE MYSQL / MARIADB
// ==========================================

type MySQLClient struct {
	db *sql.DB
}

func (c *MySQLClient) Close() error {
	return c.db.Close()
}

func (c *MySQLClient) ListSchemas(ctx context.Context) ([]string, error) {
	query := `
		SELECT schema_name 
		FROM information_schema.schemata 
		ORDER BY schema_name;`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			return nil, err
		}
		schemas = append(schemas, schema)
	}
	return schemas, nil
}

func (c *MySQLClient) ListTables(ctx context.Context, schema string) ([]string, error) {
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = COALESCE(NULLIF(?, ''), DATABASE()) AND table_type = 'BASE TABLE'
		ORDER BY table_name;`

	rows, err := c.db.QueryContext(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func (c *MySQLClient) ListViews(ctx context.Context, schema string) ([]string, error) {
	query := `
		SELECT table_name
		FROM information_schema.views
		WHERE table_schema = COALESCE(NULLIF(?, ''), DATABASE())
		ORDER BY table_name;`

	rows, err := c.db.QueryContext(ctx, query, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var view string
		if err := rows.Scan(&view); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *MySQLClient) ListIndexes(ctx context.Context, schema, table string) ([]IndexInfo, error) {
	query := `
		SELECT index_name,
		       non_unique,
		       GROUP_CONCAT(column_name ORDER BY seq_in_index SEPARATOR ', ') AS columns_list
		FROM information_schema.statistics
		WHERE table_schema = COALESCE(NULLIF(?, ''), DATABASE()) AND table_name = ?
		GROUP BY index_name, non_unique
		ORDER BY index_name;`

	rows, err := c.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var idx IndexInfo
		var nonUnique int
		if err := rows.Scan(&idx.Name, &nonUnique, &idx.Columns); err != nil {
			return nil, err
		}
		idx.Unique = nonUnique == 0
		indexes = append(indexes, idx)
	}
	return indexes, nil
}

func (c *MySQLClient) FindColumns(ctx context.Context, schema, search string, limit int) ([]ColumnMatch, error) {
	query := `
		SELECT table_schema, table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = COALESCE(NULLIF(?, ''), DATABASE())
		  AND (? = '' OR column_name LIKE ? OR table_name LIKE ?)
		ORDER BY table_name, ordinal_position
		LIMIT ?;`

	like := ""
	if search != "" {
		like = "%" + search + "%"
	}

	rows, err := c.db.QueryContext(ctx, query, schema, like, like, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []ColumnMatch
	for rows.Next() {
		var match ColumnMatch
		if err := rows.Scan(&match.Schema, &match.Table, &match.Column, &match.DataType); err != nil {
			return nil, err
		}
		matches = append(matches, match)
	}
	return matches, nil
}

func (c *MySQLClient) ListRelationships(ctx context.Context, schema, table string) ([]RelationshipInfo, error) {
	query := `
		SELECT
			kcu.table_schema,
			kcu.table_name,
			kcu.column_name,
			kcu.referenced_table_schema,
			kcu.referenced_table_name,
			kcu.referenced_column_name
		FROM information_schema.key_column_usage kcu
		WHERE kcu.referenced_table_name IS NOT NULL
		  AND kcu.table_schema = COALESCE(NULLIF(?, ''), DATABASE())
		  AND (? = '' OR kcu.table_name = ? OR kcu.referenced_table_name = ?)
		ORDER BY kcu.table_name, kcu.column_name;`

	rows, err := c.db.QueryContext(ctx, query, schema, table, table, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []RelationshipInfo
	for rows.Next() {
		var rel RelationshipInfo
		if err := rows.Scan(&rel.FromSchema, &rel.FromTable, &rel.FromColumn, &rel.ToSchema, &rel.ToTable, &rel.ToColumn); err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}
	return relationships, nil
}

func (c *MySQLClient) DescribeTable(ctx context.Context, schema, table string) ([]ColumnInfo, error) {
	query := `
		SELECT 
			column_name, 
			data_type, 
			is_nullable, 
			column_default,
			CASE WHEN column_key = 'PRI' THEN true ELSE false END as is_primary
		FROM information_schema.columns
		WHERE table_schema = COALESCE(NULLIF(?, ''), DATABASE()) AND table_name = ?
		ORDER BY ordinal_position;`

	rows, err := c.db.QueryContext(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var nullableStr string
		var defaultStr sql.NullString
		if err := rows.Scan(&col.Name, &col.Type, &nullableStr, &defaultStr, &col.PrimaryKey); err != nil {
			return nil, err
		}
		col.Nullable = strings.ToUpper(nullableStr) == "YES"
		if defaultStr.Valid {
			col.Default = &defaultStr.String
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (c *MySQLClient) ExecuteQuery(ctx context.Context, query string) ([]map[string]any, error) {
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (c *MySQLClient) ExecuteReadOnlyQuery(ctx context.Context, query string) ([]map[string]any, error) {
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		// Fallback si el motor o driver de MySQL/MariaDB no soporta transacciones de solo lectura
		return c.ExecuteQuery(ctx, query)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (c *MySQLClient) ExecuteWrite(ctx context.Context, query string) (int64, error) {
	res, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==========================================
// CLIENTE SQLITE
// ==========================================

type SQLiteClient struct {
	db *sql.DB
}

func (c *SQLiteClient) Close() error {
	return c.db.Close()
}

func (c *SQLiteClient) ListSchemas(ctx context.Context) ([]string, error) {
	return []string{"main"}, nil
}

func (c *SQLiteClient) ListTables(ctx context.Context, schema string) ([]string, error) {
	query := `
		SELECT name 
		FROM sqlite_master 
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		ORDER BY name;`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func (c *SQLiteClient) ListViews(ctx context.Context, schema string) ([]string, error) {
	query := `
		SELECT name
		FROM sqlite_master
		WHERE type = 'view'
		ORDER BY name;`

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []string
	for rows.Next() {
		var view string
		if err := rows.Scan(&view); err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (c *SQLiteClient) ListIndexes(ctx context.Context, schema, table string) ([]IndexInfo, error) {
	escapedTable := strings.ReplaceAll(table, "\"", "\"\"")
	query := fmt.Sprintf("PRAGMA index_list(\"%s\")", escapedTable)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indexes []IndexInfo
	for rows.Next() {
		var seq int
		var idx IndexInfo
		var unique int
		var origin string
		var partial int
		if err := rows.Scan(&seq, &idx.Name, &unique, &origin, &partial); err != nil {
			return nil, err
		}
		idx.Unique = unique == 1
		columns, err := c.indexColumns(ctx, idx.Name)
		if err != nil {
			return nil, err
		}
		idx.Columns = strings.Join(columns, ", ")
		indexes = append(indexes, idx)
	}
	return indexes, nil
}

func (c *SQLiteClient) FindColumns(ctx context.Context, schema, search string, limit int) ([]ColumnMatch, error) {
	tables, err := c.ListTables(ctx, schema)
	if err != nil {
		return nil, err
	}

	search = strings.ToLower(search)
	var matches []ColumnMatch
	for _, table := range tables {
		columns, err := c.DescribeTable(ctx, schema, table)
		if err != nil {
			return nil, err
		}
		for _, column := range columns {
			if search != "" &&
				!strings.Contains(strings.ToLower(column.Name), search) &&
				!strings.Contains(strings.ToLower(table), search) {
				continue
			}
			matches = append(matches, ColumnMatch{
				Schema:   "main",
				Table:    table,
				Column:   column.Name,
				DataType: column.Type,
			})
			if len(matches) >= limit {
				return matches, nil
			}
		}
	}
	return matches, nil
}

func (c *SQLiteClient) ListRelationships(ctx context.Context, schema, table string) ([]RelationshipInfo, error) {
	tables, err := c.ListTables(ctx, schema)
	if err != nil {
		return nil, err
	}

	var relationships []RelationshipInfo
	for _, tableName := range tables {
		if table != "" && tableName != table {
			if !hasSQLiteReference(ctx, c.db, tableName, table) {
				continue
			}
		}

		rels, err := c.foreignKeysForTable(ctx, tableName)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rels...)
	}

	return relationships, nil
}

func (c *SQLiteClient) foreignKeysForTable(ctx context.Context, tableName string) ([]RelationshipInfo, error) {
	escapedTable := strings.ReplaceAll(tableName, "\"", "\"\"")
	query := fmt.Sprintf("PRAGMA foreign_key_list(\"%s\")", escapedTable)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relationships []RelationshipInfo
	for rows.Next() {
		var id, seq int
		var toTable, fromColumn, toColumn, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &toTable, &fromColumn, &toColumn, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}
		relationships = append(relationships, RelationshipInfo{
			FromSchema: "main",
			FromTable:  tableName,
			FromColumn: fromColumn,
			ToSchema:   "main",
			ToTable:    toTable,
			ToColumn:   toColumn,
		})
	}
	return relationships, nil
}

func (c *SQLiteClient) indexColumns(ctx context.Context, indexName string) ([]string, error) {
	escapedIndex := strings.ReplaceAll(indexName, "\"", "\"\"")
	query := fmt.Sprintf("PRAGMA index_info(\"%s\")", escapedIndex)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var seqno, cid int
		var name string
		if err := rows.Scan(&seqno, &cid, &name); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, nil
}

func hasSQLiteReference(ctx context.Context, db *sql.DB, fromTable, targetTable string) bool {
	rels, err := (&SQLiteClient{db: db}).foreignKeysForTable(ctx, fromTable)
	if err != nil {
		return false
	}
	for _, rel := range rels {
		if rel.ToTable == targetTable || rel.FromTable == targetTable {
			return true
		}
	}
	return fromTable == targetTable
}

func (c *SQLiteClient) DescribeTable(ctx context.Context, schema, table string) ([]ColumnInfo, error) {
	escapedTable := strings.ReplaceAll(table, "\"", "\"\"")
	query := fmt.Sprintf("PRAGMA table_info(\"%s\")", escapedTable)

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var cid int
		var col ColumnInfo
		var notNull int
		var dfltVal sql.NullString
		var pk int

		if err := rows.Scan(&cid, &col.Name, &col.Type, &notNull, &dfltVal, &pk); err != nil {
			return nil, err
		}

		col.Nullable = notNull == 0
		col.PrimaryKey = pk > 0
		if dfltVal.Valid {
			col.Default = &dfltVal.String
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func (c *SQLiteClient) ExecuteQuery(ctx context.Context, query string) ([]map[string]any, error) {
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (c *SQLiteClient) ExecuteReadOnlyQuery(ctx context.Context, query string) ([]map[string]any, error) {
	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return c.ExecuteQuery(ctx, query)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (c *SQLiteClient) ExecuteWrite(ctx context.Context, query string) (int64, error) {
	res, err := c.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ==========================================
// HELPERS
// ==========================================

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0)
	for rows.Next() {
		columns := make([]any, len(cols))
		columnPointers := make([]any, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]any)
		for i, colName := range cols {
			val := columns[i]
			if b, ok := val.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		result = append(result, rowMap)
	}
	return result, nil
}

func parseIndexColumns(indexDef string) string {
	start := strings.Index(indexDef, "(")
	end := strings.LastIndex(indexDef, ")")
	if start == -1 || end == -1 || end <= start {
		return ""
	}
	return strings.TrimSpace(indexDef[start+1 : end])
}
