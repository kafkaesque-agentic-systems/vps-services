/**
 * BFF client. The browser talks only to this app's own origin; every typed
 * response is validated before use. The write pass-throughs are the one
 * deliberate exception: they exist to demonstrate the API's own behaviour,
 * so they return the upstream's status and body verbatim.
 */

import {
  isErrorResponse,
  isNamesEnvelope,
  isQuoteEnvelope,
  isQuotesEnvelope,
  isTokenRequestResponse,
  type Quote,
  type TokenRequestResponse,
} from '../types/api.js';

/** Base path of the BFF API on this origin. */
const API_BASE = `${import.meta.env.BASE_URL.replace(/\/$/, '')}/api`;

/** A failure carrying a message safe to display. */
export class ApiError extends Error {
  public override readonly name = 'ApiError';
  public readonly code: string;

  public constructor(code: string, message: string) {
    super(message);
    this.code = code;
  }
}

/** Shared typed-request pipeline: fetch, parse, map errors, validate. */
async function requestJson<T>(
  path: string,
  guard: (value: unknown) => value is T,
  init?: RequestInit,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers: { accept: 'application/json', ...init?.headers },
    });
  } catch {
    throw new ApiError('NETWORK', 'Could not reach the quotes service.');
  }

  let payload: unknown = null;
  try {
    payload = await response.json();
  } catch {
    // Handled by the checks below.
  }

  if (!response.ok) {
    if (isErrorResponse(payload)) {
      throw new ApiError(payload.error.code, payload.error.message);
    }
    if (isTokenRequestResponse(payload)) {
      // token-request 503s carry a well-formed outcome body.
      return payload as unknown as T;
    }
    throw new ApiError('HTTP_ERROR', 'The quotes service returned an error.');
  }

  if (!guard(payload)) {
    throw new ApiError('MALFORMED', 'The quotes service returned an unexpected response.');
  }

  return payload;
}

/** Fetches one random quote. */
export async function fetchRandomQuote(): Promise<Quote> {
  const envelope = await requestJson('/quote', isQuoteEnvelope);
  return envelope.quote;
}

/** Fetches one quote by ObjectId. */
export async function fetchQuoteById(id: string): Promise<Quote> {
  const envelope = await requestJson(`/quote/${encodeURIComponent(id)}`, isQuoteEnvelope);
  return envelope.quote;
}

/** Fetches the author catalogue. */
export async function fetchAuthors(): Promise<readonly string[]> {
  const envelope = await requestJson('/authors', isNamesEnvelope);
  return envelope.names;
}

/** Fetches all quotes for one author. */
export async function fetchQuotesByAuthor(name: string): Promise<readonly Quote[]> {
  const envelope = await requestJson(`/authors/${encodeURIComponent(name)}`, isQuotesEnvelope);
  return envelope.quotes;
}

/** Runs a comma-syntax keyword/phrase search. */
export async function searchQuotes(query: string): Promise<readonly Quote[]> {
  const envelope = await requestJson('/search', isQuotesEnvelope, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ query }),
  });
  return envelope.quotes;
}

/** Submits a token request. Returns the outcome body even on 503. */
export function requestToken(email: string, honeypot: string): Promise<TokenRequestResponse> {
  return requestJson('/token-request', isTokenRequestResponse, {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ email, website: honeypot }),
  });
}

/** Raw result of a pass-through: the upstream's own status and body. */
export interface PassthroughResult {
  readonly status: number;
  readonly body: unknown;
}

/**
 * Executes a PUBLIC pass-through (the tarot surface), relaying the
 * upstream response verbatim. No auth involved.
 */
export async function publicPassthrough(
  method: 'GET' | 'POST',
  path: string,
  body?: Record<string, unknown>,
): Promise<PassthroughResult> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        accept: 'application/json',
        ...(body === undefined ? {} : { 'content-type': 'application/json' }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
  } catch {
    throw new ApiError('NETWORK', 'Could not reach the service.');
  }

  let parsed: unknown = null;
  try {
    parsed = await response.json();
  } catch {
    parsed = null;
  }

  return { status: response.status, body: parsed };
}

/**
 * Executes a token-gated write, relaying the upstream response verbatim.
 *
 * @param method - POST | PUT | DELETE.
 * @param path - Path under the API base, e.g. `/quote/<id>`.
 * @param token - The user's own API token, forwarded and never stored.
 * @param body - JSON body for POST/PUT.
 */
export async function writePassthrough(
  method: 'POST' | 'PUT' | 'DELETE',
  path: string,
  token: string,
  body?: Record<string, unknown>,
): Promise<PassthroughResult> {
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method,
      headers: {
        accept: 'application/json',
        authorization: token,
        ...(body === undefined ? {} : { 'content-type': 'application/json' }),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    });
  } catch {
    throw new ApiError('NETWORK', 'Could not reach the quotes service.');
  }

  let parsed: unknown = null;
  try {
    parsed = await response.json();
  } catch {
    // A non-JSON body is still a valid demonstration; show it as text.
    parsed = null;
  }

  return { status: response.status, body: parsed };
}
