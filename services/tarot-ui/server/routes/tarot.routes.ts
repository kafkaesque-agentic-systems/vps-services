/**
 * Tarot route wiring. HTTP paths only -- no logic lives here.
 */

import { Router } from 'express';

import type { AppConfig } from '../config.js';
import {
  makeGetDecks,
  makeGetRandomCard,
  makeGetReading,
  makePostSpread,
} from '../controllers/tarot.controller.js';

/**
 * Builds the router mounted at `{basePath}/api`.
 *
 * @param config - Runtime configuration, forwarded to the controllers.
 */
export function createTarotRouter(config: AppConfig): Router {
  const router = Router();
  router.get('/card', makeGetRandomCard(config));
  // Card + concurrent random quote; the single-card page draws from this.
  router.get('/reading', makeGetReading(config));
  // Custom spread: validated here, forwarded upstream, re-ordered on return.
  router.post('/spread', makePostSpread(config));
  // Deck catalogue for the spread composer's selector.
  router.get('/decks', makeGetDecks(config));
  return router;
}
