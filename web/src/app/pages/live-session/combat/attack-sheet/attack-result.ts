import { Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { AttackRoll, PendingDamage } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../../../../core/combat/combat-log';
import { coverBonusText, degreeWord, sourceWord } from '../../../../core/combat/cover';

/**
 * What the attack sheet has shown so far, as a list of done steps (E6-07,
 * last frame): the target, the d20 with its formula and the outcome pill
 * ("Acertou", "Crítico" or "Errou"), and, once rolled, the damage. A live
 * region, so the roll is read out when it arrives. Never an armor class.
 */
@Component({
  selector: 'app-attack-result',
  imports: [MatIconModule],
  template: `
    <div class="result" role="status" aria-live="polite">
      <div class="part">
        <mat-icon class="part__check" aria-hidden="true">check</mat-icon>
        <div class="part__body">
          <span class="part__cap">Alvo</span>
          <span class="part__line"><b>{{ targetLabel() }}</b></span>
        </div>
      </div>
      @if (roll(); as r) {
        <div class="part">
          <mat-icon class="part__check" aria-hidden="true">check</mat-icon>
          <div class="part__body">
            <span class="part__cap">Rolar</span>
            <span class="part__row">
              <span class="part__num">{{ r.d20?.total }}</span>
              <span class="part__formula">{{ d20Formula() }}{{ r.d20?.physical ? ' · dado físico' : '' }}</span>
              @if (outcome(); as o) {
                <span class="pill" [class.pill--miss]="!o.hit">
                  <mat-icon aria-hidden="true">{{ o.hit ? 'check' : 'close' }}</mat-icon>{{ o.word }}
                </span>
              }
            </span>
            @if (coverLine()) {
              <span class="part__line">{{ coverLine() }}</span>
            }
          </div>
        </div>
      }
      @if (showDamage()) {
        <div class="part">
          <mat-icon class="part__check" aria-hidden="true">check</mat-icon>
          <div class="part__body">
            <span class="part__cap">Dano</span>
            @if (damage(); as d) {
              <span class="part__row">
                <span class="part__num">{{ d.amount }}</span>
                <span class="part__formula">{{ damageLine() }}{{ d.roll?.physical ? ' · dado físico' : '' }}</span>
              </span>
            } @else if (waiting()) {
              <span class="part__line">Esperando a reação do alvo.</span>
            } @else {
              <span class="part__line">Sem dano: o ataque errou.</span>
            }
          </div>
        </div>
      }
    </div>
  `,
  styleUrl: './attack-result.scss',
})
export class AttackResult {
  readonly targetLabel = input.required<string>();
  readonly roll = input<AttackRoll | null>(null);
  readonly d20Formula = input('');
  readonly outcome = input<{ word: string; hit: boolean } | null>(null);
  /** The Dano step is finished: show it (a miss says there is none). */
  readonly showDamage = input(false);
  readonly damage = input<PendingDamage | null>(null);
  /** The hit's damage waits for the target's reaction (Escudo): it is not a miss. */
  readonly waiting = input(false);
  readonly damageLine = input('');
  /** "O Goblin 2 estava com meia cobertura.": says why a miss missed, never by how much (RN-20). */
  protected readonly coverLine = computed(() => {
    const r = this.roll();
    const word = r ? degreeWord(r.cover).toLowerCase() : '';
    if (!r || !word) {
      return '';
    }
    // "Meia cobertura (marcada pelo mestre): +2 na CA.", the degree, where it came from and what it added, never the armor class (RN-20).
    const from = sourceWord(r.coverSource);
    const bonus = coverBonusText(r.cover);
    return `${capitalize(`${article(this.targetLabel())} ${this.targetLabel()}`)} estava com ${word}${from ? ` (${from})` : ''}${bonus ? `: ${bonus}` : ''}.`;
  });
}

function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1);
}
