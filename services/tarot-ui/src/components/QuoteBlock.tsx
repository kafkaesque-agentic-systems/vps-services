/**
 * The quote accompanying a revealed card.
 *
 * Semantics: a real `<blockquote>` with the author in `<cite>` inside a
 * `<footer>` -- the native structure for quoted text with attribution.
 * Announced politely via aria-live so screen-reader users hear the quote
 * arrive with the card without focus being stolen.
 */

import type { Quote } from '../types/tarot.js';

/** Props for {@link QuoteBlock}. */
export interface QuoteBlockProps {
  /** The quote to display, or `null` to render nothing (space is reserved). */
  readonly quote: Quote | null;
  /** True once the card is revealed; the quote fades in alongside it. */
  readonly visible: boolean;
}

/**
 * Cases an upstream-lowercase author name for display, e.g.
 * `stephen hawking` -> `Stephen Hawking`.
 *
 * Purely presentational: the stored value is preserved in the data. Hyphenated
 * and multi-word names are cased per word part.
 */
function formatAttribution(raw: string): string {
  return raw
    .split(/\s+/)
    .filter((word) => word.length > 0)
    .map((word) =>
      word
        .split('-')
        .map((part) => (part.length > 0 ? `${part.charAt(0).toUpperCase()}${part.slice(1)}` : part))
        .join('-'),
    )
    .join(' ');
}

/**
 * Renders the quote beneath the card.
 *
 * The wrapper always occupies a minimum height so the button below does not
 * jump when a quote appears, disappears, or changes length between draws.
 */
export function QuoteBlock({ quote, visible }: QuoteBlockProps): JSX.Element {
  return (
    <div
      aria-live="polite"
      className="flex min-h-[7rem] w-full max-w-md flex-col items-center justify-start px-4"
    >
      {quote !== null && (
        <blockquote
          key={quote.id}
          className={`flex flex-col items-center gap-3 text-center transition-opacity duration-fade ease-in-out ${
            visible ? 'animate-rise-in opacity-100' : 'opacity-0'
          }`}
        >
          {/* Decorative divider between card and quote. */}
          <span aria-hidden="true" className="flex items-center gap-3 text-gilt/50">
            <span className="h-px w-10 bg-arcane" />
            <span className="font-display text-lg leading-none">&#10022;</span>
            <span className="h-px w-10 bg-arcane" />
          </span>

          <p className="font-display text-base italic leading-relaxed text-parchment/90 sm:text-lg">
            &ldquo;{quote.text}&rdquo;
          </p>

          <footer className="text-xs tracking-[0.25em] text-mist uppercase">
            &mdash;&ensp;<cite className="not-italic text-gilt">{formatAttribution(quote.attribution)}</cite>
          </footer>
        </blockquote>
      )}
    </div>
  );
}
