/**
 * One position in a laid spread: a face-down card the user turns by clicking.
 *
 * Same containerisation discipline as the single-card stage, scaled down: a
 * FIXED-footprint 2:3 obsidian well, both faces always mounted and stacked,
 * `object-contain` so no deck's dimensions are ever cropped, and an
 * opacity-only crossfade. Nothing resizes on turn, so the grid never shifts.
 */

import { CARD_BACK_SRC } from '../lib/images.js';
import type { SpreadCard } from '../types/tarot.js';

/** Props for {@link FlipCard}. */
export interface FlipCardProps {
  /** The resolved position this card occupies. */
  readonly card: SpreadCard;
  /** Whether the card has been turned face up. */
  readonly turned: boolean;
  /** Requests the turn; ignored by the parent once turned. */
  readonly onTurn: (position: string) => void;
}

/** Renders a single click-to-turn card with its position label. */
export function FlipCard({ card, turned, onTurn }: FlipCardProps): JSX.Element {
  return (
    <div className="flex flex-col items-center">
      <button
        type="button"
        onClick={() => {
          if (!turned) {
            onTurn(card.position);
          }
        }}
        aria-label={
          turned ? `${card.position}: card revealed` : `Turn the card for ${card.position}`
        }
        className={`relative aspect-[2/3] w-32 overflow-hidden rounded-xl border bg-obsidian shadow-xl shadow-black/50 transition-colors duration-300 sm:w-36 md:w-40 ${
          turned ? 'cursor-default border-arcane' : 'cursor-pointer border-arcane hover:border-gilt/50'
        }`}
      >
        <img
          src={CARD_BACK_SRC}
          alt=""
          width={160}
          height={240}
          className={`absolute inset-1.5 h-[calc(100%-0.75rem)] w-[calc(100%-0.75rem)] object-contain drop-shadow-[0_2px_10px_rgba(0,0,0,0.6)] transition-opacity duration-fade ease-in-out ${
            turned ? 'opacity-0' : 'opacity-100'
          }`}
        />
        <img
          src={card.imageUrl}
          alt=""
          width={160}
          height={240}
          className={`absolute inset-1.5 h-[calc(100%-0.75rem)] w-[calc(100%-0.75rem)] object-contain drop-shadow-[0_2px_10px_rgba(0,0,0,0.6)] transition-opacity duration-fade ease-in-out ${
            turned ? 'opacity-100' : 'opacity-0'
          }`}
        />
      </button>

      {/* Reserved height so the label's colour change cannot move the grid. */}
      <p
        className={`mt-2 min-h-[1.25rem] max-w-[10rem] text-center text-[0.65rem] tracking-[0.2em] uppercase transition-colors duration-300 ${
          turned ? 'text-gilt' : 'text-mist'
        }`}
      >
        {card.position}
      </p>
    </div>
  );
}
