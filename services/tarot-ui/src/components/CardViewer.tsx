/**
 * Floating viewer for one revealed card.
 *
 * A minimal modal: veiled backdrop, the card large in the centre, its
 * position beneath, one close control. Only one card is ever viewable at a
 * time -- the parent holds a single `viewing` slot.
 *
 * Accessibility contract:
 *   - `role="dialog"` + `aria-modal`, labelled by the position;
 *   - focus moves to the close button on open and RETURNS to the element
 *     that opened the viewer on close;
 *   - Escape, the backdrop, and the close button all close it;
 *   - Tab is held inside the dialog (the close button is its only control);
 *   - body scroll is locked while open and restored on close.
 *
 * Sizing is viewport-driven -- `svh` rather than `vh` so mobile browser
 * chrome never clips the card -- and `object-contain` preserves every deck's
 * native aspect ratio, per the established card-rendering rule.
 */

import { useEffect, useRef } from 'react';

import type { SpreadCard } from '../types/tarot.js';

/** Props for {@link CardViewer}. */
export interface CardViewerProps {
  /** The card being viewed. */
  readonly card: SpreadCard;
  /** Requests the viewer be closed. Must be referentially stable. */
  readonly onClose: () => void;
}

/** Renders the floating card viewer. */
export function CardViewer({ card, onClose }: CardViewerProps): JSX.Element {
  const closeRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    // Remember the opener so focus can be handed back on close (WCAG 2.4.3).
    const opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeRef.current?.focus();

    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';

    const onKeyDown = (event: KeyboardEvent): void => {
      if (event.key === 'Escape') {
        onClose();
      } else if (event.key === 'Tab') {
        // The close button is the dialog's only control; keep focus on it
        // rather than letting Tab escape into the veiled page behind.
        event.preventDefault();
        closeRef.current?.focus();
      }
    };
    document.addEventListener('keydown', onKeyDown);

    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.body.style.overflow = previousOverflow;
      opener?.focus();
    };
  }, [onClose]);

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={`${card.position} card, enlarged`}
      className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-8"
    >
      {/* Veil. A real button so "click outside to close" is also a control. */}
      <button
        type="button"
        aria-label="Close the card viewer"
        onClick={onClose}
        className="absolute inset-0 animate-fade-veil cursor-default bg-void/85 backdrop-blur-sm"
      />

      <figure className="relative m-0 flex animate-bloom-in flex-col items-center gap-4">
        <img
          src={card.imageUrl}
          alt={`${card.position} card, enlarged`}
          className="max-h-[78svh] w-auto max-w-[min(92vw,30rem)] rounded-2xl border border-arcane object-contain shadow-2xl shadow-black/70"
        />

        <figcaption className="text-xs tracking-[0.25em] text-gilt uppercase">
          {card.position}
        </figcaption>

        <button
          ref={closeRef}
          type="button"
          aria-label="Close"
          onClick={onClose}
          className="absolute -top-3 -right-3 flex h-10 w-10 items-center justify-center rounded-full border border-arcane bg-obsidian text-lg text-mist shadow-lg shadow-black/50 transition-colors hover:border-gilt/50 hover:text-gilt"
        >
          &times;
        </button>
      </figure>
    </div>
  );
}
