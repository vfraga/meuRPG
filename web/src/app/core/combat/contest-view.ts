import type { Combatant, Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type CheckRoll,
  type ContestSkillOption,
  type ContestTurnState,
  type ContestView,
  ContestKind,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestWaitFor,
  ContestWinner,
  RollModeKind,
  ShoveBlockedReason,
  ShoveOutcome,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
import { CreatureSize } from '../../../gen/meurpg/rules/v1/rules_pb';
import { CREATURE_SIZES } from '../creatures/creature-types';
import { tight, upFirst } from '../format/text';
import { article } from './combat-log';
import { isPlayer } from './combat-view';
import { type NaturalMark, naturalMark } from './natural-mark';
import { reactionWait } from './reactions';

/**
 * What the contest sheets say (W7-X): the words of the grapple, the shove and the escape, as the boards draw them. The server
 * decides who won and what a player may read; here only the sentences are written. A player never reads a total, a skill or a
 * DC of an NPC (RN-20), so none of these functions is given one.
 */

/** One step of a contest sheet's stepper: done (a check), the current one, or still to come. */
export interface ContestStep {
  readonly name: string;
  readonly state: 'done' | 'current' | 'todo';
}

/** The stepper: the steps before `at` are done, `at` is current, the rest wait. */
export function contestSteps(names: readonly string[], at: number): readonly ContestStep[] {
  return names.map((name, i) => ({
    name,
    state: i < at ? 'done' : i === at ? 'current' : 'todo',
  }));
}

/** "Você precisa de uma mão livre." has its own word in the sheet; this is the size limit it adds. */
export const GRAPPLE_REMINDER = 'Tenho uma mão livre';

/** Who a sentence is about: the label and whether it is a player's character (named bare) or a creature (with its article). */
export interface Who {
  readonly label: string;
  readonly player: boolean;
}

/** A combatant as a sentence names it. */
export function whoIs(c: Pick<Combatant, 'label' | 'kind'> | undefined): Who {
  return c
    ? { label: c.label, player: isPlayer(c as Combatant) }
    : { label: 'alguém', player: true };
}

/** "o Hobgoblin", "Brisa": a player's character is named bare, a creature with its article. */
export function named(who: Who): string {
  return who.player ? who.label : `${article(who.label)} ${who.label}`;
}

/** "do Hobgoblin", "de Brisa". */
export function ofNamed(who: Who): string {
  return who.player
    ? `de ${who.label}`
    : `${article(who.label) === 'a' ? 'da' : 'do'} ${who.label}`;
}

/** "ele" or "ela", by the name. */
export function pronoun(label: string): string {
  return gendered(label, 'ele', 'ela');
}

/** "dele" or "dela", by the name. */
export function possessive(label: string): string {
  return gendered(label, 'dele', 'dela');
}

/** "Atletismo" or "Acrobacia". */
export function skillName(skill: ContestSkill): string {
  return skill === ContestSkill.ACROBATICS ? 'Acrobacia' : 'Atletismo';
}

/** "Força (Atletismo)" or "Destreza (Acrobacia)". */
export function skillLine(skill: ContestSkill): string {
  return skill === ContestSkill.ACROBATICS ? 'Destreza (Acrobacia)' : 'Força (Atletismo)';
}

/** A modifier as the sheet writes it: "+7", "−1" (a true minus), "+0". */
export function signed(n: number): string {
  return n < 0 ? `−${Math.abs(n)}` : `+${n}`;
}

/** "o" or "a" for a person (Agarrado, Agarrada), by the name. */
function gendered(label: string, male: string, female: string): string {
  return article(label) === 'a' ? female : male;
}

/** "Agarrado" or "Agarrada". */
export function grappledWord(label: string): string {
  return gendered(label, 'Agarrado', 'Agarrada');
}

/** "Derrubado" or "Derrubada". */
export function proneWord(label: string): string {
  return gendered(label, 'Derrubado', 'Derrubada');
}

/** The size of a creature in words ("Médio"); '' when the server does not say. */
export function sizeWord(size: CreatureSize): string {
  return CREATURE_SIZES.find((s) => s.size === size)?.label ?? '';
}

/** The words that name what a contest is for: "agarrar", "empurrar". */
export function verb(purpose: ContestPurpose): string {
  return purpose === ContestPurpose.SHOVE ? 'empurrar' : 'agarrar';
}

/** The title of the initiator's sheet: "Agarrar", "Empurrar", "Escapar". */
export function sheetTitle(purpose: ContestPurpose): string {
  switch (purpose) {
    case ContestPurpose.SHOVE:
      return 'Empurrar';
    case ContestPurpose.ESCAPE:
      return 'Escapar';
    default:
      return 'Agarrar';
  }
}

/** The title once the target is chosen: "Agarrar o Hobgoblin", "Agarrar Brisa". */
export function sheetTitleWith(purpose: ContestPurpose, target: string): string {
  return target ? `${sheetTitle(purpose)} ${target}` : sheetTitle(purpose);
}

/** The title of the defender's sheet: "Hobgoblin tenta agarrar você". */
export function defenderTitle(initiatorLabel: string, purpose: ContestPurpose): string {
  return `${initiatorLabel} tenta ${verb(purpose)} você`;
}

/** The d20 that counts of a roll, from the pair and the mode: the higher with advantage, the lower with disadvantage. */
export function countedFace(roll: CheckRoll): number {
  const faces = roll.faces;
  if (faces.length === 0) {
    return 0;
  }
  if (roll.mode === RollModeKind.ADVANTAGE) {
    return Math.max(...faces);
  }
  if (roll.mode === RollModeKind.DISADVANTAGE) {
    return Math.min(...faces);
  }
  return faces[0];
}

export { naturalMark, type NaturalMark } from './natural-mark';

/** The mark of the d20 that counts in a check roll (the higher with advantage, the lower with disadvantage). */
export function checkNaturalMark(roll: CheckRoll | undefined): NaturalMark {
  return roll ? naturalMark(countedFace(roll)) : '';
}

/** "1d20 (15) + 5": the d20 that counts and the modifier; the two dice of a pair are said apart (`pairOf`). */
export function rollFormula(roll: CheckRoll): string {
  const face = countedFace(roll);
  const mod =
    roll.modifier === 0 ? '' : ` ${roll.modifier < 0 ? '−' : '+'} ${Math.abs(roll.modifier)}`;
  return `1d20 (${face})${mod}`;
}

/** The two d20 of a roll with advantage or disadvantage, the one that counts marked; empty for a single die. */
export function pairOf(
  roll: CheckRoll,
): readonly { readonly value: number; readonly counts: boolean }[] {
  if (roll.faces.length < 2) {
    return [];
  }
  const counted = countedFace(roll);
  let used = false;
  return roll.faces.map((value) => {
    const counts = !used && value === counted;
    used ||= counts;
    return { value, counts };
  });
}

/** The circumstances behind the mode, one sentence each: "Vantagem: Ajuda de Orla", "Desvantagem: Envenenado". */
export function noteLines(roll: Pick<CheckRoll, 'notes'>): readonly string[] {
  return roll.notes.map((n) => `${n.advantage ? 'Vantagem' : 'Desvantagem'}: ${n.labelPt}`);
}

/** "Vantagem" or "Desvantagem", or '' for a normal roll. */
export function modeLabel(mode: RollModeKind): string {
  if (mode === RollModeKind.ADVANTAGE) {
    return 'Vantagem';
  }
  return mode === RollModeKind.DISADVANTAGE ? 'Desvantagem' : '';
}

/** The skills a defender (or the grappled one) picks from: the higher modifier first, as the board draws them. */
export function skillChoices(
  options: readonly ContestSkillOption[],
): readonly ContestSkillOption[] {
  return [...options].sort((a, b) => b.modifier - a.modifier);
}

/** Who the contest waits for, in the combat's own words ("Esperando o mestre", "Esperando Brisa"): the reaction wait the server
 * wrote, else the contest's own say. */
export function waitTitle(e: Encounter | null, contest: ContestView | undefined): string {
  const wait = e ? reactionWait(e) : null;
  if (wait) {
    return wait.title;
  }
  if (contest?.waitingFor === ContestWaitFor.PLAYER) {
    const who = e?.combatants.find((c) => c.id === contest.waitingCombatantId);
    return who ? `Esperando ${who.label}` : 'Esperando o mestre';
  }
  return 'Esperando o mestre';
}

/** The sentence after the wait: who answers and what they do ("O Hobgoblin escolhe Atletismo ou Acrobacia e rola."). */
export function waitDetail(e: Encounter | null, contest: ContestView | undefined): string {
  if (!contest) {
    return '';
  }
  const defender = e?.combatants.find((c) => c.id === contest.defenderId);
  if (contest.status === ContestStatus.AWAITING_OUTCOME) {
    return 'Você escolhe o que fazer.';
  }
  if (contest.purpose === ContestPurpose.ESCAPE) {
    return (e ? reactionWait(e)?.detail : '') ?? '';
  }
  if (contest.deferred && defender) {
    return `${defender.label} deixou o mestre rolar por ${pronoun(defender.label)}.`;
  }
  if (!defender) {
    return 'O alvo escolhe Atletismo ou Acrobacia e rola.';
  }
  const who = isPlayer(defender)
    ? upFirst(pronoun(defender.label))
    : upFirst(named(whoIs(defender)));
  return `${who} escolhe Atletismo ou Acrobacia e rola.`;
}

/** Whether the contest ended with the initiator winning. */
export function initiatorWon(c: ContestView): boolean {
  return c.winner === ContestWinner.INITIATOR;
}

/** Whether the contest is over (answered or closed by the master). */
export function isSettled(c: ContestView): boolean {
  return c.status === ContestStatus.RESOLVED || c.status === ContestStatus.CLOSED;
}

/** What the initiator reads once a grapple or a shove is decided: "Você venceu a disputa", "Você não conseguiu agarrar o Hobgoblin". */
export interface InitiatorVerdict {
  /** The bold sentence. */
  readonly lead: string;
  /** What follows it. */
  readonly rest: string;
  readonly won: boolean;
}

/** The verdict of a grapple, a shove or an escape for the one that rolled first, once decided; never a total. `own` is the
 * label of the one that reads it (an escape says "Continua agarrada" by its gender). */
export function initiatorVerdict(
  c: ContestView,
  target: Who,
  own: string,
): InitiatorVerdict | null {
  if (c.status === ContestStatus.CLOSED) {
    return { lead: 'O mestre encerrou a disputa.', rest: 'A ação foi gasta.', won: false };
  }
  if (c.status !== ContestStatus.RESOLVED) {
    return null;
  }
  if (c.purpose === ContestPurpose.ESCAPE) {
    return escapeVerdict(c, own);
  }
  const the = named(target);
  if (initiatorWon(c)) {
    return {
      lead: 'Você venceu a disputa.',
      rest:
        c.purpose === ContestPurpose.SHOVE
          ? shoveDone(c, target)
          : `${upFirst(the)} está ${grappledWord(target.label)}.`,
      won: true,
    };
  }
  const miss = `Você não conseguiu ${verb(c.purpose)} ${the}.`;
  return c.winner === ContestWinner.TIE
    ? { lead: 'Empate: nada muda.', rest: miss, won: false }
    : { lead: miss, rest: '', won: false };
}

/** What a won shove did, in words (after the choice), or the choice still to make. */
function shoveDone(c: ContestView, target: Who): string {
  const the = named(target);
  switch (c.shoveOutcome) {
    case ShoveOutcome.PRONE:
      return `${upFirst(the)} está ${proneWord(target.label)}.`;
    case ShoveOutcome.PUSH:
      return `Você empurrou ${the} 1,5 m.`;
    case ShoveOutcome.STAYS:
      return `${upFirst(the)} não saiu do lugar.`;
    default:
      return `Escolha o que fazer com ${the}.`;
  }
}

/** The result of an escape: "Você se soltou." / "Continua agarrada. A ação foi gasta." (a fixed DC says "Você escapou."). */
export function escapeVerdict(c: ContestView, own = ''): InitiatorVerdict {
  const won = initiatorWon(c);
  if (won) {
    return {
      lead: c.kind === ContestKind.ESCAPE_DC ? 'Você escapou.' : 'Você se soltou.',
      rest: '',
      won: true,
    };
  }
  return {
    lead: own && article(own) === 'a' ? 'Continua agarrada.' : 'Continua agarrado.',
    rest: 'A ação foi gasta.',
    won: false,
  };
}

/** The sentence over the roll of an escape: the app says whom the test is against, never the number. */
export function escapeIntro(grappler: Who, fixedDc: boolean): string {
  return fixedDc
    ? `Você usa a ação: um teste de Atletismo ou Acrobacia contra a força ${ofNamed(grappler)}.`
    : `Você usa a ação. Escolha Atletismo ou Acrobacia: o teste é contra o Atletismo ${ofNamed(grappler)}.`;
}

/** What the defender reads when the contest is decided: "Você perdeu a disputa. O Hobgoblin agarrou você: você está Agarrada, com deslocamento 0." */
export function defenderVerdict(
  c: ContestView,
  initiator: Who,
  ownLabel: string,
): InitiatorVerdict | null {
  const by = upFirst(named(initiator));
  if (c.status === ContestStatus.CLOSED) {
    return { lead: 'O mestre encerrou a disputa.', rest: '', won: true };
  }
  if (c.status === ContestStatus.AWAITING_OUTCOME) {
    return { lead: 'Você perdeu a disputa.', rest: `${by} vai escolher o que fazer.`, won: false };
  }
  if (c.status !== ContestStatus.RESOLVED) {
    return null;
  }
  const shove = c.purpose === ContestPurpose.SHOVE;
  if (c.winner === ContestWinner.INITIATOR) {
    return {
      lead: 'Você perdeu a disputa.',
      rest: shove
        ? shoveAgainst(c, by)
        : `${by} agarrou você: você está ${grappledWord(ownLabel)}, com deslocamento 0.`,
      won: false,
    };
  }
  return {
    lead: c.winner === ContestWinner.TIE ? 'Empate: nada muda.' : 'Você venceu a disputa.',
    rest: `${by} não conseguiu ${verb(c.purpose)} você.`,
    won: true,
  };
}

function shoveAgainst(c: ContestView, by: string): string {
  switch (c.shoveOutcome) {
    case ShoveOutcome.PRONE:
      return `${by} derrubou você.`;
    case ShoveOutcome.PUSH:
      return `${by} empurrou você 1,5 m.`;
    default:
      return `${by} não tirou você do lugar.`;
  }
}

/** The line under "Empurrar 1,5 m" in the choice: the push is blocked by a wall, a creature, or it is free. */
export function pushLine(blocked: ShoveBlockedReason): string {
  switch (blocked) {
    case ShoveBlockedReason.WALL:
      return 'Para longe de você. Há uma parede na casa de trás: ele não sai do lugar.';
    case ShoveBlockedReason.CREATURE:
      return 'Para longe de você. Há uma criatura na casa de trás: ele não sai do lugar.';
    default:
      return 'Para longe de você, em linha reta.';
  }
}

/** The line under "Derrubar": "O Hobgoblin fica Derrubado: só rasteja. Ataque corpo a corpo contra ele a até 1,5 m tem vantagem; de mais longe, desvantagem." */
export function proneLine(target: Who): string {
  return tight(
    `${upFirst(named(target))} fica ${proneWord(target.label)}: só rasteja. Ataque corpo a corpo contra ${pronoun(target.label)} a até 1,5 m tem vantagem; de mais longe, desvantagem.`,
  );
}

/** The sentence under the verdict of a won grapple: "Você o segura enquanto quiser (solte sem gastar ação). Se você se mover, leva-o junto, com o deslocamento pela metade." */
export function holdLine(targetLabel: string): string {
  const it = gendered(targetLabel, 'o', 'a');
  return `Você ${it} segura enquanto quiser (solte sem gastar ação). Se você se mover, leva-${it} junto, com o deslocamento pela metade.`;
}

/** "escondido" or "escondida", by the character's name. */
export function hiddenWord(label: string): string {
  return gendered(label, 'escondido', 'escondida');
}

/** The master's refusal of a Hide as the sheet writes it: the part up to the colon is bold ("Alguém vê você claramente:"), the rest plain. */
export function refusalParts(text: string): { readonly lead: string; readonly rest: string } {
  const at = text.indexOf(': ');
  return at < 0
    ? { lead: '', rest: text }
    : { lead: text.slice(0, at + 1), rest: text.slice(at + 2) };
}

/** A state of the player's own turn: "Escondida" or "Surpresa", with the sentence under its tag. */
export interface ContestNote {
  readonly tag: string;
  readonly icon: string;
  readonly tone: 'success' | 'pending';
  readonly text: string;
}

/** The tags the turn shows for what `contest_state` says: hidden ("Você está escondida.", never from whom) and surprised. */
export function contestNotes(
  state: ContestTurnState | undefined,
  ownLabel: string,
): readonly ContestNote[] {
  const notes: ContestNote[] = [];
  if (state?.hidden) {
    notes.push({
      tag: upFirst(hiddenWord(ownLabel)),
      icon: 'visibility',
      tone: 'success',
      text: `Você está ${hiddenWord(ownLabel)}.`,
    });
  }
  if (state?.surprised) {
    const word = gendered(ownLabel, 'surpreso', 'surpresa');
    notes.push({
      tag: 'Surpresa',
      icon: 'bolt',
      tone: 'pending',
      text: `Você está ${word} neste turno. Você não se move, não age e não reage até o fim dele.`,
    });
  }
  return notes;
}

/** The line under "Agarrar" in the attack list. */
export const GRAPPLE_DETAIL = 'Substitui um ataque · Atletismo';
/** The line under "Empurrar" in the attack list. */
export const SHOVE_DETAIL = 'Substitui um ataque · Atletismo';
/** The line under "Escapar" in the turn of a grappled combatant. */
export const ESCAPE_DETAIL = 'Ação · Atletismo ou Acrobacia, contra quem agarra';
/** What the attack list says of Agarrar and Empurrar. */
export const SPECIAL_ATTACKS_NOTE =
  'Agarrar e Empurrar usam o seu ataque. Com mais de um ataque, você escolhe qual troca.';

/** The note under the attacks of a hidden combatant: the advantage, and that the attack gives the position away for everyone. */
export function hiddenAttackNote(hidden: string): string {
  return `Atacar de onde você está ${hidden} dá vantagem ao ataque. Depois do ataque, acertando ou errando, você deixa de estar ${hidden} para todos.`;
}
