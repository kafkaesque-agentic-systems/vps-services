# PERFORMANCE / SECURITY (Audit P-3): multi-stage build.
#
# The previous single-stage `FROM golang:latest` shipped the ENTIRE Go
# toolchain (~800MB+), was unpinned (non-reproducible builds), ran as root,
# and copied source before `go mod download` — busting the module cache on
# every code change. This build:
#   * pins the toolchain version (reproducible),
#   * caches dependency downloads separately from source,
#   * produces a static binary,
#   * ships it on distroless/nonroot (~15MB, no shell, non-root).

# ---- Stage 1: build -------------------------------------------------------
FROM golang:1.19 AS builder

WORKDIR /app

# Copy manifests first so `go mod download` is cached unless deps change.
COPY src/go.mod src/go.sum ./
RUN go mod download

# Now copy the source and build a static, stripped binary.
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o quotes-server

# ---- Stage 2: runtime -----------------------------------------------------
# distroless/static carries only CA certs + tzdata; :nonroot runs as an
# unprivileged UID. No shell, package manager, or toolchain = minimal attack
# surface.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /app/quotes-server .

ENV GIN_MODE=release
EXPOSE 8080
USER nonroot

ENTRYPOINT [ "./quotes-server" ]
