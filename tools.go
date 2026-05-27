package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dbClients registra todos los pools de bases de datos activos
var dbClients = make(map[string]DBClient)

// getClient busca un cliente de base de datos en el registro por nombre (insensible a mayúsculas/minúsculas)
func getClient(dbName string) (DBClient, error) {
	name := strings.ToLower(dbName)
	if name == "" {
		name = "default"
	}
	client, ok := dbClients[name]
	if !ok {
		var available []string
		for k := range dbClients {
			available = append(available, k)
		}
		return nil, fmt.Errorf("database '%s' not found in config. Available databases: %s", dbName, strings.Join(available, ", "))
	}
	return client, nil
}

// textResult es un helper para formatear respuestas de texto legibles para el protocolo MCP
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: text,
			},
		},
	}
}

// ==========================================
// LOGGER & SECURITY HELPERS
// ==========================================

func logToolCall(toolName string, args any, duration time.Duration, err error, rowCount int, truncated bool) {
	// Si es JSON log
	if GlobalSettings.LogFormat == "json" {
		logObj := map[string]any{
			"level":       "info",
			"tool":        toolName,
			"duration_ms": duration.Milliseconds(),
			"success":     err == nil,
			"row_count":   rowCount,
			"truncated":   truncated,
		}
		if err != nil {
			logObj["error"] = err.Error()
			logObj["level"] = "error"
		}
		if GlobalSettings.AuditLog {
			logObj["audit"] = true
		}
		jsonData, _ := json.Marshal(logObj)
		log.Println(string(jsonData))
	} else {
		// Text log (default)
		status := "SUCCESS"
		if err != nil {
			status = fmt.Sprintf("FAILED: %v", err)
		}
		auditPrefix := ""
		if GlobalSettings.AuditLog {
			auditPrefix = "[AUDIT] "
		}
		log.Printf("%sTool %s executed in %v. Status: %s. Rows: %d (truncated: %t)\n",
			auditPrefix, toolName, duration, status, rowCount, truncated)
	}
}

var tableRefRegex = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|UPDATE|TABLE)\s+([a-zA-Z0-9_\."'\x60]+)`)

func extractTableNames(sql string) []string {
	matches := tableRefRegex.FindAllStringSubmatch(sql, -1)
	var tables []string
	for _, match := range matches {
		if len(match) > 1 {
			table := match[1]
			// Limpiar comillas, backticks y esquemas (ej. public.users -> users)
			table = strings.ReplaceAll(table, "\"", "")
			table = strings.ReplaceAll(table, "`", "")
			table = strings.ReplaceAll(table, "'", "")
			if idx := strings.Index(table, "."); idx != -1 {
				table = table[idx+1:]
			}
			table = strings.TrimSpace(table)
			if table != "" {
				tables = append(tables, table)
			}
		}
	}
	return tables
}

func validateQuerySafety(sql string) error {
	// Reemplazar caracteres especiales con espacios para tokenizar y verificar denylist
	replacer := strings.NewReplacer(",", " ", "(", " ", ")", " ", ";", " ", "\"", " ", "`", " ", "'", " ", ".", " ")
	clean := replacer.Replace(sql)
	words := strings.Fields(clean)

	for _, word := range words {
		// Denylist check (case-insensitive)
		for _, dt := range GlobalSettings.DeniedTables {
			if strings.EqualFold(word, dt) {
				return fmt.Errorf("table '%s' is denied by security policy", dt)
			}
		}
	}

	// Si hay allowlist, extraemos las tablas referenciadas formalmente
	if len(GlobalSettings.AllowedTables) > 0 {
		tables := extractTableNames(sql)
		for _, t := range tables {
			allowed := false
			for _, at := range GlobalSettings.AllowedTables {
				if strings.EqualFold(t, at) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("table '%s' is not in the allowlist", t)
			}
		}
	}

	return nil
}

func isSchemaAllowed(schema string, dbType string) bool {
	if len(GlobalSettings.AllowedSchemas) == 0 {
		return true
	}
	s := schema
	if s == "" {
		if dbType == "postgres" || dbType == "postgresql" {
			s = "public"
		} else {
			return true // No schema restriction or default schema
		}
	}
	for _, allowed := range GlobalSettings.AllowedSchemas {
		if strings.EqualFold(s, allowed) {
			return true
		}
	}
	return false
}

// ==========================================
// EXISTING TOOL HANDLERS (ENHANCED)
// ==========================================

// ListTablesArgs contiene los argumentos para listar tablas
type ListTablesArgs struct {
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema string `json:"schema,omitempty" jsonschema:"schema to list tables from (for Postgres defaults to 'public', for MySQL/MariaDB defaults to database name)"`
}

// ListTablesHandler maneja el listado de tablas
func ListTablesHandler(ctx context.Context, req *mcp.CallToolRequest, args ListTablesArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := "postgres"
	switch client.(type) {
	case *MySQLClient:
		dbType = "mysql"
	case *SQLiteClient:
		dbType = "sqlite"
	}

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall("list_tables", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	tables, err := client.ListTables(timeoutCtx, args.Schema)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("list_tables", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	// Filtrar según allowlist y denylist
	var filtered []string
	for _, t := range tables {
		if len(GlobalSettings.AllowedTables) > 0 {
			allowed := false
			for _, at := range GlobalSettings.AllowedTables {
				if strings.EqualFold(t, at) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}

		denied := false
		for _, dt := range GlobalSettings.DeniedTables {
			if strings.EqualFold(t, dt) {
				denied = true
				break
			}
		}
		if !denied {
			filtered = append(filtered, t)
		}
	}

	logToolCall("list_tables", args, duration, nil, len(filtered), false)

	if len(filtered) == 0 {
		return textResult("No tables found in this schema/database matching policy."), nil, nil
	}

	resText := fmt.Sprintf("Tables:\n- %s", strings.Join(filtered, "\n- "))
	return textResult(resText), nil, nil
}

// DescribeTableArgs contiene los argumentos para describir una tabla
type DescribeTableArgs struct {
	TableName string `json:"table_name" jsonschema:"name of the table to describe"`
	DBName    string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema    string `json:"schema,omitempty" jsonschema:"schema of the table (for Postgres defaults to 'public', for MySQL/MariaDB defaults to database name)"`
}

// DescribeTableHandler maneja la descripción del esquema de una tabla
func DescribeTableHandler(ctx context.Context, req *mcp.CallToolRequest, args DescribeTableArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := "postgres"
	switch client.(type) {
	case *MySQLClient:
		dbType = "mysql"
	case *SQLiteClient:
		dbType = "sqlite"
	}

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall("describe_table", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	// Validar tabla en allow/deny lists
	if len(GlobalSettings.AllowedTables) > 0 {
		allowed := false
		for _, at := range GlobalSettings.AllowedTables {
			if strings.EqualFold(args.TableName, at) {
				allowed = true
				break
			}
		}
		if !allowed {
			errSec := fmt.Errorf("table '%s' is not allowed by policy", args.TableName)
			logToolCall("describe_table", args, 0, errSec, 0, false)
			return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
		}
	}

	for _, dt := range GlobalSettings.DeniedTables {
		if strings.EqualFold(args.TableName, dt) {
			errSec := fmt.Errorf("table '%s' is denied by policy", args.TableName)
			logToolCall("describe_table", args, 0, errSec, 0, false)
			return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	columns, err := client.DescribeTable(timeoutCtx, args.Schema, args.TableName)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("describe_table", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	logToolCall("describe_table", args, duration, nil, len(columns), false)

	if len(columns) == 0 {
		return textResult(fmt.Sprintf("Table '%s' not found or has no columns.", args.TableName)), nil, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Table: %s\n", args.TableName))
	sb.WriteString("Columns:\n")
	sb.WriteString(fmt.Sprintf("%-25s %-20s %-10s %-8s %-20s\n", "Column", "Type", "Nullable", "Key", "Default"))
	sb.WriteString(strings.Repeat("-", 85) + "\n")
	for _, col := range columns {
		keyStr := ""
		if col.PrimaryKey {
			keyStr = "PK"
		}
		defStr := "NULL"
		if col.Default != nil {
			defStr = *col.Default
		}
		sb.WriteString(fmt.Sprintf("%-25s %-20s %-10t %-8s %-20s\n",
			col.Name, col.Type, col.Nullable, keyStr, defStr))
	}

	return textResult(sb.String()), nil, nil
}

// ReadQueryArgs contiene los argumentos para ejecutar consultas de lectura
type ReadQueryArgs struct {
	SQL    string `json:"sql" jsonschema:"the read-only SELECT SQL query to execute"`
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
}

// ReadQueryHandler ejecuta consultas SELECT de solo lectura
func ReadQueryHandler(ctx context.Context, req *mcp.CallToolRequest, args ReadQueryArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	// Validación básica de consulta de lectura
	cleanSQL := strings.TrimSpace(strings.ToUpper(args.SQL))
	if !strings.HasPrefix(cleanSQL, "SELECT") &&
		!strings.HasPrefix(cleanSQL, "WITH") &&
		!strings.HasPrefix(cleanSQL, "SHOW") &&
		!strings.HasPrefix(cleanSQL, "DESCRIBE") &&
		!strings.HasPrefix(cleanSQL, "EXPLAIN") {
		return textResult("Error: Only SELECT, WITH, SHOW, DESCRIBE, and EXPLAIN queries are allowed in read_query tool."), nil, nil
	}

	// Prevenir múltiples sentencias SQL
	semicolonIndex := strings.Index(cleanSQL, ";")
	if semicolonIndex != -1 && semicolonIndex < len(cleanSQL)-1 {
		after := strings.TrimSpace(cleanSQL[semicolonIndex+1:])
		if len(after) > 0 {
			return textResult("Error: Multiple SQL statements are not allowed in a single query."), nil, nil
		}
	}

	// Validar seguridad de la consulta
	if err := validateQuerySafety(args.SQL); err != nil {
		logToolCall("read_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	results, err := client.ExecuteReadOnlyQuery(timeoutCtx, args.SQL)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("read_query", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing query: %v", err)), nil, nil
	}

	// Limitar el número de filas devueltas
	truncated := false
	rowCount := len(results)
	if rowCount > GlobalSettings.MaxRows {
		results = results[:GlobalSettings.MaxRows]
		truncated = true
	}

	dbType := "postgres"
	switch client.(type) {
	case *MySQLClient:
		dbType = "mysql"
	case *SQLiteClient:
		dbType = "sqlite"
	}

	responseObj := map[string]any{
		"metadata": map[string]any{
			"row_count":     rowCount,
			"duration_ms":   duration.Milliseconds(),
			"truncated":     truncated,
			"database_type": dbType,
		},
		"data": results,
	}

	logToolCall("read_query", args, duration, nil, rowCount, truncated)

	jsonData, err := json.MarshalIndent(responseObj, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Error formatting results: %v", err)), nil, nil
	}

	return textResult(string(jsonData)), nil, nil
}

// WriteQueryArgs contiene los argumentos para consultas de modificación
type WriteQueryArgs struct {
	SQL    string `json:"sql" jsonschema:"the write SQL query to execute (INSERT, UPDATE, DELETE, CREATE, ALTER, etc.)"`
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
}

// WriteQueryHandler ejecuta consultas que modifican datos o esquema
func WriteQueryHandler(ctx context.Context, req *mcp.CallToolRequest, args WriteQueryArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()

	// Enforzar validación de modo de escritura
	if !GlobalSettings.EnableWrite {
		errWriteDisabled := fmt.Errorf("write operations are disabled by policy (MCP_ENABLE_WRITE=false)")
		logToolCall("write_query", args, 0, errWriteDisabled, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errWriteDisabled)), nil, nil
	}

	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	// Validar seguridad de la consulta
	if err := validateQuerySafety(args.SQL); err != nil {
		logToolCall("write_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	rowsAffected, err := client.ExecuteWrite(timeoutCtx, args.SQL)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("write_query", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing write query: %v", err)), nil, nil
	}

	logToolCall("write_query", args, duration, nil, int(rowsAffected), false)
	return textResult(fmt.Sprintf("Query executed successfully. Rows affected: %d", rowsAffected)), nil, nil
}

// ==========================================
// NEW COMMUNITY EDITION TOOL HANDLERS
// ==========================================

// ListSchemasArgs contiene los argumentos para listar esquemas
type ListSchemasArgs struct {
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
}

// ListSchemasHandler lista los esquemas o catálogos
func ListSchemasHandler(ctx context.Context, req *mcp.CallToolRequest, args ListSchemasArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	schemas, err := client.ListSchemas(timeoutCtx)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("list_schemas", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	// Filtrar según allowlist
	var filtered []string
	if len(GlobalSettings.AllowedSchemas) > 0 {
		for _, s := range schemas {
			for _, allowed := range GlobalSettings.AllowedSchemas {
				if strings.EqualFold(s, allowed) {
					filtered = append(filtered, s)
					break
				}
			}
		}
	} else {
		filtered = schemas
	}

	logToolCall("list_schemas", args, duration, nil, len(filtered), false)

	if len(filtered) == 0 {
		return textResult("No schemas found or all filtered by policy."), nil, nil
	}

	resText := fmt.Sprintf("Schemas:\n- %s", strings.Join(filtered, "\n- "))
	return textResult(resText), nil, nil
}

// SearchTablesArgs contiene los argumentos para buscar tablas
type SearchTablesArgs struct {
	Query  string `json:"query" jsonschema:"pattern to search for in table names (e.g. '%users%')"`
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema string `json:"schema,omitempty" jsonschema:"schema to search tables in (optional)"`
}

// SearchTablesHandler busca tablas que coincidan con un patrón
func SearchTablesHandler(ctx context.Context, req *mcp.CallToolRequest, args SearchTablesArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := "postgres"
	switch client.(type) {
	case *MySQLClient:
		dbType = "mysql"
	case *SQLiteClient:
		dbType = "sqlite"
	}

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall("search_tables", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	tables, err := client.ListTables(timeoutCtx, args.Schema)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("search_tables", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	var matched []string
	pattern := strings.ToLower(strings.ReplaceAll(args.Query, "%", ""))
	for _, t := range tables {
		// Validar si la tabla está permitida
		if len(GlobalSettings.AllowedTables) > 0 {
			allowed := false
			for _, at := range GlobalSettings.AllowedTables {
				if strings.EqualFold(t, at) {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		// Validar si la tabla está denegada
		denied := false
		for _, dt := range GlobalSettings.DeniedTables {
			if strings.EqualFold(t, dt) {
				denied = true
				break
			}
		}
		if denied {
			continue
		}

		if strings.Contains(strings.ToLower(t), pattern) {
			matched = append(matched, t)
		}
	}

	logToolCall("search_tables", args, duration, nil, len(matched), false)

	if len(matched) == 0 {
		return textResult("No tables found matching the query pattern."), nil, nil
	}

	resText := fmt.Sprintf("Tables matching '%s':\n- %s", args.Query, strings.Join(matched, "\n- "))
	return textResult(resText), nil, nil
}

// GetTableSampleArgs contiene los argumentos para obtener una muestra de filas
type GetTableSampleArgs struct {
	TableName string `json:"table_name" jsonschema:"name of the table to sample"`
	DBName    string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema    string `json:"schema,omitempty" jsonschema:"schema of the table (optional)"`
	Limit     int    `json:"limit,omitempty" jsonschema:"number of rows to retrieve (default 10, max 100)"`
}

// GetTableSampleHandler devuelve una muestra de filas de una tabla
func GetTableSampleHandler(ctx context.Context, req *mcp.CallToolRequest, args GetTableSampleArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := "postgres"
	switch client.(type) {
	case *MySQLClient:
		dbType = "mysql"
	case *SQLiteClient:
		dbType = "sqlite"
	}

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall("get_table_sample", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	// Validar tabla en allow/deny lists
	if len(GlobalSettings.AllowedTables) > 0 {
		allowed := false
		for _, at := range GlobalSettings.AllowedTables {
			if strings.EqualFold(args.TableName, at) {
				allowed = true
				break
			}
		}
		if !allowed {
			errSec := fmt.Errorf("table '%s' is not allowed by policy", args.TableName)
			logToolCall("get_table_sample", args, 0, errSec, 0, false)
			return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
		}
	}

	for _, dt := range GlobalSettings.DeniedTables {
		if strings.EqualFold(args.TableName, dt) {
			errSec := fmt.Errorf("table '%s' is denied by policy", args.TableName)
			logToolCall("get_table_sample", args, 0, errSec, 0, false)
			return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
		}
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	// Construir la consulta según el motor
	var sqlQuery string
	schemaPrefix := ""
	if args.Schema != "" {
		schemaPrefix = fmt.Sprintf("\"%s\".", strings.ReplaceAll(args.Schema, "\"", ""))
	}
	tableNameEscaped := fmt.Sprintf("\"%s\"", strings.ReplaceAll(args.TableName, "\"", ""))

	if dbType == "mysql" {
		if args.Schema != "" {
			schemaPrefix = fmt.Sprintf("`%s`.", strings.ReplaceAll(args.Schema, "`", ""))
		}
		tableNameEscaped = fmt.Sprintf("`%s`", strings.ReplaceAll(args.TableName, "`", ""))
	}

	sqlQuery = fmt.Sprintf("SELECT * FROM %s%s LIMIT %d", schemaPrefix, tableNameEscaped, limit)

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	results, err := client.ExecuteReadOnlyQuery(timeoutCtx, sqlQuery)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("get_table_sample", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing sample query: %v", err)), nil, nil
	}

	logToolCall("get_table_sample", args, duration, nil, len(results), false)

	if len(results) == 0 {
		return textResult("Table is empty."), nil, nil
	}

	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Error formatting results: %v", err)), nil, nil
	}

	return textResult(string(jsonData)), nil, nil
}

// ExplainQueryArgs contiene los argumentos para explicar una consulta
type ExplainQueryArgs struct {
	SQL    string `json:"sql" jsonschema:"the SELECT query to explain"`
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
}

// ExplainQueryHandler ejecuta un comando EXPLAIN sobre una consulta SELECT
func ExplainQueryHandler(ctx context.Context, req *mcp.CallToolRequest, args ExplainQueryArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	// Validar consulta
	cleanSQL := strings.TrimSpace(strings.ToUpper(args.SQL))
	if !strings.HasPrefix(cleanSQL, "SELECT") && !strings.HasPrefix(cleanSQL, "WITH") {
		return textResult("Error: Only SELECT and WITH queries can be explained."), nil, nil
	}

	// Validar seguridad de tablas en la consulta
	if err := validateQuerySafety(args.SQL); err != nil {
		logToolCall("explain_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	dbType := "postgres"
	switch client.(type) {
	case *MySQLClient:
		dbType = "mysql"
	case *SQLiteClient:
		dbType = "sqlite"
	}

	explainSQL := "EXPLAIN " + args.SQL
	if dbType == "sqlite" {
		explainSQL = "EXPLAIN QUERY PLAN " + args.SQL
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	results, err := client.ExecuteReadOnlyQuery(timeoutCtx, explainSQL)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall("explain_query", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing EXPLAIN: %v", err)), nil, nil
	}

	logToolCall("explain_query", args, duration, nil, len(results), false)

	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Error formatting results: %v", err)), nil, nil
	}

	return textResult(string(jsonData)), nil, nil
}
