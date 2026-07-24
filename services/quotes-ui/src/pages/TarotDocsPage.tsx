/**
 * The Tarot API documentation page.
 *
 * Served at /docs/tarot — deliberately NOT under /tarot…, because the NGINX
 * gateway prefix-matches by string and `location /tarot` (the experience app)
 * would swallow any such path, including /tarot-docs.
 */

import { useEffect } from 'react';

import { SiteHeader } from '../components/SiteHeader.js';
import { TokenRequestCard } from '../components/TokenRequestCard.js';
import { IndexChips, type IndexEntry } from '../components/explorer/OperationIndex.js';
import {
  TarotCardPanel,
  TarotDeckByIdPanel,
  TarotDecksPanel,
  TarotRandomDeckPanel,
  TarotSpreadPanel,
} from '../components/explorer/tarotPanels.js';

/** The tarot operations, in page order. */
const INDEX: readonly IndexEntry[] = [
  { id: 'op-tarot-card', method: 'GET', label: 'Random card' },
  { id: 'op-tarot-deck', method: 'GET', label: 'Random deck' },
  { id: 'op-tarot-deck-id', method: 'GET', label: 'Deck by id' },
  { id: 'op-tarot-decks', method: 'GET', label: 'All decks' },
  { id: 'op-tarot-spread', method: 'POST', label: 'Custom spread' },
];

/** Renders the tarot documentation page. */
export function TarotDocsPage(): JSX.Element {
  useEffect(() => {
    document.title = 'Thirdeye — Tarot API';
  }, []);

  return (
    <div className="flex min-h-screen flex-col gap-14 pb-16">
      <SiteHeader current="tarot" />

      <main className="mx-auto w-full max-w-5xl px-6">
        <section aria-label="Tarot API reference">
          <div className="text-center">
            <h2 className="font-display text-2xl tracking-[0.2em] text-parchment uppercase">
              The Tarot API
            </h2>
            <p className="mx-auto mt-2 max-w-xl text-sm leading-relaxed text-mist">
              Seventy decks of card imagery: random draws, full decks, and custom spreads. Every
              call requires an authorization token — request yours below.
            </p>
            <p className="mt-3 text-xs tracking-[0.15em] text-mist/70 uppercase">
              These endpoints power the{' '}
              <a
                href="/tarot"
                className="text-lilac transition-colors hover:text-parchment"
              >
                tarot experience
              </a>
              &nbsp;— try a reading there.
            </p>
          </div>

          <nav aria-label="Operations" className="mx-auto mt-8 max-w-3xl">
            <IndexChips entries={INDEX} />
          </nav>

          <div className="mt-10 grid grid-cols-1 gap-6 lg:grid-cols-2">
            <TarotCardPanel />
            <TarotRandomDeckPanel />
            <TarotDeckByIdPanel />
            <TarotDecksPanel />
            <TarotSpreadPanel />
          </div>
        </section>

        <div className="mt-14">
          <TokenRequestCard />
        </div>
      </main>

      <footer className="text-center text-xs tracking-widest text-mist/60 uppercase">
        <p>Thirdeye API</p>
      </footer>
    </div>
  );
}
