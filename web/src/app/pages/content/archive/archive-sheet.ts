import { Component } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { usageSentence } from '../../../core/content/content-kinds';
import { PairFoot } from '../../../shared/pair-foot/pair-foot';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';

export interface ArchiveSheetData {
  readonly name: string;
  /** How many characters of the campaign use the entry. */
  readonly using: number;
}

/**
 * "Arquivar Guardião do Vale?" on a phone (E10-01 state 10): a bottom sheet with the warning, how many sheets use the entry
 * and the fixed footer, "Voltar" and "Arquivar", the same width. The focus starts on the title; Esc and the X close it.
 * Nothing is deleted: the sheets that use the entry keep it, it is only not offered as a new choice (RN-23).
 */
@Component({
  selector: 'app-archive-sheet',
  imports: [MatIconModule, PairFoot, SheetFrame],
  template: `
    <app-sheet-frame [title]="'Arquivar ' + sheet.data.name + '?'" titleId="archive-t" [phone]="sheet.inSheet" (closed)="sheet.close(false)">
      <div class="mr-notice mr-notice--warning">
        <mat-icon aria-hidden="true">warning</mat-icon>
        <p>As fichas que usam {{ sheet.data.name }} continuam funcionando. A entrada só deixa de aparecer para fichas novas.</p>
      </div>
      <p class="using">{{ using }}</p>
      <div foot>
        <app-pair-foot cancelLabel="Voltar" confirmLabel="Arquivar" confirmIcon="archive" (cancel)="sheet.close(false)" (confirm)="sheet.close(true)" />
      </div>
    </app-sheet-frame>
  `,
  styles: `
    .using {
      margin: var(--mr-space-3) 0 0;
      color: var(--mr-ink-muted);
    }
  `,
})
export class ArchiveSheet {
  protected readonly sheet = injectSheet<ArchiveSheetData, boolean>();
  protected readonly using = usageSentence(this.sheet.data.name, this.sheet.data.using);
}
