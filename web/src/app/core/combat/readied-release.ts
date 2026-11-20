import {
  AreaPlacement,
  type GetTurnOptionsResponse,
  type ReadiedAction,
  ReadiedKind,
  type SpellTargets,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  type CastTargetRow,
  type Dealt,
  type TargetRule,
  castTargetRows,
  dartTargets,
  dartsAt,
  dartsPlaced,
  dartsStatus,
  targetRule,
  toggled,
} from './cast-flow';
import type { CastTarget, ReactionAnswer } from './combat-client-types';
import { laysWall } from './spell-wall';

/**
 * How a readied spell is released from its window (#857): the same pieces as the cast screens, fed by the targets the server lists
 * for the held spell (`GetTurnOptions.spell_targets` while the row waits). Darts are split, creatures are picked, and a spell that
 * stays on its caster or has no map to aim at asks nothing. A spell placed on the map (a point, a direction, around the caster) is
 * released from the cast sheet's map, which the window opens (`onMap`). Pure functions, tested without a DOM.
 */

/** What the window asks for the held spell: the rule, the rows and the darts to share out. */
export interface ReleasePlan {
  readonly rule: TargetRule;
  readonly rows: readonly CastTargetRow[];
  /** The darts (or rays) to hand out; 0 for a spell that is not shared out. */
  readonly total: number;
  /** How the spell is placed on the map; `UNSPECIFIED` for a spell with no area. */
  readonly placement: AreaPlacement;
  /** The spell is placed on the combat's map: it is released with `CastSpell` from the cast sheet's map, not from a list. */
  readonly onMap: boolean;
  /** The spell lays a wall (Muralha de Fogo). */
  readonly wall: boolean;
}

/** What was picked so far: the creatures ticked, or the darts dealt. */
export interface ReleasePick {
  readonly chosen: readonly string[];
  readonly dealt: Dealt;
}

export const NO_PICK: ReleasePick = { chosen: [], dealt: new Map() };

/**
 * The entry the server lists for the spell a row holds. `SpellTargets` carries nothing that tells a held entry from an ordinary one, so
 * the match is on the key alone, and it is exact because of what the server lists: a held entry only off the readier's turn, where no
 * ordinary entry is listed (those need the readier's turn), so one spell key holds one entry there.
 */
export function heldTargetsOf(
  res: GetTurnOptionsResponse,
  r: ReadiedAction,
): SpellTargets | undefined {
  return res.spellTargets.find((x) => x.spellKey === r.actionKey);
}

/**
 * The plan of a held spell: `null` when the server lists no targets for it, and `none` rule when it stays on its caster. The held
 * entry carries the slot it was held with, so the rule is read at that slot.
 */
export function releasePlan(
  res: GetTurnOptionsResponse,
  r: ReadiedAction,
  reactorId: string,
  mapOn = true,
): ReleasePlan | null {
  const st = heldTargetsOf(res, r);
  if (!st) {
    return null;
  }
  const slot = st.darts[0]?.slotLevel ?? 0;
  const rule = targetRule(st, reactorId, slot, slot);
  // The darts of the held slot, all of them: `rule.max` is the most creatures a list holds (10), which Magic Missile at the 9th level (11 darts) passes.
  const total = rule.kind === 'darts' ? dartsAt(st.darts, slot) : 0;
  const reach = st.rangeFt > 0 ? st.rangeFt : null;
  return {
    rule,
    rows: castTargetRows(st.targets, reactorId, reach),
    total,
    placement: st.placement,
    onMap: mapOn && st.placement !== AreaPlacement.UNSPECIFIED,
    wall: laysWall(r.actionKey),
  };
}

const MAP_TAIL = 'Lá a mesa espera até 2 min, nada sai antes de “Conjurar” e dá para voltar.';

/** The lead of what a spell placed on the map asks, by how it is placed. */
function placedLead(plan: ReleasePlan): string {
  if (plan.wall) {
    return 'Ela pede onde a muralha fica no mapa.';
  }
  switch (plan.placement) {
    case AreaPlacement.DIRECTION:
      return 'Ela pede uma direção no mapa.';
    case AreaPlacement.CASTER:
      return 'Ela age ao redor de você: o mapa mostra quem está na área.';
    default:
      return 'Ela pede um ponto no mapa.';
  }
}

/**
 * What the window says about the map (boards 857-01 and 857-05): a bold lead and the rest, for a spell that is placed and for a move;
 * `null` for anything the window answers itself. The master reads the same words for the player.
 */
export function releaseMapNotice(
  plan: ReleasePlan | null,
  kind: ReadiedKind,
): { readonly lead: string; readonly rest: string } | null {
  if (kind === ReadiedKind.MOVE) {
    return {
      lead: 'O mapa abre para você escolher o quadrado.',
      rest: 'Lá a mesa espera até 2 min, e dá para voltar antes de mover.',
    };
  }
  return kind === ReadiedKind.SPELL && plan?.onMap
    ? { lead: placedLead(plan), rest: MAP_TAIL }
    : null;
}

/** Whether the pick is enough to release: all the darts dealt, one creature, or at least the area's least. */
export function releaseComplete(plan: ReleasePlan, pick: ReleasePick): boolean {
  if (plan.onMap) {
    return true;
  }
  switch (plan.rule.kind) {
    case 'darts':
      return plan.total > 0 && dartsPlaced(pick.dealt) === plan.total;
    case 'none':
      return true;
    default:
      return pick.chosen.length >= plan.rule.min;
  }
}

/** The line under the list: the darts still to deal; nothing for the other rules. */
export function releaseStatus(plan: ReleasePlan, pick: ReleasePick): string {
  return plan.rule.kind === 'darts'
    ? dartsStatus(plan.total, pick.dealt, plan.rule.rays === true)
    : '';
}

/** The pick with a creature ticked or unticked. */
export function releaseToggled(plan: ReleasePlan, pick: ReleasePick, id: string): ReleasePick {
  return { ...pick, chosen: toggled(plan.rule, pick.chosen, id) };
}

/** The targets the answer sends: the darts dealt, or the creatures ticked; `null` while the pick is not enough. */
export function releaseTargets(plan: ReleasePlan, pick: ReleasePick): readonly CastTarget[] | null {
  if (!releaseComplete(plan, pick)) {
    return null;
  }
  return plan.rule.kind === 'darts'
    ? dartTargets(pick.dealt)
    : pick.chosen.map((combatantId) => ({ combatantId, darts: 0 }));
}

/**
 * The pick the list opens with: the creature named at Ready when the spell takes one (or several, at least one) and it can be aimed at
 * now. A spell that may meet nobody, and the darts, start empty so what is sent is what was ticked.
 */
export function releaseStart(plan: ReleasePlan, named: string): ReleasePick {
  const aimable = plan.rows.some((x) => x.id === named && !x.blocked);
  const takesOne = plan.rule.kind === 'single' || (plan.rule.kind === 'multi' && plan.rule.min > 0);
  return takesOne && aimable ? { chosen: [named], dealt: new Map() } : NO_PICK;
}

/**
 * The use of an attack or a spell of a window, with the d20 of the app: a held spell names its `targets` with their darts, an
 * attack (or a spell with no list) the one creature picked. `null` while a held spell's pick is not enough.
 */
export function aimedUse(
  plan: ReleasePlan | null,
  pick: ReleasePick,
  target: string | null,
): ReactionAnswer | null {
  if (!plan) {
    return { use: true, ...(target ? { creatureIds: [target] } : {}), die: { inApp: true } };
  }
  const targets = releaseTargets(plan, pick);
  return targets
    ? { use: true, ...(targets.length > 0 ? { targets } : {}), die: { inApp: true } }
    : null;
}
