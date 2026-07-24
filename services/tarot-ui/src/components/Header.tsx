/**
 * Shared page header: wordmark plus navigation between the tarot pages.
 *
 * Navigation uses real anchors and full page loads -- two destinations do not
 * justify a client-side router, and real links get correct middle-click,
 * new-tab, and history behaviour for free.
 */

/** Which page is currently displayed, for `aria-current` and styling. */
export type TarotPage = 'single' | 'reading';

/** Props for {@link Header}. */
export interface HeaderProps {
  /** The page being rendered. */
  readonly current: TarotPage;
}

/** App mount path without a trailing slash, e.g. `/tarot`. */
const BASE = import.meta.env.BASE_URL.replace(/\/$/, '');

/** Nav link styled by whether it is the current page. */
function NavLink({
  href,
  label,
  active,
}: {
  readonly href: string;
  readonly label: string;
  readonly active: boolean;
}): JSX.Element {
  return (
    <a
      href={href}
      {...(active ? { 'aria-current': 'page' as const } : {})}
      className={`transition-colors duration-300 ${
        active ? 'text-gilt' : 'text-mist hover:text-parchment'
      }`}
    >
      {label}
    </a>
  );
}

/** Renders the shared wordmark and page navigation. */
export function Header({ current }: HeaderProps): JSX.Element {
  return (
    <header className="text-center">
      <h1 className="font-display text-3xl tracking-[0.3em] text-parchment uppercase sm:text-4xl">
        Thirdeye
      </h1>
      <p className="mt-2 text-sm tracking-[0.2em] text-mist uppercase">Tarot</p>

      <nav
        aria-label="Tarot pages"
        className="mt-5 flex items-center justify-center gap-8 text-xs tracking-[0.25em] uppercase"
      >
        <NavLink href={`${BASE}/`} label="Single Card" active={current === 'single'} />
        <span aria-hidden="true" className="text-arcane">
          &#10022;
        </span>
        <NavLink href={`${BASE}/reading`} label="Custom Reading" active={current === 'reading'} />
      </nav>
    </header>
  );
}
