# Phased Development Plan: Secure Go MCP Server

**Objective:** Build, secure, and deploy a custom Model Context Protocol (MCP) server in Go, hosted remotely on a VPS, utilizing HTTPS + Server-Sent Events (SSE) for transport.

**Agent Instructions:** Execute this plan sequentially. For each phase, provide fully implemented, strictly idiomatic, and exhaustively documented Go code (or configuration files) as required by your core directives. Do not move to the next phase until the current phase is fully implemented and reviewed.

---

## Phase 1: Project Scaffolding & Initialization
**Goal:** Establish the foundational directory structure and initialize the Go module with required dependencies.

* **Task 1.1:** Initialize the Go module (`go mod init mcp-server`).
* **Task 1.2:** Fetch the official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk/mcp`).
* **Task 1.3:** Create the standard project directory layout (`cmd/server/`, `internal/auth/`, `internal/mcpengine/`, `internal/skills/system/`).

## Phase 2: Security & Authentication Layer
**Goal:** Implement the Bearer Token validation middleware to secure the exposed endpoints.

* **Task 2.1:** Create `internal/auth/middleware.go`.
* **Task 2.2:** Implement the `TokenAuthMiddleware` function.
    * Must read the expected token from the `MCP_SECRET_TOKEN` environment variable.
    * Must check the `Authorization` header for a Bearer token.
    * Must include a fallback to check for a `token` query parameter.
    * Must return standard HTTP 401 Unauthorized or 500 Internal Server Error where appropriate.

## Phase 3: Core MCP Engine & Transport Routing
**Goal:** Set up the main server application, initialize the MCP SDK, and expose the SSE endpoints.

* **Task 3.1:** Create `cmd/server/main.go`.
* **Task 3.2:** Initialize the MCP `ServerInfo` (Name: "Custom-VPS-MCP-Engine", Version: "1.0.0").
* **Task 3.3:** Instantiate the core MCP server using the official SDK (`mcp.NewServer`).
* **Task 3.4:** Set up the HTTP `ServeMux` to handle SSE transport (`/sse` and `/message`).
* **Task 3.5:** Wrap the mux router with the `TokenAuthMiddleware` created in Phase 2.
* **Task 3.6:** Start the HTTP server on the port defined by the `PORT` environment variable (default to 8080).

## Phase 4: Domain-Driven "Skills" Implementation
**Goal:** Create a modular system for defining and registering tools and prompts.

* **Task 4.1:** Create a foundational skill package (e.g., `internal/skills/system/system.go`).
* **Task 4.2:** Define at least one basic tool (e.g., a server health check or uptime monitor) matching the SDK's expected tool schema.
* **Task 4.3:** Implement a `registerCustomSkills` function that iterates through registered skill domains and adds them to the core MCP server using `s.AddTool()`.

## Phase 5: Containerization & Infrastructure Integration
**Goal:** Prepare the application for VPS deployment using Docker and Nginx, integrating into the existing infrastructure.

* **Task 5.1:** Write the `Dockerfile` in the project root.
    * Must be a multi-stage build using `golang:1.24-alpine` for building and `alpine:3.20` for deployment. `CGO_ENABLED=0`.
* **Task 5.2:** Generate a composite `docker-compose.yml`.
    * **Reference:** Read the existing configuration from `vps-docs/docker-compose.yml`.
    * **Action:** Generate an entirely *new* `docker-compose.yml` file in the project root. **Do not overwrite or modify the reference file.**
    * **Requirement:** Integrate the `go-mcp` container into the existing ops network. Ensure environment variables (`PORT`, `MCP_SECRET_TOKEN`) are correctly mapped.
* **Task 5.3:** Generate a composite `nginx.conf`.
    * **Reference:** Read the existing configuration from `vps-docs/nginx.conf`.
    * **Action:** Generate an entirely *new* `nginx.conf` file. **Do not overwrite or modify the reference file.**
    * **Requirement:** Integrate the new SSE proxy routes for the MCP server into the existing server blocks. Include strictly required SSE headers (`proxy_http_version 1.1`, `Connection ""`), disable buffering (`proxy_buffering off`, `proxy_cache off`), and extend `proxy_read_timeout` to support long-lived streams.

## Phase 6: E2E Review & Documentation Audit
**Goal:** Final code quality check and deployment readiness.

* **Task 6.1:** Review all generated Go files to ensure 100% compliance with the Godoc documentation requirement (every struct, interface, function, and package).
* **Task 6.2:** Verify that all error handling is explicit, contextualized, and adheres to the self-healing directives.
* **Task 6.3:** Verify that the newly generated composite infrastructure files successfully combine the existing ops network with the new MCP server requirements without modifying the `vps-docs` directory.
* **Task 6.4:** Provide a brief summary of how a client (e.g., a local Ollama instance or a cloud platform) should formulate its connection request to interact with this specific deployed endpoint.
