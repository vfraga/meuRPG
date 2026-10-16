import { Component } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import type { MatBottomSheet } from '@angular/material/bottom-sheet';
import type { MatDialog } from '@angular/material/dialog';
import type { Observable } from 'rxjs';

import { formatInt } from '../../core/format/text';
import type { SpellClass } from '../../core/spells/spells-client';
import type { SpellsState } from '../../core/spells/spells-state';
import { SheetFrame } from '../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../shared/sheet/sheet-host';
import { type MineCharacter, SpellFilters } from './spell-filters';

/** What the page hands the "Filtros" sheet: the page's own state, so the count follows the filters. */
export interface SpellFilterSheetData {
  readonly state: SpellsState;
  readonly classes: readonly SpellClass[];
  readonly mine: MineCharacter | null;
}

/** "Filtros": a bottom sheet on a phone, a dialog from a tablet up. */
export function openSpellFilterSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: SpellFilterSheetData,
): Observable<void | undefined> {
  return openSheet<SpellFilterSheet, SpellFilterSheetData, void>(
    dialog,
    bottomSheet,
    SpellFilterSheet,
    {
      data,
      ariaLabel: 'Filtros',
      labelledBy: 'spell-filters-t',
      width: '480px',
    },
  );
}

/**
 * The "Filtros" sheet of the phone (E10-11, state 3): class, circles, school and "Só as que posso aprender".
 * The body scrolls and the footer stays: "Limpar" and "Ver 73 magias", two equal buttons, the count
 * following the filters as the server answers; "Limpar" clears the filters, not the name on the page behind. A change applies at once (the page behind is the same
 * list), so "Ver" only closes.
 */
@Component({
  selector: 'app-spell-filter-sheet',
  imports: [MatButtonModule, SheetFrame, SpellFilters],
  template: `
    <app-sheet-frame title="Filtros" titleId="spell-filters-t" [phone]="sheet.inSheet" (closed)="sheet.close()">
      <app-spell-filters [state]="data.state" [classes]="data.classes" [mine]="data.mine" />
      <div foot>
        <div class="foot">
          <button matButton="outlined" type="button" (click)="data.state.clearFilters()">Limpar</button>
          <button matButton="filled" type="button" (click)="sheet.close()">{{ seeLabel() }}</button>
        </div>
      </div>
    </app-sheet-frame>
  `,
  // Two equal buttons side by side at any width (E10-11: 320 px too): the labels are short and never wrap.
  styles: `
    .foot {
      display: flex;
      gap: var(--mr-space-3);

      button {
        flex: 1 1 0;
        box-sizing: border-box;
        min-width: 0;
        min-height: 48px;
        padding-inline: 8px;
        white-space: nowrap;
      }
    }

    @media (min-width: 768px) {
      .foot {
        justify-content: flex-end;

        button {
          flex: 0 0 176px;
          min-height: 44px;
        }
      }
    }
  `,
})
export class SpellFilterSheet {
  protected readonly sheet = injectSheet<SpellFilterSheetData, void>();
  protected readonly data = this.sheet.data;

  protected seeLabel(): string {
    const n = this.data.state.total();
    return `Ver ${formatInt(n)} ${n === 1 ? 'magia' : 'magias'}`;
  }
}
