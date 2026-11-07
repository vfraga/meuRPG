import type { LegendaryNames } from './legendary-card';

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
  /** The legendary options' names per creature (`legendaryNames`), to write a spend in Portuguese. */
  readonly legendaryNames?: LegendaryNames;
}

export const NO_CONTEXT: LogContext = { master: false, players: new Set() };

export interface LogGroup {
  readonly round: number;
  /** "Rodada 2". */
  readonly title: string;
  /** "em andamento" or "encerrada". */
  readonly status: string;
  readonly lines: readonly LogLine[];
}
