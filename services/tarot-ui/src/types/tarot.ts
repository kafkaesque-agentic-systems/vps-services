/**
 * Client-side types for the BFF contract.
 *
 * These mirror `server/types/tarot.ts`. They are declared separately rather
 * than imported across the client/server boundary so the two builds stay
 * independent, and because the client must treat the response as untrusted
 * input regardless of who produced it.
 */

/** A card ready to render. `imageUrl` is already corrected by the BFF. */
export interface TarotCard {
  readonly id: string;
  readonly deck: string;
  readonly imageUrl: string;
}

/** Body of a successful `GET /tarot/api/card`. */
export interface CardResponse {
  readonly card: TarotCard;
}

/** A quote accompanying a reading. Text arrives normalised by the BFF. */
export interface Quote {
  readonly id: string;
  /** Author name as stored upstream (lowercase); cased for display client-side. */
  readonly attribution: string;
  readonly text: string;
}

/**
 * Body of a successful `GET /tarot/api/reading`.
 *
 * `quote` is `null` when the quotes upstream failed -- the card still renders
 * and the page simply omits the caption.
 */
export interface ReadingResponse {
  readonly card: TarotCard;
  readonly quote: Quote | null;
}

/** Body returned by the BFF for any failure. */
export interface ErrorResponse {
  readonly error: {
    readonly code: string;
    readonly message: string;
  };
}

/**
 * Narrows an untrusted payload to {@link CardResponse}.
 *
 * @param value - Parsed JSON of unknown shape.
 * @returns `true` when every required field is a non-empty string.
 */
export function isCardResponse(value: unknown): value is CardResponse {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const card = (value as Record<string, unknown>)['card'];
  if (typeof card !== 'object' || card === null) {
    return false;
  }
  const candidate = card as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    candidate['id'].length > 0 &&
    typeof candidate['deck'] === 'string' &&
    candidate['deck'].length > 0 &&
    typeof candidate['imageUrl'] === 'string' &&
    candidate['imageUrl'].length > 0
  );
}

/** Narrows a value to {@link Quote}. */
function isQuote(value: unknown): value is Quote {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    candidate['id'].length > 0 &&
    typeof candidate['attribution'] === 'string' &&
    candidate['attribution'].length > 0 &&
    typeof candidate['text'] === 'string' &&
    candidate['text'].length > 0
  );
}

/**
 * Narrows an untrusted payload to {@link ReadingResponse}.
 *
 * Reuses {@link isCardResponse} for the card half; the quote half accepts
 * either a well-formed quote or an explicit `null`.
 */
export function isReadingResponse(value: unknown): value is ReadingResponse {
  if (!isCardResponse(value)) {
    return false;
  }
  // isCardResponse narrowed `value` to CardResponse, which has no index
  // signature -- go back through `unknown` to inspect the extra field.
  const quote = (value as unknown as Record<string, unknown>)['quote'];
  return quote === null || isQuote(quote);
}

/** Narrows an untrusted payload to {@link ErrorResponse}. */
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
