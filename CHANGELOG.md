# Changelog

## v1.4.4

- Refocused positioning in the README.md with clear real-world use cases.
- Added `agents.md` outlining project rules, including English-only documentation and no auto-commit guidelines.
- Configured `docs/` folder to be ignored in `.gitignore`.

## v1.4.3

- Hardened `read_query` with query length limits (max 65536 characters) to protect against DoS.
- Added automatic `LIMIT` clause detection and injection for `SELECT` and `WITH` queries lacking a top-level limit.
- Improved query validation parser/heuristics using a lightweight, dependency-free tokenizer.
- Hardened DSN construction for PostgreSQL and MySQL/MariaDB to safely escape usernames and passwords with special characters.
- Documented limitations of regex/tokenization-based SQL validation and the new safety features in the README.

## v1.4.0

- Added operational CLI flags for `--version`, `--list-tools`, and `--print-effective-config`.
- Added configurable DB pool settings and startup ping timeouts.
- Added richer tool logging with request identifiers and database context.
- Encapsulated runtime DB state in a `ServerState` structure.
- Improved `suggest_query_plan` with table-role scoring and stronger date/metric prioritization.

## v1.3.0

- Added `server_info`, `list_views`, and `list_indexes` MCP tools.
- Added `find_columns`, `list_relationships`, and `suggest_query_plan` for non-technical database discovery.
- Added ready-to-copy client config examples for Claude Desktop, Cursor, Codex-compatible clients, Cline, and Roo Code.
- Added a Dockerfile and release packaging workflow for tagged builds.

## v1.1.0

- Hardened SQL validation for `read_query`, `write_query`, and `explain_query`.
- Blocked mutating CTEs from read-only execution paths.
- Rejected multi-statement SQL in write and read paths.
- Enforced schema allowlists when queries reference `schema.table`.
- Added configuration normalization and validation with fail-fast errors.
- Added tool exposure checks and expanded unit tests.
- Added CI, release checklist, and clearer Community Edition security documentation.
