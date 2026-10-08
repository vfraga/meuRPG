import { Component, computed, effect, inject, signal, viewChild } from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';

import type { Milestone } from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import type { ExperienceRow } from '../../../core/progression/experience-store';
import { reachedEffect } from '../../../core/progression/milestones';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { xpErrorMessage } from '../../../core/progression/xp-errors';
import { SheetFrame } from '../../live-session/combat/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../live-session/combat/sheet-host';
import { XpActions } from '../../../shared/xp/xp-actions';
import { type Recipient, XpRecipients } from '../../../shared/xp/xp-recipients';

/** What the sheet needs: the milestone, and the characters it may go to (all
 * the living player characters for "Marcar como alcançado", only those that
 * do not have it yet for "Dar a mais alguém"). */
export interface MarkReachedData {
  readonly campaignId: string;
  readonly milestoneId: string;
  readonly text: string;
  readonly rows: readonly ExperienceRow[];
  readonly give: boolean;
}

/** What the master did: the milestone as it is now, who got the mark, and who
 * was left out. */
export interface MarkReachedResult {
  readonly milestone: Milestone | undefined;
  readonly marked: readonly string[];
  readonly left: readonly string[];
}

/** Opens the sheet: a dialog on a desktop, a bottom sheet on a phone. It
 * answers the result, or `undefined` when the master cancelled. */
export function openMarkReached(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: MarkReachedData,
) {
  return openSheet<MarkReachedSheet, MarkReachedData, MarkReachedResult | undefined>(
    dialog,
    bottomSheet,
    MarkReachedSheet,
    {
      data,
      // The phone's sheet has no aria-labelledby: its label is the title.
      ariaLabel: data.give
        ? `Dar “${data.text}” a mais alguém`
        : `Marcar “${data.text}” como alcançado`,
      labelledBy: 'sheet-t',
      width: '560px',
    },
  );
}

/**
 * "Marcar “X” como alcançado" and "Dar “X” a mais alguém" (E8-14, MR-016): the
 * checklist of who levels up, everyone checked at first so the common case is
 * one press. The live line says what it does ("2 personagens podem subir de
 * nível", with the flag and the words), and the one filled button turns
 * dashed, with its reason, while nobody is checked. In the shared
 * `sheet-frame`: a dialog on a desktop, a bottom sheet on a phone, the footer
 * always in reach (320×568 included). One key per set of people, so a retry
 * after a lost answer never marks twice.
 */
@Component({
  selector: 'app-mark-reached-sheet',
  imports: [MatIconModule, SheetFrame, XpActions, XpRecipients],
  templateUrl: './mark-reached-sheet.html',
  styleUrl: './mark-reached-sheet.scss',
})
export class MarkReachedSheet {
  private readonly api = inject(ProgressionClient);
  private readonly sheet = injectSheet<MarkReachedData, MarkReachedResult | undefined>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;

  protected readonly checked = signal<ReadonlySet<string>>(
    new Set(this.data.rows.map((r) => r.id)),
  );
  protected readonly busy = signal(false);
  /** A request in the air: Esc and the backdrop do not close the sheet under it. */
  protected readonly lockWhileBusy = effect(() => this.sheet.lock(this.busy()));
  protected readonly error = signal('');
  private readonly frame = viewChild.required(SheetFrame);

  protected readonly title = this.data.give
    ? `Dar “${this.data.text}” a mais alguém`
    : `Marcar “${this.data.text}” como alcançado`;
  protected readonly intro = this.data.give
    ? 'Marque quem ficou de fora. Isto não cria outro marco.'
    : 'O grupo cumpriu este marco. Marque quem sobe de nível.';
  protected readonly groupLabel = this.data.give ? 'Quem recebe o marco' : 'Quem sobe de nível';
  protected readonly primaryLabel = this.data.give ? 'Dar o marco' : 'Marcar como alcançado';
  protected readonly note = this.data.give
    ? 'A marca “Pode subir de nível” some quando o jogador sobe o nível na ficha.'
    : 'A marca “Pode subir de nível” some quando o jogador sobe o nível na ficha. Quem ficou de fora pode receber depois.';

  protected readonly recipients = computed<Recipient[]>(() =>
    this.data.rows.map((r) => ({
      id: r.id,
      name: r.name,
      sub: r.sub,
      checked: this.checked().has(r.id),
      amount: '',
    })),
  );
  protected readonly effect = computed(() => reachedEffect(this.checked().size));
  protected readonly reasonToWait = computed(() =>
    this.checked().size === 0 ? 'Marque pelo menos um personagem' : '',
  );
  protected readonly blocked = computed(() => this.reasonToWait() !== '');

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

  protected async confirm(): Promise<void> {
    if (this.busy() || this.blocked()) {
      return;
    }
    const chosen = this.data.rows.filter((r) => this.checked().has(r.id));
    const ids = chosen.map((r) => r.id);
    // The same people again are a retry; other people are another request.
    const key = this.key.keyFor([this.data.give, ids]);
    this.busy.set(true);
    this.error.set('');
    try {
      const { campaignId, milestoneId, give } = this.data;
      const res = give
        ? await this.api.giveMilestoneTo(campaignId, milestoneId, ids, key)
        : await this.api.markMilestoneReached(campaignId, milestoneId, ids, key);
      this.sheet.close({
        milestone: res.milestone,
        marked: chosen.map((r) => r.name),
        left: this.data.rows.filter((r) => !this.checked().has(r.id)).map((r) => r.name),
      });
    } catch (err) {
      this.error.set(xpErrorMessage(err, this.data.give ? 'dar o marco' : 'marcar o marco'));
      this.frame().scrollToTop();
    } finally {
      this.busy.set(false);
    }
  }

  protected cancel(): void {
    this.sheet.close(undefined);
  }
}
