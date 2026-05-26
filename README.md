# mcp-octo-db

`mcp-octo-db` es un servidor del **Model Context Protocol (MCP)** desarrollado en Go. Permite conectar asistentes de Inteligencia Artificial (como Cursor, Claude Desktop, etc.) a múltiples bases de datos relacionales de forma simultánea, soportando indistintamente **PostgreSQL**, **MySQL**, **MariaDB** y **SQLite**.

El nombre `octo` (pulpo) hace referencia a su capacidad para extender tentáculos a diferentes motores y esquemas utilizando una configuración centralizada.

---

## Características

- 🔌 **Soporte Multimotor:** Conéctate a bases de datos PostgreSQL, MySQL, MariaDB y SQLite usando el mismo servidor.
- 🗃️ **Multi-DB en Simultáneo:** Mapea múltiples bases de datos usando sufijos en las variables de entorno.
- 🔒 **Consultas de Solo Lectura Protegidas:** La herramienta `read_query` restringe las consultas a operaciones `SELECT` y utiliza transacciones de solo lectura (`ReadOnly`) donde el motor lo soporte.
- 🛠️ **Introspección Completa:** Herramientas dedicadas para listar tablas y describir detalladamente la estructura de columnas (tipos, nulabilidad, llaves primarias, valores por defecto).
- ⚙️ **Configuración Portable:** Configurable completamente a través de un archivo `.env`.

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

El servidor expone cuatro herramientas principales al modelo de IA:

### 1. `list_tables`
Lista todas las tablas en una base de datos y esquema específicos.
- **Argumentos:**
  - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
  - `schema` (opcional, string): Esquema a consultar (por defecto `public` en Postgres, o el nombre de la BD actual en MySQL).

### 2. `describe_table`
Muestra la estructura de columnas, tipos, si aceptan nulos, llaves primarias y valores por defecto de una tabla.
- **Argumentos:**
  - `table_name` (requerido, string): Nombre de la tabla a describir.
  - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
  - `schema` (opcional, string): Esquema de la tabla.

### 3. `read_query`
Ejecuta de manera segura una consulta SQL de solo lectura y devuelve los resultados en formato JSON.
- **Restricciones:** Solo permite sentencias `SELECT`, `WITH`, `SHOW`, `DESCRIBE` y `EXPLAIN`.
- **Argumentos:**
  - `sql` (requerido, string): La consulta SQL a ejecutar.
  - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).

### 4. `write_query`
Ejecuta consultas de modificación de datos o estructura.
- **Argumentos:**
  - `sql` (requerido, string): La consulta SQL a ejecutar (`INSERT`, `UPDATE`, `DELETE`, `CREATE`, etc.).
  - `db_name` (opcional, string): Nombre de la base de datos (por defecto `default`).
