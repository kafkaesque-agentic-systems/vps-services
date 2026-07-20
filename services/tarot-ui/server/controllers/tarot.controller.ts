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
import { fetchRandomCard } from '../services/tarot.service.js';
import type { CardResponse, ReadingResponse } from '../types/tarot.js';

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
