/**
 * The primary call-to-action button, extracted so every page's actions share
 * one appearance. A real `<button>`: keyboard operable, focusable, announced
 * correctly for free.
 */

/** Props for {@link MysticButton}. */
export interface MysticButtonProps {
  /** Visible label. */
  readonly label: string;
  /** Button type; `submit` participates in form submission. */
  readonly type?: 'button' | 'submit';
  /** Disables interaction (also set while busy). */
  readonly disabled?: boolean;
  /** Announces an in-flight operation to assistive technology. */
  readonly busy?: boolean;
  /** Activation handler; omit for pure submit buttons. */
  readonly onClick?: () => void;
}

/** Renders the shared gilt call-to-action. */
export function MysticButton({
  label,
  type = 'button',
  disabled = false,
  busy = false,
  onClick,
}: MysticButtonProps): JSX.Element {
  return (
    <button
      type={type}
      disabled={disabled || busy}
      aria-busy={busy}
      {...(onClick === undefined ? {} : { onClick })}
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
