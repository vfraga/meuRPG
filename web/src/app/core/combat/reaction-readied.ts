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
  /** Whether this app can answer "usar" for the held action: an attack and another action yes, a move not yet (it is made on the map). */
  readonly releasable: boolean;
  /** Said under the question when the held action cannot be used from the prompt. */
  readonly limit: string;
  readonly ariaLabel: string;
}

const COST_NOTE = 'Gasta a sua reação; ela volta no começo da sua vez.';
const SPELL_NOTE =
  'A concentração acaba quando a magia sai. Deixar passar perde a magia e o espaço gasto ao preparar.';
const ONLY_PASS = 'Por aqui só dá para deixar passar.';

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
      limit: '',
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
        limit: '',
        ariaLabel: `Soltar ${name || 'a magia preparada'}`,
      };
    case ReadiedKind.MOVE:
      return {
        ...base,
        title: 'Ação preparada',
        subtitle: `Reação · Rodada ${round}`,
        question: `Mover agora${r.moveFt > 0 ? `, até ${metersText(r.moveFt)}` : ''}?`,
        note: `${COST_NOTE} Provoca ataques de oportunidade, como qualquer movimento.`,
        useLabel: 'Mover agora',
        releasable: false,
        limit: ONLY_PASS,
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
        limit: '',
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
        limit: '',
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
 * master passing or answering it by hand): the 30 s are said only when this screen's own clock ran out (`timedOut`).
 */
export function readiedGoneText(o: {
  readonly reactionUsed: boolean;
  readonly down: boolean;
  readonly timedOut: boolean;
}): string {
  if (o.down) {
    return 'Você ficou Caído.';
  }
  if (o.reactionUsed) {
    return 'A janela fechou e a sua reação foi usada.';
  }
  return o.timedOut ? 'Passou sozinho: você não respondeu em 30 s.' : 'A janela fechou.';
}

/** What the master's row of the window says (the fields of `QueueRow` a readied action fills). */
export interface ReadiedMasterRow {
  /** "Ação preparada: Espada longa no Zumbi 1 · “Quando ...”". */
  readonly description: string;
  /** "Esperando Tor". */
  readonly waits: string;
  readonly useLabel: string;
  readonly passLabel: string;
  /** Why "use" is not offered from the queue, or `''`. */
  readonly useOff: string;
}

/** The words of the use for each held kind, and why a move cannot be used from the queue. */
function useWords(
  r: ReadiedAction | undefined,
  who: string,
): Pick<ReadiedMasterRow, 'useLabel' | 'useOff'> {
  switch (r?.kind) {
    case ReadiedKind.ATTACK:
      return { useLabel: `Atacar por ${who}`, useOff: '' };
    case ReadiedKind.OTHER:
      return { useLabel: `${r.actionNamePt || 'Usar'} por ${who}`, useOff: '' };
    case ReadiedKind.MOVE:
      return {
        useLabel: `Mover ${who}`,
        useOff: `Mover é pelo mapa: mova ${who} com a janela aberta.`,
      };
    case ReadiedKind.SPELL:
      return { useLabel: `Soltar por ${who}`, useOff: '' };
    default:
      return { useLabel: `Usar por ${who}`, useOff: '' };
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
export function readiedRowFields(e: Encounter, w: ReactionWindow) {
  const row = readiedMasterRow(e, w);
  const wait = e.reactionWait;
  return {
    description: row.description,
    status: !w.answerNow && wait?.titlePt ? joinDots([wait.titlePt, wait.detailPt]) : '',
    useLabel: row.useLabel,
    passLabel: row.passLabel,
    readied: {
      waits: row.waits,
      useOff: row.useOff,
      aims: [ReadiedKind.ATTACK, ReadiedKind.SPELL].includes(readiedOfWindow(e, w)?.kind as never),
    },
  };
}
