/**
 * Business logic for the quotes read paths.
 *
 * Ports the Flask tier's behaviours faithfully:
 *   - search-term splitting on unescaped commas into words vs phrases
 *     (operations.py `split_search_terms`, including the `\,` escape);
 *   - author-name normalisation (collapse whitespace, strip punctuation,
 *     lowercase, hyphenate) before the `/authors/:name` lookup;
 *   - quote text normalisation (the data is stored hard-wrapped).
 */

import type { AppConfig } from '../config.js';
import {
  isUpstreamAuthorList,
  isUpstreamAuthorQuotes,
  isUpstreamQuoteArray,
  isUpstreamQuoteList,
  isUpstreamQuote,
  type Quote,
  type UpstreamQuote,
} from '../types/quotes.js';

/** Raised when the upstream API cannot be reached or returns junk. */
export class UpstreamError extends Error {
  public override readonly name = 'UpstreamError';
  public readonly code: string;
  /** Upstream HTTP status when the failure was a status code, else null. */
  public readonly upstreamStatus: number | null;

  public constructor(
    code: string,
    message: string,
    options?: { cause?: unknown; upstreamStatus?: number },
  ) {
    super(message, options?.cause === undefined ? undefined : { cause: options.cause });
    this.code = code;
    this.upstreamStatus = options?.upstreamStatus ?? null;
  }
}

/** Collapses hard-wrapped storage text into a single display line. */
function normaliseText(raw: string): string {
  return raw.replace(/\s+/g, ' ').trim();
}

/** Maps an upstream quote into the client-facing shape. */
function toQuote(upstream: UpstreamQuote): Quote {
  return {
    id: upstream.id,
    attribution: upstream.attribution.trim(),
    text: normaliseText(upstream.quote),
  };
}

/**
 * Bounded fetch + JSON parse + guard validation, shared by every read.
 *
 * @throws {UpstreamError} On timeout, transport failure, non-2xx status,
 *   invalid JSON, or a payload failing the guard.
 */
export async function upstreamJson<T>(
  config: AppConfig,
  endpoint: string,
  guard: (value: unknown) => value is T,
  init?: RequestInit,
): Promise<T> {
  const controller = new AbortController();
  const timeout = setTimeout(() => {
    controller.abort();
  }, config.upstreamTimeoutMs);

  let response: Response;
  try {
    response = await fetch(endpoint, {
      ...init,
      signal: controller.signal,
      headers: {
        accept: 'application/json',
        ...(config.apiServiceToken === null ? {} : { authorization: config.apiServiceToken }),
        ...init?.headers,
      },
    });
  } catch (cause) {
    const timedOut = cause instanceof Error && cause.name === 'AbortError';
    throw new UpstreamError(
      timedOut ? 'UPSTREAM_TIMEOUT' : 'UPSTREAM_UNREACHABLE',
      timedOut
        ? `upstream did not respond within ${config.upstreamTimeoutMs}ms`
        : 'quotes API could not be reached',
      { cause },
    );
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    throw new UpstreamError('UPSTREAM_STATUS', `upstream returned HTTP ${response.status}`, {
      upstreamStatus: response.status,
    });
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch (cause) {
    throw new UpstreamError('UPSTREAM_MALFORMED', 'upstream returned invalid JSON', { cause });
  }

  if (!guard(payload)) {
    throw new UpstreamError('UPSTREAM_MALFORMED', 'upstream payload did not match expectations');
  }

  return payload;
}

/** Fetches one random quote (upstream wraps it in a one-element array). */
export async function fetchRandomQuote(config: AppConfig): Promise<Quote> {
  const payload = await upstreamJson(
    config,
    `${config.quotesApiBaseUrl}/quote`,
    isUpstreamQuoteArray,
  );
  // The guard proved the array non-empty; [0] is therefore safe.
  return toQuote(payload[0] as UpstreamQuote);
}

/** Fetches one quote by its ObjectId. */
export async function fetchQuoteById(config: AppConfig, id: string): Promise<Quote> {
  const payload = await upstreamJson(
    config,
    `${config.quotesApiBaseUrl}/quote/${encodeURIComponent(id)}`,
    isUpstreamQuote,
  );
  return toQuote(payload);
}

/** Fetches the author catalogue. */
export async function fetchAuthors(config: AppConfig): Promise<readonly string[]> {
  const payload = await upstreamJson(
    config,
    `${config.quotesApiBaseUrl}/authors`,
    isUpstreamAuthorList,
  );
  return payload.names;
}

/**
 * Normalises an author name the same way the Flask tier did: collapse
 * whitespace, strip punctuation, lowercase, hyphenate.
 */
export function normaliseAuthorName(name: string): string {
  return name
    .replace(/\s+/g, ' ')
    .replace(/[^\s\w]/g, '')
    .trim()
    .toLowerCase()
    .replace(/ /g, '-');
}

/** Fetches all quotes for one author. */
export async function fetchQuotesByAuthor(config: AppConfig, name: string): Promise<readonly Quote[]> {
  const slug = normaliseAuthorName(name);
  const payload = await upstreamJson(
    config,
    `${config.quotesApiBaseUrl}/authors/${encodeURIComponent(slug)}`,
    isUpstreamAuthorQuotes,
  );
  return payload.quotes.map(toQuote);
}

/**
 * Splits a raw search string into words and phrases, porting the Flask
 * behaviour exactly: split on unescaped commas; entries containing a space
 * are phrases, the rest words; `\,` escapes a literal comma inside a phrase.
 */
export function splitSearchTerms(raw: string): { words: string[]; phrases: string[] } {
  const parts = raw
    .split(/(?<!\\),\s*/)
    .map((part) => part.trim())
    .filter((part) => part.length > 0);
  return {
    words: parts.filter((p) => !p.includes(' ')),
    phrases: parts.filter((p) => p.includes(' ')).map((p) => p.replace(/\\/g, '')),
  };
}

/** Runs a keyword/phrase search. May legitimately return zero results. */
export async function searchQuotes(config: AppConfig, rawTerms: string): Promise<readonly Quote[]> {
  const { words, phrases } = splitSearchTerms(rawTerms);
  const payload = await upstreamJson(
    config,
    `${config.quotesApiBaseUrl}/quote/search`,
    isUpstreamQuoteList,
    {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ terms: words, phrases }),
    },
  );
  return payload.map(toQuote);
}
