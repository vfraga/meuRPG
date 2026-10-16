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
  type ContestAttackOption,
  type ContestSkillOption,
  ContestKind,
  ContestPurpose,
  ContestSkill,
  ShoveBlockedReason,
  ShoveOutcome,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { RollAnimator, showOfCheck } from '../../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { type CheckDie, ContestClient } from '../../../../core/combat/contest-client';
import type { ContestState } from '../../../../core/combat/contest-state';
import {
  type Who,
  contestSteps,
  escapeIntro,
  holdLine,
  initiatorVerdict,
  named,
  ofNamed,
  proneLine,
  pushLine,
  sheetTitle,
  signed,
  sizeWord,
  skillChoices,
  skillLine,
  skillName,
  waitDetail,
  waitTitle,
  whoIs,
} from '../../../../core/combat/contest-view';
import {
  ESCAPE_AT,
  ESCAPE_STEPS,
  GRAPPLE_AT,
  GRAPPLE_STEPS,
  type Stage,
  checkOf,
  limitTextOf,
  maneuverWhy,
  stageOf,
  subtitleOf,
  targetRowsOf,
  titleOf,
  whyOf,
} from './contest-sheet-view';
import { CheckRollForm } from '../check-roll-form/check-roll-form';
import { ContestRoll } from '../contest-roll/contest-roll';
import { AttackSteps } from '../attack-sheet/attack-steps';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';
import { upFirst } from '../../../../core/format/text';

/** What the page hands the initiator's sheet: a grapple, a shove or an escape. */
export interface ContestSheetData {
  readonly campaignId: string;
  readonly encounterId: string;
  readonly initiatorId: string;
  readonly purpose: ContestPurpose;
  /** A grapple or a shove: the targets and what the Strength (Athletics) check rolls (`contest_attack_options`). */
  readonly attack?: ContestAttackOption;
  /** An escape: what each of the two skills rolls (`contest_state.escape_options`). */
  readonly escape?: readonly ContestSkillOption[];
  /** An escape: who holds the player (`contest_state.grappler_id`), when they see that creature. */
  readonly holderId?: string;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  /** Where each answer's combat goes (the page's copy of the combat). */
  readonly state: CombatState;
  /** The contests the player is in, read again by the page on every change of the combat. */
  readonly contests: ContestState;
  /** Opens at a contest that already exists: the wait, the result or the shove's choice. */
  readonly contestId?: string;
}

/** What the sheet answers when it closes: the contest it started or follows, so the page can bring it back with the result. */
export interface ContestSheetResult {
  readonly contestId: string;
}

export function openContestSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: ContestSheetData,
): Observable<ContestSheetResult | undefined> {
  return openSheet<ContestSheet, ContestSheetData, ContestSheetResult>(
    dialog,
    bottomSheet,
    ContestSheet,
    { data, ariaLabel: sheetTitle(data.purpose), labelledBy: 'contest-t' },
  );
}

/**
 * "Agarrar", "Empurrar" and "Escapar" (W7-X, boards W7-Xa 2 and 3, W7-Xb 5 and 7): the one that starts a contest. A grapple or a
 * shove is chosen from the attack list and replaces one attack: the target (a creature at reach, no more than one size above
 * the player's; "Tenho uma mão livre" is a reminder the server trusts), then the Strength (Athletics) roll, then the wait. An escape
 * spends the action: the player picks Athletics or Acrobacia and rolls against the one that holds them. The defender answers on
 * their own sheet; this one reads the contest the page keeps (`GetContestState`), so it shows "Esperando o mestre" or
 * "Esperando Brisa" (the combat's own wait, from `reaction_wait`) and then the result, or the choice of a won shove. "Fechar a
 * folha" only hides it: the page brings it back with the result. A player reads only their own total and who won (RN-20).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-contest-sheet',
  imports: [AttackSteps, CheckRollForm, ContestRoll, MatButtonModule, MatIconModule, SheetFrame],
  templateUrl: './contest-sheet.html',
  styleUrl: './contest-sheet.scss',
})
export class ContestSheet {
  private readonly api = inject(ContestClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<ContestSheetData, ContestSheetResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly escape = this.data.purpose === ContestPurpose.ESCAPE;
  protected readonly Outcome = ShoveOutcome;
  protected readonly skillLabel = skillLine;
  protected readonly signedText = signed;

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** The reminder of the board: the server trusts it, so it has to be marked before the roll. */
  protected readonly free = signal(false);
  /** SRD 5.1: only a grapple needs a free hand; a shove is an Athletics contest with no such requirement. */
  protected readonly needsHand = this.data.purpose !== ContestPurpose.SHOVE;
  protected readonly targetId = signal<string | null>(null);
  /** The skill of an escape (Atletismo or Acrobacia). */
  protected readonly skill = signal<ContestSkill>(ContestSkill.ATHLETICS);
  /** The roll form is open (the player chose the target and tapped "Rolar a disputa"). */
  private readonly rolling = signal(this.escape);
  private readonly started = signal(this.data.contestId ?? '');
  /** What a won shove will do. */
  protected readonly outcome = signal<ShoveOutcome | null>(null);

  private readonly startKeys = new ActionKey();
  private readonly shoveKeys = new ActionKey();
  private readonly frame = viewChild(SheetFrame);
  private readonly done = viewChild('done', { read: ElementRef<HTMLButtonElement> });

  protected readonly encounter = computed(() => this.data.state.encounter());
  protected readonly initiator = computed(() =>
    this.encounter()?.combatants.find((c) => c.id === this.data.initiatorId),
  );
  /** The contest this sheet follows, as the page last read it. */
  protected readonly contest = computed(() => {
    const id = this.started();
    return id ? this.data.contests.contest(id) : undefined;
  });
  protected readonly stage = computed<Stage>(() => {
    const c = this.contest();
    if (c) {
      return stageOf(c);
    }
    return this.rolling() ? 'roll' : 'target';
  });
  protected readonly stepNames = this.escape ? ESCAPE_STEPS : GRAPPLE_STEPS;
  protected readonly stepList = computed(() =>
    contestSteps(this.stepNames, (this.escape ? ESCAPE_AT : GRAPPLE_AT)[this.stage()]),
  );

  // ---- the target ----

  protected readonly rows = computed(() =>
    targetRowsOf(this.data.attack, this.encounter()?.combatants ?? []),
  );
  protected readonly target = computed(
    () => this.rows().find((r) => r.id === this.targetId()) ?? null,
  );
  protected readonly ownSize = computed(() => sizeWord(this.initiator()?.size ?? 0));
  protected readonly limitText = computed(() => limitTextOf(this.ownSize()));
  protected readonly canRoll = computed(
    () => (!this.needsHand || this.free()) && this.target() !== null,
  );
  /** Why "Rolar a disputa" is off, in words: the missing reminder first. */
  protected readonly why = computed(() =>
    whyOf(this.needsHand, this.free(), this.target() !== null),
  );

  // ---- the table maneuver that starts a grapple ----

  protected readonly maneuvers = computed(() =>
    this.data.purpose === ContestPurpose.GRAPPLE ? (this.data.attack?.maneuvers ?? []) : [],
  );
  /** The maneuver picked ("" is a plain grapple, which replaces an attack). */
  protected readonly maneuverKey = signal('');
  /** The face of the maneuver's die, typed when the d20 is rolled with physical dice. */
  protected readonly maneuverFace = signal(0);
  protected readonly maneuver = computed(() =>
    this.maneuvers().find((o) => o.key === this.maneuverKey()),
  );

  protected readonly maneuverWhy = maneuverWhy;

  protected pickManeuver(key: string): void {
    this.maneuverKey.set(key);
    this.error.set('');
  }

  protected typeManeuverFace(value: string): void {
    this.maneuverFace.set(Number.parseInt(value, 10) || 0);
  }

  // ---- the roll ----

  protected readonly options = computed<readonly ContestSkillOption[]>(() =>
    skillChoices(this.data.escape ?? []),
  );
  /** What the chosen check rolls: the Strength (Athletics) option of the attack, or the escape's skill. */
  protected readonly check = computed(() =>
    checkOf(
      this.escape
        ? (this.options().find((x) => x.skill === this.skill()) ?? this.options()[0])
        : this.data.attack?.rollOption,
    ),
  );
  protected readonly checkLine = computed(() =>
    skillLine(this.escape ? this.skill() : ContestSkill.ATHLETICS),
  );
  protected readonly modifierText = computed(() => signed(this.check().modifier));
  protected readonly rollSkill = computed(() => {
    const c = this.contest();
    const roll = c?.initiatorRoll;
    return roll ? skillName(roll.skill) : '';
  });

  // ---- what the sheet says ----

  /** The one grappled or shoved, or (for an escape) the one that holds. */
  protected readonly other = computed<Who>(() => {
    const enc = this.encounter();
    const c = this.contest();
    const id = c?.defenderId ?? (this.escape ? this.data.holderId : this.targetId());
    return whoIs(enc?.combatants.find((x) => x.id === id));
  });
  protected readonly title = computed(() =>
    titleOf(this.escape, this.data.purpose, this.stage(), this.other()),
  );
  protected readonly subtitle = computed(() => subtitleOf(this.escape, this.stage()));
  protected readonly intro = computed(() => {
    const c = this.contest();
    return escapeIntro(this.other(), c?.kind === ContestKind.ESCAPE_DC);
  });
  protected readonly waitTitle = computed(() => waitTitle(this.encounter(), this.contest()));
  protected readonly waitDetail = computed(() => waitDetail(this.encounter(), this.contest()));
  /** The footnote of the board: only said when the master is the one the contest waits for. */
  protected readonly waitingForMaster = computed(() => this.waitTitle() === 'Esperando o mestre');
  protected readonly verdict = computed(() => {
    const c = this.contest();
    return c ? initiatorVerdict(c, this.other(), this.initiator()?.label ?? '') : null;
  });
  protected readonly holds = computed(() => {
    const c = this.contest();
    return (
      !this.escape &&
      this.data.purpose === ContestPurpose.GRAPPLE &&
      !!c &&
      this.verdict()?.won === true
    );
  });
  protected readonly hold = computed(() => holdLine(this.other().label));

  // ---- the shove's choice ----

  protected readonly choice = computed(() => this.contest()?.shoveChoice);
  protected readonly pushBlocked = computed(
    () => this.choice()?.pushBlocked ?? ShoveBlockedReason.UNSPECIFIED,
  );
  protected readonly pushOff = computed(() => {
    const c = this.choice();
    return !c || !c.pushAvailable;
  });
  protected readonly pushText = computed(() => pushLine(this.pushBlocked()));
  protected readonly proneText = computed(() => proneLine(this.other()));
  protected readonly choiceLead = computed(() => `Escolha o que fazer com ${named(this.other())}.`);
  protected readonly upFirst = upFirst;
  protected readonly ofNamed = ofNamed;

  constructor() {
    // A request in the air: Esc and the backdrop do not close the sheet under it.
    effect(() => this.sheet.lock(this.busy()));
    // The first eligible target starts chosen, as the board draws it.
    effect(() => {
      const first = this.rows().find((r) => !r.blocked);
      if (this.targetId() === null && first && !this.started()) {
        this.targetId.set(first.id);
      }
    });
    // The escape starts on the skill that rolls best.
    effect(() => {
      const best = this.options()[0];
      if (this.escape && best && !this.started()) {
        this.skill.set(best.skill);
      }
    });
    // A won shove starts on the choice it can make: "Derrubar", or the push when it is free.
    effect(() => {
      if (this.stage() === 'choice' && this.outcome() === null) {
        this.outcome.set(ShoveOutcome.PRONE);
      }
    });
    // An error opens at the top of the scrolling body, where it is seen.
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
    // After the result the focus goes to its one action.
    effect(() => this.done()?.nativeElement.focus());
  }

  protected setFree(value: boolean): void {
    this.free.set(value);
    this.error.set('');
  }

  protected pick(id: string): void {
    this.targetId.set(id);
    this.error.set('');
  }

  protected pickSkill(skill: ContestSkill): void {
    this.skill.set(skill);
  }

  protected advance(): void {
    if (this.canRoll()) {
      this.rolling.set(true);
    }
  }

  protected async roll(die: CheckDie): Promise<void> {
    if (this.busy()) {
      return;
    }
    const target = this.escape ? '' : (this.targetId() ?? '');
    if (!this.escape && target === '') {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const skill = this.escape ? this.skill() : ContestSkill.ATHLETICS;
      const res = await this.api.start(
        {
          campaignId: this.data.campaignId,
          encounterId: this.data.encounterId,
          initiatorId: this.data.initiatorId,
          targetId: target,
          purpose: this.data.purpose,
          kind: ContestKind.CONTEST,
          skill,
          die,
          ...(this.maneuverKey()
            ? { maneuverKey: this.maneuverKey(), maneuverFace: this.maneuverFace() }
            : {}),
        },
        this.startKeys.keyFor({
          initiator: this.data.initiatorId,
          target,
          purpose: this.data.purpose,
          skill,
          die,
          maneuver: this.maneuverKey(),
          face: this.maneuverFace(),
        }),
      );
      this.data.state.apply(res.encounter);
      this.data.contests.applyContest(res.contest);
      // The initiator's own d20, as the sheet shows it; who wins is told when the other side has answered.
      const show = showOfCheck(
        `${sheetTitle(this.data.purpose)}: teste de ${skillName(skill)}`,
        res.contest.initiatorRoll,
        {
          withTotal: true,
        },
      );
      if (show) {
        this.animator.play(show);
      }
      this.started.set(res.contest.id);
      this.startKeys.renew();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'rolar a disputa'));
    } finally {
      this.busy.set(false);
    }
  }

  protected setOutcome(outcome: ShoveOutcome): void {
    this.outcome.set(outcome);
    this.error.set('');
  }

  /** "Confirmar": the shove's outcome, chosen by whoever won. */
  protected async confirm(): Promise<void> {
    const c = this.contest();
    const outcome = this.outcome();
    if (!c || outcome === null || this.busy()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const res = await this.api.resolveShove(
        this.data.campaignId,
        this.data.encounterId,
        c.id,
        outcome,
        this.shoveKeys.keyFor({ contest: c.id, outcome }),
      );
      this.data.state.apply(res.encounter);
      this.data.contests.applyContest(res.contest);
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'escolher o que fazer'));
    } finally {
      this.busy.set(false);
    }
  }

  protected close(): void {
    if (this.busy()) {
      return;
    }
    const id = this.started();
    this.sheet.close(id ? { contestId: id } : undefined);
  }
}
