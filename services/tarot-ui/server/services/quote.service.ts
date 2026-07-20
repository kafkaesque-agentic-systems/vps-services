/**
 * Business logic for random quote retrieval.
 *
 * Mirrors the tarot service's shape: owns the upstream HTTP client, validates
 * at the boundary, throws {@link UpstreamError} on failure. The quotes API
 * lives on the same Go server as the tarot API, so it shares the configured
 * base URL and timeout.
 */

import type { AppConfig } from '../config.js';
import { isUpstreamQuoteArray, type Quote } from '../types/quote.js';
import { UpstreamError } from './tarot.service.js';

/**
 * Collapses the upstream's hard-wrapped quote text into a single line.
 *
 * The quotes are stored with fixed-column newlines (e.g. "...but\nfor the
 * sake..."). Those wraps are an artifact of the source data, not semantics;
 * left intact they would force awkward breaks in the browser. Whitespace runs
 * are collapsed to single spaces and the result trimmed.
 */
function normaliseQuoteText(raw: string): string {
  return raw.replace(/\s+/g, ' ').trim();
}

/**
 * Fetches a random quote from the upstream API.
 *
 * @param config - Runtime configuration (base URL and timeout).
 * @returns The quote with normalised text.
 * @throws {UpstreamError} On timeout, transport failure, non-2xx status,
 *   unparseable JSON, or a payload failing validation.
 */
export async function fetchRandomQuote(config: AppConfig): Promise<Quote> {
  const endpoint = `${config.tarotApiBaseUrl}/quote?json`;
  const controller = new AbortController();
  const timeout = setTimeout(() => {
    controller.abort();
  }, config.upstreamTimeoutMs);

  let response: Response;
  try {
    response = await fetch(endpoint, {
      signal: controller.signal,
      headers: { accept: 'application/json' },
    });
  } catch (cause) {
    const timedOut = cause instanceof Error && cause.name === 'AbortError';
    throw new UpstreamError(
      timedOut ? 'UPSTREAM_TIMEOUT' : 'UPSTREAM_UNREACHABLE',
      timedOut
        ? `quotes upstream did not respond within ${config.upstreamTimeoutMs}ms`
        : 'quotes upstream could not be reached',
      { cause },
    );
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    throw new UpstreamError('UPSTREAM_STATUS', `quotes upstream returned HTTP ${response.status}`);
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch (cause) {
    throw new UpstreamError('UPSTREAM_MALFORMED', 'quotes upstream returned invalid JSON', {
      cause,
    });
  }

  if (!isUpstreamQuoteArray(payload)) {
    throw new UpstreamError(
      'UPSTREAM_MALFORMED',
      'quotes upstream payload did not match the expected one-element array shape',
    );
  }

  const [first] = payload;
  return {
    id: first.id,
    attribution: first.attribution.trim(),
    text: normaliseQuoteText(first.quote),
  };
}
