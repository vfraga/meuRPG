import {
  Component,
  ElementRef,
  Injector,
  afterNextRender,
  computed,
  inject,
  input,
  output,
  signal,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  XPBlockedReason,
  type Milestone,
} from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { MILESTONES_LIMIT, plannedCount } from '../../../core/progression/milestones';
import { MilestonesStore } from '../../../core/progression/milestones-store';
import { ProgressionClient } from '../../../core/progression/progression-client';
import {
  milestoneHasHistoryText,
  xpBlocked,
  xpErrorMessage,
} from '../../../core/progression/xp-errors';
import { MilestoneAsk } from './milestone-ask';
import { MilestoneNameForm } from './milestone-name-form';

/**
 * "Marcos planejados" (E8-14, MR-016, question 45): the master's own list of
 * the moments the group levels up, in the master's order. Each row has
 * "Marcar como alcançado" (the host opens its sheet), ↑ ↓, the pencil that
 * edits the name in place of the row, and the bin that asks in place before
 * removing. "Adicionar marco" opens the field in place, with the focus on it.
 * Only the master sees this list (the server never sends it to a player), and
 * only a planned milestone can change: a reached one is shown elsewhere.
 *
 * Every change is one call that answers with the whole list. Focus never
 * gets lost: after a move it stays on the button that was pressed; after a
 * removal it goes to the next row (or the one before, or "Adicionar marco");
 * stepping back from a form or a question returns it to the button that
 * opened it.
 */
@Component({
  selector: 'app-planned-milestones',
  imports: [MatButtonModule, MatIconModule, MilestoneAsk, MilestoneNameForm],
  templateUrl: './planned-milestones.html',
  styleUrl: './planned-milestones.scss',
})
export class PlannedMilestones {
  private readonly api = inject(ProgressionClient);
  private readonly store = inject(MilestonesStore);
  private readonly addKey = new ActionKey();
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  /** The planned milestones, in order. */
  readonly milestones = input.required<readonly Milestone[]>();
  /** How many milestones the campaign has in all (the reached ones count). */
  readonly total = input(0);

  /** The master pressed "Marcar como alcançado" on this milestone. */
  readonly reach = output<Milestone>();

  protected readonly adding = signal(false);
  protected readonly editing = signal<string | null>(null);
  protected readonly removing = signal<string | null>(null);
  /** Milestones the server refused to remove because they were reached once
   * and undone, before the list said so (`hasHistory`): "Remover" is not offered
   * again for them. */
  protected readonly kept = signal<ReadonlySet<string>>(new Set());
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** What happened, said once in a polite status ("Marco adicionado."). */
  protected readonly notice = signal('');

  protected readonly count = computed(() => plannedCount(this.milestones().length));
  protected readonly full = computed(() => this.total() >= MILESTONES_LIMIT);

  /** The question that opens a milestone's removal. */
  protected removeText(m: Milestone): string {
    return `Remover o marco ${m.text}?`;
  }

  protected openAdd(): void {
    this.closeAll();
    this.adding.set(true);
  }

  protected closeAdd(): void {
    this.adding.set(false);
    this.error.set('');
    this.focus('[data-add]');
  }

  protected async add(text: string): Promise<void> {
    // A retry of the same milestone (a lost answer, a second tap) sends the same key and adds it once.
    if (
      !(await this.run(
        () => this.api.addMilestone(this.campaignId(), text, this.addKey.keyFor(text)),
        'adicionar o marco',
      ))
    ) {
      return;
    }
    this.addKey.renew();
    this.adding.set(false);
    this.notice.set(`Marco adicionado: ${text}.`);
    this.focus('[data-add]');
  }

  protected startEdit(m: Milestone): void {
    this.closeAll();
    this.editing.set(m.id);
  }

  protected closeEdit(id: string): void {
    this.editing.set(null);
    this.error.set('');
    this.focus(`[data-id="${id}"][data-act="edit"]`);
  }

  protected async saveEdit(m: Milestone, text: string): Promise<void> {
    if (text === m.text) {
      this.closeEdit(m.id);
      return;
    }
    if (
      !(await this.run(
        () => this.api.updateMilestone(this.campaignId(), m.id, text),
        'salvar o marco',
      ))
    ) {
      return;
    }
    this.editing.set(null);
    this.notice.set('Marco salvo.');
    this.focus(`[data-id="${m.id}"][data-act="edit"]`);
  }

  protected askRemove(m: Milestone): void {
    this.closeAll();
    this.removing.set(m.id);
  }

  protected closeRemove(id: string): void {
    this.removing.set(null);
    this.error.set('');
    this.focus(`[data-id="${id}"][data-act="remove"]`);
  }

  protected async remove(m: Milestone): Promise<void> {
    const list = this.milestones();
    const at = list.findIndex((x) => x.id === m.id);
    if (
      !(await this.run(() => this.api.removeMilestone(this.campaignId(), m.id), 'remover o marco'))
    ) {
      return;
    }
    this.removing.set(null);
    this.notice.set('Marco removido.');
    // The row that took its place, else the one before it, else "Adicionar marco".
    const next = list[at + 1] ?? list[at - 1];
    this.focus(next ? `[data-id="${next.id}"][data-act="reach"]` : '[data-add]');
  }

  protected async move(m: Milestone, direction: 'up' | 'down', edge: boolean): Promise<void> {
    if (edge || this.busy()) {
      return; // already first or last: nothing to do
    }
    const act = direction === 'up' ? 'up' : 'down';
    if (
      await this.run(
        () => this.api.moveMilestone(this.campaignId(), m.id, direction),
        'mudar a ordem',
      )
    ) {
      this.notice.set(`${m.text} foi ${direction === 'up' ? 'para cima' : 'para baixo'}.`);
      this.focus(`[data-id="${m.id}"][data-act="${act}"]`);
    }
  }

  /** Runs a change that answers with the whole list and adopts it. False, with
   * `error` set, when it failed. */
  private async run(
    call: () => Promise<{ milestones: readonly Milestone[] }>,
    what: string,
  ): Promise<boolean> {
    if (this.busy()) {
      return false;
    }
    this.busy.set(true);
    this.error.set('');
    this.notice.set('');
    try {
      this.store.adopt((await call()).milestones);
      return true;
    } catch (err) {
      this.error.set(xpErrorMessage(err, what));
      const removingId = this.removing();
      if (
        removingId !== null &&
        xpBlocked(err)?.reason === XPBlockedReason.XP_BLOCKED_REASON_MILESTONE_HAS_HISTORY
      ) {
        this.kept.update((ids) => new Set(ids).add(removingId));
        this.removing.set(null);
        const text = this.milestones().find((m) => m.id === removingId)?.text;
        if (text) {
          this.error.set(milestoneHasHistoryText(`O marco “${text}”`));
        }
      }
      // The list may have moved under the master (another tab): read it again.
      void this.store.refresh();
      return false;
    } finally {
      this.busy.set(false);
    }
  }

  private closeAll(): void {
    this.adding.set(false);
    this.editing.set(null);
    this.removing.set(null);
    this.error.set('');
    this.notice.set('');
  }

  /** Focus on the first match once the screen has drawn what it will show. */
  private focus(selector: string): void {
    afterNextRender(() => this.host.nativeElement.querySelector<HTMLElement>(selector)?.focus(), {
      injector: this.injector,
    });
  }
}
