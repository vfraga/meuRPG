import { article } from '../format/article';
import {
  AttackOutcome,
  PendingDamageStatus,
  SaveOutcome,
  type TrapCaught,
  type TrapFiring,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type TrapActivity, TrapSearchSkill } from '../../../gen/meurpg/play/v1/traps_pb';
import { rollText } from '../combat/combat-dice';
import { luckyText } from '../combat/lucky';
import { conditionName } from '../combat/conditions';
import { listNames } from '../maps/scene-clues';
import { skillKeyName } from './trap-search';

/** One line of what traps did, ready to draw: an icon, the actor in bold and the rest of the sentence. */
export interface TrapLine {
  readonly id: string;
  readonly icon: string;
  /** Who (bold); empty for a line about the trap itself. */
  readonly actor: string;
  /** What follows the actor, starting with a space when there is an actor. */
  readonly text: string;
}

function attackWords(c: TrapCaught): string[] {
  return c.attacks.map((a) => {
    const word =
      a.outcome === AttackOutcome.CRITICAL_HIT
        ? 'acerto crítico'
        : a.outcome === AttackOutcome.MISS
          ? 'o ataque errou'
          : 'o ataque acertou';
    // The dice are the master's and the target's own player's: everyone else gets the word.
    const dice = a.roll
      ? ` (${rollText(a.roll)}${a.targetArmorClass ? ` contra CA ${a.targetArmorClass}` : ''})`
      : '';
    return `${word}${dice}`;
  });
}

function saveWords(c: TrapCaught, master: boolean): string[] {
  const saves = c.saves.length > 0 ? c.saves : c.save ? [c.save] : [];
  return saves.map((s) => {
    const word = s.outcome === SaveOutcome.SAVED ? 'passou no teste' : 'falhou no teste';
    // "Sortudo: 1 → 14": a halfling player's 1 the app rolled again by itself, beside the dice that have the new face.
    const lucky = luckyText(s.roll);
    const dice = s.roll
      ? ` (${rollText(s.roll)}${master && s.dc ? ` contra CD ${s.dc}` : ''}${lucky ? `; ${lucky}` : ''})`
      : '';
    return `${word}${dice}`;
  });
}

function damageWords(c: TrapCaught, master: boolean): string[] {
  return c.damages.map((d) => {
    const type = d.damageTypePt ? ` de ${d.damageTypePt}` : ' de dano';
    const half = d.half ? ' (metade)' : '';
    const state =
      d.status === PendingDamageStatus.ROLLED
        ? master
          ? ', esperando você aplicar'
          : ', esperando o mestre aplicar'
        : d.status === PendingDamageStatus.DISCARDED
          ? ', descartado'
          : d.targetDefeated
            ? ', caiu'
            : '';
    return `${d.amount}${type}${half}${state}`;
  });
}

/** What the trap did to one creature, as the sentence after its name ("caiu na armadilha Fosso escondido: 7
 * de concussão, esperando o mestre aplicar"). A viewer who may not read the dice gets the words only. */
export function caughtText(c: TrapCaught, trapName: string, master: boolean): string {
  const parts = [...attackWords(c), ...saveWords(c, master), ...damageWords(c, master)];
  const conditions = c.conditionKeys.map(conditionName).filter(Boolean);
  if (conditions.length > 0) {
    parts.push(`ficou ${listNames(conditions)}`);
  }
  const head = `caiu na armadilha ${trapName || 'sem nome'}`;
  return parts.length > 0 ? ` ${head}: ${parts.join('; ')}` : ` ${head}`;
}

/** The lines of one firing: the creatures it caught, then the trap's own line ("A armadilha X foi disparada. Agora todos a veem."). */
export function firingLines(
  firing: TrapFiring,
  id: string,
  master: boolean,
  withTrapLine = true,
): TrapLine[] {
  const lines = firing.caught.map((c, i): TrapLine => ({
    id: `${id}:${c.targetId}:${i}`,
    icon: 'arrow_downward',
    actor: c.targetLabel || 'Alguém',
    text: caughtText(c, firing.name, master),
  }));
  if (withTrapLine) {
    lines.push({
      id: `${id}:public`,
      icon: 'warning',
      actor: '',
      text: `A armadilha ${firing.name} foi disparada. Agora todos a veem.`,
    });
  }
  return lines;
}

/** The skill a search rolled, by name: the two that always find, or the one named by the key of an "Outra perícia…" search. */
function skillName(skill: TrapSearchSkill, otherKey = ''): string {
  if (skill === TrapSearchSkill.OTHER) {
    return otherKey ? skillKeyName(otherKey) : 'Outra perícia';
  }
  return skill === TrapSearchSkill.PERCEPTION ? 'Percepção' : 'Investigação';
}

/** One activity line (a firing, a search or a notice) as the sentences the "Registro" shows, newest last. */
export function activityLines(a: TrapActivity, master: boolean): TrapLine[] {
  if (a.firing) {
    return firingLines(a.firing, a.id, master);
  }
  if (a.search) {
    const s = a.search;
    // A d20 the halfling's Lucky rolled again says so beside it: "1d20 (14) + 4 = 18; Sortudo: 1 → 14".
    const rolls = [s.roll, s.secondRoll]
      .filter((r) => r !== undefined)
      .map((r) => [rollText(r), luckyText(r)].filter(Boolean).join('; '));
    const found = s.foundNames.filter(Boolean);
    const outcome =
      found.length > 0
        ? `achou ${found.length === 1 ? 'a armadilha' : 'as armadilhas'} ${listNames(found)}`
        : 'não achou nada';
    return [
      {
        id: a.id,
        icon: 'search',
        actor: s.characterName || 'Alguém',
        text: ` procurou armadilhas (${skillName(s.skill, s.otherSkillKey)}): ${rolls.join(' e ')}, e ${outcome}`,
      },
    ];
  }
  if (a.notice) {
    const n = a.notice;
    return [
      {
        id: a.id,
        icon: 'visibility',
        actor: listNames(n.characterNames.filter(Boolean)) || 'Alguém',
        text: ` notou a armadilha${n.trapName ? ` ${n.trapName}` : ''} ao passar`,
      },
    ];
  }
  return [];
}

/** What one creature's part of a firing was, short: "7 de concussão, esperando o mestre aplicar". */
function caughtBrief(c: TrapCaught, master: boolean): string {
  const parts = [...attackWords(c), ...saveWords(c, master), ...damageWords(c, master)];
  const conditions = c.conditionKeys.map(conditionName).filter(Boolean);
  if (conditions.length > 0) {
    parts.push(`ficou ${listNames(conditions)}`);
  }
  return parts.join('; ');
}

/** The combat log's one line for a firing (E9-08 F): "A armadilha Fosso escondido foi disparada: Toren caiu (7 de
 * concussão, esperando o mestre aplicar)". Every player reads it (a trap that fired is public); the dice are the
 * master's and the creature's own player's, and no DC reaches anyone else. */
export function trapLogText(firing: TrapFiring | undefined, master: boolean): string {
  if (!firing) {
    return 'Uma armadilha disparou.';
  }
  const name = firing.name || 'sem nome';
  if (firing.caught.length === 0) {
    return `A armadilha ${name} foi disparada. Ninguém estava na área.`;
  }
  const who = firing.caught.map((c) => {
    const brief = caughtBrief(c, master);
    return `${c.targetLabel || 'alguém'} caiu${brief ? ` (${brief})` : ''}`;
  });
  return `A armadilha ${name} foi disparada: ${who.join('; ')}.`;
}

/** What the player reads on their own turn after the trap caught them (E9-08 E): "Você caiu na armadilha Fosso escondido",
 * what it did to them with their own dice, no DC, and whether the damage still waits for the master. */
export interface FallNote {
  readonly title: string;
  readonly detail: string;
  readonly waiting: boolean;
}

/** `combatantId` is the player's own combatant: a creature's `character_id` is its owner's, so the match is by combatant. */
export function fallNote(firing: TrapFiring, combatantId: string, who = ''): FallNote | null {
  const mine = firing.caught.find((c) => c.targetId === combatantId);
  if (!mine) {
    return null;
  }
  const brief = caughtBrief(mine, false).replace(/, esperando o mestre aplicar/g, '');
  return {
    // The player's own character is "Você"; one of their creatures is named ("O Lobo atroz 1 caiu…").
    title: `${who ? `${article(who) === 'a' ? 'A' : 'O'} ${who}` : 'Você'} caiu na armadilha ${firing.name || 'sem nome'}.`,
    detail: brief ? `${brief.charAt(0).toUpperCase()}${brief.slice(1)}.` : '',
    waiting: mine.damages.some((d) => d.status === PendingDamageStatus.ROLLED),
  };
}
