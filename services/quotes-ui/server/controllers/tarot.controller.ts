/**
 * Public tarot pass-throughs for the API reference page.
 *
 * These endpoints are open on the upstream API, and the reference's job is
 * to show its REAL responses — so the upstream status and JSON body are
 * relayed verbatim, exactly like the token-gated quote writes but with no
 * auth requirement. Paths are FIXED per route (plus a validated id segment);
 * this is a bounded catalogue, not an open proxy.
 */

import type { AppConfig } from '../config.js';
import type { ErrorResponse } from '../types/quotes.js';
import type { Handler } from './quotes.controller.js';

/** A MongoDB ObjectId: exactly 24 hex characters. */
const OBJECT_ID = /^[0-9a-fA-F]{24}$/;

/**
 * Builds a verbatim relay to one fixed upstream tarot path.
 *
 * @param config - Runtime configuration.
 * @param method - GET or POST (the tarot surface has no authorized writes).
 * @param upstreamPath - Fixed path, with `:id` marking a validated id segment.
 */
export function makeTarotPassthrough(
  config: AppConfig,
  method: 'GET' | 'POST',
  upstreamPath: string,
): Handler {
  return async (req, res, next): Promise<void> => {
    try {
      let path = upstreamPath;
      if (upstreamPath.includes(':id')) {
        const id = req.params['id'] ?? '';
        if (!OBJECT_ID.test(id)) {
          const body: ErrorResponse = {
            error: { code: 'INVALID_REQUEST', message: 'id must be a 24-character hex ObjectId' },
          };
          res.status(400).json(body);
          return;
        }
        path = upstreamPath.replace(':id', encodeURIComponent(id));
      }

      const controller = new AbortController();
      const timeout = setTimeout(() => {
        controller.abort();
      }, config.upstreamTimeoutMs);

      let upstream: globalThis.Response;
      try {
        upstream = await fetch(`${config.quotesApiBaseUrl}${path}`, {
          method,
          signal: controller.signal,
          headers: {
            accept: 'application/json',
            // The service identity: the API is token-gated, and these demo
            // panels are the visitor's window through it.
            ...(config.apiServiceToken === null ? {} : { authorization: config.apiServiceToken }),
            ...(method === 'POST' ? { 'content-type': 'application/json' } : {}),
          },
          ...(method === 'POST' ? { body: JSON.stringify(req.body ?? {}) } : {}),
        });
      } finally {
        clearTimeout(timeout);
      }

      const text = await upstream.text();
      res.status(upstream.status);
      res.setHeader('Cache-Control', 'no-store');
      try {
        res.json(JSON.parse(text));
      } catch {
        res.type('text/plain').send(text);
      }
    } catch (error) {
      next(error);
    }
  };
}
