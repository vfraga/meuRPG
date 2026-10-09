import { Component, input } from '@angular/core';

/** One row of the table: the character and one text per column. */
export interface HighlightsTableRow {
  readonly id: string;
  readonly name: string;
  readonly cells: readonly string[];
  /** The numbers read as "nothing to say" (muted), for a row with no tries. */
  readonly muted?: boolean;
}

let nextId = 0;

/**
 * The master's per-player table (MR-032, RN-20): every number, zeros included,
 * so the master can check what the tiles say. A table of columns on a desktop;
 * on a phone each player is a block with the numbers in two columns, each with
 * its label. The combat's "Números de cada jogador" (five columns) and the
 * session's "Testes passados fora do combate" (one) are the same table. The
 * players never get it: the server sends them one row at most, and the screens
 * draw no table for them.
 */
@Component({
  selector: 'app-highlights-table',
  template: `
    <div class="tbl" [class.tbl--spread]="layout() === 'spread'" [style.--cols]="columns().length">
      <h3 class="tbl__title" [id]="id + '-title'">{{ title() }}</h3>
      @if (caption()) {
        <p class="tbl__caption">{{ caption() }}</p>
      }
      <div role="table" [attr.aria-labelledby]="id + '-title'">
        <div class="tbl__row tbl__row--head" role="row">
          <span role="columnheader">{{ nameHeader() }}</span>
          @for (c of columns(); track c) {
            <span role="columnheader">{{ c }}</span>
          }
        </div>
        @for (r of rows(); track r.id) {
          <div class="tbl__row" role="row">
            <b role="rowheader">{{ r.name }}</b>
            @for (c of columns(); track c; let i = $index) {
              <span role="cell"><span class="tbl__lbl" aria-hidden="true">{{ c }}</span><span class="tbl__num" [class.tbl__num--muted]="r.muted">{{ r.cells[i] }}</span></span>
            }
          </div>
        }
      </div>
    </div>
  `,
  styleUrl: './highlights-table.scss',
})
export class HighlightsTable {
  readonly title = input.required<string>();
  readonly columns = input.required<readonly string[]>();
  readonly rows = input.required<readonly HighlightsTableRow[]>();
  readonly nameHeader = input('Personagem');
  /** On a phone: `pairs` puts the numbers in two columns, each beside its label; `spread` puts them in one
   * row (wrapping when it must), each label above its number. */
  readonly layout = input<'pairs' | 'spread'>('pairs');
  /** A line under the title that says what the numbers count. */
  readonly caption = input('');

  protected readonly id = `tbl-${nextId++}`;
}
