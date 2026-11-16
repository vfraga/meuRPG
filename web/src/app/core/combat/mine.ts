import {
  type Combatant,
  CombatantState,
  EncounterStatus,
  type Encounter,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { groupName, isCreature } from './creature-names';
import { acts, actingIds, turnMembers } from './joint-turn';

/**
 * Everything a player moves in a combat (MR-037, E9-12): their own character and their creatures
 * (a familiar, summoned animals). The server says which are theirs (`controlled_by_me`); this
 * only gathers them into the tabs of the turn bar, one for the character and one for each
 * casting's creatures (a joint turn: they roll one initiative and act together) or for a creature
 * alone. Pure functions, tested without a DOM.
 */

/** The tab of the character itself. */
export const CHARACTER_TAB = 'character';

/** Where a tab stands in the order: its turn now, already played this round, or still to come. */
export type TabState = 'turn' | 'done' | 'wait';

export interface MineTab {
  /** `CHARACTER_TAB`, or the id of the creatures' casting (their `summon_group_id`) or of the creature alone. */
  readonly id: string;
  /** The combatants behind it: the character, or the creatures of one group, in the order. */
  readonly members: readonly Combatant[];
  readonly creature: boolean;
  /** "Sálvia", "Lobos atrozes (2)", "Nanquim". */
  readonly label: string;
  readonly state: TabState;
  /** The initiative total, when the server sent it. */
  readonly initiative: number | undefined;
  /** The members that may act now. */
  readonly acting: readonly Combatant[];
}

/** The combatants the caller plays: the character and its creatures. Always empty for the master. */
export function controlled(e: Encounter): Combatant[] {
  return e.combatants.filter((c) => c.controlledByMe || c.mine);
}

/** The creatures the caller plays. */
export function ownCreatures(e: Encounter): Combatant[] {
  return e.combatants.filter((c) => isCreature(c) && c.controlledByMe);
}

/** "Lobos atrozes (2)" for a group of several, the creature's own name for one. */
function tabLabel(members: readonly Combatant[]): string {
  return members.length > 1 ? `${groupName(members)} (${members.length})` : members[0].label;
}

/** The tabs of what the player plays: the character first, then each group in the order. The turn bar shows them only when there is more than one. */
export function mineTabs(e: Encounter): MineTab[] {
  const all = controlled(e);
  if (all.length === 0) {
    return [];
  }
  const acting = new Set(actingIds(e));
  const turnAt = Math.min(
    ...turnMembers(e)
      .map((m) => e.combatants.indexOf(m))
      .filter((i) => i >= 0),
  );
  const groups = new Map<string, Combatant[]>();
  for (const c of all) {
    const key = c.mine ? CHARACTER_TAB : c.summonGroupId || c.id;
    groups.set(key, [...(groups.get(key) ?? []), c]);
  }
  const tabs = [...groups].map(([id, members]) => {
    const live = members.filter((m) => acting.has(m.id));
    const at = Math.min(...members.map((m) => e.combatants.indexOf(m)));
    const state: TabState =
      live.length > 0
        ? 'turn'
        : Number.isFinite(turnAt) && at < turnAt
          ? 'done'
          : turnMembers(e).some((m) => members.includes(m))
            ? 'done'
            : 'wait';
    return {
      id,
      members,
      creature: id !== CHARACTER_TAB,
      label: id === CHARACTER_TAB ? members[0].label : tabLabel(members),
      state,
      initiative: members[0].initiative,
      acting: live,
    } satisfies MineTab;
  });
  // The character first, then the creatures in the order they act.
  return tabs.sort((a, b) => (a.id === CHARACTER_TAB ? -1 : b.id === CHARACTER_TAB ? 1 : 0));
}

/**
 * The tabs beside the pair card (588-04): the mount the character directs has no tab of its own while the card stands in for
 * it, so a player's own steed (Find Steed) does not show its three actions twice. Only the steed leaves a group it shares.
 */
export function tabsBesideRide(tabs: readonly MineTab[], ride: Combatant | null): MineTab[] {
  if (!ride) {
    return [...tabs];
  }
  return tabs.flatMap((tab) => {
    if (!tab.creature || !tab.members.some((m) => m.id === ride.id)) {
      return [tab];
    }
    const members = tab.members.filter((m) => m.id !== ride.id);
    const acting = tab.acting.filter((m) => m.id !== ride.id);
    return members.length ? [{ ...tab, members, acting, label: tabLabel(members) }] : [];
  });
}

/** The tab's word: "Sua vez · 10", "Já agiu · 13", "Espera · vez 10"; without the number when `compact` (a 320 px phone). */
export function tabWord(tab: MineTab, compact = false): string {
  const n = tab.initiative;
  switch (tab.state) {
    case 'turn':
      return compact || n === undefined ? 'Sua vez' : `Sua vez · ${n}`;
    case 'done':
      return compact || n === undefined ? 'Já agiu' : `Já agiu · ${n}`;
    default:
      return compact || n === undefined ? 'Espera' : `Espera · vez ${n}`;
  }
}

/** The tab whose turn it is, or `''`: the page follows it when it changes. */
export function actingTab(tabs: readonly MineTab[]): string {
  return tabs.find((t) => t.state === 'turn')?.id ?? '';
}

/** The tab the page may keep showing: the chosen one while it exists, else the character's (the creatures are gone, the tabs with them). */
export function validTab(tabs: readonly MineTab[], chosen: string): string {
  return tabs.length === 0 || tabs.some((t) => t.id === chosen) ? chosen : CHARACTER_TAB;
}

/** The members of a tab whose part still has to be ended now, read from the combat as it is (one call each): "Encerrar a parte dos Lobos" ends each one that acts. */
export function membersToEnd(e: Encounter, tab: MineTab): Combatant[] {
  return tab.members.filter((member) => {
    const now = e.combatants.find((c) => c.id === member.id);
    return !!now && acts(e, now);
  });
}

/** The ids of the combatants that are dying (0 hit points, making death saves). */
export function dyingIds(e: Encounter | null | undefined): Set<string> {
  return new Set(
    (e?.combatants ?? []).filter((c) => c.state === CombatantState.DYING).map((c) => c.id),
  );
}

/** The combatants whose turn options the player's page reads besides the character's: its creatures, and the mount the
 * character directs on this turn (its three actions are on the pair card, 588-04). */
export function optionIds(e: Encounter | null, master: boolean, ride: Combatant | null): string[] {
  if (!e || master || e.status !== EncounterStatus.ACTIVE) {
    return [];
  }
  const ids = ownCreatures(e).map((c) => c.id);
  return ride && !ids.includes(ride.id) ? [...ids, ride.id] : ids;
}
