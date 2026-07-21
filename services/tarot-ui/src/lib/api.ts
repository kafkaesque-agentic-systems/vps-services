/**
 * BFF client.
 *
 * The browser talks only to this app's own origin: the upstream tarot API's
 * address is never present in shipped JavaScript. Every response is validated
 * before use -- a 200 with an unexpected body is treated as a failure, not
 * rendered blindly.
 */

import {
  isDeckList,
  isErrorResponse,
  isReadingResponse,
  isSpreadReading,
  type ReadingResponse,
  type SpreadReading,
} from '../types/tarot.js';

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
 * Preloads an image so it can be revealed without a blank frame.
 *
 * Without this the fade-in begins before the bytes have arrived and the user
 * watches an empty rectangle brighten.
 *
 * This function is guaranteed to settle. A bare `await image.decode()` is NOT:
 * browsers may defer decode work for detached images in hidden or backgrounded
 * tabs, leaving the promise pending indefinitely and wedging the draw in the
 * "Consulting…" state (observed in a background tab, 2026-07-20). Three exits
 * cover every case:
 *
 * 1. `load` -> best-effort `decode()` -> resolve (the intended fast path);
 * 2. `error` -> resolve, letting the in-document `<img>` retry and surface
 *    its own failure;
 * 3. a timeout -> resolve, accepting a progressive paint over a stuck UI.
 *
 * Preloading is an optimisation; it must never be able to block the feature.
 *
 * @param src - Absolute image URL.
 * @param timeoutMs - Ceiling before the reveal proceeds without the preload.
 */
export function preloadImage(src: string, timeoutMs = 4000): Promise<void> {
  return new Promise<void>((resolve) => {
    const image = new Image();
    let settled = false;
    const done = (): void => {
      if (!settled) {
        settled = true;
        clearTimeout(timer);
        resolve();
      }
    };
    const timer = setTimeout(done, timeoutMs);
    image.addEventListener(
      'load',
      () => {
        // Decode is best-effort once the bytes are in; never let it wedge.
        image.decode().catch(() => undefined).finally(done);
      },
      { once: true },
    );
    image.addEventListener('error', done, { once: true });
    image.src = src;
  });
}

/**
 * Shared BFF request pipeline: fetch, parse, map error bodies, validate.
 *
 * Every endpoint follows the same discipline -- a 200 with an unexpected body
 * is a failure, not something to render blindly -- so the boilerplate lives
 * once here.
 *
 * @param path - Path under the API base, beginning with `/`.
 * @param guard - Type guard the payload must satisfy.
 * @param init - Extra fetch options (method, headers, body).
 * @param signal - Abort signal for unmount cancellation.
 * @throws {ApiError} On transport failure, a non-2xx status, or a payload that
 *   fails validation. Abort errors propagate untouched.
 */
async function requestJson<T>(
  path: string,
  guard: (value: unknown) => value is T,
  init?: RequestInit,
  signal?: AbortSignal,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers: { accept: 'application/json', ...init?.headers },
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

  if (!guard(payload)) {
    throw new ApiError('MALFORMED', 'The tarot service returned an unexpected response.');
  }

  return payload;
}

/**
 * Draws a reading -- a random card plus an accompanying quote -- from the BFF.
 *
 * The BFF fetches both upstreams concurrently, so this costs a single round
 * trip. `quote` may be `null` when the quotes upstream failed; the card is
 * always present on success.
 *
 * @param signal - Abort signal, so an in-flight draw can be cancelled if the
 *   component unmounts.
 */
export function drawReading(signal?: AbortSignal): Promise<ReadingResponse> {
  return requestJson('/reading', isReadingResponse, undefined, signal);
}

/** The body sent to `POST /tarot/api/spread`. */
export interface SpreadDraft {
  readonly name: string;
  readonly deck: string;
  readonly positions: readonly string[];
}

/**
 * Draws a custom spread from the BFF.
 *
 * Cards come back in the order the positions were requested; the BFF restores
 * the ordering the upstream's map response destroys.
 *
 * @param draft - Spread name, deck selection, and ordered positions.
 * @param signal - Abort signal, so an in-flight draw can be cancelled.
 */
export function drawSpread(draft: SpreadDraft, signal?: AbortSignal): Promise<SpreadReading> {
  return requestJson(
    '/spread',
    isSpreadReading,
    {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(draft),
    },
    signal,
  );
}

/**
 * Fetches the deck catalogue for the spread composer.
 *
 * @param signal - Abort signal, so the fetch can be cancelled on unmount.
 * @returns Sorted deck names.
 */
export async function fetchDecks(signal?: AbortSignal): Promise<readonly string[]> {
  const list = await requestJson('/decks', isDeckList, undefined, signal);
  return list.names;
}
