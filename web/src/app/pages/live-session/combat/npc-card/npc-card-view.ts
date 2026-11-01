import {
  AreaPlacement,
  AttackOutcome,
  type AttackRoll,
  type Combatant,
  type Encounter,
  type GetTurnOptionsResponse,
  type PendingDamage,
  type TargetInReach,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { type Attack, DisabledReasonCode } from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { isHit, outcomeWord } from '../../../../core/combat/attack-flow';
import { attackName, reasonText } from '../../../../core/combat/combat-options';
import { rollFormula } from '../../../../core/combat/combat-dice';
import { article } from '../../../../core/combat/combat-log';
import { coverText } from '../../../../core/combat/cover';
import { restamText } from '../../../../core/combat/theatre';
import { joinDots } from '../../../../core/format/text';
import { metersFixed, reachSquares, squaresText } from '../../../../core/units';
import { showOfDice, type RollShow } from '../../../../shared/roll-overlay/roll-animator';
import type { RollField } from '../../../../shared/multi-roll/multi-roll';

/** The d20 the master types when the roll has advantage or disadvantage: one field for each die. */
export const D20_FIELDS: readonly RollField[] = [
  { key: 'd20-1', label: 'Primeiro d20', min: 1, max: 20 },
  { key: 'd20-2', label: 'Segundo d20', min: 1, max: 20 },
];

/** The d20 just rolled, shown with the armor class until the turn changes. */
export interface LastRoll {
  readonly roll: AttackRoll;
  readonly pending: PendingDamage | null;
  readonly subject: string;
}

/** Tenths of a foot in a foot: the combat counts distances in decifeet. */
const DFT_PER_FT = 10;

/** The attacks the card lists: those that roll to hit (a save-based one is not rolled here). */
export function rollableAttacks(options: GetTurnOptionsResponse | null): Attack[] {
  return (options?.options?.attacks ?? []).flatMap((a) =>
    a.attack && a.attack.saveDc === 0 ? [a.attack] : [],
  );
}

/** The NPC's spells whose area is placed on the map (none without a map): each one a "Conjurar" for the master's picker. */
export function areaSpellsOf(
  options: GetTurnOptionsResponse | null,
  theatre: boolean,
): { key: string; name: string }[] {
  if (theatre || !options?.options) {
    return [];
  }
  return (options.options.spells ?? []).flatMap((s) => {
    const t = (options.spellTargets ?? []).find((x) => x.spellKey === s.spell?.key);
    return s.spell && s.enabled && t && t.placement !== AreaPlacement.UNSPECIFIED
      ? [{ key: s.spell.key, name: s.spell.namePt || s.spell.name }]
      : [];
  });
}

/** Why a roll is not offered: a hit whose damage is still to roll or to apply holds the attacker's next attack (the
 * server refuses it too), and so does a combatant that cannot act (incapacitated, lethargy). */
export function rollWhyOf(options: GetTurnOptionsResponse | null, owed: number): string {
  if (owed > 0) {
    return 'Role ou aplique o dano do ataque anterior antes de rolar outro.';
  }
  const off = (options?.options?.attacks ?? []).find(
    (a) =>
      !a.enabled &&
      (a.reason?.code === DisabledReasonCode.INCAPACITATED ||
        a.reason?.code === DisabledReasonCode.EFFECT_LETHARGY),
  );
  return off ? reasonText(off.reason, true) : '';
}

/** The stats row of the card: hit points, armor class, speed and movement left. */
export function statsOf(c: Combatant, theatre: boolean) {
  return {
    hp: c.hitPointsMax !== undefined ? { now: c.hitPointsCurrent ?? 0, max: c.hitPointsMax } : null,
    ac: c.armorClass,
    speed: {
      meters: metersFixed(c.speedDft / DFT_PER_FT),
      squares: theatre ? '' : squaresText(reachSquares(c.speedFt)),
    },
    movement: theatre
      ? restamText(c.movementLeftDft)
      : joinDots([
          metersFixed(c.movementLeftDft / DFT_PER_FT),
          squaresText(reachSquares(c.movementLeftFt)),
        ]),
  };
}

/** The word of the pill: a reaction that holds the roll, the shield that stopped a hit, or the outcome. */
function resultWord(roll: AttackRoll, stopped: boolean, shield: string): string {
  if (roll.heldForReaction) {
    return 'Esperando a reação';
  }
  return stopped ? `Errou: o ${shield} segurou` : outcomeWord(roll.outcome);
}

/** The armor class the roll was compared with ("contra CA 18 do Toren"), or `''` when the server sent none. */
function againstText(roll: AttackRoll, target: Combatant | undefined): string {
  if (roll.targetArmorClass === undefined) {
    return '';
  }
  const label = target?.label ?? '';
  return `contra CA ${roll.targetArmorClass} ${article(label) === 'a' ? 'da' : 'do'} ${label}`;
}

/** The result box of the last roll. Escudo can turn a hit into a miss after the roll (the master's answer, or the
 * player's), and a reaction that holds the roll (Palavras de Interrupção) leaves no outcome until it is answered. */
export function resultOf(
  last: LastRoll | null,
  e: Encounter,
  stopped: boolean,
  shield: string,
  attacks: readonly Attack[] = [],
) {
  if (!last) {
    return null;
  }
  const roll = last.roll;
  const attack = attacks.find((a) => a.key === roll.attackKey);
  const target = e.combatants.find((c) => c.id === roll.targetId);
  return {
    // The head of the block (C7-02): "Cimitarra em Brisa".
    head: attack && target ? `${attackName(attack)} em ${target.label}` : '',
    total: roll.d20?.total ?? 0,
    formula: roll.d20 ? rollFormula(roll.d20) : '',
    physical: roll.d20?.physical ?? false,
    held: roll.heldForReaction,
    word: resultWord(roll, stopped, shield),
    hit: !roll.heldForReaction && !stopped && isHit(roll.outcome),
    against: againstText(
      roll,
      e.combatants.find((c) => c.id === roll.targetId),
    ),
  };
}

/** The hint of the two typed d20: which of them counts. */
export function pairHintOf(combine: 'lower' | 'higher'): string {
  return `Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada). Conta o ${combine === 'lower' ? 'menor' : 'maior'}.`;
}

/** The label of the typed d20: "Role 1d20 para Espada (+4)". */
export function rollLabelOf(a: Attack | null): string {
  return a
    ? `Role 1d20 para ${attackName(a)} (${a.attackBonus < 0 ? '−' : '+'}${Math.abs(a.attackBonus)})`
    : '';
}

/** " · Meia cobertura (do mapa)": the cover the target has against this attacker. */
export function coverNoteOf(t: TargetInReach): string {
  const text = coverText(t.cover, t.coverSource);
  return text ? ` · ${text}` : '';
}

/** "a 9 m, fora do alcance": how far a target is and whether the attack reaches it. */
export function distanceText(ft: number | undefined, tooFar: boolean): string {
  const parts: string[] = [];
  if (ft !== undefined) {
    parts.push(`a ${metersFixed(ft)}`);
  }
  if (tooFar) {
    parts.push('fora do alcance');
  }
  return parts.join(', ');
}

/** The master's own screen shows the total and the word; a roll a reaction holds shows the face alone. */
export function attackShow(a: Attack, roll: AttackRoll): RollShow | null {
  const held = roll.heldForReaction;
  const hit = isHit(roll.outcome);
  return showOfDice(`Ataque com ${attackName(a)}`, roll.d20, {
    withTotal: true,
    outcome: held ? undefined : { word: outcomeWord(roll.outcome), good: hit },
    critical: !held && roll.outcome === AttackOutcome.CRITICAL_HIT,
    fumble: !held && !hit,
  });
}
