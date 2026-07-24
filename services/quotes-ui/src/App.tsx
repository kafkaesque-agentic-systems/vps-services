/**
 * Application shell: selects the documentation page from the URL path.
 *
 * Two static destinations do not justify a router; the Node server serves the
 * same SPA shell for every path under the root, and this branch picks the
 * page. Navigation is by real anchors with full page loads.
 */

import { QuotesDocsPage } from './pages/QuotesDocsPage.js';
import { TarotDocsPage } from './pages/TarotDocsPage.js';

/** Root component. */
export function App(): JSX.Element {
  const path = window.location.pathname.replace(/\/+$/, '');
  return path === '/docs/tarot' ? <TarotDocsPage /> : <QuotesDocsPage />;
}
