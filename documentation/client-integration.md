# MCP Client Integration Guide

This document explains how to integrate `octo-db` with various Model Context Protocol (MCP) clients and environments, and how to verify your installation.

---

## The Role of a Wrapper Script

Using a wrapper script (like `run-octo-db.sh` for Linux/macOS or `run-octo-db.bat` for Windows) is highly recommended for security and ease of setup.

### Why it is used:
MCP clients execute servers in their own runtime environments where setting shell environment variables can be difficult. The wrapper script sources a local, untracked `.env` file containing secrets, keeping them secure, and then starts the `octo-db` server.

### Advantages:
It prevents sensitive database credentials from being stored directly in your MCP client configuration files (which are stored in plain text in app configuration directories and might be backed up or shared).

### How to adapt it:
* **Linux/macOS**: Copy `run-octo-db.sh.example` to `run-octo-db.sh`, make it executable (`chmod +x run-octo-db.sh`), and update the absolute paths pointing to your local `.env` and `octo-db` binary.
* **Windows**: Copy `run-octo-db.bat.example` to `run-octo-db.bat` and update the absolute paths pointing to your local `.env` file and `octo-db.exe` binary.

---

## Client Configurations

### Codex Configuration
Add the following to your Codex configuration file (typically in `.codex` configuration folder or `codex.toml`):

```toml
[mcp_servers.octo_db]
command = "/path/to/run-octo-db.sh"
args = []
```

### Claude Desktop Configuration
Add the following to your Claude Desktop configuration file (typically `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS or `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

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

> [!NOTE]
> For Cursor, you can also add this server visually through the settings UI by choosing type `command`, entering `octo_db` as name, and specifying the wrapper script `/path/to/run-octo-db.sh` as the command.

---

## Verify Installation

To verify that the MCP server is working correctly and properly registered by your client:

1. **Verify server discovery using Codex CLI (or equivalent tool):**
   ```bash
   codex mcp list
   ```
   *This command should list `octo-db` as one of the active, connected MCP servers.*

2. **Test functionality using diagnostic prompts in your MCP client:**
   * **Prompt 1:**
     ```text
     Use octo_db and list available tables.
     ```
   * **Prompt 2:**
     ```text
     Use octo_db and describe the users table.
     ```
   * **Prompt 3:**
     ```text
     Use octo_db and explain this SQL query.
     ```
