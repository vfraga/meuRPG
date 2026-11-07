import {
  ChangeDetectionStrategy,
  Component,
  computed,
  ElementRef,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
  viewChild,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  AreaPlacement,
  type Combatant,
  type Encounter,
  type PendingDamage,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type Creature,
  type CreatureAction,
  SpellSaveSuccess,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { isHostile } from '../../../../core/combat/attack-flow';
import { CombatUndone } from '../../../../core/combat/combat-undone';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { isDown } from '../../../../core/combat/combat-view';
import {
  actionLines,
  actionName,
  areaPlacementOf,
  asksSave,
  classNote,
  limitView,
  resultNotes,
  showsText,
  usableActions,
  targetLines,
  usageText,
} from '../../../../core/combat/creature-action';
import { CheckBox } from '../../../../shared/check-box/check-box';
import type { CreatureActionOutcome } from '../area-cast/master-creature-area';
import { PendingDamages } from './pending-damages';

/** "Usar" on an action placed on the map: the page opens the picker and answers with what the action did (or `null`: nothing was used). */
export interface CreatureAreaRequest {
  readonly action: CreatureAction;
  readonly placement: AreaPlacement;
  readonly done: (outcome: CreatureActionOutcome | null) => void;
}

/**
 * The actions of the NPC's stat block that are no attack roll (W7-M): a breath weapon, a gaze, a presence, or an action with
 * no roll. Each shows what the app reads of it in Portuguese (the saving throw and its DC, the damage, the area, the
 * condition) and, when the text says more than the app computes, the SRD text as a reminder. "Usar" has the server roll each
 * target's save; on a map an action with a shape asks the master where to place it, in a theatre combat the master names the
 * targets. The result stays under the list, and the damage it opened is applied right there (`PendingDamages`).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-creature-actions',
  imports: [CheckBox, MatButtonModule, MatIconModule, PendingDamages],
  templateUrl: './creature-actions.html',
  styleUrl: './creature-actions.scss',
})
export class CreatureActions {
  private readonly api = inject(CombatClient);

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  readonly subject = input.required<Combatant>();
  readonly state = input.required<CombatState>();
  /** The NPC's stat block (`null` until read): its actions with a saving throw or no roll are listed. */
  readonly creature = input<Creature | null>(null);
  /** The damage waiting for the master, from the turn's options: the part the last use opened shows under its result. */
  readonly pendings = input<readonly PendingDamage[]>([]);
  /** Where the stat block is: `loading` shows a skeleton, `failed` the notice with "Tentar de novo". */
  readonly blockState = input<'idle' | 'loading' | 'failed' | 'ready'>('ready');
  /** "Tentar de novo" on the stat block that could not be read. */
  readonly retry = output<void>();
  private readonly title = viewChild<ElementRef<HTMLElement>>('title');
  /** The combat has no map: the master names the targets, even for an area. */
  readonly theatre = input(false);
  /** The page's own call (the turn passing) is in flight. */
  readonly turnBusy = input(false);

  /** The ids of the damage the last use opened (none once the turn moves on): the card leaves it out of its own list. */
  readonly damageOpened = output<readonly string[]>();
  /** "Usar" on an action with a shape on a map. */
  readonly pickArea = output<CreatureAreaRequest>();
  /** The damage was applied or discarded under the result. */
  readonly settledNote = output<void>();

  protected readonly actions = computed(() => usableActions(this.creature()?.actions));
  private readonly undone = inject(CombatUndone);
  /** What the last use did: its result stays under the list until the turn changes or the use is undone. */
  protected readonly outcome = signal<CreatureActionOutcome | null>(null);
  protected readonly busy = signal(false);
  protected readonly error = signal('');
  /** The creatures the actions that name targets go to: the hostile ones to start with, the master changes them. */
  protected readonly chosen = signal<ReadonlySet<string>>(new Set());
  private pickedByHand = false;

  /** Everyone who can be named: not the creature itself, not who is down. */
  protected readonly candidates = computed(() =>
    this.encounter().combatants.filter((c) => c.id !== this.subject().id && !isDown(c)),
  );
  protected readonly labels = computed(
    () => new Map(this.encounter().combatants.map((c) => [c.id, c.label])),
  );
  /** Some action needs the master to name targets. */
  protected readonly needsTargets = computed(() =>
    this.actions().some((a) => this.takesTargets(a)),
  );
  /** The creature keeps what it spent after the combat (a persistent NPC's uses a day or rest action). */
  protected readonly keeps = computed(() => this.subject().limitedActions.some((l) => l.kept));
  protected readonly spent = computed(() => this.subject().actionUsed);
  protected readonly cards = computed(() =>
    this.actions().map((a) => ({
      action: a,
      name: actionName(a),
      lines: actionLines(a),
      note: classNote(a),
      text: showsText(a) ? a.text : '',
      usage: usageText(a.usage),
      limit: this.limitOf(a),
      why: this.whyNot(a),
    })),
  );
  protected readonly shown = computed(() => {
    const o = this.outcome();
    if (!o) {
      return null;
    }
    const half = o.action.save?.onSuccess === SpellSaveSuccess.HALF;
    const ids = new Set(o.result.pendingDamages.map((p) => p.id));
    return {
      name: actionName(o.action),
      reminder: o.result.reminder,
      lines: targetLines(o.result, this.labels(), half),
      notes: resultNotes(o.result),
      pendings: this.pendings().filter((p) => ids.has(p.id)),
    };
  });

  constructor() {
    // The targets start on the hostile creatures (the party against the enemies) until the master picks his own.
    effect(() => {
      const side = this.subject().side;
      const ids = this.candidates()
        .filter((c) => isHostile(side, c.side))
        .map((c) => c.id);
      untracked(() => {
        if (this.pickedByHand) {
          // Keep his picks, drop who left the list.
          const alive = new Set(this.candidates().map((c) => c.id));
          this.chosen.update((s) => new Set([...s].filter((id) => alive.has(id))));
          return;
        }
        this.chosen.set(new Set(ids));
      });
    });
    // A result belongs to its turn: a new creature or round starts clean, and "Desfazer última ação" takes it back.
    let turnOf = '';
    let undoneSeen = this.undone.count();
    effect(() => {
      const turn = `${this.subject().id}:${this.encounter().round}`;
      const undone = this.undone.count();
      if (turn !== turnOf || undone !== undoneSeen) {
        const newTurn = turn !== turnOf;
        turnOf = turn;
        undoneSeen = undone;
        untracked(() => {
          if (newTurn) {
            this.pickedByHand = false;
          }
          this.error.set('');
          this.outcome.set(null);
        });
      }
    });
    effect(() => {
      const ids = this.outcome()?.result.pendingDamages.map((p) => p.id) ?? [];
      this.damageOpened.emit(ids);
    });
  }

  /** An action whose targets the master names: one with a save that has no shape on a map. */
  protected takesTargets(a: CreatureAction): boolean {
    if (!asksSave(a)) {
      return false;
    }
    return this.theatre() || areaPlacementOf(a) === null;
  }

  /** "Tentar de novo": the button goes with the error, so the focus moves to the section's heading first and stays there. */
  protected tryAgain(): void {
    this.title()?.nativeElement.focus();
    this.retry.emit();
  }

  /** The actions whose SRD text is open: it is a reminder, folded behind the document icon beside the name. */
  protected readonly textOpen = signal<ReadonlySet<string>>(new Set());

  protected toggleText(key: string): void {
    this.textOpen.update((s) => {
      const next = new Set(s);
      if (!next.delete(key)) {
        next.add(key);
      }
      return next;
    });
  }

  /** One filled dot for each use left, one empty for each spent. */
  protected dotsOf(u: { allowed: number; left: number }): boolean[] {
    return Array.from({ length: u.allowed }, (_, i) => i < u.left);
  }

  /** The state of the action's limit, from the server's copy for the master and the controller (`''` tag and no uses: no limit). */
  private limitOf(a: CreatureAction) {
    const copy = this.subject().limitedActions.find((l) => l.actionKey === a.key);
    return limitView(copy, this.subject().label);
  }

  /** Why "Usar" cannot be pressed now, or `''`. */
  private whyNot(a: CreatureAction): string {
    if (this.spent()) {
      return 'A ação deste turno já foi usada.';
    }
    const limit = this.limitOf(a).why;
    if (limit) {
      return limit;
    }
    if (this.takesTargets(a) && this.chosen().size === 0) {
      return 'Escolha ao menos um alvo.';
    }
    return '';
  }

  protected toggle(id: string): void {
    this.pickedByHand = true;
    this.chosen.update((s) => {
      const next = new Set(s);
      if (!next.delete(id)) {
        next.add(id);
      }
      return next;
    });
  }

  protected async use(a: CreatureAction): Promise<void> {
    if (this.busy() || this.turnBusy() || this.whyNot(a)) {
      return;
    }
    this.error.set('');
    const placement = areaPlacementOf(a);
    if (
      !this.theatre() &&
      asksSave(a) &&
      placement !== null &&
      placement !== AreaPlacement.CASTER
    ) {
      this.busy.set(true);
      this.pickArea.emit({
        action: a,
        placement,
        done: (outcome) => {
          this.busy.set(false);
          if (outcome) {
            this.outcome.set(outcome);
          }
        },
      });
      return;
    }
    this.busy.set(true);
    try {
      const targetIds = this.takesTargets(a) ? [...this.chosen()] : [];
      const res = await this.api.casts.useCreatureAction(
        this.campaignId(),
        this.encounter().id,
        this.subject().id,
        a.key,
        targetIds,
      );
      this.state().apply(res.encounter);
      this.outcome.set({ action: a, result: res.result });
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'usar a ação'));
    } finally {
      this.busy.set(false);
    }
  }
}
