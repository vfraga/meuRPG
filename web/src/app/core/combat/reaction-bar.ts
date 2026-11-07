import {
  type Encounter,
  ReactionKind,
  type ReactionWindow,
  CombatantKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { ofThe } from './move-plan';
import { listNames } from './joint-turn';
import { openWindows } from './reactions';

/** "do Mago 1" for an NPC, "de Pensantus" for a player's character: a player's name takes no article. */
export function ofReactor(w: ReactionWindow, player: boolean): string {
  return player ? `de ${w.reactorLabel}` : ofThe([w.reactorLabel]);
}

export function combatantIsPlayer(e: Encounter, id: string): boolean {
  return e.combatants.some((c) => c.id === id && c.kind === CombatantKind.PLAYER);
}

/** Why a held action of the master is off while a window is open: "Espere a reação do Mago 1." or "Responda ao pedido acima.". */
export function heldReason(e: Encounter): string {
  const open = openWindows(e);
  const of = (w: ReactionWindow): string =>
    ofReactor(w, w.reactorIsPlayer || combatantIsPlayer(e, w.reactorId));
  if (open.length === 0) {
    return '';
  }
  const first = open.find((w) => w.answerNow) ?? open[0];
  if (
    first.kind === ReactionKind.MASTER_CHECK ||
    first.kind === ReactionKind.LEGENDARY_RESISTANCE
  ) {
    return 'Responda ao pedido acima.';
  }
  if (first.kind === ReactionKind.CONCENTRATION_SAVE) {
    return `Espere o teste de concentração ${of(first)}.`;
  }
  return first.reactorLabel ? `Espere a reação ${of(first)}.` : 'Responda ao pedido acima.';
}

/** The master's bar while a window is open: "Esperando a sua reação: Mago 1" or who the master waits on. */
export function windowsBarText(e: Encounter): string {
  const open = openWindows(e);
  if (open.length === 0) {
    return '';
  }
  // A player's reaction that came to the master after 10 s without an answer (C8-04).
  const late = open.filter((w) => w.unanswered);
  if (late.length > 0) {
    // Repreensão Infernal's second step is the master's roll of the aggressor's save: the player has answered already.
    const rolls = late.filter((w) => w.secondStep && w.prompt.case === 'hellishRebukeSave');
    const due = late.filter((w) => !rolls.includes(w));
    const aggressors = rolls.flatMap((w) =>
      w.prompt.case === 'hellishRebukeSave' ? [w.prompt.value.aggressorLabel] : [],
    );
    const parts = [
      due.length > 0
        ? `responder por ${listNames([...new Set(due.map((w) => w.reactorLabel))])}`
        : '',
      rolls.length > 0 ? `o teste ${ofThe([...new Set(aggressors)])}` : '',
    ];
    return `Falta ${parts.filter(Boolean).join(' e ')}`;
  }
  const mine = open.filter(
    (w) => !w.reactorIsPlayer && !combatantIsPlayer(e, w.reactorId) && w.reactorLabel,
  );
  // The Legendary Resistance window has no reactor: it is the master's alone to answer.
  const asked = open.some(
    (w) => w.kind === ReactionKind.MASTER_CHECK || w.kind === ReactionKind.LEGENDARY_RESISTANCE,
  );
  if (mine.length > 0 || asked) {
    return mine.length > 0
      ? `Esperando a sua reação: ${listNames([...new Set(mine.map((w) => w.reactorLabel))])}`
      : 'Esperando a sua resposta';
  }
  return `Esperando a reação de ${listNames([...new Set(open.map((w) => w.reactorLabel))])}`;
}
