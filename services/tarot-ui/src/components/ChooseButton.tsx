/**
 * The draw trigger.
 *
 * A real `<button type="button">` -- keyboard operable, focusable and
 * announced correctly for free. `aria-busy` communicates the in-flight state
 * to assistive technology; the label change communicates it visually.
 */

/** Props for {@link ChooseButton}. */
export interface ChooseButtonProps {
  /** True while a draw is running; disables interaction. */
  readonly isDrawing: boolean;
  /** True once a card has been revealed, switching the label. */
  readonly hasDrawn: boolean;
  /** Invoked on activation. */
  readonly onChoose: () => void;
}

/** Renders the primary call to action. */
export function ChooseButton({ isDrawing, hasDrawn, onChoose }: ChooseButtonProps): JSX.Element {
  const label = isDrawing ? 'Consulting…' : hasDrawn ? 'Choose again' : 'Choose';

  return (
    <button
      type="button"
      onClick={onChoose}
      disabled={isDrawing}
      aria-busy={isDrawing}
      className="
        group relative inline-flex min-w-[11rem] items-center justify-center
        rounded-full border border-gilt/40 bg-gilt/10 px-8 py-3
        font-display text-lg tracking-[0.2em] text-gilt uppercase
        transition-all duration-300
        hover:enabled:border-gilt/70 hover:enabled:bg-gilt/20 hover:enabled:text-parchment
        active:enabled:scale-[0.98]
        disabled:cursor-not-allowed disabled:opacity-50
      "
    >
      {label}
    </button>
  );
}
