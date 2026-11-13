import { CharacterKind } from '../../../gen/meurpg/characters/v1/characters_pb';
import {
  type Combatant,
  CombatantKind,
  CombatantState,
  type Encounter,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from './combat-log';
import { treatedSourceName } from './combat-dice';
import { creatureTurnTitle, isCreature } from './creature-names';
import { afterTurn, jointTurn, listNames, type JointTurn, npcPlural } from './joint-turn';

/**
 * What the combat screens say about an encounter, kept as small pure
 * functions: the words, the formula, the tie groups, whose turn comes next
 * and what the player's banner reads. Tested without a DOM.
 */

/** How hurt an NPC is, as a word a player may read (RN-20), and where a
 * player's character is when it is at 0 hit points: "Caído", "Estável",
 * "Morrendo" (only the master gets that one) or "Morto". The word of a
 * person has a gender, taken from the name the way `article` does
 * ("Brisa" is "Caída"). */
export function stateWord(state: CombatantState, label = ''): string {
  const feminine = label !== '' && article(label) === 'a';
  switch (state) {
    case CombatantState.UNHURT:
      return 'Ileso';
    case CombatantState.HURT:
      return 'Ferido';
    case CombatantState.BADLY_HURT:
      return 'Muito ferido';
    case CombatantState.DEFEATED:
      return 'Derrotado';
    case CombatantState.DOWN:
      return feminine ? 'Caída' : 'Caído';
    case CombatantState.DYING:
      return 'Morrendo';
    case CombatantState.STABLE:
      return 'Estável';
    case CombatantState.DEAD:
      return feminine ? 'Morta' : 'Morto';
    default:
      return '';
  }
}

/** Whether a player's character is at 0 hit points and still in the story:
 * down, stable or dying. */
/** A player's character that died: it has no reaction and takes no part in the fight. */
export function isDead(c: Combatant): boolean {
  return c.state === CombatantState.DEAD || c.defeated;
}

export function isDown(c: Combatant): boolean {
  return (
    c.state === CombatantState.DOWN ||
    c.state === CombatantState.DYING ||
    c.state === CombatantState.STABLE
  );
}

/** The second line of a chip or a row for another player: "Jogador" (their
 * name is not sent), or the word of their state when they are down. */
export function playerWord(c: Combatant): string {
  return isDown(c) || c.state === CombatantState.DEAD ? stateWord(c.state, c.label) : 'Jogador';
}

/** The letter or letters on a token: the first letter, plus the number of a
 * numbered copy ("Goblin 2" is "G2"). */
export function combatantInitial(label: string): string {
  const letter = label.trim().charAt(0).toUpperCase();
  const copy = /\s(\d+)$/.exec(label.trim());
  return copy ? `${letter}${copy[1]}` : letter;
}

export function isPlayer(c: Combatant): boolean {
  return c.kind === CombatantKind.PLAYER;
}

/** The initiative as the master reads it: `1d20 (15) + 4 = 19`. `null`
 * until it is rolled. */
export function initiativeFormula(c: Combatant): string | null {
  if (c.initiative === undefined || c.initiativeFace === undefined) {
    return null;
  }
  const bonus = c.initiativeBonus ?? 0;
  const sign = bonus < 0 ? '−' : '+';
  const die = initiativeDie(c);
  const raised = c.initiativeTreatedAs;
  const d20 =
    raised === undefined
      ? `1d20 (${c.initiativeFace})`
      : `d20: ${c.initiativeFace} → ${raised} (${treatedSourceName(c.initiativeTreatedAsSource, c.initiativeTreatedAsSourceName)})`;
  return `${d20} ${sign} ${Math.abs(bonus)}${die ? ` + ${die} (dado)` : ''} = ${c.initiative}`;
}

/**
 * The same initiative in two lines, for a column too narrow to hold a feature's name: `line` keeps the total in
 * its place (`1d20 (12) + 3 = 15`, with the d20 that counted) and `note` explains the d20 a table minimum raised
 * (`d20: 3 → 12 (Pé de Vento)`); `note` is `null` when nothing raised it, and then `line` is `initiativeFormula`
 * unchanged. `null` until it is rolled.
 */
export function initiativeFormulaParts(c: Combatant): { line: string; note: string | null } | null {
  const whole = initiativeFormula(c);
  const raised = c.initiativeTreatedAs;
  if (whole === null || c.initiativeFace === undefined || raised === undefined) {
    return whole === null ? null : { line: whole, note: null };
  }
  const bonus = c.initiativeBonus ?? 0;
  const die = initiativeDie(c);
  return {
    line: `1d20 (${raised}) ${bonus < 0 ? '−' : '+'} ${Math.abs(bonus)}${die ? ` + ${die} (dado)` : ''} = ${c.initiative}`,
    note: `d20: ${c.initiativeFace} → ${raised} (${treatedSourceName(c.initiativeTreatedAsSource, c.initiativeTreatedAsSourceName)})`,
  };
}

/** What a Bardic Inspiration die added to the initiative: the total less the d20 and the bonus, 0 when none was used. */
export function initiativeDie(c: Combatant): number {
  if (c.initiative === undefined || c.initiativeFace === undefined) {
    return 0;
  }
  return Math.max(
    0,
    c.initiative - (c.initiativeTreatedAs ?? c.initiativeFace) - (c.initiativeBonus ?? 0),
  );
}

/** The roll still to come: `1d20 + 2 = —`. */
export function pendingFormula(c: Combatant): string {
  const bonus = c.initiativeBonus ?? 0;
  return `1d20 ${bonus < 0 ? '−' : '+'} ${Math.abs(bonus)} = —`;
}

/** A bonus with its sign: "+2", "−1", "+0". */
export function signed(n: number): string {
  return `${n < 0 ? '−' : '+'}${Math.abs(n)}`;
}

/** The runs of combatants tied on the same total and bonus that the master
 * has not ordered yet (`tie_unresolved`), each as the IDs in their order. */
export function tieGroups(combatants: readonly Combatant[]): string[][] {
  const groups: string[][] = [];
  let last: Combatant | null = null;
  for (const c of combatants) {
    if (!c.tieUnresolved) {
      last = null;
      continue;
    }
    if (last && last.initiative === c.initiative && last.initiativeBonus === c.initiativeBonus) {
      groups[groups.length - 1].push(c.id);
    } else {
      groups.push([c.id]);
    }
    last = c;
  }
  return groups;
}

/** A group's IDs with `id` moved one place up or down; the same list when it
 * is already at that end (the arrows stay inside the group). */
export function moveInGroup(group: readonly string[], id: string, direction: -1 | 1): string[] {
  const from = group.indexOf(id);
  const to = from + direction;
  const next = [...group];
  if (from < 0 || to < 0 || to >= group.length) {
    return next;
  }
  [next[from], next[to]] = [next[to], next[from]];
  return next;
}

/** "Empate em 12: Goblin 1 e Goblin 2. Escolha a ordem com as setas." */
export function tieSentence(labels: readonly string[], total: number | undefined): string {
  const names =
    labels.length <= 2
      ? labels.join(' e ')
      : `${labels.slice(0, -1).join(', ')} e ${labels[labels.length - 1]}`;
  return `Empate em ${total}: ${names}. Escolha a ordem com as setas.`;
}

/** The combatants who still have no initiative. */
export function missingInitiative(combatants: readonly Combatant[]): Combatant[] {
  return combatants.filter((c) => c.initiative === undefined);
}

/** The one on turn, or `null` (SETUP, ENDED, or a hidden one for a player). */
export function currentCombatant(e: Encounter): Combatant | null {
  return e.combatants.find((c) => c.id === e.currentCombatantId) ?? null;
}

/** Who plays after the current one: the next that is not defeated, going
 * round the order. `null` when the current is the only one, or nobody is on
 * turn (a player never knows about a hidden one in between). */
export function nextCombatant(e: Encounter): Combatant | null {
  const list = e.combatants;
  const at = list.findIndex((c) => c.id === e.currentCombatantId);
  if (at < 0) {
    return null;
  }
  for (let step = 1; step < list.length; step++) {
    const c = list[(at + step) % list.length];
    if (!c.defeated) {
      return c;
    }
  }
  return null;
}

/** The turn banner of the combat bar (master) and of the player's screen. */
export interface TurnBanner {
  /** "Vez do Capitão Goblin", "Vez do mestre", "Sua vez, Pensantus", or, in a
   * joint turn, "Vez de Brisa e Toren" and "Você encerrou a sua parte". */
  readonly title: string;
  /** Who is on turn; `null` for the master's turn (no name, no token). */
  readonly who: Combatant | null;
  /** The caller's own combatant may act now. */
  readonly mine: boolean;
  readonly masterTurn: boolean;
  /** "Em seguida: Pensantus" (master) or the player's "Você é o próximo". */
  readonly next: Combatant | null;
  /** Whether the next one is the caller's own combatant. */
  readonly nextIsMine: boolean;
  /** Who follows the turn that is running: a name, "os Goblins" for a group of
   * NPCs. `null` when nobody follows. */
  readonly after: {
    readonly name: string;
    readonly mine: boolean;
    readonly plural: boolean;
  } | null;
  /** The joint turn that is running, or `null` for a turn of one. */
  readonly joint: JointTurn | null;
  /** The caller's own part of the joint turn ended. */
  readonly ownEnded: boolean;
}

export function turnBanner(e: Encounter): TurnBanner {
  const joint = jointTurn(e);
  const after = afterTurn(e);
  if (joint) {
    return jointBanner(e, joint, after);
  }
  return e.masterTurn ? MASTER_BANNER : soloBanner(e, after);
}

const MASTER_BANNER: TurnBanner = {
  title: 'Vez do mestre',
  who: null,
  mine: false,
  masterTurn: true,
  next: null,
  nextIsMine: false,
  after: null,
  joint: null,
  ownEnded: false,
};

/** The title of a joint turn: the caller's own part when they have one, else who is on turn. */
function jointTitle(e: Encounter, joint: JointTurn, own: Combatant | undefined): string {
  if (own) {
    return own.turnPartEnded ? 'Você encerrou a sua parte' : `Sua vez, ${own.label}`;
  }
  // A player's creatures alone: "Vez dos Lobos atrozes da Sálvia" (their player's own tab says "dos seus").
  if (joint.members.every(isCreature)) {
    return creatureTurnTitle(e, joint.members, false);
  }
  const labels = joint.members.map((m) => m.label);
  const named = joint.npcOnly ? npcPlural(labels) : null;
  return named ? `Vez d${named}` : `Vez de ${listNames(labels)}`;
}

function jointBanner(e: Encounter, joint: JointTurn, after: TurnBanner['after']): TurnBanner {
  const own = joint.members.find((m) => m.mine);
  return {
    title: jointTitle(e, joint, own),
    who: own ?? joint.acting[0] ?? joint.members[0],
    mine: !!own && !own.turnPartEnded,
    masterTurn: e.masterTurn,
    next: null,
    nextIsMine: after?.mine ?? false,
    after,
    joint,
    ownEnded: !!own?.turnPartEnded,
  };
}

/** The title of a turn of one: the caller's own, a creature's, or a name with its article. */
export function soloTitle(e: Encounter, who: Combatant | null): string {
  if (!who) {
    return 'Ninguém está na vez';
  }
  if (who.mine) {
    return `Sua vez, ${who.label}`;
  }
  if (who.kind === CombatantKind.CREATURE) {
    return creatureTurnTitle(e, [who], false);
  }
  return `Vez d${article(who.label) === 'a' ? 'a' : 'o'} ${who.label}`;
}

function soloBanner(e: Encounter, after: TurnBanner['after']): TurnBanner {
  const who = currentCombatant(e);
  const next = nextCombatant(e);
  return {
    title: soloTitle(e, who),
    who,
    mine: who?.mine ?? false,
    masterTurn: false,
    next,
    nextIsMine: next?.mine ?? false,
    after,
    joint: null,
    ownEnded: false,
  };
}

/** The player's own combatant, if the master put it in the combat. */
export function ownCombatant(e: Encounter): Combatant | null {
  return e.combatants.find((c) => c.mine) ?? null;
}

/** "Rodada 2". */
export function roundLabel(round: number): string {
  return `Rodada ${round}`;
}

/** An NPC's kind as the lists say it ("Inimigo", "Minion"…); empty for a
 * player's character. */
export function npcKindLabel(kind: CharacterKind): string {
  switch (kind) {
    case CharacterKind.ENEMY:
      return 'Inimigo';
    case CharacterKind.BOSS:
      return 'Boss';
    case CharacterKind.MINION:
      return 'Minion';
    case CharacterKind.STORY:
      return 'NPC de história';
    default:
      return '';
  }
}
