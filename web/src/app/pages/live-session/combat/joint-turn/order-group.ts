import { Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

/**
 * The box of a joint turn in an order list (E8-01, MR-013): combatants adjacent
 * in the order with the same initiative total, drawn as one box with the word
 * "Turno conjunto", a link icon and "iniciativa 19" (1,5px control line, 10px
 * corners, never a side stripe). The group whose turn is running has the accent
 * border and fill, and the "Vez" pill. The rows are projected. Its styles live
 * here, so they load with the session page and not with the app shell.
 */
@Component({
  selector: 'app-order-group',
  imports: [MatIconModule],
  template: `
    <div class="box" role="group" [attr.aria-label]="label()" [class.box--turn]="onTurn()">
      <p class="head">
        <mat-icon aria-hidden="true">link</mat-icon>
        <b>{{ mounted() ? 'Montado' : 'Turno conjunto' }}</b>
        @if (names()) {
          <span>{{ names() }} ·</span>
        }
        <span>iniciativa {{ total() }}</span>
        @if (onTurn()) {
          <span class="vez">Vez</span>
        } @else if (hint()) {
          <span class="hint">{{ hint() }}</span>
        }
      </p>
      <ng-content />
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .box {
      box-sizing: border-box;
      margin: 8px 0;
      border: 1.5px solid var(--mr-control-line);
      border-radius: 10px;
    }

    ::ng-deep ol {
      margin: 0;
      padding: 0;
      list-style: none;
    }

    .box--turn {
      border-color: var(--mr-accent);
      background: var(--mr-accent-soft);
    }

    .head {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: 4px 8px;
      margin: 0;
      padding: 10px 12px 4px;
      font-size: var(--mr-font-14);
      line-height: 20px;
      color: var(--mr-ink-muted);

      b {
        color: var(--mr-ink);
      }

      .mat-icon {
        width: 18px;
        height: 18px;
        font-size: var(--mr-font-18);
        color: var(--mr-ink);
      }
    }

    .vez {
      display: inline-flex;
      align-items: center;
      box-sizing: border-box;
      height: 20px;
      padding: 0 8px;
      border: 1.5px solid var(--mr-accent);
      border-radius: var(--mr-radius-pill);
      color: var(--mr-accent-text);
      font-size: var(--mr-font-13);
      font-weight: 700;
      line-height: 1;
    }

    // "os dois agem no mesmo turno" is for the desktop.
    .hint {
      @media (max-width: 767.98px) {
        display: none;
      }
    }

    // The master's list: the box reaches past the panel's padding, so its rows
    // keep the same columns as the rows outside it.
    :host(.bleed) {
      .box {
        margin: 8px -14px;
      }

      ::ng-deep .row {
        margin: 0;
        padding: 10px 14px 10px 12px;
      }

      // The state pill under the name: the order's column is narrow.
      ::ng-deep .row__name {
        display: grid;
        justify-items: start;
        gap: 2px;
      }

      // The dots hang 10px outside their box as in the rows outside; the box's own padding is 4px narrower than a row's,
      // so the dots and "Dano/Cura" line up with the rows outside (E8-01).
      ::ng-deep .row {
        padding-right: 12.5px;
      }

      ::ng-deep .row__more,
      ::ng-deep .row__spacer {
        margin-right: -10px;
      }
    }

    // The player's column and strip: tighter.
    :host(.dense) {
      .box {
        margin: 6px 0;
      }

      .head {
        padding: 8px 10px 0;
        font-size: var(--mr-font-13);
        line-height: 18px;
      }
    }
  `,
})
export class OrderGroup {
  readonly total = input.required<number>();
  readonly onTurn = input(false);
  /** A rider and the mount it rides: the head reads "Montado" and no hint (588-04). */
  readonly mounted = input(false);
  /** For a screen reader: "Turno conjunto: Brisa e Toren, iniciativa 19". */
  readonly label = input('');
  /** Who is in it, when the box names them ("Lobos atrozes da Sálvia"). */
  readonly names = input('');
  /** The line after the total when the group is not on turn (the master's desktop). */
  readonly hint = input('');
}
