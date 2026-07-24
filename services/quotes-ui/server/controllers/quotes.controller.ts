/**
 * HTTP shaping for the quotes read paths and the authorized write
 * pass-through. Controllers translate between HTTP and the services and do
 * nothing else.
 */

import type { NextFunction, Request, Response } from 'express';

import type { AppConfig } from '../config.js';
import {
  fetchAuthors,
  fetchQuoteById,
  fetchQuotesByAuthor,
  fetchRandomQuote,
  searchQuotes,
} from '../services/quotes.service.js';
import type { ErrorResponse } from '../types/quotes.js';

/** Express handler signature used by this module. */
export type Handler = (req: Request, res: Response, next: NextFunction) => Promise<void> | void;

/** A MongoDB ObjectId: exactly 24 hex characters. */
const OBJECT_ID = /^[0-9a-fA-F]{24}$/;

/** Sends a 400 with this BFF's standard error body. */
function badRequest(res: Response, message: string): void {
  const body: ErrorResponse = { error: { code: 'INVALID_REQUEST', message } };
  res.status(400).json(body);
}

/** `GET /api/quote` — one random quote. */
export function makeGetRandomQuote(config: AppConfig): Handler {
  return async (_req, res, next): Promise<void> => {
    try {
      const quote = await fetchRandomQuote(config);
      res.setHeader('Cache-Control', 'no-store');
      res.status(200).json({ quote });
    } catch (error) {
      next(error);
    }
  };
}

/** `GET /api/quote/:id` — one quote by ObjectId. */
export function makeGetQuoteById(config: AppConfig): Handler {
  return async (req, res, next): Promise<void> => {
    try {
      const id = req.params['id'] ?? '';
      if (!OBJECT_ID.test(id)) {
        badRequest(res, 'id must be a 24-character hex ObjectId');
        return;
      }
      const quote = await fetchQuoteById(config, id);
      res.status(200).json({ quote });
    } catch (error) {
      next(error);
    }
  };
}

/** `GET /api/authors` — the author catalogue. */
export function makeGetAuthors(config: AppConfig): Handler {
  return async (_req, res, next): Promise<void> => {
    try {
      const names = await fetchAuthors(config);
      res.setHeader('Cache-Control', 'public, max-age=300');
      res.status(200).json({ names });
    } catch (error) {
      next(error);
    }
  };
}

/** `GET /api/authors/:name` — quotes by author (name normalised server-side). */
export function makeGetQuotesByAuthor(config: AppConfig): Handler {
  return async (req, res, next): Promise<void> => {
    try {
      const name = (req.params['name'] ?? '').trim();
      if (name === '' || name.length > 100) {
        badRequest(res, 'name must be 1-100 characters');
        return;
      }
      const quotes = await fetchQuotesByAuthor(config, name);
      res.status(200).json({ quotes });
    } catch (error) {
      next(error);
    }
  };
}

/** `POST /api/search` — keyword/phrase search over a raw comma syntax. */
export function makeSearch(config: AppConfig): Handler {
  return async (req, res, next): Promise<void> => {
    try {
      const raw = (req.body as Record<string, unknown> | null)?.['query'];
      if (typeof raw !== 'string' || raw.trim() === '' || raw.length > 500) {
        badRequest(res, 'query must be a non-empty string of at most 500 characters');
        return;
      }
      const quotes = await searchQuotes(config, raw.trim());
      res.setHeader('Cache-Control', 'no-store');
      res.status(200).json({ quotes });
    } catch (error) {
      next(error);
    }
  };
}

/**
 * Authorized write pass-through: POST /api/quote, PUT/DELETE /api/quote/:id.
 *
 * These panels demonstrate the API's token-gated operations, so the CALLER's
 * own Authorization header is forwarded verbatim and the upstream's exact
 * status and JSON body are relayed back — the page is showing the API's real
 * behaviour, not this BFF's opinion of it. The token is never stored and
 * never logged. Absent a token the request is refused here, saving a
 * guaranteed upstream 401 round trip.
 */
export function makeWritePassthrough(
  config: AppConfig,
  method: 'POST' | 'PUT' | 'DELETE',
): Handler {
  return async (req, res, next): Promise<void> => {
    try {
      const token = req.headers.authorization;
      if (token === undefined || token.trim() === '') {
        const body: ErrorResponse = {
          error: { code: 'NO_TOKEN', message: 'An API token is required for this operation.' },
        };
        res.status(401).json(body);
        return;
      }

      const id = req.params['id'];
      if (id !== undefined && !OBJECT_ID.test(id)) {
        badRequest(res, 'id must be a 24-character hex ObjectId');
        return;
      }

      const path = id === undefined ? '/quote' : `/quote/${encodeURIComponent(id)}`;
      const hasBody = method !== 'DELETE';

      const controller = new AbortController();
      const timeout = setTimeout(() => {
        controller.abort();
      }, config.upstreamTimeoutMs);

      // globalThis.Response: the fetch Response — the bare name resolves to
      // Express's Response type imported above.
      let upstream: globalThis.Response;
      try {
        upstream = await fetch(`${config.quotesApiBaseUrl}${path}`, {
          method,
          signal: controller.signal,
          headers: {
            accept: 'application/json',
            authorization: token,
            ...(hasBody ? { 'content-type': 'application/json' } : {}),
          },
          ...(hasBody ? { body: JSON.stringify(req.body ?? {}) } : {}),
        });
      } finally {
        clearTimeout(timeout);
      }

      // Relay status and body verbatim; parse only to re-serialise safely.
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
