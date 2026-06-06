package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type ServerState struct {
	dbClients    map[string]DBClient
	dbConnErrors map[string]error
}

func NewServerState() *ServerState {
	return &ServerState{
		dbClients:    make(map[string]DBClient),
		dbConnErrors: make(map[string]error),
	}
}

func (s *ServerState) Reset() {
	s.dbClients = make(map[string]DBClient)
	s.dbConnErrors = make(map[string]error)
}

func (s *ServerState) AddClient(name string, client DBClient) {
	s.dbClients[strings.ToLower(name)] = client
}

func (s *ServerState) AddConnectionError(name string, err error) {
	s.dbConnErrors[strings.ToLower(name)] = err
}

func (s *ServerState) AvailableDatabases() []string {
	available := make([]string, 0, len(s.dbClients))
	for name := range s.dbClients {
		available = append(available, name)
	}
	sort.Strings(available)
	return available
}

func (s *ServerState) OfflineDatabases() map[string]string {
	offline := make(map[string]string, len(s.dbConnErrors))
	for name, err := range s.dbConnErrors {
		offline[name] = err.Error()
	}
	return offline
}

var appState = NewServerState()
var requestCounter atomic.Uint64

// getClient busca un cliente de base de datos en el registro por nombre (insensible a mayúsculas/minúsculas)
func (s *ServerState) getClient(dbName string) (DBClient, error) {
	name := strings.ToLower(dbName)
	if name == "" {
		name = "default"
	}

	// 1. Intentar obtener el cliente activo
	if client, ok := s.dbClients[name]; ok {
		return client, nil
	}

	// 2. Si no está activo, verificar si falló al conectar en el arranque
	if connErr, failed := s.dbConnErrors[name]; failed {
		return nil, fmt.Errorf("database '%s' is offline. Connection failed on startup: %v", dbName, connErr)
	}

	// 3. De lo contrario, no existe en la configuración
	var available []string
	for k := range s.dbClients {
		available = append(available, k)
	}
	return nil, fmt.Errorf("database '%s' not found in config. Available databases: %s", dbName, strings.Join(available, ", "))
}

func getClient(dbName string) (DBClient, error) {
	return appState.getClient(dbName)
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

func jsonResult(payload any) *mcp.CallToolResult {
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return textResult(fmt.Sprintf("Error formatting results: %v", err))
	}
	return textResult(string(jsonData))
}

func nextRequestID() string {
	return fmt.Sprintf("req-%06d", requestCounter.Add(1))
}

// ==========================================
// LOGGER & SECURITY HELPERS
// ==========================================

func logToolCall(requestID, toolName string, args any, duration time.Duration, err error, rowCount int, truncated bool) {
	logFields := extractLogFields(args)

	// Si es JSON log
	if GlobalSettings.LogFormat == "json" {
		logObj := map[string]any{
			"request_id":  requestID,
			"level":       "info",
			"tool":        toolName,
			"duration_ms": duration.Milliseconds(),
			"success":     err == nil,
			"row_count":   rowCount,
			"truncated":   truncated,
		}
		for key, value := range logFields {
			logObj[key] = value
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
		contextSuffix := formatLogFields(logFields)
		log.Printf("%sTool %s request_id=%s executed in %v. Status: %s. Rows: %d (truncated: %t)%s\n",
			auditPrefix, toolName, requestID, duration, status, rowCount, truncated, contextSuffix)
	}
}

func extractLogFields(args any) map[string]any {
	fields := make(map[string]any)
	raw, err := json.Marshal(args)
	if err != nil {
		return fields
	}

	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return fields
	}

	for _, key := range []string{"db_name", "schema", "table_name", "query"} {
		if value, ok := values[key]; ok {
			fields[key] = value
		}
	}
	return fields
}

func formatLogFields(fields map[string]any) string {
	if len(fields) == 0 {
		return ""
	}

	orderedKeys := []string{"db_name", "schema", "table_name", "query"}
	parts := make([]string, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		value, ok := fields[key]
		if !ok {
			continue
		}
		if key == "query" {
			queryText, _ := value.(string)
			if queryText != "" {
				queryText = strings.TrimSpace(queryText)
				if len(queryText) > 80 {
					queryText = queryText[:80] + "..."
				}
				parts = append(parts, fmt.Sprintf("%s=%q", key, queryText))
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, value))
	}

	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, " ") + "]"
}

var tableRefRegex = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|INTO|UPDATE|TABLE)\s+([a-zA-Z0-9_\."'\x60]+)`)
var writeKeywordRegex = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|TRUNCATE|REPLACE|MERGE|UPSERT|GRANT|REVOKE|CALL|COPY|ATTACH|DETACH|VACUUM)\b`)

type tableRef struct {
	Schema string
	Table  string
}

type questionHints struct {
	IDValues       []string
	TimeframeTerms []string
	MetricTerms    []string
	TableTerms     []string
}

type TableProfile struct {
	Schema        string   `json:"schema,omitempty"`
	Table         string   `json:"table"`
	Role          string   `json:"role"`
	DateColumns   []string `json:"date_columns,omitempty"`
	MetricColumns []string `json:"metric_columns,omitempty"`
	IDColumns     []string `json:"id_columns,omitempty"`
	Score         int      `json:"score"`
}

func extractTableNames(sql string) []string {
	refs := extractTableRefs(sql)
	var tables []string
	for _, ref := range refs {
		tables = append(tables, ref.Table)
	}
	return tables
}

func extractTableRefs(sql string) []tableRef {
	matches := tableRefRegex.FindAllStringSubmatch(sql, -1)
	var refs []tableRef
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}

		raw := strings.TrimSpace(match[1])
		raw = strings.ReplaceAll(raw, "\"", "")
		raw = strings.ReplaceAll(raw, "`", "")
		raw = strings.ReplaceAll(raw, "'", "")
		if raw == "" {
			continue
		}

		parts := strings.Split(raw, ".")
		ref := tableRef{Table: strings.TrimSpace(parts[len(parts)-1])}
		if len(parts) > 1 {
			ref.Schema = strings.TrimSpace(parts[len(parts)-2])
		}

		if ref.Table != "" {
			refs = append(refs, ref)
		}
	}
	return refs
}

func stripSQLLiteralsAndComments(sql string) string {
	var b strings.Builder
	b.Grow(len(sql))

	inSingleQuote := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				b.WriteByte(ch)
			} else {
				b.WriteByte(' ')
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < len(sql) && sql[i+1] == '/' {
				inBlockComment = false
				b.WriteString("  ")
				i++
			} else {
				b.WriteByte(' ')
			}
			continue
		}

		if inSingleQuote {
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					b.WriteString("  ")
					i++
					continue
				}
				inSingleQuote = false
			}
			b.WriteByte(' ')
			continue
		}

		if ch == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			inLineComment = true
			b.WriteString("  ")
			i++
			continue
		}
		if ch == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			inBlockComment = true
			b.WriteString("  ")
			i++
			continue
		}
		if ch == '\'' {
			inSingleQuote = true
			b.WriteByte(' ')
			continue
		}

		b.WriteByte(ch)
	}

	return b.String()
}

func validateSingleStatement(sql string) error {
	sanitized := stripSQLLiteralsAndComments(sql)
	trimmed := strings.TrimSpace(sanitized)
	if trimmed == "" {
		return fmt.Errorf("query cannot be empty")
	}

	firstSemicolon := strings.Index(trimmed, ";")
	if firstSemicolon == -1 {
		return nil
	}

	if strings.TrimSpace(trimmed[firstSemicolon+1:]) != "" {
		return fmt.Errorf("multiple SQL statements are not allowed in a single query")
	}

	if strings.Contains(trimmed[:firstSemicolon], ";") {
		return fmt.Errorf("multiple SQL statements are not allowed in a single query")
	}

	return nil
}

func validateQuerySafety(sql string) error {
	// Reemplazar caracteres especiales con espacios para tokenizar y verificar denylist
	sanitized := stripSQLLiteralsAndComments(sql)
	replacer := strings.NewReplacer(",", " ", "(", " ", ")", " ", ";", " ", "\"", " ", "`", " ", "'", " ", ".", " ")
	clean := replacer.Replace(sanitized)
	words := strings.Fields(clean)

	for _, word := range words {
		// Denylist check (case-insensitive)
		for _, dt := range GlobalSettings.DeniedTables {
			if strings.EqualFold(word, dt) {
				return fmt.Errorf("table '%s' is denied by security policy", dt)
			}
		}
	}

	if len(GlobalSettings.AllowedSchemas) > 0 {
		for _, ref := range extractTableRefs(sql) {
			if ref.Schema == "" {
				continue
			}
			if !isSchemaAllowed(ref.Schema, "postgres") {
				return fmt.Errorf("schema '%s' is not allowed by policy", ref.Schema)
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

func validateReadOnlySQL(sql string) error {
	if err := validateSingleStatement(sql); err != nil {
		return err
	}

	sanitized := stripSQLLiteralsAndComments(sql)
	if writeKeywordRegex.MatchString(sanitized) {
		return fmt.Errorf("read_query only allows non-mutating SQL statements")
	}

	return validateQuerySafety(sql)
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

func isObjectAllowed(name string) bool {
	if len(GlobalSettings.AllowedTables) > 0 && !slices.ContainsFunc(GlobalSettings.AllowedTables, func(allowed string) bool {
		return strings.EqualFold(allowed, name)
	}) {
		return false
	}

	return !slices.ContainsFunc(GlobalSettings.DeniedTables, func(denied string) bool {
		return strings.EqualFold(denied, name)
	})
}

func detectDBType(client DBClient) string {
	switch client.(type) {
	case *MySQLClient:
		return "mysql"
	case *SQLiteClient:
		return "sqlite"
	default:
		return "postgres"
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func extractQuestionHints(question string) questionHints {
	lower := strings.ToLower(question)
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_')
	})

	stopWords := map[string]struct{}{
		"de": {}, "del": {}, "la": {}, "las": {}, "el": {}, "los": {}, "y": {}, "o": {}, "que": {}, "para": {},
		"por": {}, "con": {}, "necesito": {}, "quiero": {}, "reporte": {}, "report": {}, "dime": {}, "cuantos": {},
		"cuantas": {}, "cuanto": {}, "este": {}, "esta": {}, "estos": {}, "estas": {}, "mes": {}, "hoy": {},
		"service": {}, "tabla": {}, "items": {}, "item": {}, "services": {},
	}

	var ids []string
	var tableTerms []string
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if isNumericToken(token) {
			ids = append(ids, token)
			continue
		}
		if _, blocked := stopWords[token]; blocked {
			continue
		}
		if len(token) >= 3 {
			tableTerms = append(tableTerms, token)
		}
	}

	timeframes := []string{}
	for _, candidate := range []string{"este mes", "this month", "hoy", "today", "ayer", "yesterday", "semana", "week", "mes", "month"} {
		if strings.Contains(lower, candidate) {
			timeframes = append(timeframes, candidate)
		}
	}

	metrics := []string{}
	for _, candidate := range []string{"vend", "sold", "sale", "sales", "cantidad", "count", "total", "sum", "revenue", "ingreso"} {
		if strings.Contains(lower, candidate) {
			metrics = append(metrics, candidate)
		}
	}

	return questionHints{
		IDValues:       uniqueStrings(ids),
		TimeframeTerms: uniqueStrings(timeframes),
		MetricTerms:    uniqueStrings(metrics),
		TableTerms:     uniqueStrings(tableTerms),
	}
}

func isNumericToken(token string) bool {
	if token == "" {
		return false
	}
	for _, r := range token {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func scoreColumnMatch(match ColumnMatch, hints questionHints) int {
	score := 0
	table := strings.ToLower(match.Table)
	column := strings.ToLower(match.Column)
	for _, term := range hints.TableTerms {
		if strings.Contains(table, term) {
			score += 4
		}
		if strings.Contains(column, term) {
			score += 3
		}
	}
	for _, metric := range hints.MetricTerms {
		if strings.Contains(column, metric) || strings.Contains(table, metric) {
			score += 5
		}
	}
	if strings.HasSuffix(column, "_id") || column == "id" {
		score += 1
	}
	if strings.Contains(column, "date") || strings.Contains(column, "created") || strings.Contains(column, "sold") {
		score += 2
	}
	return score
}

func buildTableProfile(match ColumnMatch, columns []ColumnInfo, relationships []RelationshipInfo, hints questionHints) TableProfile {
	profile := TableProfile{
		Schema: match.Schema,
		Table:  match.Table,
		Role:   "catalog",
	}

	fkCount := 0
	for _, rel := range relationships {
		if strings.EqualFold(rel.FromTable, match.Table) {
			fkCount++
		}
	}

	for _, column := range columns {
		name := strings.ToLower(column.Name)
		switch {
		case name == "id" || strings.HasSuffix(name, "_id"):
			profile.IDColumns = append(profile.IDColumns, column.Name)
		}

		if strings.Contains(name, "date") || strings.Contains(name, "created") || strings.Contains(name, "sold") || strings.Contains(name, "updated") {
			profile.DateColumns = append(profile.DateColumns, column.Name)
			profile.Score += 2
		}

		if strings.Contains(name, "quantity") || strings.Contains(name, "qty") || strings.Contains(name, "total") || strings.Contains(name, "amount") || strings.Contains(name, "price") || strings.Contains(name, "sold") {
			profile.MetricColumns = append(profile.MetricColumns, column.Name)
			profile.Score += 4
		}
	}

	tableName := strings.ToLower(match.Table)
	if len(profile.MetricColumns) > 0 && len(profile.DateColumns) > 0 {
		profile.Role = "transactional"
		profile.Score += 6
	}
	if fkCount >= 2 && len(profile.MetricColumns) == 0 {
		profile.Role = "bridge"
		profile.Score += 4
	}
	if strings.Contains(tableName, "order") || strings.Contains(tableName, "sale") || strings.Contains(tableName, "invoice") || strings.Contains(tableName, "payment") {
		profile.Role = "transactional"
		profile.Score += 5
	}
	if strings.Contains(tableName, "item") && fkCount >= 1 {
		profile.Role = "bridge"
		profile.Score += 3
	}

	for _, term := range hints.TableTerms {
		if strings.Contains(tableName, term) {
			profile.Score += 3
		}
	}
	for _, metric := range hints.MetricTerms {
		if strings.Contains(tableName, metric) {
			profile.Score += 3
		}
	}

	if profile.Role == "catalog" && len(profile.DateColumns) == 0 && len(profile.MetricColumns) == 0 {
		profile.Score += 1
	}

	return profile
}

func sortProfiles(profiles []TableProfile) {
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].Score == profiles[j].Score {
			return profiles[i].Table < profiles[j].Table
		}
		return profiles[i].Score > profiles[j].Score
	})
}

func summarizeRelationships(relationships []RelationshipInfo, tables []string) []RelationshipInfo {
	if len(relationships) == 0 {
		return nil
	}
	tableSet := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		tableSet[strings.ToLower(table)] = struct{}{}
	}

	var filtered []RelationshipInfo
	for _, rel := range relationships {
		_, fromMatched := tableSet[strings.ToLower(rel.FromTable)]
		_, toMatched := tableSet[strings.ToLower(rel.ToTable)]
		if len(tableSet) == 0 || fromMatched || toMatched {
			filtered = append(filtered, rel)
		}
	}
	if len(filtered) == 0 {
		return relationships
	}
	return filtered
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
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "list_tables", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	tables, err := client.ListTables(timeoutCtx, args.Schema)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "list_tables", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	// Filtrar según allowlist y denylist
	var filtered []string
	for _, t := range tables {
		if isObjectAllowed(t) {
			filtered = append(filtered, t)
		}
	}

	logToolCall(requestID, "list_tables", args, duration, nil, len(filtered), false)

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
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "describe_table", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	// Validar tabla en allow/deny lists
	if !isObjectAllowed(args.TableName) {
		errSec := fmt.Errorf("table '%s' is blocked by policy", args.TableName)
		logToolCall(requestID, "describe_table", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	columns, err := client.DescribeTable(timeoutCtx, args.Schema, args.TableName)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "describe_table", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	logToolCall(requestID, "describe_table", args, duration, nil, len(columns), false)

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
	requestID := nextRequestID()
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

	// Validar seguridad de la consulta
	if err := validateReadOnlySQL(args.SQL); err != nil {
		logToolCall(requestID, "read_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	results, err := client.ExecuteReadOnlyQuery(timeoutCtx, args.SQL)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "read_query", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing query: %v", err)), nil, nil
	}

	// Limitar el número de filas devueltas
	truncated := false
	rowCount := len(results)
	if rowCount > GlobalSettings.MaxRows {
		results = results[:GlobalSettings.MaxRows]
		truncated = true
	}

	dbType := detectDBType(client)

	responseObj := map[string]any{
		"metadata": map[string]any{
			"row_count":     rowCount,
			"duration_ms":   duration.Milliseconds(),
			"truncated":     truncated,
			"database_type": dbType,
		},
		"data": results,
	}

	logToolCall(requestID, "read_query", args, duration, nil, rowCount, truncated)

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
	requestID := nextRequestID()

	// Enforzar validación de modo de escritura
	if !GlobalSettings.EnableWrite {
		errWriteDisabled := fmt.Errorf("write operations are disabled by policy (MCP_ENABLE_WRITE=false)")
		logToolCall(requestID, "write_query", args, 0, errWriteDisabled, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errWriteDisabled)), nil, nil
	}

	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	if err := validateSingleStatement(args.SQL); err != nil {
		logToolCall(requestID, "write_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	// Validar seguridad de la consulta
	if err := validateQuerySafety(args.SQL); err != nil {
		logToolCall(requestID, "write_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	rowsAffected, err := client.ExecuteWrite(timeoutCtx, args.SQL)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "write_query", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing write query: %v", err)), nil, nil
	}

	logToolCall(requestID, "write_query", args, duration, nil, int(rowsAffected), false)
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
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	schemas, err := client.ListSchemas(timeoutCtx)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "list_schemas", args, duration, err, 0, false)
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

	logToolCall(requestID, "list_schemas", args, duration, nil, len(filtered), false)

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
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "search_tables", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	tables, err := client.ListTables(timeoutCtx, args.Schema)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "search_tables", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	var matched []string
	pattern := strings.ToLower(strings.ReplaceAll(args.Query, "%", ""))
	for _, t := range tables {
		if !isObjectAllowed(t) {
			continue
		}

		if strings.Contains(strings.ToLower(t), pattern) {
			matched = append(matched, t)
		}
	}

	logToolCall(requestID, "search_tables", args, duration, nil, len(matched), false)

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
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)

	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "get_table_sample", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	// Validar tabla en allow/deny lists
	if !isObjectAllowed(args.TableName) {
		errSec := fmt.Errorf("table '%s' is blocked by policy", args.TableName)
		logToolCall(requestID, "get_table_sample", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
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
		logToolCall(requestID, "get_table_sample", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing sample query: %v", err)), nil, nil
	}

	logToolCall(requestID, "get_table_sample", args, duration, nil, len(results), false)

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
	requestID := nextRequestID()
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
	if err := validateReadOnlySQL(args.SQL); err != nil {
		logToolCall(requestID, "explain_query", args, 0, err, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)

	explainSQL := "EXPLAIN " + args.SQL
	if dbType == "sqlite" {
		explainSQL = "EXPLAIN QUERY PLAN " + args.SQL
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	results, err := client.ExecuteReadOnlyQuery(timeoutCtx, explainSQL)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "explain_query", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error executing EXPLAIN: %v", err)), nil, nil
	}

	logToolCall(requestID, "explain_query", args, duration, nil, len(results), false)

	return jsonResult(results), nil, nil
}

type ServerInfoArgs struct{}

func ServerInfoHandler(ctx context.Context, req *mcp.CallToolRequest, args ServerInfoArgs) (*mcp.CallToolResult, any, error) {
	_ = ctx
	_ = req
	_ = args

	payload := map[string]any{
		"name":     "octo-db",
		"version":  version,
		"settings": EffectiveConfig(nil)["settings"],
		"capabilities": map[string]any{
			"enabled_tools":       enabledToolNames(),
			"available_databases": appState.AvailableDatabases(),
			"offline_databases":   appState.OfflineDatabases(),
		},
	}

	return jsonResult(payload), nil, nil
}

type ListViewsArgs struct {
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema string `json:"schema,omitempty" jsonschema:"schema to list views from"`
}

func ListViewsHandler(ctx context.Context, req *mcp.CallToolRequest, args ListViewsArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)
	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "list_views", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	views, err := client.ListViews(timeoutCtx, args.Schema)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "list_views", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	var filtered []string
	for _, view := range views {
		if isObjectAllowed(view) {
			filtered = append(filtered, view)
		}
	}

	logToolCall(requestID, "list_views", args, duration, nil, len(filtered), false)

	return jsonResult(map[string]any{
		"db_name": args.DBName,
		"schema":  args.Schema,
		"views":   filtered,
	}), nil, nil
}

type ListIndexesArgs struct {
	TableName string `json:"table_name" jsonschema:"name of the table to inspect"`
	DBName    string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema    string `json:"schema,omitempty" jsonschema:"schema of the table"`
}

func ListIndexesHandler(ctx context.Context, req *mcp.CallToolRequest, args ListIndexesArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)
	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "list_indexes", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	if !isObjectAllowed(args.TableName) {
		errSec := fmt.Errorf("table '%s' is blocked by policy", args.TableName)
		logToolCall(requestID, "list_indexes", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	indexes, err := client.ListIndexes(timeoutCtx, args.Schema, args.TableName)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "list_indexes", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	logToolCall(requestID, "list_indexes", args, duration, nil, len(indexes), false)

	return jsonResult(map[string]any{
		"db_name":    args.DBName,
		"schema":     args.Schema,
		"table_name": args.TableName,
		"indexes":    indexes,
	}), nil, nil
}

type FindColumnsArgs struct {
	Query  string `json:"query" jsonschema:"column or concept to search for, such as service, sold, date, total, created"`
	DBName string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema string `json:"schema,omitempty" jsonschema:"schema to search in"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum number of matches to return (default 20, max 100)"`
}

func FindColumnsHandler(ctx context.Context, req *mcp.CallToolRequest, args FindColumnsArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)
	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "find_columns", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	limit := args.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	matches, err := client.FindColumns(timeoutCtx, args.Schema, strings.TrimSpace(args.Query), limit)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "find_columns", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	var filtered []ColumnMatch
	for _, match := range matches {
		if isObjectAllowed(match.Table) {
			filtered = append(filtered, match)
		}
	}

	logToolCall(requestID, "find_columns", args, duration, nil, len(filtered), false)
	return jsonResult(map[string]any{
		"db_name": args.DBName,
		"schema":  args.Schema,
		"query":   args.Query,
		"matches": filtered,
	}), nil, nil
}

type ListRelationshipsArgs struct {
	DBName    string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema    string `json:"schema,omitempty" jsonschema:"schema to inspect"`
	TableName string `json:"table_name,omitempty" jsonschema:"optional table name to focus the relationship list"`
}

func ListRelationshipsHandler(ctx context.Context, req *mcp.CallToolRequest, args ListRelationshipsArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)
	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "list_relationships", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}
	if args.TableName != "" && !isObjectAllowed(args.TableName) {
		errSec := fmt.Errorf("table '%s' is blocked by policy", args.TableName)
		logToolCall(requestID, "list_relationships", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	relationships, err := client.ListRelationships(timeoutCtx, args.Schema, args.TableName)
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "list_relationships", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	var filtered []RelationshipInfo
	for _, rel := range relationships {
		if isObjectAllowed(rel.FromTable) && isObjectAllowed(rel.ToTable) {
			filtered = append(filtered, rel)
		}
	}

	logToolCall(requestID, "list_relationships", args, duration, nil, len(filtered), false)
	return jsonResult(map[string]any{
		"db_name":       args.DBName,
		"schema":        args.Schema,
		"table_name":    args.TableName,
		"relationships": filtered,
	}), nil, nil
}

type SuggestQueryPlanArgs struct {
	Question string `json:"question" jsonschema:"natural-language question from a non-technical user, such as how many items sold for service 133 this month"`
	DBName   string `json:"db_name,omitempty" jsonschema:"name of the database to query (defaults to 'default')"`
	Schema   string `json:"schema,omitempty" jsonschema:"schema to inspect"`
}

func SuggestQueryPlanHandler(ctx context.Context, req *mcp.CallToolRequest, args SuggestQueryPlanArgs) (*mcp.CallToolResult, any, error) {
	startTime := time.Now()
	requestID := nextRequestID()
	client, err := getClient(args.DBName)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err)), nil, nil
	}

	dbType := detectDBType(client)
	if !isSchemaAllowed(args.Schema, dbType) {
		errSec := fmt.Errorf("schema '%s' is not allowed by policy", args.Schema)
		logToolCall(requestID, "suggest_query_plan", args, 0, errSec, 0, false)
		return textResult(fmt.Sprintf("Security Error: %v", errSec)), nil, nil
	}

	hints := extractQuestionHints(args.Question)
	searchTerms := uniqueStrings(append(append([]string{}, hints.TableTerms...), hints.MetricTerms...))
	if len(searchTerms) == 0 {
		searchTerms = []string{strings.TrimSpace(args.Question)}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(GlobalSettings.QueryTimeoutSeconds)*time.Second)
	defer cancel()

	columnMap := make(map[string]ColumnMatch)
	for _, term := range searchTerms {
		if term == "" {
			continue
		}
		matches, err := client.FindColumns(timeoutCtx, args.Schema, term, 25)
		if err != nil {
			logToolCall(requestID, "suggest_query_plan", args, time.Since(startTime), err, 0, false)
			return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
		}
		for _, match := range matches {
			if !isObjectAllowed(match.Table) {
				continue
			}
			key := strings.ToLower(match.Schema + "." + match.Table + "." + match.Column)
			columnMap[key] = match
		}
	}

	matches := make([]ColumnMatch, 0, len(columnMap))
	for _, match := range columnMap {
		matches = append(matches, match)
	}

	sort.Slice(matches, func(i, j int) bool {
		left := scoreColumnMatch(matches[i], hints)
		right := scoreColumnMatch(matches[j], hints)
		if left == right {
			if matches[i].Table == matches[j].Table {
				return matches[i].Column < matches[j].Column
			}
			return matches[i].Table < matches[j].Table
		}
		return left > right
	})

	if len(matches) > 12 {
		matches = matches[:12]
	}

	tableSet := make(map[string]struct{})
	var candidateTables []string
	for _, match := range matches {
		if _, ok := tableSet[match.Table]; ok {
			continue
		}
		tableSet[match.Table] = struct{}{}
		candidateTables = append(candidateTables, match.Table)
	}

	relationships, err := client.ListRelationships(timeoutCtx, args.Schema, "")
	duration := time.Since(startTime)
	if err != nil {
		logToolCall(requestID, "suggest_query_plan", args, duration, err, 0, false)
		return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
	}

	filteredRelationships := summarizeRelationships(relationships, candidateTables)
	if len(filteredRelationships) > 10 {
		filteredRelationships = filteredRelationships[:10]
	}

	profilesByTable := make(map[string]TableProfile, len(candidateTables))
	for _, match := range matches {
		if _, ok := profilesByTable[match.Table]; ok {
			continue
		}
		columns, err := client.DescribeTable(timeoutCtx, args.Schema, match.Table)
		if err != nil {
			logToolCall(requestID, "suggest_query_plan", args, time.Since(startTime), err, 0, false)
			return textResult(fmt.Sprintf("Database error: %v", err)), nil, nil
		}
		profilesByTable[match.Table] = buildTableProfile(match, columns, filteredRelationships, hints)
	}

	profiles := make([]TableProfile, 0, len(profilesByTable))
	for _, profile := range profilesByTable {
		profiles = append(profiles, profile)
	}
	sortProfiles(profiles)

	candidateTables = candidateTables[:0]
	for _, profile := range profiles {
		candidateTables = append(candidateTables, profile.Table)
	}

	var suggestedFilters []map[string]any
	for _, id := range hints.IDValues {
		suggestedFilters = append(suggestedFilters, map[string]any{
			"type":  "identifier",
			"value": id,
			"hint":  "Look for columns ending in _id or primary key columns in the candidate tables.",
		})
	}
	for _, timeframe := range hints.TimeframeTerms {
		suggestedFilters = append(suggestedFilters, map[string]any{
			"type":  "timeframe",
			"value": timeframe,
			"hint":  "Look for date or timestamp columns such as created_at, sold_at, order_date, or date.",
		})
	}

	nextSteps := []string{
		"Verify which candidate table actually stores the business event being asked about.",
		"Choose a metric column or aggregation, usually COUNT(*), SUM(quantity), or SUM(total).",
	}
	if len(filteredRelationships) > 0 {
		nextSteps = append(nextSteps, "Use the listed foreign-key relationships to build the join path between service-like and sale-like tables.")
	}
	if len(hints.IDValues) > 0 {
		nextSteps = append(nextSteps, "Apply the numeric identifier as a filter only after confirming whether it belongs to the main table or a joined table.")
	}
	if len(hints.TimeframeTerms) > 0 {
		nextSteps = append(nextSteps, "Apply the timeframe on the transaction date column, not on a catalog table unless that is the intended business meaning.")
	}

	logToolCall(requestID, "suggest_query_plan", args, duration, nil, len(matches), false)
	return jsonResult(map[string]any{
		"db_name":                  args.DBName,
		"schema":                   args.Schema,
		"question":                 args.Question,
		"interpreted_hints":        hints,
		"candidate_tables":         candidateTables,
		"candidate_table_profiles": profiles,
		"candidate_columns":        matches,
		"candidate_relations":      filteredRelationships,
		"suggested_filters":        suggestedFilters,
		"recommended_next_step":    nextSteps,
	}), nil, nil
}
