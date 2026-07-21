/**
 * The spread composition panel: name, deck selection, and a positions builder
 * with presets.
 *
 * Client-side rules mirror the BFF's (which remain authoritative): 1-10
 * positions, each non-empty and unique case-insensitively -- uniqueness
 * matters because the upstream keys its response by position, so duplicates
 * would silently collapse.
 */

import { useEffect, useId, useState } from 'react';

import { fetchDecks, type SpreadDraft } from '../lib/api.js';
import { formatDeckName } from '../lib/format.js';
import { MysticButton } from './MysticButton.js';

/** Product ceiling on positions; mirrors the BFF's MAX_SPREAD_POSITIONS. */
const MAX_POSITIONS = 10;

/** Longest position label the composer accepts; mirrors the BFF. */
const MAX_POSITION_LENGTH = 40;

/** A one-click starting point for the positions list. */
interface Preset {
  readonly label: string;
  readonly name: string;
  readonly positions: readonly string[];
}

/** Shipped presets. Celtic Cross uses the classic ten position names. */
const PRESETS: readonly Preset[] = [
  {
    label: 'Past · Present · Future',
    name: 'Past, Present, Future',
    positions: ['Past', 'Present', 'Future'],
  },
  {
    label: 'Celtic Cross',
    name: 'Celtic Cross',
    positions: [
      'Present',
      'Challenge',
      'Foundation',
      'Past',
      'Crown',
      'Future',
      'Self',
      'Environment',
      'Hopes & Fears',
      'Outcome',
    ],
  },
];

/** Props for {@link SpreadComposer}. */
export interface SpreadComposerProps {
  /** Submits the composed draft for drawing. */
  readonly onDraw: (draft: SpreadDraft) => void;
  /** True while a draw is in flight; disables the form. */
  readonly isDrawing: boolean;
  /** Draw failure to display, or `null`. */
  readonly error: string | null;
}

/** Shared styling for the dark form fields. */
const FIELD_CLASSES =
  'w-full rounded-lg border border-arcane bg-void/70 px-3 py-2 text-sm text-parchment placeholder:text-mist/40';

/** Renders the spread composition form. */
export function SpreadComposer({ onDraw, isDrawing, error }: SpreadComposerProps): JSX.Element {
  const [name, setName] = useState('');
  const [deck, setDeck] = useState('any');
  const [positions, setPositions] = useState<readonly string[]>(PRESETS[0]?.positions ?? []);
  const [draft, setDraft] = useState('');
  const [deckNames, setDeckNames] = useState<readonly string[] | null>(null);
  const nameId = useId();
  const deckId = useId();
  const positionId = useId();

  useEffect(() => {
    const controller = new AbortController();
    fetchDecks(controller.signal)
      .then(setDeckNames)
      .catch(() => {
        // The selector degrades to the two random modes; drawing still works.
        setDeckNames([]);
      });
    return () => {
      controller.abort();
    };
  }, []);

  const addPosition = (): void => {
    const position = draft.trim();
    if (
      position === '' ||
      position.length > MAX_POSITION_LENGTH ||
      positions.length >= MAX_POSITIONS ||
      positions.some((p) => p.toLowerCase() === position.toLowerCase())
    ) {
      return;
    }
    setPositions([...positions, position]);
    setDraft('');
  };

  const removePosition = (position: string): void => {
    setPositions(positions.filter((p) => p !== position));
  };

  const applyPreset = (preset: Preset): void => {
    setName(preset.name);
    setPositions(preset.positions);
  };

  const canDraw = positions.length > 0 && !isDrawing;

  return (
    <form
      className="w-full max-w-md rounded-2xl border border-arcane bg-obsidian/60 p-6 sm:p-8"
      onSubmit={(event) => {
        event.preventDefault();
        if (canDraw) {
          onDraw({ name: name.trim() === '' ? 'Custom Spread' : name.trim(), deck, positions });
        }
      }}
    >
      <fieldset disabled={isDrawing} className="flex flex-col gap-6">
        <div className="flex flex-col gap-2">
          <label htmlFor={nameId} className="text-xs tracking-[0.2em] text-mist uppercase">
            Spread name
          </label>
          <input
            id={nameId}
            type="text"
            value={name}
            maxLength={100}
            placeholder="Custom Spread"
            onChange={(event) => {
              setName(event.target.value);
            }}
            className={FIELD_CLASSES}
          />
        </div>

        <div className="flex flex-col gap-2">
          <label htmlFor={deckId} className="text-xs tracking-[0.2em] text-mist uppercase">
            Deck
          </label>
          <select
            id={deckId}
            value={deck}
            onChange={(event) => {
              setDeck(event.target.value);
            }}
            className={FIELD_CLASSES}
          >
            <optgroup label="Modes">
              <option value="any">Any deck — one deck for the whole spread</option>
              <option value="all">Every card from its own deck</option>
            </optgroup>
            <optgroup label={deckNames === null ? 'Decks (loading…)' : 'Decks'}>
              {(deckNames ?? []).map((deckName) => (
                <option key={deckName} value={deckName}>
                  {formatDeckName(deckName)}
                </option>
              ))}
            </optgroup>
          </select>
        </div>

        <div className="flex flex-col gap-2">
          <label htmlFor={positionId} className="text-xs tracking-[0.2em] text-mist uppercase">
            Positions
            <span className="ml-2 text-mist/60 normal-case tracking-normal">
              {positions.length}/{MAX_POSITIONS}
            </span>
          </label>

          <div className="flex flex-wrap gap-2">
            {positions.map((position) => (
              <span
                key={position}
                className="inline-flex items-center gap-1.5 rounded-full border border-arcane bg-void/70 py-1 pr-1.5 pl-3 text-xs text-parchment"
              >
                {position}
                <button
                  type="button"
                  aria-label={`Remove position ${position}`}
                  onClick={() => {
                    removePosition(position);
                  }}
                  className="rounded-full px-1 text-mist transition-colors hover:text-gilt"
                >
                  &times;
                </button>
              </span>
            ))}
          </div>

          <div className="flex gap-2">
            <input
              id={positionId}
              type="text"
              value={draft}
              maxLength={MAX_POSITION_LENGTH}
              placeholder={positions.length >= MAX_POSITIONS ? 'Limit reached' : 'Add a position…'}
              disabled={positions.length >= MAX_POSITIONS}
              onChange={(event) => {
                setDraft(event.target.value);
              }}
              onKeyDown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  addPosition();
                }
              }}
              className={FIELD_CLASSES}
            />
            <button
              type="button"
              onClick={addPosition}
              disabled={draft.trim() === '' || positions.length >= MAX_POSITIONS}
              className="rounded-lg border border-arcane px-4 text-sm text-mist transition-colors hover:enabled:border-gilt/50 hover:enabled:text-gilt disabled:cursor-not-allowed disabled:opacity-40"
            >
              Add
            </button>
          </div>

          <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1">
            {PRESETS.map((preset) => (
              <button
                key={preset.label}
                type="button"
                onClick={() => {
                  applyPreset(preset);
                }}
                className="text-[0.65rem] tracking-[0.15em] text-amethyst uppercase transition-colors hover:text-gilt"
              >
                {preset.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex flex-col items-center gap-3 pt-2">
          <MysticButton
            label={isDrawing ? 'Laying the spread…' : 'Draw'}
            type="submit"
            busy={isDrawing}
            disabled={!canDraw}
          />
          {error !== null && (
            <p role="alert" className="max-w-xs text-center text-sm text-parchment/90">
              {error}
            </p>
          )}
        </div>
      </fieldset>
    </form>
  );
}
