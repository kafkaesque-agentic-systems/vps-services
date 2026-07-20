/**
 * Tarot route wiring. HTTP paths only -- no logic lives here.
 */

import { Router } from 'express';

import type { AppConfig } from '../config.js';
import { makeGetRandomCard, makeGetReading } from '../controllers/tarot.controller.js';

/**
 * Builds the router mounted at `{basePath}/api`.
 *
 * @param config - Runtime configuration, forwarded to the controllers.
 */
export function createTarotRouter(config: AppConfig): Router {
  const router = Router();
  router.get('/card', makeGetRandomCard(config));
  // Card + concurrent random quote; the primary endpoint the UI draws from.
  router.get('/reading', makeGetReading(config));
  return router;
}
