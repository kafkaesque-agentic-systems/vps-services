/**
 * Types for the upstream quotes API.
 *
 * Verified against the live endpoint (2026-07-20): `GET /quote?json` returns a
 * one-element ARRAY, not a bare object, and the author field is named
 * `attribution`:
 *
 *   [{"id":"633a...","attribution":"epictetus","quote":"Know you not..."}]
 *
 * Quote text arrives hard-wrapped with embedded newlines; normalisation is a
 * display concern and happens in the service layer, not here.
 */

/** One quote document exactly as the upstream returns it. */
export interface UpstreamQuote {
  readonly id: string;
  readonly attribution: string;
  readonly quote: string;
}

/** A quote as served to the browser: text normalised, field names stable. */
export interface Quote {
  readonly id: string;
  /** Author name as stored upstream (lowercase); casing is a client concern. */
  readonly attribution: string;
  /** Quote text with upstream hard-wrap newlines collapsed to single spaces. */
  readonly text: string;
}

/**
 * Narrows an untrusted payload to a one-element array of {@link UpstreamQuote}.
 *
 * The upstream wraps the single random quote in an array; an empty array or a
 * malformed element both fail the guard rather than producing a partial quote.
 *
 * @param value - Parsed JSON of unknown shape.
 * @returns `true` when `value[0]` is a well-formed quote document.
 */
export function isUpstreamQuoteArray(value: unknown): value is readonly [UpstreamQuote] {
  if (!Array.isArray(value) || value.length === 0) {
    return false;
  }
  const first: unknown = value[0];
  if (typeof first !== 'object' || first === null) {
    return false;
  }
  const candidate = first as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    candidate['id'].length > 0 &&
    typeof candidate['attribution'] === 'string' &&
    candidate['attribution'].length > 0 &&
    typeof candidate['quote'] === 'string' &&
    candidate['quote'].length > 0
  );
}
