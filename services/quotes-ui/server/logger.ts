/**
 * Minimal structured JSON logger.
 *
 * One JSON object per line on stdout/stderr, matching the operational style of
 * the Go services so `docker compose logs` stays machine-greppable. No
 * dependency is warranted for this volume of logging.
 */

/** Severity levels, ordered least to most severe. */
export type LogLevel = 'debug' | 'info' | 'warn' | 'error';

/** Arbitrary structured context attached to a log line. */
export type LogFields = Readonly<Record<string, unknown>>;

/**
 * Serialises an `Error` into a plain object safe for JSON output.
 *
 * Stack traces are retained in logs (operator-facing) but are never included in
 * HTTP responses -- see the error handler.
 */
function serialiseError(error: unknown): LogFields {
  if (error instanceof Error) {
    return {
      errorName: error.name,
      errorMessage: error.message,
      ...(error.stack === undefined ? {} : { stack: error.stack }),
    };
  }
  return { errorMessage: String(error) };
}

/**
 * Writes one structured log line.
 *
 * @param level - Severity; `error` goes to stderr, everything else to stdout.
 * @param message - Short, stable, human-readable event description.
 * @param fields - Optional structured context.
 */
function write(level: LogLevel, message: string, fields?: LogFields): void {
  const line = JSON.stringify({
    ts: new Date().toISOString(),
    level,
    msg: message,
    ...fields,
  });
  if (level === 'error') {
    process.stderr.write(`${line}\n`);
  } else {
    process.stdout.write(`${line}\n`);
  }
}

export const logger = {
  debug: (message: string, fields?: LogFields): void => {
    write('debug', message, fields);
  },
  info: (message: string, fields?: LogFields): void => {
    write('info', message, fields);
  },
  warn: (message: string, fields?: LogFields): void => {
    write('warn', message, fields);
  },
  /**
   * Logs an error, expanding an `Error` cause into name/message/stack fields.
   */
  error: (message: string, error?: unknown, fields?: LogFields): void => {
    write('error', message, {
      ...fields,
      ...(error === undefined ? {} : serialiseError(error)),
    });
  },
} as const;
