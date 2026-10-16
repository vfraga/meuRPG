import {
  AttackOutcome,
  type CombatLogDamage,
  type DiceRoll,
  CombatEffect,
  type CombatLogEffect,
  CombatLogEffectChange,
  type CombatLogEntry,
  CombatLogKind,
  type CombatLogRound,
  type CombatLogModeChange,
  type CombatLogStateChange,
  type CombatLogSpellTarget,
  DeathSaveOutcome,
  JumpKind,
  LayOnHandsCureKind,
  PendingDamageStatus,
  type SaveResult,
  SaveOutcome,
  SpellEffectGain,
  SpellEffectKind,
  SpellEffectOutcome,
  WildShapeEndReason,
  HitRiderChoice,
  HitRiderLogResult,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatantStateKind, DamageStepKind } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { ContestLogLine } from '../../../gen/meurpg/play/v1/contest_types_pb';
import { extraDiceOf, physicalSplitFormula, rollText, splitFormula } from './combat-dice';
import { modeWord } from './roll-mode';
import { conditionName, listNames } from './conditions';
import { metersFixed, metersText } from '../units';
import { circleLabel } from './combat-options';
import { metamagicName } from '../resources/metamagic';
import { pointsText } from '../resources/pools';
import { countsSentence } from './death-saves';
import { degreeWord, sourceWord } from './cover';
import { effectWords, gainWords, poolRollText, reasonWords } from './hp-effects';
import { trapLogText } from '../traps/trap-log';
import { article } from '../format/article';
import { naturalMark } from './natural-mark';
import { luckyText } from './lucky';

/**
 * The combat log as the screens read it (E6-11, E6-14, E6-15): the server
 * sends structured entries, and the sentence is written here, in
 * Portuguese, from what each kind carries. A kind this app doesn't know
 * (a newer server's) is skipped, never shown as an empty line.
 */

/** One line of the log, ready to draw: an icon, the actor in bold, and the
 * rest of the sentence. */
export interface LogLine {
  readonly id: string;
  readonly icon: string;
  /** Who did it (bold); empty for "Combate iniciado". */
  readonly actor: string;
  /** What follows the actor, starting with a space when there is one. */
  readonly text: string;
  /** The master's alone ("Só o mestre vê"). */
  readonly hidden: boolean;
  readonly undoable: boolean;
  /** A second sentence under the line, whole, with its own actor: "Ilaria gastou diamantes de 300 PO". */
  readonly note?: string;
  /** The master's account of a pool spell (Sono): the dice, each creature and what is left of the total. */
  readonly card?: PoolCard;
  /** The d20 pair of an attack, both faces with the one that counts marked. */
  readonly roll?: LogRoll;
  /** Lines under the sentence: the mode of the roll, the parts of the damage, the resistance steps, a reason. */
  readonly notes?: readonly string[];
}

/** The two d20 of a roll with advantage or disadvantage. */
export interface LogRoll {
  readonly label: string;
  readonly faces: readonly { readonly value: number; readonly counts: boolean }[];
}

/** One creature of the pool card, in the order the pool went through them. */
export interface PoolCardRow {
  readonly id: string;
  readonly label: string;
  /** "7 PV". */
  readonly hitPoints: string;
  /** "15 − 7 = 8 restam", "27 é mais que 8 restantes". */
  readonly math: string;
  /** "Adormeceu · Inconsciente", "Não afetado". */
  readonly word: string;
  readonly icon: string;
  readonly affected: boolean;
}

/** What only the master reads under a Sono or a Leque Cromático (E8-03). */
export interface PoolCard {
  /** "5d8 (2, 4, 1, 5, 3) = 15": the total is the pool of hit points. */
  readonly roll: string;
  /** The caster typed the sum of physical dice: there are no faces, and the card says so. */
  readonly physical: boolean;
  readonly rows: readonly PoolCardRow[];
  /** What the spell does, once, for the one who has not seen it before. */
  readonly note: string;
  /** "Pensantus gastou um espaço de 1º nível.". */
  readonly slot: string;
  /** The creatures that got a condition, to change it ("Mudar as condições do Goblin 1"). */
  readonly changeFor: readonly { readonly id: string; readonly label: string }[];
  /** The same in one sentence, for a screen reader. */
  readonly summary: string;
}

/** Who is reading, for the spells that read hit points: the master's sentence has the hit
 * points and the limit, the player's only who was affected; and a player's character is named
 * without an article ("Brisa"), a creature with one ("o Goblin 1"). */
export interface LogContext {
  readonly master: boolean;
  /** The labels of the player characters in this combat. */
  readonly players: ReadonlySet<string>;
  /** The combat is played without a map (RN-25): a move is "gastou 6,0 m de movimento", and the start says so. */
  readonly theatre?: boolean;
}

const NO_CONTEXT: LogContext = { master: false, players: new Set() };

export interface LogGroup {
  readonly round: number;
  /** "Rodada 2". */
  readonly title: string;
  /** "em andamento" or "encerrada". */
  readonly status: string;
  readonly lines: readonly LogLine[];
}

export { article };

/** "no Toren", "na Brisa". */
function inThe(name: string): string {
  return `${article(name) === 'a' ? 'na' : 'no'} ${name}`;
}

/** "o Goblin 1", "a Brisa". */
function the(name: string): string {
  return `${article(name)} ${name}`;
}

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
function damageText(d: CombatLogDamage): string {
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

/** What a class resource did (Cura pelas Mãos, Conjuração Flexível, Inspiração de Bardo). The hit points a touch gave back
 * are only in the entry for the master and the target's player; why a touch did nothing is the master's alone. */
function resourceText(e: CombatLogEntry): { icon: string; text: string } {
  const r = e.resource;
  const target = e.targetLabel || 'alguém';
  switch (r?.key) {
    case 'feature:lay-on-hands':
      if (r.nothingHappened) {
        const why = r.nothingReason ? `; o toque não agiu: ${r.nothingReason}` : '';
        return {
          icon: 'favorite',
          text: ` tocou ${the(target)} com a Cura pelas Mãos (${pointsText(r.spent)}): sem efeito${why}`,
        };
      }
      if (r.cure === LayOnHandsCureKind.POISON) {
        return {
          icon: 'favorite',
          text: ` neutralizou o veneno de ${target} com a Cura pelas Mãos (${pointsText(r.spent)})`,
        };
      }
      if (r.cure === LayOnHandsCureKind.DISEASE) {
        return {
          icon: 'favorite',
          text: ` curou a doença de ${target} com a Cura pelas Mãos (${pointsText(r.spent)})`,
        };
      }
      return {
        icon: 'favorite',
        text:
          r.healed === undefined
            ? ` usou a Cura pelas Mãos em ${target} (${pointsText(r.spent)})`
            : ` curou ${r.healed} PV de ${target} com a Cura pelas Mãos (${pointsText(r.spent)})`,
      };
    case 'feature:flexible-casting-creating-spell-slots':
      return {
        icon: 'auto_awesome',
        text: ` criou um espaço de ${circleLabel(r.slotLevel)} com a Conjuração Flexível (${pointsText(r.spent)} de feitiçaria)`,
      };
    case 'feature:flexible-casting-converting-spell-slot':
      return {
        icon: 'auto_awesome',
        text: ` converteu um espaço de ${circleLabel(r.slotLevel)} em ${pointsText(r.gained)} de feitiçaria`,
      };
    case 'feature:bardic-inspiration':
      return {
        icon: 'music_note',
        text:
          r.dieSides > 0
            ? ` deu um d${r.dieSides} da Inspiração de Bardo a ${target}`
            : ` deu a Inspiração de Bardo a ${target}`,
      };
    default:
      return { icon: 'bolt', text: ' usou um recurso da classe' };
  }
}

function attackText(e: CombatLogEntry): string {
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
function castTargetText(t: CombatLogSpellTarget, ctx: LogContext): string {
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
    out = `${the(who)} ${verb}${dc}${damage}`;
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
function castText(e: CombatLogEntry, ctx: LogContext): { text: string; card?: PoolCard } {
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
function reviveCastText(e: CombatLogEntry, ctx: LogContext): { text: string; note?: string } {
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

// ---- the spells that read hit points (E8-03) ----

/** "o Goblin 1", "Brisa": a creature has an article, a player's character does not. */
function subject(label: string, ctx: LogContext): string {
  return ctx.players.has(label) ? label : the(label);
}

function upFirst(text: string): string {
  return `${text.charAt(0).toUpperCase()}${text.slice(1)}`;
}

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

function hpCastText(e: CombatLogEntry, ctx: LogContext): { text: string; card?: PoolCard } {
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
  const out = (spell.targets ?? []).map((t) => {
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
    const facts: string[] = [];
    if (fx?.hitPointsBefore !== undefined) {
      facts.push(`${fx.hitPointsBefore} PV`);
    }
    if (spell.effectThreshold !== undefined) {
      facts.push(`limite de ${spell.effectThreshold}`);
    }
    const detail = facts.length > 0 ? ` (${facts.join(', ')})` : '';
    const healed =
      fx?.healed !== undefined && w.affected ? gainWords(spell.effectKind, fx.healed, fx.gain) : '';
    const reason =
      !w.affected && fx
        ? reasonWords(fx.reason, fx.hitPointsBefore, fx.poolLeft, spell.effectThreshold)
        : '';
    const result =
      healed || (w.affected ? w.present : `${w.present}${reason ? `: ${reason}` : ''}`);
    return `${prep}${detail}: ${result}`;
  });
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
  const rows: PoolCardRow[] = targets.map((t) => {
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
    const left = fx?.poolLeft;
    let math = '';
    if (w.affected && hp !== undefined && left !== undefined) {
      math = `${left + hp} − ${hp} = ${left} restam`;
    } else if (fx && !w.affected) {
      math = reasonWords(fx.reason, hp, left, undefined);
    }
    return {
      id: t.targetId,
      label,
      hitPoints: hp === undefined ? '' : `${hp} PV`,
      math,
      word: w.affected ? `${w.past}${condition ? ` · ${condition}` : ''}` : 'Não afetado',
      icon: w.icon,
      affected: w.affected,
    };
  });
  const roll = poolRollText(spell.poolRoll).replace(' · dado físico', '');
  const summary = `${roll} PV${spell.poolRoll.physical ? ', dado físico' : ''}. ${rows
    .map(
      (r) =>
        `${r.label}${r.hitPoints ? ` (${r.hitPoints})` : ''} ${r.affected ? `${r.word.toLowerCase()}${r.math ? `, ${r.math}` : ''}` : 'não é afetado'}.`,
    )
    .join(' ')}`;
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

function deathSaveText(e: CombatLogEntry): string {
  const s = e.deathSave;
  if (!s) {
    return ' faz um teste contra a morte';
  }
  // A table that hides the death saves (RN-24) tells everyone else only the result: no roll, no outcome, no counts.
  if (s.stable && !s.roll && s.outcome === DeathSaveOutcome.UNSPECIFIED) {
    return ' estabilizou';
  }
  const word =
    s.outcome === DeathSaveOutcome.REVIVED
      ? 'volta com 1 PV'
      : s.outcome === DeathSaveOutcome.CRITICAL_FAILURE
        ? 'falha crítica, conta duas falhas'
        : s.outcome === DeathSaveOutcome.FAILURE
          ? 'falha'
          : 'sucesso';
  // The d20 is the master's and the character's own player's; the others read the outcome.
  const roll = s.roll
    ? `rola o teste contra a morte: ${rollText(s.roll)}, `
    : 'faz um teste contra a morte: ';
  let out = ` ${roll}${word}`;
  if (s.outcome !== DeathSaveOutcome.REVIVED) {
    out += ` (${countsSentence(s.successes, s.failures)})`;
  }
  if (s.stable) {
    out += '. Estável: não rola mais';
  } else if (s.dying) {
    out += '. Morrendo: o mestre confirma a morte';
  }
  return out;
}

/** "está Envenenado e Derrubado", "ficou sem condições", and the concentration that ended. */
function conditionsText(e: CombatLogEntry): string {
  const parts: string[] = [];
  const names = e.conditions.map(conditionName).filter(Boolean);
  // An entry that only ended a concentration carries no conditions: it must
  // not read as "ficou sem condições".
  if (names.length > 0 || !e.concentrationEndedKey) {
    parts.push(names.length > 0 ? `ficou ${listNames(names)}` : 'ficou sem condições');
  }
  if (e.concentrationEndedKey) {
    parts.push('deixou de se concentrar');
  }
  return ` ${parts.join('; ')}`;
}

/** What a standard action reads as in the log, by its key. */
function actionText(e: CombatLogEntry): string {
  switch (e.key) {
    case 'standard:hide':
      return ' se esconde';
    case 'standard:search':
      return ' procura ao redor';
    case 'standard:ready':
      return ' se prepara';
    case 'standard:use-an-object':
      return ' usa um objeto';
    default:
      return ` usa ${e.keyNamePt || 'uma ação'}`;
  }
}

function hitPointsText(e: CombatLogEntry): string {
  const n = Math.abs(e.hitPointsDelta);
  const after = e.hitPointsAfter === undefined ? '' : `, agora com ${e.hitPointsAfter} PV`;
  if (e.hitPointsDelta < 0) {
    return ` perdeu ${n} PV, por ajuste do mestre${after}`;
  }
  if (e.hitPointsDelta > 0) {
    return ` recuperou ${n} PV, por ajuste do mestre${after}`;
  }
  return ` teve os PV ajustados pelo mestre${after}`;
}

/** Movement spent by number, in a combat without a map: "gastou 6,0 m de movimento". */
function spentText(e: CombatLogEntry): string {
  const length = e.distanceDft > 0 ? metersFixed(e.distanceDft / 10) : metersFixed(e.distanceFt);
  return ` gastou ${length} de movimento`;
}

/** A move: "anda 3,0 m", "saltou 4,5 m" or "saltou 1,8 m para cima". A long jump that landed in
 * difficult terrain reminds the master (only he gets the flag) of the SRD's Acrobacia check: the app rolls nothing. */
function movedText(e: CombatLogEntry): string {
  if (e.jump === JumpKind.HIGH) {
    return ` saltou ${metersFixed((e.jumpHeightDft || e.distanceDft) / 10)} para cima`;
  }
  const length = e.distanceDft > 0 ? metersFixed(e.distanceDft / 10) : metersText(e.distanceFt);
  if (e.jump === JumpKind.LONG) {
    return ` saltou ${length}${e.jumpRunningStart ? ', com corrida' : ''}${e.landingDifficult ? ' e caiu em terreno difícil. Acrobacia CD 10 ou cai Derrubado' : ''}`;
  }
  return ` anda ${length}${spentNote(e)}`;
}

/** What a walk spent when it is not the distance: ", gasta 6,2 m de movimento (terreno difícil)". The
 * reasons come from the SRD: another creature's space and rubble cost double, a drag halves the speed and a
 * prone creature crawls at double the cost. Nothing when the walk spent what it went (or the event is old). */
function spentNote(e: CombatLogEntry): string {
  const distance = e.distanceDft > 0 ? e.distanceDft : e.distanceFt * 10;
  if (e.spentDft <= 0 || e.spentDft === distance) {
    return '';
  }
  const factor = (e.moveDragging ? 2 : 1) * (e.moveCrawling ? 2 : 1);
  const reasons: string[] = [];
  if (e.spentDft > distance * factor) {
    reasons.push('terreno difícil');
  }
  if (e.moveDragging) {
    reasons.push('arrastando');
  }
  if (e.moveCrawling) {
    reasons.push('rastejando');
  }
  return `, gasta ${metersFixed(e.spentDft / 10)} de movimento${reasons.length > 0 ? ` (${reasons.join(', ')})` : ''}`;
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
function modeNote(m: CombatLogModeChange | undefined): string {
  if (!m) {
    return '';
  }
  const reason = m.reason ? ` — ${m.reason}` : '';
  return `${modeWord(m.mode)} (sugerido: ${modeWord(m.suggestedMode)})${reason}`;
}

/** The dice of each part of a damage, "Espada curta 1d6 (4) + 2 = 6", and the steps resistance took. */
function damageNotes(d: CombatLogDamage | undefined): string[] {
  if (!d) {
    return [];
  }
  const parts = d.parts.map((p) => {
    const dice = p.diceCount > 0 ? `${p.diceCount}d${p.diceSides}` : '';
    const faces = p.faces.length > 0 ? ` (${p.faces.join(', ')})` : '';
    const flat = p.flat !== 0 ? ` ${p.flat > 0 ? '+' : '−'} ${Math.abs(p.flat)}` : '';
    const total = p.sum + p.flat;
    const out = p.counted ? '' : ' — não conta';
    return `${p.labelPt} ${dice}${faces}${dice ? flat : flat.trim()} = ${total}${out}`.trim();
  });
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
  return [...parts, ...steps];
}

/** "Fúria de Toren começou", "Fúria de Toren acabou (não atacou nem sofreu dano)". */
function stateText(e: CombatLogEntry, st: CombatLogStateChange): string {
  const noun =
    st.kind === CombatantStateKind.RAGE
      ? 'Fúria'
      : st.kind === CombatantStateKind.HUNTERS_MARK_TARGET
        ? 'Marca'
        : st.kind === CombatantStateKind.DODGING
          ? 'Esquiva'
          : st.kind === CombatantStateKind.RECKLESS
            ? 'Ataque descuidado'
            : 'Estado';
  const who = e.actorLabel || e.targetLabel;
  const name = who ? `${noun} de ${who}` : noun;
  if (st.started) {
    return `${name} começou`;
  }
  const reason = stateReason(st.reason);
  return `${name} acabou${reason ? ` (${reason})` : ''}`;
}

function stateReason(key: string): string {
  switch (key) {
    case 'no_attack':
      return 'não atacou nem sofreu dano';
    case 'unconscious':
      return 'ficou inconsciente';
    case 'by_choice':
      return 'por escolha';
    case 'duration':
      return 'a duração acabou';
    case 'left':
      return 'a concentração ou o combate acabou';
    default:
      return key;
  }
}

/** "Brisa pediu Vantagem; o mestre aprovou". */
function modeAnsweredText(m: CombatLogModeChange): string {
  const asked = `pediu ${modeWord(m.mode)}`;
  return ` ${asked}; o mestre ${m.approved ? 'aprovou' : 'recusou'}`;
}

/** "do Hobgoblin", "de Brisa". */
function ofSubject(label: string, ctx: LogContext): string {
  return ctx.players.has(label)
    ? `de ${label}`
    : `${article(label) === 'a' ? 'da' : 'do'} ${label}`;
}

/** "surpreso" or "surpresa", by the name. */
function surprisedWord(label: string): string {
  return article(label) === 'a' ? 'surpresa' : 'surpreso';
}

/** "escondido" or "escondida", by the name. */
function hiddenWordOf(label: string): string {
  return article(label) === 'a' ? 'escondida' : 'escondido';
}

/** What each line of a contest or special action says (W7-X) and its icon: the sentence follows the actor, who is in bold. */
const CONTEST_LINES: Partial<
  Record<ContestLogLine, { icon: string; say: (w: ContestWords) => string }>
> = {
  [ContestLogLine.GRAPPLED]: { icon: 'pan_tool', say: (w) => ` agarrou ${w.target}` },
  [ContestLogLine.GRAPPLE_FAILED]: {
    icon: 'pan_tool',
    say: (w) => ` não conseguiu agarrar ${w.target}`,
  },
  [ContestLogLine.SHOVE_PRONE]: { icon: 'open_with', say: (w) => ` derrubou ${w.target}` },
  [ContestLogLine.SHOVE_PUSHED]: {
    icon: 'open_with',
    say: (w) => ` empurrou ${w.target} 1,5\u00a0m`,
  },
  [ContestLogLine.SHOVE_STAYS]: {
    icon: 'open_with',
    say: (w) => ` empurrou ${w.target}, que não saiu do lugar`,
  },
  [ContestLogLine.SHOVE_FAILED]: {
    icon: 'open_with',
    say: (w) => ` não conseguiu empurrar ${w.target}`,
  },
  [ContestLogLine.ESCAPED]: { icon: 'pan_tool', say: (w) => ` se soltou ${w.from}` },
  [ContestLogLine.ESCAPE_FAILED]: {
    icon: 'pan_tool',
    say: (w) => ` não conseguiu se soltar ${w.from}`,
  },
  [ContestLogLine.CLOSED]: {
    icon: 'pan_tool',
    say: (w) => ` teve a disputa com ${w.target} encerrada pelo mestre`,
  },
  [ContestLogLine.RELEASED]: { icon: 'pan_tool', say: (w) => ` soltou ${w.target}` },
  [ContestLogLine.HIDE_TRIED]: { icon: 'visibility_off', say: () => ' tentou se esconder' },
  [ContestLogLine.HIDE_APPLIED]: { icon: 'visibility_off', say: () => ' se escondeu' },
  [ContestLogLine.HIDE_REFUSED]: {
    icon: 'visibility_off',
    say: () => ' não conseguiu se esconder',
  },
  [ContestLogLine.HELPED]: { icon: 'handshake', say: (w) => ` ajudou ${w.target}` },
  [ContestLogLine.SURPRISED]: { icon: 'bolt', say: (w) => ` está ${w.surprised}` },
  [ContestLogLine.SURPRISE_CLEARED]: {
    icon: 'bolt',
    say: (w) => ` não está mais ${w.surprised}`,
  },
  [ContestLogLine.STOOD_UP]: { icon: 'accessibility_new', say: () => ' se levantou' },
  [ContestLogLine.HIDE_REVEALED]: {
    icon: 'visibility',
    say: (w) => ` não está mais ${w.hidden}`,
  },
};

/** The names a contest line is written with: the target ("o Hobgoblin", "Brisa"), "do Hobgoblin", and "surpresa". */
interface ContestWords {
  readonly target: string;
  readonly from: string;
  readonly surprised: string;
  readonly hidden: string;
}

/** A contest or a special action (W7-X): the sentence from the line the server named, never a total or a DC (RN-20). `null` for a line this app does not know. */
function contestText(e: CombatLogEntry, ctx: LogContext): { icon: string; text: string } | null {
  const line = e.contest ? CONTEST_LINES[e.contest.line] : undefined;
  if (!line) {
    return null;
  }
  const label = e.targetLabel || 'alguém';
  return {
    icon: line.icon,
    text: line.say({
      target: subject(label, ctx),
      from: ofSubject(label, ctx),
      surprised: surprisedWord(e.actorLabel),
      hidden: hiddenWordOf(e.actorLabel),
    }),
  };
}

/** What a line builder is given: the entry, the fields every line shares, the combat's name and who is reading. */
interface LineArgs {
  readonly e: CombatLogEntry;
  readonly base: { id: string; actor: string; hidden: boolean; undoable: boolean };
  readonly encounterName: string;
  readonly ctx: LogContext;
}

/** One builder per kind of entry, so a new kind is one more row and no function grows with it. A builder answers `null` for an
 * entry it cannot write (an effect with no effect in it). */
const LINE_BUILDERS: ReadonlyMap<CombatLogKind, (args: LineArgs) => LogLine | null> = new Map([
  [
    CombatLogKind.COMBAT_BEGUN,
    ({ base, encounterName, ctx }) => {
      return {
        ...base,
        icon: 'flag',
        actor: '',
        text:
          (encounterName ? `Combate iniciado: ${encounterName}` : 'Combate iniciado') +
          (ctx.theatre ? ', sem mapa (teatro da mente)' : ''),
      };
    },
  ],
  // A spell attack (Raio de Fogo) has the sparkles E6-15 draws for spells.
  [
    CombatLogKind.ATTACK,
    ({ e, base }) => {
      return {
        ...base,
        icon: e.key.startsWith('spell:') ? 'auto_awesome' : 'swords',
        text: attackText(e),
        roll: pairOf(e.attackRoll, 'd20 do ataque'),
        notes: [modeNote(e.modeChange), luckyText(e.attackRoll), ...damageNotes(e.damage)].filter(
          Boolean,
        ),
      };
    },
  ],
  [
    CombatLogKind.ACTION,
    ({ e, base }) => {
      return {
        ...base,
        icon: e.key === 'standard:hide' ? 'visibility_off' : 'bolt',
        text: actionText(e),
      };
    },
  ],
  [
    CombatLogKind.MOVED,
    ({ e, base, ctx }) => {
      return {
        ...base,
        icon: ctx.theatre ? 'directions_run' : 'arrow_forward',
        text: ctx.theatre ? spentText(e) : movedText(e),
      };
    },
  ],
  // The server puts the NPC whose hit points changed in the target.
  [
    CombatLogKind.HIT_POINTS_ADJUSTED,
    ({ e, base }) => {
      return {
        ...base,
        actor: e.actorLabel || e.targetLabel,
        icon: 'healing',
        text: hitPointsText(e),
      };
    },
  ],
  [
    CombatLogKind.REVEAL_CHANGED,
    ({ e, base }) => {
      return {
        ...base,
        actor: e.actorLabel || e.targetLabel,
        icon: e.nowHidden ? 'visibility_off' : 'visibility',
        text: e.nowHidden ? ' foi escondido dos jogadores' : ' foi revelado aos jogadores',
      };
    },
  ],
  [
    CombatLogKind.COMBAT_ENDED,
    ({ base }) => {
      return { ...base, icon: 'flag', actor: '', text: 'Combate encerrado' };
    },
  ],
  [
    CombatLogKind.SPELL_CAST,
    ({ e, base, ctx }) => {
      if (e.spell?.effectKind === SpellEffectKind.REVIVE) {
        return { ...base, icon: 'favorite', ...reviveCastText(e, ctx) };
      }
      const cast = castText(e, ctx);
      // "Sortudo: 1 → 14": a spell attack's d20 the halfling's Lucky rolled again, or a halfling player's save the app
      // rolled again by itself (the server sends the roll to whoever may read it), kept by the event so a reload still
      // finds it.
      const lucky = (e.spell?.targets ?? [])
        .flatMap((t) => [luckyText(t.attackRoll), luckyText(t.save?.roll)])
        .filter(Boolean);
      return {
        ...base,
        icon: 'auto_awesome',
        text: cast.text,
        card: cast.card,
        ...(lucky.length ? { notes: lucky } : {}),
      };
    },
  ],
  // The master brought a dead character back: the server puts the combatant in the target and sends no actor.
  [
    CombatLogKind.CHARACTER_REVIVED,
    ({ e, base }) => {
      return {
        ...base,
        actor: '',
        icon: 'favorite',
        text: `O mestre reviveu ${e.targetLabel || 'alguém'}`,
      };
    },
  ],
  [
    CombatLogKind.REACTION,
    ({ e, base }) => {
      const slot = e.spell?.slot;
      return {
        ...base,
        icon: 'shield',
        text: ` conjura ${e.keyNamePt || 'uma magia'}${slot ? ` (${circleLabel(slot.level)})` : ''}, com a reação`,
      };
    },
  ],
  [
    CombatLogKind.DEATH_SAVE,
    ({ e, base }) => {
      return { ...base, icon: 'heart_broken', text: deathSaveText(e) };
    },
  ],
  // The server puts the character in the target.
  [
    CombatLogKind.DEATH_CONFIRMED,
    ({ e, base }) => {
      return { ...base, actor: e.targetLabel || e.actorLabel, icon: 'close', text: ' morreu' };
    },
  ],
  [
    CombatLogKind.CONDITIONS_CHANGED,
    ({ e, base }) => {
      return {
        ...base,
        actor: e.targetLabel || e.actorLabel,
        icon: 'label',
        text: conditionsText(e),
      };
    },
  ],
  // A trap that fired is public: everyone reads the line (the dice stay with the master and the creature's player).
  [
    CombatLogKind.TRAP_TRIGGERED,
    ({ e, base, ctx }) => {
      return { ...base, icon: 'warning', actor: '', text: trapLogText(e.trap, ctx.master) };
    },
  ],
  // A member of a joint turn ended their part and the turn goes on.
  [
    CombatLogKind.TURN_PART_ENDED,
    ({ e, base }) => {
      return {
        ...base,
        icon: 'flag',
        text: ` encerrou a parte ${article(e.actorLabel) === 'a' ? 'dela' : 'dele'}`,
      };
    },
  ],
  [
    CombatLogKind.WILD_SHAPE,
    ({ e, base }) => {
      return { ...base, icon: 'pets', text: wildShapeText(e) };
    },
  ],
  // The master offered it, in a combat without a map: the mover is the actor and the reactor the target.
  [
    CombatLogKind.OPPORTUNITY_OFFERED,
    ({ e, base }) => {
      return { ...base, icon: 'swords', text: ` saiu do alcance de ${e.targetLabel || 'alguém'}` };
    },
  ],
  [
    CombatLogKind.HIT_RIDER,
    ({ e, base }) => {
      return { ...base, icon: 'sports_martial_arts', text: hitRiderText(e) };
    },
  ],
  // A move opened a closed door (RN-26): "Toren abriu a porta." The server sends the line only to who saw or remembers the door.
  [
    CombatLogKind.DOOR_OPENED,
    ({ base }) => {
      return { ...base, icon: 'door_open', text: ' abriu a porta' };
    },
  ],
  [
    CombatLogKind.EFFECT_ENDED,
    ({ e, base, ctx }) => {
      return {
        ...base,
        icon: e.effectEnd?.effect === CombatEffect.SHIELD ? 'shield' : 'favorite_border',
        actor: '',
        text: effectEndedText(e, ctx),
      };
    },
  ],
  [
    CombatLogKind.EFFECT,
    ({ e, base, ctx }) => {
      return e.effect ? effectLine(base, e.effect, ctx) : null;
    },
  ],
  [
    CombatLogKind.EXHAUSTION,
    ({ e, base, ctx }) => {
      return e.effect
        ? { ...base, icon: 'battery_alert', actor: '', text: exhaustionLogText(e.effect, ctx) }
        : null;
    },
  ],
  [
    CombatLogKind.RESOURCE,
    ({ e, base }) => {
      const r = resourceText(e);
      return { ...base, icon: r.icon, text: r.text };
    },
  ],
  // A reaction was answered or closed (PM-04): one line, written on the server for who reads it (the master's has the
  // numbers of the NPCs, the players' never names a reactor they do not see).
  [
    CombatLogKind.REACTION_WINDOW,
    ({ e, base }) => {
      return { ...base, icon: 'bolt', actor: '', text: e.reactionTextPt };
    },
  ],
  // The master put monsters in the combat (RN-29): the line is his alone, with the hit points and the dice they were rolled with.
  [
    CombatLogKind.MONSTERS_ADDED,
    ({ e, base }) => {
      return { ...base, hidden: true, icon: 'pets', actor: '', text: monstersAddedText(e) };
    },
  ],
  // The master took an extra out of a damage; the reason is only for him and the attacker's player.
  [
    CombatLogKind.DAMAGE_PART_REMOVED,
    ({ e, base }) => {
      return {
        ...base,
        icon: 'remove_circle_outline',
        actor: '',
        text: `O mestre tirou ${e.keyNamePt ? the(e.keyNamePt) : 'um extra'}${e.reason ? `: “${e.reason}”` : ''}`,
      };
    },
  ],
  [
    CombatLogKind.STATE_CHANGED,
    ({ e, base }) => {
      return e.state
        ? { ...base, icon: 'local_fire_department', actor: '', text: stateText(e, e.state) }
        : null;
    },
  ],
  [
    CombatLogKind.CONTEST,
    ({ e, base, ctx }) => {
      const line = contestText(e, ctx);
      return line ? { ...base, icon: line.icon, text: line.text } : null;
    },
  ],
  [
    CombatLogKind.ROLL_MODE_ANSWERED,
    ({ e, base }) => {
      return e.modeChange
        ? {
            ...base,
            icon: 'casino',
            text: modeAnsweredText(e.modeChange),
            notes: e.modeChange.reason ? [`Motivo: “${e.modeChange.reason}”`] : undefined,
          }
        : null;
    },
  ],
]);

/** The line of one entry, or `null` for a kind this app doesn't know. */
export function logLine(
  e: CombatLogEntry,
  encounterName = '',
  ctx: LogContext = NO_CONTEXT,
): LogLine | null {
  const base = { id: e.id, actor: e.actorLabel, hidden: e.hidden, undoable: e.undoable };
  return LINE_BUILDERS.get(e.kind)?.({ e, base, encounterName, ctx }) ?? null;
}

/** Who an effect line is about, after the preposition ("em"/"a") or alone: "em Brisa e no Goblin 2", "Brisa e o Goblin 2". */
function effectTargets(fx: CombatLogEffect, ctx: LogContext, prep: '' | 'em' | 'a' = ''): string {
  const names = fx.targetLabels.map((label) => {
    if (prep === '') {
      return subject(label, ctx);
    }
    if (ctx.players.has(label)) {
      return `${prep} ${label}`;
    }
    const feminine = article(label) === 'a';
    const joined = prep === 'em' ? (feminine ? 'na' : 'no') : feminine ? 'à' : 'ao';
    return `${joined} ${label}`;
  });
  if (names.length === 0) {
    return prep === '' ? 'alguém' : `${prep} alguém`;
  }
  return listNames(names);
}

type EffectLine = (fx: CombatLogEffect, ctx: LogContext) => string;

/** "(d20 12, total 15 contra CD 14)": the numbers of a saving throw against an effect, only for the master. */
function effectSaveNumbers(fx: CombatLogEffect, ctx: LogContext): string {
  if (!ctx.master || fx.total === undefined || fx.dc === undefined) {
    return '';
  }
  // "20 natural" is information only: a saving throw has no automatic success or failure (SRD 5.1).
  const natural = fx.d20 !== undefined ? naturalMark(fx.d20) : '';
  return ` (${fx.d20 !== undefined ? `d20 ${fx.d20}, ` : ''}${natural ? `${natural}, ` : ''}total ${fx.total} contra CD ${fx.dc})`;
}

function effectSaveWords(fx: CombatLogEffect): string {
  return fx.abilityNamePt ? ` de ${fx.abilityNamePt}` : '';
}

const EFFECT_LINES: Partial<Record<CombatLogEffectChange, EffectLine>> = {
  [CombatLogEffectChange.ADDED]: (fx, ctx) =>
    `${fx.sourceNamePt} foi aplicado ${effectTargets(fx, ctx, 'em')}`,
  [CombatLogEffectChange.ENDED]: (fx, ctx) =>
    `${fx.sourceNamePt} acabou ${effectTargets(fx, ctx, 'em')}`,
  [CombatLogEffectChange.DURATION_CHANGED]: (fx, ctx) =>
    `A duração de ${fx.sourceNamePt} ${effectTargets(fx, ctx, 'em')} mudou`,
  [CombatLogEffectChange.SAVED]: (fx, ctx) =>
    `${upFirst(effectTargets(fx, ctx))} passou no teste${effectSaveWords(fx)} contra ${fx.sourceNamePt}${effectSaveNumbers(fx, ctx)}`,
  [CombatLogEffectChange.SAVE_FAILED]: (fx, ctx) =>
    `${upFirst(effectTargets(fx, ctx))} falhou no teste${effectSaveWords(fx)} contra ${fx.sourceNamePt}${effectSaveNumbers(fx, ctx)}`,
  [CombatLogEffectChange.SAVE_SKIPPED]: (fx, ctx) =>
    `${upFirst(effectTargets(fx, ctx))} não fez o teste${effectSaveWords(fx)} contra ${fx.sourceNamePt}`,
  [CombatLogEffectChange.DAMAGE]: (fx, ctx) =>
    `${fx.sourceNamePt} causou ${fx.amount} de dano${fx.damageTypePt ? ` ${fx.damageTypePt}` : ''} ${effectTargets(fx, ctx, 'a')}`,
  [CombatLogEffectChange.TEMP_HP]: (fx, ctx) =>
    `${fx.sourceNamePt} deu ${fx.amount} PV temporários ${effectTargets(fx, ctx, 'a')}`,
};

/** The line of a lasting effect, with the d20 the halfling's Lucky rolled again on its saving throw as a note
 * ("Sortudo: 1 → 14", sent only to whoever may read the roll). */
function effectLine(
  base: Pick<LogLine, 'id' | 'actor' | 'hidden' | 'undoable'>,
  fx: CombatLogEffect,
  ctx: LogContext,
): LogLine {
  const lucky = luckyText(fx);
  return {
    ...base,
    icon: 'auto_awesome',
    actor: '',
    text: effectLogText(fx, ctx),
    ...(lucky ? { notes: [lucky] } : {}),
  };
}

/** The line of a lasting effect: "Heroísmo deu 5 PV temporários a Brisa", "Brisa passou no teste de Destreza contra Teia". */
function effectLogText(fx: CombatLogEffect, ctx: LogContext): string {
  const named = fx.sourceNamePt ? fx : { ...fx, sourceNamePt: 'Um efeito' };
  const line = EFFECT_LINES[fx.change];
  return line ? line(named, ctx) : `${named.sourceNamePt}: ${effectTargets(fx, ctx)}`;
}

/** "Brisa agora tem exaustão de nível 2". */
function exhaustionLogText(fx: CombatLogEffect, ctx: LogContext): string {
  const who = upFirst(effectTargets(fx, ctx));
  if (fx.reason === 'frenzy') {
    // The end of a frenzied rage (SRD 5.1, Berserker): "Frenesi: +1 nível de exaustão · Ragna agora tem exaustão de nível 1".
    return `Frenesi: +1 nível de exaustão · ${who} agora tem exaustão de nível ${fx.exhaustionLevel}`;
  }
  return fx.exhaustionLevel > 0
    ? `${who} agora tem exaustão de nível ${fx.exhaustionLevel}`
    : `${who} ficou sem exaustão`;
}

/** "O Escudo Arcano de Pensantus acabou", "A Ajuda de Sálvia acabou" and, for the master, who has the numbers
 * (the server sends them to nobody else: zero), ": PV 43 → 38". */
function effectEndedText(e: CombatLogEntry, ctx: LogContext): string {
  const end = e.effectEnd;
  const label = e.actorLabel || e.targetLabel || 'alguém';
  const of = ctx.players.has(label)
    ? `de ${label}`
    : `${article(label) === 'a' ? 'da' : 'do'} ${label}`;
  if (end?.effect === CombatEffect.SHIELD) {
    return `O Escudo Arcano ${of} acabou`;
  }
  if (end?.effect !== CombatEffect.AID) {
    return `Um efeito ${of} acabou`;
  }
  const base = `A Ajuda ${of} acabou`;
  if (!ctx.master || end.hitPointsMaxBefore <= 0) {
    return base;
  }
  return end.hitPointsBefore === end.hitPointsAfter
    ? `${base}: PV ${end.hitPointsAfter}, máximo ${end.hitPointsMaxBefore} → ${end.hitPointsMaxAfter}`
    : `${base}: PV ${end.hitPointsBefore} → ${end.hitPointsAfter}`;
}

/** "Bandido 1: 9 PV (2d8 + 2: 3, 4)", or "Bandido 1: 11 PV (média)": what the master reads of the monsters that came in. */
export function monstersAddedText(e: CombatLogEntry): string {
  const n = e.monsters.length;
  const each = e.monsters.map((m) => {
    if (!m.rolled) {
      return `${m.label}: ${m.hitPoints} PV (média)`;
    }
    const dice = m.dice.replace(/([+-])/, ' $1 ');
    return `${m.label}: ${m.hitPoints} PV (${dice}: ${m.faces.join(', ')})`;
  });
  return `Você pôs ${n === 1 ? 'um monstro' : `${n} monstros`} no combate. ${each.join('; ')}`;
}

/** "virou o Lobo", "voltou à forma normal": a druid's change of form. The damage that passed on to the druid is only in the
 * entry the master and the druid's player get (RN-20); everyone else reads that the druid is back and why, with no number. */
function wildShapeText(e: CombatLogEntry): string {
  const w = e.wildShape;
  if (!w) {
    return ' mudou de forma';
  }
  if (w.started) {
    return ` virou ${the(w.beastNamePt)}`;
  }
  const she = article(e.actorLabel) === 'a' ? 'ela' : 'ele';
  switch (w.endReason) {
    case WildShapeEndReason.DAMAGE:
      return ` voltou à forma normal: ${the(w.beastNamePt)} caiu a 0 PV${w.carriedDamage > 0 ? ` e ${w.carriedDamage} de dano passaram para ${she}` : ''}`;
    case WildShapeEndReason.MASTER:
      return ` voltou à forma normal, porque o mestre levou ${the(w.beastNamePt)} a 0 PV`;
    case WildShapeEndReason.ZERO_HP:
      return ' voltou à forma normal, ao cair a 0 PV';
    case WildShapeEndReason.UNCONSCIOUS:
      return ' voltou à forma normal, ao ficar inconsciente';
    default:
      return ' voltou à forma normal';
  }
}

/** The rounds as the log draws them: latest first, the current round
 * "em andamento". Every round after the first ends with "A rodada N
 * começou", the earliest line of its group. */
export function logGroups(
  rounds: readonly CombatLogRound[],
  currentRound: number,
  encounterName = '',
  ctx: LogContext = NO_CONTEXT,
): LogGroup[] {
  return rounds.map((r) => {
    const lines = r.entries
      .map((e) => logLine(e, encounterName, ctx))
      .filter((l): l is LogLine => l !== null);
    if (r.round > 1) {
      lines.push({
        id: `round-${r.round}`,
        icon: 'skip_next',
        actor: '',
        text: `A rodada ${r.round} começou`,
        hidden: false,
        undoable: false,
      });
    }
    return {
      round: r.round,
      // Round 0 is before the combat begins: the monsters the master put in while it was being set up.
      title: r.round === 0 ? 'Antes do combate' : `Rodada ${r.round}`,
      status:
        r.round === 0
          ? currentRound === 0
            ? 'em preparação'
            : 'encerrada'
          : r.round >= currentRound
            ? 'em andamento'
            : 'encerrada',
      lines,
    };
  });
}

/** The newest line of all, for the collapsed log. */
export function latestLine(groups: readonly LogGroup[]): LogLine | null {
  for (const g of groups) {
    const first = g.lines.find((l) => !l.id.startsWith('round-'));
    if (first) {
      return first;
    }
  }
  return null;
}

/** How many entries the log has, for "9 entradas" (the synthetic "A rodada
 * começou" lines don't count). */
export function entryCount(groups: readonly LogGroup[]): number {
  return groups.reduce((n, g) => n + g.lines.filter((l) => !l.id.startsWith('round-')).length, 0);
}

/** The entry the undo would take back, named for the confirmation:
 * "o ataque do Capitão Goblin ao Toren (5 de dano)". */
export function undoLabel(e: CombatLogEntry): string {
  switch (e.kind) {
    case CombatLogKind.ATTACK: {
      const target = e.targetLabel || 'alguém';
      const to = article(target) === 'a' ? 'à' : 'ao';
      const damage =
        e.damage &&
        e.damage.status !== PendingDamageStatus.AWAITING_ROLL &&
        e.outcome !== AttackOutcome.MISS
          ? ` (${e.damage.amount} de dano)`
          : e.outcome === AttackOutcome.MISS
            ? ' (errou)'
            : '';
      return `o ataque ${article(e.actorLabel) === 'a' ? 'da' : 'do'} ${e.actorLabel} ${to} ${target}${damage}`;
    }
    case CombatLogKind.ACTION:
      return `a ação de ${e.actorLabel}${e.keyNamePt ? ` (${e.keyNamePt})` : ''}`;
    case CombatLogKind.HIT_POINTS_ADJUSTED:
      return `o ajuste de PV de ${e.actorLabel || e.targetLabel}`;
    case CombatLogKind.SPELL_CAST:
      return `a magia ${e.keyNamePt || ''} ${article(e.actorLabel) === 'a' ? 'da' : 'do'} ${e.actorLabel}`.replace(
        '  ',
        ' ',
      );
    case CombatLogKind.REACTION:
      return `a reação ${article(e.actorLabel) === 'a' ? 'da' : 'do'} ${e.actorLabel}`;
    case CombatLogKind.OPPORTUNITY_OFFERED:
      return `a oferta de ataque de oportunidade a ${e.targetLabel || 'alguém'}`;
    case CombatLogKind.DEATH_SAVE:
      return `o teste contra a morte ${article(e.actorLabel) === 'a' ? 'da' : 'do'} ${e.actorLabel}`;
    case CombatLogKind.CONDITIONS_CHANGED:
      return `a mudança de condições ${article(e.targetLabel) === 'a' ? 'da' : 'do'} ${e.targetLabel}`;
    default:
      return 'a última ação';
  }
}

/** The last entry the master may undo, from the groups' source rounds. */
export function undoableEntry(rounds: readonly CombatLogRound[]): CombatLogEntry | null {
  for (const r of rounds) {
    const e = r.entries.find((x) => x.undoable);
    if (e) {
      return e;
    }
  }
  return null;
}

/** The first `max` lines of the log, latest first, keeping the rounds they
 * belong to: the player's panel on a laptop shows a short stretch and
 * "Ver o registro todo" opens the rest. */
export function truncateGroups(groups: readonly LogGroup[], max: number): LogGroup[] {
  const out: LogGroup[] = [];
  let left = max;
  for (const g of groups) {
    if (left <= 0) {
      break;
    }
    const lines = g.lines.slice(0, left);
    left -= lines.length;
    out.push({ ...g, lines });
  }
  return out;
}

/** The line of an answered monk rider: "Toren usou a Técnica da Mão Aberta (Derrubar) em Goblin: ...". The roll and the DC only come for the master and the monk's player. */
function hitRiderText(e: CombatLogEntry): string {
  const r = e.hitRider;
  const target = e.targetLabel || 'alguém';
  if (!r || r.choice === HitRiderChoice.DECLINE) {
    return ` dispensou a técnica em ${target}`;
  }
  const names: Partial<Record<HitRiderChoice, string>> = {
    [HitRiderChoice.PRONE]: 'a Técnica da Mão Aberta (Derrubar)',
    [HitRiderChoice.PUSH]: 'a Técnica da Mão Aberta (Empurrar até 4,5 m)',
    [HitRiderChoice.NO_REACTIONS]:
      'a Técnica da Mão Aberta (sem reações até o fim do próximo turno)',
    [HitRiderChoice.STUN]: 'o Golpe Atordoante',
  };
  const roll = r.numbers ? ` (1d20 ${r.d20} = ${r.saveTotal} contra CD ${r.dc})` : '';
  let result = '';
  if (r.result === HitRiderLogResult.SAVED) {
    result = `: ${target} passou no teste${roll}`;
  } else if (r.result === HitRiderLogResult.FAILED) {
    result = `: ${target} falhou no teste${roll}`;
    if (r.choice === HitRiderChoice.PUSH) {
      result += '; o mestre posiciona a criatura até 4,5 m mais longe';
    }
  }
  return ` usou ${names[r.choice] ?? 'a técnica'} em ${target}${result}`;
}
