import { Component, computed, input, output } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type Combatant,
  CombatantState,
  type HeldRollOffer,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { effectivePreference } from '../../../../core/campaigns/dice-labels';
import type { AttackDie } from '../../../../core/combat/combat-client-types';
import { countText } from '../../../../core/combat/death-saves';
import type { HeldAnswer } from '../../../../core/held-rolls/held-roll-offer';
import { HeldRollPrompt } from '../../../../shared/held-roll-prompt/held-roll-prompt';
import { EndTurn } from '../turn-panel/end-turn';
import { RollPicker } from '../../../../shared/roll-picker/roll-picker';
import { DeathMarks } from './death-marks';

/**
 * What a player does while down (E6-13, RN-03): the death saves on their turn.
 * "Testes contra a morte" has the two rows of marks ("Sucessos 0 de 3",
 * "Falhas 1 de 3"), the one filled button "Rolar teste contra a morte" (the
 * d20 in the app or typed, RN-18) with "10 ou mais é um sucesso" under it, and
 * the small print; "Enquanto estiver com 0 PV" lists what they cannot do and the
 * one thing that brings them back. Once the save of the turn is rolled the
 * button gives way to the result, announced in a live region ("Teste contra a
 * morte: 1d20 (14) = 14. Sucesso."), and "Encerrar turno" stands in its place;
 * a stable character has nothing to roll. The master is the only one told a
 * character is dying: here the counts are all there is.
 */
@Component({
  selector: 'app-death-saves',
  imports: [DeathMarks, EndTurn, HeldRollPrompt, MatIconModule, RollPicker],
  template: `
    <section class="card" aria-labelledby="ds-title">
      <h3 class="card__title" id="ds-title">Testes contra a morte</h3>
      @if (ownerOnly()) {
        <!-- The table hides the death saves (RN-24): only the owner and the master see these marks. -->
        <p class="private" data-testid="death-private"><mat-icon aria-hidden="true">visibility_off</mat-icon>Só você e o mestre</p>
      }
      <div class="row">
        <span class="row__name"><b>Sucessos</b><small>{{ count(own().deathSuccesses) }}</small></span>
        <app-death-marks kind="success" [count]="own().deathSuccesses" />
      </div>
      <div class="row">
        <span class="row__name"><b>Falhas</b><small>{{ count(own().deathFailures) }}</small></span>
        <app-death-marks kind="failure" [count]="own().deathFailures" />
      </div>

      @if (held(); as question) {
        <!-- The d20 is rolled and kept: a halfling's 1 asks Lucky first, then a Bardic Inspiration die; nothing is written until the player answers (SRD 5.1). -->
        <app-held-roll-prompt
          [offer]="question"
          look="card"
          [canApp]="canApp()"
          [canType]="canType()"
          [preferApp]="preferApp()"
          [busy]="busy()"
          (answer)="answer.emit($event)"
        />
      } @else if (own().deathSaveDue) {
        <app-roll-picker
          [compact]="true"
          [canApp]="canApp()"
          [canType]="canType()"
          [preferApp]="preferApp()"
          label="Role 1d20 para o teste contra a morte"
          hint="Role o seu dado e digite o número que saiu (1 a 20)."
          totalNote="Teste contra a morte"
          [busy]="busy()"
          appLabel="Rolar teste contra a morte"
          (app)="roll.emit({ inApp: true })"
          (typed)="roll.emit({ face: $event })"
        />
        <p class="rule"><b>Role 1d20: 10 ou mais é um sucesso</b></p>
      } @else {
        @if (result()) {
          <p class="result" role="status" aria-live="polite">{{ result() }}</p>
        }
        @if (stable()) {
          <p class="result">Estável: não rola mais testes contra a morte. Fica com 0 PV até receber cura.</p>
        }
        <app-end-turn [own]="spent" [busy]="busy()" [block]="true" (endTurn)="endTurn.emit()" />
      }

      <ul class="print">
        <li>3 sucessos: você se estabiliza.</li>
        <li>3 falhas: o mestre confirma a morte.</li>
        <li>20 no dado: você volta com 1 PV. 1 no dado: conta duas falhas.</li>
        <li>Dano em quem está com 0 PV conta como uma falha.</li>
      </ul>
    </section>

    <section class="card" aria-labelledby="ds-down">
      <h3 class="card__title" id="ds-down">Enquanto estiver com 0 PV</h3>
      <ul class="cannot">
        @for (t of cannot; track t) {
          <li>
            <mat-icon aria-hidden="true">block</mat-icon><span>{{ t }}</span><span class="tag">Não pode</span>
          </li>
        }
        <li>
          <mat-icon aria-hidden="true">check</mat-icon><span>Receber cura de um aliado: você volta a agir</span><span class="tag">Pode</span>
        </li>
      </ul>
    </section>
  `,
  styleUrl: './death-saves.scss',
  host: { style: 'display: flex; flex-direction: column; gap: var(--mr-space-4)' },
})
export class DeathSaves {
  /** The player's own combatant, down. */
  readonly own = input.required<Combatant>();
  readonly busy = input(false);
  readonly diceMode = input.required<DiceMode>();
  readonly dicePreference = input.required<DicePreference>();
  /** The sentence of the save just rolled this turn ("Teste contra a morte: ..."). */
  readonly result = input('');
  /** The table hides the death saves from the other players (RN-24): the card says who sees them. */
  readonly ownerOnly = input(false);
  /** The question the death save of this turn waits for, when its d20 was kept for one. */
  readonly held = input<HeldRollOffer | null>(null);

  readonly roll = output<AttackDie>();
  /** The player's answer to the question that waits. */
  readonly answer = output<HeldAnswer>();
  readonly endTurn = output<void>();

  /** Nothing left to do while down: "Encerrar turno" is the one filled button and never asks. */
  protected readonly spent = { actionUsed: true, bonusActionUsed: true };
  protected readonly cannot = ['Atacar ou conjurar magias', 'Mover-se', 'Usar a reação'];
  protected readonly canApp = computed(() => this.diceMode() !== DiceMode.PHYSICAL);
  protected readonly canType = computed(() => this.diceMode() !== DiceMode.APP);
  protected readonly preferApp = computed(
    () => effectivePreference(this.diceMode(), this.dicePreference()) === DicePreference.APP,
  );
  protected readonly stable = computed(() => this.own().state === CombatantState.STABLE);

  protected count(n: number): string {
    return countText(n);
  }
}
