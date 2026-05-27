package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type Settings struct {
	EnableWrite         bool
	MaxRows             int
	QueryTimeoutSeconds int
	AllowedSchemas      []string
	AllowedTables       []string
	DeniedTables        []string
	LogLevel            string
	LogFormat           string
	AuditLog            bool
}

var GlobalSettings = Settings{
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

type YamlConfig struct {
	Databases map[string]struct {
		Type     string `yaml:"type"`
		Host     string `yaml:"host"`
		Port     string `yaml:"port"`
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Name     string `yaml:"name"`
		SSLMode  string `yaml:"sslmode"`
	} `yaml:"databases"`
	Settings struct {
		EnableWrite         *bool    `yaml:"enable_write"`
		MaxRows             *int     `yaml:"max_rows"`
		QueryTimeoutSeconds *int     `yaml:"query_timeout_seconds"`
		AllowedSchemas      []string `yaml:"allowed_schemas"`
		AllowedTables       []string `yaml:"allowed_tables"`
		DeniedTables        []string `yaml:"denied_tables"`
		LogLevel            string   `yaml:"log_level"`
		LogFormat           string   `yaml:"log_format"`
		AuditLog            *bool    `yaml:"audit_log"`
	} `yaml:"settings"`
}

func LoadConfig(envPath, configPath string) (map[string]DBConfig, error) {
	// 1. Cargar .env si existe o si se especifica
	if envPath != "" {
		if err := godotenv.Load(envPath); err != nil {
			return nil, fmt.Errorf("failed to load specified env file %s: %w", envPath, err)
		}
	} else {
		// Intentar cargar .env por defecto (silenciosamente si no existe)
		err := godotenv.Load()
		if err != nil {
			if exePath, exeErr := os.Executable(); exeErr == nil {
				exeDir := filepath.Dir(exePath)
				envDefaultPath := filepath.Join(exeDir, ".env")
				_ = godotenv.Load(envDefaultPath)
			}
		}
	}

	dbConfigs := make(map[string]DBConfig)
	var yamlCfg YamlConfig
	yamlLoaded := false

	// 2. Cargar config YAML si se especifica
	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, &yamlCfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
		}
		yamlLoaded = true

		// Cargar DBs del YAML
		for name, db := range yamlCfg.Databases {
			dbConfigs[strings.ToLower(name)] = DBConfig{
				Type:     db.Type,
				Host:     db.Host,
				Port:     db.Port,
				User:     db.User,
				Password: db.Password,
				Name:     db.Name,
				SSLMode:  db.SSLMode,
			}
		}
	}

	// 3. Descubrir/sobreescribir con bases de datos del entorno
	envDBs, err := DiscoverDBs()
	if err == nil && len(envDBs) > 0 {
		for name, cfg := range envDBs {
			dbConfigs[name] = cfg
		}
	}

	// 4. Resolver Settings (Precedencia: Env > YAML > Defaults)
	// Enable Write
	if val, ok := os.LookupEnv("MCP_ENABLE_WRITE"); ok {
		GlobalSettings.EnableWrite = strings.ToLower(val) == "true"
	} else if yamlLoaded && yamlCfg.Settings.EnableWrite != nil {
		GlobalSettings.EnableWrite = *yamlCfg.Settings.EnableWrite
	}

	// Max Rows
	if val, ok := os.LookupEnv("MCP_MAX_ROWS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.MaxRows = i
		}
	} else if yamlLoaded && yamlCfg.Settings.MaxRows != nil {
		GlobalSettings.MaxRows = *yamlCfg.Settings.MaxRows
	}

	// Query Timeout
	if val, ok := os.LookupEnv("MCP_QUERY_TIMEOUT_SECONDS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.QueryTimeoutSeconds = i
		}
	} else if yamlLoaded && yamlCfg.Settings.QueryTimeoutSeconds != nil {
		GlobalSettings.QueryTimeoutSeconds = *yamlCfg.Settings.QueryTimeoutSeconds
	}

	// Allowed Schemas
	if val, ok := os.LookupEnv("MCP_ALLOWED_SCHEMAS"); ok {
		GlobalSettings.AllowedSchemas = parseCommaList(val)
	} else if yamlLoaded && len(yamlCfg.Settings.AllowedSchemas) > 0 {
		GlobalSettings.AllowedSchemas = yamlCfg.Settings.AllowedSchemas
	}

	// Allowed Tables
	if val, ok := os.LookupEnv("MCP_ALLOWED_TABLES"); ok {
		GlobalSettings.AllowedTables = parseCommaList(val)
	} else if yamlLoaded && len(yamlCfg.Settings.AllowedTables) > 0 {
		GlobalSettings.AllowedTables = yamlCfg.Settings.AllowedTables
	}

	// Denied Tables
	if val, ok := os.LookupEnv("MCP_DENIED_TABLES"); ok {
		GlobalSettings.DeniedTables = parseCommaList(val)
	} else if yamlLoaded && len(yamlCfg.Settings.DeniedTables) > 0 {
		GlobalSettings.DeniedTables = yamlCfg.Settings.DeniedTables
	}

	// Log Level
	if val, ok := os.LookupEnv("MCP_LOG_LEVEL"); ok {
		GlobalSettings.LogLevel = val
	} else if yamlLoaded && yamlCfg.Settings.LogLevel != "" {
		GlobalSettings.LogLevel = yamlCfg.Settings.LogLevel
	}

	// Log Format
	if val, ok := os.LookupEnv("MCP_LOG_FORMAT"); ok {
		GlobalSettings.LogFormat = val
	} else if yamlLoaded && yamlCfg.Settings.LogFormat != "" {
		GlobalSettings.LogFormat = yamlCfg.Settings.LogFormat
	}

	// Audit Log
	if val, ok := os.LookupEnv("MCP_AUDIT_LOG"); ok {
		GlobalSettings.AuditLog = strings.ToLower(val) == "true"
	} else if yamlLoaded && yamlCfg.Settings.AuditLog != nil {
		GlobalSettings.AuditLog = *yamlCfg.Settings.AuditLog
	}

	return dbConfigs, nil
}

func parseCommaList(val string) []string {
	parts := strings.Split(val, ",")
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
