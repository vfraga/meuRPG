import { computed } from '@angular/core';

import { DiceMode } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { d20Count, modeStatus, orNormal } from '../../../../core/combat/roll-mode';
import type { RollField } from '../../../../shared/multi-roll/multi-roll';
import type { CastChoices, CastHooks } from './cast-choices';
import type { CastState } from './cast-state';

/** The two d20 a spell attack with advantage or disadvantage asks for, typed. */
export const D20_FIELDS: readonly RollField[] = [
  { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
  { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
];

/** The roll mode of a spell attack's d20: what the targets suggest, what the caster picks. */
export class CastMode {
  readonly approval: 'free' | 'blocked';
  /** The targets chosen for a spell attack, with the circumstances of each. */
  private readonly attacked = computed(() =>
    (this.s.data.targets?.targets ?? []).filter((t) => this.s.chosen().includes(t.combatantId)),
  );
  /** The targets suggest different modes: the pick, if any, counts for all of them. */
  readonly mixed = computed(
    () => new Set(this.attacked().map((t) => orNormal(t.rollMode))).size > 1,
  );
  readonly suggested = computed(() => orNormal(this.attacked()[0]?.rollMode ?? RollMode.NORMAL));
  readonly sources = computed(() =>
    this.attacked().length === 1 ? this.attacked()[0].sources : [],
  );
  readonly chosen = computed(() => this.s.picked() ?? this.suggested());
  private readonly state = computed(() =>
    this.mixed() && this.s.picked() === null
      ? 'ready'
      : modeStatus(this.approval, this.suggested(), this.chosen(), this.s.reason(), null),
  );
  /** The d20 of the attack may be rolled: the mode is settled. */
  readonly ready = computed(() => this.state() === 'ready');
  readonly faceCount = computed(() => d20Count(this.chosen()));
  readonly combine = computed(() => (this.chosen() === RollMode.DISADVANTAGE ? 'lower' : 'higher'));
  /** The d20 of a spell attack may be typed only for a single target (the server). */
  readonly canType = computed(
    () =>
      this.s.data.diceMode !== DiceMode.APP &&
      (this.choices.kind() !== 'attack' || this.s.chosen().length <= 1),
  );

  constructor(
    private readonly s: CastState,
    private readonly choices: CastChoices,
    private readonly hooks: CastHooks,
  ) {
    this.approval = s.data.master ? 'free' : 'blocked';
  }

  set(mode: RollMode): void {
    this.s.picked.set(mode);
    this.hooks.newCast();
  }
}
