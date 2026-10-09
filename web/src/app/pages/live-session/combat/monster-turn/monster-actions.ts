import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  output,
  signal,
  untracked,
} from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatSelectModule } from '@angular/material/select';

import {
  type Combatant,
  type Encounter,
  CombatantState,
  SaveOutcome,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  type CreatureAction,
  type CreatureActionResult,
  type CreatureTurn,
  CreatureActionKind,
} from '../../../../../gen/meurpg/play/v1/creatures_pb';
import { isHit, outcomeWord } from '../../../../core/combat/attack-flow';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { CreatureClient, newKey } from '../../../../core/combat/creature-client';
import {
  actionButton,
  actionLine,
  rechargeText,
  unavailableWhy,
  usageText,
} from '../../../../core/combat/creature-turn';
import { DamageSteps } from './damage-steps';

/** "passou" / "falhou": a saving throw as the line says it. */
function saveWord(outcome: SaveOutcome): string {
  return outcome === SaveOutcome.SAVED ? 'passou' : 'falhou';
}

/**
 * The actions of the monster on turn (W7-M): each one a button of 44 px with what it does and its
 * limit; "Ataque múltiplo" is a guided sequence (its steps are labels, the server counts them);
 * an attack and a saving throw are made whole by the server (`UseCreatureAction`). A grey button
 * says why in a sentence the screen reader reaches (`aria-describedby`).
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-monster-actions',
  imports: [DamageSteps, MatButtonModule, MatFormFieldModule, MatIconModule, MatSelectModule],
  templateUrl: './monster-actions.html',
  styleUrl: './monster-actions.scss',
})
export class MonsterActions {
  private readonly api = inject(CreatureClient);

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  readonly subject = input.required<Combatant>();
  readonly turn = input.required<CreatureTurn>();
  readonly state = input.required<CombatState>();
  /** An action was used: the page reads the turn again. */
  readonly used = output<void>();

  protected readonly busy = signal(false);
  protected readonly error = signal('');
  protected readonly targetIds = signal<readonly string[]>([]);
  protected readonly last = signal<{ action: string; result: CreatureActionResult } | null>(null);
  private key = newKey();

  /** Who the monster can aim at: whoever is not defeated and is not itself. */
  protected readonly targets = computed(() =>
    this.encounter().combatants.filter(
      (c) => c.id !== this.subject().id && c.state !== CombatantState.DEFEATED,
    ),
  );
  protected readonly rows = computed(() =>
    this.turn().actions.map((a) => ({
      action: a,
      name: a.namePt || a.name,
      line: actionLine(a),
      limit: usageText(a),
      why: this.why(a),
      button: actionButton(a),
      multi: a.kind === CreatureActionKind.MULTIATTACK,
      rechargeRoll: this.rechargeFor(a),
    })),
  );
  protected readonly sequence = computed(() => this.turn().sequence);
  protected readonly lastOutcomes = computed(() => {
    const cast = this.last()?.result.cast;
    return (cast?.targets ?? []).map((t) => ({
      id: t.combatantId,
      label: this.encounter().combatants.find((c) => c.id === t.combatantId)?.label ?? '',
      word: t.save ? saveWord(t.save.outcome) : '',
    }));
  });
  protected readonly immune = computed(() =>
    (this.last()?.result.immuneTargetIds ?? []).map(
      (id) => this.encounter().combatants.find((c) => c.id === id)?.label ?? '',
    ),
  );

  constructor() {
    // A new turn starts clean.
    let who = '';
    effect(() => {
      const id = this.subject().id;
      if (id !== who) {
        who = id;
        untracked(() => {
          this.last.set(null);
          this.error.set('');
          this.targetIds.set([]);
        });
      }
    });
    // A target that left is not kept.
    effect(() => {
      const ids = new Set(this.targets().map((t) => t.id));
      const kept = untracked(() => this.targetIds()).filter((id) => ids.has(id));
      if (kept.length !== untracked(() => this.targetIds()).length) {
        untracked(() => this.targetIds.set(kept));
      }
    });
  }

  private rechargeFor(a: CreatureAction): { word: string; detail: string; roll: number } | null {
    const u = a.usage;
    if (!u || u.rechargeRoll <= 0) {
      return null;
    }
    return { ...rechargeText(u.rechargeRoll, u.rechargeMin), roll: u.rechargeRoll };
  }

  /** What keeps the button grey, or `''`. */
  private why(a: CreatureAction): string {
    const limit = unavailableWhy(a);
    if (limit) {
      return limit;
    }
    if (a.kind === CreatureActionKind.ATTACK && this.targetIds().length !== 1) {
      return 'Escolha um alvo.';
    }
    if (a.kind === CreatureActionKind.SAVE && this.targetIds().length === 0) {
      return 'Escolha quem a ação atinge.';
    }
    if (this.subject().actionUsed && a.kind !== CreatureActionKind.TEXT_ONLY) {
      return 'A ação deste turno já foi usada.';
    }
    return '';
  }

  protected readonly saveWord = saveWord;
  protected readonly outcomeWord = outcomeWord;
  protected readonly isHit = isHit;

  /** The Portuguese name of the action a step makes, or the step's own name. */
  protected stepName(actionKey: string, name: string): string {
    const a = this.turn().actions.find((x) => x.key === actionKey);
    return a?.namePt || name;
  }

  /** The steps of a routine one by one ("Garra", "Garra"), the way the sequence will list them. */
  protected previewSteps(r: CreatureAction): string[] {
    return (r.routines[0]?.steps ?? []).flatMap((st) =>
      Array.from({ length: Math.max(st.count, 1) }, () => this.stepName(st.actionKey, st.name)),
    );
  }

  protected pickTargets(ids: string[]): void {
    this.targetIds.set(ids);
    this.key = newKey();
  }

  protected async use(a: CreatureAction): Promise<void> {
    if (this.busy() || this.why(a)) {
      return;
    }
    this.busy.set(true);
    this.error.set('');
    const targets =
      a.kind === CreatureActionKind.ATTACK || a.kind === CreatureActionKind.SAVE
        ? this.targetIds()
        : [];
    try {
      const res = await this.api.useAction(
        this.campaignId(),
        this.encounter().id,
        this.subject().id,
        a.key,
        targets,
        this.key,
      );
      this.key = newKey();
      this.state().apply(res.encounter);
      this.last.set({ action: a.namePt || a.name, result: res.result });
      this.used.emit();
    } catch (err) {
      this.error.set(combatErrorMessage(err, 'usar a ação'));
    } finally {
      this.busy.set(false);
    }
  }
}
