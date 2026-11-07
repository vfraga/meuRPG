import { ChangeDetectionStrategy, Component, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * One row of the Tela da mesa panel: a real radio button under the row (the arrow keys move the choice, a screen reader
 * hears the title, the subtitle and "selecionado, 2 de 6"), the kind's icon, the title and one line under it, and
 * the row's own control at the right (`<ng-content>`, the ⓘ of "Espelhar"). The chosen row is filled and ringed, never
 * by colour alone (the radio dot); a row that cannot be picked is dashed and its subtitle says why.
 */
@Component({
  selector: 'app-tv-choice-row',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule],
  template: `
    <div class="row" [class.row--on]="selected()" [class.row--off]="!available()">
      <label class="row__main">
        <input
          type="radio"
          class="mr-visually-hidden"
          [name]="group()"
          [checked]="selected()"
          [disabled]="!available() || busy()"
          (change)="picked.emit()"
        />
        <span class="row__radio" aria-hidden="true">
          @if (!available()) {
            <mat-icon>block</mat-icon>
          }
        </span>
        <mat-icon class="row__icon" aria-hidden="true">{{ icon() }}</mat-icon>
        <span class="row__text">
          <span class="row__title">{{ title() }}</span>
          <span class="row__sub">{{ sub() }}</span>
        </span>
      </label>
      <span class="row__aside"><ng-content /></span>
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .row {
      display: flex;
      align-items: center;
      box-sizing: border-box;
      min-height: 48px;
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-sm);
      background: var(--mr-surface);
      color: var(--mr-ink);

      &:has(input:focus-visible) {
        outline: 3px solid var(--mr-focus);
        outline-offset: 2px;
      }
    }

    .row--on {
      border: 2px solid var(--mr-accent);
      background: var(--mr-accent-soft);
    }

    .row--off {
      border-style: dashed;
      background: transparent;
      color: var(--mr-ink-muted);
    }

    .row__main {
      display: flex;
      flex: 1;
      align-items: center;
      gap: 12px;
      min-width: 0;
      padding: 4px 10px 4px 14px;
      cursor: pointer;
    }

    .row--off .row__main {
      cursor: default;
    }

    // The radio dot; a row that cannot be picked has the "not allowed" mark in its place.
    .row__radio {
      display: inline-flex;
      flex: none;
      align-items: center;
      justify-content: center;
      box-sizing: border-box;
      width: 20px;
      height: 20px;
      border: 2px solid var(--mr-ink-muted);
      border-radius: var(--mr-radius-pill);
    }

    .row--on .row__radio {
      border-color: var(--mr-accent);
      background: radial-gradient(circle, var(--mr-accent) 45%, transparent 50%);
    }

    .row--off .row__radio {
      width: 24px;
      height: 24px;
      border: 0;

      .mat-icon {
        width: 22px;
        height: 22px;
        font-size: var(--mr-font-20);
      }
    }

    .row__icon {
      flex: none;
      width: 24px;
      height: 24px;
      font-size: var(--mr-font-24);
    }

    .row__text {
      display: flex;
      flex-direction: column;
      min-width: 0;
    }

    .row__title {
      font-size: var(--mr-font-15);
      font-weight: 700;
      line-height: 20px;
    }

    .row__sub {
      font-size: var(--mr-font-13);
      line-height: 17px;
      color: var(--mr-ink-muted);
    }

    // From a tablet up the rows are tighter: the panel must fit a 1280x720 screen.
    @media (min-width: 768px) {
      .row {
        min-height: 44px;
      }

      .row__main {
        padding-block: 2px;
      }

      .row__sub {
        line-height: 16px;
      }
    }

    .row__aside:not(:empty) {
      flex: none;
      margin-inline-end: 6px;
    }
  `,
})
export class TvChoiceRow {
  readonly group = input.required<string>();
  readonly icon = input.required<string>();
  readonly title = input.required<string>();
  readonly sub = input.required<string>();
  readonly selected = input(false);
  /** False: the row cannot be picked now (dashed, with its reason in the subtitle). */
  readonly available = input(true);
  /** A choice is on its way: no row can be picked. */
  readonly busy = input(false);

  /** The master picked this row. */
  readonly picked = output<void>();
}
