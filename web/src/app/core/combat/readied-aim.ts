import {
  type Encounter,
  type GetTurnOptionsResponse,
  type ReadiedAction,
  ReadiedKind,
} from '../../../gen/meurpg/play/v1/combat_pb';
import type { Attack } from '../../../gen/meurpg/rules/v1/rules_pb';
import { stateWord } from './combat-view';
import { type ReadiedTargetRow, readiedLegend, readiedTargetRows } from './reaction-readied';

/**
 * Who a readied attack or spell is aimed at when its window is answered (#582): read from the readier's own turn options and the
 * combat, the same for the readier's prompt and the master's queue. The server decides what can be aimed at.
 */

/** What the creatures list asks of the one who answers: `loading` and `failed` are about reading it. */
export type AimNeed = 'loading' | 'required' | 'optional' | 'failed';

export interface ReadiedAim {
  readonly need: AimNeed;
  readonly rows: readonly ReadiedTargetRow[];
  readonly legend: string;
  /** The creature named at Ready when it can be aimed at now, else `null`. */
  readonly chosen: string | null;
}

/** What a melee attack reaches when it says nothing, in feet. */
const MELEE_REACH_FT = 5;

/** How far the attack may be aimed: its long range when it has one (a thrown weapon, a bow), else its melee reach. */
function attackReachFt(a: Attack): number {
  if (a.longRangeFt > 0) {
    return a.longRangeFt;
  }
  return a.melee ? Math.max(MELEE_REACH_FT, a.reachFt) : a.rangeFt;
}

/**
 * The creatures a spell that is not an attack may name: everyone in the combat. The readier is off its turn while the window is
 * open, and the server lists the targets of the spells it can cast now only, so it has no list (or darts, or placement) for a held spell.
 */
function spellRows(e: Encounter | null, reactorId: string, named: string): ReadiedTargetRow[] {
  return (e?.combatants ?? []).map((c) => ({
    id: c.id,
    title: c.label,
    sub: [
      c.id === reactorId ? 'Você mesmo' : '',
      c.id === named ? 'O alvo que você disse' : '',
      stateWord(c.state, c.label),
    ]
      .filter(Boolean)
      .join(' · '),
    blocked: '',
  }));
}

/** The aim of the window's row: an attack (or a cantrip released as one) needs a creature, any other spell may name one. */
export function readiedAim(
  res: GetTurnOptionsResponse,
  r: ReadiedAction,
  e: Encounter | null,
  reactorId: string,
): ReadiedAim {
  const named = r.targetCombatantId;
  // Only an attack that rolls to hit has a list of targets: a cantrip with a saving throw is released as a spell.
  const aimed = res.attackTargets.find((x) => x.attackKey === r.actionKey);
  if (r.kind === ReadiedKind.ATTACK || aimed) {
    const attack = res.options?.attacks.find((a) => a.attack?.key === r.actionKey)?.attack;
    const rows = readiedTargetRows(
      aimed?.targets ?? [],
      named,
      attack ? attackReachFt(attack) : MELEE_REACH_FT,
    );
    const mine = rows.find((x) => x.id === named && !x.blocked);
    return {
      need: 'required',
      rows,
      legend: readiedLegend(rows),
      chosen: mine ? named : null,
    };
  }
  const rows = spellRows(e, reactorId, named);
  const mine = rows.find((x) => x.id === named);
  return { need: 'optional', rows, legend: 'Em quem (opcional)', chosen: mine ? named : null };
}
