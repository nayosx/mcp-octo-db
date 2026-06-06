# Changelog

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
