import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import type {
  Combatant,
  GetTurnOptionsResponse,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { combatantInitial } from '../../../../core/combat/combat-view';
import { pairActions, type PairAction } from '../../../../core/combat/pair-actions';
import { metersFixed } from '../../../../core/units';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { EconomyShapes } from '../../../../shared/economy-shapes/economy-shapes';

/**
 * The rider's turn with the mount it directs (588-04 desktop, 588-05 phone): one card in the accent frame, with the mount's
 * line (its action and the metres it has left, from its own `GetTurnOptions`) and the three actions it takes while it carries
 * someone, "Disparada", "Desengajar" and "Esquivar" (`TakeAction` with the mount's id). It stands in for a creature tab: the
 * pair has one "Mover" (the rider's, which moves the pair) and one "Encerrar turno" (it ends both parts). A refused action
 * keeps its place, dashed, with the reason under the three ("Caído: o mestre conduz o Cavalo de guerra.", 588-10 card 5).
 */
@Component({
  selector: 'app-pair-card',
  imports: [CombatantToken, EconomyShapes, MatButtonModule, MatIconModule],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <section class="pair" aria-labelledby="pair-title">
      <p class="pair__head" id="pair-title">
        <mat-icon aria-hidden="true">link</mat-icon><b>{{ rider().label }} e {{ mount().label }}</b
        ><span class="pair__joint"> · turno conjunto</span>
      </p>
      <div class="pair__mount">
        <app-combatant-token [initial]="initial()" [npc]="true" [size]="28" />
        <span class="pair__name">{{ mount().label }}</span>
        <app-economy-shapes variant="inline" [shapes]="shapes()" [label]="'O que ' + mount().label + ' tem'" />
      </div>
      <div class="pair__actions">
        @for (a of actions(); track a.key) {
          <button
            matButton="outlined"
            type="button"
            class="pair__act"
            [class.mr-button--off]="a.off"
            [disabled]="a.off || busy()"
            disabledInteractive
            [attr.aria-describedby]="a.off ? 'pair-why' : null"
            [attr.data-action]="a.key"
            (click)="press(a)"
          >
            <mat-icon aria-hidden="true">{{ a.icon }}</mat-icon>{{ a.name }}
          </button>
        }
      </div>
      @if (why(); as text) {
        <p class="pair__why" id="pair-why"><mat-icon aria-hidden="true">block</mat-icon>{{ text }}</p>
      } @else {
        <p class="pair__note">
          Montaria adestrada: só estas três ações; a reação fica livre. "Encerrar turno" encerra a vez dos dois.
        </p>
      }
    </section>
  `,
  styleUrl: './pair-card.scss',
})
export class PairCard {
  readonly rider = input.required<Combatant>();
  readonly mount = input.required<Combatant>();
  /** The mount's own `GetTurnOptions`, read for the rider's player (the three actions and the mount's economy only). */
  readonly options = input<GetTurnOptionsResponse | null>(null);
  readonly busy = input(false);
  /** The key of the action pressed ("standard:dash"). */
  readonly act = output<string>();

  protected readonly initial = computed(() => combatantInitial(this.mount().label));
  private readonly view = computed(() => pairActions(this.options(), this.rider(), this.mount()));
  protected readonly actions = computed(() => this.view().actions);
  protected readonly why = computed(() => this.view().why);
  protected readonly shapes = computed(() => {
    const economy = this.options()?.options?.economy;
    const used = economy?.action?.used ?? this.mount().actionUsed;
    const left = metersFixed(economy?.movement?.leftFt ?? this.mount().movementLeftFt);
    return [
      {
        kind: 'action' as const,
        text: 'Ação',
        name: used ? 'Ação usada' : 'Ação disponível',
        spent: used,
      },
      { kind: 'move' as const, text: left, name: `${left} de movimento`, spent: false },
    ];
  });

  protected press(a: PairAction): void {
    if (!a.off && !this.busy()) {
      this.act.emit(a.key);
    }
  }
}
