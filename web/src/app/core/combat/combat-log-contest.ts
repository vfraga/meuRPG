import { type CombatLogEntry } from '../../../gen/meurpg/play/v1/combat_pb';
import { ContestLogLine } from '../../../gen/meurpg/play/v1/contest_types_pb';
import { LogContext } from './combat-log-types';
import { ofSubject, subject } from './combat-log-words';

/** What each line of a contest or special action says (W7-X) and its icon: the sentence follows the actor, who is in bold. */
const CONTEST_LINES: Partial<
  Record<ContestLogLine, { icon: string; say: (w: ContestWords) => string }>
> = {
  [ContestLogLine.GRAPPLED]: { icon: 'pan_tool', say: (w) => ` agarrou ${w.target}` },
  [ContestLogLine.GRAPPLE_FAILED]: {
    icon: 'pan_tool',
    say: (w) => ` não conseguiu agarrar ${w.target}`,
  },
  [ContestLogLine.SHOVE_PRONE]: { icon: 'open_with', say: (w) => ` derrubou ${w.target}` },
  [ContestLogLine.SHOVE_PUSHED]: {
    icon: 'open_with',
    say: (w) => ` empurrou ${w.target} 1,5\u00a0m`,
  },
  [ContestLogLine.SHOVE_STAYS]: {
    icon: 'open_with',
    say: (w) => ` empurrou ${w.target}, que não saiu do lugar`,
  },
  [ContestLogLine.SHOVE_FAILED]: {
    icon: 'open_with',
    say: (w) => ` não conseguiu empurrar ${w.target}`,
  },
  [ContestLogLine.ESCAPED]: { icon: 'pan_tool', say: (w) => ` se soltou ${w.from}` },
  [ContestLogLine.ESCAPE_FAILED]: {
    icon: 'pan_tool',
    say: (w) => ` não conseguiu se soltar ${w.from}`,
  },
  [ContestLogLine.CLOSED]: {
    icon: 'pan_tool',
    say: (w) => ` teve a disputa com ${w.target} encerrada pelo mestre`,
  },
  [ContestLogLine.RELEASED]: { icon: 'pan_tool', say: (w) => ` soltou ${w.target}` },
  [ContestLogLine.HIDE_TRIED]: { icon: 'visibility_off', say: () => ' tentou se esconder' },
  [ContestLogLine.HIDE_APPLIED]: { icon: 'visibility_off', say: () => ' se escondeu' },
  [ContestLogLine.HIDE_REFUSED]: {
    icon: 'visibility_off',
    say: () => ' não conseguiu se esconder',
  },
  [ContestLogLine.HELPED]: { icon: 'handshake', say: (w) => ` ajudou ${w.target}` },
  [ContestLogLine.SURPRISED]: { icon: 'bolt', say: () => ' sofre surpresa' },
  [ContestLogLine.SURPRISE_CLEARED]: {
    icon: 'bolt',
    say: () => ' não sofre mais surpresa',
  },
  [ContestLogLine.STOOD_UP]: { icon: 'accessibility_new', say: () => ' se levantou' },
  [ContestLogLine.HIDE_REVEALED]: {
    icon: 'visibility',
    say: () => ' não se esconde mais',
  },
};

/** The names a contest line is written with: the target ("o Hobgoblin", "Brisa"), "do Hobgoblin". */
interface ContestWords {
  readonly target: string;
  readonly from: string;
}

/** A contest or a special action (W7-X): the sentence from the line the server named, never a total or a DC (RN-20). `null` for a line this app does not know. */
export function contestText(
  e: CombatLogEntry,
  ctx: LogContext,
): { icon: string; text: string } | null {
  const line = e.contest ? CONTEST_LINES[e.contest.line] : undefined;
  if (!line) {
    return null;
  }
  const label = e.targetLabel || 'alguém';
  return {
    icon: line.icon,
    text: line.say({
      target: subject(label, ctx),
      from: ofSubject(label, ctx),
    }),
  };
}
