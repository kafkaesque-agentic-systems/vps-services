/**
 * HTTP shaping for the token-request workflow.
 *
 * Bot defence is honeypot + rate limit (the limiter is wired in the routes):
 * the form carries a `website` field rendered invisibly in the client; humans
 * leave it empty, naive bots fill it. A filled honeypot returns the SAME
 * success response as a real request — giving a bot no signal it was caught.
 */

import type { Request, Response } from 'express';

import type { AppConfig } from '../config.js';
import { logger } from '../logger.js';
import { registerTokenRequest } from '../services/token.service.js';
import type { ErrorResponse } from '../types/quotes.js';
import type { Handler } from './quotes.controller.js';

/** Pragmatic e-mail shape check; the mail flow is the real validator. */
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Response body for the three workflow outcomes. */
interface TokenRequestResponse {
  readonly outcome: 'accepted' | 'duplicate' | 'unavailable';
  readonly message: string;
}

/** Copy matches the Flask tier's flash messages in spirit. */
const MESSAGES: Record<TokenRequestResponse['outcome'], string> = {
  accepted:
    'We have received your request. It will be reviewed by a team member and a response will be sent out soon.',
  duplicate:
    'This email has already requested a token. Please be patient while we review your request.',
  unavailable: 'There was an internal error processing your request. Please try again later.',
};

/** `POST /api/token-request` — request an API token by email. */
export function makeTokenRequest(config: AppConfig): Handler {
  return async (req: Request, res: Response): Promise<void> => {
    const body = (req.body ?? {}) as Record<string, unknown>;
    const email = typeof body['email'] === 'string' ? body['email'].trim() : '';
    const honeypot = typeof body['website'] === 'string' ? body['website'].trim() : '';

    if (honeypot !== '') {
      // Bot: pretend success, register nothing, log for observability.
      logger.warn('token-request honeypot tripped', { ip: req.ip ?? 'unknown' });
      const response: TokenRequestResponse = { outcome: 'accepted', message: MESSAGES.accepted };
      res.status(200).json(response);
      return;
    }

    if (email === '' || email.length > 254 || !EMAIL.test(email)) {
      const errorBody: ErrorResponse = {
        error: { code: 'INVALID_EMAIL', message: 'Please provide a valid email address.' },
      };
      res.status(400).json(errorBody);
      return;
    }

    const outcome = await registerTokenRequest(config, email);
    const response: TokenRequestResponse = { outcome, message: MESSAGES[outcome] };
    res.status(outcome === 'unavailable' ? 503 : 200).json(response);
  };
}
