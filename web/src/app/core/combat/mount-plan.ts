import type { Combatant } from '../../../gen/meurpg/play/v1/combat_pb';
import { type MountChoice, MountRefusal } from '../../../gen/meurpg/rules/v1/rules_pb';
import { metersFixed } from '../units';
import { type Square } from './combat-grid';
import { combatantInitial } from './combat-view';
import { article } from './combat-log';
import { lineLengthDft } from './jump-plan';

/**
 * Mounting and dismounting on the "Mover" page (RN-21, "Combate montado"): the server decides what can be mounted and says why
 * not (`TurnOptions.mount`); these functions only word that, and name the distance a refused square is from the rider. Pure
 * TypeScript, tested without a DOM.
 */

const DFT_PER_FT = 10;

/** One row of the "Montaria" panel. */
export interface MountRow {
  readonly id: string;
  readonly name: string;
  readonly initial: string;
  /** "Ao lado · custa 3,8 m", or the reason the button is dashed. */
  readonly line: string;
  /** The mount cannot be mounted now. */
  readonly refused: boolean;
}

const cost = (dft: number): string => metersFixed(dft / DFT_PER_FT);

/** Who sits on the combatant, if the viewer sees them. */
function riderOf(mountId: string, combatants: readonly Combatant[]): string {
  return combatants.find((c) => c.mountedOnId === mountId)?.label ?? '';
}

/** Why a mount cannot be mounted, in a sentence. The server says nothing of what the viewer does not see (RN-10): a rider
 * that is hidden leaves "Já leva alguém." */
export function mountRefusalText(
  choice: MountChoice,
  combatants: readonly Combatant[],
  from: Square,
): string {
  const mount = combatants.find((c) => c.id === choice.combatantId);
  switch (choice.refusal) {
    case MountRefusal.TOO_SMALL:
      return 'Pequeno demais para você: precisa ser maior.';
    case MountRefusal.TOO_FAR: {
      const away = mount ? lineLengthDft(from, { col: mount.col, row: mount.row }) : 0;
      return `A ${cost(away)}: chegue ao lado dele primeiro.`;
    }
    case MountRefusal.NO_MOVEMENT:
      return `Falta movimento: custa ${cost(choice.costDft)}, restam ${cost(Math.max(choice.costDft - choice.missingDft, 0))}.`;
    case MountRefusal.MOVE_USED:
      return 'Você já montou ou desmontou neste movimento.';
    case MountRefusal.TAKEN:
      return `Já leva ${riderOf(choice.combatantId, combatants) || 'alguém'}.`;
    case MountRefusal.ALREADY_MOUNTED:
      return 'Você já está montado.';
    default:
      return '';
  }
}

/** The rows of the panel, in the server's order. */
export function mountRows(
  mounts: readonly MountChoice[],
  combatants: readonly Combatant[],
  from: Square,
): MountRow[] {
  return mounts.flatMap((m) => {
    const mount = combatants.find((c) => c.id === m.combatantId);
    if (!mount) {
      return [];
    }
    const reason = mountRefusalText(m, combatants, from);
    return [
      {
        id: m.combatantId,
        name: mount.label,
        initial: combatantInitial(mount.label),
        line: reason || `Ao lado · custa ${cost(m.costDft)}`,
        refused: reason !== '',
      },
    ];
  });
}

/** Why getting off is not possible now, "" when it is. `rider` is the name that has the movement left. */
export function dismountRefusalText(
  refusal: MountRefusal,
  costDft: number,
  missingDft: number,
  rider: string,
): string {
  switch (refusal) {
    case MountRefusal.NO_ROOM:
      return 'Nenhum quadrado livre ao lado do cavalo.';
    case MountRefusal.NO_MOVEMENT:
      return `Falta movimento: desmontar custa ${cost(costDft)}, restam ${cost(Math.max(costDft - missingDft, 0))} a ${rider}.`;
    case MountRefusal.MOVE_USED:
      return 'Você já montou ou desmontou neste movimento.';
    default:
      return '';
  }
}

const on = (mount: string, feminine: string, masculine: string): string =>
  `${article(mount) === 'a' ? feminine : masculine} ${mount}`;

/** "Montado no Cavalo de guerra." */
export function mountedNotice(mount: string): string {
  return `Montado ${on(mount, 'na', 'no')}.`;
}

/** What follows it: whose movement it is, and that the rider goes along. */
export function mountedDetail(rider: string): string {
  return `O movimento é dele, e ${rider} vai junto no mesmo quadrado.`;
}

/** The rider's walk is the mount's: "Depois restam 3,0 m ao Cavalo de guerra. Os 9,0 m de Tor ficam para desmontar." */
export function rideDetail(
  leftAfterDft: number,
  mount: string,
  riderLeftDft: number,
  rider: string,
): string {
  return `Depois restam ${cost(leftAfterDft)} ${on(mount, 'à', 'ao')}. Os ${cost(riderLeftDft)} de ${rider} ficam para desmontar.`;
}

/** The card of a chosen landing square. */
export function dismountCard(
  costDft: number,
  riderLeftDft: number,
  rider: string,
  controlled: boolean,
): { readonly title: string; readonly detail: string } {
  const after = Math.max(riderLeftDft - costDft, 0);
  const back = controlled ? ' O cavalo volta à iniciativa dele.' : '';
  return {
    title: `Desmontar aqui · ${cost(costDft)}`,
    detail: `Depois restam ${cost(after)} a ${rider}, a pé.${back}`,
  };
}

/** The line under the title while the rider picks where to get off. */
export function dismountLead(costDft: number, rider: string): string {
  return `Desmontar custa ${cost(costDft)}, metade do deslocamento ${on(rider, 'da', 'de')}. Toque num quadrado ao lado do cavalo.`;
}

/** The landing square nearest to where the arrows pointed, so a nudge never lands on a square that cannot be chosen. */
export function nearestLanding(target: Square, landing: readonly Square[]): Square | null {
  let best: Square | null = null;
  let bestDistance = Infinity;
  for (const sq of landing) {
    const d = lineLengthDft(sq, target);
    if (d < bestDistance) {
      best = sq;
      bestDistance = d;
    }
  }
  return best;
}
