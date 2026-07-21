/**
 * Display formatting helpers shared across components.
 */

/** Formats a deck slug such as `dark_fairytale` for display: `Dark Fairytale`. */
export function formatDeckName(deck: string): string {
  return deck
    .split('_')
    .filter((part) => part.length > 0)
    .map((part) => `${part.charAt(0).toUpperCase()}${part.slice(1)}`)
    .join(' ');
}
