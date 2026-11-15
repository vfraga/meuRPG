import { type Signal, computed } from '@angular/core';

import { damageFormula, extraDiceOf } from '../../../../core/combat/combat-dice';
import { criticalSentence, criticalSum, brutalTyped } from '../../../../core/combat/critical';
import { isTradePart } from '../../../../core/combat/power-attack-trade';
import { activeParts, rollFormula } from '../../../../core/combat/damage-parts';
import {
  awaitedSumOf,
  keptRowsOf,
  removalFor,
  sumFormula,
} from '../../../../core/combat/group-take-out';
import { article } from '../../../../core/format/article';
import { typedBonusLine, typedFields } from '../../../../core/combat/typed-fields';
import type { RollField } from '../../../../shared/multi-roll/multi-roll';
import {
  afterDamageText,
  brutalOf,
  damageDiceOf,
  damageHintOf,
  damageIntroOf,
  damageLabelOf,
  damageRangeOf,
  fixedTextOf,
} from './attack-sheet-view';
import type { AttackState } from './attack-state';

/** What the damage step says and asks for: the dice still to roll, the critical's groups, the typed sum's fields and
 * the line told once the damage is rolled. */
export class AttackDamage {
  /** The dice of the damage still to roll: "2d6" (doubled on a critical hit). */
  readonly dice = computed(() => damageDiceOf(this.s.pending()));
  /** The damage when a feature adds dice to the critical (Crítico Brutal): the screens say the groups and the feature's name. */
  private readonly brutal = computed(() => brutalOf(this.s.pending()));
  /** "2d12 + 1d12 + 3" and "2d12 do crítico (dados dobrados) e 1d12 do Crítico Brutal (nível 9), mais 3 de modificador, de cortante.". */
  readonly brutalCard = computed(() => {
    const b = this.brutal();
    return b ? { sum: criticalSum(b), sentence: criticalSentence(b, b.damageTypePt) } : null;
  });
  readonly range = computed(() => damageRangeOf(this.brutal(), this.dice()));
  readonly parts = computed(() => this.s.pending()?.parts ?? []);
  /** One field for each damage type of the damage that is marked, for physical dice. */
  readonly typed = computed(() => typedFields(this.parts(), this.s.picks()));
  readonly fields = computed<readonly RollField[]>(() =>
    this.typed().map((f) => ({
      key: f.key,
      label: f.label,
      hint: f.hint,
      min: f.min,
      max: f.max,
    })),
  );
  /** "Sem o + 3: o app soma o bônus.": the fixed number the player does not type. */
  readonly bonusLine = computed(() =>
    typedBonusLine(this.flat(), this.parts().find((p) => isTradePart(p))?.labelPt ?? ''),
  );
  /** The button of the typed answer: "Aplicar 14 de dano" once every sum is there. */
  readonly applyLabel = (total: number | null): string =>
    total === null ? 'Aplicar dano' : `Aplicar ${total} de dano`;
  readonly flat = computed(() =>
    activeParts(this.parts(), this.s.picks()).reduce((sum, p) => sum + p.flat, 0),
  );
  /** "Rolar 1d8 + 2d6 + 3 no app": the dice of every part that counts, then the fixed number. */
  readonly partsLabel = computed(() => `Rolar ${rollFormula(this.parts(), this.s.picks())} no app`);
  readonly line = computed(() => {
    const d = this.s.damage();
    return d?.roll ? damageFormula(d.roll, d.damageTypePt, extraDiceOf(d), d.criticalMax) : '';
  });
  /** The master took an extra out of a group: the one sum this damage awaits, and what stays (`AWAITING_GROUP_SUM`). */
  readonly awaited = computed(() => {
    const p = this.s.pending();
    return p ? awaitedSumOf(p) : null;
  });
  readonly kept = computed(() => {
    const p = this.s.pending();
    return this.awaited() && p ? keptRowsOf(p) : [];
  });
  /** The part the master took out, as the log read by the attacker's player says it, with the reason. */
  readonly removal = computed(() => {
    const p = this.s.pending();
    return p ? removalFor(p, this.s.data.state.removals()) : undefined;
  });
  /** "O mestre tirou o Ataque Furtivo: “motivo”. Digite de novo só a soma de perfurante." */
  readonly groupNotice = computed(() => {
    const a = this.awaited();
    const r = this.removal();
    if (!a) {
      return '';
    }
    const what = r ? `${article(r.name)} ${r.name}` : 'um extra';
    return `O mestre tirou ${what}${r?.reason ? `: “${r.reason}”` : ''}. Digite de novo só a soma de ${a.typePt}.`;
  });
  /** "Dano: 4 + 3 + 6 de frio = 13", and the total, as the sum is typed. */
  readonly groupFormula = computed(() => {
    const a = this.awaited();
    const kept = this.kept();
    return a ? (sum: number) => sumFormula(a, kept, sum) : null;
  });
  /** What the hit points of the target never carry: the total without the typed sum, added to it by the field. */
  readonly groupModifier = computed(
    () => (this.awaited()?.flat ?? 0) + this.kept().reduce((n, k) => n + k.total, 0),
  );
  /** The typed sum's live total with the groups, only for the damage with extra dice. */
  readonly typedFormula = computed(() => {
    const b = this.brutal();
    return b
      ? (sum: number) => ({ text: brutalTyped(b, sum), total: sum + b.criticalMax + b.bonus })
      : null;
  });
  readonly totalNote = computed(() => {
    const b = this.brutal();
    return b && b.damageTypePt ? `de ${b.damageTypePt}` : 'Dano total';
  });
  readonly appLabel = computed(() =>
    this.brutal() ? 'Rolar dano no app' : `Rolar ${this.dice()?.name ?? ''} no app`,
  );
  readonly after = computed(() =>
    afterDamageText(this.s.damage(), this.s.data.state.encounter(), this.targetLabel()),
  );
  readonly defeated = computed(() => this.s.damage()?.targetDefeated ?? false);
  readonly label = computed(() => damageLabelOf(this.brutal(), this.dice(), this.name));
  readonly hint = computed(() => damageHintOf(this.brutal(), this.s.pending(), this.range()));
  /** What the typed sum is added to: the modifier and the critical's fixed maximum (the server's numbers, shown in the total before it is sent). */
  readonly modifier = computed(
    () => (this.s.pending()?.bonus ?? 0) + (this.s.pending()?.criticalMax ?? 0),
  );
  readonly fixedText = computed(() => fixedTextOf(this.brutal(), this.s.pending()));
  /** The critical's line only for whoever rolls physical dice (the app's dice are the server's). */
  readonly showCritical = computed(() => !this.canApp || this.s.typing());
  /** The line above the damage's dice: what a critical hit rolls under the table's rule ("role os dados duas vezes", "o máximo mais uma rolagem"), or "Acertou: role o dano.". */
  readonly intro = computed(() => damageIntroOf(this.s.pending(), this.crit()));

  constructor(
    private readonly s: AttackState,
    private readonly name: string,
    private readonly canApp: boolean,
    /** The roll was a critical hit. */
    private readonly crit: Signal<boolean | undefined>,
    private readonly targetLabel: Signal<string>,
  ) {}
}
