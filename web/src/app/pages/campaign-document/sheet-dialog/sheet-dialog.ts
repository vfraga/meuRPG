import { Component, OnInit, computed, inject, input, output, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { RouterLink } from '@angular/router';

import { DocDialog } from '../doc-dialog/doc-dialog';
import { DocumentLinks, type SheetView } from '../../../shared/campaign-document/document-clients';
import { type LookupState, lookupFailure } from '../document-copy';

/**
 * The sheet link's dialog: the character's name, class and level and race,
 * and "Abrir ficha". The details come from `GetCharacter`, so a sheet
 * deleted since the link was written (or one the master may not read) shows
 * "Esta ficha foi apagada."
 */
@Component({
  selector: 'app-document-sheet-dialog',
  imports: [DocDialog, MatButtonModule, MatIconModule, RouterLink],
  templateUrl: './sheet-dialog.html',
  styleUrl: './sheet-dialog.scss',
})
export class DocumentSheetDialog implements OnInit {
  private readonly links = inject(DocumentLinks);

  readonly campaignId = input.required<string>();
  readonly characterId = input.required<string>();
  readonly text = input('');
  readonly closed = output<void>();

  protected readonly state = signal<LookupState<SheetView>>({ status: 'loading' });
  protected readonly summary = computed(() => {
    const s = this.state();
    if (s.status !== 'ready') {
      return '';
    }
    return [s.value.classSummary, s.value.raceName].filter((part) => part !== '').join(', ');
  });

  ngOnInit(): void {
    this.load();
  }

  protected load(): void {
    this.state.set({ status: 'loading' });
    this.links.getCharacter(this.campaignId(), this.characterId()).then(
      (value) => this.state.set({ status: 'ready', value }),
      (err: unknown) => this.state.set(lookupFailure(err)),
    );
  }
}
