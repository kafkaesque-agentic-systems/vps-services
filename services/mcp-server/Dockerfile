# syntax=docker/dockerfile:1
#
# Multi-stage Dockerfile for the Custom-VPS-MCP-Engine.
#
# The two-stage design keeps the final image tiny and free of build tooling:
#   * Stage 1 ("builder") uses the full Go toolchain to compile a static binary.
#   * Stage 2 ("runtime") copies ONLY that binary into a minimal Alpine base.

##########################################################################
# Stage 1 — builder
##########################################################################
#
# HERMETIC BUILDS — DIGEST PINNING (Audit O-5):
#   The base images are pinned by sha256 digest, not just by tag. Tags like
#   "1.25-alpine" are mutable (they are re-pointed as patch releases ship), so
#   pinning the digest guarantees every build — today and in six months — uses
#   the EXACT same base image bytes. This is essential for reproducible,
#   auditable production builds.
#
#   >>> ACTION REQUIRED: replace the placeholder digests below with the real
#       digests for your platform before building. Obtain them with:
#         docker buildx imagetools inspect golang:1.25-alpine
#         docker buildx imagetools inspect alpine:3.20
#       (or `docker pull <image> && docker inspect --format='{{index .RepoDigests 0}}' <image>`)
#
#   NOTE ON THE GO VERSION: the plan originally called for golang:1.24-alpine,
#   but go.mod declares `go 1.25.0`; a 1.24 toolchain would trigger a non-hermetic
#   network toolchain download at build time, so we pin 1.25-alpine to match.
FROM golang:1.25-alpine@sha256:523c3effe300580ed375e43f43b1c9b091b68e935a7c3a92bfcc4e7ed55b18c2 AS builder

# Install git+ca-certificates in case any (current or future) module needs to be
# fetched over HTTPS from a VCS. Kept in the builder stage only — it never
# reaches the runtime image.
RUN apk add --no-cache git ca-certificates

WORKDIR /src

# --- Dependency layer (cached independently of source changes) ---------------
#
# Copy only the module manifests first. Docker caches this layer, so `go mod
# download` re-runs only when go.mod/go.sum actually change — not on every source
# edit. This dramatically speeds up iterative builds.
COPY go.mod go.sum ./
RUN go mod download

# --- Source + compile --------------------------------------------------------
COPY . .

# Build flags explained:
#   CGO_ENABLED=0    -> fully static binary, no libc dependency (Task 5.1).
#   GOOS=linux       -> explicit target OS for deterministic builds.
#   -trimpath        -> strip local filesystem paths for reproducibility.
#   -ldflags "-s -w" -> drop symbol table + DWARF to shrink the binary.
#
# The entry point package lives at ./cmd/server (the composition root).
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/mcp-server \
    ./cmd/server

##########################################################################
# Stage 2 — runtime
##########################################################################
#
# alpine:3.20 (per Task 5.1), also pinned by digest for hermetic builds.
# >>> ACTION REQUIRED: replace the placeholder digest as described above.
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc AS runtime

# Runtime OS packages, deliberately minimal:
#   ca-certificates -> outbound TLS for any future skill that needs it.
#   tzdata          -> REQUIRED by the system_time tool: time.LoadLocation needs
#                      the IANA timezone database, which Alpine omits by default.
#   wget            -> provided by BusyBox in the base image; used by the
#                      HEALTHCHECK below. (No extra install needed; noted for clarity.)
#   tar             -> also provided by BusyBox; used by snapshot_create to archive
#                      the VPS services tree.
#   docker-cli      -> docker compose lifecycle tools (system_down/up/logs).
#   docker-cli-compose -> Compose v2 plugin required by `docker compose`.
RUN apk add --no-cache ca-certificates tzdata docker-cli docker-cli-compose

# Run as a non-root, unprivileged user to limit blast radius if compromised.
RUN addgroup -S mcp && adduser -S -G mcp -H -s /sbin/nologin mcp

WORKDIR /app

# Copy ONLY the compiled binary from the builder stage.
COPY --from=builder /out/mcp-server /app/mcp-server

# Drop privileges for everything that follows.
USER mcp

# Document the port the server listens on (informational; publishes nothing).
EXPOSE 8080

# In-container default; docker-compose injects the real PORT/MCP_SECRET_TOKEN.
ENV PORT=8080

# HEALTHCHECK (Audit C-3): probe the unauthenticated /healthz liveness endpoint.
# A non-2xx response or a connection failure makes wget exit non-zero, marking
# the container unhealthy — which docker-compose's `service_healthy` gate keys on.
#   --start-period gives the process a moment to bind before failures count.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

# Exec form so the binary is PID 1 and receives SIGTERM directly, enabling the
# graceful-shutdown logic in main() to run on `docker stop`.
ENTRYPOINT ["/app/mcp-server"]
