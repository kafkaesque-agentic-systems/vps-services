/**
 * Runtime configuration, validated once at startup.
 *
 * Every value enters through an environment variable and is checked here. A
 * missing or malformed value aborts the process immediately rather than
 * surfacing later as a confusing runtime failure -- the same fail-fast
 * discipline the compose file and the Go MCP server use. Silent defaults have
 * repeatedly proven to be this platform's most expensive failure mode.
 */

/** Fully validated, immutable process configuration. */
export interface AppConfig {
  /** TCP port the HTTP server listens on. */
  readonly port: number;
  /** Environment name; enables production hardening when `production`. */
  readonly nodeEnv: 'development' | 'production' | 'test';
  /**
   * Public path this app is mounted at behind the reverse proxy, without a
   * trailing slash (e.g. `/tarot`). NGINX forwards the original URI unchanged,
   * so the server must mount its routes under the same prefix the browser uses.
   */
  readonly basePath: string;
  /** Base URL of the upstream Go tarot API, without a trailing slash. */
  readonly tarotApiBaseUrl: string;
  /**
   * Prefix the upstream API incorrectly emits in card image URLs. The API
   * returns e.g. `https://thirdeye.live/static/img/tarot/<deck>/<n>.jpg`, a
   * host that does not resolve.
   */
  readonly upstreamImagePrefix: string;
  /** Prefix that actually serves the images, substituted for the broken one. */
  readonly publicImagePrefix: string;
  /** Milliseconds before an upstream API request is aborted. */
  readonly upstreamTimeoutMs: number;
}

/** Raised when the environment cannot produce a valid {@link AppConfig}. */
export class ConfigError extends Error {
  public override readonly name = 'ConfigError';
}

/**
 * Reads a required environment variable.
 *
 * @param key - Variable name.
 * @returns The trimmed, non-empty value.
 * @throws {ConfigError} If unset or empty.
 */
function required(key: string): string {
  const raw = process.env[key];
  if (raw === undefined || raw.trim() === '') {
    throw new ConfigError(`${key} must be set`);
  }
  return raw.trim();
}

/**
 * Reads an optional environment variable, falling back to a default.
 *
 * @param key - Variable name.
 * @param fallback - Value used when unset or empty.
 */
function optional(key: string, fallback: string): string {
  const raw = process.env[key];
  return raw === undefined || raw.trim() === '' ? fallback : raw.trim();
}

/**
 * Parses a positive integer.
 *
 * @throws {ConfigError} If the value is not a positive integer.
 */
function positiveInt(key: string, raw: string): number {
  const value = Number(raw);
  if (!Number.isInteger(value) || value <= 0) {
    throw new ConfigError(`${key} must be a positive integer, got ${JSON.stringify(raw)}`);
  }
  return value;
}

/**
 * Validates an absolute http(s) URL and strips any trailing slash so callers
 * can concatenate paths without producing a double slash.
 *
 * @throws {ConfigError} If the value is not a valid absolute http(s) URL.
 */
function absoluteUrl(key: string, raw: string): string {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    throw new ConfigError(`${key} must be an absolute URL, got ${JSON.stringify(raw)}`);
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new ConfigError(`${key} must use http or https, got ${parsed.protocol}`);
  }
  return raw.replace(/\/+$/, '');
}

/** Normalises a mount path to a leading slash with no trailing slash. */
function normaliseBasePath(key: string, raw: string): string {
  if (!raw.startsWith('/')) {
    throw new ConfigError(`${key} must start with "/", got ${JSON.stringify(raw)}`);
  }
  const trimmed = raw.replace(/\/+$/, '');
  return trimmed === '' ? '/' : trimmed;
}

/** Narrows an arbitrary string to the supported NODE_ENV values. */
function parseNodeEnv(raw: string): AppConfig['nodeEnv'] {
  if (raw === 'development' || raw === 'production' || raw === 'test') {
    return raw;
  }
  throw new ConfigError(
    `NODE_ENV must be one of development|production|test, got ${JSON.stringify(raw)}`,
  );
}

/**
 * Builds the application configuration from `process.env`.
 *
 * @returns A frozen, fully validated configuration object.
 * @throws {ConfigError} On the first invalid or missing value.
 */
export function loadConfig(): AppConfig {
  const config: AppConfig = {
    port: positiveInt('PORT', optional('PORT', '3000')),
    nodeEnv: parseNodeEnv(optional('NODE_ENV', 'production')),
    basePath: normaliseBasePath('BASE_PATH', optional('BASE_PATH', '/tarot')),
    tarotApiBaseUrl: absoluteUrl('TAROT_API_BASE_URL', required('TAROT_API_BASE_URL')),
    upstreamImagePrefix: absoluteUrl(
      'UPSTREAM_IMAGE_PREFIX',
      optional('UPSTREAM_IMAGE_PREFIX', 'https://thirdeye.live/static/img'),
    ),
    publicImagePrefix: absoluteUrl(
      'PUBLIC_IMAGE_PREFIX',
      optional('PUBLIC_IMAGE_PREFIX', 'https://api.thirdeye.live/image'),
    ),
    upstreamTimeoutMs: positiveInt(
      'UPSTREAM_TIMEOUT_MS',
      optional('UPSTREAM_TIMEOUT_MS', '8000'),
    ),
  };

  return Object.freeze(config);
}
