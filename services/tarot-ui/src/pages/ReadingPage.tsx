/**
 * The custom reading page: compose a spread, draw it, and turn each card by
 * hand.
 *
 * Lives at `{basePath}/reading` -- NOT `/tarot/spread`, which NGINX
 * prefix-routes to the Go API (and prefix matching ignores segment
 * boundaries, so even `/tarot/spreads` would be swallowed).
 */

import { useCallback, useEffect, useState } from 'react';

import { CardViewer } from '../components/CardViewer.js';
import { FlipCard } from '../components/FlipCard.js';
import { Header } from '../components/Header.js';
import { MysticButton } from '../components/MysticButton.js';
import { SpreadComposer } from '../components/SpreadComposer.js';
import { useSpread } from '../hooks/useSpread.js';
import type { SpreadCard } from '../types/tarot.js';

/** Renders the custom spread page. */
export function ReadingPage(): JSX.Element {
  const { phase, reading, turned, error, isDrawing, draw, turn, reset } = useSpread();

  // One slot: only a single card is ever in the floating viewer at a time.
  const [viewing, setViewing] = useState<SpreadCard | null>(null);
  // Stable identity so the viewer's focus/scroll-lock effect runs once per
  // open rather than on every parent render.
  const closeViewer = useCallback(() => {
    setViewing(null);
  }, []);

  useEffect(() => {
    document.title = 'ThirdEye — Tarot Reading';
  }, []);

  return (
    <div className="flex min-h-screen flex-col items-center justify-between gap-8 px-6 py-10">
      <Header current="reading" />

      <main className="flex w-full flex-col items-center gap-8">
        {phase === 'compose' && (
          <SpreadComposer onDraw={draw} isDrawing={isDrawing} error={error} />
        )}

        {phase === 'reading' && reading !== null && (
          <section
            aria-label={`Reading: ${reading.name}`}
            className="flex w-full flex-col items-center gap-8 animate-rise-in"
          >
            <div className="text-center">
              <h2 className="font-display text-xl tracking-[0.2em] text-parchment uppercase">
                {reading.name}
              </h2>
              {/* Reserved height: the hint disappears once every card is up. */}
              <p className="mt-2 min-h-[1rem] text-xs tracking-[0.2em] text-mist/70 uppercase">
                {turned.size < reading.cards.length
                  ? `${reading.cards.length} cards · click each to reveal`
                  : ' '}
              </p>
            </div>

            <ul className="flex max-w-4xl list-none flex-wrap justify-center gap-x-6 gap-y-8 p-0">
              {reading.cards.map((card) => (
                <li key={card.position}>
                  <FlipCard
                    card={card}
                    turned={turned.has(card.position)}
                    onTurn={turn}
                    onView={setViewing}
                  />
                </li>
              ))}
            </ul>

            <MysticButton
              label="New Reading"
              onClick={() => {
                setViewing(null);
                reset();
              }}
            />
          </section>
        )}
      </main>

      {viewing !== null && <CardViewer card={viewing} onClose={closeViewer} />}

      <footer className="text-center text-xs tracking-widest text-mist/60 uppercase">
        <p>Compose a custom spread</p>
      </footer>
    </div>
  );
}
