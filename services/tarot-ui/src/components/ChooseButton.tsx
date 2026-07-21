/**
 * The single-card page's draw trigger: a {@link MysticButton} whose label
 * tracks the draw lifecycle.
 */

import { MysticButton } from './MysticButton.js';

/** Props for {@link ChooseButton}. */
export interface ChooseButtonProps {
  /** True while a draw is running; disables interaction. */
  readonly isDrawing: boolean;
  /** True once a card has been revealed, switching the label. */
  readonly hasDrawn: boolean;
  /** Invoked on activation. */
  readonly onChoose: () => void;
}

/** Renders the primary call to action. */
export function ChooseButton({ isDrawing, hasDrawn, onChoose }: ChooseButtonProps): JSX.Element {
  const label = isDrawing ? 'Consulting…' : hasDrawn ? 'Choose again' : 'Choose';
  return <MysticButton label={label} busy={isDrawing} onClick={onChoose} />;
}
