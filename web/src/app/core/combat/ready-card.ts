import {
  ReadiedKind,
  ReadiedStatus,
  type Combatant,
  type ReadiedAction,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import { heldName } from './ready-held';

/** What the readied card says (#582, board 582-03): the held action, the trigger in quotes and where it stands. */
export interface ReadiedCardView {
  /** "Espada longa no Zumbi 1", "Mover, até 7,5 m", "Esquivar". */
  readonly title: string;
  /** The trigger, quoted. */
  readonly quote: string;
  /** The line under it: what the reaction costs on the readier's turn, what it waits for off it. */
  readonly state: string;
  /** A held spell is the readier's concentration: the card says so. */
  readonly concentration: boolean;
}

/** "no Zumbi 1", "na Brisa": the creature the trigger names, after the held action; `''` for none. */
export function targetPhrase(label: string): string {
  return label ? `${article(label) === 'a' ? 'na' : 'no'} ${label}` : '';
}

/** Whether the combatant holds an action that still waits for its trigger. */
export function holdsReadied(c: Pick<Combatant, 'readied'> | null | undefined): boolean {
  return !!c?.readied && c.readied.status === ReadiedStatus.WAITING;
}

/** The card of a readied action. `ownTurn`: the readier is on turn (its card has the "⋮" and says what the reaction costs). */
export function readiedCardView(
  r: ReadiedAction,
  combatants: readonly Pick<Combatant, 'id' | 'label'>[],
  o: { readonly ownTurn: boolean; readonly reactionUsed: boolean },
): ReadiedCardView {
  const named = combatants.find((c) => c.id === r.targetCombatantId)?.label ?? '';
  const target =
    r.kind === ReadiedKind.ATTACK || r.kind === ReadiedKind.SPELL ? targetPhrase(named) : '';
  return {
    title: heldName(r, target),
    quote: `“${r.triggerText}”`,
    state: o.ownTurn
      ? 'Gasta a reação quando o mestre disparar · se perde no começo da sua vez'
      : `Esperando o gatilho · ${o.reactionUsed ? 'reação usada' : 'reação livre'}`,
    concentration: r.kind === ReadiedKind.SPELL,
  };
}
