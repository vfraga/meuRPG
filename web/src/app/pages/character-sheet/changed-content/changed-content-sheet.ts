import { Component, inject } from '@angular/core';
import {
  MatBottomSheet,
  MAT_BOTTOM_SHEET_DATA,
  MatBottomSheetRef,
} from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MAT_DIALOG_DATA, MatDialog, MatDialogRef } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import type { ChangedContentVm } from '../character-sheet.types';
import { changeSheetTitle, tieAcChange, whoLines } from './changed-content-format';

export interface ChangedContentData {
  readonly change: ChangedContentVm;
  /** The viewer can edit the sheet (the master, or the owner while it is not locked): they fix it. Otherwise the master does. */
  readonly canEdit: boolean;
  /** The skills the sheet is proficient in, by name: said under a sentence about the skills, as drawn. */
  readonly skills?: readonly string[];
}

/**
 * "O que mudou" (E10-02 state 8): the sentences of one changed table entry, as the server wrote
 * them, who fixes it and when the notice goes. A bottom sheet on a phone and a dialog from a
 * tablet up, like the spell's "?". "Fechar" is an outlined button: there is nothing to confirm.
 */
@Component({
  selector: 'app-changed-content-sheet',
  imports: [MatButtonModule, MatIconModule],
  template: `
    <div class="sheet" [class.sheet--phone]="inSheet">
      @if (inSheet) {
        <span class="sheet__handle" aria-hidden="true"></span>
      }
      <header class="sheet__head">
        <h2 class="sheet__title" id="changed-title">{{ title }}</h2>
        <button type="button" class="sheet__close" aria-label="Fechar" (click)="close()">
          <mat-icon aria-hidden="true">close</mat-icon>
        </button>
      </header>
      <div class="sheet__body" role="region" aria-labelledby="changed-title" tabindex="0">
        <div class="mr-notice mr-notice--warning">
          <mat-icon aria-hidden="true">warning</mat-icon>
          <ul class="sheet__list">
            @for (m of data.change.messages; track $index) {
              <li>{{ tie(m) }}</li>
            }
            @if (skillsLine) {
              <li class="sheet__skills">{{ skillsLine }}</li>
            }
          </ul>
        </div>
        @for (w of who; track $index) {
          <p class="sheet__who">
            {{ w }}
          </p>
        }
      </div>
      <div class="sheet__foot">
        <button matButton="outlined" type="button" class="sheet__action" (click)="close()">Fechar</button>
      </div>
    </div>
  `,
  styles: `
    :host {
      display: block;
    }

    .sheet {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
      box-sizing: border-box;
      max-height: calc(92dvh - 48px);
      padding: var(--mr-space-5) var(--mr-space-6) var(--mr-space-4);

      &--phone {
        max-height: calc(100dvh - 40px);
        padding: var(--mr-space-2) 2px;
      }
    }

    .sheet__handle {
      align-self: center;
      width: 36px;
      height: 4px;
      border-radius: var(--mr-radius-pill);
      background: var(--mr-control-line);
    }

    .sheet__head {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: var(--mr-space-3);
    }

    .sheet__title {
      margin: 0;
      font-family: var(--mr-font-display);
      font-size: var(--mr-font-26);
      font-weight: 700;
      line-height: 30px;
      overflow-wrap: anywhere;
    }

    .sheet__close {
      display: flex;
      flex: none;
      align-items: center;
      justify-content: center;
      width: 44px;
      height: 44px;
      padding: 0;
      border: 0;
      border-radius: 50%;
      background: none;
      color: var(--mr-ink);
      cursor: pointer;

      &:focus-visible {
        outline: 2px solid var(--mr-focus);
        outline-offset: -4px;
      }
    }

    // The sentences scroll inside; the title and "Fechar" stay in view.
    .sheet__body {
      display: flex;
      flex: 1 1 auto;
      flex-direction: column;
      gap: var(--mr-space-3);
      min-height: 0;
      overflow-y: auto;
    }

    .sheet__list {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-2);
      margin: 0;
      padding: 0;
      list-style: none;
    }

    .sheet__who {
      margin: 0;
      color: var(--mr-ink-muted);
    }

    .sheet__foot {
      padding-top: 10px;
      border-top: 1px solid var(--mr-rule);
      background: var(--mr-surface);
    }

    .sheet__action {
      width: 100%;
    }
  `,
})
export class ChangedContentSheet {
  protected readonly data =
    inject<ChangedContentData | null>(MAT_DIALOG_DATA, { optional: true }) ??
    inject<ChangedContentData>(MAT_BOTTOM_SHEET_DATA);
  private readonly dialogRef = inject<MatDialogRef<ChangedContentSheet>>(MatDialogRef, {
    optional: true,
  });
  private readonly sheetRef = inject<MatBottomSheetRef<ChangedContentSheet>>(MatBottomSheetRef, {
    optional: true,
  });

  protected readonly tie = tieAcChange;
  protected readonly inSheet = this.sheetRef !== null;
  protected readonly title = changeSheetTitle(this.data.change);
  /** Who adjusts and when the notice goes, for each kind of line: a shield line has its own sentences (the owner stows the gear), any other is about the numbers. */
  protected readonly who = whoLines(this.data.change, this.data.canEdit);
  /** "Perícias da ficha: Atletismo, Natureza.", under a sentence that talks about the skills. */
  protected readonly skillsLine =
    (this.data.skills?.length ?? 0) > 0 && this.data.change.messages.some((m) => /perícia/i.test(m))
      ? `Perícias da ficha: ${new Intl.ListFormat('pt-BR', { type: 'conjunction' }).format(this.data.skills!)}.`
      : '';

  protected close(): void {
    this.dialogRef?.close();
    this.sheetRef?.dismiss();
  }
}

/** Opens "O que mudou": a bottom sheet on a phone, a 560 px dialog from a tablet up. Focus returns to the button. */
export function openChangedContent(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: ChangedContentData,
): void {
  const phone = typeof matchMedia === 'function' && matchMedia(PHONE_QUERY).matches;
  if (phone) {
    bottomSheet.open(ChangedContentSheet, {
      data,
      ariaLabel: `O que mudou: ${data.change.namePt}`,
      autoFocus: 'first-heading',
      panelClass: 'mr-sheet',
    });
  } else {
    dialog.open(ChangedContentSheet, {
      data,
      width: '560px',
      maxWidth: 'calc(100vw - 32px)',
      ariaLabelledBy: 'changed-title',
      autoFocus: 'first-heading',
    });
  }
}
