import { ChangeDetectionStrategy, Component, effect, signal } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { ActionKey } from '../../../core/connect/idempotency';
import type { FavorRecipient } from '../../../core/play/favor-recipients';
import {
  FAVOR_PASS_NOTE,
  FAVOR_WORK_LABEL,
  favorPassSubtitle,
} from '../../../core/play/favor-pass';
import type { FavorState } from '../../../core/play/favor-state';
import { PairFoot } from '../../../shared/pair-foot/pair-foot';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet } from '../../../shared/sheet/sheet-host';
import { type PickRow, ResourcePick } from '../../../shared/resource-pick/resource-pick';

/** What the sheet's panel hands the dialog. */
export interface FavorPassData {
  readonly favor: FavorState;
  readonly fromId: string;
  readonly fromName: string;
  readonly recipients: readonly FavorRecipient[];
}

/**
 * "Passar a Inspiração" (RN-43, board 581-04): the holder picks another player character and asks the master, who accepts or refuses.
 * Nothing moves until he answers, and the dialog says so. The first character is chosen, as the board has it; "Pedir ao mestre" reads
 * "Pedindo ao mestre…" while the call is on its way and "Cancelar" is dashed with it. A refusal stays in the dialog, in an alert; a
 * successful ask closes it with `true`. A bottom sheet on a phone and a dialog from a tablet up.
 */
@Component({
  selector: 'app-favor-pass-sheet',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatIconModule, PairFoot, ResourcePick, SheetFrame],
  template: `
    <app-sheet-frame
      title="Passar a Inspiração"
      titleId="favor-pass-t"
      icon="keyboard_tab"
      [subtitle]="subtitle"
      [small]="true"
      [phone]="inSheet"
      (closed)="close()"
    >
      <div class="body">
        <app-resource-pick label="Para quem" [rows]="rows" [chosen]="target()" (pick)="choose($event)" />
        <p class="note">{{ note }}</p>
        @if (error()) {
          <div class="mr-notice mr-notice--danger" role="alert">
            <mat-icon aria-hidden="true">error</mat-icon>
            <p>{{ error() }}</p>
          </div>
        }
      </div>
      <app-pair-foot
        foot
        [confirmLabel]="busy() ? working : 'Pedir ao mestre'"
        confirmIcon="keyboard_tab"
        [ready]="target() !== '' && !busy()"
        [cancelOff]="busy()"
        (cancel)="close()"
        (confirm)="send()"
      />
    </app-sheet-frame>
  `,
  styles: `
    :host {
      display: block;
    }

    .body {
      display: flex;
      flex-direction: column;
      gap: var(--mr-space-3);
    }

    .note {
      margin: 0;
      font-size: var(--mr-font-14);
      line-height: 19px;
      color: var(--mr-ink-muted);
    }
  `,
})
export class FavorPassSheet {
  private readonly sheet = injectSheet<FavorPassData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly subtitle = favorPassSubtitle(this.data.fromName);
  protected readonly note = FAVOR_PASS_NOTE;
  protected readonly working = FAVOR_WORK_LABEL.ask;
  protected readonly rows: readonly PickRow[] = this.data.recipients.map((r) => ({
    id: r.characterId,
    title: r.name,
    sub: r.detail,
    blocked: '',
  }));
  protected readonly target = signal(this.data.recipients[0]?.characterId ?? '');
  protected readonly error = signal('');
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the dialog under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  private readonly key = new ActionKey();

  protected choose(id: string): void {
    this.target.set(id);
    this.error.set('');
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(false);
    }
  }

  protected async send(): Promise<void> {
    const to = this.target();
    if (this.busy() || to === '') {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    // The key is made once for each choice, so a repeated tap or a retry after a lost answer asks once.
    const failure = await this.data.favor.ask(
      this.data.fromId,
      to,
      this.key.keyFor({ from: this.data.fromId, to }),
    );
    this.busy.set(false);
    if (failure === null) {
      this.key.renew();
      this.sheet.close(true);
    } else {
      this.error.set(failure);
    }
  }
}
