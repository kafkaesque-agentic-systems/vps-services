/**
 * The eight operation panels of the API explorer.
 *
 * Each panel is a thin composition of the shared primitives: a form bound to
 * local state, a runner, and a result view. Read panels call the typed BFF
 * client; the token-gated write panels use the verbatim pass-through so the
 * page demonstrates the API's real responses, status codes and all.
 */

import { useState } from 'react';

import {
  fetchAuthors,
  fetchQuoteById,
  fetchQuotesByAuthor,
  fetchRandomQuote,
  searchQuotes,
  writePassthrough,
} from '../../lib/api.js';
import { Field, JsonView, OperationCard, RunButton } from './primitives.js';
import type { RunnerState } from './useRunner.js';
import { useRunner } from './useRunner.js';

/** Public API host shown in the curl examples. */
const API_HOST = 'https://api.thirdeye.live';

/** Shared result area: error alert or highlighted JSON. */
function RunResult({ state }: { readonly state: RunnerState }): JSX.Element | null {
  if (state.error !== null) {
    return (
      <p role="alert" className="text-sm text-method-delete">
        {state.error}
      </p>
    );
  }
  if (state.result === undefined) {
    return null;
  }
  return <JsonView data={state.result} />;
}

/** GET /quote — random quote. */
export function RandomQuotePanel(): JSX.Element {
  const runner = useRunner();
  return (
    <OperationCard
      id="op-random"
      method="GET"
      path="/quote"
      title="Random quote"
      description="Returns one random quote from the collection."
      curl={`curl ${API_HOST}/quote \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(fetchRandomQuote);
        }}
      >
        <RunButton label="Try it" busy={runner.busy} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** GET /quote/:id — quote by id. */
export function QuoteByIdPanel(): JSX.Element {
  const runner = useRunner();
  const [id, setId] = useState('');
  return (
    <OperationCard
      id="op-by-id"
      method="GET"
      path="/quote/:id"
      title="Quote by id"
      description="Fetches a single quote by its 24-character object id."
      curl={`curl ${API_HOST}/quote/633ad6fdfee324ff7116b1f8 \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() => fetchQuoteById(id.trim()));
        }}
      >
        <Field label="Quote id" value={id} onChange={setId} placeholder="633ad6fdfee324ff7116b1f8" />
        <RunButton label="Fetch" busy={runner.busy} disabled={id.trim() === ''} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** GET /authors — the author catalogue. */
export function AuthorsPanel(): JSX.Element {
  const runner = useRunner();
  return (
    <OperationCard
      id="op-authors"
      method="GET"
      path="/authors"
      title="All authors"
      description="Lists every author represented in the collection."
      curl={`curl ${API_HOST}/authors \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(fetchAuthors);
        }}
      >
        <RunButton label="Try it" busy={runner.busy} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** GET /authors/:name — quotes by author. */
export function AuthorQuotesPanel(): JSX.Element {
  const runner = useRunner();
  const [name, setName] = useState('');
  return (
    <OperationCard
      id="op-author-quotes"
      method="GET"
      path="/authors/:name"
      title="Quotes by author"
      description="All quotes attributed to one author. Type a name naturally — punctuation and casing are normalised for you."
      curl={`curl ${API_HOST}/authors/james-dean \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() => fetchQuotesByAuthor(name));
        }}
      >
        <Field label="Author" value={name} onChange={setName} placeholder="James Dean" />
        <RunButton label="Fetch" busy={runner.busy} disabled={name.trim() === ''} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** POST /quote/search — keyword and phrase search. */
export function SearchPanel(): JSX.Element {
  const runner = useRunner();
  const [query, setQuery] = useState('');
  return (
    <OperationCard
      id="op-search"
      method="POST"
      path="/quote/search"
      title="Search"
      description="Comma-separated words and phrases; entries containing a space search as phrases. Escape a literal comma inside a phrase with a backslash."
      curl={`curl -X POST ${API_HOST}/quote/search \\\n  -H 'Authorization: <your token>' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"terms":["meditation"],"phrases":["when one experiences truth"]}'`}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() => searchQuotes(query));
        }}
      >
        <Field
          label="Search terms"
          value={query}
          onChange={setQuery}
          placeholder="meditation, when one experiences truth"
        />
        <RunButton label="Search" busy={runner.busy} disabled={query.trim() === ''} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** Shared token field state and helper copy for the write panels. */
function TokenField({
  token,
  onChange,
}: {
  readonly token: string;
  readonly onChange: (next: string) => void;
}): JSX.Element {
  return (
    <Field
      label="Your API token"
      value={token}
      onChange={onChange}
      type="password"
      placeholder="Paste the token issued to you"
    />
  );
}

/** POST /quote — add a quote (token required). */
export function AddQuotePanel(): JSX.Element {
  const runner = useRunner();
  const [token, setToken] = useState('');
  const [attribution, setAttribution] = useState('');
  const [text, setText] = useState('');
  return (
    <OperationCard
      id="op-add"
      method="POST"
      path="/quote"
      title="Add a quote"
      description="Submits a new quote under your token. The response below is the API's own, status code and all."
      curl={`curl -X POST ${API_HOST}/quote \\\n  -H 'Authorization: <your token>' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"attribution":"seneca","quote":"..."}'`}
      authorized
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() =>
            writePassthrough('POST', '/quote', token.trim(), {
              attribution: attribution.trim(),
              quote: text.trim(),
            }),
          );
        }}
      >
        <TokenField token={token} onChange={setToken} />
        <Field label="Attribution" value={attribution} onChange={setAttribution} placeholder="seneca" />
        <Field label="Quote" value={text} onChange={setText} placeholder="The quote text…" />
        <RunButton
          label="Submit"
          busy={runner.busy}
          disabled={token.trim() === '' || attribution.trim() === '' || text.trim() === ''}
        />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** PUT /quote/:id — update a quote (token required). */
export function UpdateQuotePanel(): JSX.Element {
  const runner = useRunner();
  const [token, setToken] = useState('');
  const [id, setId] = useState('');
  const [attribution, setAttribution] = useState('');
  const [text, setText] = useState('');
  return (
    <OperationCard
      id="op-update"
      method="PUT"
      path="/quote/:id"
      title="Update a quote"
      description="Rewrites a quote you own. Requires the quote id and your token."
      curl={`curl -X PUT ${API_HOST}/quote/<id> \\\n  -H 'Authorization: <your token>' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"id":"<id>","attribution":"...","quote":"..."}'`}
      authorized
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() =>
            writePassthrough('PUT', `/quote/${encodeURIComponent(id.trim())}`, token.trim(), {
              id: id.trim(),
              attribution: attribution.trim(),
              quote: text.trim(),
            }),
          );
        }}
      >
        <TokenField token={token} onChange={setToken} />
        <Field label="Quote id" value={id} onChange={setId} placeholder="24-character hex id" />
        <Field label="Attribution" value={attribution} onChange={setAttribution} />
        <Field label="Quote" value={text} onChange={setText} />
        <RunButton
          label="Update"
          busy={runner.busy}
          disabled={token.trim() === '' || id.trim() === ''}
        />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** DELETE /quote/:id — delete a quote (token required). */
export function DeleteQuotePanel(): JSX.Element {
  const runner = useRunner();
  const [token, setToken] = useState('');
  const [id, setId] = useState('');
  return (
    <OperationCard
      id="op-delete"
      method="DELETE"
      path="/quote/:id"
      title="Delete a quote"
      description="Removes a quote you own. This is irreversible — the API will hold you to that."
      curl={`curl -X DELETE ${API_HOST}/quote/<id> \\\n  -H 'Authorization: <your token>'`}
      authorized
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() =>
            writePassthrough('DELETE', `/quote/${encodeURIComponent(id.trim())}`, token.trim()),
          );
        }}
      >
        <TokenField token={token} onChange={setToken} />
        <Field label="Quote id" value={id} onChange={setId} placeholder="24-character hex id" />
        <RunButton
          label="Delete"
          busy={runner.busy}
          disabled={token.trim() === '' || id.trim() === ''}
        />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}
