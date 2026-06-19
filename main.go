package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "1.4.6"

var globalDBConfigs map[string]DBConfig

func main() {
	// Configurar el log para escribir a stderr (crítico para que no ensucie stdout, usado por el protocolo MCP)
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Definir banderas
	envPath := flag.String("env", "", "Path to custom .env file")
	configPath := flag.String("config", "", "Path to YAML config file")
	showVersion := flag.Bool("version", false, "Print version and exit")
	listTools := flag.Bool("list-tools", false, "Print enabled MCP tools and exit")
	printEffectiveConfig := flag.Bool("print-effective-config", false, "Print the effective configuration with masked secrets and exit")

	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	// Obtener argumentos posicionales
	args := flag.Args()

	if len(args) > 0 && args[0] == "setup" {
		runSetup()
		return
	}

	isDoctor := false
	if len(args) > 0 && args[0] == "doctor" {
		isDoctor = true
	}

	// Cargar configuración con precedencia
	dbConfigs, err := LoadConfig(*envPath, *configPath)
	if err != nil {
		if isDoctor {
			fmt.Printf("FAIL: Configuration loading failed: %v\n", err)
			os.Exit(1)
		}
		log.Fatalf("Error loading configuration: %v\n", err)
	}

	globalDBConfigs = dbConfigs

	if *printEffectiveConfig {
		output, err := json.MarshalIndent(EffectiveConfig(dbConfigs), "", "  ")
		if err != nil {
			log.Fatalf("Error formatting effective configuration: %v\n", err)
		}
		fmt.Println(string(output))
		return
	}

	if *listTools {
		output, err := json.MarshalIndent(enabledToolNames(), "", "  ")
		if err != nil {
			log.Fatalf("Error formatting tool list: %v\n", err)
		}
		fmt.Println(string(output))
		return
	}

	if isDoctor {
		runDoctor(dbConfigs)
		return
	}

	if len(dbConfigs) == 0 {
		log.Println("Warning: No databases configured. Please configure at least one database in config.yaml or environment variables.")
	}

	appState.Reset()

	// Conectarse a cada una de las bases de datos encontradas
	for name, cfg := range dbConfigs {
		log.Printf("Connecting to database '%s' (%s at %s:%s)...", name, cfg.Type, cfg.Host, cfg.Port)
		client, err := NewDBClient(cfg)
		if err != nil {
			appState.AddConnectionError(name, err)
			log.Printf(" WARNING: Failed to connect to database '%s': %v. The server will continue to start, but queries to this database will fail.\n", name, err)
			continue
		}
		appState.AddClient(name, client)
		log.Println(" Connected successfully.")
	}

	// Garantizar que cerremos todas las conexiones al salir
	defer func() {
		for name, client := range appState.dbClients {
			log.Printf("Closing connection pool for database '%s'...\n", name)
			client.Close()
		}
	}()

	// Crear el servidor MCP
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "octo-db",
			Version: version,
		},
		nil,
	)

	// Registrar las herramientas en el servidor
	log.Println("Registering MCP tools...")
	registerTools(server)

	// Arrancar el transporte stdio para MCP
	log.Println("Starting octo-db server on stdio transport...")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server execution failed: %v\n", err)
	}
}

func registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "server_info",
		Description: "Return server metadata, active policies, available databases, and enabled MCP tools.",
	}, ServerInfoHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tables",
		Description: "List all tables in the specified database and schema.",
	}, ListTablesHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "describe_table",
		Description: "Describe the structure of a specific table, including columns, types, nullability, primary keys, and default values.",
	}, DescribeTableHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "read_query",
		Description: "Execute a read-only SQL query (SELECT, SHOW, DESCRIBE, EXPLAIN, WITH) on the specified database and return the results as JSON.",
	}, ReadQueryHandler)

	if GlobalSettings.EnableWrite {
		log.Println("Enabling write_query tool (write mode active)")
		mcp.AddTool(server, &mcp.Tool{
			Name:        "write_query",
			Description: "Execute an SQL query that modifies data or schema (INSERT, UPDATE, DELETE, CREATE, ALTER, etc.) on the specified database.",
		}, WriteQueryHandler)
	} else {
		log.Println("write_query tool is disabled (read-only mode active by default)")
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_schemas",
		Description: "List all schemas/databases in the specified database.",
	}, ListSchemasHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search_tables",
		Description: "Search for tables matching a query pattern (e.g. '%users%') in the specified database.",
	}, SearchTablesHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_columns",
		Description: "Search for likely columns by business term, such as service, quantity, total, sold, created, or date.",
	}, FindColumnsHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_views",
		Description: "List all views in the specified database and schema.",
	}, ListViewsHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_table_sample",
		Description: "Get a sample of rows from a table (default 10, max 100 rows).",
	}, GetTableSampleHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_indexes",
		Description: "List indexes defined on a table, including uniqueness and indexed columns.",
	}, ListIndexesHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_relationships",
		Description: "List foreign-key relationships between tables, optionally focused on a single table.",
	}, ListRelationshipsHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "explain_query",
		Description: "Explain the execution plan of a SELECT query in the specified database.",
	}, ExplainQueryHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "suggest_query_plan",
		Description: "Turn a non-technical reporting question into candidate tables, columns, joins, and filters without executing SQL.",
	}, SuggestQueryPlanHandler)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "doctor",
		Description: "Run configuration validation and test connections to all configured databases (equivalent to the CLI 'doctor' command).",
	}, DoctorHandler)
}

func enabledToolNames() []string {
	tools := []string{
		"server_info",
		"list_tables",
		"describe_table",
		"read_query",
		"list_schemas",
		"search_tables",
		"find_columns",
		"list_views",
		"get_table_sample",
		"list_indexes",
		"list_relationships",
		"explain_query",
		"suggest_query_plan",
		"doctor",
	}
	if GlobalSettings.EnableWrite {
		tools = append(tools, "write_query")
	}
	return tools
}

func runDoctor(dbConfigs map[string]DBConfig) {
	fmt.Printf("octo-db Doctor v%s - Configuration & Connection Diagnostics\n", version)
	fmt.Println("==========================================================")
	fmt.Printf("Global Settings:\n")
	fmt.Printf("  Enable Write:          %t\n", GlobalSettings.EnableWrite)
	fmt.Printf("  Max Rows:              %d\n", GlobalSettings.MaxRows)
	fmt.Printf("  Query Timeout:         %ds\n", GlobalSettings.QueryTimeoutSeconds)
	fmt.Printf("  Max Open Conns:        %d\n", GlobalSettings.MaxOpenConns)
	fmt.Printf("  Max Idle Conns:        %d\n", GlobalSettings.MaxIdleConns)
	fmt.Printf("  Conn Max Lifetime:     %ds\n", GlobalSettings.ConnMaxLifetimeSecs)
	fmt.Printf("  Conn Max Idle Time:    %ds\n", GlobalSettings.ConnMaxIdleTimeSecs)
	fmt.Printf("  Allowed Schemas:       %v\n", GlobalSettings.AllowedSchemas)
	fmt.Printf("  Allowed Tables:        %v\n", GlobalSettings.AllowedTables)
	fmt.Printf("  Denied Tables:         %v\n", GlobalSettings.DeniedTables)
	fmt.Printf("  Log Level:             %s\n", GlobalSettings.LogLevel)
	fmt.Printf("  Log Format:            %s\n", GlobalSettings.LogFormat)
	fmt.Printf("  Audit Log:             %t\n", GlobalSettings.AuditLog)
	fmt.Printf("  Enabled Tools:         %v\n", enabledToolNames())
	fmt.Println("----------------------------------------------------------")

	if len(dbConfigs) == 0 {
		fmt.Println("FAIL: No databases configured. Please check your config.yaml or .env file.")
		os.Exit(1)
	}

	fmt.Println("Configuration validation: OK")
	fmt.Println("----------------------------------------------------------")

	allPassed := true
	for name, cfg := range dbConfigs {
		// Ocultar contraseña en el reporte
		maskedPassword := "****"
		if cfg.Password == "" {
			maskedPassword = "(empty)"
		}
		fmt.Printf("Database '%s':\n", name)
		fmt.Printf("  Type:     %s\n", cfg.Type)
		fmt.Printf("  Host:     %s\n", cfg.Host)
		fmt.Printf("  Port:     %s\n", cfg.Port)
		fmt.Printf("  User:     %s\n", cfg.User)
		fmt.Printf("  Name:     %s\n", cfg.Name)
		fmt.Printf("  Password: %s\n", maskedPassword)
		fmt.Printf("  SSLMode:  %s\n", cfg.SSLMode)

		// Test Connection
		fmt.Printf("  Testing connection... ")
		client, err := NewDBClient(cfg)
		if err != nil {
			fmt.Printf("FAIL\n  Reason: %v\n", err)
			allPassed = false
		} else {
			fmt.Println("OK")
			client.Close()
		}
		fmt.Println("----------------------------------------------------------")
	}

	if allPassed {
		fmt.Println("SUCCESS: All databases connected successfully!")
		os.Exit(0)
	} else {
		fmt.Println("FAIL: One or more database connections failed.")
		os.Exit(1)
	}
}
