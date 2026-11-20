import {
  type CombatantState,
  type DiceRoll,
  type PendingDamage,
  PendingDamageStatus,
  type SpellCast,
  type SpellTargetResult,
  AttackOutcome,
  SaveOutcome,
  SpellEffectGain,
  SpellEffectKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { AdvantageSource, RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { ActionEconomy } from '../../../gen/meurpg/rules/v1/rules_pb';
import { rollFormula } from './combat-dice';
import { d20Formula } from './roll-mode';
import { tight } from '../format/text';
import { article } from './combat-log';
import { aidChange, effectWords, gainLine } from './hp-effects';
import { stateWord } from './combat-view';
import { diceNaturalMark } from './natural-mark';
import { luckyText } from './lucky';

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
  /** "1 natural" beside "Errou": a natural 1 always misses an attack roll (SRD 5.1). Empty otherwise. */
  readonly natural?: string;
  /** The pill's icon when it is not the check or the cross: the moon of "Adormeceu". */
  readonly icon?: string;
  /** The formulas: the d20, "Dardo 1: 1d4 (3) + 1 = 4", the damage. */
  readonly lines: readonly string[];
  /** The big line: "7 de energia", "8 de cura", "Esperando o mestre aplicar o dano". */
  readonly summary: string;
  /** The damage of this target still waits to be rolled. */
  readonly owed: boolean;
  /** The d20 of the attack and of the save, with the mode each rolled in and why: two faces are drawn for advantage and disadvantage. */
  readonly d20s?: readonly CastD20[];
}

/** A d20 of a cast result: "Ataque" or "Resistência", the roll and the circumstances behind its mode. */
export interface CastD20 {
  readonly caption: string;
  readonly roll: DiceRoll;
  readonly mode: RollMode;
  readonly sources: readonly AdvantageSource[];
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
    const w = effectWords(
      cast.effectKind,
      cast.effectConditionKey,
      t.effect.outcome,
      label,
      t.effect.gain,
    );
    const who = isNpc(t.combatantId) ? `${article(label)} ${label}` : label;
    const sentence = `${who} ${w.past.charAt(0).toLowerCase()}${w.past.slice(1)}.`;
    parts.push(`${sentence.charAt(0).toUpperCase()}${sentence.slice(1)}`);
  }
  return parts.join(' ');
}

/** What one side of a target's result adds to its row; a later part's word and tone win over an earlier one's. */
interface RowPart {
  readonly word?: string;
  readonly tone?: CastRow['tone'];
  readonly icon?: string;
  readonly natural?: string;
  readonly lines: readonly string[];
  readonly d20s?: readonly CastD20[];
}

const NO_PART: RowPart = { lines: [] };

function castRow(
  t: SpellTargetResult,
  p: PendingDamage | undefined,
  who: { label: string; state: CombatantState } | undefined,
  cast: SpellCast,
): CastRow {
  const label = who?.label ?? 'Alvo';
  const parts = [effectPart(t, cast, label), attackPart(t), savePart(t, p)];
  const damage = damagePart(t, p);
  const d20s = parts.flatMap((part) => part.d20s ?? []);
  const natural = parts.find((part) => part.natural)?.natural ?? '';
  return {
    id: t.combatantId,
    label,
    state: who ? stateWord(who.state) : '',
    word: parts.reduce((word, part) => part.word ?? word, ''),
    tone: parts.reduce<CastRow['tone']>((tone, part) => part.tone ?? tone, 'plain'),
    ...(natural ? { natural } : {}),
    icon: parts[0].icon,
    lines: [...parts.flatMap((part) => part.lines), ...damage.lines],
    summary: damage.summary,
    owed: damage.owed,
    ...(d20s.length ? { d20s } : {}),
  };
}

/** A spell that reads hit points: the outcome in a word and an icon, never a number of the target's (a player has
 * none), and the heal's amount only when it is sent. */
function effectPart(t: SpellTargetResult, cast: SpellCast, label: string): RowPart {
  const fx = t.effect;
  if (!fx) {
    return NO_PART;
  }
  const w = effectWords(cast.effectKind, cast.effectConditionKey, fx.outcome, label, fx.gain);
  // Ajuda: "PV 0 → 5 de 45" where the server sent the numbers (the master and the target's own player).
  const change =
    cast.effectKind === SpellEffectKind.MAX_HP
      ? aidChange(fx.hitPointsBefore, fx.healed, fx.hitPointsAfter, fx.hitPointsMaxAfter)
      : null;
  const lines: string[] = [];
  // The one who woke up reads the change alone: "Acorda com 5 PV" would say it twice.
  if (fx.healed !== undefined && !(change && fx.gain === SpellEffectGain.CURRENT)) {
    lines.push(gainLine(cast.effectKind, fx.healed, fx.gain));
  }
  if (change) {
    lines.push(change);
  }
  return { word: w.past, icon: w.icon, tone: w.affected ? 'good' : 'plain', lines };
}

/** The attack roll of a spell attack and what it came to: critical, hit or miss, and a natural 1. */
function attackPart(t: SpellTargetResult): RowPart {
  const lines: string[] = [];
  const d20s: CastD20[] = [];
  if (t.attackRoll) {
    lines.push(d20Formula(t.attackRoll));
    // "Sortudo: 1 → 14": the d20 the halfling's Lucky rolled again; the formula already has the new face.
    const lucky = luckyText(t.attackRoll);
    if (lucky) {
      lines.push(lucky);
    }
    d20s.push({
      caption: 'Ataque',
      roll: t.attackRoll,
      mode: t.attackMode,
      sources: t.attackSources,
    });
  }
  if (t.outcome === AttackOutcome.UNSPECIFIED) {
    return { lines, d20s };
  }
  const hit = t.outcome !== AttackOutcome.MISS;
  return {
    word: t.outcome === AttackOutcome.CRITICAL_HIT ? 'Crítico' : hit ? 'Acertou' : 'Errou',
    tone: hit ? 'good' : 'bad',
    natural:
      !hit && t.attackRoll && diceNaturalMark(t.attackRoll) === '1 natural' ? '1 natural' : '',
    lines,
    d20s,
  };
}

/** The saving throw of the target: its word, its roll or why it has none. */
function savePart(t: SpellTargetResult, p: PendingDamage | undefined): RowPart {
  const save = t.save;
  if (!save) {
    return NO_PART;
  }
  const saved = save.outcome === SaveOutcome.SAVED;
  const word = savedWord(saved, !!p?.half);
  const tone = saved ? 'plain' : 'good';
  const dc = save.dc > 0 ? `, CD ${save.dc}` : '';
  if (save.roll) {
    // "Sortudo: 1 → 14": a halfling player's 1 the app rolled again by itself; the formula already has the new face.
    const lucky = luckyText(save.roll);
    return {
      word,
      tone,
      lines: [tight(`Resistência: ${d20Formula(save.roll)}`) + dc, ...(lucky ? [lucky] : [])],
      d20s: [{ caption: 'Resistência', roll: save.roll, mode: save.mode, sources: save.sources }],
    };
  }
  if (save.sculpted) {
    return { word, tone, lines: ['Resistência: passa sem rolar (Esculpir Magias)'] };
  }
  if (save.autoFailed) {
    return { word, tone, lines: [`Resistência: Falha automática${dc}`] };
  }
  return { word, tone, lines: save.dc > 0 ? [`CD ${save.dc}`] : [] };
}

/** The darts and the damage of the target, and whether its damage still waits for the roll. */
function damagePart(
  t: SpellTargetResult,
  p: PendingDamage | undefined,
): { lines: string[]; summary: string; owed: boolean } {
  const lines: string[] = [];
  const owed = !!p && p.status === PendingDamageStatus.AWAITING_ROLL;
  if (t.darts > 0 && p?.roll) {
    lines.push(...dartLines(p.roll, t.darts));
  } else if (t.darts > 0) {
    lines.push(`${t.darts} ${t.darts === 1 ? 'dardo' : 'dardos'}`);
  }
  if (!p || owed || !p.roll) {
    return { lines, summary: '', owed };
  }
  if (t.darts === 0) {
    lines.push(rollFormula(p.roll));
  }
  return { lines, summary: damageSummary(p), owed };
}

/** The big line of a rolled damage: "7 de energia", "8 de cura", "6 de frio. Esperando o mestre aplicar". */
function damageSummary(p: PendingDamage): string {
  if (p.healing) {
    return `${p.amount} de cura`;
  }
  const base = `${p.amount} de ${p.damageTypePt || 'dano'}`;
  return p.status === PendingDamageStatus.ROLLED ? `${base}. Esperando o mestre aplicar` : base;
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
