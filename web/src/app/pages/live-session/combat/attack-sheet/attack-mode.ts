import { computed } from '@angular/core';

import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { ModeChoice } from '../../../../core/combat/combat-client-types';
import { d20Count, modeStatus, orNormal } from '../../../../core/combat/roll-mode';
import { modeChoiceOf } from './attack-sheet-view';
import type { AttackState } from './attack-state';
import type { ModeRequests } from './mode-requests';

/** The roll mode of the d20: what the server suggests for the target, what the player picked or the master decided. */
export class AttackMode {
  readonly approval: 'free' | 'request';
  /** The target's circumstances from the options: the suggested mode and why. An opportunity attack has none. */
  private readonly reach = computed(() => {
    const id = this.s.targetId();
    return this.s.data.targets.find((t) => t.combatantId === id) ?? null;
  });
  readonly hasMode = computed(() => !this.s.data.opportunity && this.reach() !== null);
  readonly suggested = computed(() => orNormal(this.reach()?.rollMode ?? RollMode.NORMAL));
  readonly sources = computed(() => this.reach()?.sources ?? []);
  readonly criticalOnHit = computed(() => this.reach()?.criticalOnHit ?? false);
  readonly chosen = computed<RollMode>(() => this.requests.picked() ?? this.suggested());
  /** The mode the d20 rolls with: the master's decision on a request, else the player's choice. */
  readonly rollMode = computed(() => this.requests.answered()?.decidedMode ?? this.chosen());
  private readonly state = computed(() =>
    modeStatus(
      this.approval,
      this.suggested(),
      this.chosen(),
      this.requests.reason(),
      this.requests.request(),
    ),
  );
  /** The dice may be rolled: the mode is settled (or the server picks it, for an opportunity attack). */
  readonly ready = computed(() => !this.hasMode() || this.state() === 'ready');
  readonly faceCount = computed(() => (this.hasMode() ? d20Count(this.rollMode()) : 1));
  readonly combine = computed(() =>
    this.rollMode() === RollMode.DISADVANTAGE ? 'lower' : 'higher',
  );

  constructor(
    private readonly s: AttackState,
    private readonly requests: ModeRequests,
  ) {
    this.approval = s.data.master ? 'free' : 'request';
  }

  set(mode: RollMode): void {
    this.requests.picked.set(mode);
    this.s.error.set('');
  }

  /** What the roll sends about the mode: nothing when the suggestion stands, else the mode, why, and the answered request. */
  choice(): ModeChoice | undefined {
    return modeChoiceOf({
      hasMode: this.hasMode(),
      answered: this.requests.answered(),
      chosen: this.chosen(),
      suggested: this.suggested(),
      reason: this.requests.reason(),
    });
  }
}
