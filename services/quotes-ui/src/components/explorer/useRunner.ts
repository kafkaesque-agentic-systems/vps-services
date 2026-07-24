/**
 * Per-panel execution state: one in-flight run, its result or error.
 *
 * Every explorer panel shares this lifecycle, so it lives once here. Results
 * are `unknown` by design — panels display whatever came back (typed data
 * from the BFF reads, verbatim upstream bodies from the write
 * pass-throughs) through the JsonView.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { ApiError } from '../../lib/api.js';

/** Public surface of {@link useRunner}. */
export interface RunnerState {
  /** True while a run is in flight. */
  readonly busy: boolean;
  /** The last successful result, or `undefined` before one exists. */
  readonly result: unknown;
  /** User-safe error message, or `null`. */
  readonly error: string | null;
  /** Executes the given operation, replacing previous result/error. */
  readonly run: (operation: () => Promise<unknown>) => void;
}

/** Manages one panel's run lifecycle. */
export function useRunner(): RunnerState {
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<unknown>(undefined);
  const [error, setError] = useState<string | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const run = useCallback((operation: () => Promise<unknown>): void => {
    setBusy(true);
    setError(null);
    operation()
      .then((value) => {
        if (mounted.current) {
          setResult(value);
        }
      })
      .catch((cause: unknown) => {
        if (mounted.current) {
          setResult(undefined);
          setError(
            cause instanceof ApiError ? cause.message : 'Something went wrong. Please try again.',
          );
        }
      })
      .finally(() => {
        if (mounted.current) {
          setBusy(false);
        }
      });
  }, []);

  return { busy, result, error, run };
}
