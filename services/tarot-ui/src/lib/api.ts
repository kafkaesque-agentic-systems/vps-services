/**
 * BFF client.
 *
 * The browser talks only to this app's own origin: the upstream tarot API's
 * address is never present in shipped JavaScript. Every response is validated
 * before use -- a 200 with an unexpected body is treated as a failure, not
 * rendered blindly.
 */

import { isCardResponse, isErrorResponse, type TarotCard } from '../types/tarot.js';

/**
 * Base path the app is mounted at.
 *
 * Vite substitutes `import.meta.env.BASE_URL` at build time from the `base`
 * option, so the client and server agree on the mount point without it being
 * hard-coded in two places.
 */
const API_BASE = `${import.meta.env.BASE_URL.replace(/\/$/, '')}/api`;

/** A failure that carries a message safe to display to the user. */
export class ApiError extends Error {
  public override readonly name = 'ApiError';
  public readonly code: string;

  public constructor(code: string, message: string) {
    super(message);
    this.code = code;
  }
}

/**
 * Preloads and decodes an image so it can be revealed without a blank frame.
 *
 * Without this the fade-in begins before the bytes have arrived and the user
 * watches an empty rectangle brighten. `decode()` resolves only once the image
 * is ready to paint. A decode failure is non-fatal: the caller still renders,
 * and the browser retries the load through the `<img>` element itself.
 *
 * @param src - Absolute image URL.
 */
export async function preloadImage(src: string): Promise<void> {
  const image = new Image();
  image.src = src;
  try {
    await image.decode();
  } catch {
    // Ignored deliberately -- see doc comment.
  }
}

/**
 * Draws a random card from the BFF.
 *
 * @param signal - Abort signal, so an in-flight draw can be cancelled if the
 *   component unmounts.
 * @returns The drawn card, with a reachable `imageUrl`.
 * @throws {ApiError} On transport failure, a non-2xx status, or a payload that
 *   fails validation.
 */
export async function drawCard(signal?: AbortSignal): Promise<TarotCard> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}/card`, {
      headers: { accept: 'application/json' },
      ...(signal === undefined ? {} : { signal }),
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === 'AbortError') {
      throw cause;
    }
    throw new ApiError('NETWORK', 'Could not reach the tarot service.');
  }

  let payload: unknown = null;
  try {
    payload = await response.json();
  } catch {
    // Fall through: an unparseable body is handled by the checks below.
  }

  if (!response.ok) {
    if (isErrorResponse(payload)) {
      throw new ApiError(payload.error.code, payload.error.message);
    }
    throw new ApiError('HTTP_ERROR', 'The tarot service returned an error.');
  }

  if (!isCardResponse(payload)) {
    throw new ApiError('MALFORMED', 'The tarot service returned an unexpected response.');
  }

  return payload.card;
}
