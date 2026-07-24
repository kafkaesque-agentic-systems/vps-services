/**
 * Business logic for custom spreads and the deck catalogue.
 *
 * Follows the established service shape: owns the upstream HTTP calls,
 * validates at the boundary, throws {@link UpstreamError} on failure, and
 * applies the image-URL correction so the browser never sees an unreachable
 * host.
 */

import type { AppConfig } from '../config.js';
import {
  isUpstreamDeckList,
  isUpstreamReading,
  type SpreadCard,
  type SpreadDrawRequest,
  type SpreadReading,
} from '../types/spread.js';
import { UpstreamError, correctImageUrl } from './tarot.service.js';

/**
 * Shared fetch wrapper: bounded by the configured timeout, JSON-parsed, and
 * guard-validated. Exists because three upstream calls were about to repeat
 * the same abort/parse/guard boilerplate.
 *
 * @param config - Runtime configuration (timeout).
 * @param endpoint - Absolute upstream URL.
 * @param guard - Type guard the parsed payload must satisfy.
 * @param init - Extra fetch options (method, headers, body).
 * @returns The validated payload.
 * @throws {UpstreamError} On timeout, transport failure, non-2xx status,
 *   unparseable JSON, or a payload failing the guard.
 */
async function upstreamJson<T>(
  config: AppConfig,
  endpoint: string,
  guard: (value: unknown) => value is T,
  init?: RequestInit,
): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => {
    controller.abort();
  }, config.upstreamTimeoutMs);

  let response: Response;
  try {
    response = await fetch(endpoint, {
      ...init,
      signal: controller.signal,
      headers: {
        accept: 'application/json',
        ...(config.apiServiceToken === null ? {} : { authorization: config.apiServiceToken }),
        ...init?.headers,
      },
    });
  } catch (cause) {
    const timedOut = cause instanceof Error && cause.name === 'AbortError';
    throw new UpstreamError(
      timedOut ? 'UPSTREAM_TIMEOUT' : 'UPSTREAM_UNREACHABLE',
      timedOut
        ? `upstream did not respond within ${config.upstreamTimeoutMs}ms`
        : 'upstream tarot API could not be reached',
      { cause },
    );
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    throw new UpstreamError('UPSTREAM_STATUS', `upstream returned HTTP ${response.status}`);
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch (cause) {
    throw new UpstreamError('UPSTREAM_MALFORMED', 'upstream returned invalid JSON', { cause });
  }

  if (!guard(payload)) {
    throw new UpstreamError(
      'UPSTREAM_MALFORMED',
      'upstream payload did not match the expected shape',
    );
  }

  return payload;
}

/**
 * Draws a custom spread from the upstream API.
 *
 * The upstream response keys cards by position in a MAP, which destroys the
 * client's ordering (Gin marshals map keys alphabetically). The request's own
 * positions list is the only surviving record of the intended order, so the
 * map is re-zipped here into an ordered array. A position missing from the
 * response fails loudly rather than shipping a reading with silent holes.
 *
 * @param config - Runtime configuration.
 * @param request - The validated spread request (validation is the
 *   controller's job; this service trusts its caller).
 * @returns The reading with cards in request-position order and corrected
 *   image URLs.
 * @throws {UpstreamError} On any transport, status, or shape failure.
 */
export async function fetchSpread(
  config: AppConfig,
  request: SpreadDrawRequest,
): Promise<SpreadReading> {
  const payload = await upstreamJson(
    config,
    `${config.tarotApiBaseUrl}/tarot/spread`,
    isUpstreamReading,
    {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({
        name: request.name,
        deck: request.deck,
        positions: request.positions,
      }),
    },
  );

  const cards: SpreadCard[] = [];
  for (const position of request.positions) {
    const raw = payload.spread[position];
    if (raw === undefined) {
      throw new UpstreamError(
        'UPSTREAM_MALFORMED',
        `spread response is missing the "${position}" position`,
      );
    }
    cards.push({ position, imageUrl: correctImageUrl(raw, config) });
  }

  return { name: payload.name, cards };
}

/**
 * Fetches the deck catalogue for the composer's deck selector.
 *
 * Non-string entries (the upstream field is `[]interface{}` on the Go side)
 * are dropped rather than failing the list; the result is sorted for a stable
 * presentation.
 *
 * @param config - Runtime configuration.
 * @returns Alphabetically sorted deck names.
 * @throws {UpstreamError} On any transport, status, or shape failure.
 */
export async function fetchDeckNames(config: AppConfig): Promise<readonly string[]> {
  const payload = await upstreamJson(
    config,
    `${config.tarotApiBaseUrl}/tarot/decks`,
    isUpstreamDeckList,
  );

  return payload.names
    .filter((name): name is string => typeof name === 'string' && name.length > 0)
    .sort((a, b) => a.localeCompare(b));
}
