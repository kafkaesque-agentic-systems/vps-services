/**
 * State machine for composing and revealing a custom spread.
 *
 *   compose -> (draw) -> reading
 *      ^                    |
 *      +----- (reset) ------+
 *
 * Cards land face DOWN; each is turned individually by the user, tracked in
 * `turned`. Faces are preloaded fire-and-forget after the draw so the first
 * click flips instantly -- but the grid never waits on them, and preloadImage
 * is guaranteed to settle, so a slow image can delay nothing.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { ApiError, drawSpread, preloadImage, type SpreadDraft } from '../lib/api.js';
import type { SpreadReading } from '../types/tarot.js';

/** Where the page currently is. */
export type SpreadPhase = 'compose' | 'reading';

/** Public surface of {@link useSpread}. */
export interface UseSpreadResult {
  /** Current phase; decides composer vs laid spread. */
  readonly phase: SpreadPhase;
  /** The drawn reading, or `null` before the first successful draw. */
  readonly reading: SpreadReading | null;
  /** Positions the user has turned face up. */
  readonly turned: ReadonlySet<string>;
  /** User-safe error message, or `null`. */
  readonly error: string | null;
  /** True while a draw request is in flight. */
  readonly isDrawing: boolean;
  /** Submits a composed spread. Ignored if a draw is already running. */
  readonly draw: (draft: SpreadDraft) => void;
  /** Turns one position face up. Turning is one-way. */
  readonly turn: (position: string) => void;
  /** Returns to the composer, keeping its inputs for adjustment. */
  readonly reset: () => void;
}

/** Drives the custom-spread interaction. */
export function useSpread(): UseSpreadResult {
  const [phase, setPhase] = useState<SpreadPhase>('compose');
  const [reading, setReading] = useState<SpreadReading | null>(null);
  const [turned, setTurned] = useState<ReadonlySet<string>>(new Set());
  const [error, setError] = useState<string | null>(null);
  const [isDrawing, setIsDrawing] = useState(false);

  const inFlight = useRef<AbortController | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      inFlight.current?.abort();
    };
  }, []);

  const draw = useCallback((draft: SpreadDraft): void => {
    if (inFlight.current !== null) {
      return;
    }
    const controller = new AbortController();
    inFlight.current = controller;

    setError(null);
    setIsDrawing(true);

    void (async (): Promise<void> => {
      try {
        const result = await drawSpread(draft, controller.signal);
        if (!mounted.current || controller.signal.aborted) {
          return;
        }
        // Warm the faces so the first turn flips instantly. Fire-and-forget:
        // the laid grid shows card backs and must not wait on any of this.
        for (const card of result.cards) {
          void preloadImage(card.imageUrl);
        }
        setReading(result);
        setTurned(new Set());
        setPhase('reading');
      } catch (cause) {
        if (!mounted.current || controller.signal.aborted) {
          return;
        }
        setError(
          cause instanceof ApiError
            ? cause.message
            : 'Something interrupted the reading. Please try again.',
        );
      } finally {
        if (inFlight.current === controller) {
          inFlight.current = null;
        }
        if (mounted.current) {
          setIsDrawing(false);
        }
      }
    })();
  }, []);

  const turn = useCallback((position: string): void => {
    setTurned((prev) => {
      if (prev.has(position)) {
        return prev;
      }
      const next = new Set(prev);
      next.add(position);
      return next;
    });
  }, []);

  const reset = useCallback((): void => {
    setPhase('compose');
    setError(null);
  }, []);

  return { phase, reading, turned, error, isDrawing, draw, turn, reset };
}
