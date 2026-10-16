import {
  type CombatLogEntry,
  type CombatLogSpellTarget,
  SpellEffectGain,
  SpellEffectKind,
  SpellEffectOutcome,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { conditionName } from './conditions';
import { circleLabel } from './combat-options';
import { effectWords, gainWords, poolRollText, reasonWords } from './hp-effects';
import { LogContext, PoolCard, PoolCardRow } from './combat-log-types';
import { inThe, subject, upFirst } from './combat-log-words';

// ---- the spells that read hit points (E8-03) ----

/** "o Goblin 1 adormece. O Capitão Goblin não foi afetado.": one clause for each creature, the
 * first lower case because it follows a colon. */
function clauses(e: CombatLogEntry, ctx: LogContext): string {
  const spell = e.spell!;
  return (spell.targets ?? [])
    .map((t, i) => {
      const label = t.targetLabel || 'alguém';
      const w = effectWords(
        spell.effectKind,
        spell.effectConditionKey,
        t.effect?.outcome ?? SpellEffectOutcome.UNSPECIFIED,
        label,
        t.effect?.gain,
      );
      const verb = w.affected ? w.present : notAffectedPast(label);
      const text =
        woke(spell.effectKind, t.effect, `${subject(label, ctx)}`) ??
        `${subject(label, ctx)} ${verb}.`;
      return i === 0 ? text : upFirst(text);
    })
    .join(' ');
}

/** Ajuda on a target that was at 0: "Toren acordou com 5 PV (Ajuda).", the amount only where the server sent it
 * (the master and the target's own player). `null` for any other target. */
function woke(
  kind: SpellEffectKind,
  fx: { outcome: SpellEffectOutcome; gain: SpellEffectGain; healed?: number } | undefined,
  who: string,
): string | null {
  if (
    kind !== SpellEffectKind.MAX_HP ||
    fx?.outcome !== SpellEffectOutcome.AFFECTED ||
    fx.gain !== SpellEffectGain.CURRENT
  ) {
    return null;
  }
  return `${who} acordou${fx.healed === undefined ? '' : ` com ${fx.healed} PV`} (Ajuda).`;
}

function notAffectedPast(label: string): string {
  return effectWords(
    SpellEffectKind.UNSPECIFIED,
    '',
    SpellEffectOutcome.NOT_AFFECTED,
    label,
  ).past.toLowerCase();
}

export function hpCastText(e: CombatLogEntry, ctx: LogContext): { text: string; card?: PoolCard } {
  const spell = e.spell!;
  const name = e.keyNamePt || 'uma magia';
  if (!ctx.master) {
    // A player reads who was affected, never why, and never a number of an enemy's (RN-20).
    return { text: ` conjura ${name}: ${clauses(e, ctx)}` };
  }
  if (spell.effectKind === SpellEffectKind.POOL) {
    const card = poolCard(e);
    const slot = spell.slot ? ` (${circleLabel(spell.slot.level)})` : '';
    return { text: ` conjura ${name}${slot}`, card };
  }
  // One creature and its hit points: "no Capitão Goblin (27 PV, limite de 150): fica atordoado."
  const out = (spell.targets ?? []).map((t) => masterTargetText(spell, t, ctx));
  return { text: ` conjura ${name} ${out.join('; ')}` };
}

/** The master's table for a pool spell: the dice, then each creature from the lowest hit
 * points up with the total that is left, and the word with the condition it got. */
function poolCard(e: CombatLogEntry): PoolCard | undefined {
  const spell = e.spell!;
  if (!spell.poolRoll) {
    return undefined;
  }
  const targets = [...(spell.targets ?? [])].sort((a, b) => {
    const oa = a.effect?.poolOrder ?? Number.MAX_SAFE_INTEGER;
    const ob = b.effect?.poolOrder ?? Number.MAX_SAFE_INTEGER;
    return oa - ob;
  });
  const condition = conditionName(spell.effectConditionKey);
  const rows = targets.map((t) => poolRow(spell, t, condition));
  const roll = poolRollText(spell.poolRoll).replace(' · dado físico', '');
  const summary = poolSummary(roll, spell.poolRoll.physical, rows);
  return {
    roll,
    physical: spell.poolRoll.physical || spell.poolRoll.faces.length === 0,
    rows,
    note: 'A magia começa por quem tem menos PV e vai descontando do total; só afeta quem tem PV igual ou menor que o que sobra. Os PV de quem ficou assim não mudam: a condição não é ferimento.',
    slot: spell.slot ? `${e.actorLabel} gastou um espaço de ${circleLabel(spell.slot.level)}.` : '',
    changeFor: rows
      .filter((r) => r.affected && !!condition)
      .map((r) => ({ id: r.id, label: r.label })),
    summary,
  };
}

/** The arithmetic of a creature of the pool: what was left after it, or why it was not affected. */
function poolMath(fx: CombatLogSpellTarget['effect'], affected: boolean): string {
  const hp = fx?.hitPointsBefore;
  const left = fx?.poolLeft;
  if (affected && hp !== undefined && left !== undefined) {
    return `${left + hp} − ${hp} = ${left} restam`;
  }
  return fx && !affected ? reasonWords(fx.reason, hp, left, undefined) : '';
}

/** One creature of the pool card: its hit points, the sum that was left, and the word with the condition it got. */
function poolRow(spell: CastSpell, t: CombatLogSpellTarget, condition: string): PoolCardRow {
  const label = t.targetLabel || 'alguém';
  const fx = t.effect;
  const w = effectWords(
    spell.effectKind,
    spell.effectConditionKey,
    fx?.outcome ?? SpellEffectOutcome.UNSPECIFIED,
    label,
    fx?.gain,
  );
  const hp = fx?.hitPointsBefore;
  const math = poolMath(fx, w.affected);
  return {
    id: t.targetId,
    label,
    hitPoints: hp === undefined ? '' : `${hp} PV`,
    math,
    word: w.affected ? `${w.past}${condition ? ` · ${condition}` : ''}` : 'Não afetado',
    icon: w.icon,
    affected: w.affected,
  };
}

/** The card in one paragraph, for a screen reader: "5d8 = 22 PV. Goblin 1 (7 PV) adormeceu, 22 − 7 = 15 restam." */
function poolSummary(roll: string, physical: boolean, rows: readonly PoolCardRow[]): string {
  const sentences = rows.map(
    (r) =>
      `${r.label}${r.hitPoints ? ` (${r.hitPoints})` : ''} ${r.affected ? `${r.word.toLowerCase()}${r.math ? `, ${r.math}` : ''}` : 'não é afetado'}.`,
  );
  return `${roll} PV${physical ? ', dado físico' : ''}. ${sentences.join(' ')}`;
}

type CastSpell = NonNullable<CombatLogEntry['spell']>;

/** One creature of a spell that reads hit points, as the master reads it: "no Capitão Goblin (27 PV, limite de 150): fica atordoado". */
function masterTargetText(spell: CastSpell, t: CombatLogSpellTarget, ctx: LogContext): string {
  const label = t.targetLabel || 'alguém';
  const fx = t.effect;
  const w = effectWords(
    spell.effectKind,
    spell.effectConditionKey,
    fx?.outcome ?? SpellEffectOutcome.UNSPECIFIED,
    label,
    fx?.gain,
  );
  const prep = ctx.players.has(label) ? `em ${label}` : inThe(label);
  return `${prep}${hitPointFacts(spell, fx)}: ${masterResult(spell, fx, w)}`;
}

/** " (27 PV, limite de 150)": the hit points before and the limit of the spell, whichever the entry has. */
function hitPointFacts(spell: CastSpell, fx: CombatLogSpellTarget['effect']): string {
  const facts: string[] = [];
  if (fx?.hitPointsBefore !== undefined) {
    facts.push(`${fx.hitPointsBefore} PV`);
  }
  if (spell.effectThreshold !== undefined) {
    facts.push(`limite de ${spell.effectThreshold}`);
  }
  return facts.length > 0 ? ` (${facts.join(', ')})` : '';
}

/** What happened to the creature: the healing, the condition it got, or why it was spared. */
function masterResult(
  spell: CastSpell,
  fx: CombatLogSpellTarget['effect'],
  w: ReturnType<typeof effectWords>,
): string {
  const healed =
    fx?.healed !== undefined && w.affected ? gainWords(spell.effectKind, fx.healed, fx.gain) : '';
  if (healed) {
    return healed;
  }
  if (w.affected) {
    return w.present;
  }
  const reason = fx
    ? reasonWords(fx.reason, fx.hitPointsBefore, fx.poolLeft, spell.effectThreshold)
    : '';
  return `${w.present}${reason ? `: ${reason}` : ''}`;
}
