/**
 * Centralised error handling.
 *
 * Every failure in the app converges here so responses are formatted in exactly
 * one place. The guiding rule: operators get the full detail in the logs,
 * clients get a stable code and a safe message. Stack traces, upstream URLs,
 * and internal messages are never written to a response.
 */

import type { NextFunction, Request, Response } from 'express';

import { logger } from '../logger.js';
import { UpstreamError } from '../services/tarot.service.js';
import type { ErrorResponse } from '../types/tarot.js';

/**
 * Maps an {@link UpstreamError} code to the HTTP status the client should see.
 *
 * Every upstream failure is a *gateway* failure from the browser's point of
 * view -- this service is healthy, its dependency is not -- so these are 502/504
 * rather than 500.
 */
function statusForUpstream(code: string): number {
  switch (code) {
    case 'UPSTREAM_TIMEOUT':
      return 504;
    case 'UPSTREAM_UNREACHABLE':
    case 'UPSTREAM_STATUS':
    case 'UPSTREAM_MALFORMED':
    case 'UNEXPECTED_IMAGE_URL':
      return 502;
    default:
      return 502;
  }
}

/**
 * Express error middleware.
 *
 * The four-argument signature is required for Express to recognise this as an
 * error handler; `_next` is therefore unused but must be declared.
 */
export function errorHandler(
  error: unknown,
  req: Request,
  res: Response,
  _next: NextFunction,
): void {
  // Nothing can be done once headers are on the wire; hand back to Express so
  // it destroys the socket rather than attempting a second response.
  if (res.headersSent) {
    logger.error('error after headers sent', error, { path: req.path });
    return;
  }

  if (error instanceof UpstreamError) {
    const status = statusForUpstream(error.code);
    // Full detail to the log, including the cause chain.
    logger.error('upstream failure', error, {
      path: req.path,
      code: error.code,
      status,
    });
    const body: ErrorResponse = {
      error: {
        code: error.code,
        message: 'The tarot service is temporarily unavailable. Please try again.',
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

/** Terminal 404 handler for unmatched API routes. */
export function notFoundHandler(_req: Request, res: Response): void {
  const body: ErrorResponse = {
    error: { code: 'NOT_FOUND', message: 'Not found.' },
  };
  res.status(404).json(body);
}
