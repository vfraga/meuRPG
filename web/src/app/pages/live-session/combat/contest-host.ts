import { type Signal, effect, signal, untracked, computed } from '@angular/core';
import type { MatBottomSheet } from '@angular/material/bottom-sheet';
import type { MatDialog } from '@angular/material/dialog';
import type { Observable } from 'rxjs';

import type { DiceMode, DicePreference } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type Combatant,
  type Encounter,
  type GetTurnOptionsResponse,
  EncounterStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  type ContestAttackOption,
  ContestAttackOptionKind,
  ContestPurpose,
  ContestStatus,
  HideAttemptStatus,
} from '../../../../gen/meurpg/play/v1/contest_types_pb';
import { ActionEconomy } from '../../../../gen/meurpg/rules/v1/rules_pb';
import type { CombatState } from '../../../core/combat/combat-state';
import type { ContestClient } from '../../../core/combat/contest-client';
import { ContestState } from '../../../core/combat/contest-state';
import { isSettled } from '../../../core/combat/contest-view';
import { owesMountQuestion } from '../../../core/combat/mount-save';
import { helpAllies, helpTargets } from '../../../core/combat/help-view';
import {
  ContestAnswerSheet,
  openContestAnswerSheet,
} from './contest-answer-sheet/contest-answer-sheet';
import { ContestSheet, openContestSheet } from './contest-sheet/contest-sheet';
import { openHelpSheet } from './help-sheet/help-sheet';
import { openHideSheet } from './hide-sheet/hide-sheet';
import type { TargetFlow } from './target-mode/target-flow';
import { trackOpen } from '../../../shared/sheet/sheet-host';

/** What the host reads from the combat page. */
export interface ContestHostDeps {
  readonly dialog: MatDialog;
  readonly bottomSheet: MatBottomSheet;
  readonly api: ContestClient;
  readonly campaignId: () => string;
  readonly isMaster: () => boolean;
  readonly encounter: Signal<Encounter | null>;
  readonly own: Signal<Combatant | null>;
  /** The player's `GetTurnOptions` (the special attacks, the contest facts of the turn). */
  readonly options: Signal<GetTurnOptionsResponse | null>;
  readonly state: () => CombatState;
  readonly diceMode: () => DiceMode;
  readonly preference: () => DicePreference;
  /** The player's choice of a target on the map (C7-04); absent where the sheet keeps its list. */
  readonly target?: () => TargetFlow;
}

/** The creature the player tapped on the map goes first in the sheet's list, where the sheet starts its choice (C7-04). */
function tappedFirst(option: ContestAttackOption, id: string): ContestAttackOption {
  const tapped = option.targets.filter((t) => t.combatantId === id);
  return { ...option, targets: [...tapped, ...option.targets.filter((t) => t.combatantId !== id)] };
}

/**
 * The player's side of the contests of a combat (W7-X): reads the contest facts again whenever the combat changes (every contest
 * change reaches the table as `encounter_changed`), opens the sheets the player asks for ("Agarrar", "Empurrar", "Escapar",
 * "Esconder", "Ajudar") and the ones the combat asks of them (the defender's answer, the winner's choice of a shove), once each,
 * and brings back a sheet that was only hidden ("Fechar a folha") when its contest is decided. Nothing here is kept in the
 * browser: the contest lives on the server, and a reload opens the same questions again. The master's cards are not here.
 */
export class ContestHost {
  readonly contests = new ContestState();
  /** A contest sheet is on screen: no second one opens over it. */
  readonly sheetOpen = signal(false);
  /** The sheet that is on screen is a contest's own (the answer or the contest sheet), the one `dismissSheet` may close. */
  private contestSheetOpen = false;
  /** The questions the player already got a sheet for (this page's life): each opens by itself once. */
  private readonly handled = new Set<string>();
  /** The sheets that were closed while their contest still waited: they come back with the result. */
  private readonly waiting = new Set<string>();

  /** The contests that wait for this player's answer or choice, as the server reads them. */
  readonly pending = computed(() =>
    this.deps.isMaster()
      ? []
      : this.contests
          .contests()
          .filter(
            (c) =>
              (c.status === ContestStatus.AWAITING_DEFENDER && c.youAnswer) ||
              (c.status === ContestStatus.AWAITING_OUTCOME && c.youChoose),
          ),
  );
  /** The keys of the Hide actions the character has ("standard:hide", the rogue's Cunning Action). */
  readonly hideKeys = computed(
    () => this.deps.options()?.contestState?.hideActions.map((a) => a.key) ?? [],
  );

  constructor(private readonly deps: ContestHostDeps) {
    effect(() => {
      const e = deps.encounter();
      const campaignId = deps.campaignId();
      if (deps.isMaster() || !e || e.status !== EncounterStatus.ACTIVE) {
        untracked(() => this.contests.clear());
        return;
      }
      void e.revision;
      untracked(() => void this.contests.load(deps.api, campaignId, e.id));
    });
    // The question a contest asks of this player opens its sheet by itself, once; a reload asks it again.
    effect(() => {
      const list = this.contests.contests();
      void this.contests.attempts();
      const e = deps.encounter();
      // The combat's own question about the mount comes first: no contest sheet opens while it is owed (the question stays in
      // `pending`, with the way back, and opens when the mount question is answered).
      if (this.sheetOpen() || deps.isMaster() || (e && owesMountQuestion(e))) {
        return;
      }
      untracked(() => this.openNext(list));
    });
  }

  private openNext(list: ReturnType<ContestState['contests']>): void {
    for (const c of list) {
      const answer = c.status === ContestStatus.AWAITING_DEFENDER && c.youAnswer;
      const choose = c.status === ContestStatus.AWAITING_OUTCOME && c.youChoose;
      const key = `${answer ? 'answer' : 'choose'}:${c.id}`;
      if ((answer || choose) && !this.handled.has(key)) {
        this.handled.add(key);
        if (answer) {
          this.openAnswer(c.id);
        } else {
          this.openContest(c.purpose, c.initiatorId, c.id);
        }
        return;
      }
    }
    this.reopenDecided(list);
  }

  /** A sheet that was hidden while its contest waited comes back once the contest is decided. */
  private reopenDecided(list: ReturnType<ContestState['contests']>): void {
    for (const key of [...this.waiting]) {
      const [kind, id] = key.split(':');
      const contest = list.find((c) => c.id === id);
      if (kind === 'contest' && contest && isSettled(contest)) {
        this.waiting.delete(key);
        this.openContest(contest.purpose, contest.initiatorId, contest.id);
        return;
      }
      if (kind === 'answer' && contest && isSettled(contest)) {
        this.waiting.delete(key);
        this.openAnswer(contest.id);
        return;
      }
      const attempt = this.contests.attempt(id);
      if (kind === 'hide' && attempt && attempt.status !== HideAttemptStatus.PENDING) {
        this.waiting.delete(key);
        this.openHideAt(attempt.hiderId, attempt.id);
        return;
      }
    }
  }

  /** Closes the contest's answer or result sheet: the combat has a question of its own to put in its place (the mount save). */
  dismissSheet(): void {
    if (!this.sheetOpen() || !this.contestSheetOpen) {
      return;
    }
    for (const ref of this.deps.dialog.openDialogs) {
      const sheet = ref.componentInstance;
      if (sheet instanceof ContestAnswerSheet || sheet instanceof ContestSheet) {
        ref.close();
      }
    }
    this.deps.bottomSheet.dismiss();
  }

  /** Runs a sheet: no other opens meanwhile, and what it answers is told to `onResult`. */
  private track<R>(
    sheet: Observable<R | undefined>,
    onResult: (r: R) => void,
    contestOwn = false,
  ): void {
    this.contestSheetOpen = contestOwn;
    trackOpen(this.sheetOpen, sheet, {
      next: (r) => {
        if (r !== undefined) {
          onResult(r);
        }
      },
      complete: () => {
        this.contestSheetOpen = false;
      },
      error: () => {
        this.contestSheetOpen = false;
      },
    });
  }

  // ---- what the player asks for ----

  /** "Agarrar" or "Empurrar": the target, the roll and the wait. */
  openAttack(kind: ContestAttackOptionKind): void {
    const own = this.deps.own();
    const option = this.deps.options()?.contestAttackOptions.find((o) => o.kind === kind);
    const e = this.deps.encounter();
    if (!own || !option || !e) {
      return;
    }
    const shove = kind === ContestAttackOptionKind.SHOVE;
    const labelOf = (id: string) => e.combatants.find((c) => c.id === id)?.label ?? 'Alguém';
    // The target is tapped on the map first (C7-04); the sheet then opens with it chosen.
    const mapped = this.deps
      .target?.()
      ?.contest(option, shove ? 'Empurrar' : 'Agarrar', labelOf, (target) =>
        this.startContest(own.id, e.id, option, target),
      );
    if (!mapped) {
      this.startContest(own.id, e.id, option, null);
    }
  }

  /** The grapple or shove sheet for the target chosen on the map, or the list's own first one. */
  private startContest(
    initiatorId: string,
    encounterId: string,
    option: ContestAttackOption,
    target: string | null,
  ): void {
    this.track(
      openContestSheet(this.deps.dialog, this.deps.bottomSheet, {
        campaignId: this.deps.campaignId(),
        encounterId,
        initiatorId,
        purpose:
          option.kind === ContestAttackOptionKind.SHOVE
            ? ContestPurpose.SHOVE
            : ContestPurpose.GRAPPLE,
        attack: target ? tappedFirst(option, target) : option,
        diceMode: this.deps.diceMode(),
        preference: this.deps.preference(),
        state: this.deps.state(),
        contests: this.contests,
      }),
      (r) => this.waitFor('contest', r.contestId),
      true,
    );
  }

  /** "Escapar": the action, Atletismo or Acrobacia against whoever holds the character. */
  openEscape(): void {
    const own = this.deps.own();
    const facts = this.deps.options()?.contestState;
    const e = this.deps.encounter();
    if (!own || !facts || !e) {
      return;
    }
    this.track(
      openContestSheet(this.deps.dialog, this.deps.bottomSheet, {
        campaignId: this.deps.campaignId(),
        encounterId: e.id,
        initiatorId: own.id,
        purpose: ContestPurpose.ESCAPE,
        escape: facts.escapeOptions,
        holderId: facts.grapplerId,
        diceMode: this.deps.diceMode(),
        preference: this.deps.preference(),
        state: this.deps.state(),
        contests: this.contests,
      }),
      (r) => this.waitFor('contest', r.contestId),
      true,
    );
  }

  /** A contest that already exists: the shove's choice, or the result. */
  private openContest(purpose: ContestPurpose, initiatorId: string, contestId: string): void {
    const e = this.deps.encounter();
    if (!e) {
      return;
    }
    this.track(
      openContestSheet(this.deps.dialog, this.deps.bottomSheet, {
        campaignId: this.deps.campaignId(),
        encounterId: e.id,
        initiatorId,
        purpose,
        diceMode: this.deps.diceMode(),
        preference: this.deps.preference(),
        state: this.deps.state(),
        contests: this.contests,
        contestId,
      }),
      (r) => this.waitFor('contest', r.contestId),
      true,
    );
  }

  /** The defender's sheet: the skill and the roll. */
  private openAnswer(contestId: string): void {
    const e = this.deps.encounter();
    if (!e) {
      return;
    }
    const contest = this.contests.contest(contestId);
    const initiator = e.combatants.find((c) => c.id === contest?.initiatorId)?.label ?? 'Alguém';
    this.track(
      openContestAnswerSheet(
        this.deps.dialog,
        this.deps.bottomSheet,
        {
          campaignId: this.deps.campaignId(),
          encounterId: e.id,
          contestId,
          diceMode: this.deps.diceMode(),
          preference: this.deps.preference(),
          state: this.deps.state(),
          contests: this.contests,
        },
        `${initiator} tenta ${contest?.purpose === ContestPurpose.SHOVE ? 'empurrar' : 'agarrar'} você`,
      ),
      (r) => this.waitFor('answer', r.contestId),
      true,
    );
  }

  /** "Esconder": the Dexterity (Stealth) check, spending the action or the bonus action the key says. */
  openHide(actionKey: string): void {
    const own = this.deps.own();
    if (own) {
      this.openHideAt(own.id, '', actionKey);
    }
  }

  /** "Esconder" of one of the player's creatures, with the creature's own turn options. */
  openHideFor(
    combatantId: string,
    actionKey: string,
    options: GetTurnOptionsResponse | null,
  ): void {
    this.openHideAt(combatantId, '', actionKey, options);
  }

  private openHideAt(
    combatantId: string,
    attemptId: string,
    actionKey = '',
    options: GetTurnOptionsResponse | null = this.deps.options(),
  ): void {
    const e = this.deps.encounter();
    const facts = options?.contestState;
    if (!e) {
      return;
    }
    const action = facts?.hideActions.find((a) => a.key === (actionKey || 'standard:hide'));
    this.track(
      openHideSheet(this.deps.dialog, this.deps.bottomSheet, {
        campaignId: this.deps.campaignId(),
        encounterId: e.id,
        combatantId,
        actionKey: actionKey || action?.key || 'standard:hide',
        economy: action?.economy === ActionEconomy.BONUS_ACTION ? 'Ação bônus' : 'Ação',
        option: facts?.hideOption,
        diceMode: this.deps.diceMode(),
        preference: this.deps.preference(),
        state: this.deps.state(),
        contests: this.contests,
        ...(attemptId ? { attemptId } : {}),
      }),
      (r) => {
        const attempt = this.contests.attempt(r.attemptId);
        if (attempt?.status === HideAttemptStatus.PENDING) {
          this.waiting.add(`hide:${r.attemptId}`);
        }
      },
    );
  }

  /** "Ajudar": an ally's next check of a task, or their first attack on a creature within 1,5 m. */
  openHelp(): void {
    const own = this.deps.own();
    if (own) {
      this.openHelpFor(own.id, this.deps.options());
    }
  }

  /** "Ajudar" of a combatant of the player's: the character, or a creature with its own turn options (a familiar can't
   * attack, but it can take the Help action: SRD 5.1, Find Familiar). */
  openHelpFor(helperId: string, options: GetTurnOptionsResponse | null): void {
    const e = this.deps.encounter();
    if (!e) {
      return;
    }
    this.track(
      openHelpSheet(this.deps.dialog, this.deps.bottomSheet, {
        campaignId: this.deps.campaignId(),
        encounterId: e.id,
        helperId,
        allies: helpAllies(e, helperId),
        targets: helpTargets(options, e, helperId),
        state: this.deps.state(),
      }),
      () => undefined,
    );
  }

  /** "Responder": the question a contest still asks of the player (the sheet was closed without answering). */
  answerPending(): void {
    const c = this.pending()[0];
    const e = this.deps.encounter();
    if (!c || this.sheetOpen() || (e && owesMountQuestion(e))) {
      return;
    }
    if (c.status === ContestStatus.AWAITING_DEFENDER) {
      this.openAnswer(c.id);
    } else {
      this.openContest(c.purpose, c.initiatorId, c.id);
    }
  }

  /** A contest that still waits when its sheet closes comes back with the result. */
  private waitFor(kind: 'contest' | 'answer', contestId: string): void {
    const key = `${kind}:${contestId}`;
    const c = this.contests.contest(contestId);
    this.waiting.delete(key);
    if (c && !isSettled(c)) {
      this.waiting.add(key);
    }
  }
}
