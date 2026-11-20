import { Component, inject, input } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { ChangedContentVm } from '../character-sheet.types';
import {
  changeAnnouncement,
  changeIntro,
  changeTitle,
  tieAcChange,
} from './changed-content-format';
import { openChangedContent } from './changed-content-sheet';

/**
 * "A classe mudou" (MR-025, RN-23, question 80, E10-02 state 8): the notice under the sheet's header
 * when the master changed a table entry the sheet uses and the sheet now has an issue because of
 * it. The numbers above already use the new rules; this says what no longer fits, in the server's
 * own sentences, and never blocks another edit. The notice goes by itself when the numbers match
 * again: the server works it out on every read. Only the sheet's owner and the master read it;
 * never who changed it nor the history of the edits. One notice per changed entry, the same for a
 * race, a background, a subclass or a spell.
 */
@Component({
  selector: 'app-changed-content',
  imports: [MatButtonModule, MatIconModule],
  template: `
    @for (c of changes(); track c.key) {
      <section class="mr-notice mr-notice--warning change" [attr.aria-label]="title(c)">
        <mat-icon aria-hidden="true">warning</mat-icon>
        <div class="change__body">
          <p class="mr-visually-hidden" role="status">{{ announcement(c) }}</p>
          <p class="change__text">
            <strong>{{ title(c) }}</strong> {{ intro(c) }}
          </p>
          <ul class="change__list">
            @for (m of c.messages; track $index) {
              <li>{{ tie(m) }}</li>
            }
          </ul>
          <button matButton="outlined" type="button" class="change__open" (click)="open(c)">Ver o que mudou</button>
        </div>
      </section>
    }
  `,
  styles: `
    :host {
      display: contents;
    }

    .change__body {
      display: flex;
      flex-direction: column;
      align-items: flex-start;
      gap: var(--mr-space-2);
      min-width: 0;
    }

    .change__text,
    .change__list {
      margin: 0;
    }

    .change__list {
      padding-left: 20px;
    }

    .change__open {
      width: 100%;

      @media (min-width: 600px) {
        width: auto;
      }
    }
  `,
})
export class ChangedContentNotice {
  private readonly dialog = inject(MatDialog);
  private readonly bottomSheet = inject(MatBottomSheet);

  readonly changes = input.required<readonly ChangedContentVm[]>();
  /** The viewer can edit the sheet: "Quem ajusta: você". */
  readonly canEdit = input(false);
  /** The sheet's skills, by name, for the line under a sentence about them. */
  readonly skills = input<readonly string[]>([]);

  protected readonly title = changeTitle;
  protected readonly intro = changeIntro;
  protected readonly announcement = changeAnnouncement;
  protected readonly tie = tieAcChange;

  protected open(change: ChangedContentVm): void {
    openChangedContent(this.dialog, this.bottomSheet, {
      change,
      canEdit: this.canEdit(),
      skills: this.skills(),
    });
  }
}
