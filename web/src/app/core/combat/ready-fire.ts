import {
  EncounterStatus,
  ReactionKind,
  ReactionWindowStatus,
  type Combatant,
  type Encounter,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import { isDown } from './combat-view';
import { cannotAct } from './conditions';
import { holdsReadied, readiedCardView } from './ready-card';

/** One row of the master's "Ações preparadas" (#582, boards 582-04 and 582-09). */
export interface FireRow {
  /** `ReadiedAction.id`: what `FireReadied` names. */
  readonly id: string;
  readonly combatantId: string;
  readonly who: string;
  /** "Espada longa no Zumbi 1", "Mover, até 7,5 m". */
  readonly held: string;
  /** The trigger, quoted. */
  readonly quote: string;
  /** The readier is hidden from the players: only the master reads its row. */
  readonly hidden: boolean;
  /** Why "Disparar" is off, in the board's words; `''` when the master may fire it. */
  readonly why: string;
}

/** "o Zumbi 1", "a Brisa". */
function withArticle(label: string): string {
  return `${article(label)} ${label}`;
}

/** "o Zumbi 1", "o Zumbi 1 e a Brisa", "o Zumbi 1, a Brisa e o Tor". */
function listOf(labels: readonly string[]): string {
  const named = labels.map(withArticle);
  return named.length < 2
    ? (named[0] ?? '')
    : `${named.slice(0, -1).join(', ')} e ${named[named.length - 1]}`;
}

/**
 * Why the master cannot fire this readier's action now, from what the combat already tells him, in the order `FireReadied` checks
 * (docs/architecture/play/readying.md): a readier out of the fight, surprised, acting now, a concentration save or a hidden reveal
 * that wait, incapacitated, its reaction spent, its window open. `''` when none holds; the server still decides.
 */
function fireWhy(e: Encounter, c: Combatant, surprised: readonly string[]): string {
  const windows = e.reactionWindows.filter(
    (w) => w.status === ReactionWindowStatus.OPEN && w.reactorId === c.id,
  );
  if (c.defeated || (c.hitPointsCurrent ?? 1) <= 0 || isDown(c)) {
    return `${c.label} está Caído (0 PV): não reage.`;
  }
  if (surprised.includes(c.id)) {
    return `${c.label} foi surpreendido: só reage depois da primeira vez dele.`;
  }
  const group = e.turnGroupIds.length > 1 ? e.turnGroupIds : [e.currentCombatantId];
  if (group.includes(c.id) && !c.turnPartEnded) {
    const others = group
      .filter((id) => id !== c.id)
      .map((id) => e.combatants.find((m) => m.id === id)?.label ?? '')
      .filter(Boolean);
    return others.length > 0
      ? `${c.label} ainda joga a parte dele da vez, junto com ${listOf(others)}. Dispare depois que ele encerrar a parte dele.`
      : `É a vez de ${c.label}: só dispara fora da vez dele.`;
  }
  if (windows.some((w) => w.kind === ReactionKind.CONCENTRATION_SAVE)) {
    return `Falta o teste de concentração de ${c.label}.`;
  }
  if (e.pendingHiddenReveals.length > 0) {
    return 'Responda antes a pergunta sobre a criatura escondida.';
  }
  if (cannotAct(c)) {
    return `${c.label} está Incapacitado: não reage.`;
  }
  if (c.reactionUsed) {
    return `${c.label} já usou a reação nesta rodada.`;
  }
  if (windows.some((w) => w.kind === ReactionKind.READIED)) {
    return `A janela de ${c.label} já está aberta: espere a resposta.`;
  }
  return '';
}

/**
 * The readied actions that wait, for the master's "Ações preparadas", in the order of the fight. `surprised` is the list the contest
 * state gives (`GetContestState`, the combatants marked surprised). The server sends `Combatant.readied` to its owner and the master
 * alone (RN-10): nothing here decides who may read it.
 */
export function fireRows(e: Encounter, surprised: readonly string[]): readonly FireRow[] {
  if (e.status !== EncounterStatus.ACTIVE) {
    return [];
  }
  return e.combatants
    .filter((c) => holdsReadied(c))
    .map((c) => {
      const r = c.readied!;
      const view = readiedCardView(r, e.combatants, {
        ownTurn: false,
        reactionUsed: c.reactionUsed,
      });
      return {
        id: r.id,
        combatantId: c.id,
        who: c.label,
        held: view.title,
        quote: view.quote,
        hidden: c.hidden,
        why: fireWhy(e, c, surprised),
      };
    });
}
