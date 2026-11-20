import { DiceMode } from '../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  type GetTurnOptionsResponse,
  type ReadiedAction,
  ReadiedKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { type AdvantageSource, RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import type { SpellDetails } from '../../../gen/meurpg/rules/v1/rules_pb';
import { article } from '../format/article';
import { tight } from '../format/text';
import { metersText } from '../units';
import { spellKind } from './cast-flow';
import type { ReactionAnswer } from './combat-client-types';
import { poolDice } from './hp-effects';
import type { ReleasePlan } from './readied-release';
import { orNormal } from './roll-mode';

/**
 * The roll step of a readied action released from its window (#938, boards 938-01 to 938-03): at a table where the dice may be typed,
 * the use of a held action that rolls is two steps of the same sheet, and nothing is sent before the last button of the second. Only a
 * held weapon attack, a held spell attack and a held pool spell (Sono, in a combat without a map) roll. Pure functions, tested without
 * a DOM: what the step asks of the readier's own turn options, the one line of what is released and the answer typed dice make.
 */

/** The d20 of a held attack: one die, or the pair of advantage or disadvantage the server suggests for the chosen creature. */
export interface AttackRelease {
  readonly kind: 'attack';
  readonly name: string;
  readonly bonus: number;
  /** One typed d20, or the two of advantage or disadvantage. */
  readonly faces: 1 | 2;
  /** Which of the pair counts. */
  readonly combine: 'higher' | 'lower';
  readonly mode: RollMode;
  /** The circumstances behind the mode, as the server lists them for the readier. */
  readonly sources: readonly AdvantageSource[];
  /** "Role 1d20 para Espada longa (+6)". */
  readonly label: string;
  readonly hint: string;
  readonly totalNote: string;
}

/** The pool of dice of a held spell that rolls one (Sono, Leque Cromático): the sum of its dice at the slot it was held with. */
export interface PoolRelease {
  readonly kind: 'pool';
  readonly name: string;
  readonly min: number;
  readonly max: number;
  /** "Role 5d8 para Sono: some os dados". */
  readonly label: string;
  readonly hint: string;
  readonly totalNote: string;
}

export type ReleaseRoll = AttackRelease | PoolRelease;

/** What the step is made from. */
export interface ReleaseRollInput {
  readonly readied: ReadiedAction | undefined;
  /** The readier's own `GetTurnOptions`; `null` until it is read. */
  readonly options: GetTurnOptionsResponse | null;
  readonly plan: ReleasePlan | null;
  /** The creature a held attack is aimed at. */
  readonly target: string | null;
  /** The held spell's details, for the pool of its dice; `null` until they are read. */
  readonly details: SpellDetails | null;
}

/** The step exists at a table where the dice may be typed; at a table of the app's dice step 1's button answers at once. */
export function releaseRollAsks(mode: DiceMode): boolean {
  return mode === DiceMode.PHYSICAL || mode === DiceMode.PLAYERS_CHOOSE;
}

function signed(bonus: number): string {
  return `${bonus < 0 ? '−' : '+'}${Math.abs(bonus)}`;
}

/** What the step asks for the held action; `null` for an action that rolls nothing here (or whose roll is another piece's: the rays, the map). */
export function releaseRollOf(i: ReleaseRollInput): ReleaseRoll | null {
  const r = i.readied;
  if (!r || !i.options) {
    return null;
  }
  const aimed = i.options.attackTargets.find((x) => x.attackKey === r.actionKey);
  if (r.kind === ReadiedKind.ATTACK || (r.kind === ReadiedKind.SPELL && aimed)) {
    if (i.plan?.rule.kind === 'darts' && i.plan.rule.rays) {
      return null;
    }
    const row = aimed?.targets.find((t) => t.combatantId === i.target);
    if (!row) {
      return null;
    }
    const attack = i.options.options?.attacks.find((a) => a.attack?.key === r.actionKey)?.attack;
    const name = r.actionNamePt || attack?.namePt || r.actionKey;
    const bonus = attack?.attackBonus ?? 0;
    const mode = orNormal(row.rollMode);
    const faces = mode === RollMode.NORMAL ? 1 : 2;
    const combine = mode === RollMode.ADVANTAGE ? 'higher' : 'lower';
    return {
      kind: 'attack',
      name,
      bonus,
      faces,
      combine,
      mode,
      sources: row.sources,
      label: `Role 1d20 para ${name} (${signed(bonus)})`,
      hint:
        faces === 1
          ? 'Role o seu dado e digite o número que saiu (1 a 20).'
          : `Role os dois d20 e digite os dois números, na ordem em que saíram (1 a 20 cada). Conta o ${combine === 'lower' ? 'menor' : 'maior'}.`,
      totalNote: 'Total do ataque',
    };
  }
  if (r.kind === ReadiedKind.SPELL && i.plan && !i.plan.onMap && i.details) {
    const pool =
      spellKind(r.actionKey, i.details) === 'pool' ? poolDice(i.details, r.slotLevel) : null;
    if (!pool) {
      return null;
    }
    const name = r.actionNamePt || r.actionKey;
    return {
      kind: 'pool',
      name,
      min: pool.count,
      max: pool.count * pool.sides,
      label: `Role ${pool.count}d${pool.sides} para ${name}: some os dados`,
      hint: `Digite a soma dos dados, de ${pool.count} a ${pool.count * pool.sides}.`,
      totalNote: 'Total dos dados',
    };
  }
  return null;
}

/** The line of what is released, the attack sheet's own "chosen" row without "Trocar". */
export interface ReleaseChosen {
  readonly icon: string;
  readonly caption: string;
  /** The bold name. */
  readonly lead: string;
  readonly rest: string;
}

/** "no Ogro", "na Aranha". */
function inThe(label: string): string {
  return `${article(label) === 'a' ? 'na' : 'no'} ${label}`;
}

/** "no Ogro", "no Ogro e no Zumbi 1", "no Ogro, no Zumbi 1 e na Aranha". */
function inTheList(labels: readonly string[]): string {
  const parts = labels.map(inThe);
  return parts.length < 2
    ? parts.join('')
    : `${parts.slice(0, -1).join(', ')} e ${parts[parts.length - 1]}`;
}

export function releaseChosenOf(
  roll: ReleaseRoll,
  readied: ReadiedAction | undefined,
  targets: readonly { readonly id: string; readonly label: string }[],
  distanceFt: number | undefined,
): ReleaseChosen {
  const spell = readied?.kind === ReadiedKind.SPELL;
  const caption = !spell
    ? 'Ataque preparado'
    : readied.slotLevel > 0
      ? `Magia preparada · espaço de ${readied.slotLevel}º nível`
      : 'Magia preparada';
  const where = inTheList(targets.map((t) => t.label));
  const far =
    roll.kind === 'attack' && distanceFt !== undefined
      ? tight(` · a ${metersText(distanceFt)}`)
      : '';
  return {
    icon: spell ? 'auto_fix_high' : 'swords',
    caption,
    lead: roll.name,
    rest: where ? ` ${where}${far}` : '',
  };
}

/** What the player did at the step: rolled in the app, or typed the numbers (the d20, its pair or the pool's sum) and the faces an effect adds. */
export type ReleaseTyped =
  'app' | { readonly typed: readonly number[]; readonly extra: readonly number[] };

/** The answer of the window: the use the first step built, with the die the second one made. */
export function releaseAnswerOf(
  base: ReactionAnswer,
  roll: ReleaseRoll,
  how: ReleaseTyped,
): ReactionAnswer {
  if (how === 'app') {
    return { ...base, die: { inApp: true } };
  }
  const extra = how.extra.length > 0 ? { extraDieFaces: how.extra } : {};
  if (roll.kind === 'pool') {
    return { ...base, die: { poolSum: how.typed[0] } };
  }
  return {
    ...base,
    ...extra,
    die: roll.faces === 1 ? { typed: how.typed[0] } : { d20Faces: how.typed },
  };
}
