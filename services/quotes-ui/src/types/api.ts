/**
 * Client-side types for the BFF contract, with runtime guards at the
 * boundary. Mirrors `server/types` without importing across the builds.
 */

/** A quote ready to render (text normalised by the BFF). */
export interface Quote {
  readonly id: string;
  readonly attribution: string;
  readonly text: string;
}

/** Body returned by the BFF for any failure. */
export interface ErrorResponse {
  readonly error: {
    readonly code: string;
    readonly message: string;
  };
}

/** Outcome of a token request. */
export interface TokenRequestResponse {
  readonly outcome: 'accepted' | 'duplicate' | 'unavailable';
  readonly message: string;
}

/** Narrows a value to {@link Quote}. */
export function isQuote(value: unknown): value is Quote {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    typeof candidate['attribution'] === 'string' &&
    typeof candidate['text'] === 'string'
  );
}

/** Narrows `{quote: Quote}`. */
export function isQuoteEnvelope(value: unknown): value is { readonly quote: Quote } {
  return (
    typeof value === 'object' &&
    value !== null &&
    isQuote((value as Record<string, unknown>)['quote'])
  );
}

/** Narrows `{quotes: Quote[]}` (possibly empty). */
export function isQuotesEnvelope(value: unknown): value is { readonly quotes: readonly Quote[] } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const quotes = (value as Record<string, unknown>)['quotes'];
  return Array.isArray(quotes) && quotes.every(isQuote);
}

/** Narrows `{names: string[]}`. */
export function isNamesEnvelope(value: unknown): value is { readonly names: readonly string[] } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const names = (value as Record<string, unknown>)['names'];
  return Array.isArray(names) && names.every((n) => typeof n === 'string');
}

/** Narrows a token-request response. */
export function isTokenRequestResponse(value: unknown): value is TokenRequestResponse {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    (candidate['outcome'] === 'accepted' ||
      candidate['outcome'] === 'duplicate' ||
      candidate['outcome'] === 'unavailable') &&
    typeof candidate['message'] === 'string'
  );
}

/** Narrows the BFF error body. */
export function isErrorResponse(value: unknown): value is ErrorResponse {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const error = (value as Record<string, unknown>)['error'];
  if (typeof error !== 'object' || error === null) {
    return false;
  }
  const candidate = error as Record<string, unknown>;
  return typeof candidate['code'] === 'string' && typeof candidate['message'] === 'string';
}
