import { hookSentence } from './hook-text';
import {
  AttackOutcome,
  type CombatLogDamage,
  type DiceRoll,
  type CombatLogEntry,
  type CombatLogModeChange,
  type CombatLogSpellTarget,
  PendingDamageStatus,
  type SaveResult,
  SaveOutcome,
  SpellEffectKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { DamageStepKind } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { extraDiceOf, physicalSplitFormula, splitFormula } from './combat-dice';
import { countedOf, countedSources, partLine, rolledLine } from './combat-log-dice';
import { modeWord } from './roll-mode';
import { listNames } from './conditions';
import { circleLabel } from './combat-options';
import { metamagicName } from '../resources/metamagic';
import { pointsText } from '../resources/pools';
import { degreeWord, sourceWord } from './cover';
import { hpCastText } from './combat-log-hp-cast';
import { LogContext, LogRoll, PoolCard } from './combat-log-types';
import { inThe, pushText, the } from './combat-log-words';

/** Whether the attack is made from afar, so the sentence says "atira". */
function isShot(key: string): boolean {
  return /bow|sling|dart|blowgun|net|^spell:/.test(key);
}

/** The applied damage in words: ", 25 de dano", or with the extra dice of a critical (Crítico Brutal) the groups of dice,
 * for whoever has the roll: the faces when the app rolled them, the counts and "dados físicos" when only the sum was typed. */
function appliedWords(d: CombatLogDamage): string {
  const extra = extraDiceOf(d);
  const kind = d.damageTypePt || 'dano';
  const cut = (formula: string) => formula.replace(/ = \d+$/, '');
  const split = extra && d.roll ? splitFormula(d.roll, extra, d.criticalMax) : null;
  if (split) {
    return `, dano ${cut(split)} = ${d.amount} de ${kind}`;
  }
  const typed = extra && d.roll ? physicalSplitFormula(d.roll, extra, d.criticalMax) : null;
  return typed
    ? `, dano ${cut(typed)} = ${d.amount} de ${kind}, dados físicos`
    : `, ${d.amount} de dano`;
}

/** The damage of an attack or of one target of a cast, as the sentence
 * tells it: ", 5 de dano", the dice the master overruled, a heal, a half. */
export function damageText(d: CombatLogDamage): string {
  const temp = d.tempHpGained > 0 ? `, ganha ${d.tempHpGained} PV temporários` : '';
  const hook = d.hook ? `. ${hookSentence(d.hook)}` : '';
  return `${baseDamageText(d)}${temp}${hook}`;
}

function baseDamageText(d: CombatLogDamage): string {
  if (d.healing) {
    return ` recupera ${d.amount} PV`;
  }
  switch (d.status) {
    case PendingDamageStatus.AWAITING_ROLL:
      return ', falta rolar o dano';
    case PendingDamageStatus.ROLLED:
      return `, ${d.amount} de dano, esperando o mestre aplicar`;
    case PendingDamageStatus.DISCARDED:
      return `, ${d.amount} de dano descartado`;
    default: {
      // The master may apply another number than the dice made: only he is told both.
      const other =
        d.rolledAmount !== undefined && d.rolledAmount !== d.amount
          ? ` (o dado deu ${d.rolledAmount})`
          : '';
      const half = d.half ? ' (metade)' : '';
      const failures =
        d.deathFailuresAdded > 0
          ? `, ${d.deathFailuresAdded === 1 ? 'uma falha' : 'duas falhas'} no teste contra a morte`
          : '';
      const words = appliedWords(d);
      // The half-orc's Relentless Endurance kept the target on its feet (SRD 5.1).
      const relentless = d.relentlessEndurance ? '. Resistência Implacável: fica com 1 PV' : '';
      return `${words}${half}${other}${failures}${relentless}`;
    }
  }
}

/** What a hit on a concentrating target reminds the table (RN-22). */
function concentrationText(d: CombatLogDamage | undefined): string {
  return d?.concentrationDc !== undefined
    ? `. Teste de Constituição, CD ${d.concentrationDc}, para manter a concentração`
    : '';
}

/** The master's own sum for an attack on covered target, "(CA 17: 15 + 2 de meia cobertura, do mapa)":
 * only he gets the armor class and the bonus, so nobody else reads it (RN-20). */
function coverNote(
  e: Pick<CombatLogEntry, 'cover' | 'coverSource' | 'targetArmorClass' | 'coverBonus'>,
): string {
  if (e.targetArmorClass === undefined || e.coverBonus <= 0) {
    return '';
  }
  const degree = degreeWord(e.cover).toLowerCase();
  const from = sourceWord(e.coverSource);
  return ` (CA ${e.targetArmorClass}: ${e.targetArmorClass - e.coverBonus} + ${e.coverBonus} de ${degree}${from ? `, ${from.replace('marcada pelo mestre', 'marcada por você')}` : ''})`;
}

/** ", com Magia Duplicada (1 ponto de feitiçaria)": the Metamagic a casting used is no secret (the caster's choice). */
function metamagicNote(
  spell:
    { readonly metamagicKeys: readonly string[]; readonly sorceryPointsSpent: number } | undefined,
): string {
  if (!spell || spell.metamagicKeys.length === 0) {
    return '';
  }
  return `, com ${spell.metamagicKeys.map(metamagicName).join(' e ')} (${pointsText(spell.sorceryPointsSpent)} de feitiçaria)`;
}

/** The die added after the d20 (the master's and the attacker's alone, like the d20): ", com o d8 da Inspiração de Bardo (+6)". */
function bonusDiceNote(e: CombatLogEntry): string {
  return e.bonusDice
    .filter((b) => b.used)
    .map((b) => `, com o d${b.sides} da Inspiração de Bardo (+${b.face})`)
    .join('');
}

export function attackText(e: CombatLogEntry): string {
  const target = e.targetLabel || 'alguém';
  const weapon = e.keyNamePt ? ` com ${the(e.keyNamePt)}` : '';
  const verb = isShot(e.key) ? `atira ${inThe(target)}` : `ataca ${the(target)}`;
  const opportunity = e.asReaction ? ' (ataque de oportunidade)' : '';
  let out = ` ${verb}${weapon}${opportunity}: ${e.outcome === AttackOutcome.CRITICAL_HIT ? 'crítico' : e.outcome === AttackOutcome.MISS ? 'errou' : 'acertou'}`;
  out += coverNote(e);
  out += bonusDiceNote(e);
  if (e.stoppedByReaction) {
    return `${out}, o Escudo Arcano segurou`; // the outcome is already "errou"
  }
  const d = e.damage;
  if (d && e.outcome !== AttackOutcome.MISS) {
    out += damageText(d);
    if (d.targetDefeated) {
      out += `. ${target} derrotado`;
    } else if (d.targetDown) {
      out += `. ${target} caiu`;
    }
    out += concentrationText(d);
  }
  // An opportunity attack that dropped the mover to 0 hit points sends it back to where it left the reach.
  if (e.returnedToReach) {
    out += `. ${target} voltou ao último quadrado dentro do alcance`;
  } else if (e.returnBlocked) {
    out += `. ${target} não pôde voltar ao último quadrado dentro do alcance: ele estava ocupado`;
  }
  return out;
}

/** The parenthesis after a save: the DC, and for the master the d20 of an NPC whose sheet has no
 * saving throw bonus. `bonus_known` is only sent to the master and is false for everyone else, and
 * that false means nothing; for the master it is an NPC whose roll is the bare d20, which the
 * master may overrule. */
function saveNotes(save: SaveResult, ctx: LogContext): string {
  const unknown =
    ctx.master && save.roll && !save.bonusKnown ? `d20 ${save.roll.total}, bônus desconhecido` : '';
  const notes = [save.dc > 0 ? `CD ${save.dc}` : '', unknown].filter((n) => n !== '');
  return notes.length > 0 ? ` (${notes.join('; ')})` : '';
}

/** One target of a cast: what the roll, the save and the damage did to it. */
export function castTargetText(t: CombatLogSpellTarget, ctx: LogContext): string {
  const who = t.targetLabel || 'alguém';
  // A spell with two damage types (Tempestade de Gelo) rolls each one: the target's text tells all.
  const damage = t.damage ? [t.damage, ...t.moreDamages].map(damageText).join('') : '';
  let out: string;
  if (t.darts > 0) {
    out = `${t.darts} ${t.darts === 1 ? 'dardo' : 'dardos'} ${inThe(who)}${damage}`;
  } else if (t.save) {
    const dc = saveNotes(t.save, ctx);
    // Esculpir Magias: spared by the caster, no roll.
    const verb = t.save.sculpted
      ? 'resistiu sem rolar (Esculpir Magias)'
      : t.save.outcome === SaveOutcome.SAVED
        ? 'resistiu'
        : 'falhou';
    const push = pushText(t);
    out = `${the(who)} ${verb}${dc}${damage}${push === '' ? '' : `; ${push}`}`;
  } else if (t.outcome !== AttackOutcome.UNSPECIFIED) {
    out = `${inThe(who)}: ${t.outcome === AttackOutcome.CRITICAL_HIT ? 'crítico' : t.outcome === AttackOutcome.MISS ? 'errou' : 'acertou'}${coverNote(t)}${t.outcome === AttackOutcome.MISS ? '' : damage}`;
  } else if (t.damage?.healing) {
    out = `${the(who)}${damage}`;
  } else {
    out = inThe(who);
  }
  return out;
}

/** "conjura Mísseis Mágicos (1º nível): 2 dardos no Capitão Goblin, 7 de
 * dano; 1 dardo no Goblin 2, 5 de dano". A spell with no effect the app
 * knows only names who it touches ("conjura Sono no Goblin 1"). */
export function castText(e: CombatLogEntry, ctx: LogContext): { text: string; card?: PoolCard } {
  if (e.spell && e.spell.effectKind !== SpellEffectKind.UNSPECIFIED) {
    return hpCastText(e, ctx);
  }
  const slot = e.spell?.slot;
  const circle = slot ? ` (${circleLabel(slot.level)})` : '';
  const targets = e.spell?.targets ?? [];
  const plain = targets.every(
    (t) => t.darts === 0 && !t.save && t.outcome === AttackOutcome.UNSPECIFIED && !t.damage,
  );
  let out = ` conjura ${e.keyNamePt || 'uma magia'}${circle}`;
  if (targets.length > 0) {
    out += plain
      ? ` ${listNames(targets.map((t) => castTargetText(t, ctx)))}`
      : `: ${targets.map((t) => castTargetText(t, ctx)).join('; ')}`;
  }
  out += metamagicNote(e.spell);
  if (e.spell?.concentrationEndedKey) {
    out += '. A concentração anterior acabou';
  }
  return { text: out };
}

/** "Ilaria conjurou Revivificar em Toren: voltou com 1 PV (morreu na rodada 3)", and for whoever may read it the
 * diamonds on a line of their own. The round of the death is the master's alone; the diamonds, the master's and the
 * caster's player's (the server leaves the rest out of the entry). */
export function reviveCastText(
  e: CombatLogEntry,
  ctx: LogContext,
): { text: string; note?: string } {
  const spell = e.spell!;
  const name = e.keyNamePt || 'Revivificar';
  const who = spell.targets.at(0)?.targetLabel || e.targetLabel || 'alguém';
  const prep = ctx.players.has(who) ? `em ${who}` : inThe(who);
  const round =
    ctx.master && spell.revivedDeathRound !== undefined
      ? ` (morreu na rodada ${spell.revivedDeathRound})`
      : '';
  return {
    text: ` conjurou ${name} ${prep}: voltou com 1 PV${round}`,
    note: spell.materialSpent
      ? `${e.actorLabel || 'Quem conjurou'} gastou diamantes de 300 PO`
      : undefined,
  };
}

/** The d20 pair of a roll, or nothing for a single die. */
export function pairOf(roll: DiceRoll | undefined, label: string): LogRoll | undefined {
  if (!roll || roll.diceCount !== 2 || roll.faces.length !== 2) {
    return undefined;
  }
  return {
    label,
    faces: roll.faces.map((value, i) => ({ value, counts: i === roll.countedIndex })),
  };
}

/** "Vantagem (sugerido: Normal) — o alvo está caído": the mode an attack rolled with when it is not what the server suggested. */
export function modeNote(m: CombatLogModeChange | undefined): string {
  if (!m) {
    return '';
  }
  const reason = m.reason ? ` — ${m.reason}` : '';
  return `${modeWord(m.mode)} (sugerido: ${modeWord(m.suggestedMode)})${reason}`;
}

/** The dice of each part of a damage, "Espada curta 1d6 (4) + 2 = 6", the steps resistance took, and the dice a floor raised
 * ("2d10 (1 → 2, 7) = 9", "Chama Insistente: o 1 contou como 2."). */
export function damageNotes(d: CombatLogDamage | undefined): string[] {
  if (!d) {
    return [];
  }
  const steps = d.steps.map((st) => {
    const word =
      st.kind === DamageStepKind.IMMUNITY
        ? 'imunidade'
        : st.kind === DamageStepKind.VULNERABILITY
          ? 'vulnerabilidade'
          : 'resistência';
    const base = `${st.labelPt || word}: ${st.before} → ${st.after}`;
    return st.ignored ? `${base} (o mestre deixou de fora)` : base;
  });
  return [...d.parts.map(partLine), ...rolledLine(d), ...steps, ...countedSources(countedOf(d))];
}
