/**
 * Token-request workflow: the one feature with real business logic.
 *
 * Flow (parity with the Flask tier):
 *   1. check/register the email via `GET /admin/tokens/:email` using the
 *      server-held AUTHORIZED admin secret — the browser NEVER sees it;
 *   2. Result 0 = newly registered -> notify the admin by email;
 *      Result 1 = already requested -> polite rejection;
 *   3. any transport/shape failure -> "try again later".
 *
 * The email travels percent-encoded and untransformed (the Flask C-9 fix is
 * preserved). Notification mail uses nodemailer with native DKIM signing,
 * replacing the hand-rolled Python SMTP module; a missing DKIM key degrades
 * to unsigned mail with a warning, never a failed request.
 */

import { readFile } from 'node:fs/promises';

import nodemailer from 'nodemailer';

import type { AppConfig } from '../config.js';
import { logger } from '../logger.js';
import { isTokenCheckResult } from '../types/quotes.js';
import { upstreamJson, UpstreamError } from './quotes.service.js';

/** Outcome of a token request, mapped to user-facing copy by the client. */
export type TokenRequestOutcome = 'accepted' | 'duplicate' | 'unavailable';

/** Path the DKIM private key is mounted at in the container (optional). */
const DKIM_KEY_PATH = '/etc/dkim/thirdeye.live.omail.pem';

/**
 * Checks/registers a token request for `email`.
 *
 * @returns The workflow outcome; never throws — every failure maps to
 *   `unavailable`, mirroring the Flask tier's fail-closed sentinel.
 */
export async function registerTokenRequest(
  config: AppConfig,
  email: string,
): Promise<TokenRequestOutcome> {
  if (config.adminToken === null) {
    // Feature disabled (no AUTHORIZED secret in this environment).
    logger.warn('token request received but AUTHORIZED is not configured');
    return 'unavailable';
  }

  let result: number;
  try {
    const payload = await upstreamJson(
      config,
      `${config.quotesApiBaseUrl}/admin/tokens/${encodeURIComponent(email)}`,
      isTokenCheckResult,
      { headers: { authorization: config.adminToken } },
    );
    result = payload.Result;
  } catch (error) {
    logger.error(
      'token-request upstream check failed',
      error instanceof UpstreamError ? error : new Error(String(error)),
    );
    return 'unavailable';
  }

  if (result === 1) {
    return 'duplicate';
  }

  // Newly registered: notify the admin. Fire-and-forget with logging — the
  // user's request already succeeded; mail failure is an operator concern.
  void sendNotification(config, email);
  return 'accepted';
}

/** Sends the admin notification email, logging (never throwing) failures. */
async function sendNotification(config: AppConfig, email: string): Promise<void> {
  if (config.mail === null) {
    logger.warn('token request registered but mail is not configured', { email });
    return;
  }

  let dkim: { domainName: string; keySelector: string; privateKey: string } | undefined;
  try {
    const privateKey = await readFile(DKIM_KEY_PATH, 'utf8');
    dkim = { domainName: 'thirdeye.live', keySelector: 'omail', privateKey };
  } catch {
    logger.warn('DKIM key unavailable; sending unsigned notification');
  }

  try {
    const transport = nodemailer.createTransport({
      host: config.mail.host,
      port: 587,
      secure: false,
      auth: { user: config.mail.username, pass: config.mail.password },
      ...(dkim === undefined ? {} : { dkim }),
    });

    await transport.sendMail({
      to: config.mail.to,
      from: config.mail.from,
      subject: 'API Token Request Alert',
      text: `Notification of token request. Please review and respond to ${email}`,
    });
    logger.info('token-request notification sent', { forEmail: email });
  } catch (error) {
    logger.error('token-request notification failed', error, { forEmail: email });
  }
}
