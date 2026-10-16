import type { ReactionWindow } from '../../../../../gen/meurpg/play/v1/combat_pb';
import { type SlotRow, slotRows } from '../../../../core/combat/cast-flow';
import type { ReactionAnswer } from '../../../../core/combat/combat-client-types';
import {
  REACTION_NAMES,
  type ResultView,
  type SlotAfter,
  rebukeRows,
  slotChoices,
} from '../../../../core/combat/reactions';
import { metersText } from '../../../../core/units';
import type { ReactionSheetData } from './reaction-sheet-data';

// What the reaction sheet shows and sends, worked out from the window: pure, so the component only holds the state.

/** What the sheet shows: the question, the die, the answer, or that the question is gone. */
export type Stage = 'ask' | 'roll' | 'result' | 'gone';

/** The pseudo level of the Infernal Legacy's row, which has no slot. */
export const RACIAL_LEVEL = 0;

/** Counterspell is cast with a slot of the 3rd level or above. */
const COUNTERSPELL_MIN_LEVEL = 3;

/** The rows of the radio list: the slots (or, for Repreensão Infernal, the Infernal Legacy and the slots). */
export function slotRowsOf(
  w: ReactionWindow,
  usage: ReactionSheetData['usage'],
  pact: ReactionSheetData['pact'],
): readonly SlotRow[] {
  switch (w.prompt.case) {
    case 'hellishRebuke':
      return rebukeRows(w).map((r) => ({
        level: r.racial ? RACIAL_LEVEL : r.level,
        pact: r.pact,
        free: r.free,
        total: null,
        used: 0,
        enabled: r.enabled,
        title: r.title,
        count: r.detail,
      }));
    case 'shield':
    case 'featherFall':
      return slotRows(1, slotChoices(w), usage, pact);
    case 'counterspell':
      return slotRows(COUNTERSPELL_MIN_LEVEL, slotChoices(w), usage, pact);
    default:
      return [];
  }
}

/** The maneuvers a reduction offers, when the window is a maneuver reduction. */
export function maneuverOptionsOf(w: ReactionWindow) {
  const p = w.prompt;
  return p.case === 'maneuverReduce' ? p.value.options : [];
}

/** The maneuver picked at first: the first one. */
export function firstManeuverKey(w: ReactionWindow): string {
  return maneuverOptionsOf(w)[0]?.key ?? '';
}

/** Who falls, with "aliado" and how far, for Queda Suave. */
export function fallingRowsOf(w: ReactionWindow) {
  const p = w.prompt;
  return p.case === 'featherFall'
    ? p.value.falling.map((f) => ({
        id: f.combatantId,
        label: f.label,
        detail: [
          ...(f.ally ? ['aliado'] : []),
          ...(f.distanceFt !== undefined ? [`a ${metersText(f.distanceFt)}`] : []),
        ].join(' · '),
      }))
    : [];
}

/** How many creatures Queda Suave can save. */
export function maxTargetsOf(w: ReactionWindow): number {
  const p = w.prompt;
  return p.case === 'featherFall' ? Math.max(1, p.value.maxTargets) : 1;
}

/** Queda Suave starts with the allies that fall chosen (as many as it can save). */
export function initialPicked(w: ReactionWindow): ReadonlySet<string> {
  const p = w.prompt;
  if (p.case !== 'featherFall') {
    return new Set();
  }
  return new Set(
    p.value.falling
      .filter((f) => f.ally)
      .slice(0, maxTargetsOf(w))
      .map((f) => f.combatantId),
  );
}

/** The card of the monk's second step, before the answer of the first one is known (after a reload). */
export function throwBackCard(round: number): ResultView {
  return {
    title: REACTION_NAMES.deflectMissiles,
    subtitle: `Rodada ${round}`,
    icon: 'swap_calls',
    pill: null,
    text: [],
    chips: [],
    note: '',
    throwBack: true,
  };
}

/** What the question collects: the slot or the Infernal Legacy, the creatures, the maneuver, the die. */
export interface Collected {
  readonly asked: ReactionWindow;
  readonly row: SlotRow | null;
  readonly creatureIds: readonly string[] | null;
  readonly maneuverKey: string;
}

/** The answer the question collects. */
export function answerOf(use: boolean, c: Collected, die?: ReactionAnswer['die']): ReactionAnswer {
  if (!use) {
    return { use: false };
  }
  const hellish = c.asked.prompt.case === 'hellishRebuke';
  const racial = hellish && c.row?.level === RACIAL_LEVEL;
  return {
    use: true,
    ...(c.row && !racial ? { slot: { level: c.row.level, pact: c.row.pact } } : {}),
    ...(racial ? { useRacial: true } : {}),
    ...(c.creatureIds ? { creatureIds: [...c.creatureIds] } : {}),
    ...(c.maneuverKey ? { maneuverKey: c.maneuverKey } : {}),
    ...(die ? { die } : {}),
  };
}

/** The slot the answer spent, for "Espaços de 3º nível: 1 livre de 2". */
export function slotAfter(answer: ReactionAnswer, row: SlotRow | null): SlotAfter | null {
  if (!answer.slot || !row) {
    return null;
  }
  return { level: row.level, free: Math.max(0, row.free - 1), total: row.total };
}

/** The slot an arrow picks from the buttons: the next enabled one, the list wraps; `null` when there is nowhere to go. */
export function nextSlot(
  rows: readonly SlotRow[],
  chosen: SlotRow | null,
  forward: boolean,
): SlotRow | null {
  const enabled = rows.filter((r) => r.enabled);
  const at = enabled.findIndex((r) => r.level === chosen?.level && r.pact === chosen?.pact);
  if (enabled.length < 2 || at < 0) {
    return null;
  }
  return enabled[(at + (forward ? 1 : enabled.length - 1)) % enabled.length];
}

/** What Esc does at each stage: passes, goes back, keeps the missile or closes. */
export type EscapeAction = 'pass' | 'back' | 'keep' | 'close' | 'none';

export function escapeAction(
  stage: Stage,
  isConcentration: boolean,
  throwBack: boolean,
): EscapeAction {
  switch (stage) {
    case 'ask':
      // A concentration save is decided by its owner, never by Esc.
      return isConcentration ? 'none' : 'pass';
    case 'roll':
      return 'back';
    case 'result':
      return throwBack ? 'keep' : 'close';
    default:
      return 'close';
  }
}
