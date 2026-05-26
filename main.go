package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	// Configurar el log para escribir a stderr (crítico para que no ensucie stdout, usado por el protocolo MCP)
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Intentar cargar .env desde el CWD actual
	err := godotenv.Load()
	if err != nil {
		// Si falla, buscar .env al lado del ejecutable (útil para cuando el cliente MCP arranca el proceso en otro CWD)
		if exePath, exeErr := os.Executable(); exeErr == nil {
			exeDir := filepath.Dir(exePath)
			envPath := filepath.Join(exeDir, ".env")
			if loadErr := godotenv.Load(envPath); loadErr == nil {
				log.Printf("Loaded .env from executable directory: %s\n", envPath)
			} else {
				log.Println("Warning: No .env file found in CWD or next to executable. Using environment variables.")
			}
		}
	} else {
		log.Println("Loaded .env from current working directory.")
	}

	// Descubrir todas las bases de datos configuradas en el entorno
	dbConfigs, err := DiscoverDBs()
	if err != nil {
		log.Fatalf("Error discovering databases: %v\n", err)
	}

	if len(dbConfigs) == 0 {
		log.Println("Warning: No databases configured. Please set DB_NAME or DB_NAME_<SUFFIX> variables.")
	}

	// Conectarse a cada una de las bases de datos encontradas
	for name, cfg := range dbConfigs {
		log.Printf("Connecting to database '%s' (%s at %s:%s)...", name, cfg.Type, cfg.Host, cfg.Port)
		client, err := NewDBClient(cfg)
		if err != nil {
			log.Fatalf("\nFailed to connect to database '%s': %v\n", name, err)
		}
		dbClients[name] = client
		log.Println(" Connected successfully.")
	}

	// Garantizar que cerremos todas las conexiones al salir
	defer func() {
		for name, client := range dbClients {
			log.Printf("Closing connection pool for database '%s'...\n", name)
			client.Close()
		}
	}()

	// Crear el servidor MCP
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "mcp-octo-db",
			Version: "1.0.0",
		},
		nil,
	)

	// Registrar las herramientas en el servidor
	log.Println("Registering MCP tools...")

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

	mcp.AddTool(server, &mcp.Tool{
		Name:        "write_query",
		Description: "Execute an SQL query that modifies data or schema (INSERT, UPDATE, DELETE, CREATE, ALTER, etc.) on the specified database.",
	}, WriteQueryHandler)

	// Arrancar el transporte stdio para MCP
	log.Println("Starting mcp-octo-db server on stdio transport...")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server execution failed: %v\n", err)
	}
}
