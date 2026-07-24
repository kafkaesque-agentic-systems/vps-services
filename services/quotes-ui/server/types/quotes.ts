/**
 * Types for the upstream Go quotes API.
 *
 * Shapes verified against the live endpoints (2026-07-23):
 *
 *   GET  /quote           -> [ {id, attribution, quote} ]      (array of ONE)
 *   GET  /quote/:id       -> {id, attribution, quote}          (bare object)
 *   GET  /authors         -> {names: string[]}
 *   GET  /authors/:name   -> {names: "<slug>", quotes: [...]}  (names is a STRING here)
 *   POST /quote/search    -> [ {id, attribution, quote} ]
 *   errors                -> {status: number, error: string}
 *
 * Quote text arrives hard-wrapped with embedded newlines (same as the tarot
 * quotes); normalisation is applied in the service layer.
 */

/** One quote document as the upstream returns it. */
export interface UpstreamQuote {
  readonly id: string;
  readonly attribution: string;
  readonly quote: string;
}

/** A quote as served to the browser: text normalised. */
export interface Quote {
  readonly id: string;
  readonly attribution: string;
  readonly text: string;
}

/** Narrows one element to {@link UpstreamQuote}. */
export function isUpstreamQuote(value: unknown): value is UpstreamQuote {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    typeof candidate['attribution'] === 'string' &&
    typeof candidate['quote'] === 'string' &&
    candidate['quote'].length > 0
  );
}

/** Narrows a payload to a non-empty array of quotes. */
export function isUpstreamQuoteArray(value: unknown): value is readonly UpstreamQuote[] {
  return Array.isArray(value) && value.length > 0 && value.every(isUpstreamQuote);
}

/** Narrows a payload to a POSSIBLY EMPTY array of quotes (search results). */
export function isUpstreamQuoteList(value: unknown): value is readonly UpstreamQuote[] {
  return Array.isArray(value) && value.every(isUpstreamQuote);
}

/** Narrows the `/authors` payload. */
export function isUpstreamAuthorList(value: unknown): value is { readonly names: readonly string[] } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const names = (value as Record<string, unknown>)['names'];
  return Array.isArray(names) && names.every((n) => typeof n === 'string');
}

/** Narrows the `/authors/:name` payload (note: `names` is a string here). */
export function isUpstreamAuthorQuotes(
  value: unknown,
): value is { readonly names: string; readonly quotes: readonly UpstreamQuote[] } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['names'] === 'string' &&
    Array.isArray(candidate['quotes']) &&
    candidate['quotes'].every(isUpstreamQuote)
  );
}

/** Narrows the admin token-check payload (`{Result: 0|1}`). */
export function isTokenCheckResult(value: unknown): value is { readonly Result: number } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  return typeof (value as Record<string, unknown>)['Result'] === 'number';
}

/** Error body this BFF returns for any failure. Never leaks internals. */
export interface ErrorResponse {
  readonly error: {
    readonly code: string;
    readonly message: string;
  };
}
