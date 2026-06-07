# Agent Instructions and Guidelines: `mcp-octo-db`

This repository follows strict guidelines for AI agents and coding assistants collaborating on the project.

## Project Rules & Constraints

### 1. Language Rule
* **All project text, including documentation, code comments, commit messages, roadmap files, and logs, must be written in English.**
* Do not generate documentation or files in other languages unless explicitly requested by the user.

### 2. Security & Safety Guards
* Always prioritize read-first safety policies.
* Ensure all SQL query execution paths have strict guards:
  * Maximum SQL query length checks.
  * Automatic `LIMIT` clause injection for `SELECT` and `WITH` statements.
  * Proper escaping of credentials in connection strings using `net/url`.
* Do not introduce direct write capabilities without validating user-explicit settings (`GlobalSettings.EnableWrite`).

### 3. Dependency Constraints
* Avoid introducing heavy or unnecessary external dependencies. Prefer Go standard library packages or lightweight, well-established libraries.
* Maintain the performance and minimal footprint of this server.
