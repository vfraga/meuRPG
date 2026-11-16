import { ChangeDetectionStrategy, Component, computed, input, output } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';

import {
  type Combatant,
  CombatantKind,
  type GetTurnOptionsResponse,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../../../../core/format/article';
import { combatantInitial } from '../../../../core/combat/combat-view';
import { pairActions, type PairAction } from '../../../../core/combat/pair-actions';
import { metersFixed } from '../../../../core/units';
import { PHONE_QUERY, mediaQuery } from '../../../../shared/map-view/media-query';
import { CombatantToken } from '../../../../shared/combatant-token/combatant-token';
import { EconomyShapes, economyShapes } from '../../../../shared/economy-shapes/economy-shapes';

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
    <section
      class="pair"
      [attr.aria-labelledby]="phone() ? null : 'pair-title'"
      [attr.aria-label]="phone() ? rider().label + ' e ' + mount().label + ', turno conjunto' : null"
    >
      @if (!phone()) {
        <!-- On a phone the card comes under the rider's own shapes and "Mover": the mount's line comes alone (588-05). -->
        <p class="pair__head" id="pair-title">
          <mat-icon aria-hidden="true">link</mat-icon><b>{{ rider().label }} e {{ mount().label }}</b
          ><span class="pair__joint"> · turno conjunto</span>
        </p>
        <div class="pair__line pair__rider">
          <app-combatant-token [initial]="riderInitial()" [mine]="true" [size]="28" />
          <span class="pair__name">{{ rider().label }}</span>
          <app-economy-shapes variant="inline" [shapes]="riderShapes()" [label]="'O que ' + rider().label + ' tem'" />
        </div>
      }
      <div class="pair__line pair__mount">
        <app-combatant-token [initial]="initial()" [npc]="npc()" [creature]="!npc()" [size]="28" />
        <span class="pair__name">{{ mount().label }}</span>
        @if (shapes(); as s) {
          <app-economy-shapes variant="inline" [shapes]="s" [label]="'O que ' + theMount() + ' tem'" />
        }
        <!-- The region is in the card from the start, with or without words, so "Carregando…" is announced when it comes. -->
        <span class="pair__loading mr-muted" role="status">{{ shapes() ? '' : 'Carregando…' }}</span>
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
            [attr.aria-label]="a.name + ' ' + ofMount()"
            [attr.aria-describedby]="a.off && why() ? 'pair-why' : null"
            [attr.data-action]="a.key"
            (click)="press(a)"
          >
            <mat-icon aria-hidden="true">{{ a.icon }}</mat-icon>{{ a.name }}
          </button>
        }
      </div>
      <!-- The reason arrives after the card is drawn: the region is there from the start, so it is announced. -->
      <div class="mr-live" role="status">
        @if (why(); as text) {
          <p class="pair__why" id="pair-why"><mat-icon aria-hidden="true">block</mat-icon>{{ text }}</p>
        }
      </div>
      @if (!why()) {
        @if (phone()) {
          <p class="pair__note"><mat-icon aria-hidden="true">link</mat-icon>Adestrada: só estas três. "Mover" leva os dois.</p>
        } @else {
          <p class="pair__note">
            Montaria adestrada: só estas três ações; a reação fica livre. Encerrar turno encerra a vez dos dois.
          </p>
        }
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
  /** The phone's card comes under the rider's own shapes and "Mover": its last line is the short one (588-05). */
  protected readonly phone = mediaQuery(PHONE_QUERY);
  /** The key of the action pressed ("standard:dash"). */
  readonly act = output<string>();

  protected readonly initial = computed(() => combatantInitial(this.mount().label));
  protected readonly riderInitial = computed(() => combatantInitial(this.rider().label));
  /** An NPC's token for an NPC; a player's own steed (Find Steed) is a creature. */
  protected readonly npc = computed(() => this.mount().kind === CombatantKind.NPC);
  /** "o Cavalo de guerra", "a Égua". */
  protected readonly theMount = computed(
    () => `${article(this.mount().label)} ${this.mount().label}`,
  );
  /** "do Cavalo de guerra": whose action a button is, in its accessible name. */
  protected readonly ofMount = computed(() => `d${this.theMount()}`);
  private readonly view = computed(() => pairActions(this.options(), this.rider(), this.mount()));
  protected readonly actions = computed(() => this.view().actions);
  protected readonly why = computed(() => this.view().why);
  /** The rider's own action, bonus action and reaction, on one line (588-04). */
  protected readonly riderShapes = computed(() => economyShapes(this.rider()));
  /** The mount's action and the metres it has left; `null` until its options are read (nothing is made up meanwhile). */
  protected readonly shapes = computed(() => {
    const res = this.options();
    if (!res) {
      return null;
    }
    const economy = res.options?.economy;
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
