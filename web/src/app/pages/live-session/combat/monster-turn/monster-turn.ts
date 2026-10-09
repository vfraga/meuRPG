import {
  ChangeDetectionStrategy,
  Component,
  effect,
  inject,
  input,
  signal,
  untracked,
} from '@angular/core';

import type { Combatant, Encounter } from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { CreatureTurn } from '../../../../../gen/meurpg/play/v1/creatures_pb';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import type { CombatState } from '../../../../core/combat/combat-state';
import { CreatureClient } from '../../../../core/combat/creature-client';
import { MonsterActions } from './monster-actions';
import { MonsterSheet } from './monster-sheet';

/**
 * A monster's turn for the master (W7-M): the sheet of its stat block beside its actions. It reads
 * `GetCreatureTurn` again whenever the combat changes (an action used, a recharge rolled, a
 * Legendary Resistance spent), so the buttons and the limits are always the server's.
 */
@Component({
  changeDetection: ChangeDetectionStrategy.OnPush,
  selector: 'app-monster-turn',
  imports: [MonsterActions, MonsterSheet],
  template: `
    @if (turn(); as t) {
      <div class="grid">
        <app-monster-sheet [turn]="t" [subject]="subject()" />
        <app-monster-actions
          [campaignId]="campaignId()"
          [encounter]="encounter()"
          [subject]="subject()"
          [turn]="t"
          [state]="state()"
        />
      </div>
    }
    @if (error()) {
      <p class="error" role="alert">{{ error() }}</p>
    }
  `,
  styles: `
    :host {
      display: block;
    }

    .grid {
      display: grid;
      grid-template-columns: minmax(0, 1fr);
      gap: 16px;
    }

    .error {
      margin: 8px 0 0;
      color: var(--mr-danger-ink);
    }
  `,
})
export class MonsterTurn {
  private readonly api = inject(CreatureClient);

  readonly campaignId = input.required<string>();
  readonly encounter = input.required<Encounter>();
  readonly subject = input.required<Combatant>();
  readonly state = input.required<CombatState>();

  protected readonly turn = signal<CreatureTurn | null>(null);
  protected readonly error = signal('');
  private ticket = 0;

  constructor() {
    effect(() => {
      // Each change of the combat reads the turn again; an older answer never replaces a newer one.
      const e = this.encounter();
      const who = this.subject().id;
      void e.revision;
      const campaignId = this.campaignId();
      const mine = ++this.ticket;
      void this.api.turn(campaignId, e.id, who).then(
        (t) => {
          if (mine === this.ticket) {
            untracked(() => {
              this.turn.set(t);
              this.error.set('');
            });
          }
        },
        (err: unknown) => {
          if (mine === this.ticket) {
            untracked(() => this.error.set(combatErrorMessage(err, 'abrir a ficha do monstro')));
          }
        },
      );
    });
  }
}
