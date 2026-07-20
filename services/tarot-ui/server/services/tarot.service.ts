/**
 * Business logic for tarot card retrieval.
 *
 * This layer owns the upstream HTTP client and the image-URL correction. It
 * knows nothing about Express: it takes configuration, returns domain objects,
 * and throws {@link UpstreamError} on failure.
 */

import type { AppConfig } from '../config.js';
import { isUpstreamCard, type TarotCard } from '../types/tarot.js';

/** Raised when the upstream tarot API cannot be reached or returns junk. */
export class UpstreamError extends Error {
  public override readonly name = 'UpstreamError';
  /** Stable code surfaced to the client in the error body. */
  public readonly code: string;

  public constructor(code: string, message: string, options?: { cause?: unknown }) {
    super(message, options);
    this.code = code;
  }
}

/**
 * Rewrites a card image URL emitted by the upstream API into one that resolves.
 *
 * The Go API returns image URLs rooted at a host that does not resolve, e.g.
 * `https://thirdeye.live/static/img/tarot/hustling/15.jpg`. The bytes are
 * actually served from `https://api.thirdeye.live/image/...`. Only the prefix
 * differs; the `/tarot/<deck>/<n>.jpg` tail is already correct.
 *
 * The correction is applied here, server-side, rather than in the browser, so
 * that exactly one place in the system knows about the discrepancy and the
 * client never receives a URL it cannot load.
 *
 * A URL already pointing at the public prefix is returned unchanged, which
 * keeps this safe to apply twice and correct on the day the upstream is fixed.
 *
 * @param rawUrl - The `card` field exactly as the upstream returned it.
 * @param config - Supplies the broken and public prefixes.
 * @returns An absolute, reachable image URL.
 * @throws {UpstreamError} If the URL matches neither prefix, since silently
 *   forwarding an unrecognised URL would render as a broken image with no
 *   explanation anywhere.
 */
export function correctImageUrl(rawUrl: string, config: AppConfig): string {
  if (rawUrl.startsWith(config.publicImagePrefix)) {
    return rawUrl;
  }

  if (rawUrl.startsWith(config.upstreamImagePrefix)) {
    const tail = rawUrl.slice(config.upstreamImagePrefix.length);
    return `${config.publicImagePrefix}${tail.startsWith('/') ? '' : '/'}${tail}`;
  }

  throw new UpstreamError(
    'UNEXPECTED_IMAGE_URL',
    `upstream image URL matched neither the known-broken prefix ` +
      `(${config.upstreamImagePrefix}) nor the public prefix ` +
      `(${config.publicImagePrefix})`,
  );
}

/**
 * Fetches a random card from the upstream API and returns it corrected.
 *
 * @param config - Runtime configuration.
 * @returns A card whose `imageUrl` is guaranteed reachable.
 * @throws {UpstreamError} On timeout, transport failure, non-2xx status,
 *   unparseable JSON, a payload failing validation, or an unrecognised
 *   image URL.
 */
export async function fetchRandomCard(config: AppConfig): Promise<TarotCard> {
  const endpoint = `${config.tarotApiBaseUrl}/tarot/card`;
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
    // AbortError is the timeout above; anything else is a transport failure.
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
    throw new UpstreamError(
      'UPSTREAM_STATUS',
      `upstream tarot API returned HTTP ${response.status}`,
    );
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch (cause) {
    throw new UpstreamError('UPSTREAM_MALFORMED', 'upstream returned invalid JSON', { cause });
  }

  if (!isUpstreamCard(payload)) {
    throw new UpstreamError(
      'UPSTREAM_MALFORMED',
      'upstream payload did not match the expected card shape',
    );
  }

  return {
    id: payload.id,
    deck: payload.deck,
    imageUrl: correctImageUrl(payload.card, config),
  };
}
