/**
 * Composition root for the tarot front-end service.
 *
 * Wires configuration, security middleware, the static React bundle, the BFF
 * routes, and graceful shutdown. Layering is strict and one-directional:
 * routes -> controllers -> services. Nothing below this file reads
 * `process.env`.
 *
 * Served publicly at `{BASE_PATH}` behind the NGINX gateway; the browser never
 * learns the upstream API's address.
 */

import path from 'node:path';
import { fileURLToPath } from 'node:url';

import express from 'express';
import helmet from 'helmet';

import { ConfigError, loadConfig, type AppConfig } from './config.js';
import { logger } from './logger.js';
import { errorHandler, notFoundHandler } from './middleware/errorHandler.js';
import { createHealthRouter } from './routes/health.routes.js';
import { createTarotRouter } from './routes/tarot.routes.js';

/** Seconds to let in-flight requests finish before forcing exit on SIGTERM. */
const SHUTDOWN_GRACE_MS = 10_000;

/**
 * Absolute path to the built client bundle.
 *
 * Compiled output lives at `dist/server/index.js`, so the client at
 * `dist/client` is one level up from this file's directory.
 */
function resolveClientDir(): string {
  const here = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(here, '../client');
}

/**
 * Origin serving card images, for the CSP `img-src` allow-list.
 *
 * In production this is the same origin as the app, so `'self'` would already
 * cover it -- but naming it explicitly means the policy stays correct if the
 * image store ever moves to its own host, and it lets the app run correctly
 * outside the gateway during development.
 */
function imageOrigin(config: AppConfig): string {
  return new URL(config.publicImagePrefix).origin;
}

/** Builds the fully configured Express application. */
function createApp(config: AppConfig): express.Express {
  const app = express();

  // Behind NGINX: trust the proxy so req.ip and req.protocol reflect the
  // original client rather than the gateway.
  app.set('trust proxy', 1);
  app.disable('x-powered-by');

  app.use(
    helmet({
      contentSecurityPolicy: {
        directives: {
          defaultSrc: ["'self'"],
          // Card images come from the configured image store. Behind the
          // gateway that is the same origin as the app; naming it explicitly
          // keeps the policy correct if it ever moves, and keeps the app
          // usable outside the gateway.
          imgSrc: ["'self'", 'data:', imageOrigin(config)],
          // Tailwind ships a static stylesheet; no inline styles are emitted.
          styleSrc: ["'self'"],
          scriptSrc: ["'self'"],
          connectSrc: ["'self'"],
          objectSrc: ["'none'"],
          frameAncestors: ["'none'"],
          baseUri: ["'self'"],
          formAction: ["'self'"],
          // Helmet merges its defaults in, including upgrade-insecure-requests
          // — inert behind the TLS gateway, but it blanks the page on
          // plain-HTTP localhost in Safari (assets force-upgraded to
          // https://localhost, TLS failure). Null removes it; see quotes-ui.
          upgradeInsecureRequests: null,
        },
      },
      // TLS is terminated at NGINX, which owns HSTS for the whole domain.
      hsts: false,
      crossOriginEmbedderPolicy: false,
    }),
  );

  // This service exposes no cross-origin API and accepts no request bodies;
  // the tight limit is defence in depth rather than a functional requirement.
  app.use(express.json({ limit: '16kb' }));
  app.use(express.urlencoded({ extended: false, limit: '16kb' }));

  // Liveness, deliberately outside basePath and never proxied publicly.
  app.use('/healthz', createHealthRouter());

  const apiMount = config.basePath === '/' ? '/api' : `${config.basePath}/api`;
  app.use(apiMount, createTarotRouter(config));

  // Unmatched API paths must return JSON, not the SPA shell -- otherwise a
  // typo'd endpoint yields 200 + HTML and the client fails confusingly.
  app.use(apiMount, notFoundHandler);

  const clientDir = resolveClientDir();
  app.use(
    config.basePath,
    express.static(clientDir, {
      index: false,
      // Never issue the directory redirect (`/tarot` -> `/tarot/`). Behind the
      // gateway that Location header is derived from the internal host, so it
      // can leak an unroutable URL to the browser. The explicit shell route
      // below serves the bare mount path instead.
      redirect: false,
      maxAge: '1h',
      // Content-hashed assets are safe to cache aggressively and immutably.
      setHeaders: (res, filePath) => {
        if (filePath.includes(`${path.sep}assets${path.sep}`)) {
          res.setHeader('Cache-Control', 'public, max-age=31536000, immutable');
        }
      },
    }),
  );

  // SPA shell for the mount point and anything beneath it.
  const shellPaths = config.basePath === '/' ? ['/', '/*'] : [config.basePath, `${config.basePath}/*`];
  app.get(shellPaths, (_req, res) => {
    res.setHeader('Cache-Control', 'no-cache');
    res.sendFile(path.join(clientDir, 'index.html'));
  });

  app.use(errorHandler);

  return app;
}

/** Boots the server and installs signal handlers. */
function main(): void {
  let config: AppConfig;
  try {
    config = loadConfig();
  } catch (error) {
    // Fail fast and loud: a misconfigured process must never start and appear
    // healthy while being subtly wrong.
    if (error instanceof ConfigError) {
      logger.error('invalid configuration; refusing to start', error);
    } else {
      logger.error('configuration failed; refusing to start', error);
    }
    process.exit(1);
  }

  const app = createApp(config);
  const server = app.listen(config.port, () => {
    logger.info('tarot-ui listening', {
      port: config.port,
      basePath: config.basePath,
      upstream: config.tarotApiBaseUrl,
      nodeEnv: config.nodeEnv,
    });
  });

  server.on('error', (error) => {
    logger.error('server error', error);
    process.exit(1);
  });

  /** Drains in-flight requests, then exits. */
  const shutdown = (signal: string): void => {
    logger.info('shutdown signal received; draining', {
      signal,
      graceMs: SHUTDOWN_GRACE_MS,
    });

    const force = setTimeout(() => {
      logger.warn('grace period elapsed; forcing exit');
      process.exit(1);
    }, SHUTDOWN_GRACE_MS);
    // Do not keep the event loop alive solely for the force-exit timer.
    force.unref();

    server.close((error) => {
      if (error) {
        logger.error('error during shutdown', error);
        process.exit(1);
      }
      logger.info('shutdown complete');
      process.exit(0);
    });
  };

  process.on('SIGTERM', () => {
    shutdown('SIGTERM');
  });
  process.on('SIGINT', () => {
    shutdown('SIGINT');
  });
}

main();
