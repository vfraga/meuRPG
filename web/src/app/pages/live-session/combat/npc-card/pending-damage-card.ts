import {
  type Combatant,
  type Encounter,
  type PendingDamage,
  PendingDamageStatus,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { hitPointsAfter, hitPointsLine } from '../../../../core/combat/attack-flow';
import {
  damageFormula,
  diceName,
  extraDiceOf,
  sumRange,
} from '../../../../core/combat/combat-dice';
import { article } from '../../../../core/combat/combat-log';
import { isPlayer } from '../../../../core/combat/combat-view';
import { criticalHint, criticalTypedHint, fixedParts } from '../../../../core/combat/critical';
import {
  type Removal,
  type SpellSave,
  awaitedSumOf,
  keptRowsOf,
  removalFor,
  takeOutPlan,
} from '../../../../core/combat/group-take-out';
import { heldReason } from '../../../../core/combat/reaction-master';

/**
 * Which part of a trap's hit a card is: "Armadilha: perfurante (2 de 4)". A trap that fires several times (darts) opens
 * one card per damage type per hit, all on the same target and trap point; the card carries no hit number, so the
 * position among the cards of the same target, point and type, in the order they were opened, stands for it.
 */
export function trapLabel(p: PendingDamage, all: readonly PendingDamage[]): string {
  if (!p.trapPointId) {
    return '';
  }
  const same = all.filter(
    (q) =>
      q.trapPointId === p.trapPointId &&
      q.targetId === p.targetId &&
      q.damageTypeKey === p.damageTypeKey,
  );
  const word = `${p.trapName || 'Armadilha'}: ${p.damageTypePt || 'dano'}`;
  return same.length > 1 ? `${word} (${same.indexOf(p) + 1} de ${same.length})` : word;
}

/** The target's hit points before and after `total` damage: the points of the form a druid in Wild Shape wears. */
function plainLine(target: Combatant, total: number): string {
  if (target.hitPointsMax === undefined) {
    return '';
  }
  const own = target.hitPointsCurrent ?? 0;
  return hitPointsLine(
    target.label,
    own,
    target.hitPointsMax,
    hitPointsAfter(own, target.hitPointsTemporary ?? 0, total),
  );
}

/** The target's hit points before and after the damage. A druid in Wild Shape loses the beast's points first
 * ("Lobo: 11 de 11 PV, depois 4"); what the beast cannot take passes to the druid (SRD 5.1, "Wild Shape"). */
export function effectLine(target: Combatant, total: number): string {
  const beastCurrent = target.wildShapeHitPointsCurrent;
  const beastMax = target.wildShapeHitPointsMax;
  if (target.wildShapeBeastKey === '' || beastCurrent === undefined || beastMax === undefined) {
    return plainLine(target, total);
  }
  const line = hitPointsLine(
    target.wildShapeBeastNamePt || 'Forma animal',
    beastCurrent,
    beastMax,
    hitPointsAfter(beastCurrent, 0, total),
  );
  const over = total - beastCurrent;
  if (over > 0 && target.hitPointsMax !== undefined) {
    return `${line}; o resto (${over}) passa para ${plainLine(target, over)}`;
  }
  return line;
}

/** Why "Rolar dano" is off: a window the combat waits on holds the damage (the answer is in the master's cards,
 * `ReactionQueue`), or the reaction of the target is awaited with no window to name. */
function heldText(p: PendingDamage, e: Encounter, target: Combatant | undefined): string {
  const awaiting = p.status === PendingDamageStatus.AWAITING_REACTION;
  const rolled = p.status === PendingDamageStatus.ROLLED;
  const window = awaiting || !rolled ? heldReason(e) : '';
  if (window || !awaiting) {
    return window;
  }
  return `Espere a reação ${article(target?.label ?? '') === 'a' ? 'da' : 'do'} ${target?.label ?? 'alvo'}.`;
}

/** What the roll form of a damage shows: its dice, the sum a typed roll may be and the critical's line (RN-24). */
function rollForm(p: PendingDamage) {
  const name = diceName(p.diceCount, p.diceSides);
  const range = sumRange(p.diceCount, p.diceSides);
  return {
    dice: name,
    range,
    label: p.diceCount > 1 ? `Role ${name} para o dano: some os dois` : `Role ${name} para o dano`,
    line: p.roll ? damageFormula(p.roll, p.damageTypePt, extraDiceOf(p), p.criticalMax) : '',
    // A critical hit follows the table's rule: what to roll, said the way it asks, and what the typed sum is.
    crit: p.critical
      ? (criticalHint(p.criticalRule, p.diceCount, p.diceSides, p.criticalMax)?.line ?? '')
      : '',
    typedHint: criticalTypedHint(p.criticalRule, name, range.min, range.max, p.criticalMax),
    modifier: p.bonus + p.criticalMax,
    fixedText: fixedParts(p.criticalMax, p.bonus),
  };
}

/** "Zona de magia: Nuvem de Adagas" for a damage a spell zone owes: the server marks no zone and no reason on it, so the
 * card matches the zone the caster has for that spell (the cast's own damage has a cast id, a trap's a trap point). Generic
 * words: whether the creature came in or began its turn there is in the log only. */
export function zoneLabel(p: PendingDamage, e: Encounter): string {
  if (p.castId || p.trapPointId) {
    return '';
  }
  const zone = e.zones.find(
    (z) => z.spellKey === p.attackKey && z.ownerCombatantId === p.attackerId,
  );
  return zone ? `Zona de magia: ${zone.spellNamePt}` : '';
}

/** Divine Smite's dice stay out of every group and keep their own key: the server refuses to take them out. */
const NOT_REMOVABLE: ReadonlySet<string> = new Set(['divine-smite', 'divine-smite-extra']);

/** The keys of the parts the master may take out: a choosable extra that counts, inside a typed group or not (a group asks a new
 * sum). Never while the damage waits for a sum, for a spell's damage or for a part a spell's hit effect made. */
function removableKeys(p: PendingDamage): Set<string> {
  if (p.status === PendingDamageStatus.AWAITING_GROUP_SUM || p.castId) {
    return new Set();
  }
  const effects = new Set((p.partRolls ?? []).filter((r) => r.hitEffect).map((r) => r.partKey));
  return new Set(
    (p.parts ?? [])
      .filter((x) => x.choosable && !NOT_REMOVABLE.has(x.key) && !effects.has(x.key))
      .map((x) => x.key),
  );
}

/** The Portuguese name of each damage type of the parts, by key. */
function typeNames(p: PendingDamage): Map<string, string> {
  return new Map(
    (p.parts ?? []).filter((x) => x.damageTypeKey).map((x) => [x.damageTypeKey, x.damageTypePt]),
  );
}

/** What each removable part's panel says. */
function plansOf(
  p: PendingDamage,
  attacker: Combatant | undefined,
  target: Combatant | undefined,
  saves: readonly SpellSave[],
) {
  const keys = removableKeys(p);
  return new Map(
    (p.partRolls ?? [])
      .filter((r) => keys.has(r.partKey))
      .map((r) => [r.partKey, takeOutPlan(r, p, attacker, target, saves)]),
  );
}

/** The parts out of the awaited group: the one the log names with its reason, or, before the log is read, the extras of the type that no longer count. */
function takenOutOf(p: PendingDamage, typeKey: string, removal: Removal | undefined) {
  if (removal) {
    return [{ name: removal.name, reason: removal.reason }];
  }
  return (p.parts ?? [])
    .filter((x) => x.damageTypeKey === typeKey && x.choosable && !x.selected && !x.available)
    .map((x) => ({ name: x.labelPt, reason: '' }));
}

/** " Até lá, o Zumbi 1 fica com 22 PV.": what the target keeps while the sum is awaited. */
function holdsLine(target: Combatant): string {
  return target.hitPointsCurrent === undefined
    ? ''
    : ` Até lá, ${article(target.label)} ${target.label} fica com ${target.hitPointsCurrent} PV.`;
}

/** The weapon's name, for the head of a card. */
function weaponName(p: PendingDamage): string {
  return (p.parts ?? []).find((x) => !x.choosable && !x.auto)?.labelPt ?? '';
}

/** What the card of a damage that awaits the sum of a damage type says (nothing for any other damage). */
function groupSumCard(
  p: PendingDamage,
  removals: readonly Removal[],
  target: Combatant | undefined,
) {
  const awaitedSum = awaitedSumOf(p);
  if (!awaitedSum) {
    return {
      awaitedSum,
      typeTitle: '',
      awaitedRange: '',
      kept: [],
      takenOut: [],
      hpNote: '',
    };
  }
  const { min, max, typePt } = awaitedSum;
  return {
    awaitedSum,
    typeTitle: typePt.charAt(0).toUpperCase() + typePt.slice(1),
    awaitedRange: min === max ? `deve ser ${min}` : `de ${min} a ${max}`,
    kept: keptRowsOf(p),
    takenOut: takenOutOf(p, awaitedSum.typeKey, removalFor(p, removals)),
    hpNote: target ? holdsLine(target) : '',
  };
}

/** One pending damage as its card shows it. */
export function pendingCard(
  p: PendingDamage,
  e: Encounter,
  all: readonly PendingDamage[],
  removals: readonly Removal[] = [],
  saves: readonly SpellSave[] = [],
) {
  const attacker = e.combatants.find((c) => c.id === p.attackerId);
  const target = e.combatants.find((c) => c.id === p.targetId);
  const rolled = p.status === PendingDamageStatus.ROLLED;
  const applied = p.status === PendingDamageStatus.APPLIED;
  // What "Aplicar" applies: the damage after resistance, vulnerability and immunity when a step changed it.
  const total = p.amountAfterSteps ?? p.amount;
  return {
    p,
    total,
    /** The extras the master may take out: the ones the player could choose, while they count, and no damage-type sum covers. */
    removable: removableKeys(p),
    plans: plansOf(p, attacker, target, saves),
    typeNames: typeNames(p),
    autoKeys: new Set((p.parts ?? []).filter((x) => x.auto).map((x) => x.key)),
    /** The damage was applied (the master's options list the latest one a take-out can still undo): only its breakdown and "Tirar". */
    applied,
    /** A part of a typed group was taken out: the damage waits for the sum of that type alone. */
    ...groupSumCard(p, removals, target),
    weapon: weaponName(p),
    rolls: p.partRolls ?? [],
    steps: p.steps ?? [],
    rolled,
    awaiting: p.status === PendingDamageStatus.AWAITING_REACTION,
    held: heldText(p, e, target),
    // A player rolls the damage of their own attack; the master waits.
    waitsForPlayer: !rolled && !!attacker && isPlayer(attacker),
    trapLabel: trapLabel(p, all),
    zoneLabel: zoneLabel(p, e),
    attacker: attacker?.label ?? '',
    target: target?.label ?? '',
    ...rollForm(p),
    effect: rolled && target ? effectLine(target, total) : '',
  };
}
