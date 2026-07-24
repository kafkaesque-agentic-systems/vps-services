/**
 * Minimal fixed-window rate limiter for the token-request endpoint.
 *
 * In-memory and per-process by design: this protects ONE low-traffic endpoint
 * on a single-instance service from naive abuse, together with the form's
 * honeypot field. It is not, and does not need to be, a distributed limiter.
 * The map is pruned on every hit so it cannot grow unbounded.
 */

import type { NextFunction, Request, Response } from 'express';

import type { ErrorResponse } from '../types/quotes.js';

interface Window {
  count: number;
  resetAt: number;
}

/**
 * Builds a limiter allowing `limit` requests per `windowMs` per client IP.
 *
 * `req.ip` honours Express's `trust proxy` setting, so behind the NGINX
 * gateway this is the real client address from X-Forwarded-For.
 */
export function makeRateLimiter(limit: number, windowMs: number) {
  const windows = new Map<string, Window>();

  return (req: Request, res: Response, next: NextFunction): void => {
    const now = Date.now();

    // Prune expired windows so the map tracks only active clients.
    for (const [key, value] of windows) {
      if (value.resetAt <= now) {
        windows.delete(key);
      }
    }

    const ip = req.ip ?? 'unknown';
    const current = windows.get(ip);

    if (current === undefined) {
      windows.set(ip, { count: 1, resetAt: now + windowMs });
      next();
      return;
    }

    current.count += 1;
    if (current.count > limit) {
      const body: ErrorResponse = {
        error: {
          code: 'RATE_LIMITED',
          message: 'Too many requests. Please try again later.',
        },
      };
      res.setHeader('Retry-After', Math.ceil((current.resetAt - now) / 1000).toString());
      res.status(429).json(body);
      return;
    }

    next();
  };
}
