/**
 * The five tarot operation panels of the API reference.
 *
 * All public — the tarot surface has no authorized writes. Responses are the
 * upstream's own, relayed verbatim, so what a developer sees here is exactly
 * what their own client will receive.
 */

import { useState } from 'react';

import { publicPassthrough } from '../../lib/api.js';
import { Field, JsonView, OperationCard, RunButton } from './primitives.js';
import type { RunnerState } from './useRunner.js';
import { useRunner } from './useRunner.js';

/** Public API host shown in the curl examples. */
const API_HOST = 'https://api.thirdeye.live';

/** Shared result area (duplicated from panels.tsx by design: same primitive,
 * different module — the two groups stay independently editable). */
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

/** GET /tarot/card — random card. */
export function TarotCardPanel(): JSX.Element {
  const runner = useRunner();
  return (
    <OperationCard
      id="op-tarot-card"
      method="GET"
      path="/tarot/card"
      title="Random card"
      description="Draws one card from a random deck. The card value is a direct image URL."
      curl={`curl ${API_HOST}/tarot/card \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() => publicPassthrough('GET', '/tarot/card'));
        }}
      >
        <RunButton label="Try it" busy={runner.busy} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** GET /tarot/decks — deck catalogue. */
export function TarotDecksPanel(): JSX.Element {
  const runner = useRunner();
  return (
    <OperationCard
      id="op-tarot-decks"
      method="GET"
      path="/tarot/decks"
      title="All decks"
      description="Lists the name of every tarot deck in the collection."
      curl={`curl ${API_HOST}/tarot/decks \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() => publicPassthrough('GET', '/tarot/decks'));
        }}
      >
        <RunButton label="Try it" busy={runner.busy} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** GET /tarot/deck — random full deck. */
export function TarotRandomDeckPanel(): JSX.Element {
  const runner = useRunner();
  return (
    <OperationCard
      id="op-tarot-deck"
      method="GET"
      path="/tarot/deck"
      title="Random deck"
      description="Returns one random deck in full: its id, name, and every card image URL."
      curl={`curl ${API_HOST}/tarot/deck \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() => publicPassthrough('GET', '/tarot/deck'));
        }}
      >
        <RunButton label="Try it" busy={runner.busy} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** GET /tarot/deck/:id — deck by id. */
export function TarotDeckByIdPanel(): JSX.Element {
  const runner = useRunner();
  const [id, setId] = useState('');
  return (
    <OperationCard
      id="op-tarot-deck-id"
      method="GET"
      path="/tarot/deck/:id"
      title="Deck by id"
      description="Fetches a specific deck by its 24-character object id (take one from a card draw)."
      curl={`curl ${API_HOST}/tarot/deck/63ea8a9f95cb628e0f9e1f11 \\\n  -H 'Authorization: <your token>'`}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          runner.run(() =>
            publicPassthrough('GET', `/tarot/deck/${encodeURIComponent(id.trim())}`),
          );
        }}
      >
        <Field label="Deck id" value={id} onChange={setId} placeholder="24-character hex id" />
        <RunButton label="Fetch" busy={runner.busy} disabled={id.trim() === ''} />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}

/** POST /tarot/spread — custom spread. */
export function TarotSpreadPanel(): JSX.Element {
  const runner = useRunner();
  const [name, setName] = useState('Past, Present, Future');
  const [deck, setDeck] = useState('any');
  const [positions, setPositions] = useState('past, present, future');
  return (
    <OperationCard
      id="op-tarot-spread"
      method="POST"
      path="/tarot/spread"
      title="Custom spread"
      description={
        'Deals a named spread. deck may be a deck name, "any" (one random deck, drawn without replacement) or "all" (every position from its own random deck). Up to 78 positions.'
      }
      curl={`curl -X POST ${API_HOST}/tarot/spread \\\n  -H 'Authorization: <your token>' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"name":"PPF","deck":"any","positions":["past","present","future"]}'`}
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          const positionList = positions
            .split(',')
            .map((p) => p.trim())
            .filter((p) => p.length > 0);
          runner.run(() =>
            publicPassthrough('POST', '/tarot/spread', {
              name: name.trim(),
              deck: deck.trim(),
              positions: positionList,
            }),
          );
        }}
      >
        <Field label="Spread name" value={name} onChange={setName} />
        <Field label="Deck" value={deck} onChange={setDeck} placeholder='any, all, or a deck name' />
        <Field
          label="Positions (comma separated)"
          value={positions}
          onChange={setPositions}
          placeholder="past, present, future"
        />
        <RunButton
          label="Deal"
          busy={runner.busy}
          disabled={deck.trim() === '' || positions.trim() === ''}
        />
      </form>
      <RunResult state={runner} />
    </OperationCard>
  );
}
