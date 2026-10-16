import { Component, computed, input } from '@angular/core';
import { MatIconModule } from '@angular/material/icon';

import type { AttackRoll, PendingDamage } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../../../../core/combat/combat-log';
import { coverBonusText, degreeWord, sourceWord } from '../../../../core/combat/cover';
import { luckyText } from '../../../../core/combat/lucky';
import { DamageBreakdown } from '../damage-parts/damage-breakdown';
import { D20Faces } from '../roll-mode/d20-faces';
import { upFirst } from '../../../../core/format/text';

/**
 * What the attack sheet has shown so far, as a list of done steps (E6-07,
 * last frame): the target, the d20 with its formula and the outcome pill
 * ("Acertou", "Crítico" or "Errou"), and, once rolled, the damage. A live
 * region, so the roll is read out when it arrives. Never an armor class.
 */
@Component({
  selector: 'app-attack-result',
  imports: [D20Faces, DamageBreakdown, MatIconModule],
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
                  <mat-icon aria-hidden="true">{{ o.hit ? 'check' : 'close' }}</mat-icon>{{ o.word }}{{ o.natural ? ' · ' + o.natural : '' }}
                </span>
              }
            </span>
            <app-d20-faces [roll]="r.d20" [mode]="r.mode" [suggested]="r.suggestedMode" [reason]="r.modeReason" [sources]="r.sources" />
            @if (luckyLine()) {
              <span class="part__line">{{ luckyLine() }}</span>
            }
            @if (r.criticalOnHit) {
              <span class="part__line">Acerto crítico automático (a 1,5 m de alvo paralisado ou inconsciente)</span>
            }
            @if (asking()) {
              <span class="part__line">O mestre ainda não disse se acertou.</span>
            }
            @if (bonusLine()) {
              <span class="part__line">{{ bonusLine() }}</span>
            }
            @if (held(); as h) {
              <span class="part__line"><b>{{ h.title }}.</b> {{ h.detail }}</span>
            }
            @if (coverLine()) {
              <span class="part__line">{{ coverLine() }}</span>
            }
          </div>
        </div>
      }
      @if (showDamage() && !held() && !asking()) {
        <div class="part">
          <mat-icon class="part__check" aria-hidden="true">check</mat-icon>
          <div class="part__body">
            <span class="part__cap">Dano</span>
            @if (damage(); as d) {
              <span class="part__row">
                <span class="part__num">{{ d.amount }}</span>
                <span class="part__formula">{{ damageLine() }}{{ d.roll?.physical ? ' · dado físico' : '' }}</span>
              </span>
              @if (breakdown()) {
                <app-damage-breakdown
                  [rolls]="d.partRolls"
                  [steps]="d.steps"
                  [amount]="d.amount"
                  [amountAfterSteps]="d.amountAfterSteps"
                />
              }
            } @else if (waiting()) {
              <span class="part__line">Esperando o mestre.</span>
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
  /** `natural`: "1 natural" beside "Errou" (a natural 1 always misses, SRD 5.1), empty otherwise. */
  readonly outcome = input<{ word: string; hit: boolean; natural?: string } | null>(null);
  /** A reaction holds the roll: what the combat waits for, instead of an outcome. */
  readonly held = input<{ title: string; detail: string } | null>(null);
  /** The Dano step is finished: show it (a miss says there is none). */
  readonly showDamage = input(false);
  readonly damage = input<PendingDamage | null>(null);
  /** The hit's damage waits for the target's reaction (Escudo): it is not a miss. */
  readonly waiting = input(false);
  readonly damageLine = input('');
  /** What each part of the damage rolled, and the steps resistance took, once there is any. */
  protected readonly breakdown = computed(() => {
    const d = this.damage();
    return !!d && ((d.partRolls ?? []).length > 0 || (d.steps ?? []).length > 0);
  });
  /** The roll is held for a question (Lucky, the Bardic Inspiration die): the d20 is shown, the result is not (and there
   * is no armor class). */
  readonly asking = input(false);
  /** "Sortudo: 1 → 14": the d20 the halfling's Lucky rolled again; the faces and the total are already the new ones. */
  protected readonly luckyLine = computed(() => luckyText(this.roll()?.d20));
  /** "+ 6 no d8 da Inspiração de Bardo": the die the player added after the d20; the total already has it. */
  protected readonly bonusLine = computed(() =>
    (this.roll()?.bonusDice ?? [])
      .filter((b) => b.used)
      .map((b) => `Com o d${b.sides} da Inspiração de Bardo (+${b.face}).`)
      .join(' '),
  );
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
    return `${upFirst(`${article(this.targetLabel())} ${this.targetLabel()}`)} estava com ${word}${from ? ` (${from})` : ''}${bonus ? `: ${bonus}` : ''}.`;
  });
}
