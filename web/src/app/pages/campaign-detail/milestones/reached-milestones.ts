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

import type { Milestone, XPAward } from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import {
  leveledLine,
  markLines,
  reachedWhen,
  undoMilestoneConsequence,
} from '../../../core/progression/milestones';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { xpAborted, xpErrorMessage, xpNothingToUndo } from '../../../core/progression/xp-errors';
import { MilestoneAsk } from './milestone-ask';

/**
 * "Marcos alcançados" (E8-14, MR-016): each milestone that was reached, the
 * last one first, with when and for whom. The master reads who marked and,
 * under it, "Dar a mais alguém" (a character who was left out or arrived
 * late gets the same milestone, without a second one) and "Desfazer", on the
 * milestone whose mark is the last award; a player reads "Subiram de nível:
 * …" and nothing to press. "Desfazer" asks in place, with "Voltar" in focus;
 * its call carries `expected_award_id`, so a stale screen changes nothing.
 */
@Component({
  selector: 'app-reached-milestones',
  imports: [MatButtonModule, MatIconModule, MilestoneAsk],
  templateUrl: './reached-milestones.html',
  styleUrl: './reached-milestones.scss',
})
export class ReachedMilestones {
  private readonly api = inject(ProgressionClient);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  /** The reached milestones, the last reached first. */
  readonly milestones = input.required<readonly Milestone[]>();
  readonly isMaster = input(false);
  /** The milestones some living character does not have yet ("Dar a mais alguém"). */
  readonly giveable = input<ReadonlySet<string>>(new Set());

  /** The master pressed "Dar a mais alguém". */
  readonly give = output<Milestone>();
  /** The master undid the last mark; the host reads everything again. */
  readonly undone = output<string>();

  protected readonly asking = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** Why a stale screen changed nothing. */
  protected readonly notice = signal('');
  private readonly key = new ActionKey();

  protected readonly rows = computed(() =>
    this.milestones().map((m) => ({
      m,
      when: reachedWhen(m),
      lines: this.isMaster() ? markLines(m) : [leveledLine(m)],
      last: this.isMaster() ? m.marks.find((a) => a.canUndo) : undefined,
      give: this.isMaster() && this.giveable().has(m.id),
    })),
  );

  protected askTitle(m: Milestone): string {
    return `Desfazer o marco “${m.text}”?`;
  }

  protected consequence(m: Milestone, mark: XPAward): string {
    return undoMilestoneConsequence(m, mark);
  }

  protected ask(m: Milestone): void {
    this.asking.set(m.id);
    this.error.set('');
    this.notice.set('');
  }

  protected back(m: Milestone): void {
    this.asking.set(null);
    this.error.set('');
    // The question replaced the button that opened it: the focus goes back to it.
    afterNextRender(
      () => this.host.nativeElement.querySelector<HTMLElement>(`[data-undo="${m.id}"]`)?.focus(),
      {
        injector: this.injector,
      },
    );
  }

  protected async undo(mark: XPAward): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.undoLast(this.campaignId(), mark.id, this.key.keyFor(mark.id));
      this.asking.set(null);
      this.undone.emit('Marco desfeito.');
    } catch (err) {
      if (xpAborted(err)) {
        // Another award is the last now: read again and say so.
        this.asking.set(null);
        this.notice.set(
          'A lista mudou enquanto você olhava: outro marco foi dado ou desfeito. Ela foi atualizada; confira e tente de novo.',
        );
        this.undone.emit('');
      } else {
        // A failed call is retried with the same key: it changes nothing twice.
        this.error.set(xpErrorMessage(err, 'desfazer o marco'));
        if (xpNothingToUndo(err)) {
          // Its message says the screen was updated: the list is read again.
          this.undone.emit('');
        }
      }
    } finally {
      this.busy.set(false);
    }
  }
}
