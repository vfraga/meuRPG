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
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import { type XPAward, XPAwardMode } from '../../../../gen/meurpg/progression/v1/progression_pb';
import { ActionKey } from '../../../core/connect/idempotency';
import { ExperienceStore } from '../../../core/progression/experience-store';
import { ProgressionClient } from '../../../core/progression/progression-client';
import { xpAborted, xpErrorMessage, xpNothingToUndo } from '../../../core/progression/xp-errors';
import { awardTitle, townUndoneText } from '../../../core/progression/treasure';
import {
  awardEach,
  awardTotal,
  awardWhen,
  givenLine,
  modeTag,
  undoneLine,
} from '../../../core/progression/xp-labels';
import { undoConsequence, undoTitle } from './undo-text';

/**
 * The XP history of the campaign (E7-09): the awards, newest first, each with
 * when, who gave it, why, the mode tag and how much each one got; an undone
 * award keeps its line with the tag "Desfeito". Every member reads it. Only
 * the master, and only on the last award that is not undone (`can_undo`), has
 * "Desfazer".
 *
 * "Desfazer" asks in place, never in a dialog: the line becomes the question
 * (a warm `alertdialog` notice that says who loses what), the focus goes to
 * "Voltar" (the safe answer) and comes back to "Desfazer" when the master
 * steps back. The call carries `expected_award_id`: when it answers `aborted`,
 * someone gave or undid another award meanwhile, so the history is read again
 * and the screen says so. One key per question, so a retry never undoes two.
 */
@Component({
  selector: 'app-award-history',
  imports: [MatButtonModule, MatIconModule],
  templateUrl: './award-history.html',
  styleUrl: './award-history.scss',
})
export class AwardHistory {
  private readonly api = inject(ProgressionClient);
  private readonly store = inject(ExperienceStore);
  private readonly injector = inject(Injector);
  private readonly host = inject<ElementRef<HTMLElement>>(ElementRef);

  readonly campaignId = input.required<string>();
  /** XP numbers are not shown in a campaign that levels by milestones. */
  readonly milestones = input(false);
  /** The master reads which awards can still be undone, and why not. */
  readonly isMaster = input(false);
  /** The host has its own news on screen (a conversion just made): the history keeps its notice to itself. */
  readonly hideNotice = input(false);
  /** An award was undone: the host drops what it was saying about the one before. */
  readonly undone = output<void>();

  protected readonly awards = this.store.awards;
  protected readonly hasMore = computed(() => this.store.nextPageToken() !== '');
  protected readonly loadingMore = this.store.loadingMore;
  protected readonly Mode = XPAwardMode;

  /** The award the master is being asked about. */
  protected readonly asking = signal<string | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** What happened after the question: "XP desfeito", or that the history changed. */
  protected readonly notice = signal('');
  private readonly key = new ActionKey();

  private readonly voltar = viewChild('voltar', { read: ElementRef<HTMLButtonElement> });
  private readonly box = viewChild<ElementRef<HTMLElement>>('box');

  protected readonly tag = modeTag;
  protected readonly when = awardWhen;
  protected readonly given = givenLine;
  protected readonly heading = awardTitle;
  protected readonly each = awardEach;
  protected readonly total = awardTotal;
  protected readonly undoneText = undoneLine;
  protected readonly title = undoTitle;
  protected consequence(award: XPAward): string {
    return undoConsequence(award, this.store.rows());
  }

  protected ask(award: XPAward): void {
    this.asking.set(award.id);
    this.error.set('');
    this.notice.set('');
    afterNextRender(
      () => {
        // The whole question below the sticky app bar, then the focus on the safe answer.
        this.box()?.nativeElement.scrollIntoView({ block: 'start' });
        this.voltar()?.nativeElement.focus({ preventScroll: true });
      },
      { injector: this.injector },
    );
  }

  protected back(awardId: string): void {
    this.asking.set(null);
    this.error.set('');
    // The question replaced the "Desfazer" that opened it: focus goes back to it.
    afterNextRender(
      // Code puts the focus back here, so it asks for the ring the browser would not draw on its own.
      () =>
        this.host.nativeElement
          .querySelector<HTMLElement>(`[data-undo="${awardId}"]`)
          ?.focus({ focusVisible: true } as FocusOptions),
      { injector: this.injector },
    );
  }

  protected async undo(award: XPAward): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.undoLast(this.campaignId(), award.id, this.key.keyFor(award.id));
      this.asking.set(null);
      this.undone.emit();
      this.notice.set(
        award.mode === XPAwardMode.XP_AWARD_MODE_MILESTONE
          ? 'Marco desfeito.'
          : award.treasureCount > 0
            ? townUndoneText(award)
            : 'XP desfeito: o histórico guarda o prêmio como “Desfeito”.',
      );
      await this.store.refresh();
    } catch (err) {
      if (xpAborted(err)) {
        // Someone gave or undid another award: read again and say so.
        this.asking.set(null);
        this.undone.emit();
        this.notice.set(
          'O histórico mudou enquanto você olhava: outro prêmio foi dado ou desfeito. A lista foi atualizada; confira e tente de novo.',
        );
        await this.store.refresh();
      } else {
        this.error.set(xpErrorMessage(err, 'desfazer o prêmio'));
        if (xpNothingToUndo(err)) {
          // Its message says the screen was updated: the history is read again, so there is no award left to undo on it,
          // and whoever showed the XP of the party reads it again too (another tab changed it).
          this.undone.emit();
          await this.store.refresh();
        }
        // A failed call is retried with the same key: it changes nothing twice.
      }
    } finally {
      this.busy.set(false);
    }
  }

  protected more(): void {
    void this.store.more();
  }
}
