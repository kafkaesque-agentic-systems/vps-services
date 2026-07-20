/**
 * State machine for drawing a card.
 *
 * Coordinates two independent asynchronous things -- a CSS fade and a network
 * round trip -- so the reveal never stutters:
 *
 *   idle -> concealing -> revealing
 *              |
 *              +-------> error
 *
 * On `draw()` the back begins fading out while the request is already in
 * flight. The card is revealed only once BOTH the fade has finished and the
 * image has been decoded, so the user never sees a half-faded back snap away,
 * nor an empty frame brightening before the artwork arrives.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { ApiError, drawCard, preloadImage } from '../lib/api.js';
import type { TarotCard } from '../types/tarot.js';

/** Fade length in ms. Must match `transitionDuration.fade` in tailwind.config.ts. */
export const FADE_MS = 700;

/** Where the draw currently is. */
export type DrawPhase = 'idle' | 'concealing' | 'revealing' | 'error';

/** Public surface of {@link useTarotCard}. */
export interface UseTarotCardResult {
  /** Current phase; drives which face is visible. */
  readonly phase: DrawPhase;
  /** The drawn card, or `null` before the first successful draw. */
  readonly card: TarotCard | null;
  /** User-safe error message, or `null`. */
  readonly error: string | null;
  /** True while a draw is in progress; disables the trigger. */
  readonly isDrawing: boolean;
  /** Starts a draw. Ignored if one is already running. */
  readonly draw: () => void;
}

/** Resolves after `ms`, used to enforce the minimum fade duration. */
function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, ms);
  });
}

/** True when the user has asked for reduced motion. */
function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  );
}

/**
 * Drives the draw interaction.
 *
 * @returns Phase, card, error and the `draw` trigger.
 */
export function useTarotCard(): UseTarotCardResult {
  const [phase, setPhase] = useState<DrawPhase>('idle');
  const [card, setCard] = useState<TarotCard | null>(null);
  const [error, setError] = useState<string | null>(null);

  const inFlight = useRef<AbortController | null>(null);
  const mounted = useRef(true);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      // Abandon any in-flight draw so a late resolution cannot set state on an
      // unmounted component.
      inFlight.current?.abort();
    };
  }, []);

  const draw = useCallback((): void => {
    if (inFlight.current !== null) {
      return;
    }

    const controller = new AbortController();
    inFlight.current = controller;

    setError(null);
    setPhase('concealing');

    const fadeMs = prefersReducedMotion() ? 0 : FADE_MS;

    void (async (): Promise<void> => {
      try {
        // Fade and fetch run concurrently; the slower one governs.
        const [drawn] = await Promise.all([
          drawCard(controller.signal).then(async (result): Promise<TarotCard> => {
            await preloadImage(result.imageUrl);
            return result;
          }),
          delay(fadeMs),
        ]);

        if (!mounted.current || controller.signal.aborted) {
          return;
        }
        setCard(drawn);
        setPhase('revealing');
      } catch (cause) {
        if (!mounted.current || controller.signal.aborted) {
          return;
        }
        setError(
          cause instanceof ApiError
            ? cause.message
            : 'Something interrupted the reading. Please try again.',
        );
        // Return to the back so the interaction remains obviously repeatable.
        setPhase('error');
      } finally {
        if (inFlight.current === controller) {
          inFlight.current = null;
        }
      }
    })();
  }, []);

  return {
    phase,
    card,
    error,
    isDrawing: phase === 'concealing',
    draw,
  };
}
