import { type CombatLogEntry, CombatLogKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { article } from '../format/article';
import type { LogLine } from './combat-log-types';

/** What a builder of these lines is given: the entry and the fields every line shares. */
interface MountArgs {
  readonly e: CombatLogEntry;
  readonly base: { id: string; actor: string; hidden: boolean; undoable: boolean };
}

/** The numbers of a rider's save: "(15 + 2 = 17, CD 10)" for the master and the rider's player (with no DC
 * for a player whose table hides it), "(falha automática)" for a rider that
 * failed with no d20, and nothing for another player, who is sent no number. */
function mountSaveRoll(e: CombatLogEntry): string {
  const save = e.mountSave;
  if (save?.autoFail) {
    return ' (falha automática)';
  }
  if (!save?.d20) {
    return '';
  }
  const modifier = save.total - save.d20;
  const sign = modifier < 0 ? '-' : '+';
  // The DC is a player's only when the table shows the DC of saves (RN-24): the server sends 0 otherwise.
  const dc = save.dc > 0 ? `, CD ${save.dc}` : '';
  return ` (${save.d20} ${sign} ${Math.abs(modifier)} = ${save.total}${dc})`;
}

/** The master's note of a fall that found no free square next to the mount. */
function landing(e: CombatLogEntry): string {
  const save = e.mountSave;
  if (!save?.noRoomNextToMount) {
    return '';
  }
  const mount = e.targetLabel
    ? `${article(e.targetLabel) === 'a' ? 'da' : 'do'} ${e.targetLabel}`
    : 'da montaria';
  return ` e parou no quadrado livre mais perto: não havia lugar ao lado ${mount}`;
}

/** "no Cavalo de guerra", "na Aranha": the mount with the article that agrees with its name. */
function onMount(label: string, preposition: 'em' | 'de'): string {
  if (!label) {
    return preposition === 'em' ? 'em uma montaria' : 'de uma montaria';
  }
  const feminine = article(label) === 'a';
  const word = preposition === 'em' ? (feminine ? 'na' : 'no') : feminine ? 'da' : 'do';
  return `${word} ${label}`;
}

const line = (args: MountArgs, text: string): LogLine => ({ ...args.base, icon: 'pets', text });

/** The lines of a mount (SRD 5.1, "Mounting and Dismounting", "Mounted Combat"). The server sends a line only to who sees both
 * combatants, and the master's alone when either is hidden. A save's numbers are the master's and the rider's player's (RN-10). */
export const MOUNT_LINES: readonly (readonly [CombatLogKind, (args: MountArgs) => LogLine])[] = [
  [CombatLogKind.MOUNTED, (a) => line(a, ` montou ${onMount(a.e.targetLabel, 'em')}`)],
  [CombatLogKind.DISMOUNTED, (a) => line(a, ` desmontou ${onMount(a.e.targetLabel, 'de')}`)],
  [CombatLogKind.MOUNT_SAVE, (a) => line(a, ` se segurou na montaria${mountSaveRoll(a.e)}`)],
  [
    CombatLogKind.RIDER_THROWN,
    (a) => line(a, ` caiu da montaria${mountSaveRoll(a.e)}${landing(a.e)}`),
  ],
];
