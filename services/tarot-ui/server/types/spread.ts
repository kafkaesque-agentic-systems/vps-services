/**
 * Types for the custom spread flow.
 *
 * Contract facts verified against the Go source and the live endpoint
 * (2026-07-20):
 *
 *   - `POST /tarot/spread` accepts `{name, deck, positions}` where deck is a
 *     deck name, `"any"` (one random deck, drawn without replacement) or
 *     `"all"` (each position sampled from an independent random deck).
 *   - The response is `{name, spread}` where `spread` is a Go MAP keyed by
 *     position -- Gin marshals map keys ALPHABETICALLY, so the client's
 *     position ORDER IS DESTROYED in transit. The BFF re-zips the map back
 *     into an ordered array using the request's own positions list.
 *   - Card values carry the same unreachable image host as /tarot/card and
 *     need the standard rewrite.
 *   - `GET /tarot/decks` returns `{names: string[]}` (69 decks).
 */

/** A spread request as this BFF forwards it upstream. */
export interface SpreadDrawRequest {
  readonly name: string;
  readonly deck: string;
  readonly positions: readonly string[];
}

/** One resolved position in a reading, image URL already corrected. */
export interface SpreadCard {
  readonly position: string;
  readonly imageUrl: string;
}

/**
 * Successful `POST {basePath}/api/spread` body.
 *
 * `cards` preserves the REQUEST's position order -- the upstream map cannot.
 */
export interface SpreadReading {
  readonly name: string;
  readonly cards: readonly SpreadCard[];
}

/** Successful `GET {basePath}/api/decks` body. */
export interface DeckListResponse {
  readonly names: readonly string[];
}

/** The upstream reading exactly as the Go API returns it. */
export interface UpstreamReading {
  readonly name: string;
  readonly spread: Readonly<Record<string, string>>;
}

/**
 * Narrows an untrusted payload to {@link UpstreamReading}.
 *
 * Every value in `spread` must be a non-empty string; a single malformed entry
 * fails the whole payload rather than producing a partially valid reading.
 */
export function isUpstreamReading(value: unknown): value is UpstreamReading {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  if (typeof candidate['name'] !== 'string') {
    return false;
  }
  const spread = candidate['spread'];
  if (typeof spread !== 'object' || spread === null || Array.isArray(spread)) {
    return false;
  }
  return Object.values(spread).every((v) => typeof v === 'string' && v.length > 0);
}

/** Narrows an untrusted payload to the upstream deck list shape. */
export function isUpstreamDeckList(value: unknown): value is { readonly names: readonly unknown[] } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  return Array.isArray((value as Record<string, unknown>)['names']);
}
