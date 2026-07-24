/**
 * Composition root for the quotes front-end service.
 *
 * Mounted at the DOMAIN ROOT behind the NGINX gateway, replacing the Flask
 * quotes-web tier. Wires configuration, security middleware, the static
 * React bundle, the BFF routes, and graceful shutdown. Layering is strict:
 * routes -> controllers -> services; nothing below this file reads
 * `process.env`.
 */

import path from 'node:path';
import { fileURLToPath } from 'node:url';

import express from 'express';
import helmet from 'helmet';

import { ConfigError, loadConfig, type AppConfig } from './config.js';
import { logger } from './logger.js';
import { errorHandler, notFoundHandler } from './middleware/errorHandler.js';
import { createApiRouter } from './routes/api.routes.js';
import { createHealthRouter } from './routes/health.routes.js';

/** Seconds to let in-flight requests finish before forcing exit on SIGTERM. */
const SHUTDOWN_GRACE_MS = 10_000;

/** Absolute path to the built client bundle (dist/client, sibling of us). */
function resolveClientDir(): string {
  const here = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(here, '../client');
}

/** Builds the fully configured Express application. */
function createApp(config: AppConfig): express.Express {
  const app = express();

  app.set('trust proxy', 1);
  app.disable('x-powered-by');

  app.use(
    helmet({
      contentSecurityPolicy: {
        directives: {
          // Strict self-only policy — the honeypot + rate-limit bot defence
          // was chosen over reCAPTCHA precisely so no third-party script
          // origin needs to be allowed here.
          defaultSrc: ["'self'"],
          imgSrc: ["'self'", 'data:'],
          styleSrc: ["'self'"],
          scriptSrc: ["'self'"],
          connectSrc: ["'self'"],
          objectSrc: ["'none'"],
          frameAncestors: ["'none'"],
          baseUri: ["'self'"],
          formAction: ["'self'"],
          // Helmet MERGES these directives with its defaults, and the default
          // set includes upgrade-insecure-requests. Behind the TLS-terminating
          // gateway that directive is inert — but on plain-HTTP localhost,
          // Safari honours it strictly and rewrites every asset fetch to
          // https://localhost, where no TLS exists: title loads, page blank.
          // (Chromium exempts localhost, which hid the bug.) Null removes it.
          upgradeInsecureRequests: null,
        },
      },
      // TLS terminates at NGINX, which owns HSTS for the domain.
      hsts: false,
      crossOriginEmbedderPolicy: false,
    }),
  );

  // The largest legitimate body is a quote submission; 32kb is generous.
  app.use(express.json({ limit: '32kb' }));
  app.use(express.urlencoded({ extended: false, limit: '32kb' }));

  app.use('/healthz', createHealthRouter());

  const apiMount = config.basePath === '/' ? '/api' : `${config.basePath}/api`;
  app.use(apiMount, createApiRouter(config));
  app.use(apiMount, notFoundHandler);

  const clientDir = resolveClientDir();
  app.use(
    config.basePath,
    express.static(clientDir, {
      index: false,
      redirect: false,
      maxAge: '1h',
      setHeaders: (res, filePath) => {
        if (filePath.includes(`${path.sep}assets${path.sep}`)) {
          res.setHeader('Cache-Control', 'public, max-age=31536000, immutable');
        }
      },
    }),
  );

  const shellPaths =
    config.basePath === '/' ? ['/', '/*'] : [config.basePath, `${config.basePath}/*`];
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
    if (error instanceof ConfigError) {
      logger.error('invalid configuration; refusing to start', error);
    } else {
      logger.error('configuration failed; refusing to start', error);
    }
    process.exit(1);
  }

  const app = createApp(config);
  const server = app.listen(config.port, () => {
    logger.info('quotes-ui listening', {
      port: config.port,
      basePath: config.basePath,
      upstream: config.quotesApiBaseUrl,
      nodeEnv: config.nodeEnv,
      tokenRequests: config.adminToken !== null ? 'enabled' : 'DISABLED (AUTHORIZED unset)',
      mail: config.mail !== null ? 'configured' : 'DISABLED',
    });
  });

  server.on('error', (error) => {
    logger.error('server error', error);
    process.exit(1);
  });

  const shutdown = (signal: string): void => {
    logger.info('shutdown signal received; draining', { signal, graceMs: SHUTDOWN_GRACE_MS });
    const force = setTimeout(() => {
      logger.warn('grace period elapsed; forcing exit');
      process.exit(1);
    }, SHUTDOWN_GRACE_MS);
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
