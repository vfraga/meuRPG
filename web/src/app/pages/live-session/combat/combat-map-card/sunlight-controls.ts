import { ChangeDetectionStrategy, Component, computed, input, model, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { SunlightScope } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { type PaintTool, paintStatus } from '../../../../core/combat/sunlight-paint';

/**
 * The master's sunlight controls over the map (#704), shown only while the sun is on and the combat has a map: a label "Luz do sol"
 * and one two-part segmented control, "Todo o mapa" | "Onde eu pintar" (a radio group, arrows move the choice), and, for the
 * second, "Pintar", which opens paint mode: a tinted card with the count, the two gestures (one finger paints, two scroll) and one
 * row with the tool "Pintar" | "Apagar" and "Pronto". The map card draws the squares; this only asks. Every control is 44px tall.
 */
@Component({
  selector: 'app-sunlight-controls',
  imports: [MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    @if (!painting()) {
      <div class="sc__row">
        <span class="sc__label" id="sc-scope-label"><mat-icon aria-hidden="true">wb_sunny</mat-icon>Luz do sol</span>
        <div class="sc__seg sc__seg--scope" role="radiogroup" aria-label="Onde a luz do sol vale">
          <button type="button" role="radio" class="sc__opt" [attr.aria-checked]="!squares()" [attr.tabindex]="squares() ? -1 : 0" [class.sc__opt--on]="!squares()" (click)="scopeChange.emit(map)" (keydown)="arrows($event)">
            @if (!squares()) {<mat-icon aria-hidden="true">check</mat-icon>}Todo o mapa
          </button>
          <button type="button" role="radio" class="sc__opt" [attr.aria-checked]="squares()" [attr.tabindex]="squares() ? 0 : -1" [class.sc__opt--on]="squares()" (click)="scopeChange.emit(spots)" (keydown)="arrows($event)">
            @if (squares()) {<mat-icon aria-hidden="true">check</mat-icon>}Onde eu pintar
          </button>
        </div>
        @if (squares()) {
          <button type="button" class="sc__btn sc__start" aria-label="Pintar luz do sol no mapa" (click)="painting.set(true)">
            <mat-icon aria-hidden="true">brush</mat-icon>Pintar
          </button>
        }
      </div>
    } @else {
      <div class="sc__card">
        <p class="sc__status" role="status">{{ status() }}</p>
        <ul class="sc__hints">
          <li>Um dedo: arrasta e pinta</li>
          <li>Dois dedos: rola o mapa</li>
        </ul>
        <div class="sc__row sc__row--paint">
          <div class="sc__seg sc__seg--tools" role="group" aria-label="Ferramenta de pintura">
            <button type="button" class="sc__opt" [class.sc__opt--on]="tool() === 'paint'" [attr.aria-pressed]="tool() === 'paint'" aria-label="Pintar quadrados de luz do sol" (click)="tool.set('paint')">
              <mat-icon aria-hidden="true">brush</mat-icon>Pintar
            </button>
            <button type="button" class="sc__opt" [class.sc__opt--on]="tool() === 'erase'" [attr.aria-pressed]="tool() === 'erase'" aria-label="Apagar quadrados de luz do sol" (click)="tool.set('erase')">
              <mat-icon aria-hidden="true">ink_eraser</mat-icon>Apagar
            </button>
          </div>
          <button type="button" class="sc__btn sc__done" aria-label="Pronto: sair da pintura da luz do sol" (click)="painting.set(false)">
            <mat-icon aria-hidden="true">check</mat-icon>Pronto
          </button>
        </div>
      </div>
    }
  `,
  styles: `
    :host {
      display: block;
      container-type: inline-size;
    }
    // A card too narrow for the label, the two equal parts and "Pintar" on one row (the map column at 1280 px) puts the label on its own
    // line, so the parts and "Pintar" still share a row.
    @container (max-width: 520px) {
      .sc__row:not(.sc__row--paint) .sc__label {
        flex: 1 0 100%;
        min-height: 32px;
      }
    }
    .sc__row {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      gap: var(--mr-space-2) var(--mr-space-3);
    }
    .sc__row--paint {
      flex-wrap: nowrap;
    }
    .sc__label {
      display: inline-flex;
      align-items: center;
      gap: var(--mr-space-1);
      min-height: 44px;
      font-weight: 700;
      color: var(--mr-ink);
    }
    // The two-part segmented controls: equal parts of one height, the chosen one filled (and, for the scope, ticked), so the choice
    // never rests on colour alone.
    .sc__seg {
      display: flex;
      flex: 1 1 auto;
      max-width: 420px;
      box-sizing: border-box;
      min-height: 44px;
      overflow: hidden;
      // The edge is an inset ring, not a border, so the segments are exactly as tall as the 44px buttons beside them.
      box-shadow: inset 0 0 0 1px var(--mr-control-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);
    }
    .sc__seg--scope {
      // Both labels stay on one line: the control is at least as wide as its two labels, and the row wraps before it shrinks.
      flex: 0 1 auto;
      // The two parts are one width, the widest label's (the check sits inside it): the columns share the group equally.
      display: inline-grid;
      grid-auto-flow: column;
      grid-auto-columns: 1fr;
    }
    .sc__seg--tools {
      flex: 1 1 0;
      min-width: 0;
    }
    .sc__opt {
      display: inline-flex;
      flex: 1 1 auto;
      white-space: nowrap;
      align-items: center;
      justify-content: center;
      gap: var(--mr-space-1);
      box-sizing: border-box;
      min-width: 0;
      min-height: 44px;
      padding: 0 var(--mr-space-3);
      border: 0;
      background: transparent;
      color: var(--mr-ink);
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }
    .sc__opt + .sc__opt {
      box-shadow: inset 1px 0 0 var(--mr-control-line);
    }
    .sc__opt--on {
      background: var(--mr-accent);
      color: var(--mr-on-accent);
    }
    .sc__opt:focus-visible,
    .sc__btn:focus-visible {
      outline: 3px solid var(--mr-focus, currentColor);
      outline-offset: -3px;
    }
    .sc__btn {
      display: inline-flex;
      white-space: nowrap;
      align-items: center;
      justify-content: center;
      gap: var(--mr-space-1);
      box-sizing: border-box;
      min-width: 44px;
      min-height: 44px;
      padding: 0 var(--mr-space-4);
      border: 1px solid var(--mr-control-line);
      border-radius: var(--mr-radius-md);
      background: var(--mr-surface);
      color: var(--mr-ink);
      font: inherit;
      font-weight: 700;
      cursor: pointer;
    }
    .sc__done {
      flex: none;
    }
    // Painting: a tinted card with the count, the two gestures and, on one row of 44px, the tool and "Pronto".
    .sc__card {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
      padding: var(--mr-space-3);
      border: 1px solid var(--mr-accent);
      border-radius: var(--mr-radius-lg);
      background: var(--mr-accent-soft);
    }
    .sc__status {
      margin: 0;
      font-weight: 700;
      color: var(--mr-ink);
    }
    .sc__hints {
      margin: 0;
      padding: 0;
      list-style: none;
      font-size: var(--mr-font-14);
      color: var(--mr-ink-muted);
    }
  `,
})
export class SunlightControls {
  readonly scope = input.required<SunlightScope>();
  /** Squares lit now (the server's), for the status line. */
  readonly count = input(0);
  readonly painting = model(false);
  readonly tool = model<PaintTool>('paint');
  /** "Todo o mapa" or "Onde eu pintar" was picked. */
  readonly scopeChange = output<SunlightScope>();

  protected readonly map = SunlightScope.MAP;
  protected readonly spots = SunlightScope.SQUARES;
  protected readonly squares = computed(() => this.scope() === SunlightScope.SQUARES);
  /** Arrow keys move the choice between the two segments (radio group), as the native radio does. */
  protected arrows(event: KeyboardEvent): void {
    const next = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[event.key];
    if (next === undefined) {
      return;
    }
    event.preventDefault();
    const to = (this.squares() ? 1 : 0) + next;
    const radios = Array.from(
      (event.currentTarget as HTMLElement).parentElement!.querySelectorAll<HTMLElement>(
        '[role="radio"]',
      ),
    );
    const target = radios[(to + radios.length) % radios.length];
    target.focus();
    target.click();
  }

  protected readonly status = computed(() => paintStatus(this.count()));
}
