/**
 * The Quotes API documentation page, served at the domain root.
 */

import { useEffect } from 'react';

import { QuoteHero } from '../components/QuoteHero.js';
import { SiteHeader } from '../components/SiteHeader.js';
import { TokenRequestCard } from '../components/TokenRequestCard.js';
import { IndexChips, type IndexEntry } from '../components/explorer/OperationIndex.js';
import {
  AddQuotePanel,
  AuthorQuotesPanel,
  AuthorsPanel,
  DeleteQuotePanel,
  QuoteByIdPanel,
  RandomQuotePanel,
  SearchPanel,
  UpdateQuotePanel,
} from '../components/explorer/panels.js';

/** The quotes operations, in page order. */
const INDEX: readonly IndexEntry[] = [
  { id: 'op-random', method: 'GET', label: 'Random quote' },
  { id: 'op-by-id', method: 'GET', label: 'Quote by id' },
  { id: 'op-authors', method: 'GET', label: 'All authors' },
  { id: 'op-author-quotes', method: 'GET', label: 'Quotes by author' },
  { id: 'op-search', method: 'POST', label: 'Search' },
  { id: 'op-add', method: 'POST', label: 'Add a quote' },
  { id: 'op-update', method: 'PUT', label: 'Update a quote' },
  { id: 'op-delete', method: 'DELETE', label: 'Delete a quote' },
];

/** Renders the quotes documentation page. */
export function QuotesDocsPage(): JSX.Element {
  useEffect(() => {
    document.title = 'Thirdeye — Quotes API';
  }, []);

  return (
    <div className="flex min-h-screen flex-col gap-14 pb-16">
      <SiteHeader current="quotes" />

      <QuoteHero />

      <main className="mx-auto w-full max-w-5xl px-6">
        <section aria-label="Quotes API reference">
          <div className="text-center">
            <h2 className="font-display text-2xl tracking-[0.2em] text-parchment uppercase">
              The Quotes API
            </h2>
            <p className="mx-auto mt-2 max-w-xl text-sm leading-relaxed text-mist">
              Thousands of quotes, searchable by keyword, phrase and author. Every call requires
              an authorization token — request yours below. Each operation runs live, right here.
            </p>
          </div>

          <nav aria-label="Operations" className="mx-auto mt-8 max-w-3xl">
            <IndexChips entries={INDEX} />
          </nav>

          <div className="mt-10 grid grid-cols-1 gap-6 lg:grid-cols-2">
            <RandomQuotePanel />
            <QuoteByIdPanel />
            <AuthorsPanel />
            <AuthorQuotesPanel />
            <SearchPanel />
            <AddQuotePanel />
            <UpdateQuotePanel />
            <DeleteQuotePanel />
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
