import { Component, computed, effect, inject, signal, viewChild } from '@angular/core';
import { toSignal } from '@angular/core/rxjs-interop';
import { AbstractControl, FormControl, ValidationErrors, Validators } from '@angular/forms';
import { MatIconModule } from '@angular/material/icon';
import { startWith } from 'rxjs';

import { type XPAward, XPBlockedReason } from '../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../core/connect/idempotency';
import type { ExperienceRow } from '../../core/progression/experience-store';
import { ProgressionClient } from '../../core/progression/progression-client';
import { xpBlocked, xpErrorMessage } from '../../core/progression/xp-errors';
import { SheetFrame } from '../../pages/live-session/combat/sheet-frame/sheet-frame';
import { injectSheet } from '../../pages/live-session/combat/sheet-host';
import { XpActions } from './xp-actions';
import { XpReason } from './xp-reason';
import { type Recipient, XpRecipients } from './xp-recipients';

export interface MilestoneData {
  readonly campaignId: string;
  readonly campaignName: string;
  readonly rows: readonly ExperienceRow[];
}

const filled = (control: AbstractControl): ValidationErrors | null =>
  String(control.value ?? '').trim() ? null : { required: true };

/**
 * "Registrar marco" (E7-08, MR-016) in a campaign that levels by milestones:
 * what happened (up to 120 characters) and who gets the mark, everyone
 * checked at first. The live line says what it does ("3 personagens podem
 * subir de nível"), and there is no XP number anywhere, because there is no
 * XP to count. A dialog on a desktop, a bottom sheet on a phone, in the
 * shared `sheet-frame`.
 */
@Component({
  selector: 'app-milestone-sheet',
  imports: [MatIconModule, SheetFrame, XpActions, XpReason, XpRecipients],
  templateUrl: './milestone-sheet.html',
  styleUrl: './milestone-sheet.scss',
})
export class MilestoneSheet {
  private readonly api = inject(ProgressionClient);
  private readonly sheet = injectSheet<MilestoneData, XPAward | undefined>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly reason = new FormControl('', {
    nonNullable: true,
    validators: [filled, Validators.maxLength(120)],
  });
  private readonly reasonText = toSignal(this.reason.valueChanges.pipe(startWith('')), {
    initialValue: '',
  });

  protected readonly rows = signal<readonly ExperienceRow[]>(this.data.rows);
  protected readonly checked = signal<ReadonlySet<string>>(
    new Set(this.data.rows.map((r) => r.id)),
  );
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');

  private readonly reasonField = viewChild.required(XpReason);
  private readonly frame = viewChild.required(SheetFrame);

  protected readonly recipients = computed<Recipient[]>(() =>
    this.rows().map((r) => ({
      id: r.id,
      name: r.name,
      sub: r.sub,
      checked: this.checked().has(r.id),
      amount: '',
    })),
  );
  protected readonly note = 'O aviso some quando o nível sobe na ficha.';
  protected readonly effect = computed(() => {
    const n = this.checked().size;
    return n === 0
      ? ''
      : n === 1
        ? '1 personagem pode subir de nível'
        : `${n} personagens podem subir de nível`;
  });
  protected readonly reasonToWait = computed(() =>
    this.checked().size === 0 ? 'Marque pelo menos um personagem' : '',
  );
  protected readonly blocked = computed(
    () => this.reasonToWait() !== '' || this.reasonText().trim() === '',
  );

  private readonly key = new ActionKey();

  protected toggle(id: string): void {
    this.checked.update((set) => {
      const next = new Set(set);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
  }

  protected async register(): Promise<void> {
    if (this.busy()) {
      return;
    }
    if (this.reason.invalid) {
      this.reason.markAsTouched();
      this.reasonField().focus();
      return;
    }
    if (this.blocked()) {
      return;
    }
    const reason = this.reason.value.trim();
    const ids = this.rows()
      .filter((r) => this.checked().has(r.id))
      .map((r) => r.id);
    // New values are a new milestone; the same values again are a retry.
    const key = this.key.keyFor([reason, ids]);
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.markMilestone(this.data.campaignId, reason, ids, key);
      this.sheet.close(res.award);
    } catch (err) {
      this.error.set(xpErrorMessage(err, 'registrar o marco'));
      // A character that cannot receive (died, left) leaves the list, so the retry can go.
      const blocked = xpBlocked(err);
      if (
        blocked?.reason === XPBlockedReason.XP_BLOCKED_REASON_CHARACTER_NOT_ELIGIBLE &&
        blocked.characterId
      ) {
        this.rows.update((rows) => rows.filter((r) => r.id !== blocked.characterId));
        this.checked.update((set) => {
          const next = new Set(set);
          next.delete(blocked.characterId);
          return next;
        });
      }
      this.frame().scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }

  protected cancel(): void {
    this.sheet.close(undefined);
  }
}
