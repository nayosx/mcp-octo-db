package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
// ESTRUCTURAS DE ARGUMENTOS Y MANEJADORES
// ==========================================

// ListTablesArgs contiene los argumentos para listar tablas
type ListTablesArgs struct {
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema string `json:"schema,omitempty" jsonschema:"schema to list tables from (for Postgres defaults to 'public', for MySQL/MariaDB defaults to database name)"`
}

// ListTablesHandler maneja el listado de tablas
func ListTablesHandler(ctx context.Context, req *mcp.CallToolRequest, args ListTablesArgs) (*mcp.CallToolResult, any, error) {
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	tables, err := client.ListTables(ctx, args.Schema)
	if err != nil {
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	if len(tables) == 0 {
		return textResult("No tables found in this schema/database."), nil, nil
	}

	resText := fmt.Sprintf("Tables:\n- %s", strings.Join(tables, "\n- "))
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
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	columns, err := client.DescribeTable(ctx, args.Schema, args.TableName)
	if err != nil {
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

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

	results, err := client.ExecuteReadOnlyQuery(ctx, args.SQL)
	if err != nil {
		return textResult(fmt.Sprintf("Database error executing query: %v", err)), nil, nil
	}

	if len(results) == 0 {
		return textResult("Query executed successfully. 0 rows returned."), nil, nil
	}

	jsonData, err := json.MarshalIndent(results, "", "  ")
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
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	rowsAffected, err := client.ExecuteWrite(ctx, args.SQL)
	if err != nil {
		return textResult(fmt.Sprintf("Database error executing write query: %v", err)), nil, nil
	}

	return textResult(fmt.Sprintf("Query executed successfully. Rows affected: %d", rowsAffected)), nil, nil
}
