/**
 * Card image constants shared by every page.
 */

/**
 * Base URL of the card image store.
 *
 * Root-relative by default: behind the NGINX gateway the image store is served
 * on this app's own origin, which keeps the bundle free of any hard-coded
 * domain. Override with `VITE_IMAGE_BASE` to run outside the gateway.
 */
export const IMAGE_BASE = import.meta.env.VITE_IMAGE_BASE ?? '/image';

/** Card back shown while any card is face down. */
export const CARD_BACK_SRC = `${IMAGE_BASE}/tarot/back-images/back-01.jpg`;
