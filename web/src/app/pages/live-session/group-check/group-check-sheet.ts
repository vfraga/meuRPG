import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  Injector,
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

import { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import type { LuckyOffer, OutsideInspirationOffer } from '../../../../gen/meurpg/play/v1/combat_pb';
import { type GroupCheckView, RollModeKind } from '../../../../gen/meurpg/play/v1/contest_types_pb';
import type { RollGroupCheckResponse } from '../../../../gen/meurpg/play/v1/contests_pb';
import { effectivePreference } from '../../../core/campaigns/dice-labels';
import { ActionKey } from '../../../core/connect/idempotency';
import {
  type InspirationRoll,
  type LuckyRoll,
  ResourceClient,
} from '../../../core/resources/resources-client';
import { classResourceErrorMessage } from '../../../core/resources/resources-errors';
import { RollAnimator, showOfCheck, showOfDice } from '../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import {
  type CheckDie,
  ContestClient,
  InspirationAsked,
} from '../../../core/combat/contest-client';
import type { GroupCheckState } from '../../../core/combat/group-check-state';
import { contestSteps, signed } from '../../../core/combat/contest-view';
import { askedTest } from '../../../core/combat/contest-master';
import { AttackSteps } from '../combat/attack-sheet/attack-steps';
import { CheckRollForm } from '../combat/check-roll-form/check-roll-form';
import { ContestRoll } from '../combat/contest-roll/contest-roll';
import { SheetFrame } from '../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../shared/sheet/sheet-host';
import { InspirationOffer } from '../inspiration/inspiration-offer';
import { LuckyOfferCard } from '../inspiration/lucky-offer';

/** What the card hands the group check's sheet. */
export interface GroupCheckSheetData {
  readonly campaignId: string;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: GroupCheckState;
}

const STEPS = ['Rolar', 'Resultado'] as const;

/** The sheet's title: "Teste em grupo" only for a group check; a request judged one by one is a "Teste" (or "Teste de resistência"). */
export function groupCheckHeading(c: GroupCheckView | null | undefined): string {
  if (c?.group) {
    return 'Teste em grupo';
  }
  return c?.save ? 'Teste de resistência' : 'Teste';
}

export function openGroupCheckSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: GroupCheckSheetData,
): Observable<boolean | undefined> {
  return openSheet<GroupCheckSheet, GroupCheckSheetData, boolean>(
    dialog,
    bottomSheet,
    GroupCheckSheet,
    { data, ariaLabel: groupCheckHeading(data.state.view()), labelledBy: 'group-check-t' },
  );
}

/**
 * "Teste em grupo" (W7-X, board W7-Xc 10): the master asked every character for the same check (SRD 5.1, Group Checks). The player
 * rolls their own once, in the app or with the physical d20, and waits: they read their own total and "Esperando o mestre"; "passou"
 * or "falhou" and the group's verdict only appear if the master shows the DC (RN-20). The DC and the other characters' totals never
 * reach the player. "Fechar a folha" only hides it. A halfling's d20 of 1 is held for the Lucky question ("Tirou 1: rolar de
 * novo?") before anything is written; when the character also holds a Bardic Inspiration die, the die's question comes next,
 * on the new roll, on the same sheet.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-group-check-sheet',
  imports: [
    AttackSteps,
    CheckRollForm,
    ContestRoll,
    InspirationOffer,
    LuckyOfferCard,
    MatButtonModule,
    MatIconModule,
    SheetFrame,
  ],
  templateUrl: './group-check-sheet.html',
  styleUrl: '../combat/contest-sheet/contest-sheet.scss',
})
export class GroupCheckSheet {
  private readonly api = inject(ContestClient);
  /** Read when the player answers a question of a held roll: a sheet that never asks does not build the client. */
  private readonly lazyInjector = inject(Injector);
  private get resources(): ResourceClient {
    return this.lazyInjector.get(ResourceClient);
  }
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<GroupCheckSheetData, boolean>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  private readonly keys = new ActionKey();
  /** One key per answer to a question of the held roll (Lucky's, the die's): the same answer again is a retry. */
  private readonly luckyKeys = new ActionKey();
  private readonly dieKeys = new ActionKey();
  /** The d20 is held because a halfling's d20 shows a 1: the Lucky question, before anything is written. */
  protected readonly luckyOffer = signal<LuckyOffer | null>(null);
  /** After the Lucky answer, the Bardic Inspiration question on the new roll, when the character holds a die. */
  protected readonly offer = signal<OutsideInspirationOffer | null>(null);
  protected readonly canApp = this.data.diceMode !== DiceMode.PHYSICAL;
  protected readonly canType = this.data.diceMode !== DiceMode.APP;
  protected readonly preferApp =
    effectivePreference(this.data.diceMode, this.data.preference) === DicePreference.APP;
  private readonly frame = viewChild(SheetFrame);
  private readonly done = viewChild('done', { read: ElementRef<HTMLButtonElement> });

  protected readonly check = computed(() => this.data.state.view());
  protected readonly own = this.data.state.own;
  protected readonly skill = computed(() => this.check()?.skillNamePt ?? '');
  protected readonly heading = computed(() => groupCheckHeading(this.check()));
  /** "O mestre pediu um teste de Percepção de todo o grupo." or "O mestre pede um teste de resistência de Constituição.". */
  protected readonly askLine = computed(() => {
    const what = askedTest(this.skill());
    return this.check()?.group
      ? `O mestre pediu ${what} de todo o grupo.`
      : `O mestre pede ${what}.`;
  });
  protected readonly roll = computed(() => this.own()?.roll);
  protected readonly stage = computed<'roll' | 'result'>(() =>
    this.check()?.youRoll && !this.own()?.answered ? 'roll' : 'result',
  );
  protected readonly stepList = computed(() =>
    contestSteps(STEPS, this.stage() === 'roll' ? 0 : 1),
  );
  protected readonly open = computed(() => this.check()?.open ?? false);
  protected readonly option = computed(() => this.check()?.yourOption);
  protected readonly mode = computed(() => this.option()?.mode ?? RollModeKind.NORMAL);
  protected readonly modifier = computed(() => this.option()?.modifier ?? 0);
  protected readonly notes = computed(() => this.option()?.notes ?? []);
  protected readonly modifierText = computed(() => signed(this.modifier()));
  /** "Passou" or "Falhou", only when the master shows the DC. */
  protected readonly ownPass = computed(() => {
    const m = this.own();
    return m?.passedKnown ? (m.passed ? 'Passou' : 'Falhou') : '';
  });
  protected readonly verdict = computed(() => {
    const c = this.check();
    return c && !c.open && c.verdictKnown
      ? c.groupPassed
        ? 'O grupo passou.'
        : 'O grupo falhou.'
      : '';
  });

  constructor() {
    effect(() => this.sheet.lock(this.busy()));
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
    effect(() => this.done()?.nativeElement.focus());
  }

  protected async rollWith(die: CheckDie): Promise<void> {
    const check = this.check();
    if (!check || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const made = await this.api.rollGroupCheck(
        this.data.campaignId,
        check.id,
        die,
        this.keys.keyFor({ check: check.id, die }),
      );
      if (made.$typeName === 'meurpg.play.v1.LuckyOffer') {
        // Held for the Lucky question: the d20 alone tumbles, nothing is written yet.
        this.luckyOffer.set(made);
        const held = showOfDice(`Teste de ${this.skill()}`, made.d20);
        if (held) {
          this.animator.play(held);
        }
        return;
      }
      this.settle(made);
    } catch (err) {
      if (err instanceof InspirationAsked) {
        // Not an error: the d20 is rolled and kept, and the session panel asks about the die.
        this.error.set('');
        this.sheet.close(true);
        return;
      }
      this.error.set(combatErrorMessage(err, 'rolar o teste'));
      // The check changed under the sheet (the master closed it): read it again.
      void this.data.state.load(this.api, this.data.campaignId);
    } finally {
      this.busy.set(false);
    }
  }

  /** The check written: the player's own d20, as the result step shows it; "passou" only comes when the master closes it. */
  private settle(view: GroupCheckView): void {
    this.luckyOffer.set(null);
    this.offer.set(null);
    this.data.state.apply(view);
    const show = showOfCheck(`Teste de ${this.skill()}`, this.own()?.roll, { withTotal: true });
    if (show) {
      this.animator.play(show);
    }
  }

  /** What a question's answer came to: the die's question on the new roll, or the check written. */
  private afterAnswer(res: RollGroupCheckResponse): void {
    if (res.inspirationOffer) {
      this.luckyOffer.set(null);
      this.offer.set(res.inspirationOffer);
      return;
    }
    if (!res.groupCheck) {
      throw new Error('The answer came without its group check');
    }
    this.settle(res.groupCheck);
  }

  /** "Rolar de novo" (rolled in the app, or the typed face) or "Ficar com o 1": the check is written with the new d20, or the
   * Bardic Inspiration question comes next, on the new roll. */
  protected async answerLucky(reroll: boolean, die: LuckyRoll | null): Promise<void> {
    const offer = this.luckyOffer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideLucky(
        this.data.campaignId,
        offer.holdId,
        reroll,
        die,
        this.luckyKeys.keyFor({ hold: offer.holdId, reroll, die }),
      );
      if (res.result.case !== 'groupCheck') {
        throw new Error('AnswerOutsideLucky answered without its group check');
      }
      this.luckyKeys.renew();
      this.afterAnswer(res.result.value);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder ao Sortudo'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The Bardic Inspiration question that came after Lucky's: the check is written, with the die when it was used. */
  protected async answerDie(use: boolean, die: InspirationRoll | null): Promise<void> {
    const offer = this.offer();
    if (!offer || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.resources.answerOutsideInspiration(
        this.data.campaignId,
        offer.holdId,
        use,
        die,
        this.dieKeys.keyFor({ hold: offer.holdId, use, die }),
      );
      if (res.result.case !== 'groupCheck') {
        throw new Error('AnswerOutsideInspiration answered without its group check');
      }
      this.afterAnswer(res.result.value);
    } catch (err) {
      this.error.set(classResourceErrorMessage(err, 'responder sobre a Inspiração de Bardo'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close(this.stage() === 'result');
    }
  }
}
