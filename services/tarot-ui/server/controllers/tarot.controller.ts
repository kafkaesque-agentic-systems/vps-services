/**
 * HTTP shaping for the tarot routes.
 *
 * Controllers translate between HTTP and the service layer and do nothing else:
 * no upstream calls, no business rules. Errors are delegated to the centralised
 * handler via `next` so that every failure is formatted in exactly one place.
 */

import type { NextFunction, Request, Response } from 'express';

import type { AppConfig } from '../config.js';
import { logger } from '../logger.js';
import { fetchRandomQuote } from '../services/quote.service.js';
import { fetchDeckNames, fetchSpread } from '../services/spread.service.js';
import { fetchRandomCard } from '../services/tarot.service.js';
import type { DeckListResponse, SpreadDrawRequest } from '../types/spread.js';
import type { CardResponse, ErrorResponse, ReadingResponse } from '../types/tarot.js';

/**
 * Maximum positions accepted for a custom spread.
 *
 * The Go API allows 78, but a 78-card wall of thumbnails is a UI failure, so
 * the product ceiling is 10 -- the size of a Celtic Cross. Enforced here as
 * well as in the client, because the client is advisory and this boundary is
 * not.
 */
const MAX_SPREAD_POSITIONS = 10;

/** Longest accepted spread name / position label, defence against abuse. */
const MAX_NAME_LENGTH = 100;
const MAX_POSITION_LENGTH = 40;
const MAX_DECK_LENGTH = 64;

/**
 * Validates and normalises an untrusted request body into a
 * {@link SpreadDrawRequest}.
 *
 * Positions are trimmed and must be unique case-insensitively: the upstream
 * keys its response map by position, so duplicates would silently collapse
 * into a single entry and the reading would come back short.
 *
 * @param body - Parsed request body of unknown shape.
 * @returns The normalised request, or a human-readable rejection reason.
 */
function parseSpreadBody(body: unknown): SpreadDrawRequest | { readonly reason: string } {
  if (typeof body !== 'object' || body === null) {
    return { reason: 'request body must be a JSON object' };
  }
  const candidate = body as Record<string, unknown>;

  const rawName = candidate['name'];
  if (rawName !== undefined && typeof rawName !== 'string') {
    return { reason: 'name must be a string' };
  }
  const name = (rawName ?? '').trim().slice(0, MAX_NAME_LENGTH) || 'Custom Spread';

  const rawDeck = candidate['deck'];
  if (typeof rawDeck !== 'string' || rawDeck.trim() === '' || rawDeck.length > MAX_DECK_LENGTH) {
    return { reason: 'deck must be a non-empty string' };
  }
  const deck = rawDeck.trim();

  const rawPositions = candidate['positions'];
  if (!Array.isArray(rawPositions)) {
    return { reason: 'positions must be an array' };
  }
  if (rawPositions.length === 0 || rawPositions.length > MAX_SPREAD_POSITIONS) {
    return { reason: `positions must contain between 1 and ${MAX_SPREAD_POSITIONS} entries` };
  }

  const positions: string[] = [];
  const seen = new Set<string>();
  for (const entry of rawPositions) {
    if (typeof entry !== 'string') {
      return { reason: 'every position must be a string' };
    }
    const position = entry.trim();
    if (position === '' || position.length > MAX_POSITION_LENGTH) {
      return { reason: `positions must be 1-${MAX_POSITION_LENGTH} characters` };
    }
    const key = position.toLowerCase();
    if (seen.has(key)) {
      return { reason: `duplicate position "${position}" - position names must be unique` };
    }
    seen.add(key);
    positions.push(position);
  }

  return { name, deck, positions };
}

/** Express handler signature used by this module. */
export type Handler = (req: Request, res: Response, next: NextFunction) => Promise<void> | void;

/**
 * Builds the `GET {basePath}/api/card` handler.
 *
 * Configuration is injected rather than imported so the handler stays pure with
 * respect to the environment and is trivially testable.
 *
 * @param config - Runtime configuration.
 * @returns An Express handler returning a {@link CardResponse}.
 */
export function makeGetRandomCard(config: AppConfig): Handler {
  return async (_req: Request, res: Response, next: NextFunction): Promise<void> => {
    try {
      const card = await fetchRandomCard(config);
      const body: CardResponse = { card };

      // A draw is random by definition: caching it would defeat the feature.
      res.setHeader('Cache-Control', 'no-store');
      res.status(200).json(body);
    } catch (error) {
      next(error);
    }
  };
}

/**
 * Builds the `GET {basePath}/api/reading` handler: card plus quote.
 *
 * The two upstream calls run CONCURRENTLY -- mirroring the Go tarot client's
 * asynchronous card/quote pattern -- so the reading costs one round trip's
 * latency, not two.
 *
 * Failure handling is deliberately asymmetric. The card is the feature: if it
 * fails, the request fails through the normal error path. The quote is
 * garnish: if it fails, the reading is served with `quote: null` and the
 * failure is logged at warn, because refusing to show a card over a missing
 * caption would invert the feature's priorities.
 *
 * @param config - Runtime configuration.
 * @returns An Express handler returning a {@link ReadingResponse}.
 */
export function makeGetReading(config: AppConfig): Handler {
  return async (_req: Request, res: Response, next: NextFunction): Promise<void> => {
    try {
      const [card, quoteResult] = await Promise.all([
        fetchRandomCard(config),
        // The catch keeps a quote failure from rejecting the combined await;
        // the card promise stays fatal by design.
        fetchRandomQuote(config).catch((error: unknown) => {
          logger.warn('quote fetch failed; serving reading without quote', {
            reason: error instanceof Error ? error.message : String(error),
          });
          return null;
        }),
      ]);

      const body: ReadingResponse = { card, quote: quoteResult };
      res.setHeader('Cache-Control', 'no-store');
      res.status(200).json(body);
    } catch (error) {
      next(error);
    }
  };
}

/**
 * Builds the `POST {basePath}/api/spread` handler.
 *
 * Validation happens HERE, at this service's own boundary, rather than by
 * forwarding junk upstream and relaying whatever Gin says: the rejection
 * messages stay meaningful to this UI's vocabulary, and malformed bodies never
 * leave the process.
 *
 * @param config - Runtime configuration.
 * @returns An Express handler returning a spread reading with cards in
 *   request-position order.
 */
export function makePostSpread(config: AppConfig): Handler {
  return async (req: Request, res: Response, next: NextFunction): Promise<void> => {
    try {
      const parsed = parseSpreadBody(req.body);
      if ('reason' in parsed) {
        const body: ErrorResponse = {
          error: { code: 'INVALID_SPREAD', message: parsed.reason },
        };
        res.status(400).json(body);
        return;
      }

      const reading = await fetchSpread(config, parsed);
      res.setHeader('Cache-Control', 'no-store');
      res.status(200).json(reading);
    } catch (error) {
      next(error);
    }
  };
}

/**
 * Builds the `GET {basePath}/api/decks` handler.
 *
 * The catalogue changes rarely, so responses are cacheable for an hour --
 * unlike the draws, which are `no-store` by nature.
 *
 * @param config - Runtime configuration.
 * @returns An Express handler returning the sorted deck names.
 */
export function makeGetDecks(config: AppConfig): Handler {
  return async (_req: Request, res: Response, next: NextFunction): Promise<void> => {
    try {
      const names = await fetchDeckNames(config);
      const body: DeckListResponse = { names };
      res.setHeader('Cache-Control', 'public, max-age=3600');
      res.status(200).json(body);
    } catch (error) {
      next(error);
    }
  };
}
