import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { MatBottomSheet } from '@angular/material/bottom-sheet';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatIconModule } from '@angular/material/icon';
import type { Observable } from 'rxjs';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type CheckOption,
  HideAttemptStatus,
  RollModeKind,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { article } from '../../../../core/combat/combat-log';
import { RollAnimator, showOfCheck } from '../../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { type CheckDie, ContestClient } from '../../../../core/combat/contest-client';
import type { ContestState } from '../../../../core/combat/contest-state';
import {
  contestSteps,
  hiddenWord,
  refusalParts,
  signed,
} from '../../../../core/combat/contest-view';
import { AttackSteps } from '../attack-sheet/attack-steps';
import { CheckRollForm } from '../check-roll-form/check-roll-form';
import { ContestRoll } from '../contest-roll/contest-roll';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';

/** What the page hands the Hide sheet. */
export interface HideSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly combatantId: string;
  /** The action that gives Hide: "standard:hide", or the rogue's Cunning Action (a bonus action). */
  readonly actionKey: string;
  /** "Ação" or "Ação bônus": what the Hide spends. */
  readonly economy: string;
  /** What the Dexterity (Stealth) check rolls (`contest_state.hide_option`). */
  readonly option: CheckOption | undefined;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
  readonly contests: ContestState;
  /** Opens at an attempt that already exists: the wait or the master's answer. */
  readonly attemptId?: string;
}

/** The attempt the sheet made or follows, so the page can bring it back with the master's answer. */
export interface HideSheetResult {
  readonly attemptId: string;
}

const STEPS = ['Teste', 'Resultado'] as const;
/** The Dexterity (Stealth) check, as the sheet names it. */
const CHECK = 'Furtividade';

export function openHideSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: HideSheetData,
): Observable<HideSheetResult | undefined> {
  return openSheet<HideSheet, HideSheetData, HideSheetResult>(dialog, bottomSheet, HideSheet, {
    data,
    ariaLabel: 'Esconder-se',
    labelledBy: 'hide-t',
  });
}

/**
 * "Esconder-se" (W7-X, board W7-Xc 8): a Dexterity (Stealth) check, with the action or, for the rogue's Cunning Action, the
 * bonus action. The roll waits for the master, who says whether there is somewhere to hide and what each creature notices; the
 * player reads only "Você está escondida." (never who noticed, never a Perception, RN-10 and RN-20) or the master's refusal, and
 * the action is spent either way. It reads the attempt the page keeps; "Fechar a folha" only hides it, and the page brings it
 * back with the master's answer.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-hide-sheet',
  imports: [AttackSteps, CheckRollForm, ContestRoll, MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './hide-sheet.html',
  styleUrl: '../contest-sheet/contest-sheet.scss',
})
export class HideSheet {
  private readonly api = inject(ContestClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<HideSheetData, HideSheetResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly started = signal(this.data.attemptId ?? '');
  private readonly keys = new ActionKey();
  private readonly frame = viewChild(SheetFrame);
  private readonly done = viewChild('done', { read: ElementRef<HTMLButtonElement> });

  protected readonly checkName = CHECK;
  protected readonly steps = computed(() => contestSteps(STEPS, this.attempt() ? 1 : 0));
  protected readonly own = computed(() =>
    this.data.state.encounter()?.combatants.find((c) => c.id === this.data.combatantId),
  );
  protected readonly attempt = computed(() => {
    const id = this.started();
    return id ? this.data.contests.attempt(id) : undefined;
  });
  protected readonly mode = this.data.option?.mode ?? RollModeKind.NORMAL;
  protected readonly notes = this.data.option?.notes ?? [];
  protected readonly modifier = this.data.option?.modifier ?? 0;
  protected readonly modifierText = signed(this.modifier);
  protected readonly subtitle = `${this.data.economy} · Destreza (${CHECK})`;
  /** "quem a vê claramente" / "quem o vê claramente", by the character's name. */
  protected readonly seen = computed(() => (article(this.own()?.label ?? '') === 'a' ? 'a' : 'o'));
  protected readonly status = computed(() => this.attempt()?.status);
  protected readonly Pending = HideAttemptStatus.PENDING;
  protected readonly Applied = HideAttemptStatus.APPLIED;
  protected readonly hidden = computed(() => hiddenWord(this.own()?.label ?? ''));
  protected readonly refusal = computed(() => refusalParts(this.attempt()?.refusal ?? ''));

  constructor() {
    effect(() => this.sheet.lock(this.busy()));
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
    effect(() => this.done()?.nativeElement.focus());
  }

  protected async roll(die: CheckDie): Promise<void> {
    if (this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.hide(
        this.data.campaignId,
        this.data.encounterId,
        this.data.combatantId,
        this.data.actionKey,
        die,
        this.keys.keyFor({ who: this.data.combatantId, action: this.data.actionKey, die }),
      );
      this.data.state.apply(res.encounter);
      this.data.contests.applyAttempt(res.attempt);
      // The Dexterity (Stealth) d20; whether it worked waits for the master, so no word is told.
      const show = showOfCheck('Teste de Furtividade', res.attempt.roll, { withTotal: true });
      if (show) {
        this.animator.play(show);
      }
      this.started.set(res.attempt.id);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'se esconder'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      const id = this.started();
      this.sheet.close(id ? { attemptId: id } : undefined);
    }
  }
}
