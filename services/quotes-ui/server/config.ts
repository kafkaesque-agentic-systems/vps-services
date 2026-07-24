/**
 * Runtime configuration, validated once at startup.
 *
 * Same fail-fast discipline as the rest of the platform: required values
 * abort the process when missing. The token-request secrets are the one
 * deliberate exception — they are OPTIONAL, and their absence disables that
 * single feature loudly (503 from its endpoint, warning at boot) rather than
 * preventing the whole demo site from starting. This keeps local review and
 * development honest without production credentials.
 */

/** Fully validated, immutable process configuration. */
export interface AppConfig {
  readonly port: number;
  readonly nodeEnv: 'development' | 'production' | 'test';
  /** Public mount path, no trailing slash; `/` for this app in production. */
  readonly basePath: string;
  /** Base URL of the upstream Go quotes API, no trailing slash. */
  readonly quotesApiBaseUrl: string;
  /** Milliseconds before an upstream request is aborted. */
  readonly upstreamTimeoutMs: number;
  /**
   * Service token attached to upstream API calls (the API is token-gated).
   * `null` disables attachment, which only works against a public API — kept
   * optional so local development degrades rather than refusing to start.
   */
  readonly apiServiceToken: string | null;
  /**
   * Admin bearer secret for the token-request check
   * (`GET /admin/tokens/:email`). `null` disables the token-request feature.
   */
  readonly adminToken: string | null;
  /** SMTP settings for the token-request notification. `null` = no mail. */
  readonly mail: {
    readonly host: string;
    readonly username: string;
    readonly password: string;
    readonly to: string;
    readonly from: string;
  } | null;
}

/** Raised when the environment cannot produce a valid {@link AppConfig}. */
export class ConfigError extends Error {
  public override readonly name = 'ConfigError';
}

/** Reads an optional variable, empty treated as unset. */
function optional(key: string): string | null {
  const raw = process.env[key];
  return raw === undefined || raw.trim() === '' ? null : raw.trim();
}

/** Reads an optional variable with a default. */
function optionalWithDefault(key: string, fallback: string): string {
  return optional(key) ?? fallback;
}

/** Reads a required variable. @throws {ConfigError} when unset. */
function required(key: string): string {
  const value = optional(key);
  if (value === null) {
    throw new ConfigError(`${key} must be set`);
  }
  return value;
}

/** Parses a positive integer. @throws {ConfigError} on anything else. */
function positiveInt(key: string, raw: string): number {
  const value = Number(raw);
  if (!Number.isInteger(value) || value <= 0) {
    throw new ConfigError(`${key} must be a positive integer, got ${JSON.stringify(raw)}`);
  }
  return value;
}

/** Validates an absolute http(s) URL, stripping any trailing slash. */
function absoluteUrl(key: string, raw: string): string {
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    throw new ConfigError(`${key} must be an absolute URL, got ${JSON.stringify(raw)}`);
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new ConfigError(`${key} must use http or https`);
  }
  return raw.replace(/\/+$/, '');
}

/** Normalises the mount path: leading slash, no trailing slash. */
function normaliseBasePath(key: string, raw: string): string {
  if (!raw.startsWith('/')) {
    throw new ConfigError(`${key} must start with "/"`);
  }
  const trimmed = raw.replace(/\/+$/, '');
  return trimmed === '' ? '/' : trimmed;
}

/** Narrows NODE_ENV to its supported values. */
function parseNodeEnv(raw: string): AppConfig['nodeEnv'] {
  if (raw === 'development' || raw === 'production' || raw === 'test') {
    return raw;
  }
  throw new ConfigError(`NODE_ENV must be development|production|test, got ${JSON.stringify(raw)}`);
}

/**
 * Builds the mail configuration from MAILSERVER / MAILPASS.
 *
 * Mirrors the Flask tier's variables so `.environs` needs no new names. All
 * or nothing: a partially configured mailer is a misconfiguration and fails
 * loudly rather than half-working.
 */
function loadMailConfig(): AppConfig['mail'] {
  const username = optional('MAILSERVER');
  const password = optional('MAILPASS');
  if (username === null && password === null) {
    return null;
  }
  if (username === null || password === null) {
    throw new ConfigError('MAILSERVER and MAILPASS must be set together (or neither)');
  }
  return {
    host: optionalWithDefault('MAILHOST', 'mail.thirdeye.live'),
    username,
    password,
    to: optionalWithDefault('TOKEN_NOTIFY_TO', 'admin@thepromethean.net'),
    from: optionalWithDefault(
      'TOKEN_NOTIFY_FROM',
      'Quotes API Notifications <theoracle@thirdeye.live>',
    ),
  };
}

/**
 * Builds the application configuration from `process.env`.
 *
 * @returns A frozen, fully validated configuration object.
 * @throws {ConfigError} On the first invalid value.
 */
export function loadConfig(): AppConfig {
  const config: AppConfig = {
    port: positiveInt('PORT', optionalWithDefault('PORT', '3000')),
    nodeEnv: parseNodeEnv(optionalWithDefault('NODE_ENV', 'production')),
    basePath: normaliseBasePath('BASE_PATH', optionalWithDefault('BASE_PATH', '/')),
    quotesApiBaseUrl: absoluteUrl('QUOTES_API_BASE_URL', required('QUOTES_API_BASE_URL')),
    upstreamTimeoutMs: positiveInt(
      'UPSTREAM_TIMEOUT_MS',
      optionalWithDefault('UPSTREAM_TIMEOUT_MS', '10000'),
    ),
    // AUTHORIZED matches the Flask tier's variable name for the admin secret.
    adminToken: optional('AUTHORIZED'),
    apiServiceToken: optional('API_SERVICE_TOKEN'),
    mail: loadMailConfig(),
  };

  return Object.freeze(config);
}
