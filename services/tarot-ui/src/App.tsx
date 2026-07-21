/**
 * Application shell: selects the page from the URL path.
 *
 * Two static destinations do not justify a router dependency; the Node server
 * serves the same SPA shell for every path under the base, and this branch
 * picks the page. Navigation is by real anchors with full page loads.
 */

import { ReadingPage } from './pages/ReadingPage.js';
import { SingleCardPage } from './pages/SingleCardPage.js';

/** Root component. */
export function App(): JSX.Element {
  const path = window.location.pathname.replace(/\/+$/, '');
  return path.endsWith('/reading') ? <ReadingPage /> : <SingleCardPage />;
}
