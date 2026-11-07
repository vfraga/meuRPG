import type { DiceRoll } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type AdvantageSource,
  AdvantageSourceKind,
  type DamageStep,
  type RollModeRequest,
  RollMode,
  RollModeRequestStatus,
} from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { extraDiceText, rollFormula } from './combat-dice';

/** The three modes a d20 is rolled with, in the order the radio group shows them. */
export const ROLL_MODES: readonly RollMode[] = [
  RollMode.NORMAL,
  RollMode.ADVANTAGE,
  RollMode.DISADVANTAGE,
];

/** A mode the server left unset reads as a normal roll. */
export function orNormal(mode: RollMode): RollMode {
  return mode === RollMode.UNSPECIFIED ? RollMode.NORMAL : mode;
}

/** "Normal", "Vantagem" or "Desvantagem". */
export function modeWord(mode: RollMode): string {
  switch (orNormal(mode)) {
    case RollMode.ADVANTAGE:
      return 'Vantagem';
    case RollMode.DISADVANTAGE:
      return 'Desvantagem';
    default:
      return 'Normal';
  }
}

function rank(mode: RollMode): number {
  switch (orNormal(mode)) {
    case RollMode.ADVANTAGE:
      return 1;
    case RollMode.DISADVANTAGE:
      return -1;
    default:
      return 0;
  }
}

/** The chosen mode is better for the roller than the suggestion: only the master gives it. */
export function isBetter(chosen: RollMode, suggested: RollMode): boolean {
  return rank(chosen) > rank(suggested);
}

/** How many d20 the mode rolls: one, or two for advantage and disadvantage. */
export function d20Count(mode: RollMode): 1 | 2 {
  return orNormal(mode) === RollMode.NORMAL ? 1 : 2;
}

const SENTENCE_SOURCES: ReadonlySet<AdvantageSourceKind> = new Set([
  AdvantageSourceKind.EFFECT_DIE,
  AdvantageSourceKind.OTHER_SOURCE,
  AdvantageSourceKind.EFFECT_SAVE,
  AdvantageSourceKind.EFFECT_CHECK,
]);

/** "Vantagem" or "Desvantagem" for the tag of a source. */
export function sourceWord(source: AdvantageSource): string {
  return source.effect === RollMode.ADVANTAGE ? 'Vantagem' : 'Desvantagem';
}

/** The line of a source under a roll: "Vantagem: Alvo Paralisado a 1,5 m: vantagem". A die an effect added ("Bênção, de Tavo..."),
 * "Outra fonte" (an effect the master keeps from the players) and the advantage an active effect gives on a saving throw or an
 * ability check ("Efeito ativo: vantagem em testes de habilidade") are the server's sentence as it is: it already says what they are. */
export function sourceLine(source: AdvantageSource): string {
  return SENTENCE_SOURCES.has(source.kind)
    ? source.textPt
    : `${sourceWord(source)}: ${source.textPt}`;
}

/** The lines "Bênção +1d4: 3" for the dice an effect added to a roll, named after the effect. A roll whose sources already
 * carry those dice (a check or a saving throw) gets none here, so a die is never said twice. */
export function extraDieLines(
  roll: Pick<DiceRoll, 'extraDice'> | null | undefined,
  sources: readonly AdvantageSource[],
): string[] {
  if ((sources ?? []).some((s) => s.kind === AdvantageSourceKind.EFFECT_DIE)) {
    return [];
  }
  return (roll?.extraDice ?? [])
    .filter((d) => d.face > 0 && d.sourceNamePt !== '')
    .map((d) => `${d.sourceNamePt} ${d.sign < 0 ? '−' : '+'}1d${d.faces}: ${d.face}`);
}

/** How the roll mode stands for whoever is about to roll. */
export type ModeStatus =
  /** Roll now with this mode. */
  | 'ready'
  /** A change from the suggestion needs its reason (1 to 120 characters). */
  | 'needs-reason'
  /** A better mode than the suggestion: ask the master. */
  | 'needs-request'
  /** The request waits for the master. */
  | 'pending'
  /** A better mode than the suggestion that this roll cannot ask for. */
  | 'blocked';

/** How a better mode than the suggested one is reached: the master sets it, a player asks him, or cannot. */
export type Approval = 'free' | 'request' | 'blocked';

/** The longest reason a request or a mode change takes. */
export const REASON_MAX = 120;

/** The reason is 1 to 120 characters on one line. */
export function reasonValid(reason: string): boolean {
  const text = reason.trim();
  return text.length >= 1 && text.length <= REASON_MAX && !/[\r\n]/.test(text);
}

/** Where the roll mode stands: what the screen asks before the dice may be rolled. */
export function modeStatus(
  approval: Approval,
  suggested: RollMode,
  chosen: RollMode,
  reason: string,
  request: RollModeRequest | null,
): ModeStatus {
  if (request?.status === RollModeRequestStatus.PENDING) {
    return 'pending';
  }
  if (request?.status === RollModeRequestStatus.ANSWERED) {
    return 'ready';
  }
  if (rank(chosen) === rank(suggested)) {
    return 'ready';
  }
  if (isBetter(chosen, suggested) && approval !== 'free') {
    if (approval === 'blocked') {
      return 'blocked';
    }
    return reasonValid(reason) ? 'needs-request' : 'needs-reason';
  }
  return reasonValid(reason) ? 'ready' : 'needs-reason';
}

/** The request of the attack is still open (waits or is answered), by id. */
export function liveRequest(
  requests: readonly RollModeRequest[],
  id: string,
): RollModeRequest | null {
  const r = requests.find((x) => x.id === id);
  return r && r.status !== RollModeRequestStatus.CLOSED ? r : null;
}

/** The master's card: "Pedido de Vantagem: Toren → Goblin (Espada curta) — “estou escondido”". */
export function requestLine(r: RollModeRequest, labelOf: (combatantId: string) => string): string {
  return `Pedido de ${modeWord(r.requestedMode)}: ${labelOf(r.combatantId)} → ${labelOf(r.targetId)} (${r.attackNamePt}) — “${r.reason}”`;
}

/** The two d20 of a roll, or the one: each face with whether it is the one that counts. */
export function d20Faces(roll: DiceRoll): readonly { face: number; counted: boolean }[] {
  const pair = roll.diceCount > 1 && roll.faces.length > 1;
  return roll.faces.map((face, i) => ({ face, counted: !pair || i === roll.countedIndex }));
}

/** "Resistência a fogo (tiefling): 10 → 5". */
export function stepLine(step: DamageStep): string {
  return `${step.labelPt}: ${step.before} → ${step.after}`;
}

/** The formula of a d20: `14 + 5 = 19` for the face that counts of a pair, the usual one for a single d20; `named` writes the
 * source of each extra die ("Golpe Certeiro 1d8 (5)"). */
export function d20Formula(roll: DiceRoll, named = false): string {
  if (roll.diceCount > 1 && roll.faces.length > 1) {
    const kept = roll.faces[roll.countedIndex] ?? roll.faces[0];
    const mod =
      roll.modifier === 0 ? '' : ` ${roll.modifier < 0 ? '−' : '+'} ${Math.abs(roll.modifier)}`;
    return `${kept}${mod}${extraDiceText(roll, named).text} = ${roll.total}`;
  }
  return rollFormula(roll, named);
}
