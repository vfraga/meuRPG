import { Component, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { ChangeRowsList } from '../change-rows/change-rows';
import { LevelUpSession } from '../level-up-session';

/**
 * Step "Resumo" of the guided level-up (MR-040, E8-15): every change of the level, before →
 * after, as the server derived it; the line that the rest of the sheet does not change and
 * stays locked; and, quietly, what the master adds in the sheet editor (the features whose
 * choice the flow does not cover). "Confirmar o nível N" lives in the page's footer, the
 * screen's one filled button.
 */
@Component({
  selector: 'app-summary-step',
  imports: [ChangeRowsList, MatIconModule],
  template: `
    <section class="mr-panel" aria-labelledby="summary-title">
      <h2 class="mr-panel__title" id="summary-title">O que muda</h2>
      <p class="lead">Confira antes de confirmar. Depois, só o mestre muda a ficha.</p>
      @if (s().draft.featSummary(); as feat) {
        <p class="feat">
          <strong>Talento: {{ feat.name }}</strong>
          @if (feat.increase) {
            <span>{{ feat.increase }}</span>
          }
        </p>
      }
      <app-change-rows [rows]="s().rows()" />
    </section>
    <p class="rest">
      <mat-icon aria-hidden="true">lock</mat-icon>
      <span>O resto da ficha não muda e continua travado.</span>
    </p>
    @if (s().masterAdds()) {
      <p class="rest rest--quiet">
        <mat-icon aria-hidden="true">edit_note</mat-icon>
        <span>O mestre acrescenta pelo editor: {{ s().masterAdds() }}.</span>
      </p>
    }
  `,
  styles: `
    :host {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-4);
    }

    .lead {
      margin: 0 0 var(--mr-space-2);
      font-size: var(--mr-font-15);
      line-height: 20px;
      color: var(--mr-ink-muted);
    }

    .feat {
      display: flex;
      flex-wrap: wrap;
      gap: var(--mr-space-1) var(--mr-space-3);
      margin: 0 0 var(--mr-space-3);
      font-size: var(--mr-font-17);
    }

    .rest {
      display: flex;
      align-items: flex-start;
      gap: var(--mr-space-3);
      margin: 0;
      font-size: var(--mr-font-15);
      line-height: 20px;

      &--quiet {
        color: var(--mr-ink-muted);
      }

      .mat-icon {
        flex: none;
        width: 20px;
        height: 20px;
        font-size: var(--mr-font-20);
      }
    }
  `,
})
export class SummaryStep {
  readonly s = input.required<LevelUpSession>();
}
