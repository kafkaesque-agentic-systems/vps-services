/**
 * Type definitions for the upstream tarot API and this service's own contract.
 *
 * The upstream response is untrusted input: it crosses a process boundary, so
 * it arrives as `unknown` and is narrowed by an explicit guard before any field
 * is read. `any` is banned project-wide, which makes that narrowing mandatory
 * rather than optional.
 */

/**
 * A single card exactly as the Go API returns it.
 *
 * `card` is a fully-qualified image URL, but the upstream emits a host that
 * does not resolve; {@link TarotCard} carries the corrected value.
 */
export interface UpstreamCard {
  readonly id: string;
  readonly deck: string;
  readonly card: string;
}

/** A card after image-URL correction, as served to the browser. */
export interface TarotCard {
  /** MongoDB ObjectId of the source deck document. */
  readonly id: string;
  /** Deck name, e.g. `hustling`. */
  readonly deck: string;
  /** Reachable absolute image URL. */
  readonly imageUrl: string;
}

/** Successful `GET {basePath}/api/card` body. */
export interface CardResponse {
  readonly card: TarotCard;
}

/** Error body returned by every failing route. Never leaks internals. */
export interface ErrorResponse {
  readonly error: {
    /** Stable, machine-readable code for the client to branch on. */
    readonly code: string;
    /** Human-readable message, safe to display. */
    readonly message: string;
  };
}

/**
 * Type guard narrowing an untrusted upstream payload to {@link UpstreamCard}.
 *
 * Validates that every field is present and is a non-empty string. Returning
 * `false` rather than throwing lets the caller decide how to report the
 * failure, and keeps the guard usable in conditional expressions.
 *
 * @param value - Parsed JSON of unknown shape.
 * @returns `true` if `value` is a well-formed upstream card.
 */
export function isUpstreamCard(value: unknown): value is UpstreamCard {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as Record<string, unknown>;
  return (
    typeof candidate['id'] === 'string' &&
    candidate['id'].length > 0 &&
    typeof candidate['deck'] === 'string' &&
    candidate['deck'].length > 0 &&
    typeof candidate['card'] === 'string' &&
    candidate['card'].length > 0
  );
}
