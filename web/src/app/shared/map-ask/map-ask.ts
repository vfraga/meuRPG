import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  inject,
  input,
  output,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';

import { focusWithRing } from '../../core/creatures/focus-ring';

let nextId = 0;

/**
 * An in-place question of the map editor (E9-01 4: "Mudar a grade?", "Trocar a imagem?", "Esquecer o que foi
 * visto?"): it opens inside the panel of whoever asked, with the focus on its title, "Voltar" first (outlined)
 * and the one filled button under it, which says what it erases. Nothing is erased before the second click;
 * Esc closes it. Presentational: the screen puts the words and the field between the title and the buttons,
 * runs the call, and puts the focus back on the button that asked. Both buttons are as wide as the room, 44 px
 * high (48 px on a phone).
 */
@Component({
  selector: 'app-map-ask',
  imports: [MatButtonModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="ask" role="group" [attr.aria-labelledby]="id" (keydown.escape)="cancel.emit()">
      <h3 #titleEl class="ask__title" [id]="id" tabindex="-1">{{ title() }}</h3>
      <ng-content />
      <div class="ask__pair" [class.ask__pair--row]="row()" [class.ask__pair--equal]="equal()">
        <button matButton="outlined" type="button" (click)="cancel.emit()">{{ cancelLabel() }}</button>
        <button
          matButton="filled"
          type="button"
          [class.mr-button--off]="!ready()"
          [disabled]="busy() || !ready()"
          disabledInteractive
          (click)="!busy() && ready() && confirm.emit()"
        >
          {{ confirmLabel() }}
        </button>
      </div>
      <ng-content select="[extra]" />
    </section>
  `,
  styles: `
    :host {
      display: block;
    }

    .ask {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      scroll-margin-top: calc(64px + var(--mr-space-3));
    }

    // Room for the focus ring around the words, so it never touches them.
    // Room under the title too, for the sticky save bar of "Regras da mesa" on a phone.
    .ask__title {
      // The ring hugs the words, never a box across the whole panel (it read as an error frame).
      align-self: flex-start;
      margin: 0 -6px;
      padding: 2px 6px;
      font-family: var(--mr-font-display);
      font-size: 22px;
      font-weight: 700;
      line-height: 28px;
    }

    .ask__title:focus-visible,
    .ask__title[data-ring] {
      border-radius: 4px;
      outline: 2px solid var(--mr-focus);
      outline-offset: 2px;
    }

    .ask__pair {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
    }

    // Beside the words (the header's question): two buttons in a row, so the page below does not move far.
    .ask__pair--row {
      flex-direction: row;
      flex-wrap: wrap;
    }

    .ask__pair button {
      width: 100%;
      min-height: 44px;

      @media (max-width: 767.98px) {
        min-height: 48px;
      }
    }

    // After the rule above, which it overrides: side by side, each as wide as half the card.
    .ask__pair--row button {
      flex: 1 1 11rem;
      width: auto;
    }

    /* The puzzles' questions (E10-06 state 5): the two buttons stay side by side and equal at any width, 48 px high, even at 320 px,
       where 11rem each would stack them. Opt-in: the other screens keep theirs. */
    .ask__pair--equal {
      flex-wrap: nowrap;
    }

    .ask__pair--equal button {
      flex: 1 1 0;
      min-width: 0;
      min-height: 48px;
      white-space: nowrap;
    }
  `,
})
export class MapAsk {
  private readonly injector = inject(Injector);
  protected readonly id = `ask-${nextId++}`;
  private readonly heading = viewChild.required<ElementRef<HTMLElement>>('titleEl');

  readonly title = input.required<string>();
  readonly confirmLabel = input.required<string>();
  /** The filled button can be pressed; false draws the dashed one (the reason is a sentence above). */
  readonly ready = input(true);
  readonly busy = input(false);
  /** The two buttons side by side instead of one over the other (the header, where a tall question would push the map down). */
  readonly row = input(false);
  /** With `row`: the buttons share the row equally and never wrap, at any width (the puzzles' questions). */
  readonly equal = input(false);
  /** The words of the safe button ("Voltar" by default). */
  readonly cancelLabel = input('Voltar');
  readonly cancel = output<void>();
  readonly confirm = output<void>();

  constructor() {
    afterNextRender(
      () => {
        const el = this.heading().nativeElement;
        el.scrollIntoView?.({ block: 'nearest' });
        focusWithRing(el);
      },
      { injector: this.injector },
    );
  }
}
