# octo-db

[![CI](https://github.com/nayosx/mcp-octo-db/actions/workflows/ci.yml/badge.svg)](https://github.com/nayosx/mcp-octo-db/actions/workflows/ci.yml)
[![Release](https://github.com/nayosx/mcp-octo-db/actions/workflows/release.yml/badge.svg)](https://github.com/nayosx/mcp-octo-db/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`octo-db` is a Go-based Model Context Protocol server that helps AI agents inspect and query relational databases with practical guardrails for local and test environments.

Current release: `v1.4.0`

## Why This Exists

Most agents can reason about code much better than they can safely reason about live database access. `octo-db` gives them a structured bridge:

- one MCP server
- multiple databases
- read-first defaults
- explicit write enablement
- policy controls for schemas and tables

The goal is to make database-aware agents genuinely useful for developers without pretending this Community Edition is a full security boundary.

## What It Can Do

- Connect to PostgreSQL, MySQL, MariaDB, and SQLite.
- Load multiple databases from `.env` and `config.yaml`.
- Expose discovery tools like `server_info`, `list_schemas`, `list_tables`, `list_views`, `search_tables`, `find_columns`, `describe_table`, `list_indexes`, and `list_relationships`.
- Suggest query-building steps for non-technical reporting questions with `suggest_query_plan`.
- Expose query tools like `read_query`, `get_table_sample`, `explain_query`, and optional `write_query`.
- Start even if one configured database is offline, so other configured databases remain available.

## Security Model / Threat Model

This Community Edition is built for **local development and test environments**. It provides guardrails, not hard isolation.

What it does:

- disables `write_query` by default
- blocks multi-statement SQL in read and write paths
- blocks mutating CTEs from read-only tools
- enforces `allowed_schemas`, `allowed_tables`, and `denied_tables`
- caps rows and applies timeouts
- keeps logs on `stderr` so the MCP `stdio` channel stays clean

What it does not do:

- per-operation human approval workflows
- strong sandboxing against every SQL dialect trick
- row-level or tenant-level access isolation
- centralized policy management across many users
- PII masking or enterprise-grade audit pipelines

Use it as a developer-facing connector with safety rails, not as your only production control plane.

## Quick Start

Requirements:

- Go 1.22+
- access to your target databases

Local Postgres test environment:

```bash
docker compose up -d
```

Build:

```bash
git clone <your-repo-url>
cd <your-local-folder>
go build -o octo-db
```

Run diagnostics:

```bash
./octo-db doctor
```

Print operational info:

```bash
./octo-db --version
./octo-db --list-tools
./octo-db --print-effective-config
```

## Configuration

Precedence order:

1. environment variables
2. `config.yaml`
3. built-in defaults

### `.env`

Use [.env.example](/home/ness/Development/go/mcp_octo_db/.env.example) as a base.

```env
DB_TYPE=postgres
DB_HOST=127.0.0.1
DB_PORT=5432
DB_USER=appuser
DB_PASSWORD=change_me
DB_NAME=appdb
DB_SSLMODE=disable

DB_TYPE_ANALYTICS=mysql
DB_HOST_ANALYTICS=127.0.0.1
DB_PORT_ANALYTICS=3306
DB_USER_ANALYTICS=analytics_user
DB_PASSWORD_ANALYTICS=change_me
DB_NAME_ANALYTICS=analytics_db

MCP_ENABLE_WRITE=false
MCP_MAX_ROWS=500
MCP_QUERY_TIMEOUT_SECONDS=10
MCP_MAX_OPEN_CONNS=10
MCP_MAX_IDLE_CONNS=5
MCP_CONN_MAX_LIFETIME_SECONDS=300
MCP_CONN_MAX_IDLE_TIME_SECONDS=180
MCP_ALLOWED_SCHEMAS=public,analytics
MCP_ALLOWED_TABLES=users,orders
MCP_DENIED_TABLES=secrets,audit_backups
MCP_LOG_LEVEL=info
MCP_LOG_FORMAT=text
MCP_AUDIT_LOG=false
```

### `config.yaml`

Use [config.yaml.example](/home/ness/Development/go/mcp_octo_db/config.yaml.example) as a base.

Supported settings:

- `enable_write`
- `max_rows`
- `query_timeout_seconds`
- `max_open_conns`
- `max_idle_conns`
- `conn_max_lifetime_seconds`
- `conn_max_idle_time_seconds`
- `allowed_schemas`
- `allowed_tables`
- `denied_tables`
- `log_level`
- `log_format`
- `audit_log`

Validation rules:

- database names must be non-empty
- supported types are `postgres`, `postgresql`, `mysql`, `mariadb`, `sqlite`, `sqlite3`
- `name` is required for every database
- `host`, `port`, and `user` are required for Postgres/MySQL/MariaDB
- `max_rows` and `query_timeout_seconds` must be greater than zero
- `max_open_conns` must be greater than zero
- `max_idle_conns` must be zero or greater and cannot exceed `max_open_conns`
- `conn_max_lifetime_seconds` and `conn_max_idle_time_seconds` must be zero or greater
- `log_level` must be one of `debug`, `info`, `warn`, `error`
- `log_format` must be `text` or `json`

### Safe Configuration Patterns

Local read-only setup:

```yaml
settings:
  enable_write: false
  max_rows: 200
  query_timeout_seconds: 10
  allowed_schemas:
    - public
```

Narrow table access:

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

Higher-throughput local agent usage:

```yaml
settings:
  max_open_conns: 20
  max_idle_conns: 10
  conn_max_lifetime_seconds: 300
  conn_max_idle_time_seconds: 180
```

## MCP Tools

Always available:

- `server_info`
- `list_schemas`
- `list_tables`
- `list_views`
- `search_tables`
- `find_columns`
- `describe_table`
- `list_indexes`
- `list_relationships`
- `get_table_sample`
- `read_query`
- `explain_query`
- `suggest_query_plan`

Conditionally available:

- `write_query` when `enable_write=true`

### Tool Notes

- `server_info` returns active settings, enabled tools, configured databases, and offline databases.
- `list_views` exposes views using the same schema policy checks as tables.
- `list_indexes` returns index names, uniqueness, and indexed columns for a table.
- `find_columns` helps map business words like `service`, `sold`, `total`, or `date` to likely real columns.
- `list_relationships` exposes foreign-key paths that an agent can use to build joins.
- `suggest_query_plan` does not execute SQL; it proposes candidate tables, columns, joins, and filters from a natural-language request.
- `read_query` accepts `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, and `EXPLAIN`, but blocks mutating CTEs.
- `write_query` remains a global opt-in for the Community Edition.

### Example Workflows

Find likely columns for a business concept:

```json
{
  "tool": "find_columns",
  "arguments": {
    "db_name": "default",
    "schema": "public",
    "query": "service"
  }
}
```

Inspect real joins before generating SQL:

```json
{
  "tool": "list_relationships",
  "arguments": {
    "db_name": "default",
    "schema": "public",
    "table_name": "order_items"
  }
}
```

Turn a non-technical question into a safe plan first:

```json
{
  "tool": "suggest_query_plan",
  "arguments": {
    "db_name": "default",
    "schema": "public",
    "question": "quiero saber cuantos items se vendieron para el service 133 este mes"
  }
}
```

Typical flow for a non-technical request:

1. `suggest_query_plan`
2. `find_columns`
3. `list_relationships`
4. `read_query`

This gives the agent a chance to discover the right joins and date columns before touching production-like data.

## Diagnostics

```bash
./octo-db doctor
./octo-db --env /path/to/.env doctor
./octo-db --config /path/to/config.yaml doctor
```

`doctor` reports:

- normalized settings
- enabled MCP tools
- configuration validation status
- connection results per configured database

Useful operational flags:

```bash
./octo-db --version
./octo-db --list-tools
./octo-db --print-effective-config
```

Notes:

- `--list-tools` reflects whether `write_query` is currently enabled.
- `--print-effective-config` masks passwords before printing.
- `doctor` is the quickest way to confirm connectivity and the currently active policy surface.

## Client Examples

Ready-to-copy example configurations live in [examples/](/home/ness/Development/go/mcp_octo_db/examples):

- [Claude Desktop](/home/ness/Development/go/mcp_octo_db/examples/claude-desktop.json)
- [Cursor](/home/ness/Development/go/mcp_octo_db/examples/cursor.json)
- [Codex-compatible clients](/home/ness/Development/go/mcp_octo_db/examples/codex.json)
- [Cline](/home/ness/Development/go/mcp_octo_db/examples/cline.json)
- [Roo Code](/home/ness/Development/go/mcp_octo_db/examples/roo-code.json)

Generic MCP `stdio` example:

```json
{
  "mcpServers": {
    "octo-db": {
      "command": "/absolute/path/to/octo-db",
      "args": [
        "--config",
        "/absolute/path/to/config.yaml"
      ],
      "cwd": "/absolute/path/to/project"
    }
  }
}
```

## Docker

Build the image:

```bash
docker build -t octo-db .
```

Run diagnostics in a container:

```bash
docker run --rm \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  octo-db --config /app/config.yaml doctor
```

You can also print the effective config from the container:

```bash
docker run --rm \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  octo-db --config /app/config.yaml --print-effective-config
```

## Development

Useful local checks:

```bash
go test ./...
go vet ./...
go build ./...
./octo-db doctor
```

Recommended manual smoke test after changes:

1. Run `./octo-db doctor`
2. Call `server_info`
3. Call `find_columns` with a known business term
4. Call `suggest_query_plan` with a non-technical question
5. Confirm `read_query` still enforces policy and row limits

CI is defined in [.github/workflows/ci.yml](/home/ness/Development/go/mcp_octo_db/.github/workflows/ci.yml) and release packaging in [.github/workflows/release.yml](/home/ness/Development/go/mcp_octo_db/.github/workflows/release.yml).

## Release Checklist

- run `go test ./...`
- run `go vet ./...`
- run `go build ./...`
- verify `doctor` with a real local config
- review the examples in [examples/](/home/ness/Development/go/mcp_octo_db/examples)
- review [CHANGELOG.md](/home/ness/Development/go/mcp_octo_db/CHANGELOG.md)
- tag a version like `v1.4.0`

## Changelog

Release notes live in [CHANGELOG.md](/home/ness/Development/go/mcp_octo_db/CHANGELOG.md).

## License

MIT. See [LICENSE](/home/ness/Development/go/mcp_octo_db/LICENSE).
