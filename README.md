# mcp-octo-db (Community Edition)

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

`mcp-octo-db` is a **Model Context Protocol (MCP)** server written in Go and distributed under the open-source MIT license. It allows you to connect AI assistants (such as Cursor, Claude Desktop, etc.) to multiple local or test relational databases simultaneously, supporting **PostgreSQL**, **MySQL**, **MariaDB**, and **SQLite** out of the box.

The name `octo` (octopus) refers to its ability to extend tentacles to different engines and schemas using a centralized configuration.

> [!NOTE]
> **Community Edition Positioning:**
> This public repository serves as an open-source tool and local connector for developers. It is designed to simplify AI integration in local and test environments. If your organization requires a centralized access control layer, advanced audit policies (SIEM/Loki), sensitive data masking (PII), human approval workflows, or multi-tenant deployment, please refer to the documentation for the Enterprise edition.

---

## Features (Community Edition)

- 🔌 **Multi-Engine Support:** Connect to PostgreSQL, MySQL, MariaDB, and SQLite databases using the same server.
- 🗃️ **Simultaneous Multi-DB:** Map multiple databases using YAML files or environment variables.
- 🛡️ **Security by Default:**
  - Read-only mode by default (write operations must be explicitly enabled with `enable_write: true`).
  - Block destructive/DML queries (such as `DROP`, `DELETE`, `UPDATE`) in the read channel and protect against multiple SQL statements in a single call.
  - Support for table and schema exclusion/inclusion rules (`allowed_schemas`, `allowed_tables`, `denied_tables`).
  - Automatic row capping (`max_rows`) and timeouts (`query_timeout_seconds`) to optimize resource and token consumption.
- ⚙️ **Flexible Configuration & Diagnostics:** Supports `.env` files and YAML files with clear precedence. Includes the `doctor` command for quick local connectivity testing.
- 🛠️ **Expanded MCP Tools:** Introspection tools (`list_tables`, `describe_table`), structured query tools (`read_query`, `write_query`), and new navigation utilities (`list_schemas`, `search_tables`, `get_table_sample`, `explain_query`).

---

## Requirements and Test Environment

- [Go](https://go.dev/) (Version 1.22 or higher)
- Access to the databases you want to query (Postgres, MySQL, MariaDB).

### 🐳 Quick Development Environment (Postgres in Docker)
The repository includes a preconfigured `docker-compose.yml` file. To spin up a local PostgreSQL instance for testing with the same default credentials from `.env.example`, run:

```bash
docker compose up -d
```

This will start a Postgres container on port `5432` with the database `appdb` and user `appuser`.

---

## Installation and Build

1. Clone the repository:
   ```bash
   git clone https://github.com/nayosx/mcp-octo-db.git
   cd mcp-octo-db
   ```

2. Download and tidy Go dependencies:
   ```bash
   go mod tidy
   ```

3. Compile the binary:
   ```bash
   go build -o mcp-octo-db
   ```

This will generate the `mcp-octo-db` executable (or `mcp-octo-db.exe` on Windows) in the root directory.

---

## Configuration (`.env`)

Create a `.env` file in the root of the project. You can use the `.env.example` file as a template:

```bash
cp .env.example .env
```

### Configure the Main Database (`default`):
```env
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_USER=appuser
DB_PASSWORD=my_password
DB_NAME=appdb
DB_SSLMODE=disable
```

### Configure Additional Databases (e.g., `analytics`):
To add additional databases, append variables with a suffix of your choice (e.g., `_ANALYTICS`):
```env
DB_TYPE_ANALYTICS=mysql
DB_HOST_ANALYTICS=127.0.0.1
DB_PORT_ANALYTICS=3306
DB_USER_ANALYTICS=analytics_user
DB_PASSWORD_ANALYTICS=analytics_pass
DB_NAME_ANALYTICS=analyticsdb
```
The server will automatically detect the suffix and register the database under the name `analytics` (in lowercase).

### Configure SQLite Databases (e.g., `sqlite_test`):
For SQLite databases, the `DB_NAME` field represents the path to the `.db` file. No host, port, user, or password fields are required:
```env
DB_TYPE_SQLITE_TEST=sqlite
DB_NAME_SQLITE_TEST=/home/ness/Development/go/mcp_octo_db/test.db
```
The server will register it under the name `sqlite_test`.

---

## Advanced Configuration (`config.yaml`)

In addition to environment variables, you can configure the server using a YAML configuration file. This is ideal for clearly managing multiple databases and security controls (allowlists, denylists, row limits, and timeouts).

Create a `config.yaml` file based on the provided example:
```bash
cp config.yaml.example config.yaml
```

The file format allows you to define:
- **databases**: A list of relational database connections (`postgres`, `mysql`, `sqlite`).
- **settings**:
  - `enable_write`: If set to `true`, enables the `write_query` tool (default is `false` for safety).
  - `max_rows`: Maximum number of rows returned in queries (default is `500`).
  - `query_timeout_seconds`: Execution time limit for queries in seconds (default is `10`).
  - `allowed_schemas`: List of allowed schemas (e.g., `[public]`).
  - `allowed_tables`: List of allowed tables (e.g., `[users, products]`).
  - `denied_tables`: List of restricted tables (e.g., `[secrets, passwords]`).
  - `log_format`: Log format (`text` or `json` for structured logging).

---

## CLI Flags and Diagnostics (`doctor`)

The `mcp-octo-db` executable supports configuration flags and diagnostic subcommands:

1. **Specify a `.env` file:**
   ```bash
   ./mcp-octo-db --env /path/to/my/.env
   ```

2. **Specify a `config.yaml` file:**
   ```bash
   ./mcp-octo-db --config /path/to/my/config.yaml
   ```

3. **Run Diagnostics (`doctor`):**
   Validates the connection to all databases and checks active security policies without exposing secrets:
   ```bash
   ./mcp-octo-db [--config config.yaml] [--env .env] doctor
   ```

---

## Integration with MCP Clients

> [!NOTE]
> **Open Standard:** Since the Model Context Protocol (MCP) is an open standard developed to unify the connection between AI and tools, this server is compatible with **any client or environment that supports MCP** (Cursor, Claude Desktop, Antigravity, OpenCode, Codex, Cline, Roo Code, etc.).

The server communicates via the standard input/output transport (`stdio`). Configuration details for the most common clients are listed below:

### 1. Claude Desktop
Edit the `claude_desktop_config.json` configuration file:
- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux:** `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mcp-octo-db": {
      "command": "/home/ness/Development/go/mcp_octo_db/mcp-octo-db",
      "args": [],
      "cwd": "/home/ness/Development/go/mcp_octo_db"
    }
  }
}
```

### 2. Antigravity (agy)
In your `antigravity` configuration environment, add the MCP server to your global or workspace settings using the same JSON format:
```json
"mcpServers": {
  "mcp-octo-db": {
    "command": "/home/ness/Development/go/mcp_octo_db/mcp-octo-db",
    "cwd": "/home/ness/Development/go/mcp_octo_db"
  }
}
```

### 3. Codex / OpenCode
For integrations based on VS Code extensions or IDEs compatible with Codex and OpenCode:
1. Install the MCP client plugin (e.g., Cline, Roo Code, or the native Codex/OpenCode client).
2. Add a new `mcp-octo-db` server configuration:
   - **Command:** `/home/ness/Development/go/mcp_octo_db/mcp-octo-db`
   - **Cwd:** `/home/ness/Development/go/mcp_octo_db`
   - **Type:** `stdio` or `command`

### 4. Cursor
Go to **Settings > Features > MCP**, click **+ Add New MCP Server** and configure it as follows:
- **Name:** `mcp-octo-db`
- **Type:** `command`
- **Command:** `/home/ness/Development/go/mcp_octo_db/mcp-octo-db`
- **Cwd (optional):** `/home/ness/Development/go/mcp_octo_db` (required to load `.env` correctly).

---

## Exposed Tools

The server exposes **eight core tools** to the AI model, categorized by their purpose:

### 1. Schema Introspection & Exploration

* **`list_schemas`**: Lists all schemas or catalogs in a specified database.
  - **Arguments:**
    - `db_name` (optional, string): Name of the database (defaults to `default`).

* **`list_tables`**: Lists all tables in a specified database and schema.
  - **Arguments:**
    - `db_name` (optional, string): Name of the database (defaults to `default`).
    - `schema` (optional, string): Schema to query (for Postgres defaults to `public`, for MySQL/MariaDB defaults to the current database name).

* **`search_tables`**: Searches for tables matching a query pattern (performs a `LIKE` search).
  - **Arguments:**
    - `query` (required, string): Pattern to search for in table names (e.g., `%users%`).
    - `db_name` (optional, string): Name of the database (defaults to `default`).
    - `schema` (optional, string): Schema of the table.

* **`describe_table`**: Shows the structure of a specific table, detailing columns, types, nullability, primary keys, and default values.
  - **Arguments:**
    - `table_name` (required, string): Name of the table to describe.
    - `db_name` (optional, string): Name of the database (defaults to `default`).
    - `schema` (optional, string): Schema of the table.

### 2. Querying & Data Sampling

* **`get_table_sample`**: Retrieves a quick sample of rows from a selected table. Useful for inspecting the actual data format without writing manual SQL.
  - **Arguments:**
    - `table_name` (required, string): Name of the table.
    - `db_name` (optional, string): Name of the database (defaults to `default`).
    - `schema` (optional, string): Schema of the table.
    - `limit` (optional, int): Number of rows to retrieve (default is `10`, max is `100`).

* **`read_query`**: Safely executes a read-only SQL query and returns the results in structured JSON format.
  - **Restrictions:** Only allows `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, and `EXPLAIN` statements. Protects against multiple SQL statements in a single call (SQL injection prevention) and validates against security lists.
  - **Arguments:**
    - `sql` (required, string): The SQL query to execute.
    - `db_name` (optional, string): Name of the database (defaults to `default`).

### 3. Optimization & Execution Plan

* **`explain_query`**: Obtains the execution plan of a `SELECT` query to analyze its performance (uses `EXPLAIN` in Postgres/MySQL and `EXPLAIN QUERY PLAN` in SQLite).
  - **Arguments:**
    - `sql` (required, string): The `SELECT` or `WITH` query to explain.
    - `db_name` (optional, string): Name of the database (defaults to `default`).

### 4. Data & Structure Modification

* **`write_query`**: Executes queries that modify data or structure.
  - **Restrictions:** Only active if `enable_write` is set to `true` in the global settings.
  - **Arguments:**
    - `sql` (required, string): The SQL query to execute (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, `ALTER`, etc.).
    - `db_name` (optional, string): Name of the database (defaults to `default`).

---

## Advanced Use Cases & Capabilities

Thanks to the integration of Model Context Protocol (MCP) and this server, AI assistants can interact with databases autonomously but under safe bounds to execute advanced development and administration tasks:

### 📊 1. Query Performance Auditing & Analysis
When you notice a query is slow, you can ask the assistant to examine its execution plan and suggest optimizations.
- **Example Prompt:** *"Explain the execution plan for the query that searches for users by email in the 'default' database, and let me know if it needs an index."*
- The assistant will call `explain_query` with the specified SQL. Upon receiving the plan (detecting a `Seq Scan` in PostgreSQL or a `SCAN TABLE` in SQLite), it will propose the correct index. If write mode is active, it can execute the `CREATE INDEX` statement directly via `write_query`.

### 🔍 2. Reverse Engineering & Documenting Unknown Schemas
If you connect the server to a legacy database or one without up-to-date documentation, the AI can explore it for you.
- **Example Prompt:** *"Explore the analytics database, find tables related to users and billing, and generate a schema description along with a sample of data."*
- The assistant will sequentially use `list_tables` or `search_tables` to find key tables, `describe_table` to inspect data types and primary/foreign keys, and `get_table_sample` to view actual data. Finally, it will generate a clean Markdown report with ER diagrams.

### 📈 3. Generating Complex Analytical Reports
You can perform data analysis by asking for specific aggregations and summaries:
- **Example Prompt:** *"Generate a report of the top 5 best-selling products in the last month using a query involving CTEs (WITH) and joins on the 'analytics' database."*
- Thanks to Common Table Expressions (CTE/WITH) support, the AI can structure robust SQL queries and run them through `read_query`. The server automatically applies row limits (`max_rows`) and execution time limits (`query_timeout_seconds`) to protect database health and avoid excessive token consumption.

### 💾 4. Structure Migration & Data Seeding (Write Mode)
If you are developing a new feature and need to create test data structures:
- **Example Prompt:** *"Create a table named 'audit_logs' with columns id (UUID), action (VARCHAR), and created_at (TIMESTAMP). Then insert 3 test rows."*
- If `enable_write: true` is configured in your `config.yaml` or `.env` (`MCP_ENABLE_WRITE=true`), the assistant will call `write_query` to send the `CREATE TABLE` statement followed by the corresponding `INSERT` queries, validating at all times that the affected tables are not in the blocklist (`denied_tables`).

---

## Fault Tolerance and High Availability at Startup

One of the most critical features of the current version is its **resilience during startup**.

### Behavior in previous versions:
Previously, if a single configured database failed to connect (due to network issues, incorrect passwords, an unreachable host, etc.), the server executed a `log.Fatalf` and aborted immediately. This broke communication over the standard input/output transport (`stdio`) and caused the entire MCP environment to crash for the client (like Claude Desktop or Cursor), blocking interaction with the remaining healthy databases.

### Current behavior:
- **Fault-Tolerant Startup:** If a configured database is unavailable at startup, the server writes a detailed warning to the diagnostic error channel (`os.Stderr`) and continues connecting to the other databases. The MCP server starts and remains online without interruption.
- **Hot-Error Reporting:** If the assistant tries to interact with a database that failed to connect, the server does not crash. Instead, the validator intercepts the call and returns a detailed explanatory message to the assistant:
  ```json
  "database 'my_db' is offline. Connection failed on startup: connection timeout on port 5432"
  ```
- **Quick Diagnostics:** You can verify the actual connectivity status of all your databases at any time by running the local diagnostic command:
  ```bash
  ./mcp-octo-db doctor
  ```

---
---

# Documentación en Español (Spanish Documentation)

## mcp-octo-db (Community Edition)

`mcp-octo-db` es un servidor del **Model Context Protocol (MCP)** desarrollado en Go y distribuido bajo la licencia de código abierto MIT. Permite conectar asistentes de Inteligencia Artificial (como Cursor, Claude Desktop, etc.) a múltiples bases de datos relacionales locales o de prueba de forma simultánea, soportando indistintamente **PostgreSQL**, **MySQL**, **MariaDB** y **SQLite**.

El nombre `octo` (pulpo) hace referencia a su capacidad para extender tentáculos a diferentes motores y esquemas utilizando una configuración centralizada.

> [!NOTE]
> **Posicionamiento de la Community Edition:**
> Este repositorio público funciona como una herramienta y conector local de código abierto para desarrolladores. Está diseñado para simplificar la integración de IA en entornos locales y de prueba. Si tu organización requiere una capa de control de accesos centralizada, políticas de auditoría avanzadas (SIEM/Loki), enmascaramiento de datos sensibles (PII), flujos de aprobación humana (Approval Workflows) o despliegue multi-tenant, consulta la documentación sobre la versión Enterprise.

---

## Características (Community Edition)

- 🔌 **Soporte Multimotor:** Conéctate a bases de datos PostgreSQL, MySQL, MariaDB y SQLite usando el mismo servidor.
- 🗃️ **Multi-DB en Simultáneo:** Mapea múltiples bases de datos usando archivos YAML o variables de entorno.
- 🛡️ **Seguridad por Defecto:**
  - Modo de solo lectura por defecto (las escrituras se habilitan explícitamente con `enable_write: true`).
  - Bloqueo de consultas destructivas/DML (como `DROP`, `DELETE`, `UPDATE`) en el canal de lectura y protección contra múltiples sentencias SQL en un mismo llamado.
  - Soporte para reglas de exclusión/inclusión de tablas y esquemas (`allowed_schemas`, `allowed_tables`, `denied_tables`).
  - Capping automático de filas (`max_rows`) y timeouts (`query_timeout_seconds`) para optimizar el consumo de tokens y recursos.
- ⚙️ **Configuración Flexible y Diagnósticos:** Soporta archivos `.env` y archivos YAML con precedencia clara. Incluye el comando `doctor` para pruebas rápidas de conectividad local.
- 🛠️ **Herramientas MCP Ampliadas:** Herramientas de introspección (`list_tables`, `describe_table`), consultas estructuradas (`read_query`, `write_query`), y nuevas utilidades de navegación (`list_schemas`, `search_tables`, `get_table_sample`, `explain_query`).

---

## Requisitos y Entorno de Prueba

- [Go](https://go.dev/) (Versión 1.22 o superior)
- Acceso a las bases de datos que deseas consultar (Postgres, MySQL, MariaDB).

### 🐳 Entorno de Desarrollo Rápido (Postgres en Docker)
El repositorio incluye un archivo `docker-compose.yml` preconfigurado. Para levantar una instancia local de PostgreSQL para pruebas con las mismas credenciales predeterminadas de `.env.example`, ejecuta:

```bash
docker compose up -d
```

Esto iniciará un contenedor de Postgres en el puerto `5432` con la base de datos `appdb` y el usuario `appuser`.

---

## Instalación y Construcción

1. Clona el repositorio:
   ```bash
   git clone https://github.com/nayosx/mcp-octo-db.git
   cd mcp-octo-db
   ```

2. Descarga y limpia las dependencias de Go:
   ```bash
   go mod tidy
   ```

3. Compila el binario:
   ```bash
   go build -o mcp-octo-db
   ```

Esto generará el ejecutable `mcp-octo-db` (o `mcp-octo-db.exe` en Windows) en el directorio raíz.

---

## Configuración (`.env`)

Crea un archivo `.env` en la raíz del proyecto. Puedes tomar como base el archivo `.env.example`:

```bash
cp .env.example .env
```

### Configurar la Base Principal (`default`):
```env
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_USER=appuser
DB_PASSWORD=mi_contraseña
DB_NAME=appdb
DB_SSLMODE=disable
```

### Configurar Bases Adicionales (Ej: `analytics`):
Para añadir bases de datos adicionales, agrega variables con un sufijo de tu elección (ej. `_ANALYTICS`):
```env
DB_TYPE_ANALYTICS=mysql
DB_HOST_ANALYTICS=127.0.0.1
DB_PORT_ANALYTICS=3306
DB_USER_ANALYTICS=analytics_user
DB_PASSWORD_ANALYTICS=analytics_pass
DB_NAME_ANALYTICS=analyticsdb
```
El servidor detectará de manera automática el sufijo y registrará la base de datos bajo el nombre `analytics` (en minúsculas).

### Configurar Bases SQLite (Ej: `sqlite_test`):
Para bases de datos SQLite, el campo `DB_NAME` representa la ruta al archivo `.db`. No se requieren campos de host, puerto, usuario o contraseña:
```env
DB_TYPE_SQLITE_TEST=sqlite
DB_NAME_SQLITE_TEST=/home/ness/Development/go/mcp_octo_db/test.db
```
El servidor la registrará bajo el nombre `sqlite_test`.

---

## Configuración Avanzada (`config.yaml`)

Además de las variables de entorno, puedes configurar el servidor usando un archivo de configuración YAML. Esto es ideal para gestionar de forma clara múltiples bases de datos y controles de seguridad (allowlists, denylists, límites de filas y timeouts).

Crea un archivo `config.yaml` tomando como base el ejemplo provisto:
```bash
cp config.yaml.example config.yaml
```

El formato del archivo permite definir:
- **databases**: Un listado de conexiones a bases de datos relacionales (`postgres`, `mysql`, `sqlite`).
- **settings**:
  - `enable_write`: Si está en `true`, habilita la herramienta `write_query` (por defecto `false` para mayor seguridad).
  - `max_rows`: Límite máximo de filas devueltas en consultas (por defecto `500`).
  - `query_timeout_seconds`: Tiempo límite de ejecución de consulta en segundos (por defecto `10`).
  - `allowed_schemas`: Lista de esquemas permitidos (ej. `[public]`).
  - `allowed_tables`: Lista de tablas permitidas (ej. `[users, products]`).
  - `denied_tables`: Lista de tablas restringidas (ej. `[secrets, passwords]`).
  - `log_format`: Formato de logs (`text` o `json` para logs estructurados).

---

## Banderas de CLI y Diagnóstico (`doctor`)

El ejecutable `mcp-octo-db` admite banderas de configuración y subcomandos de diagnóstico:

1. **Especificar archivo `.env`:**
   ```bash
   ./mcp-octo-db --env /ruta/a/mi/.env
   ```

2. **Especificar archivo `config.yaml`:**
   ```bash
   ./mcp-octo-db --config /ruta/a/mi/config.yaml
   ```

3. **Ejecutar Diagnóstico (`doctor`):**
   Valida la conexión a todas las bases de datos y comprueba las políticas de seguridad activas sin exponer secretos:
   ```bash
   ./mcp-octo-db [--config config.yaml] [--env .env] doctor
   ```

---

## Integración con Clientes MCP

> [!NOTE]
> **Estándar Abierto:** Dado que Model Context Protocol (MCP) es un estándar abierto desarrollado para unificar la conexión entre IA y herramientas, este servidor es compatible con **cualquier cliente o entorno que soporte MCP** (Cursor, Claude Desktop, Antigravity, OpenCode, Codex, Cline, Roo Code, etc.).

El servidor se comunica a través del transporte de entrada/salida estándar (`stdio`). A continuación se detallan las configuraciones para los clientes más comunes:

### 1. Claude Desktop
Edita el archivo de configuración `claude_desktop_config.json`:
- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux:** `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "mcp-octo-db": {
      "command": "/home/ness/Development/go/mcp_octo_db/mcp-octo-db",
      "args": [],
      "cwd": "/home/ness/Development/go/mcp_octo_db"
    }
  }
}
```

### 2. Antigravity (agy)
En tu entorno de configuración de `antigravity`, añade el servidor MCP en tus ajustes globales o del workspace usando el mismo formato JSON:
```json
"mcpServers": {
  "mcp-octo-db": {
    "command": "/home/ness/Development/go/mcp_octo_db/mcp-octo-db",
    "cwd": "/home/ness/Development/go/mcp_octo_db"
  }
}
```

### 3. Codex / OpenCode
Para integraciones basadas en extensiones de VS Code o IDEs compatibles con Codex y OpenCode:
1. Instala el plugin cliente MCP (ej. Cline, Roo Code, o el cliente nativo de Codex/OpenCode).
2. Agrega una nueva configuración de servidor `mcp-octo-db`:
   - **Command:** `/home/ness/Development/go/mcp_octo_db/mcp-octo-db`
   - **Cwd:** `/home/ness/Development/go/mcp_octo_db`
   - **Type:** `stdio` o `command`

### 4. Cursor
Ve a **Settings > Features > MCP**, haz clic en **+ Add New MCP Server** y configúralo de la siguiente manera:
- **Name:** `mcp-octo-db`
- **Type:** `command`
- **Command:** `/home/ness/Development/go/mcp_octo_db/mcp-octo-db`
- **Cwd (opcional):** `/home/ness/Development/go/mcp_octo_db` (requerido para cargar el `.env` correctamente).

---

## Herramientas Expuestas

El servidor expone **ocho herramientas** principales al modelo de IA, clasificadas por su propósito:

### 1. Introspección y Exploración del Esquema

* **`list_schemas`**: Lista todos los esquemas o catálogos en una base de datos específica.
  - **Argumentos:**
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).

* **`list_tables`**: Lista todas las tablas en una base de datos y esquema específicos.
  - **Argumentos:**
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
    - `schema` (opcional, string): Esquema a consultar (por defecto `public` en Postgres, o el nombre de la BD actual en MySQL).

* **`search_tables`**: Busca tablas cuyos nombres coincidan con un patrón de búsqueda (búsqueda tipo `LIKE`).
  - **Argumentos:**
    - `query` (requerido, string): Patrón a buscar en los nombres de las tablas (ej. `%users%`).
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
    - `schema` (opcional, string): Esquema de la tabla.

* **`describe_table`**: Muestra la estructura de columnas, tipos, si aceptan nulos, llaves primarias y valores por defecto de una tabla.
  - **Argumentos:**
    - `table_name` (requerido, string): Nombre de la tabla a describir.
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
    - `schema` (opcional, string): Esquema de la tabla.

### 2. Consultas y Muestreo de Datos

* **`get_table_sample`**: Obtiene una muestra rápida de filas de una tabla seleccionada. Excelente para ver el formato real de los datos sin escribir SQL manualmente.
  - **Argumentos:**
    - `table_name` (requerido, string): Nombre de la tabla.
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
    - `schema` (opcional, string): Esquema de la tabla.
    - `limit` (opcional, int): Número de filas a recuperar (por defecto `10`, máximo `100`).

* **`read_query`**: Ejecuta de manera segura una consulta SQL de solo lectura y devuelve los resultados en formato JSON.
  - **Restricciones:** Solo permite sentencias `SELECT`, `WITH`, `SHOW`, `DESCRIBE` y `EXPLAIN`. Protege contra múltiples sentencias SQL en un mismo llamado (prevención de inyección SQL) y valida contra listas de seguridad.
  - **Argumentos:**
    - `sql` (requerido, string): La consulta SQL a ejecutar.
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).

### 3. Optimización y Plan de Ejecución

* **`explain_query`**: Obtiene el plan de ejecución de una consulta `SELECT` para analizar su rendimiento (usa `EXPLAIN` en Postgres/MySQL y `EXPLAIN QUERY PLAN` en SQLite).
  - **Argumentos:**
    - `sql` (requerido, string): La consulta `SELECT` o `WITH` a explicar.
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).

### 4. Modificación de Datos y Estructura

* **`write_query`**: Ejecuta consultas de modificación de datos o estructura.
  - **Restricciones:** Solo está activa si `enable_write` está en `true` en la configuración global.
  - **Argumentos:**
    - `sql` (requerido, string): La consulta SQL a ejecutar (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, `ALTER`, etc.).
    - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).

---

## Casos de Uso Avanzados y Qué se Puede Hacer

Gracias a la integración del Model Context Protocol (MCP) y de este servidor, los asistentes de Inteligencia Artificial pueden interactuar de forma autónoma pero controlada con las bases de datos para realizar tareas avanzadas de desarrollo y administración:

### 📊 1. Auditoría y Análisis del Rendimiento de Consultas
Cuando notas que una consulta es lenta, puedes pedirle al asistente que examine el plan de ejecución para sugerir optimizaciones.
- **Ejemplo de Instrucción:** *"Explica el plan de ejecución de la consulta que busca usuarios por email en la base 'default' y dime si requiere un índice."*
- El asistente llamará a `explain_query` con el SQL especificado. Tras recibir el plan (detectando un `Seq Scan` en PostgreSQL o un `SCAN TABLE` en SQLite), propondrá la creación del índice adecuado. Si el modo de escritura está activo, podrá ejecutar la sentencia `CREATE INDEX` a través de `write_query` de forma directa.

### 🔍 2. Ingeniería Inversa y Documentación de Esquemas Desconocidos
Si conectas el servidor a una base de datos antigua o de la cual no posees documentación actualizada, la IA puede explorarla por ti.
- **Ejemplo de Instrucción:** *"Explora la base de datos de analítica, busca las tablas relacionadas con usuarios y facturación, y genera una descripción del esquema junto con una muestra de datos."*
- El asistente usará secuencialmente `list_tables` o `search_tables` para encontrar tablas clave, `describe_table` para ver tipos de datos y llaves primarias/foráneas, y `get_table_sample` para examinar el formato real de los datos. Al final, generará un reporte limpio en Markdown con diagramas de relación.

### 📈 3. Generación de Reportes Analíticos Complejos
Puedes realizar análisis de datos pidiendo agregaciones y resúmenes específicos:
- **Ejemplo de Instrucción:** *"Genera un reporte del top 5 de productos más vendidos en el último mes usando una consulta que involucre CTEs (WITH) y joins en la base de datos 'analytics'."*
- Gracias al soporte de expresiones comunes de tabla (CTE/WITH), la IA puede estructurar consultas SQL robustas y enviarlas a través de `read_query`. El servidor aplicará automáticamente el límite de filas (`max_rows`) y el tiempo máximo de ejecución (`query_timeout_seconds`) para proteger la salud de la base de datos y evitar consumir excesivos tokens.

### 💾 4. Migración de Estructura y Poblado de Datos (Modo Escritura)
Si estás desarrollando una nueva funcionalidad y necesitas crear nuevas estructuras de datos de prueba:
- **Ejemplo de Instrucción:** *"Crea una tabla llamada 'audit_logs' con columnas id (UUID), action (VARCHAR), y created_at (TIMESTAMP). Luego inserta 3 filas de prueba."*
- Si `enable_write: true` está configurado en tu `config.yaml` o `.env` (`MCP_ENABLE_WRITE=true`), el asistente llamará a `write_query` enviando la sentencia `CREATE TABLE` y posteriormente el query `INSERT` correspondiente, validando en todo momento que las tablas afectadas no estén en la lista de denegación (`denied_tables`).

---

## Tolerancia a Fallos y Alta Disponibilidad en el Inicio

Una de las características más críticas de la versión actual es su **resiliencia durante el arranque**.

### Comportamiento en versiones anteriores:
Anteriormente, si una sola base de datos de las múltiples configuradas fallaba al conectar (debido a problemas de red, contraseñas incorrectas, un host inaccesible, etc.), el servidor ejecutaba un `log.Fatalf` y abortaba inmediatamente. Esto rompía la comunicación a través del transporte de entrada/salida estándar (`stdio`) y hacía caer todo el entorno MCP para el cliente (como Claude Desktop o Cursor), bloqueando la interacción con el resto de las bases de datos sanas.

### Comportamiento actual:
- **Arranque tolerante:** Si una base de datos configurada no está disponible al iniciar, el servidor escribe una advertencia detallada en el canal de errores de diagnóstico (`os.Stderr`) y continúa conectando el resto de las bases de datos. El servidor MCP inicia y permanece en línea sin interrupciones.
- **Reporte de error en caliente:** Si el asistente intenta interactuar con una base de datos que falló al conectar, el servidor no se cae. En su lugar, el validador intercepta la llamada y le devuelve al asistente un mensaje explicativo detallado:
  ```json
  "database 'mi_db' is offline. Connection failed on startup: connection timeout on port 5432"
  ```
- **Diagnóstico rápido:** Puedes verificar en cualquier momento el estado de conectividad real de todas tus bases de datos ejecutando el comando de diagnóstico local:
  ```bash
  ./mcp-octo-db doctor
  ```
