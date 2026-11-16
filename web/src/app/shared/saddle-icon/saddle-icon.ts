import { ChangeDetectionStrategy, Component } from '@angular/core';

/**
 * A saddle, drawn in the line weight of the Material Symbols the app uses (the font has none): the "Montar" button, the
 * "Montaria" panel's title and "Cavalgar" (588-01, 588-02). It is 1 em wide and takes the colour of the text beside it.
 * Decorative: the words beside it name it.
 */
@Component({
  selector: 'app-saddle-icon',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false">
      <path
        d="M3 9.5c0-1.1.9-2 2-2h2.2c.6 0 1.2.3 1.5.8.7 1 1.7 1.5 3.3 1.5s2.6-.5 3.3-1.5c.3-.5.9-.8 1.5-.8H19c1.1 0 2 .9 2 2 0 2.3-1.1 4.2-3.2 5.6V18a1 1 0 0 1-2 0v-1.6c-1.1.3-2.4.5-3.8.5s-2.7-.2-3.8-.5V18a1 1 0 0 1-2 0v-2.9C4.1 13.7 3 11.8 3 9.5Z"
      />
    </svg>
  `,
  styles: `
    :host {
      display: inline-flex;
      flex: none;
      width: 1.5em;
      height: 1.5em;
      vertical-align: middle;
    }

    svg {
      width: 100%;
      height: 100%;
      fill: none;
      stroke: currentColor;
      stroke-width: 1.8;
      stroke-linejoin: round;
    }
  `,
})
export class SaddleIcon {}
