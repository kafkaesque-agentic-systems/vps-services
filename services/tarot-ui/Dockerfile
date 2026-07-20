# syntax=docker/dockerfile:1
#
# Tarot front-end: React client + typed Node BFF.
#
# Multi-stage so the runtime image carries neither the toolchain nor the dev
# dependencies -- only production node_modules and the compiled output. Runs as
# a non-root user, matching the hardening applied to the other services.

# ---------------------------------------------------------------------------
# Stage 1: install ALL dependencies (dev included) for the build.
# ---------------------------------------------------------------------------
FROM node:22-alpine AS deps

WORKDIR /app

# Copy only the manifests first so this layer is cached until they change --
# dependency installs dominate build time.
COPY package.json package-lock.json ./

# `npm ci` installs exactly the lockfile, failing if the two have diverged.
RUN npm ci

# ---------------------------------------------------------------------------
# Stage 2: type-check and build both client and server.
# ---------------------------------------------------------------------------
FROM node:22-alpine AS build

WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .

# BASE_PATH must match the public mount path: Vite bakes it into asset URLs,
# so a mismatch yields a page whose scripts 404 behind the gateway.
ARG BASE_PATH=/tarot
ENV BASE_PATH=${BASE_PATH}

# VITE_IMAGE_BASE is deliberately left unset: it defaults to the root-relative
# "/image", which NGINX serves on this app's own origin. Setting an absolute
# URL here would hard-code a domain into the bundle.

# `npm run build` type-checks the client, builds it with Vite, then compiles
# the server with tsc. Any type error fails the image build.
RUN npm run build

# Re-resolve dependencies without dev packages for the runtime stage.
RUN npm prune --omit=dev

# ---------------------------------------------------------------------------
# Stage 3: minimal runtime.
# ---------------------------------------------------------------------------
FROM node:22-alpine AS runtime

WORKDIR /app

ENV NODE_ENV=production

# The node image ships an unprivileged `node` user (uid 1000); use it rather
# than minting another. Ownership is set at copy time so no chown layer is
# needed.
COPY --from=build --chown=node:node /app/node_modules ./node_modules
COPY --from=build --chown=node:node /app/dist ./dist
COPY --from=build --chown=node:node /app/package.json ./package.json

USER node

EXPOSE 3000

# Liveness probe hits the unauthenticated /healthz route the server exposes
# outside its public base path. Uses node itself -- the image has no curl.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD node -e "fetch('http://127.0.0.1:'+(process.env.PORT||3000)+'/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"

# Direct exec form: node runs as PID 1 and receives SIGTERM itself, which the
# server handles by draining in-flight requests. Wrapping in npm would swallow
# the signal and force a 10s kill on every deploy.
CMD ["node", "dist/server/index.js"]
