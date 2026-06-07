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
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetimeSecs int
	ConnMaxIdleTimeSecs int
	AllowedSchemas      []string
	AllowedTables       []string
	DeniedTables        []string
	LogLevel            string
	LogFormat           string
	AuditLog            bool
}

var defaultSettings = Settings{
	EnableWrite:         false,
	MaxRows:             500,
	QueryTimeoutSeconds: 10,
	MaxOpenConns:        10,
	MaxIdleConns:        5,
	ConnMaxLifetimeSecs: 300,
	ConnMaxIdleTimeSecs: 180,
	AllowedSchemas:      []string{},
	AllowedTables:       []string{},
	DeniedTables:        []string{},
	LogLevel:            "info",
	LogFormat:           "text",
	AuditLog:            false,
}

var GlobalSettings = defaultSettings

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
		MaxOpenConns        *int     `yaml:"max_open_conns"`
		MaxIdleConns        *int     `yaml:"max_idle_conns"`
		ConnMaxLifetimeSecs *int     `yaml:"conn_max_lifetime_seconds"`
		ConnMaxIdleTimeSecs *int     `yaml:"conn_max_idle_time_seconds"`
		AllowedSchemas      []string `yaml:"allowed_schemas"`
		AllowedTables       []string `yaml:"allowed_tables"`
		DeniedTables        []string `yaml:"denied_tables"`
		LogLevel            string   `yaml:"log_level"`
		LogFormat           string   `yaml:"log_format"`
		AuditLog            *bool    `yaml:"audit_log"`
	} `yaml:"settings"`
}

func LoadConfig(envPath, configPath string) (map[string]DBConfig, error) {
	GlobalSettings = defaultSettings

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
			dbConfigs[strings.ToLower(strings.TrimSpace(name))] = DBConfig{
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
	if val, ok := lookupEnvWithFallback("OCTO_DB_ENABLE_WRITE", "MCP_ENABLE_WRITE"); ok {
		GlobalSettings.EnableWrite = strings.ToLower(val) == "true"
	} else if yamlLoaded && yamlCfg.Settings.EnableWrite != nil {
		GlobalSettings.EnableWrite = *yamlCfg.Settings.EnableWrite
	}

	// Max Rows
	if val, ok := lookupEnvWithFallback("OCTO_DB_MAX_ROWS", "MCP_MAX_ROWS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.MaxRows = i
		}
	} else if yamlLoaded && yamlCfg.Settings.MaxRows != nil {
		GlobalSettings.MaxRows = *yamlCfg.Settings.MaxRows
	}

	// Query Timeout
	if val, ok := lookupEnvWithFallback("OCTO_DB_QUERY_TIMEOUT_SECONDS", "MCP_QUERY_TIMEOUT_SECONDS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.QueryTimeoutSeconds = i
		}
	} else if yamlLoaded && yamlCfg.Settings.QueryTimeoutSeconds != nil {
		GlobalSettings.QueryTimeoutSeconds = *yamlCfg.Settings.QueryTimeoutSeconds
	}

	// Pool settings
	if val, ok := lookupEnvWithFallback("OCTO_DB_MAX_OPEN_CONNS", "MCP_MAX_OPEN_CONNS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.MaxOpenConns = i
		}
	} else if yamlLoaded && yamlCfg.Settings.MaxOpenConns != nil {
		GlobalSettings.MaxOpenConns = *yamlCfg.Settings.MaxOpenConns
	}

	if val, ok := lookupEnvWithFallback("OCTO_DB_MAX_IDLE_CONNS", "MCP_MAX_IDLE_CONNS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.MaxIdleConns = i
		}
	} else if yamlLoaded && yamlCfg.Settings.MaxIdleConns != nil {
		GlobalSettings.MaxIdleConns = *yamlCfg.Settings.MaxIdleConns
	}

	if val, ok := lookupEnvWithFallback("OCTO_DB_CONN_MAX_LIFETIME_SECONDS", "MCP_CONN_MAX_LIFETIME_SECONDS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.ConnMaxLifetimeSecs = i
		}
	} else if yamlLoaded && yamlCfg.Settings.ConnMaxLifetimeSecs != nil {
		GlobalSettings.ConnMaxLifetimeSecs = *yamlCfg.Settings.ConnMaxLifetimeSecs
	}

	if val, ok := lookupEnvWithFallback("OCTO_DB_CONN_MAX_IDLE_TIME_SECONDS", "MCP_CONN_MAX_IDLE_TIME_SECONDS"); ok {
		if i, err := strconv.Atoi(val); err == nil {
			GlobalSettings.ConnMaxIdleTimeSecs = i
		}
	} else if yamlLoaded && yamlCfg.Settings.ConnMaxIdleTimeSecs != nil {
		GlobalSettings.ConnMaxIdleTimeSecs = *yamlCfg.Settings.ConnMaxIdleTimeSecs
	}

	// Allowed Schemas
	if val, ok := lookupEnvWithFallback("OCTO_DB_ALLOWED_SCHEMAS", "MCP_ALLOWED_SCHEMAS"); ok {
		GlobalSettings.AllowedSchemas = parseCommaList(val)
	} else if yamlLoaded && len(yamlCfg.Settings.AllowedSchemas) > 0 {
		GlobalSettings.AllowedSchemas = yamlCfg.Settings.AllowedSchemas
	}

	// Allowed Tables
	if val, ok := lookupEnvWithFallback("OCTO_DB_ALLOWED_TABLES", "MCP_ALLOWED_TABLES"); ok {
		GlobalSettings.AllowedTables = parseCommaList(val)
	} else if yamlLoaded && len(yamlCfg.Settings.AllowedTables) > 0 {
		GlobalSettings.AllowedTables = yamlCfg.Settings.AllowedTables
	}

	// Denied Tables
	if val, ok := lookupEnvWithFallback("OCTO_DB_DENIED_TABLES", "MCP_DENIED_TABLES"); ok {
		GlobalSettings.DeniedTables = parseCommaList(val)
	} else if yamlLoaded && len(yamlCfg.Settings.DeniedTables) > 0 {
		GlobalSettings.DeniedTables = yamlCfg.Settings.DeniedTables
	}

	// Log Level
	if val, ok := lookupEnvWithFallback("OCTO_DB_LOG_LEVEL", "MCP_LOG_LEVEL"); ok {
		GlobalSettings.LogLevel = val
	} else if yamlLoaded && yamlCfg.Settings.LogLevel != "" {
		GlobalSettings.LogLevel = yamlCfg.Settings.LogLevel
	}

	// Log Format
	if val, ok := lookupEnvWithFallback("OCTO_DB_LOG_FORMAT", "MCP_LOG_FORMAT"); ok {
		GlobalSettings.LogFormat = val
	} else if yamlLoaded && yamlCfg.Settings.LogFormat != "" {
		GlobalSettings.LogFormat = yamlCfg.Settings.LogFormat
	}

	// Audit Log
	if val, ok := lookupEnvWithFallback("OCTO_DB_AUDIT_LOG", "MCP_AUDIT_LOG"); ok {
		GlobalSettings.AuditLog = strings.ToLower(val) == "true"
	} else if yamlLoaded && yamlCfg.Settings.AuditLog != nil {
		GlobalSettings.AuditLog = *yamlCfg.Settings.AuditLog
	}

	normalizeDBConfigs(dbConfigs)
	normalizeSettings(&GlobalSettings)

	if err := validateLoadedConfig(dbConfigs, GlobalSettings); err != nil {
		return nil, err
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

func lookupEnvWithFallback(keys ...string) (string, bool) {
	for _, key := range keys {
		if val, ok := os.LookupEnv(key); ok {
			return val, true
		}
	}
	return "", false
}

func normalizeDBConfigs(configs map[string]DBConfig) {
	for name, cfg := range configs {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		cfg.Type = strings.ToLower(strings.TrimSpace(cfg.Type))
		cfg.Host = strings.TrimSpace(cfg.Host)
		cfg.Port = strings.TrimSpace(cfg.Port)
		cfg.User = strings.TrimSpace(cfg.User)
		cfg.Password = strings.TrimSpace(cfg.Password)
		cfg.Name = strings.TrimSpace(cfg.Name)
		cfg.SSLMode = strings.TrimSpace(cfg.SSLMode)

		delete(configs, name)
		configs[normalizedName] = cfg
	}
}

func normalizeSettings(settings *Settings) {
	settings.AllowedSchemas = uniqueLowerTrimmed(settings.AllowedSchemas)
	settings.AllowedTables = uniqueLowerTrimmed(settings.AllowedTables)
	settings.DeniedTables = uniqueLowerTrimmed(settings.DeniedTables)
	settings.LogLevel = strings.ToLower(strings.TrimSpace(settings.LogLevel))
	settings.LogFormat = strings.ToLower(strings.TrimSpace(settings.LogFormat))
}

func uniqueLowerTrimmed(values []string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
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

func validateLoadedConfig(dbConfigs map[string]DBConfig, settings Settings) error {
	var issues []string

	if settings.MaxRows <= 0 {
		issues = append(issues, "settings.max_rows must be greater than 0")
	}
	if settings.QueryTimeoutSeconds <= 0 {
		issues = append(issues, "settings.query_timeout_seconds must be greater than 0")
	}
	if settings.MaxOpenConns <= 0 {
		issues = append(issues, "settings.max_open_conns must be greater than 0")
	}
	if settings.MaxIdleConns < 0 {
		issues = append(issues, "settings.max_idle_conns must be greater than or equal to 0")
	}
	if settings.MaxIdleConns > settings.MaxOpenConns {
		issues = append(issues, "settings.max_idle_conns cannot be greater than settings.max_open_conns")
	}
	if settings.ConnMaxLifetimeSecs < 0 {
		issues = append(issues, "settings.conn_max_lifetime_seconds must be greater than or equal to 0")
	}
	if settings.ConnMaxIdleTimeSecs < 0 {
		issues = append(issues, "settings.conn_max_idle_time_seconds must be greater than or equal to 0")
	}
	if settings.LogFormat != "text" && settings.LogFormat != "json" {
		issues = append(issues, "settings.log_format must be one of: text, json")
	}
	switch settings.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		issues = append(issues, "settings.log_level must be one of: debug, info, warn, error")
	}

	for name, cfg := range dbConfigs {
		if name == "" {
			issues = append(issues, "database names cannot be empty")
			continue
		}

		switch cfg.Type {
		case "postgres", "postgresql", "mysql", "mariadb", "sqlite", "sqlite3":
		default:
			issues = append(issues, fmt.Sprintf("database '%s': unsupported type '%s'", name, cfg.Type))
			continue
		}

		if cfg.Name == "" {
			issues = append(issues, fmt.Sprintf("database '%s': name is required", name))
		}

		switch cfg.Type {
		case "postgres", "postgresql", "mysql", "mariadb":
			if cfg.Host == "" {
				issues = append(issues, fmt.Sprintf("database '%s': host is required for %s", name, cfg.Type))
			}
			if cfg.Port == "" {
				issues = append(issues, fmt.Sprintf("database '%s': port is required for %s", name, cfg.Type))
			}
			if cfg.User == "" {
				issues = append(issues, fmt.Sprintf("database '%s': user is required for %s", name, cfg.Type))
			}
		}
	}

	if len(issues) > 0 {
		return fmt.Errorf("configuration validation failed:\n- %s", strings.Join(issues, "\n- "))
	}

	return nil
}

func EffectiveConfig(dbConfigs map[string]DBConfig) map[string]any {
	maskedDBs := make(map[string]map[string]any, len(dbConfigs))
	for name, cfg := range dbConfigs {
		maskedDBs[name] = map[string]any{
			"type":     cfg.Type,
			"host":     cfg.Host,
			"port":     cfg.Port,
			"user":     cfg.User,
			"password": maskSecret(cfg.Password),
			"name":     cfg.Name,
			"sslmode":  cfg.SSLMode,
		}
	}

	return map[string]any{
		"settings": map[string]any{
			"enable_write":               GlobalSettings.EnableWrite,
			"max_rows":                   GlobalSettings.MaxRows,
			"query_timeout_seconds":      GlobalSettings.QueryTimeoutSeconds,
			"max_open_conns":             GlobalSettings.MaxOpenConns,
			"max_idle_conns":             GlobalSettings.MaxIdleConns,
			"conn_max_lifetime_seconds":  GlobalSettings.ConnMaxLifetimeSecs,
			"conn_max_idle_time_seconds": GlobalSettings.ConnMaxIdleTimeSecs,
			"allowed_schemas":            GlobalSettings.AllowedSchemas,
			"allowed_tables":             GlobalSettings.AllowedTables,
			"denied_tables":              GlobalSettings.DeniedTables,
			"log_level":                  GlobalSettings.LogLevel,
			"log_format":                 GlobalSettings.LogFormat,
			"audit_log":                  GlobalSettings.AuditLog,
		},
		"databases": maskedDBs,
	}
}

func maskSecret(value string) string {
	if value == "" {
		return "(empty)"
	}
	return "****"
}
