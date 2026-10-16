import type { Combatant, Encounter } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type CheckRoll,
  type ContestSkillOption,
  type ContestView,
  type GroupCheckMemberView,
  type GroupCheckView,
  type HideAttemptView,
  type HideObserver,
  type SurpriseSuggestion,
  ContestPurpose,
  ContestSkill,
  ContestStatus,
  ContestWinner,
  ShoveBlockedReason,
  SurpriseSuggestionReason,
} from '../../../gen/meurpg/play/v1/contest_types_pb';
import { article } from './combat-log';
import {
  type Who,
  countedFace,
  named,
  pronoun,
  signed,
  skillLine,
  verb,
  whoIs,
} from './contest-view';
import { upFirst } from '../format/text';

/**
 * What the master's contest cards say (W7-X, boards W7-Xa 3, W7-Xb 4 to 7, W7-Xc 8, 10 and 11): the sentences of the answer to a
 * contest, the shove's choice, the Hide decision, the group check and the surprise. The master reads what a player never does
 * (an NPC's total, the escape DC, who noticed a hider, the group's verdict, RN-10 and RN-20); the server already left it out of
 * every player's reply, so nothing here hides it again. Pure functions, tested without a DOM.
 */

/** The label of a combatant, `alguém` when the id names none. */
function labelOf(e: Encounter | null, id: string): string {
  return e?.combatants.find((c) => c.id === id)?.label ?? 'alguém';
}

function combatantOf(e: Encounter | null, id: string): Combatant | undefined {
  return e?.combatants.find((c) => c.id === id);
}

/** The title of a contest card: "Toren tenta agarrar o Hobgoblin", "Hobgoblin tenta agarrar Brisa", "Brisa tenta escapar". */
export function masterContestTitle(e: Encounter | null, c: ContestView): string {
  const who = labelOf(e, c.initiatorId);
  if (c.purpose === ContestPurpose.ESCAPE) {
    return `${who} tenta escapar`;
  }
  return `${who} tenta ${verb(c.purpose)} ${named(whoIs(combatantOf(e, c.defenderId)))}`;
}

/** "pelo Hobgoblin", "pela Cobra", "por Brisa": who the master rolls for. */
export function byWho(who: Who): string {
  return who.player
    ? `por ${who.label}`
    : `${article(who.label) === 'a' ? 'pela' : 'pelo'} ${who.label}`;
}

/** "Toren: Força (Atletismo)" over the roll line. */
export function rollHead(label: string, skill: ContestSkill): string {
  return `${label}: ${skillLine(skill)}`;
}

/** "15 + 5 = 20": the d20 that counts, the modifier, the total. */
export function totalLine(roll: CheckRoll): string {
  const mod = roll.modifier < 0 ? `− ${Math.abs(roll.modifier)}` : `+ ${roll.modifier}`;
  return `${countedFace(roll)} ${mod} = ${roll.total}`;
}

/** The skill the app suggests for the defender: the one with the higher modifier. */
export function suggestedSkill(options: readonly ContestSkillOption[]): ContestSkill {
  return (options.find((o) => o.suggested) ?? options[0])?.skill ?? ContestSkill.ATHLETICS;
}

/** "Atletismo (+1) Acrobacia (+1)": what each skill would roll, for the line under the choice. */
export function optionLabel(o: ContestSkillOption): string {
  const name = o.skill === ContestSkill.ACROBATICS ? 'Acrobacia' : 'Atletismo';
  return o.known ? `${name} (${signed(o.modifier)})` : `${name} (modificador desconhecido)`;
}

/** What the card says of the verdict once the master rolled: "Toren vence", "Empate: nada muda", "O Hobgoblin vence". */
export function winnerLine(e: Encounter | null, c: ContestView): string {
  switch (c.winner) {
    case ContestWinner.INITIATOR:
      return `${labelOf(e, c.initiatorId)} vence`;
    case ContestWinner.DEFENDER:
      return `${upFirst(named(whoIs(combatantOf(e, c.defenderId))))} vence`;
    case ContestWinner.TIE:
      return 'Empate: nada muda';
    default:
      return '';
  }
}

/** Whether the contest waits for a player's own answer (a player defender who has not left the roll to the master). */
export function waitingForPlayer(c: ContestView): boolean {
  return c.status === ContestStatus.AWAITING_DEFENDER && !c.deferred && c.waitingCombatantId !== '';
}

/** The line under "Empurrar 1,5 m" in the master's choice: the push is blocked by a wall, a creature, or it is free. */
export function masterPushLine(shover: string, blocked: ShoveBlockedReason): string {
  switch (blocked) {
    case ShoveBlockedReason.WALL:
      return `Para longe ${shover}. Há uma parede na casa de trás: ele não sai do lugar.`;
    case ShoveBlockedReason.CREATURE:
      return `Para longe ${shover}. Há uma criatura na casa de trás: ele não sai do lugar.`;
    default:
      return `Para longe ${shover}, em linha reta.`;
  }
}

/** "Esperando a sua resposta" lines of the answer card, by who the contest waits for. */
export function answerWait(e: Encounter | null, c: ContestView): string {
  if (c.deferred) {
    const defender = labelOf(e, c.defenderId);
    return `${defender} deixou o mestre rolar por ${pronoun(defender)}.`;
  }
  if (waitingForPlayer(c)) {
    return `Esperando ${labelOf(e, c.waitingCombatantId)}. O jogador está respondendo no celular.`;
  }
  return 'Esperando a sua resposta.';
}

// ---- the grapple of an NPC with a fixed escape DC ----

/** The lowest and the highest escape DC the server takes. */
export const ESCAPE_DC_MIN = 1;
export const ESCAPE_DC_MAX = 40;

/** Whether `text` is a whole escape DC the server takes (1 to 40). */
export function escapeDcOf(text: string): number | null {
  const n = Number(text.trim());
  return text.trim() !== '' && Number.isInteger(n) && n >= ESCAPE_DC_MIN && n <= ESCAPE_DC_MAX
    ? n
    : null;
}

// ---- Hide ----

/** One observer of a hider, as the master's card lists it. */
export interface ObserverRow {
  readonly id: string;
  readonly label: string;
  readonly passive: string;
  readonly noticed: boolean;
}

/** The observers of a pending Hide with their passive Perception and whether they notice the total. */
export function observerRows(
  e: Encounter | null,
  attempt: HideAttemptView,
): readonly ObserverRow[] {
  return attempt.observers.map((o: HideObserver) => ({
    id: o.combatantId,
    label: labelOf(e, o.combatantId),
    passive: o.known
      ? `Percepção passiva ${o.passivePerception}`
      : 'Percepção passiva desconhecida',
    noticed: o.noticed,
  }));
}

/** "Aplicar: escondida (3 não notam)" / "Aplicar: escondido (1 não nota)" / "Aplicar: ninguém é enganado". */
export function applyLabel(hidden: string, notNoticing: number): string {
  if (notNoticing === 0) {
    return 'Aplicar: ninguém é enganado';
  }
  return `Aplicar: ${hidden} (${notNoticing} ${notNoticing === 1 ? 'não nota' : 'não notam'})`;
}

/** How many of the observers do not notice the hider: those the total beats and the master did not mark "Vê claramente". */
export function notNoticing(rows: readonly ObserverRow[], clearly: ReadonlySet<string>): number {
  return rows.filter((r) => !r.noticed && !clearly.has(r.id)).length;
}

/** The longest refusal the server takes. */
export const REFUSAL_MAX = 120;

// ---- the group check ----

/** The line under the group check's title: "4 de 5 responderam. O grupo passa se ao menos metade dos convocados passar.". */
export function groupProgress(g: GroupCheckView): string {
  const answered = g.members.filter((m) => m.answered).length;
  const base = `${answered} de ${g.members.length} responderam.`;
  return g.group ? `${base} O grupo passa se ao menos metade dos convocados passar.` : base;
}

/**
 * The title of the card: "Teste em grupo: Furtividade, CD 13" (no DC: no ", CD"). A request judged one by one is
 * "Teste pedido: Furtividade, CD 13": it is not a group check.
 */
export function groupTitle(g: GroupCheckView): string {
  return `${g.group ? 'Teste em grupo' : 'Teste pedido'}: ${g.skillNamePt}${g.dc > 0 ? `, CD ${g.dc}` : ''}`;
}

/**
 * What the master asked, as a noun phrase for a sentence: "um teste de Percepção", "um teste de Força", "um teste de
 * resistência de Constituição" (the server names a check "Teste de Força" and a saving throw "Teste de resistência de ...").
 */
export function askedTest(skillNamePt: string): string {
  return skillNamePt.startsWith('Teste de ')
    ? `um t${skillNamePt.slice(1)}`
    : `um teste de ${skillNamePt}`;
}

/** The verdict only the master reads: "2 de 5 passaram; precisa de 3. O grupo falhou.". */
export function groupVerdict(g: GroupCheckView): string {
  if (!g.verdictKnown) {
    return '';
  }
  return `${g.passedCount} de ${g.members.length} passaram; precisa de ${g.needed}. ${
    g.groupPassed ? 'O grupo passou.' : 'O grupo falhou.'
  }`;
}

/** What a member's row says of the roll: "Furtividade +7" is the first line, this is the second ("19 Passou", "Não respondeu"). */
export function memberResult(m: GroupCheckMemberView): string {
  if (!m.answered || !m.roll) {
    return 'Não respondeu';
  }
  const pass = m.passedKnown ? (m.passed ? ' Passou' : ' Falhou') : '';
  return `${m.roll.total}${pass}`;
}

/** The bonus a member's row names: "Furtividade +7" (the roll's own once rolled). */
export function memberBonus(g: GroupCheckView, m: GroupCheckMemberView): string {
  return m.roll ? `${g.skillNamePt} ${signed(m.roll.modifier)}` : g.skillNamePt;
}

// ---- surprise ----

/** One row of "Quem está surpreso?". */
export interface SurpriseRow {
  readonly id: string;
  readonly label: string;
  readonly detail: string;
  readonly suggested: boolean;
}

function surpriseDetail(e: Encounter, s: SurpriseSuggestion): string {
  const passive = `Percepção passiva ${s.passivePerception}`;
  const hider = s.hiders.find((h) => h.beats) ?? s.hiders[0];
  switch (s.reason) {
    case SurpriseSuggestionReason.HIDERS_BEAT:
      return hider
        ? `${passive} · Furtividade de ${labelOf(e, hider.combatantId)} ${hider.stealthTotal} vence`
        : passive;
    case SurpriseSuggestionReason.NOTICES:
      return hider
        ? `${passive} · Furtividade de ${labelOf(e, hider.combatantId)} ${hider.stealthTotal} não vence: nota`
        : `${passive} · nota`;
    default:
      return `${passive} · Não está escondido: nota`;
  }
}

/** The rows of the suggestion, in the server's order; the master's own mark is `surprised`. */
export function surpriseRows(
  e: Encounter,
  suggestions: readonly SurpriseSuggestion[],
): readonly SurpriseRow[] {
  return suggestions.map((s) => ({
    id: s.combatantId,
    label: labelOf(e, s.combatantId),
    detail: surpriseDetail(e, s),
    suggested: s.suggested,
  }));
}
