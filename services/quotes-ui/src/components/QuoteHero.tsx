/**
 * The hero: one large serif quote with attribution and a redraw control.
 * Height is reserved via min-heights so redraws never shift the page.
 */

import { useCallback, useEffect, useRef, useState } from 'react';

import { ApiError, fetchRandomQuote } from '../lib/api.js';
import type { Quote } from '../types/api.js';

/** Served when the API is unreachable, mirroring the Flask fallback. */
const FALLBACK: Quote = {
  id: 'fallback',
  attribution: 'seneca',
  text: "Every new beginning comes from some other beginning's end.",
};

/** Title-cases a stored-lowercase attribution for display. */
function formatAttribution(raw: string): string {
  return raw
    .split(/\s+/)
    .filter((w) => w.length > 0)
    .map((w) => `${w.charAt(0).toUpperCase()}${w.slice(1)}`)
    .join(' ');
}

/** Renders the hero quote section. */
export function QuoteHero(): JSX.Element {
  const [quote, setQuote] = useState<Quote | null>(null);
  const [busy, setBusy] = useState(false);
  const mounted = useRef(true);

  const draw = useCallback((): void => {
    setBusy(true);
    fetchRandomQuote()
      .then((next) => {
        if (mounted.current) {
          setQuote(next);
        }
      })
      .catch((error: unknown) => {
        if (mounted.current) {
          // Degrade to the fallback rather than an empty hero.
          setQuote((previous) => previous ?? FALLBACK);
          if (!(error instanceof ApiError)) {
            throw error;
          }
        }
      })
      .finally(() => {
        if (mounted.current) {
          setBusy(false);
        }
      });
  }, []);

  useEffect(() => {
    mounted.current = true;
    draw();
    return () => {
      mounted.current = false;
    };
  }, [draw]);

  return (
    <section aria-label="Random quote" className="mx-auto w-full max-w-3xl px-6 text-center">
      <div className="flex min-h-[12rem] flex-col items-center justify-center gap-5">
        {quote !== null && (
          <blockquote key={quote.id} className="animate-rise-in">
            <p className="font-display text-2xl leading-relaxed text-parchment italic sm:text-3xl">
              &ldquo;{quote.text}&rdquo;
            </p>
            <footer className="mt-4 text-xs tracking-[0.3em] text-lilac uppercase">
              &mdash;&ensp;<cite className="not-italic">{formatAttribution(quote.attribution)}</cite>
            </footer>
          </blockquote>
        )}
      </div>

      <button
        type="button"
        onClick={draw}
        disabled={busy}
        aria-busy={busy}
        className="mt-6 rounded-full border border-amethyst/40 bg-amethyst/10 px-6 py-2 text-xs tracking-[0.25em] text-lilac uppercase transition-all duration-300 hover:enabled:border-amethyst/70 hover:enabled:bg-amethyst/20 hover:enabled:text-parchment disabled:cursor-not-allowed disabled:opacity-50"
      >
        {busy ? 'Consulting…' : 'Another'}
      </button>
    </section>
  );
}
