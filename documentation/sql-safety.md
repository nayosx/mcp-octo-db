# SQL Safety & Security Guidelines

This document details the security model, threat model, query validation rules, and active guardrails enforced by `octo-db`.

---

## Security Model / Threat Model

`octo-db` is built for **local development and test environments**. It provides guardrails, not hard isolation.

### What it does:
* Disables `write_query` by default.
* Blocks multi-statement SQL in read and write paths.
* Blocks mutating CTEs from read-only tools.
* Enforces `allowed_schemas`, `allowed_tables`, and `denied_tables`.
* Caps rows and applies timeouts.
* Keeps logs on `stderr` so the MCP `stdio` channel stays clean.

### What it does not do:
* Per-operation human approval workflows.
* Strong sandboxing against every SQL dialect trick.
* Row-level or tenant-level access isolation.
* Centralized policy management across many users.
* PII masking or enterprise-grade audit pipelines.

> [!IMPORTANT]
> Use `octo-db` as a developer-facing connector with safety rails, not as your only production control plane.

---

## Security Recommendations

To ensure safe operation of the MCP server, please follow these guidelines:
* **Keep `.env` files secured**: Never commit `.env` or configurations with real credentials to your repository. Ensure `.env` is listed in your `.gitignore` file.
* **Do not share credentials**: Never store plain secrets or database passwords in shared MCP client configuration files (e.g., `claude_desktop_config.json`).
* **Use read-only database users**: Create a dedicated database user for the MCP server that only has read permissions (`SELECT`) on the necessary tables and schemas.
* **Limit write privileges**: Keep `OCTO_DB_ENABLE_WRITE` set to `false` unless write capabilities are strictly necessary. If enabled, restrict the database user permissions to the minimal set of write privileges needed.
* **Review queries before execution**: Be cautious and verify query plans or suggest plans before executing raw SQL writes or complex reads that could lock tables or impact performance.

---

## SQL Safety & Query Guidelines

To ensure the safety of your database and prevent data destruction, `octo-db` enforces strict security boundaries on the SQL queries the AI agent can execute.

### What is ALLOWED in `read_query`
* **Read-Only Statements**: `SELECT`, `SHOW`, `DESCRIBE`, and `EXPLAIN`.
* **Joins, Aggregations & Grouping**: Complex read-only analysis is fully supported:
  ```sql
  SELECT c.category_name, COUNT(p.id) as total_products, AVG(p.price) as avg_price
  FROM products p
  INNER JOIN categories c ON p.category_id = c.id
  WHERE p.status = 'active'
  GROUP BY c.category_name
  HAVING COUNT(p.id) > 5
  ORDER BY total_products DESC;
  ```
* **Subqueries (Nested Selects)**:
  ```sql
  SELECT email, username
  FROM users
  WHERE id IN (
      SELECT DISTINCT user_id 
      FROM orders 
      WHERE total_amount > 1000
  );
  ```
* **Read-Only CTEs (Common Table Expressions)**: You can use `WITH` clauses to structure complex queries:
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
* **Execution Plans**:
  ```sql
  EXPLAIN SELECT * FROM orders WHERE user_id = 123;
  ```

### What is BLOCKED in `read_query`
* **Mutating Keywords**: Any query containing statements like `INSERT`, `UPDATE`, `DELETE`, `DROP`, `ALTER`, `CREATE`, `TRUNCATE`, `REPLACE`, `MERGE`, `UPSERT`, `GRANT`, `REVOKE`, `CALL`, `COPY`, `ATTACH`, `DETACH`, or `VACUUM` is immediately blocked.
* **Schema & DDL Changes**:
  ```sql
  -- THIS IS BLOCKED:
  ALTER TABLE users ADD COLUMN is_admin BOOLEAN DEFAULT FALSE;
  
  -- THIS IS BLOCKED:
  DROP TABLE audit_logs;
  ```
* **Mutating CTEs**: Write operations hidden inside `WITH` statements are strictly blocked:
  ```sql
  -- THIS IS BLOCKED:
  WITH deleted_users AS (
      DELETE FROM users WHERE last_login < '2025-01-01' RETURNING id
  )
  SELECT * FROM deleted_users;
  ```
* **Multi-Statement Queries**: Multiple SQL statements separated by semicolons are blocked to prevent SQL injection or stacked query attacks:
  ```sql
  -- THIS IS BLOCKED:
  SELECT * FROM users; DROP TABLE products;
  ```
* **Privilege & DB Administration Mutations**:
  ```sql
  -- THIS IS BLOCKED:
  GRANT ALL PRIVILEGES ON DATABASE appdb TO evil_user;
  
  -- THIS IS BLOCKED:
  VACUUM FULL;
  ```
* **SQLite Database Attachments**:
  ```sql
  -- THIS IS BLOCKED:
  ATTACH DATABASE '/etc/passwd' AS pwned;
  ```

---

## Active Guardrails

* **Query Length Constraints**: The server rejects any SQL query exceeding 65,536 characters to prevent Denial of Service (DoS) attacks or excessive memory usage.
* **Auto-LIMIT Injection**: For queries that query data (`SELECT` and `WITH ... SELECT`), `octo-db` detects if a top-level `LIMIT` clause is missing. If missing, it automatically appends `LIMIT <MaxRows>` (default: `500`) to safeguard database resources *before* execution.
* **Row Cap Enforcement**: Responses are still capped to `OCTO_DB_MAX_ROWS` (default: `500`) in the server output, ensuring the AI model context is not flooded.
* **Schema & Table Allowlists/Denylists**: Queries targeting schemas or tables listed in `OCTO_DB_DENIED_TABLES` (or not matching `OCTO_DB_ALLOWED_TABLES` / `OCTO_DB_ALLOWED_SCHEMAS`) are rejected prior to execution.

---

## Limitations of the SQL Validation System

* **No AST Parsing**: The validation system does not build a full SQL Abstract Syntax Tree (AST). It uses a lightweight, dependency-free tokenizer and regular expressions. Consequently, complex dialect-specific queries, functions, or deeply nested scopes may not be perfectly understood.
* **Heuristics for SQL Keywords**: Keywords used as identifiers (e.g., a column or table named `limit` without quotes) are evaluated using context-sensitive heuristics (such as parenthesis nesting depth and following tokens) to tell them apart from actual clauses.
* **Dialect Incompatibilities with Trailing Clauses**: Query clauses that must succeed `LIMIT` (such as `FOR UPDATE`, `FOR SHARE`, or `INTO`) are blocked or rejected when no limit is present, since appending a limit to the end would result in database syntax errors.
* **Procedural blocks and semicolons**: Semicolon checks are designed to restrict executions to a single SQL statement. Inline procedural code blocks containing semicolons may not be correctly validated. Always use a dedicated database user with strict read-only permissions for safety.

---

## Real-World Prompt-to-Query Workflows

Here is how natural language prompts from a user translate into sequential MCP tool calls and final SQL executions:

### Scenario 1: Detecting Purchase Anomalies (Fraud Analysis)
1. **User Prompt**: 
   > *"Find users who spent over 200% more this month compared to their historical monthly average, and check what categories they bought."*
2. **AI Tool Execution Chain**:
   * **Identify connections**: The AI calls `list_relationships(table_name="orders")` and discovers that `orders` has a foreign key to `users` and `order_items` connects `orders` to `products`.
   * **Verify columns**: The AI calls `find_columns(query="date")` and `find_columns(query="amount")` to confirm that dates are stored in `created_at` and amounts in `total_amount`.
   * **Plan and verify performance**: The AI plans a query with window functions to compute averages. To ensure it won't crash the database, it calls `explain_query` on the SQL statement.
   * **Fetch data safely**: Once the execution plan is confirmed to use indexes, the AI calls `read_query` to get the list of anomalies.
3. **Resulting SQL Query executed by the MCP**:
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

### Scenario 2: Schema Inspection for Backend Code Generation
1. **User Prompt**:
   > *"Check the database structure for our users table and write a Go struct representing it, along with a secure handler."*
2. **AI Tool Execution Chain**:
   * **Locate the table**: The AI calls `list_tables()` or `search_tables(query="user")` to find the exact name of the table (`users`).
   * **Inspect the columns**: The AI calls `describe_table(table_name="users")` to get column types, nullability, and primary keys.
   * **Generate code**: Using the JSON structure returned by the MCP, the AI writes the Go code and creates files locally.
3. **MCP Output utilized by the AI**:
   ```json
   [
     {"column_name": "id", "data_type": "integer", "is_nullable": "NO", "column_default": "nextval('users_id_seq')"},
     {"column_name": "email", "data_type": "character varying", "is_nullable": "NO", "column_default": "null"},
     {"column_name": "created_at", "data_type": "timestamp without time zone", "is_nullable": "YES", "column_default": "now()"}
   ]
   ```
