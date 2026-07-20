/**
 * Tarot route wiring. HTTP paths only -- no logic lives here.
 */

import { Router } from 'express';

import type { AppConfig } from '../config.js';
import { makeGetRandomCard } from '../controllers/tarot.controller.js';

/**
 * Builds the router mounted at `{basePath}/api`.
 *
 * @param config - Runtime configuration, forwarded to the controller.
 */
export function createTarotRouter(config: AppConfig): Router {
  const router = Router();
  router.get('/card', makeGetRandomCard(config));
  return router;
}
