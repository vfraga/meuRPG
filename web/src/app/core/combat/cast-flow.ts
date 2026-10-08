import {
  type CombatantState,
  type DartsAtSlot,
  type DiceRoll,
  type PendingDamage,
  PendingDamageStatus,
  type SpellCast,
  type SpellTargetResult,
  type SpellTargets,
  type TargetInReach,
  AttackOutcome,
  SaveOutcome,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  ActionEconomy,
  type SlotChoice,
  type SpellDetails,
  SpellAttackType,
  SpellRangeKind,
  SpellSaveSuccess,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { rollFormula } from './combat-dice';
import { metersFixed, metersText } from '../units';
import { joinDots, tight } from '../format/text';
import { circleLabel } from './combat-options';
import { article } from './combat-log';
import { listing } from './cover';
import { effectWords, hpSpellKind, poolDice } from './hp-effects';
import { stateWord } from './combat-view';

/**
 * The steps of the cast sheet (E6-09): the slot, the targets (and Magic
 * Missile's darts), what the cast did and the damage still to roll. The
 * sheet's logic is kept here as small pure functions, so the choices, the
 * counters and the result lines are tested without a DOM.
 */

/** What a spell does, which decides what the sheet asks and what it shows. */
export type SpellKind = 'attack' | 'save' | 'darts' | 'heal' | 'plain' | 'pool' | 'hp';

export function spellKind(key: string, details: SpellDetails | null): SpellKind {
  if (key === 'spell:magic-missile') {
    return 'darts';
  }
  // The spells that read hit points: Sono and Leque Cromático roll a pool before
  // the cast, the others only name who they touch (E8-03).
  const hp = hpSpellKind(details);
  if (hp) {
    return hp === 'pool' ? 'pool' : 'hp';
  }
  if (!details) {
    return 'plain';
  }
  if (
    details.attackType === SpellAttackType.MELEE ||
    details.attackType === SpellAttackType.RANGED
  ) {
    return 'attack';
  }
  if (details.save) {
    return 'save';
  }
  return Object.keys(details.healBySlotLevel).length > 0 ? 'heal' : 'plain';
}

// ---- the slot ----

/** A row of the slot step: "1º nível", "1 livre de 4". */
export interface SlotRow {
  readonly level: number;
  readonly pact: boolean;
  readonly free: number;
  /** How many slots of this level the character has; `null` when unknown. */
  readonly total: number | null;
  readonly used: number;
  readonly enabled: boolean;
  readonly title: string;
  /** "1 livre de 4". */
  readonly count: string;
}

/** "1 livre de 4", "0 livres de 2"; without the total, "1 livre". */
export function freeText(free: number, total: number | null): string {
  const word = free === 1 ? 'livre' : 'livres';
  return total === null ? `${free} ${word}` : `${free} ${word} de ${total}`;
}

/**
 * The rows a spell of `level` can be cast with: every circle the character
 * has from the spell's own up (the ones with no free slot are listed too,
 * disabled, so "2º nível: Sem espaço livre" is seen), and the pact slots.
 * `choices` are the server's free slots (`SpellOption.slots`), who decides
 * what is enabled; `usage` has the totals, for "de 4".
 */
export function slotRows(
  level: number,
  choices: readonly SlotChoice[],
  usage: readonly { readonly level: number; readonly total: number; readonly used: number }[],
  pact: { readonly slotLevel: number; readonly total: number; readonly used: number } | null = null,
): SlotRow[] {
  const rows: SlotRow[] = [];
  const levels = new Set<number>(choices.filter((c) => !c.pact).map((c) => c.level));
  for (const u of usage) {
    if (u.level >= level) {
      levels.add(u.level);
    }
  }
  for (const l of [...levels].sort((a, b) => a - b)) {
    const choice = choices.find((c) => c.level === l && !c.pact);
    const u = usage.find((x) => x.level === l);
    const free = choice?.free ?? (u ? u.total - u.used : 0);
    rows.push(row(l, false, free, u?.total ?? null, u?.used ?? 0, !!choice && choice.free > 0));
  }
  const pactChoice = choices.find((c) => c.pact);
  if (pactChoice || (pact && pact.slotLevel >= level)) {
    const l = pactChoice?.level ?? pact?.slotLevel ?? 0;
    const free = pactChoice?.free ?? (pact ? pact.total - pact.used : 0);
    rows.push(
      row(l, true, free, pact?.total ?? null, pact?.used ?? 0, !!pactChoice && pactChoice.free > 0),
    );
  }
  return rows;
}

function row(
  level: number,
  pact: boolean,
  free: number,
  total: number | null,
  used: number,
  enabled: boolean,
): SlotRow {
  return {
    level,
    pact,
    free,
    total,
    used,
    enabled,
    title: pact ? `${circleLabel(level)} (pacto)` : circleLabel(level),
    count: freeText(free, total),
  };
}

/** The row a spell opens with: the lowest free one. */
export function defaultSlot(rows: readonly SlotRow[]): SlotRow | null {
  return rows.find((r) => r.enabled) ?? null;
}

/** "É o seu último espaço de 1º nível: depois dele, o Escudo Arcano fica sem
 * espaço." The shield part only when Escudo is prepared and this slot is the
 * last one it could be cast with (`shieldFree` is the free slots of Escudo's
 * own options; `null` when the character has no Escudo). */
export function lastSlotWarning(
  slot: SlotRow | null,
  shieldFree: number | null,
  shieldName = 'Escudo Arcano',
): string {
  if (!slot || slot.free !== 1) {
    return '';
  }
  const base = `É o seu último espaço de ${circleLabel(slot.level)}`;
  return shieldFree === 1 ? `${base}: depois dele, o ${shieldName} fica sem espaço.` : `${base}.`;
}

// ---- the targets ----

/** How many targets a cast takes and whether the step is there at all. */
export interface TargetRule {
  /** `none`: the spell stays on the caster (or has no one to aim at). */
  readonly kind: 'none' | 'single' | 'multi' | 'darts';
  /** The most targets (10 is the server's limit for any cast). */
  readonly max: number;
  /** At least this many: 0 for an area, which may meet nobody. */
  readonly min: number;
}

export const MAX_CAST_TARGETS = 10;

/** Magic Missile's darts at a slot level (3 at the 1st, one more for each level above). */
export function dartsAt(darts: readonly DartsAtSlot[], level: number): number {
  return darts.find((d) => d.slotLevel === level)?.darts ?? 0;
}

export function targetRule(
  st: SpellTargets | undefined,
  casterId: string,
  spellLevel: number,
  slotLevel: number,
): TargetRule {
  if (!st) {
    return { kind: 'none', max: 0, min: 0 };
  }
  if (st.darts.length > 0) {
    const n = dartsAt(st.darts, slotLevel);
    return { kind: 'darts', max: Math.min(n, MAX_CAST_TARGETS), min: 1 };
  }
  // A spell that stays on the caster lists only the caster.
  if (st.maxTargets === 0 && st.targets.every((t) => t.combatantId === casterId)) {
    return { kind: 'none', max: 0, min: 0 };
  }
  if (st.maxTargets === 0) {
    return { kind: 'multi', max: MAX_CAST_TARGETS, min: 0 }; // an area: any number, even none
  }
  // The server says how many more targets each slot level above the spell's adds; a server that sets only the flag means one.
  const perLevel = st.targetsPerLevel > 0 ? st.targetsPerLevel : st.extraTargetPerLevel ? 1 : 0;
  const extra = perLevel * Math.max(slotLevel - spellLevel, 0);
  const max = Math.min(st.maxTargets + extra, MAX_CAST_TARGETS);
  return { kind: max === 1 ? 'single' : 'multi', max, min: 1 };
}

/** A target row: "Ferido · a 7,5 m", and why it can't be chosen. */
export interface CastTargetRow {
  readonly id: string;
  readonly label: string;
  readonly sub: string;
  readonly blocked: string;
  /** The cover the target has against this caster ("Meia cobertura (do mapa)") and its pictogram. */
  readonly cover: string;
  readonly coverMark: 'half' | 'three' | null;
}

/** The rows of the target step. `reach` is "Longe demais: alcance de 36 m"
 * for a target the spell cannot reach. */
export function castTargetRows(
  targets: readonly TargetInReach[],
  casterId: string,
  reachFt: number | null,
): CastTargetRow[] {
  return targets.flatMap((t) => {
    const cover = listing(t);
    if (cover.kind === 'left-out') {
      return [];
    }
    const parts: string[] = [];
    if (t.combatantId === casterId) {
      parts.push('você');
    } else {
      const word = stateWord(t.state, t.label);
      if (word) {
        parts.push(word);
      }
      if (t.distanceFt !== undefined) {
        parts.push(`a ${metersFixed(t.distanceFt)}`);
      }
    }
    return [
      {
        id: t.combatantId,
        label: t.combatantId === casterId ? `${t.label} (você)` : t.label,
        sub: tight(joinDots(parts)),
        blocked:
          cover.kind === 'blocked'
            ? cover.text
            : t.tooFar
              ? tight(reachFt ? `Longe demais: alcance de ${metersText(reachFt)}` : 'Longe demais')
              : '',
        cover: cover.kind === 'listed' ? cover.text : '',
        coverMark: cover.kind === 'listed' ? cover.mark : null,
      },
    ];
  });
}

/** Whether choosing `id` is allowed on top of the ones already chosen. */
export function canPick(rule: TargetRule, chosen: readonly string[], id: string): boolean {
  return chosen.includes(id) || chosen.length < rule.max;
}

/** The ids with `id` toggled (a single target replaces the choice). */
export function toggled(rule: TargetRule, chosen: readonly string[], id: string): string[] {
  if (rule.kind === 'single') {
    return [id];
  }
  if (chosen.includes(id)) {
    return chosen.filter((c) => c !== id);
  }
  return canPick(rule, chosen, id) ? [...chosen, id] : [...chosen];
}

// ---- the darts ----

/** The darts handed out so far: by target. */
export type Dealt = ReadonlyMap<string, number>;

export function dartsPlaced(dealt: Dealt): number {
  let n = 0;
  for (const v of dealt.values()) {
    n += v;
  }
  return n;
}

/** The counter in the footer next to "Conjurar": what is missing, and when all are placed. */
export function dartsStatus(total: number, dealt: Dealt): string {
  const placed = dartsPlaced(dealt);
  return placed === total
    ? `${total} de ${total} ${total === 1 ? 'dardo distribuído' : 'dardos distribuídos'}.`
    : `Distribua todos os dardos: ${placed} de ${total}.`;
}

/** The dealt darts with one more or one less on a target, inside 0 and the total. */
export function dealOne(
  dealtNow: Dealt,
  id: string,
  delta: 1 | -1,
  total: number,
): Map<string, number> {
  const next = new Map(dealtNow);
  const now = next.get(id) ?? 0;
  const value = now + delta;
  if (value < 0 || (delta > 0 && dartsPlaced(dealtNow) >= total)) {
    return next;
  }
  next.set(id, value);
  return next;
}

/** The targets of the request: who got at least one dart, each with its darts. */
export function dartTargets(dealtNow: Dealt): { combatantId: string; darts: number }[] {
  return [...dealtNow]
    .filter(([, n]) => n > 0)
    .map(([combatantId, darts]) => ({ combatantId, darts }));
}

// ---- what the sheet says ----

const ABILITY_PT: Partial<Record<Ability, string>> = {
  [Ability.STRENGTH]: 'Força',
  [Ability.DEXTERITY]: 'Destreza',
  [Ability.CONSTITUTION]: 'Constituição',
  [Ability.INTELLIGENCE]: 'Inteligência',
  [Ability.WISDOM]: 'Sabedoria',
  [Ability.CHARISMA]: 'Carisma',
};

/** The dice a spell makes at a slot level ("8d6"), from its details; `''` when there are none. */
export function damageDice(details: SpellDetails | null, slotLevel: number): string {
  const d = details?.damage[0];
  if (!d) {
    return '';
  }
  return (
    d.bySlotLevel[slotLevel] ??
    d.bySlotLevel[details?.spell?.level ?? 0] ??
    Object.values(d.byCharacterLevel)[0] ??
    ''
  );
}

/** "Ação · alcance 36 m · 3 dardos de 1d4 + 1 de energia, sempre acertam". */
export function castSubtitle(
  economy: ActionEconomy,
  kind: SpellKind,
  details: SpellDetails | null,
  slotLevel: number,
  darts: number,
): string {
  const parts = [
    economy === ActionEconomy.BONUS_ACTION
      ? 'Ação bônus'
      : economy === ActionEconomy.REACTION
        ? 'Reação'
        : 'Ação',
  ];
  const range = details?.range;
  if (range?.kind === SpellRangeKind.RANGED && range.distanceFt > 0) {
    parts.push(`alcance ${metersText(range.distanceFt)}`);
  } else if (range?.kind === SpellRangeKind.TOUCH) {
    parts.push('toque');
  } else if (range?.kind === SpellRangeKind.SELF) {
    parts.push('pessoal');
  }
  const type = details?.damage[0]?.damageTypePt ?? '';
  const dice = damageDice(details, slotLevel);
  switch (kind) {
    case 'darts':
      parts.push(`${darts || 3} dardos de 1d4 + 1 de ${type || 'energia'}, sempre acertam`);
      break;
    case 'attack':
      parts.push(`ataque de magia${dice ? ` · ${dice}${type ? ` de ${type}` : ''}` : ''}`);
      break;
    case 'save': {
      const ability = ABILITY_PT[details?.save?.ability ?? Ability.UNSPECIFIED];
      const half = details?.save?.onSuccess === SpellSaveSuccess.HALF ? ', metade se resistir' : '';
      parts.push(
        `resistência${ability ? ` de ${ability}` : ''}${dice ? ` · ${dice}${type ? ` de ${type}` : ''}${half}` : ''}`,
      );
      break;
    }
    case 'heal':
      parts.push('cura');
      break;
    case 'pool': {
      const pool = poolDice(details, slotLevel);
      parts.push(pool ? `${pool.count}d${pool.sides} de pontos de vida` : 'pontos de vida');
      break;
    }
    default:
  }
  return tight(joinDots(parts));
}

// ---- what the cast did ----

/** One target of the result: who, what the roll or the save said, and the damage. */
export interface CastRow {
  readonly id: string;
  readonly label: string;
  /** "Muito ferido", "Derrotado": how the target is now. */
  readonly state: string;
  /** "Acertou", "Errou", "Crítico", "Falhou", "Resistiu: metade". */
  readonly word: string;
  readonly tone: 'good' | 'bad' | 'plain';
  /** The pill's icon when it is not the check or the cross: the moon of "Adormeceu". */
  readonly icon?: string;
  /** The formulas: the d20, "Dardo 1: 1d4 (3) + 1 = 4", the damage. */
  readonly lines: readonly string[];
  /** The big line: "7 de energia", "8 de cura", "Esperando o mestre aplicar o dano". */
  readonly summary: string;
  /** The damage of this target still waits to be rolled. */
  readonly owed: boolean;
}

/** The darts of a rolled damage, one line each: "Dardo 1: 1d4 (3) + 1 = 4". A
 * typed roll has no faces, only the sum. */
export function dartLines(roll: DiceRoll, darts: number): string[] {
  if (roll.physical || roll.faces.length !== darts || darts === 0) {
    return [`${darts} ${darts === 1 ? 'dardo' : 'dardos'}: ${rollFormula(roll)} · dado físico`];
  }
  const each = roll.modifier / darts;
  return roll.faces.map(
    (f, i) =>
      `Dardo ${i + 1}: 1d${roll.diceSides} (${f}) ${each < 0 ? '−' : '+'} ${Math.abs(each)} = ${f + each}`,
  );
}

function savedWord(saved: boolean, half: boolean): string {
  return saved ? (half ? 'Resistiu: metade' : 'Resistiu') : 'Falhou';
}

/** The result of a cast, row by row. `pendings` is every damage or heal of the
 * cast as it is now (the rolled ones carry their dice); `states` the targets'
 * state words now. */
export function castRows(
  cast: SpellCast,
  pendings: ReadonlyMap<string, PendingDamage>,
  labels: ReadonlyMap<string, { label: string; state: CombatantState }>,
): CastRow[] {
  return cast.targets.map((t) =>
    castRow(t, pendings.get(t.pendingDamageId), labels.get(t.combatantId), cast),
  );
}

/** What a spell that reads hit points did, as one live sentence for the whole
 * cast, in the past: "O Goblin 1 adormeceu. O Capitão Goblin não foi afetado."
 * A player's character has no article ("Brisa ficou estável"). Empty for any
 * other spell. */
export function effectSentence(
  cast: SpellCast,
  labels: ReadonlyMap<string, { label: string; state: CombatantState }>,
  isNpc: (id: string) => boolean,
): string {
  const parts: string[] = [];
  for (const t of cast.targets) {
    if (!t.effect) {
      continue;
    }
    const label = labels.get(t.combatantId)?.label ?? 'Alvo';
    const w = effectWords(cast.effectKind, cast.effectConditionKey, t.effect.outcome, label);
    const who = isNpc(t.combatantId) ? `${article(label)} ${label}` : label;
    const sentence = `${who} ${w.past.charAt(0).toLowerCase()}${w.past.slice(1)}.`;
    parts.push(`${sentence.charAt(0).toUpperCase()}${sentence.slice(1)}`);
  }
  return parts.join(' ');
}

function castRow(
  t: SpellTargetResult,
  p: PendingDamage | undefined,
  who: { label: string; state: CombatantState } | undefined,
  cast: SpellCast,
): CastRow {
  const label = who?.label ?? 'Alvo';
  const lines: string[] = [];
  let word = '';
  let icon: string | undefined;
  let tone: CastRow['tone'] = 'plain';
  if (t.effect) {
    // A spell that reads hit points: the outcome in a word and an icon, never a number
    // of the target's (a player has none), and the heal's amount only when it is sent.
    const w = effectWords(cast.effectKind, cast.effectConditionKey, t.effect.outcome, label);
    word = w.past;
    icon = w.icon;
    tone = w.affected ? 'good' : 'plain';
    if (t.effect.healed !== undefined) {
      lines.push(`${t.effect.healed} PV recuperados`);
    }
  }
  if (t.attackRoll) {
    lines.push(rollFormula(t.attackRoll));
  }
  if (t.outcome !== AttackOutcome.UNSPECIFIED) {
    const hit = t.outcome !== AttackOutcome.MISS;
    word = t.outcome === AttackOutcome.CRITICAL_HIT ? 'Crítico' : hit ? 'Acertou' : 'Errou';
    tone = hit ? 'good' : 'bad';
  }
  if (t.save) {
    const saved = t.save.outcome === SaveOutcome.SAVED;
    word = savedWord(saved, !!p?.half);
    tone = saved ? 'plain' : 'good';
    if (t.save.roll) {
      lines.push(
        `${tight(`Resistência: ${rollFormula(t.save.roll)}`)}${t.save.dc > 0 ? `, CD ${t.save.dc}` : ''}`,
      );
    } else if (t.save.dc > 0) {
      lines.push(`CD ${t.save.dc}`);
    }
  }
  let summary = '';
  const owed = !!p && p.status === PendingDamageStatus.AWAITING_ROLL;
  if (t.darts > 0 && p?.roll) {
    lines.push(...dartLines(p.roll, t.darts));
  } else if (t.darts > 0) {
    lines.push(`${t.darts} ${t.darts === 1 ? 'dardo' : 'dardos'}`);
  }
  if (p && !owed && p.roll) {
    if (t.darts === 0) {
      lines.push(rollFormula(p.roll));
    }
    summary = p.healing
      ? `${p.amount} de cura`
      : p.status === PendingDamageStatus.ROLLED
        ? `${p.amount} de ${p.damageTypePt || 'dano'}. Esperando o mestre aplicar`
        : `${p.amount} de ${p.damageTypePt || 'dano'}`;
  }
  return {
    id: t.combatantId,
    label,
    state: who ? stateWord(who.state, who.label) : '',
    word,
    tone,
    icon,
    lines,
    summary,
    owed,
  };
}

/** The damage rolls a cast still owes, grouped as the server settles them:
 * the damages of one cast id are one roll (an area spell, a heal for many),
 * the ones with no cast id (Magic Missile's) one roll each. */
export function rollGroups(pendings: readonly PendingDamage[]): PendingDamage[][] {
  const groups: PendingDamage[][] = [];
  for (const p of pendings) {
    if (p.status !== PendingDamageStatus.AWAITING_ROLL) {
      continue;
    }
    const same = p.castId ? groups.find((g) => g[0].castId === p.castId) : undefined;
    if (same) {
      same.push(p);
    } else {
      groups.push([p]);
    }
  }
  return groups;
}

/** "Sua ação foi usada." for an action spell; "Sua ação bônus foi usada." */
export function spentLine(economy: ActionEconomy): string {
  return economy === ActionEconomy.BONUS_ACTION
    ? 'Sua ação bônus foi usada.'
    : 'Sua ação foi usada.';
}
