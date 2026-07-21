/**
 * The single-card page: draw one card with an accompanying quote.
 *
 * This is the original page content, moved from App when the app gained a
 * second page; behaviour is unchanged.
 */

import { CardStage } from '../components/CardStage.js';
import { ChooseButton } from '../components/ChooseButton.js';
import { Header } from '../components/Header.js';
import { QuoteBlock } from '../components/QuoteBlock.js';
import { useTarotCard } from '../hooks/useTarotCard.js';

/** Renders the single-card draw page. */
export function SingleCardPage(): JSX.Element {
  const { phase, card, quote, error, isDrawing, draw } = useTarotCard();

  return (
    <div className="flex min-h-screen flex-col items-center justify-between gap-8 px-6 py-10">
      <Header current="single" />

      <main className="flex flex-col items-center gap-6">
        <CardStage phase={phase} card={card} />

        <QuoteBlock quote={quote} visible={phase === 'revealing'} />

        <div className="flex flex-col items-center gap-4">
          <ChooseButton isDrawing={isDrawing} hasDrawn={card !== null} onChoose={draw} />

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
