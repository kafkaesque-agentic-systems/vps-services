/**
 * Liveness endpoint.
 *
 * Mounted at `/healthz` OUTSIDE the public base path, mirroring the Go MCP
 * server's convention: the container healthcheck probes it directly over the
 * docker network, and NGINX never proxies it publicly.
 */

import { Router } from 'express';

/** Body returned by `GET /healthz`. */
interface HealthResponse {
  readonly status: 'ok';
  readonly uptimeSeconds: number;
}

/** Builds the liveness router. */
export function createHealthRouter(): Router {
  const router = Router();

  router.get('/', (_req, res) => {
    const body: HealthResponse = {
      status: 'ok',
      uptimeSeconds: Math.floor(process.uptime()),
    };
    res.setHeader('Cache-Control', 'no-store');
    res.status(200).json(body);
  });

  return router;
}
