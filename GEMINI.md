# Project Context: truenas_scripts

This repository contains scripts and tools for managing a TrueNAS Scale 25.10.x instance.

## Project Overview
- **Purpose:** Automate management tasks for a specific TrueNAS Scale instance (UGREEN DXP4800 Plus).
- **Primary Language:** Golang.
- **API Communication:** JSON-RPC 2.0 over WebSockets using the [TrueNAS API Client](https://github.com/truenas/api_client_golang).
- **Target System:** TrueNAS Scale 25.10.x.
  - **Hardware:** UGREEN DXP4800 Plus, 64GB RAM.
  - **Storage:**
    - Pool `Tank`: 4x12TB HDD.
    - Pool `Flash`: 2x4TB NVMe SSD.

## Architecture & Structure
The project is structured as a Golang monorepo:
- **Commands:** Located in `cmd/<command_name>/main.go`.
- **Shared Code:** Located in packages at the root of the repository (e.g., `pkg_name/`).
- **Dependencies:** Managed via Go modules (`go.mod`, `go.sum`) in the root directory.

## Development Conventions
- **Language Standards:** Follow idiomatic Go patterns.
- **Testing:**
  - Write unit tests for all Go scripts and packages.
  - Bash scripts are exempt from mandatory testing but should be kept simple.
  - Command: `go test ./...`
- **API Usage:** Prefer the `api_client` websocket client for all TrueNAS interactions.
- **Documentation:** Refer to the [TrueNAS JSON-RPC API Documentation](https://api.truenas.com/v25.10/jsonrpc.html).

## Key Commands
- **Testing:** `go test ./...`
- **Building:** `go build ./cmd/tn-certificate-update`
- **Running:** `go run cmd/tn-certificate-update/main.go`
