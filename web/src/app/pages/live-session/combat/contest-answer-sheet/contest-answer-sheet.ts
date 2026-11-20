import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  computed,
  effect,
  Injector,
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
  type ContestSkillOption,
  type ContestView,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  RollModeKind,
} from '../../../../../gen/meurpg/play/v1/contest_types_pb';
import { ActionKey } from '../../../../core/connect/idempotency';
import { contestHeld, waysOf } from '../contest-held';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';
import { article } from '../../../../core/combat/combat-log';
import { RollAnimator, showOfCheck } from '../../../../shared/roll-overlay/roll-animator';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { type CheckDie, ContestClient } from '../../../../core/combat/contest-client';
import type { ContestState } from '../../../../core/combat/contest-state';
import {
  contestSteps,
  defenderTitle,
  defenderVerdict,
  isSettled,
  noteLines,
  signed,
  skillChoices,
  skillLine,
  skillName,
  waitTitle,
  whoIs,
} from '../../../../core/combat/contest-view';
import { AttackSteps } from '../attack-sheet/attack-steps';
import { CheckRollForm } from '../check-roll-form/check-roll-form';
import { ContestRoll } from '../contest-roll/contest-roll';
import { SheetFrame } from '../../../../shared/sheet/sheet-frame/sheet-frame';
import { injectSheet, openSheet } from '../../../../shared/sheet/sheet-host';
import { FavorSpend, circumstancesOfNotes } from '../../../../core/play/favor-spend';
import { FavorState } from '../../../../core/play/favor-state';
import { isPlayer } from '../../../../core/combat/combat-view';

/** What the page hands the defender's sheet. */
export interface ContestAnswerData {
  readonly campaignId: string;
  readonly encounterId: string;
  /** The contest that waits for this player's answer (read with `GetContestState`). */
  readonly contestId: string;
  readonly diceMode: DiceMode;
  readonly preference: DicePreference;
  readonly state: CombatState;
  readonly contests: ContestState;
}

/** The contest the sheet answered or follows, so the page can bring it back with the result. */
export interface ContestAnswerResult {
  readonly contestId: string;
}

const STEPS = ['Escolha', 'Resultado'] as const;

export function openContestAnswerSheet(
  dialog: MatDialog,
  bottomSheet: MatBottomSheet,
  data: ContestAnswerData,
  ariaLabel: string,
): Observable<ContestAnswerResult | undefined> {
  return openSheet<ContestAnswerSheet, ContestAnswerData, ContestAnswerResult>(
    dialog,
    bottomSheet,
    ContestAnswerSheet,
    { data, ariaLabel, labelledBy: 'contest-answer-t' },
  );
}

/**
 * "Hobgoblin tenta agarrar você" (W7-X, board W7-Xb 4): the player is the target of a grapple or a shove, and chooses Força
 * (Atletismo) or Destreza (Acrobacia) and rolls: in the app, with the typed d20 (or the pair), or leaving the roll to the master
 * ("Deixar o mestre rolar por mim"). The two skills come with their modifier and the circumstances that change the roll's mode.
 * It reads the contest the page keeps, so the result shows when it is decided; the player reads only whether they won, never the
 * other side's total (RN-20). A tie changes nothing. It opens by itself when the contest waits for this player (the window of kind
 * CONTEST is theirs), and answers with `RespondContest`, never with `AnswerReaction`.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-contest-answer-sheet',
  imports: [
    AttackSteps,
    CheckRollForm,
    ContestRoll,
    HeldRollPrompt,
    MatButtonModule,
    MatIconModule,
    SheetFrame,
  ],
  templateUrl: './contest-answer-sheet.html',
  styleUrl: '../contest-sheet/contest-sheet.scss',
})
export class ContestAnswerSheet {
  private readonly api = inject(ContestClient);
  private readonly animator = inject(RollAnimator);
  private readonly sheet = injectSheet<ContestAnswerData, ContestAnswerResult>();
  protected readonly data = this.sheet.data;
  protected readonly inSheet = this.sheet.inSheet;
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly skill = signal<ContestSkill | null>(null);

  private readonly injector = inject(Injector);
  private get resources(): ResourceClient {
    return this.injector.get(ResourceClient); // built when a question is answered: the sheet needs it for no other call
  }
  /** The question of the defender's roll when the d20 was kept for one: Lucky's 1, then the Bardic Inspiration die. */
  protected readonly held = contestHeld({
    resources: () => this.resources,
    campaignId: this.data.campaignId,
    busy: this.busy,
    error: this.error,
    side: 'contestAnswer',
    settled: (encounter, contest) => {
      this.data.state.apply(encounter);
      this.keys.renew();
      this.settled(contest, true, this.chosenSkill());
    },
    asked: (encounter) => this.data.state.apply(encounter),
  });
  protected readonly ways = waysOf(this.data);
  private readonly keys = new ActionKey();
  private readonly frame = viewChild(SheetFrame);
  private readonly done = viewChild('done', { read: ElementRef<HTMLButtonElement> });

  protected readonly contest = computed(() => this.data.contests.contest(this.data.contestId));
  protected readonly encounter = computed(() => this.data.state.encounter());
  protected readonly initiator = computed(() => {
    const id = this.contest()?.initiatorId;
    return this.encounter()?.combatants.find((c) => c.id === id);
  });
  protected readonly own = computed(() => {
    const id = this.contest()?.defenderId;
    return this.encounter()?.combatants.find((c) => c.id === id);
  });
  protected readonly purpose = computed(() => this.contest()?.purpose ?? ContestPurpose.GRAPPLE);
  protected readonly stage = computed<'choose' | 'wait' | 'result'>(() => {
    const c = this.contest();
    if (!c) {
      return 'choose';
    }
    if (isSettled(c) || c.status === ContestStatus.AWAITING_OUTCOME) {
      return 'result';
    }
    return c.youAnswer ? 'choose' : 'wait';
  });
  protected readonly stepList = computed(() =>
    contestSteps(STEPS, this.stage() === 'result' ? 1 : 0),
  );
  protected readonly title = computed(() =>
    defenderTitle(this.initiator()?.label ?? 'Alguém', this.purpose()),
  );
  protected readonly byPlayer = computed(
    () => !!this.initiator() && whoIs(this.initiator()).player,
  );
  /** "contra o teste dele (SRD, Grappling)": the citation is the board's wording for an NPC's test. */
  protected readonly intro = computed(() => {
    const label = this.initiator()?.label ?? '';
    const of = label ? (article(label) === 'a' ? 'dela' : 'dele') : 'dele';
    const rule = this.purpose() === ContestPurpose.SHOVE ? 'Shoving a Creature' : 'Grappling';
    return this.byPlayer()
      ? { tail: `contra o teste ${of}.` }
      : { tail: `contra o teste ${of} (SRD, ${rule}).` };
  });
  protected readonly options = computed<readonly ContestSkillOption[]>(() =>
    skillChoices(this.contest()?.answerOptions ?? []),
  );
  protected readonly chosen = computed(() => {
    const picked = this.skill();
    return this.options().find((o) => o.skill === picked) ?? this.options()[0];
  });
  protected readonly chosenSkill = computed(() => this.chosen()?.skill ?? ContestSkill.ATHLETICS);
  protected readonly mode = computed(() => this.chosen()?.mode ?? RollModeKind.NORMAL);
  protected readonly modifier = computed(() => this.chosen()?.modifier ?? 0);
  protected readonly checkName = computed(() => skillLine(this.chosenSkill()));
  protected readonly waitTitle = computed(() => waitTitle(this.encounter(), this.contest()));
  protected readonly waitText = 'O mestre rola por você.';
  protected readonly roll = computed(() => this.contest()?.defenderRoll);
  protected readonly rollSkill = computed(() => {
    const r = this.roll();
    return r ? skillName(r.skill) : '';
  });
  protected readonly verdict = computed(() => {
    const c = this.contest();
    return c ? defenderVerdict(c, whoIs(this.initiator())) : null;
  });
  /** The master's Inspiração (RN-43) for the defender's d20; never for "Deixar o mestre rolar por mim" (no d20 is rolled). */
  protected readonly fav = new FavorSpend({
    favor: inject(FavorState),
    who: () => {
      const me = this.own();
      return me && isPlayer(me) ? { characterId: me.characterId, name: me.label } : null;
    },
    circumstances: () => circumstancesOfNotes(this.mode(), this.chosen()?.notes ?? []),
  });
  protected readonly signedText = signed;
  protected readonly skillLabel = skillLine;
  protected readonly noteLines = noteLines;

  constructor() {
    effect(() => this.sheet.lock(this.busy()));
    // The skill that rolls best starts chosen, as the board draws it.
    effect(() => {
      const best = this.options()[0];
      if (best && this.skill() === null) {
        this.skill.set(best.skill);
      }
    });
    effect(() => {
      if (this.error()) {
        this.frame()?.scrollToTop();
      }
    });
    effect(() => this.done()?.nativeElement.focus());
  }

  protected pick(skill: ContestSkill): void {
    this.skill.set(skill);
    this.error.set('');
  }

  /** The roll (`die`), or "Deixar o mestre rolar por mim" (`null`), with the skill that is chosen. */
  protected async answer(die: CheckDie | null): Promise<void> {
    if (this.busy() || !this.contest()) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    try {
      const skill = this.chosenSkill();
      const spend = die !== null && this.fav.spend();
      const res = await this.api.respond(
        {
          campaignId: this.data.campaignId,
          encounterId: this.data.encounterId,
          contestId: this.data.contestId,
          skill,
          die,
          ...(spend ? { spendFavor: true } : {}),
        },
        this.keys.keyFor({ contest: this.data.contestId, skill, die, favor: spend }),
      );
      this.fav.spent();
      this.data.state.apply(res.encounter);
      if (!res.contest) {
        // The d20 is kept for a question (a halfling's 1, a Bardic Inspiration die): the player decides before the contest settles.
        this.held.ask(res.held);
        return;
      }
      this.settled(res.contest, die !== null, skill);
    } catch (err) {
      this.error.set(this.fav.refused(err) ?? combatErrorMessage(err, 'responder à disputa'));
    } finally {
      this.busy.set(false);
    }
  }

  /** The contest is answered: the defender's own d20 (nothing when the master rolls for the player). */
  private settled(contest: ContestView, rolled: boolean, skill: ContestSkill): void {
    this.data.contests.applyContest(contest);
    const show = rolled
      ? showOfCheck(`Teste de ${skillName(skill)}`, contest.defenderRoll, { withTotal: true })
      : null;
    if (show) {
      this.animator.play(show);
    }
  }

  protected close(): void {
    if (!this.busy()) {
      this.sheet.close({ contestId: this.data.contestId });
    }
  }
}
