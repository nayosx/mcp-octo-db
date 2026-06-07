# octo-db

[![CI](https://github.com/nayosx/mcp-octo-db/actions/workflows/ci.yml/badge.svg)](https://github.com/nayosx/mcp-octo-db/actions/workflows/ci.yml)
[![Release](https://github.com/nayosx/mcp-octo-db/actions/workflows/release.yml/badge.svg)](https://github.com/nayosx/mcp-octo-db/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`octo-db` is a Go-based Model Context Protocol server that helps AI agents inspect and query relational databases with practical guardrails for local and test environments.

Current release: `v1.4.3`

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

## Security Recommendations

To ensure safe operation of the MCP server, please follow these guidelines:
- **Keep `.env` files secured**: Never commit `.env` or configurations with real credentials to your repository. Ensure `.env` is listed in your `.gitignore` file.
- **Do not share credentials**: Never store plain secrets or database passwords in shared MCP client configuration files (e.g., `claude_desktop_config.json`).
- **Use read-only database users**: Create a dedicated database user for the MCP server that only has read permissions (`SELECT`) on the necessary tables and schemas.
- **Limit write permissions**: Keep `OCTO_DB_ENABLE_WRITE` set to `false` unless write capabilities are strictly necessary. If enabled, restrict the database user permissions to the minimal set of write privileges needed.
- **Review queries before execution**: Be cautious and verify query plans or suggest plans before executing raw SQL writes or complex reads that could lock tables or impact performance.

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
mkdir -p dist
go build -o dist/octo-db .
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

Use [.env.example](.env.example) as a base. The environment variables now use the `OCTO_DB_` prefix as primary, but fully support legacy `DB_` / `MCP_` variables for backward compatibility.

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

#### Why the `OCTO_DB_` prefix?

Using the structured prefix `OCTO_DB_<ALIAS>_<PROPERTY>` provides three critical benefits:

- **Multi-Database Support**: Rather than being limited to a single generic configuration (like `DB_HOST`), you can define multiple target databases in the same environment by changing the middle `<ALIAS>` placeholder (e.g., `OCTO_DB_MAIN_...` vs. `OCTO_DB_ANALYTICS_...`). The server automatically scans for environment variables ending with `_DATABASE` and registers them as independent database aliases.
- **Namespacing & Conflict Avoidance**: Standard environment names like `DB_HOST` or `DB_USER` are widely used by other applications, Docker environments, and libraries. Standardizing on the `OCTO_DB_` prefix ensures that this MCP server's configuration never clashes with or overrides other environment variables.
- **Configuration Clarity**: It groups all configurations related to this MCP server (such as global policies `OCTO_DB_ENABLE_WRITE`, timeouts, and connection strings) under a single searchable prefix (e.g., `env | grep OCTO_DB_`).

### `config.yaml`

Use [config.yaml.example](config.yaml.example) as a base.

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

The server registers and exposes the following tools:

| Tool | Description | Read Only | Risk Level |
| --- | --- | --- | --- |
| `server_info` | Return server metadata, active policies, available databases, and enabled MCP tools. | Yes | Low |
| `list_schemas` | List all schemas/databases in the specified database. | Yes | Low |
| `list_tables` | List all tables in the specified database and schema. | Yes | Low |
| `list_views` | List all views in the specified database and schema. | Yes | Low |
| `search_tables` | Search for tables matching a query pattern (e.g. `%users%`) in the specified database. | Yes | Low |
| `find_columns` | Search for likely columns by business term, such as service, quantity, total, sold, created, or date. | Yes | Low |
| `describe_table` | Show structure of a specific table, including columns, types, nullability, primary keys, and default values. | Yes | Low |
| `list_indexes` | List indexes defined on a table, including uniqueness and indexed columns. | Yes | Low |
| `list_relationships` | List foreign-key relationships between tables, optionally focused on a single table. | Yes | Low |
| `get_table_sample` | Get a sample of rows from a table (default 10, max 100 rows). | Yes | Low |
| `read_query` | Execute a read-only SQL query (`SELECT`, `SHOW`, `DESCRIBE`, `EXPLAIN`, `WITH`) on the specified database. | Yes | Medium |
| `write_query` | Execute write operations (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, `ALTER`, etc.) on the specified database (active only if `enable_write=true`). | No | High |
| `explain_query` | Explain the execution plan of a SELECT query in the specified database. | Yes | Low |
| `suggest_query_plan` | Turn a non-technical reporting question into candidate tables, columns, joins, and filters without executing SQL. | Yes | Low |

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

## SQL Safety & Query Guidelines

To ensure the safety of your database and prevent data destruction, `octo-db` enforces strict security boundaries on the SQL queries the AI agent can execute.

### What is ALLOWED in `read_query`
- **Read-Only Statements**: `SELECT`, `SHOW`, `DESCRIBE`, and `EXPLAIN`.
- **Joins, Aggregations & Grouping**: Complex read-only analysis is fully supported:
  ```sql
  SELECT c.category_name, COUNT(p.id) as total_products, AVG(p.price) as avg_price
  FROM products p
  INNER JOIN categories c ON p.category_id = c.id
  WHERE p.status = 'active'
  GROUP BY c.category_name
  HAVING COUNT(p.id) > 5
  ORDER BY total_products DESC;
  ```
- **Subqueries (Nested Selects)**:
  ```sql
  SELECT email, username
  FROM users
  WHERE id IN (
      SELECT DISTINCT user_id 
      FROM orders 
      WHERE total_amount > 1000
  );
  ```
- **Read-Only CTEs (Common Table Expressions)**: You can use `WITH` clauses to structure complex queries:
  ```sql
  WITH monthly_sales AS (
      SELECT product_id, SUM(quantity) as total_sold
      FROM order_items
      GROUP BY product_id
  )
  SELECT p.name, ms.total_sold
  FROM products p
  JOIN monthly_sales ms ON p.id = ms.product_id;
  ```
- **Execution Plans**:
  ```sql
  EXPLAIN SELECT * FROM orders WHERE user_id = 123;
  ```

### What is BLOCKED in `read_query`
- **Mutating Keywords**: Any query containing statements like `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, `REPLACE`, `MERGE`, `UPSERT`, `GRANT`, `REVOKE`, `CALL`, `COPY`, `ATTACH`, `DETACH`, or `VACUUM` is immediately blocked.
- **Schema & DDL Changes**:
  ```sql
  -- THIS IS BLOCKED:
  ALTER TABLE users ADD COLUMN is_admin BOOLEAN DEFAULT FALSE;
  
  -- THIS IS BLOCKED:
  DROP TABLE audit_logs;
  ```
- **Mutating CTEs**: Write operations hidden inside `WITH` statements are strictly blocked:
  ```sql
  -- THIS IS BLOCKED:
  WITH deleted_users AS (
      DELETE FROM users WHERE last_login < '2025-01-01' RETURNING id
  )
  SELECT * FROM deleted_users;
  ```
- **Multi-Statement Queries**: Multiple SQL statements separated by semicolons are blocked to prevent SQL injection or stacked query attacks:
  ```sql
  -- THIS IS BLOCKED:
  SELECT * FROM users; DROP TABLE products;
  ```
- **Privilege & DB Administration Mutations**:
  ```sql
  -- THIS IS BLOCKED:
  GRANT ALL PRIVILEGES ON DATABASE appdb TO evil_user;
  
  -- THIS IS BLOCKED:
  VACUUM FULL;
  ```
- **SQLite Database Attachments**:
  ```sql
  -- THIS IS BLOCKED:
  ATTACH DATABASE '/etc/passwd' AS pwned;
  ```

### Active Guardrails
- **Query Length Constraints**: The server rejects any SQL query exceeding 65,536 characters to prevent Denial of Service (DoS) attacks or excessive memory usage.
- **Auto-LIMIT Injection**: For queries that query data (`SELECT` and `WITH ... SELECT`), `octo-db` detects if a top-level `LIMIT` clause is missing. If missing, it automatically appends `LIMIT <MaxRows>` (default: `500`) to safeguard database resources *before* execution.
- **Row Cap Enforcement**: Responses are still capped to `OCTO_DB_MAX_ROWS` (default: `500`) in the server output, ensuring the AI model context is not flooded.
- **Schema & Table Allowlists/Denylists**: Queries targeting schemas or tables listed in `OCTO_DB_DENIED_TABLES` (or not matching `OCTO_DB_ALLOWED_TABLES` / `OCTO_DB_ALLOWED_SCHEMAS`) are rejected prior to execution.

### Limitations of the SQL Validation System
- **No AST Parsing**: The validation system does not build a full SQL Abstract Syntax Tree (AST). It uses a lightweight, dependency-free tokenizer and regular expressions. Consequently, complex dialect-specific queries, functions, or deeply nested scopes may not be perfectly understood.
- **Heuristics for SQL Keywords**: Keywords used as identifiers (e.g., a column or table named `limit` without quotes) are evaluated using context-sensitive heuristics (such as parenthesis nesting depth and following tokens) to tell them apart from actual clauses.
- **Dialect Incompatibilities with Trailing Clauses**: Query clauses that must succeed `LIMIT` (such as `FOR UPDATE`, `FOR SHARE`, or `INTO`) are blocked or rejected when no limit is present, since appending a limit to the end would result in database syntax errors.
- **Procedural blocks and semicolons**: Semicolon checks are designed to restrict executions to a single SQL statement. Inline procedural code blocks containing semicolons may not be correctly validated. Always use a dedicated database user with strict read-only permissions for safety.

### Real-World Prompt-to-Query Workflows

Here is how natural language prompts from a user translate into sequential MCP tool calls and final SQL executions:

#### Scenario 1: Detecting Purchase Anomalies (Fraud Analysis)
- **User Prompt**:
  > *"Find users who spent over 200% more this month compared to their historical monthly average, and check what categories they bought."*
- **AI Tool Execution Chain**:
  1. **Identify connections**: The AI calls `list_relationships(table_name="orders")` and discovers that `orders` has a foreign key to `users` and `order_items` connects `orders` to `products`.
  2. **Verify columns**: The AI calls `find_columns(query="date")` and `find_columns(query="amount")` to confirm that dates are stored in `created_at` and amounts in `total_amount`.
  3. **Plan and verify performance**: The AI plans a query with window functions to compute averages. To ensure it won't crash the database, it calls `explain_query` on the SQL statement.
  4. **Fetch data safely**: Once the execution plan is confirmed to use indexes, the AI calls `read_query` to get the list of anomalies.
- **Resulting SQL Query executed by the MCP**:
  ```sql
  WITH user_stats AS (
      SELECT user_id, AVG(total_amount) as avg_historic
      FROM orders
      WHERE created_at < DATE_TRUNC('month', CURRENT_DATE)
      GROUP BY user_id
  )
  SELECT o.user_id, o.id as order_id, o.total_amount, us.avg_historic, p.category
  FROM orders o
  JOIN user_stats us ON o.user_id = us.user_id
  JOIN order_items oi ON o.id = oi.order_id
  JOIN products p ON oi.product_id = p.id
  WHERE o.created_at >= DATE_TRUNC('month', CURRENT_DATE)
    AND o.total_amount > (us.avg_historic * 3);
  ```

#### Scenario 2: Schema Inspection for Backend Code Generation
- **User Prompt**:
  > *"Check the database structure for our users table and write a Go struct representing it, along with a secure handler."*
- **AI Tool Execution Chain**:
  1. **Locate the table**: The AI calls `list_tables()` or `search_tables(query="user")` to find the exact name of the table (`users`).
  2. **Inspect the columns**: The AI calls `describe_table(table_name="users")` to get column types, nullability, and primary keys.
  3. **Generate code**: Using the JSON structure returned by the MCP, the AI writes the Go code and creates files locally.
- **MCP Output utilized by the AI**:
  ```json
  [
    {"column_name": "id", "data_type": "integer", "is_nullable": "NO", "column_default": "nextval('users_id_seq')"},
    {"column_name": "email", "data_type": "character varying", "is_nullable": "NO", "column_default": "null"},
    {"column_name": "created_at", "data_type": "timestamp without time zone", "is_nullable": "YES", "column_default": "now()"}
  ]
  ```

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

## Verify Installation

To verify that the MCP server is working correctly and properly registered by your client:

1. **Verify server discovery using Codex CLI (or equivalent tool):**
   ```bash
   codex mcp list
   ```
   *This command should list `octo-db` as one of the active, connected MCP servers.*

2. **Test functionality using diagnostic prompts in your MCP client:**
   
   - *Prompt 1:*
     ```text
     Use octo_db and list available tables.
     ```
   - *Prompt 2:*
     ```text
     Use octo_db and describe the users table.
     ```
   - *Prompt 3:*
     ```text
     Use octo_db and explain this SQL query.
     ```

## MCP Client Configuration

Here are example configurations to integrate the `octo-db` server with different Model Context Protocol clients.

### Wrapper Script

Using a wrapper script (like `run-octo-db.sh` for Linux/macOS or `run-octo-db.bat` for Windows) is highly recommended for security and ease of setup.

* **Why it is used**: MCP clients execute servers in their own runtime environments where setting shell variables can be difficult. The wrapper script sources a local, untracked `.env` file containing secrets, keeping them secure, and then starts the `octo-db` server.
* **Advantages**: It prevents sensitive database passwords and usernames from being stored directly in your MCP client configuration files (which are stored in plain text in app configuration directories and might be backed up/shared).
* **How to adapt it**:
  - **Linux/macOS**: Copy `run-octo-db.sh.example` to `run-octo-db.sh`, make it executable (`chmod +x run-octo-db.sh`), and update the absolute paths pointing to your local `.env` and `octo-db` binary.
  - **Windows**: Copy `run-octo-db.bat.example` to `run-octo-db.bat` and update the absolute paths pointing to your local `.env` file and `octo-db.exe` binary.

### Codex Configuration

Add the following to your Codex configuration file (typically in `.codex` configuration folder or `codex.toml`):

```toml
[mcp_servers.octo_db]
command = "/path/to/run-octo-db.sh"
args = []
```

### Claude Desktop Configuration

Add the following to your Claude Desktop configuration file (e.g., `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS or `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "octo_db": {
      "command": "/path/to/run-octo-db.sh"
    }
  }
}
```

### Cursor / Cline / Roo Code Configuration

Configure the server in the client's MCP configuration settings using a generic wrapper path:

```json
{
  "mcpServers": {
    "octo_db": {
      "command": "/path/to/run-octo-db.sh",
      "args": [],
      "disabled": false
    }
  }
}
```

*Note: For Cursor, you can also add this server visually through the settings UI by choosing type `command`, entering `octo_db` as name, and specifying the wrapper script `/path/to/run-octo-db.sh` as the command.*

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
go build -o dist/octo-db .
./dist/octo-db doctor
```

### Makefile Targets

You can also use the bundled `Makefile`:

```bash
make build
make build-all
make test
make vet
make checksums
make clean
```

What they do:

- `make build`: builds the local binary into `dist/octo-db`
- `make build-all`: cross-compiles the supported release targets
- `make checksums`: generates `dist/checksums.txt`
- `make clean`: removes `dist/`

Recommended manual smoke test after changes:

1. Run `./octo-db doctor`
2. Call `server_info`
3. Call `find_columns` with a known business term
4. Call `suggest_query_plan` with a non-technical question
5. Confirm `read_query` still enforces policy and row limits

CI is defined in [.github/workflows/ci.yml](.github/workflows/ci.yml) and release packaging in [.github/workflows/release.yml](.github/workflows/release.yml).

## Distribution

The distribution of `octo-db` binaries is fully automated. Whenever a new tag (matching `v*`) is pushed, the Release GitHub Action automatically compiles and bundles static binaries for major operating systems and architectures.

### Supported Platform Targets
- **Linux**: `amd64` and `arm64` (packaged as `.tar.gz` archives)
- **macOS**: `amd64` and `arm64` (packaged as `.tar.gz` archives)
- **Windows**: `amd64` (packaged as `.zip` archive)

### How to obtain and run release binaries
1. Navigate to the **Releases** page of the repository on GitHub.
2. Download the compressed archive matching your operating system and architecture.
3. Extract the downloaded archive:
   - For Linux/macOS: `tar -xzf octo-db-vX.Y.Z-goos-goarch.tar.gz`
   - For Windows: Extract the `.zip` file using your file manager or PowerShell.
4. (Optional but recommended) Verify release integrity by checking the SHA-256 checksums published alongside the release archives:
   ```bash
   sha256sum --check checksums-goos-goarch.txt
   ```
5. Run the binary inside the extracted folder:
   ```bash
   ./octo-db --version
   ```

## Release Checklist

- run `go test ./...`
- run `go vet ./...`
- run `make build-all`
- run `make checksums`
- verify `doctor` with a real local config
- review the examples in [examples/](examples/)
- review [CHANGELOG.md](CHANGELOG.md)
- tag a version like `v1.4.3`

## Changelog

Release notes live in [CHANGELOG.md](CHANGELOG.md).

## License

MIT. See [LICENSE](LICENSE).
