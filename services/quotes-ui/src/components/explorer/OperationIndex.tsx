/**
 * The operation index: method-badged anchor chips into the cards below.
 * Shared by both documentation pages.
 */

import { MethodBadge, type HttpMethod } from './primitives.js';

/** One entry in an operation index. */
export interface IndexEntry {
  readonly id: string;
  readonly method: HttpMethod;
  readonly label: string;
}

/** Renders a centred row of index chips. */
export function IndexChips({ entries }: { readonly entries: readonly IndexEntry[] }): JSX.Element {
  return (
    <div className="flex flex-wrap justify-center gap-2">
      {entries.map((entry) => (
        <a
          key={entry.id}
          href={`#${entry.id}`}
          className="inline-flex items-center gap-2 rounded-full border border-arcane bg-obsidian/60 py-1.5 pr-4 pl-2 text-xs text-mist transition-colors hover:border-amethyst/50 hover:text-parchment"
        >
          <MethodBadge method={entry.method} />
          {entry.label}
        </a>
      ))}
    </div>
  );
}
