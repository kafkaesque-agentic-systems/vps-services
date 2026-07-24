/**
 * Centralised error handling: operators get full detail in the logs, clients
 * get a stable code and a safe message. Stack traces and upstream URLs never
 * reach a response.
 */

import type { NextFunction, Request, Response } from 'express';

import { logger } from '../logger.js';
import { UpstreamError } from '../services/quotes.service.js';
import type { ErrorResponse } from '../types/quotes.js';

/** Maps an upstream failure to the status the client should see. */
function statusForUpstream(error: UpstreamError): number {
  if (error.code === 'UPSTREAM_TIMEOUT') {
    return 504;
  }
  // A 404 from the API (unknown quote id, unknown author) is the CALLER's
  // lookup missing, not a gateway fault — relay it as such.
  if (error.upstreamStatus === 404) {
    return 404;
  }
  return 502;
}

/** Express error middleware (the 4-arg signature is load-bearing). */
export function errorHandler(
  error: unknown,
  req: Request,
  res: Response,
  _next: NextFunction,
): void {
  if (res.headersSent) {
    logger.error('error after headers sent', error, { path: req.path });
    return;
  }

  if (error instanceof UpstreamError) {
    const status = statusForUpstream(error);
    logger.error('upstream failure', error, { path: req.path, code: error.code, status });
    const body: ErrorResponse = {
      error: {
        code: error.code,
        message:
          status === 404
            ? 'Nothing was found for that lookup.'
            : 'The quotes service is temporarily unavailable. Please try again.',
      },
    };
    res.status(status).json(body);
    return;
  }

  logger.error('unhandled error', error, { path: req.path });
  const body: ErrorResponse = {
    error: { code: 'INTERNAL', message: 'Something went wrong.' },
  };
  res.status(500).json(body);
}

/** Terminal JSON 404 for unmatched API routes. */
export function notFoundHandler(_req: Request, res: Response): void {
  const body: ErrorResponse = { error: { code: 'NOT_FOUND', message: 'Not found.' } };
  res.status(404).json(body);
}
