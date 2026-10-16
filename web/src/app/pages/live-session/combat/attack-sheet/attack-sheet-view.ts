import { Code, ConnectError } from '@connectrpc/connect';

import {
  AttackOutcome,
  type AttackRoll,
  type Combatant,
  CombatantSide,
  type Encounter,
  type InspirationOffer,
  type PendingDamage,
  PendingDamageStatus,
  type TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import type { RollModeRequest } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { Attack, BonusAttackRule } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import {
  isHit,
  missMark,
  outcomeWord,
  targetAfter,
  targetRows,
} from '../../../../core/combat/attack-flow';
import type { ModeChoice } from '../../../../core/combat/combat-client-types';
import { article } from '../../../../core/combat/combat-log';
import { beamSpentText, bonusSpentText } from '../../../../core/combat/combat-options';
import { diceName, sumRange } from '../../../../core/combat/combat-dice';
import {
  brutalLabel,
  brutalTypedHint,
  criticalHint,
  criticalTypedHint,
  fixedParts,
  hasExtraDice,
  typedRange,
} from '../../../../core/combat/critical';
import { combatErrorMessage } from '../../../../core/combat/combat-errors';
import { isPlayer, stateWord } from '../../../../core/combat/combat-view';
import { LUCKY_TRAIT } from '../../../../core/combat/lucky';
import { reactionWait } from '../../../../core/combat/reactions';
import { liveRequest } from '../../../../core/combat/roll-mode';
import { type ExtraDieField, missingDice } from '../../../../core/effects/effects';
import { joinDots, tight } from '../../../../core/format/text';
import type { InspirationRoll, LuckyRoll } from '../../../../core/resources/resources-client';
import {
  type RollShow,
  showOfDice,
  showOfDie,
} from '../../../../shared/roll-overlay/roll-animator';

/** What the attack sheet shows and asks, as pure functions of the attack, the combat and the roll. */

/** The d4 of Bênção and Perdição. */
export const EFFECT_DIE_FACES = 4;

/** An opportunity attack, and any reaction, reaches 5 ft: whatever range the weapon has when thrown. */
const REACTION_REACH_FT = 5;

/** An ally is tagged ("Aliada"), so a table of friends does not misread the list. */
export function withAllies(
  rows: ReturnType<typeof targetRows>,
  combatants: readonly Combatant[],
): ReturnType<typeof targetRows> {
  return rows.map((r) => {
    const c = combatants.find((x) => x.id === r.id);
    const ally =
      c && (isPlayer(c) || c.side === CombatantSide.PARTY)
        ? article(c.label) === 'a'
          ? 'Aliada'
          : 'Aliado'
        : '';
    return ally ? { ...r, sub: tight(joinDots([r.sub, ally].filter(Boolean))) } : r;
  });
}

/** The targets of the sheet before the allies are tagged: an opportunity attack has only the mover; the throw back
 * reaches the long range; a reaction reaches 5 ft; anything else the attack's range. */
export function attackTargetRows(i: {
  readonly opportunity: { readonly targetId: string; readonly targetLabel: string } | undefined;
  readonly thrown: boolean;
  readonly asReaction: boolean;
  readonly targets: readonly TargetInReach[];
  readonly attack: Pick<Attack, 'rangeFt' | 'longRangeFt'>;
}): ReturnType<typeof targetRows> {
  if (i.opportunity) {
    return [
      {
        id: i.opportunity.targetId,
        label: i.opportunity.targetLabel,
        sub: '',
        blocked: '',
        cover: '',
        coverMark: null,
      },
    ];
  }
  if (i.thrown) {
    const long = i.attack.longRangeFt;
    return targetRows(
      i.targets.map(
        (t) =>
          ({
            ...t,
            tooFar: t.distanceFt === undefined ? t.tooFar : t.distanceFt > long,
          }) as typeof t,
      ),
      long,
    );
  }
  if (i.asReaction) {
    return targetRows(
      i.targets.map(
        (t) =>
          ({
            ...t,
            tooFar: t.distanceFt === undefined || t.distanceFt > REACTION_REACH_FT,
          }) as typeof t,
      ),
      REACTION_REACH_FT,
    );
  }
  return targetRows(i.targets, i.attack.rangeFt);
}

/** The request of this attack as the combat shows it now (waiting, answered), or none. */
export function currentRequest(
  sent: RollModeRequest | null,
  encounter: Encounter | null,
): RollModeRequest | null {
  if (!sent) {
    return null;
  }
  const all = encounter?.rollModeRequests ?? [];
  const live = liveRequest(all, sent.id);
  return live ?? (all.some((r) => r.id === sent.id) ? null : sent);
}

/** What the roll sends about the mode: nothing when the suggestion stands, else the mode, why, and the answered request. */
export function modeChoiceOf(i: {
  readonly hasMode: boolean;
  readonly answered: RollModeRequest | null;
  readonly chosen: number;
  readonly suggested: number;
  readonly reason: string;
}): ModeChoice | undefined {
  if (!i.hasMode) {
    return undefined;
  }
  const r = i.answered;
  if (r) {
    return {
      mode: r.decidedMode,
      reason: r.decidedMode === r.suggestedMode ? '' : r.reason,
      requestId: r.id,
    };
  }
  return i.chosen === i.suggested ? undefined : { mode: i.chosen, reason: i.reason.trim() };
}

/** A held roll has no result yet: neither a word nor a pill (the master has not said, or a reaction holds it). */
export function outcomeOf(r: AttackRoll | null, asking: boolean) {
  return r && !r.heldForReaction && !asking
    ? {
        word: outcomeWord(r.outcome),
        hit: isHit(r.outcome),
        crit: r.outcome === AttackOutcome.CRITICAL_HIT,
        natural: missMark(r),
      }
    : null;
}

/** The d20 is rolled but a reaction holds the result: "Esperando o mestre. O resultado do seu ataque sai quando ele
 * responder." */
export function heldWait(
  r: AttackRoll | null,
  e: Encounter | null,
): { title: string; detail: string } | null {
  return r?.heldForReaction
    ? ((e ? reactionWait(e) : null) ?? {
        title: 'Esperando o mestre',
        detail: 'O resultado do seu ataque sai quando ele responder.',
      })
    : null;
}

/** What the attack spent: the reaction, one of Extra Attack's attacks (the action stays open for the rest) or the action. */
export function spentText(i: {
  readonly byMaster: boolean;
  readonly asReaction: boolean;
  readonly bonusRule: BonusAttackRule | undefined;
  readonly bonusAttacksLeft: number | undefined;
  readonly cantrip: boolean;
  readonly name: string;
  readonly beamsLeft: number | undefined;
  readonly attacksLeft: number | undefined;
  readonly attacksPerAction: number | undefined;
}): string {
  if (i.byMaster) {
    return 'A reação dele foi usada.';
  }
  if (i.asReaction) {
    return 'Sua reação foi usada.';
  }
  const bonus = bonusSpentText(i.bonusRule, i.bonusAttacksLeft);
  if (bonus) {
    return bonus;
  }
  if (i.cantrip && (i.beamsLeft ?? 0) > 1) {
    return beamSpentText(i.name, i.beamsLeft ?? 0);
  }
  const left = (i.attacksLeft ?? 1) - 1;
  if (!i.cantrip && (i.attacksPerAction ?? 1) > 1 && left > 0) {
    return `Você ainda tem ${left} ${left === 1 ? 'ataque' : 'ataques'} desta ação.`;
  }
  return 'Sua ação foi usada.';
}

/** The dice of the damage still to roll: "2d6" (doubled on a critical hit). */
export function damageDiceOf(p: PendingDamage | null) {
  return p
    ? {
        count: p.diceCount,
        sides: p.diceSides,
        bonus: p.bonus,
        name: diceName(p.diceCount, p.diceSides),
      }
    : null;
}

/** The damage when a feature adds dice to the critical (Crítico Brutal): the screens say the groups and the feature's name. */
export function brutalOf(p: PendingDamage | null): PendingDamage | null {
  return p && hasExtraDice(p) ? p : null;
}

/** The range a typed sum may be. */
export function damageRangeOf(
  brutal: PendingDamage | null,
  dice: { count: number; sides: number } | null,
): { min: number; max: number } {
  if (brutal) {
    return typedRange(brutal);
  }
  return dice ? sumRange(dice.count, dice.sides) : { min: 1, max: 1 };
}

/** The line above the damage's dice to type: "Role 2d6 para o dano: some os dois". */
export function damageLabelOf(
  brutal: PendingDamage | null,
  dice: { count: number; name: string } | null,
  attackName: string,
): string {
  if (brutal) {
    return brutalLabel(brutal, `${article(attackName)} ${attackName}`);
  }
  return dice && dice.count > 1
    ? `Role ${dice.name} para o dano: some os dois`
    : `Role ${dice?.name ?? ''} para o dano`;
}

/** The hint under the typed field of the damage. */
export function damageHintOf(
  brutal: PendingDamage | null,
  p: PendingDamage | null,
  range: { min: number; max: number },
): string {
  if (brutal) {
    return brutalTypedHint(brutal);
  }
  return criticalTypedHint(
    p?.criticalRule ?? 0,
    p ? diceName(p.diceCount, p.diceSides) : '',
    range.min,
    range.max,
    p?.criticalMax ?? 0,
  );
}

/** What the typed sum is added to: the modifier and the critical's fixed maximum. */
export function fixedTextOf(brutal: PendingDamage | null, p: PendingDamage | null): string {
  if (brutal) {
    // The maximum stands in the live total's groups; the field's note is the modifier alone.
    return brutal.bonus === 0
      ? ''
      : `${brutal.bonus < 0 ? '−' : '+'} ${Math.abs(brutal.bonus)} de bônus`;
  }
  return fixedParts(p?.criticalMax ?? 0, p?.bonus ?? 0);
}

/** The line above the damage's dice: what a critical hit rolls under the table's rule ("role os dados duas vezes", "o
 * máximo mais uma rolagem"), or "Acertou: role o dano.". */
export function damageIntroOf(p: PendingDamage | null, crit: boolean | undefined): string {
  if (!p) {
    return '';
  }
  const hint = criticalHint(p.criticalRule, p.diceCount, p.diceSides, p.criticalMax);
  if (hint) {
    return hint.line;
  }
  return p.critical || crit
    ? `Acerto crítico: os dados do dano dobram (${diceName(p.diceCount, p.diceSides)}).`
    : 'Acertou: role o dano.';
}

/** What the target has left once the damage is rolled. */
export function afterDamageText(
  damage: PendingDamage | null,
  encounter: Encounter | null,
  targetLabel: string,
): string {
  if (!damage) {
    return '';
  }
  const target = encounter?.combatants.find((c) => c.id === damage.targetId);
  return targetAfter(
    targetLabel,
    !!target && isPlayer(target),
    damage,
    target ? stateWord(target.state) : '',
  );
}

/**
 * The d20 the app just rolled, tumbling (RN-20): the face, and the total and the word only where the sheet shows them (a
 * roll held for a reaction or for the Inspiração de Bardo question shows the face alone). Never an armor class.
 */
export function attackShow(
  name: string,
  res: { readonly roll: AttackRoll; readonly offer?: InspirationOffer },
): RollShow | null {
  const r = res.roll;
  const told = !res.offer && !r.heldForReaction && r.outcome !== AttackOutcome.UNSPECIFIED;
  const hit = isHit(r.outcome);
  return showOfDice(`Ataque com ${name}`, r.d20, {
    withTotal: !r.heldForReaction,
    outcome: told ? { word: outcomeWord(r.outcome), good: hit } : undefined,
    critical: told && r.outcome === AttackOutcome.CRITICAL_HIT,
    fumble: told && !hit,
  });
}

/** The damage dice the app just rolled, as the result step shows them: the faces, the roll's total and the damage type. */
export function damageShow(p: PendingDamage | undefined): RollShow | null {
  if (!p || p.status === PendingDamageStatus.AWAITING_REACTION) {
    return null;
  }
  return showOfDice('Dano', p.roll, { withTotal: true, note: p.damageTypePt || undefined });
}

/** The d20 the app rolled again for Lucky, tumbling: the new face alone, "1 → 14" (the sheet tells the result). */
export function luckyRerollShow(roll: AttackRoll, die: LuckyRoll | null): RollShow | null {
  if (!die || !('inApp' in die)) {
    return null;
  }
  const again = roll.d20?.rerolled.find((r) => r.sourceKey === LUCKY_TRAIT);
  return again
    ? showOfDie('Sortudo', roll.d20?.diceSides ?? 0, again.to, `${again.from} → ${again.to}`)
    : null;
}

/** The die the player added after the d20, rolled in the app (the sheet's "Com o d8 da Inspiração de Bardo (+5)" line). */
export function inspirationShow(
  roll: AttackRoll,
  use: boolean,
  die: InspirationRoll | null,
): RollShow | null {
  const bonus = use && die && 'inApp' in die ? roll.bonusDice.find((b) => b.used) : undefined;
  return bonus ? showOfDie('Inspiração de Bardo', bonus.sides, bonus.face, `+${bonus.face}`) : null;
}

/** What a refused roll says: physical dice and an effect the app did not list (one the master keeps from the players)
 * make the server say how many more d4 the roll takes, and the sheet asks for them. */
export function rollRefusal(err: unknown): { more: number; message: string } {
  const connectErr = ConnectError.from(err, Code.Unavailable);
  const more = connectErr.code === Code.InvalidArgument ? missingDice(connectErr.rawMessage) : 0;
  if (more > 0) {
    return {
      more,
      message:
        more === 1
          ? 'Esta rolagem leva mais um d4: role-o e digite o resultado.'
          : `Esta rolagem leva mais ${more} d4: role-os e digite os resultados.`,
    };
  }
  return { more: 0, message: combatErrorMessage(err, 'rolar o ataque') };
}

/** What a roll from physical dice still lacks of the d4 typed for it; `''` when it has them all. */
export function extraDiceProblem(
  fields: readonly ExtraDieField[],
  faces: readonly (number | null)[],
): string {
  if (faces.length === fields.length && !faces.some((f) => f === null)) {
    return '';
  }
  return fields.length === 1
    ? `Digite o resultado do d${fields[0].faces} antes de confirmar.`
    : 'Digite o resultado de cada dado extra antes de confirmar.';
}

/** The signed attack bonus, "+6" or "−1". */
export function signedBonus(b: number): string {
  return `${b < 0 ? '−' : '+'}${Math.abs(b)}`;
}
