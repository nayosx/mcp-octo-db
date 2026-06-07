# Octo DB Developer & Contributor Guide

This document describes the developer workflows, diagnostics utility, Docker commands, release procedures, and build targets for `octo-db`.

---

## Diagnostics (`doctor` command)

`octo-db` includes a built-in diagnostic utility to verify connectivity and validate your configuration before running the server.

### Run Diagnostics
```bash
./octo-db doctor
./octo-db --env /path/to/.env doctor
./octo-db --config /path/to/config.yaml doctor
```

### What `doctor` reports:
* Normalized settings
* Enabled MCP tools
* Configuration validation status
* Connection results per configured database (helps debug connection strings and permissions)

### Useful Operational Flags
* `./octo-db --version`: Prints the current version.
* `./octo-db --list-tools`: Reflects whether `write_query` is currently enabled.
* `./octo-db --print-effective-config`: Prints active settings (masks passwords for safety).

---

## Docker

### Build the Image
```bash
docker build -t octo-db .
```

### Run Diagnostics in a Container
```bash
docker run --rm \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  octo-db --config /app/config.yaml doctor
```

### Print Effective Config from Container
```bash
docker run --rm \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  octo-db --config /app/config.yaml --print-effective-config
```

---

## Local Development Workflow

Useful commands for local checks:
```bash
go test ./...
go vet ./...
go build -o dist/octo-db .
./dist/octo-db doctor
```

### Makefile Targets
```bash
make build
make build-all
make test
make vet
make checksums
make clean
```

* `make build`: Builds the local binary into `dist/octo-db`.
* `make build-all`: Cross-compiles the supported release targets.
* `make checksums`: Generates `dist/checksums.txt` containing SHA-256 signatures.
* `make clean`: Removes the `dist/` directory.

### Recommended Manual Smoke Test
1. Run `./octo-db doctor`.
2. Call `server_info`.
3. Call `find_columns` with a known business term.
4. Call `suggest_query_plan` with a non-technical question.
5. Confirm `read_query` still enforces policy and row limits.

---

## Release Checklist

* Run `go test ./...`
* Run `go vet ./...`
* Run `make build-all`
* Run `make checksums`
* Verify `doctor` with a real local configuration
* Review the examples in [examples/](../examples/)
* Review [CHANGELOG.md](../CHANGELOG.md)
* Tag a version like `v1.4.4`

---

## Distribution

The distribution of `octo-db` binaries is fully automated. Whenever a new tag (matching `v*`) is pushed, the Release GitHub Action automatically compiles and bundles static binaries for major operating systems and architectures.

### Supported Platform Targets:
* **Linux**: `amd64` and `arm64` (packaged as `.tar.gz` archives)
* **macOS**: `amd64` and `arm64` (packaged as `.tar.gz` archives)
* **Windows**: `amd64` (packaged as `.zip` archive)

### How to obtain and run release binaries:
1. Navigate to the **Releases** page of the repository on GitHub.
2. Download the compressed archive matching your operating system and architecture.
3. Extract the downloaded archive:
   * **Linux/macOS**: `tar -xzf octo-db-vX.Y.Z-goos-goarch.tar.gz`
   * **Windows**: Extract the `.zip` file using your file manager or PowerShell.
4. (Optional but recommended) Verify release integrity by checking the SHA-256 checksums published alongside the release archives:
   ```bash
   sha256sum --check checksums-goos-goarch.txt
   ```
5. Run the binary inside the extracted folder:
   ```bash
   ./octo-db --version
   ```
