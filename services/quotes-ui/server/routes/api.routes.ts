/**
 * API route wiring. HTTP paths only -- no logic lives here.
 */

import { Router } from 'express';

import type { AppConfig } from '../config.js';
import {
  makeGetAuthors,
  makeGetQuoteById,
  makeGetQuotesByAuthor,
  makeGetRandomQuote,
  makeSearch,
  makeWritePassthrough,
} from '../controllers/quotes.controller.js';
import { makeTarotPassthrough } from '../controllers/tarot.controller.js';
import { makeTokenRequest } from '../controllers/token.controller.js';
import { makeRateLimiter } from '../middleware/rateLimit.js';

/** Builds the router mounted at `{basePath}api`. */
export function createApiRouter(config: AppConfig): Router {
  const router = Router();

  // Read paths.
  router.get('/quote', makeGetRandomQuote(config));
  router.get('/quote/:id', makeGetQuoteById(config));
  router.get('/authors', makeGetAuthors(config));
  router.get('/authors/:name', makeGetQuotesByAuthor(config));
  router.post('/search', makeSearch(config));

  // Token-gated write pass-throughs (caller supplies their own token).
  router.post('/quote', makeWritePassthrough(config, 'POST'));
  router.put('/quote/:id', makeWritePassthrough(config, 'PUT'));
  router.delete('/quote/:id', makeWritePassthrough(config, 'DELETE'));

  // Tarot surface: public, relayed verbatim for the API reference.
  router.get('/tarot/card', makeTarotPassthrough(config, 'GET', '/tarot/card'));
  router.get('/tarot/deck', makeTarotPassthrough(config, 'GET', '/tarot/deck'));
  router.get('/tarot/deck/:id', makeTarotPassthrough(config, 'GET', '/tarot/deck/:id'));
  router.get('/tarot/decks', makeTarotPassthrough(config, 'GET', '/tarot/decks'));
  router.post('/tarot/spread', makeTarotPassthrough(config, 'POST', '/tarot/spread'));

  // Token request: honeypot in the controller, 5 attempts/hour/IP here.
  router.post('/token-request', makeRateLimiter(5, 60 * 60 * 1000), makeTokenRequest(config));

  return router;
}
