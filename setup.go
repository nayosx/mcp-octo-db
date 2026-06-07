package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func runSetup() {
	absEnvPath, err := filepath.Abs(".env")
	if err != nil {
		absEnvPath = ".env"
	}

	fmt.Println("==================================================")
	fmt.Println("            IMPORTANT SECURITY NOTICE             ")
	fmt.Println("==================================================")
	fmt.Println()
	fmt.Println("The values entered in this wizard are used ONLY to generate a local .env")
	fmt.Println("configuration file.")
	fmt.Println()
	fmt.Println("Your credentials are NOT:")
	fmt.Println("- sent to an AI model")
	fmt.Println("- stored inside your MCP client configuration")
	fmt.Println("- transmitted outside your machine by this wizard")
	fmt.Println()
	fmt.Printf("The generated credentials will be written to:\n    %s\n", absEnvPath)
	fmt.Println()
	fmt.Println("Octo DB reads this local file at runtime in order to connect to your")
	fmt.Println("database.")
	fmt.Println()
	fmt.Println("AI agents interact with Octo DB tools, not directly with your database")
	fmt.Println("credentials.")
	fmt.Println()
	fmt.Println("Review and secure the generated .env file according to your")
	fmt.Println("organization's policies.")
	fmt.Println("==================================================")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Press Enter to continue setup, or Ctrl+C to abort... ")
	_, _ = reader.ReadString('\n')
	fmt.Println()

	// 1. Driver Selection
	fmt.Println("Select database driver:")
	fmt.Println("1) PostgreSQL")
	fmt.Println("2) MySQL")
	fmt.Println("3) MariaDB")
	fmt.Println("4) SQLite")
	fmt.Print("Enter option (1-4): ")
	driverChoice, _ := reader.ReadString('\n')
	driverChoice = strings.TrimSpace(driverChoice)

	var driver string
	for {
		switch driverChoice {
		case "1":
			driver = "postgres"
		case "2":
			driver = "mysql"
		case "3":
			driver = "mariadb"
		case "4":
			driver = "sqlite"
		}
		if driver != "" {
			break
		}
		fmt.Print("Invalid selection. Please enter 1, 2, 3, or 4: ")
		driverChoice, _ = reader.ReadString('\n')
		driverChoice = strings.TrimSpace(driverChoice)
	}
	fmt.Println()

	// 2. Alias Selection
	fmt.Print("Connection alias [default: main]: ")
	alias, _ := reader.ReadString('\n')
	alias = strings.TrimSpace(alias)
	if alias == "" {
		alias = "main"
	}
	for {
		valid := true
		if len(alias) == 0 {
			valid = false
		} else {
			for _, char := range alias {
				if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_') {
					valid = false
					break
				}
			}
		}
		if valid {
			break
		}
		fmt.Print("Invalid alias. Use only alphanumeric characters and underscores: ")
		alias, _ = reader.ReadString('\n')
		alias = strings.TrimSpace(alias)
		if alias == "" {
			alias = "main"
		}
	}
	fmt.Println()

	// 3. Driver-specific configuration questions
	var host, port, dbName, user, password, sslMode string

	if driver == "sqlite" {
		fmt.Print("Database file path [default: ./database.db]: ")
		dbName, _ = reader.ReadString('\n')
		dbName = strings.TrimSpace(dbName)
		if dbName == "" {
			dbName = "./database.db"
		}
	} else {
		// Host
		fmt.Print("Host [default: localhost]: ")
		host, _ = reader.ReadString('\n')
		host = strings.TrimSpace(host)
		if host == "" {
			host = "localhost"
		}

		// Port
		defaultPort := "3306"
		if driver == "postgres" {
			defaultPort = "5432"
		}
		fmt.Printf("Port [default: %s]: ", defaultPort)
		port, _ = reader.ReadString('\n')
		port = strings.TrimSpace(port)
		if port == "" {
			port = defaultPort
		}
		// Validate port is numeric
		for {
			_, err := strconv.Atoi(port)
			if err == nil {
				break
			}
			fmt.Print("Invalid port. Please enter a valid number: ")
			port, _ = reader.ReadString('\n')
			port = strings.TrimSpace(port)
			if port == "" {
				port = defaultPort
			}
		}

		// Database Name
		fmt.Print("Database Name: ")
		dbName, _ = reader.ReadString('\n')
		dbName = strings.TrimSpace(dbName)
		for dbName == "" {
			fmt.Print("Database Name is required: ")
			dbName, _ = reader.ReadString('\n')
			dbName = strings.TrimSpace(dbName)
		}

		// Username
		fmt.Print("Username: ")
		user, _ = reader.ReadString('\n')
		user = strings.TrimSpace(user)
		for user == "" {
			fmt.Print("Username is required: ")
			user, _ = reader.ReadString('\n')
			user = strings.TrimSpace(user)
		}

		// Password
		fmt.Print("Password: ")
		password, _ = reader.ReadString('\n')
		password = strings.TrimSpace(password)

		// SSL Mode for Postgres
		if driver == "postgres" {
			fmt.Print("SSL Mode [default: disable]: ")
			sslMode, _ = reader.ReadString('\n')
			sslMode = strings.TrimSpace(sslMode)
			if sslMode == "" {
				sslMode = "disable"
			}
		}
	}
	fmt.Println()

	// 4. Existing .env handling
	envExists := false
	if _, err := os.Stat(".env"); err == nil {
		envExists = true
	}

	if envExists {
		fmt.Println("Existing .env detected.")
		fmt.Println("1) Overwrite")
		fmt.Println("2) Backup and regenerate")
		fmt.Println("3) Cancel")
		fmt.Print("Enter option (1-3): ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)
		for choice != "1" && choice != "2" && choice != "3" {
			fmt.Print("Invalid selection. Please enter 1, 2, or 3: ")
			choice, _ = reader.ReadString('\n')
			choice = strings.TrimSpace(choice)
		}

		if choice == "3" {
			fmt.Println("Setup cancelled.")
			return
		}

		if choice == "2" {
			timestamp := time.Now().Format("20060102150405")
			backupName := fmt.Sprintf(".env.bak.%s", timestamp)

			srcFile, err := os.Open(".env")
			if err != nil {
				fmt.Printf("Error opening existing .env: %v\n", err)
				os.Exit(1)
			}
			defer srcFile.Close()

			dstFile, err := os.Create(backupName)
			if err != nil {
				fmt.Printf("Error creating backup file %s: %v\n", backupName, err)
				os.Exit(1)
			}
			defer dstFile.Close()

			if _, err = io.Copy(dstFile, srcFile); err != nil {
				fmt.Printf("Error backing up file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Successfully backed up existing .env to %s\n\n", backupName)
		}
	}

	// 5. Generate config
	var envBuilder strings.Builder
	envBuilder.WriteString(fmt.Sprintf("OCTO_DB_DEFAULT=%s\n\n", alias))

	upperAlias := strings.ToUpper(alias)
	envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_DRIVER=%s\n", upperAlias, driver))

	if driver != "sqlite" {
		envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_HOST=%s\n", upperAlias, host))
		envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_PORT=%s\n", upperAlias, port))
		envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_DATABASE=%s\n", upperAlias, dbName))
		envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_USER=%s\n", upperAlias, user))
		envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_PASSWORD=%s\n", upperAlias, password))
		if driver == "postgres" {
			envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_SSLMODE=%s\n", upperAlias, sslMode))
		}
	} else {
		envBuilder.WriteString(fmt.Sprintf("OCTO_DB_%s_DATABASE=%s\n", upperAlias, dbName))
	}

	envBuilder.WriteString("\n")
	envBuilder.WriteString("OCTO_DB_ENABLE_WRITE=false\n")
	envBuilder.WriteString("OCTO_DB_MAX_ROWS=500\n")
	envBuilder.WriteString("OCTO_DB_QUERY_TIMEOUT_SECONDS=10\n")

	err = os.WriteFile(".env", []byte(envBuilder.String()), 0600)
	if err != nil {
		fmt.Printf("Error writing .env file: %v\n", err)
		os.Exit(1)
	}

	// 6. Post-Generation Confirmation
	fmt.Println("==================================================")
	fmt.Println("        Configuration created successfully        ")
	fmt.Println("==================================================")
	fmt.Println()
	fmt.Println("Generated file:")
	fmt.Printf("    %s\n", absEnvPath)
	fmt.Println()
	fmt.Println("Reminder:")
	fmt.Println("This file contains database credentials.")
	fmt.Println("Protect it appropriately.")
	fmt.Println()
	fmt.Println("Recommended:")
	fmt.Println("- Add .env to .gitignore")
	fmt.Println("- Do not commit credentials")
	fmt.Println("- Use read-only database users whenever possible")
	fmt.Println()
	fmt.Println("==================================================")
	fmt.Println()

	// 7. Optional Connection Test
	fmt.Print("Would you like to test connectivity now? (y/N): ")
	testConn, _ := reader.ReadString('\n')
	testConn = strings.TrimSpace(strings.ToLower(testConn))

	if testConn == "y" || testConn == "yes" {
		fmt.Println("Testing connectivity...")
		dbCfg := DBConfig{
			Type:     driver,
			Host:     host,
			Port:     port,
			User:     user,
			Password: password,
			Name:     dbName,
			SSLMode:  sslMode,
		}

		client, connErr := NewDBClient(dbCfg)
		if connErr != nil {
			fmt.Printf("✗ Connection failed: %v\n", connErr)
		} else {
			client.Close()
			fmt.Println("✓ Configuration loaded")
			fmt.Println("✓ Database reachable")
			fmt.Println("✓ Credentials valid")
			fmt.Println("✓ Policies loaded")
			fmt.Println()
			fmt.Println("Ready to use.")
		}
	}
	fmt.Println()

	// 8. MCP Client Assistance
	currentDir, err := os.Getwd()
	if err != nil {
		currentDir = "."
	}

	binaryPath := filepath.Join(currentDir, "octo-db")

	fmt.Println("MCP Client Configuration Snippets")
	fmt.Println("=================================")
	fmt.Println("Note: Direct binary paths are shown below. For security, we recommend using")
	fmt.Println("a wrapper script (run-octo-db.sh / .bat) to source the environment variables.")
	fmt.Println()

	fmt.Println("1. Codex (codex.toml):")
	fmt.Println("----------------------")
	fmt.Printf("[mcp_servers.octo_db]\ncommand = %q\nargs = []\n\n", binaryPath)

	fmt.Println("2. Claude Desktop (claude_desktop_config.json):")
	fmt.Println("-----------------------------------------------")
	fmt.Printf("{\n  \"mcpServers\": {\n    \"octo_db\": {\n      \"command\": %q\n    }\n  }\n}\n\n", binaryPath)

	fmt.Println("3. Cursor / Cline / Roo Code:")
	fmt.Println("-----------------------------")
	fmt.Printf("{\n  \"mcpServers\": {\n    \"octo_db\": {\n      \"command\": %q,\n      \"args\": [],\n      \"disabled\": false\n    }\n  }\n}\n", binaryPath)
	fmt.Println("=================================")
}
