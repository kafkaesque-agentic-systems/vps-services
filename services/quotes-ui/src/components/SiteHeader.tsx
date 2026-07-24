/**
 * Shared site header: wordmark plus navigation between the two API
 * documentation pages. Real anchors, no router.
 */

/** Props for {@link SiteHeader}. */
export interface SiteHeaderProps {
  /** Which documentation page is being rendered. */
  readonly current: 'quotes' | 'tarot';
}

/** Nav link styled by whether it is the current property. */
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
        active ? 'text-lilac' : 'text-mist hover:text-parchment'
      }`}
    >
      {label}
    </a>
  );
}

/** Renders the wordmark and property navigation. */
export function SiteHeader({ current }: SiteHeaderProps): JSX.Element {
  return (
    <header className="pt-10 text-center">
      <p className="font-display text-3xl tracking-[0.3em] text-parchment uppercase sm:text-4xl">
        Thirdeye
      </p>
      <p className="mt-2 text-sm tracking-[0.2em] text-mist uppercase">API Reference</p>

      <nav
        aria-label="API documentation"
        className="mt-5 flex items-center justify-center gap-8 text-xs tracking-[0.25em] uppercase"
      >
        <NavLink href="/" label="Quotes API" active={current === 'quotes'} />
        <span aria-hidden="true" className="text-arcane">
          &#10022;
        </span>
        <NavLink href="/docs/tarot" label="Tarot API" active={current === 'tarot'} />
      </nav>
    </header>
  );
}
