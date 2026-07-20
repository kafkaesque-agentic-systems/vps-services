/**
 * Application shell.
 *
 * Deliberately thin: it owns no logic beyond wiring the draw hook to the two
 * presentational components. Semantic landmarks (`header`, `main`, `footer`)
 * give assistive technology real structure rather than a pile of divs.
 */

import { CardStage } from './components/CardStage.js';
import { ChooseButton } from './components/ChooseButton.js';
import { QuoteBlock } from './components/QuoteBlock.js';
import { useTarotCard } from './hooks/useTarotCard.js';

/** Root component. */
export function App(): JSX.Element {
  const { phase, card, quote, error, isDrawing, draw } = useTarotCard();

  return (
    <div className="flex min-h-screen flex-col items-center justify-between gap-8 px-6 py-10">
      <header className="text-center">
        <h1 className="font-display text-3xl tracking-[0.3em] text-parchment uppercase sm:text-4xl">
          ThirdEye
        </h1>
        <p className="mt-2 text-sm tracking-[0.2em] text-mist uppercase">Tarot</p>
      </header>

      <main className="flex flex-col items-center gap-6">
        <CardStage phase={phase} card={card} />

        <QuoteBlock quote={quote} visible={phase === 'revealing'} />

        <div className="flex flex-col items-center gap-4">
          <ChooseButton isDrawing={isDrawing} hasDrawn={card !== null} onChoose={draw} />

          {/*
            role="alert" so failures are announced immediately -- unlike the
            card caption, an error is something the user must act on.
          */}
          {error !== null && (
            <p
              role="alert"
              className="max-w-xs text-center text-sm text-parchment/90 animate-rise-in"
            >
              {error}
            </p>
          )}
        </div>
      </main>

      <footer className="text-center text-xs tracking-widest text-mist/60 uppercase">
        <p>Draw a single card</p>
      </footer>
    </div>
  );
}
