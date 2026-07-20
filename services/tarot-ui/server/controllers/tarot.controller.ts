/**
 * HTTP shaping for the tarot routes.
 *
 * Controllers translate between HTTP and the service layer and do nothing else:
 * no upstream calls, no business rules. Errors are delegated to the centralised
 * handler via `next` so that every failure is formatted in exactly one place.
 */

import type { NextFunction, Request, Response } from 'express';

import type { AppConfig } from '../config.js';
import { fetchRandomCard } from '../services/tarot.service.js';
import type { CardResponse } from '../types/tarot.js';

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
