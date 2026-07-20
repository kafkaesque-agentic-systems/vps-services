/**
 * The card itself: a fixed-ratio well holding two cross-fading faces.
 *
 * Both faces are always mounted and stacked; only opacity changes. Swapping
 * `src` on a single element would flash, and unmounting would forfeit the
 * transition entirely.
 */

import type { DrawPhase } from '../hooks/useTarotCard.js';
import type { TarotCard } from '../types/tarot.js';

/**
 * Base URL of the card image store.
 *
 * Root-relative by default: behind the NGINX gateway the image store is served
 * on this app's own origin, which keeps the bundle free of any hard-coded
 * domain. Override with `VITE_IMAGE_BASE` to run outside the gateway.
 */
const IMAGE_BASE = import.meta.env.VITE_IMAGE_BASE ?? '/image';

/** Card back shown while the card is face down. */
const CARD_BACK_SRC = `${IMAGE_BASE}/tarot/back-images/back-01.jpg`;

/** Props for {@link CardStage}. */
export interface CardStageProps {
  /** Current draw phase; decides which face is visible. */
  readonly phase: DrawPhase;
  /** The drawn card, or `null` before the first successful draw. */
  readonly card: TarotCard | null;
}

/** Formats a deck slug such as `dark_fairytale` for display. */
function formatDeckName(deck: string): string {
  return deck
    .split('_')
    .filter((part) => part.length > 0)
    .map((part) => `${part.charAt(0).toUpperCase()}${part.slice(1)}`)
    .join(' ');
}

/**
 * Renders the card well.
 *
 * The back shows while idle or after an error; the face shows once revealed.
 * During `concealing` neither is visible, which produces the deliberate beat
 * between concealing and revealing.
 */
export function CardStage({ phase, card }: CardStageProps): JSX.Element {
  const backVisible = phase === 'idle' || phase === 'error';
  const faceVisible = phase === 'revealing' && card !== null;

  return (
    <figure className="m-0 flex flex-col items-center gap-5">
      <div className="relative aspect-[2/3] w-64 sm:w-72 md:w-80">
        {/* Ambient glow. Purely decorative, so hidden from assistive tech. */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -inset-6 animate-drift-glow rounded-[2rem] bg-amethyst/20 blur-2xl"
        />

        <div className="relative h-full w-full overflow-hidden rounded-2xl border border-arcane bg-obsidian shadow-2xl shadow-black/60">
          <img
            src={CARD_BACK_SRC}
            alt="Face-down tarot card"
            width={320}
            height={480}
            className={`absolute inset-0 h-full w-full object-cover transition-opacity duration-fade ease-in-out ${
              backVisible ? 'opacity-100' : 'opacity-0'
            }`}
          />

          {card !== null && (
            <img
              key={card.id + card.imageUrl}
              src={card.imageUrl}
              alt={`Tarot card drawn from the ${formatDeckName(card.deck)} deck`}
              width={320}
              height={480}
              className={`absolute inset-0 h-full w-full object-cover transition-opacity duration-fade ease-in-out ${
                faceVisible ? 'opacity-100' : 'opacity-0'
              }`}
            />
          )}
        </div>
      </div>

      {/*
        Announced politely so screen-reader users learn the outcome without the
        draw stealing focus. Space is reserved via min-height so revealing the
        caption does not shift the layout.
      */}
      <figcaption
        aria-live="polite"
        className="flex min-h-[1.5rem] items-center text-sm tracking-wide text-mist"
      >
        {faceVisible && card !== null ? (
          <span className="animate-rise-in">
            from the <span className="text-gilt">{formatDeckName(card.deck)}</span> deck
          </span>
        ) : null}
      </figcaption>
    </figure>
  );
}
