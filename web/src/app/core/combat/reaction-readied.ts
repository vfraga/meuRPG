import {
  type Encounter,
  ReactionKind,
  type ReactionWindow,
  type ReadiedAction,
  ReadiedKind,
  type TargetInReach,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import { joinDots, tight } from '../format/text';
import { metersFixed, metersText } from '../units';
import { stateWord } from './combat-view';
import { combatantIsPlayer, ofReactor } from './reaction-bar';
import { targetPhrase } from './ready-card';
import { heldName } from './ready-held';

/**
 * The window a readied action opens when the master fires it (#582, `ReactionKind.READIED`): what its readier reads in the
 * prompt (boards 582-06 to 582-08) and what the master reads in his queue (582-05). The window has no prompt of its own: the
 * trigger and the held action are in the readier's `Combatant.readied`, which only the readier's player and the master get
 * (RN-10). Pure functions, tested without a DOM.
 */

/** Whether the window is a readied action's. */
export function isReadiedWindow(w: ReactionWindow): boolean {
  return w.kind === ReactionKind.READIED;
}

/** The row the window answers: what its reactor holds, when the caller may read it. */
export function readiedOfWindow(e: Encounter, w: ReactionWindow): ReadiedAction | undefined {
  const row = e.combatants.find((c) => c.id === w.reactorId)?.readied;
  return row && row.id !== '' ? row : undefined;
}

/** How the prompt reads. */
export interface ReadiedPromptView {
  readonly title: string;
  readonly subtitle: string;
  readonly lead: string;
  /** The trigger in quotes; `''` when the caller cannot read it. */
  readonly quote: string;
  readonly question: string;
  readonly note: string;
  readonly declineLabel: string;
  /** The words of the one button that uses it ("Esquivar agora"); an attack's are made from the target. */
  readonly useLabel: string;
  /** Whether this app can answer "usar" for the held action: every kind is released from the prompt, a spell or a move that needs the map on the map (#857, #856). */
  readonly releasable: boolean;
  /** What the held action costs, one line each with its icon (boards 857-01 and 857-05); empty for the kinds that keep the single `note`. */
  readonly legend: readonly ReadiedLegendLine[];
  readonly ariaLabel: string;
}

/** One line of what a held action costs: the icon, a bold lead (`''` for none) and the rest. */
export interface ReadiedLegendLine {
  readonly icon: string;
  readonly lead: string;
  readonly text: string;
}

const COST_NOTE = 'Gasta a sua reação; ela volta no começo da sua vez.';
/** The note of every held spell, whether or not it keeps its concentration once released (product owner, #857). */
export const SPELL_NOTE =
  'Guardada, a magia prende a sua concentração. Deixar passar perde a magia e o espaço.';
const MOVE_COST = 'gasta quando você mover; volta no começo da sua vez.';
const PROVOKES = 'Provoca ataques de oportunidade, como qualquer movimento.';

/** The three lines of a held spell's window: the reaction, the slot spent at Ready and the concentration it holds. */
function spellLegend(slotLevel: number): readonly ReadiedLegendLine[] {
  const reaction = { icon: 'square', lead: 'Reação:', text: 'gasta quando a magia sair.' };
  const note = { icon: 'adjust', lead: '', text: SPELL_NOTE };
  return slotLevel > 0
    ? [
        reaction,
        {
          icon: 'auto_fix_normal',
          lead: 'Sem espaço:',
          text: `o de ${slotLevel}º nível foi gasto ao preparar.`,
        },
        note,
      ]
    : [reaction, note];
}

/** "a Espada longa", "o Arco curto": the held weapon with its article. */
function theHeld(name: string): string {
  return `${article(name)} ${name}`;
}

/** The prompt of the readier's window: `r` is the row it holds (`undefined` when the caller cannot read it). */
export function readiedPrompt(
  r: ReadiedAction | undefined,
  round: number,
  reactorLabel: string,
): ReadiedPromptView {
  const base = {
    lead: 'O mestre disse que o gatilho aconteceu:',
    quote: r ? `“${r.triggerText}”` : '',
    declineLabel: 'Deixar passar',
  };
  const name = r?.actionNamePt ?? '';
  if (!r) {
    return {
      ...base,
      title: 'Ação preparada',
      subtitle: `Reação · Rodada ${round}`,
      question: 'Usar a ação preparada agora?',
      note: COST_NOTE,
      useLabel: 'Usar agora',
      releasable: true,
      legend: [],
      ariaLabel: 'Ação preparada',
    };
  }
  switch (r.kind) {
    case ReadiedKind.SPELL:
      return {
        ...base,
        title: `Soltar ${name || 'a magia preparada'}`,
        subtitle: `Reação · a ação preparada de ${reactorLabel}`,
        question: `Soltar ${name || 'a magia preparada'} agora?`,
        note: SPELL_NOTE,
        useLabel: 'Soltar agora',
        releasable: true,
        legend: spellLegend(r.slotLevel),
        ariaLabel: `Soltar ${name || 'a magia preparada'}`,
      };
    case ReadiedKind.MOVE:
      return {
        ...base,
        title: 'Ação preparada',
        subtitle: `Reação · Rodada ${round}`,
        question: `Mover agora${r.moveFt > 0 ? `, até ${metersText(r.moveFt)}` : ''}?`,
        note: `${COST_NOTE} ${PROVOKES}`,
        useLabel: 'Mover agora',
        releasable: true,
        legend: [
          { icon: 'square', lead: 'Reação:', text: MOVE_COST },
          { icon: 'open_with', lead: '', text: PROVOKES },
        ],
        ariaLabel: 'Ação preparada: mover',
      };
    case ReadiedKind.OTHER:
      return {
        ...base,
        title: 'Ação preparada',
        subtitle: `Reação · Rodada ${round}`,
        question: `${name || 'Usar a ação preparada'} agora?`,
        note: COST_NOTE,
        useLabel: `${name || 'Usar'} agora`,
        releasable: true,
        legend: [],
        ariaLabel: `Ação preparada: ${name}`,
      };
    default:
      return {
        ...base,
        title: 'Ação preparada',
        subtitle: `Reação · Rodada ${round}`,
        question: `Atacar com ${name ? theHeld(name) : 'a arma preparada'} agora?`,
        note: COST_NOTE,
        useLabel: 'Atacar',
        releasable: true,
        legend: [],
        ariaLabel: `Ação preparada: atacar com ${name}`,
      };
  }
}

/** One creature of the list an attack of the window may be aimed at. */
export interface ReadiedTargetRow {
  readonly id: string;
  readonly title: string;
  readonly sub: string;
  /** Why it cannot be aimed at, written in its card; `''` when it can. */
  readonly blocked: string;
}

/** The creatures of the list, from the attack's targets as the server sent them: how hurt, how far, and its reason against one. */
export function readiedTargetRows(
  targets: readonly TargetInReach[],
  namedId: string,
  reachFt: number,
): ReadiedTargetRow[] {
  return targets.map((t) => {
    const parts: string[] = [];
    if (namedId !== '' && t.combatantId === namedId) {
      parts.push('O alvo que você disse');
    }
    const word = stateWord(t.state, t.label);
    if (word) {
      parts.push(word);
    }
    if (t.distanceFt !== undefined) {
      parts.push(`a ${metersFixed(t.distanceFt)}`);
    }
    const blocked = t.untargetable
      ? 'Cobertura total: não dá para mirar'
      : t.tooFar
        ? `Fora de alcance: até ${metersText(reachFt)}`
        : '';
    return { id: t.combatantId, title: t.label, sub: tight(joinDots(parts)), blocked };
  });
}

/** "Em quem · 1 criatura ao alcance". */
export function readiedLegend(rows: readonly ReadiedTargetRow[]): string {
  const n = rows.filter((r) => !r.blocked).length;
  return `Em quem · ${n === 0 ? 'ninguém' : `${n} ${n === 1 ? 'criatura' : 'criaturas'}`} ao alcance`;
}

/**
 * What the readier reads when the window closed with no answer from them. Nothing on the wire says who closed it (the time, or the
 * master passing or answering it by hand): the wait is said only when this screen's own clock ran out (`timedOut`).
 */
export function readiedGoneText(o: {
  readonly reactionUsed: boolean;
  readonly down: boolean;
  readonly timedOut: boolean;
  /** The player had been on the map: the wait that ran out was the one of 2 minutes. */
  readonly afterMap?: boolean;
}): string {
  if (o.down) {
    return 'Condição Caído.';
  }
  if (o.reactionUsed) {
    return 'A janela fechou e a sua reação foi usada.';
  }
  return o.timedOut
    ? `Passou sozinho: você não respondeu em ${o.afterMap ? '2 min' : '30 s'}.`
    : 'A janela fechou.';
}

/** What the master's row of the window says (the fields of `QueueRow` a readied action fills). */
export interface ReadiedMasterRow {
  /** "Ação preparada: Espada longa no Zumbi 1 · “Quando ...”". */
  readonly description: string;
  /** "Esperando Tor". */
  readonly waits: string;
  readonly useLabel: string;
  readonly passLabel: string;
}

/** The words of the use for each held kind. A move and a spell that needs the map open it for the master (#857, #856), so none is off. */
function useWords(r: ReadiedAction | undefined, who: string): Pick<ReadiedMasterRow, 'useLabel'> {
  switch (r?.kind) {
    case ReadiedKind.ATTACK:
      return { useLabel: `Atacar por ${who}` };
    case ReadiedKind.OTHER:
      return { useLabel: `${r.actionNamePt || 'Usar'} por ${who}` };
    case ReadiedKind.MOVE:
      return { useLabel: `Mover por ${who}` };
    case ReadiedKind.SPELL:
      return { useLabel: `Soltar por ${who}` };
    default:
      return { useLabel: `Usar por ${who}` };
  }
}

/** The master's words for the window: the held action, the trigger, and the two answers (the safe one first). */
export function readiedMasterRow(e: Encounter, w: ReactionWindow): ReadiedMasterRow {
  const r = readiedOfWindow(e, w);
  const target = r ? (e.combatants.find((c) => c.id === r.targetCombatantId)?.label ?? '') : '';
  const aimed = r?.kind === ReadiedKind.ATTACK || r?.kind === ReadiedKind.SPELL;
  const held = r ? heldName(r, aimed ? targetPhrase(target) : '') : (w.trigger?.actionNamePt ?? '');
  const description = tight(
    joinDots([`Ação preparada${held ? `: ${held}` : ''}`, r ? `“${r.triggerText}”` : '']),
  );
  return {
    description,
    waits: `Esperando ${w.reactorLabel}`,
    passLabel: 'Deixar passar',
    ...useWords(r, w.reactorLabel),
  };
}

/** The fields of the master's queue row for the window: what it holds, the wait the server writes while another window holds the roll (`reaction_wait`), and the answers. */
export function readiedRowFields(e: Encounter, w: ReactionWindow, before?: ReactionWindow) {
  const row = readiedMasterRow(e, w);
  const wait = e.reactionWait;
  return {
    description: row.description,
    status: !w.answerNow && wait?.titlePt ? joinDots([wait.titlePt, wait.detailPt]) : '',
    useLabel: row.useLabel,
    passLabel: row.passLabel,
    readied: {
      after:
        !w.answerNow && before
          ? `Depois ${ofReactor(before, before.reactorIsPlayer || combatantIsPlayer(e, before.reactorId))}`
          : '',
      aims: [ReadiedKind.ATTACK, ReadiedKind.SPELL].includes(readiedOfWindow(e, w)?.kind as never),
    },
  };
}
