# Octo DB Configuration Guide

This document details the configuration options, precedence rules, and safe configuration patterns for `octo-db`.

---

## Precedence Order

When starting `octo-db`, configurations are resolved in the following order:
1. Environment variables (highest priority)
2. `config.yaml`
3. Built-in defaults (lowest priority)

---

## Environment Variables (`.env`)

You can configure `octo-db` via environment variables. Use the provided [.env.example](../.env.example) as a base. 

The environment variables use the `OCTO_DB_` prefix as primary, but fully support legacy `DB_` / `MCP_` variables for backward compatibility.

### Configuration Reference

```env
# Default connection suffix
OCTO_DB_DEFAULT=main

# Default Connection settings (main)
OCTO_DB_MAIN_DRIVER=mysql
OCTO_DB_MAIN_HOST=localhost
OCTO_DB_MAIN_PORT=3306
OCTO_DB_MAIN_DATABASE=my_database
OCTO_DB_MAIN_USER=my_user
OCTO_DB_MAIN_PASSWORD=change_me
OCTO_DB_MAIN_SSLMODE=disable

# MCP Server settings
OCTO_DB_ENABLE_WRITE=false
OCTO_DB_MAX_ROWS=500
OCTO_DB_QUERY_TIMEOUT_SECONDS=10
OCTO_DB_MAX_OPEN_CONNS=10
OCTO_DB_MAX_IDLE_CONNS=5
OCTO_DB_CONN_MAX_LIFETIME_SECONDS=300
OCTO_DB_CONN_MAX_IDLE_TIME_SECONDS=180
OCTO_DB_ALLOWED_SCHEMAS=public,analytics
OCTO_DB_ALLOWED_TABLES=users,orders
OCTO_DB_DENIED_TABLES=secrets,audit_backups
OCTO_DB_LOG_LEVEL=info
OCTO_DB_LOG_FORMAT=text
OCTO_DB_AUDIT_LOG=false
```

### Why the `OCTO_DB_` Prefix?

Using the structured prefix `OCTO_DB_<ALIAS>_<PROPERTY>` provides three critical benefits:

1. **Multi-Database Support**: Rather than being limited to a single generic configuration (like `DB_HOST`), you can define multiple target databases in the same environment by changing the middle `<ALIAS>` placeholder (e.g., `OCTO_DB_MAIN_...` vs. `OCTO_DB_ANALYTICS_...`). The server automatically scans for environment variables ending with `_DATABASE` and registers them as independent database aliases.
2. **Namespacing & Conflict Avoidance**: Standard environment names like `DB_HOST` or `DB_USER` are widely used by other applications, Docker environments, and libraries. Standardizing on the `OCTO_DB_` prefix ensures that this MCP server's configuration never clashes with or overrides other environment variables.
3. **Configuration Clarity**: It groups all configurations related to this MCP server (such as global policies `OCTO_DB_ENABLE_WRITE`, timeouts, and connection strings) under a single searchable prefix (e.g., `env | grep OCTO_DB_`).

---

## Configuration File (`config.yaml`)

Use [config.yaml.example](../config.yaml.example) as a base.

### Supported Settings:
* `enable_write` (boolean)
* `max_rows` (integer)
* `query_timeout_seconds` (integer)
* `max_open_conns` (integer)
* `max_idle_conns` (integer)
* `conn_max_lifetime_seconds` (integer)
* `conn_max_idle_time_seconds` (integer)
* `allowed_schemas` (array of strings)
* `allowed_tables` (array of strings)
* `denied_tables` (array of strings)
* `log_level` (`debug`, `info`, `warn`, `error`)
* `log_format` (`text`, `json`)
* `audit_log` (boolean)

### Validation Rules:
* Database names must be non-empty.
* Supported drivers/types are `postgres`, `postgresql`, `mysql`, `mariadb`, `sqlite`, `sqlite3`.
* `name` is required for every database.
* `host`, `port`, and `user` are required for Postgres/MySQL/MariaDB.
* `max_rows` and `query_timeout_seconds` must be greater than zero.
* `max_open_conns` must be greater than zero.
* `max_idle_conns` must be zero or greater and cannot exceed `max_open_conns`.
* `conn_max_lifetime_seconds` and `conn_max_idle_time_seconds` must be zero or greater.
* `log_level` must be one of `debug`, `info`, `warn`, `error`.
* `log_format` must be `text` or `json`.

---

## Safe Configuration Patterns

### Local Read-Only Setup
```yaml
settings:
  enable_write: false
  max_rows: 200
  query_timeout_seconds: 10
  allowed_schemas:
    - public
```

### Narrow Table Access
```yaml
settings:
  allowed_tables:
    - services
    - orders
    - order_items
  denied_tables:
    - secrets
    - audit_backups
```

### Higher-Throughput Local Agent Usage
```yaml
settings:
  max_open_conns: 20
  max_idle_conns: 10
  conn_max_lifetime_seconds: 300
  conn_max_idle_time_seconds: 180
```
