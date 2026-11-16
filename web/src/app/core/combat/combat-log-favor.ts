import { type CombatLogEntry, CombatLogKind } from '../../../gen/meurpg/play/v1/combat_pb';
import type { LogLine } from './combat-log-types';

/** What a favor line builder is given. */
interface FavorLineArgs {
  readonly e: CombatLogEntry;
  readonly base: Omit<LogLine, 'icon' | 'text'>;
}

/** A line about the Inspiração (RN-43, board 581-07, state 11): the server writes the whole sentence for the reader, with only the names
 * and the facts that reader may read ("O mestre devolveu a Inspiração de Jabuti.", "Brisa passou a Inspiração para você."), so the
 * screen never builds one of its own: a reader that has no line gets no entry at all. The sentence stands alone, with no actor. */
function favorLine(icon: string) {
  return ({ e, base }: FavorLineArgs): LogLine | null => {
    const text = e.favor?.textPt ?? '';
    return text === '' ? null : { ...base, icon, actor: '', text };
  };
}

/** The three lines of the Inspiração's pass and refund. */
export const FAVOR_LINES: readonly (readonly [
  CombatLogKind,
  (args: FavorLineArgs) => LogLine | null,
])[] = [
  [CombatLogKind.FAVOR_PASSED, favorLine('keyboard_tab')],
  [CombatLogKind.FAVOR_REFUNDED, favorLine('star_border')],
  [CombatLogKind.FAVOR_PASS_ANSWERED, favorLine('keyboard_tab')],
];
