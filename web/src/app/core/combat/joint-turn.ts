import {
  type Combatant,
  CombatantKind,
  type Encounter,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { boxedPair } from './mount-view';
import { article } from './combat-log';
import { groupName, ofOwner, pluralName } from './creature-names';
import { distanceText } from '../units';

/**
 * Joint turns (MR-013, RN-19, RN-20): combatants adjacent in the order with
 * the same initiative total take one turn together. The server decides who is
 * in the turn that is running (`Encounter.turn_group_ids`, with each member's
 * `turn_part_ended`) and sends each viewer only what they may see; these
 * functions only read that, and group the order list for the boxes. Pure
 * TypeScript, tested without a DOM.
 */

/** The turn that is running, as the viewer is told about it. */
export interface JointTurn {
  /** The members the viewer is sent, in the order. */
  readonly members: readonly Combatant[];
  /** Who still acts. */
  readonly acting: readonly Combatant[];
  /** Whose part ended. */
  readonly ended: readonly Combatant[];
  /** NPCs alone: the master's group, which a player only hears named ("Vez
   * dos Goblins"), with no flags and no box. */
  readonly npcOnly: boolean;
  /** Everyone who still acts is hidden from the viewer: the turn waits for
   * the master's part ("Falta o mestre"). */
  readonly waitsForMaster: boolean;
}

/** The combatants of the turn that is running, in the order. Reads
 * `turn_group_ids`, and, for a copy that has none (an older server), the one
 * named in `current_combatant_id`. */
export function turnMembers(e: Encounter): Combatant[] {
  if (e.status !== EncounterStatus.ACTIVE) {
    return [];
  }
  const ids =
    e.turnGroupIds.length > 0 ? e.turnGroupIds : e.currentCombatantId ? [e.currentCombatantId] : [];
  return ids.flatMap((id) => e.combatants.find((c) => c.id === id) ?? []);
}

/** The ids of the combatants that may act now: in the turn, part not ended. */
export function actingIds(e: Encounter): string[] {
  return turnMembers(e)
    .filter((c) => !c.turnPartEnded)
    .map((c) => c.id);
}

/** Whether the combatant may act now. */
export function acts(e: Encounter, c: Combatant): boolean {
  return actingIds(e).includes(c.id);
}

/**
 * Whom the master's action card is for: the first NPC of the turn that still
 * acts, so a joint turn that also holds a player (who is `current_combatant_id`,
 * the first of the group) still gives the master the NPC's attacks. `null`
 * when every member still acting is a player (or nobody is): the caller then
 * uses the current combatant.
 */
export function masterActor(e: Encounter): Combatant | null {
  const npcs = turnMembers(e).filter((c) => c.kind !== CombatantKind.PLAYER);
  return npcs.find((c) => !c.turnPartEnded && !c.defeated) ?? null;
}

/** The joint turn, or `null` for a turn of one (everything works as it
 * always did). A group the viewer sees one member of, whose part ended while
 * the master's part is missing, is still a joint turn: it says who waits. */
export function jointTurn(e: Encounter): JointTurn | null {
  const members = turnMembers(e);
  const waitsForMaster = e.masterTurn && members.length > 0;
  if (members.length < 2 && !(members.length === 1 && members[0].turnPartEnded && waitsForMaster)) {
    return null;
  }
  const npcOnly = members.every((c) => c.kind === CombatantKind.NPC);
  return {
    members,
    acting: members.filter((c) => !c.turnPartEnded),
    ended: members.filter((c) => c.turnPartEnded),
    npcOnly,
    waitsForMaster,
  };
}

/** A name with its article: "a Brisa", "o Toren". */
export function withArticle(label: string): string {
  return `${article(label)} ${label}`;
}

/** "Brisa e Toren", "Brisa, Toren e Pensantus". */
export function listNames(labels: readonly string[]): string {
  return labels.length <= 1
    ? labels.join('')
    : `${labels.slice(0, -1).join(', ')} e ${labels[labels.length - 1]}`;
}

/** What a player hears for a group of NPCs that are copies of one: "os
 * Goblins". `null` when they are not copies of one NPC (the app then lists the
 * names): never one member picked. */
export function npcPlural(labels: readonly string[]): string | null {
  const bases = labels.map((l) => l.replace(/\s+\d+$/, ''));
  if (labels.length < 2 || !bases.every((b) => b === bases[0])) {
    return null;
  }
  return `${article(bases[0]) === 'a' ? 'as' : 'os'} ${pluralName(bases[0])}`;
}

/** One entry of the order list: a combatant alone, or a box of a joint turn. */
export type OrderItem =
  | { readonly kind: 'single'; readonly combatant: Combatant }
  | {
      readonly kind: 'group';
      /** The initiative total they share. */
      readonly total: number;
      readonly members: readonly Combatant[];
      /** It is the group whose turn is running. */
      readonly onTurn: boolean;
      /** The box is a rider and its mount ("Montado"), not a turn that was shared by chance. */
      readonly mounted: boolean;
    };

/**
 * The order list with its boxes. The master has every total, so every run of
 * two or more adjacent combatants with the same total is a box. A player has
 * only the totals of the players, so a box is a run of players with the same
 * total (an NPC-only group never shows a box: RN-20), plus the NPCs the
 * server names in the group on turn when it holds a player's character.
 */
export function orderItems(e: Encounter, master: boolean): OrderItem[] {
  const list = e.combatants;
  const onTurn = turnMembers(e);
  const onTurnHoldsPlayer =
    onTurn.length > 1 && onTurn.some((c) => c.kind === CombatantKind.PLAYER);
  const inTurnGroup = new Set(onTurnHoldsPlayer ? onTurn.map((c) => c.id) : []);
  const out: OrderItem[] = [];
  let run: Combatant[] = [];
  const flush = () => {
    if (run.length > 1) {
      const total = run.find((c) => c.initiative !== undefined)?.initiative ?? 0;
      out.push({
        kind: 'group',
        total,
        members: run,
        onTurn: onTurn.length > 1 && run.some((c) => onTurn.some((t) => t.id === c.id)),
        mounted: run.length === 2 && boxedPair(run[0], run[1], master),
      });
    } else {
      out.push(...run.map((combatant) => ({ kind: 'single' as const, combatant })));
    }
    run = [];
  };
  for (const c of list) {
    const joins = run.length > 0 && sameGroup(run[0], c, master, inTurnGroup);
    if (!joins) {
      flush();
    }
    run.push(c);
  }
  flush();
  return out;
}

function sameGroup(
  a: Combatant,
  b: Combatant,
  master: boolean,
  inTurnGroup: ReadonlySet<string>,
): boolean {
  if (inTurnGroup.has(a.id) && inTurnGroup.has(b.id)) {
    return true;
  }
  if (inTurnGroup.has(a.id) || inTurnGroup.has(b.id)) {
    return false;
  }
  // A rider and its mount are one box whatever the totals the viewer reads: the mount takes its rider's.
  if (boxedPair(a, b, master)) {
    return true;
  }
  if (a.initiative === undefined || b.initiative === undefined || a.initiative !== b.initiative) {
    return false;
  }
  // A player never groups NPCs: they have no total for the player anyway. The totals of characters
  // and of their creatures are public (RN-20), so those group.
  return master || (a.kind !== CombatantKind.NPC && b.kind !== CombatantKind.NPC);
}

/** The ids of the NPC-only groups a player is told about (to name them). */
export function npcGroupOf(e: Encounter, id: string): readonly Combatant[] | null {
  const group = e.npcOnlyGroups.find((g) => g.combatantIds.includes(id));
  return group
    ? group.combatantIds.flatMap((x) => e.combatants.find((c) => c.id === x) ?? [])
    : null;
}

/** Who plays after the turn that is running: the first combatant after the
 * group's last member that is not defeated, going round the order, as a name
 * (a group of NPCs a player was told about is "os Goblins", never one of
 * them), and whether it is the caller's own. `null` when nobody follows. */
export function afterTurn(
  e: Encounter,
): { readonly name: string; readonly mine: boolean; readonly plural: boolean } | null {
  const list = e.combatants;
  const members = turnMembers(e);
  if (members.length === 0) {
    return null;
  }
  const lastAt = Math.max(...members.map((m) => list.findIndex((c) => c.id === m.id)));
  const inGroup = new Set(members.map((m) => m.id));
  for (let step = 1; step <= list.length; step++) {
    const c = list[(lastAt + step) % list.length];
    if (c.defeated || inGroup.has(c.id)) {
      continue;
    }
    if (c.kind === CombatantKind.CREATURE) {
      // A player's creatures: "os Lobos atrozes da Sálvia" for a group that shares a total, the creature's name alone.
      const run = creatureRun(list, list.indexOf(c));
      const owner = ofOwner(e, c);
      const name = run.length > 1 ? `${groupName(run)}${owner ? ` ${owner}` : ''}` : c.label;
      return { name, mine: run.some((m) => m.controlledByMe), plural: run.length > 1 };
    }
    const npcGroup = npcGroupOf(e, c.id) ?? masterGroupFrom(list, list.indexOf(c));
    if (npcGroup) {
      const labels = npcGroup.map((m) => m.label);
      return { name: npcPlural(labels) ?? listNames(labels), mine: false, plural: true };
    }
    return { name: c.label, mine: c.mine, plural: false };
  }
  return null;
}

/** The creatures from `at` on that share the first one's total and owner (one casting's, in a joint turn), without the defeated. */
function creatureRun(list: readonly Combatant[], at: number): readonly Combatant[] {
  const first = list[at];
  const run: Combatant[] = [];
  for (let i = at; i < list.length; i++) {
    const c = list[i];
    if (
      c.kind !== CombatantKind.CREATURE ||
      c.initiative !== first.initiative ||
      c.ownerCharacterId !== first.ownerCharacterId
    ) {
      break;
    }
    if (!c.defeated) {
      run.push(c);
    }
  }
  return run.length > 0 ? run : [first];
}

/**
 * The master's copy has no `npc_only_groups`: the master reads the groups
 * from the totals, as the order list does. The run of combatants from `at`
 * on with the same total, when it holds an NPC (only the master has an NPC's
 * total, so a player never gets one here), without the defeated; `null` for a
 * turn of one.
 */
function masterGroupFrom(list: readonly Combatant[], at: number): readonly Combatant[] | null {
  const first = list[at];
  if (first.initiative === undefined) {
    return null;
  }
  const run: Combatant[] = [];
  for (let i = at; i < list.length && list[i].initiative === first.initiative; i++) {
    run.push(list[i]);
  }
  const living = run.filter((c) => !c.defeated);
  return living.length > 1 && living.some((c) => c.kind !== CombatantKind.PLAYER) ? living : null;
}

/** "Falta a Brisa. O turno passa quando ela encerrar a parte dela.", or the
 * plural, or "Falta o mestre...". `masterToo` adds the master's hidden part. */
export function missingLine(labels: readonly string[], masterToo: boolean): string {
  const who = [...labels.map(withArticle), ...(masterToo ? ['o mestre'] : [])];
  if (who.length === 0) {
    return '';
  }
  if (who.length === 1) {
    const feminine = who[0].startsWith('a ');
    const [he, his] = feminine ? ['ela', 'dela'] : ['ele', 'dele'];
    return `Falta ${who[0]}. O turno passa quando ${he} encerrar a parte ${his}.`;
  }
  const list = `${who.slice(0, -1).join(', ')} e ${who[who.length - 1]}`;
  return `Faltam ${list}. O turno passa quando todos encerrarem a parte deles.`;
}

/** What a member's part still has and what it spent, as the words the cards
 * list: "Ação", "Ação bônus", "Reação" and the movement left. */
export interface PartEconomy {
  /** "Ação", "Ação bônus", "Reação": still available. */
  readonly left: readonly string[];
  /** The movement left, "7,5 m · 5 quadrados", or `''` when there is none. */
  readonly movement: string;
  /** What it spent. */
  readonly used: readonly string[];
}

export function partEconomy(c: Combatant): PartEconomy {
  const left: string[] = [];
  const used: string[] = [];
  for (const [name, spent] of [
    ['Ação', c.actionUsed],
    ['Ação bônus', c.bonusActionUsed],
    ['Reação', c.reactionUsed],
  ] as const) {
    (spent ? used : left).push(name);
  }
  return { left, movement: c.movementLeftFt > 0 ? distanceText(c.movementLeftFt) : '', used };
}

/** "Ação, Ação bônus, Reação e 9 m · 6 quadrados": what the question "Encerrar
 * a sua parte?" says is still left. Empty when nothing is. */
export function leftSentence(c: Combatant): string {
  const e = partEconomy(c);
  const items = [...e.left, ...(e.movement ? [e.movement] : [])];
  return items.length <= 1
    ? items.join('')
    : `${items.slice(0, -1).join(', ')} e ${items[items.length - 1]}`;
}

/** Who plays right before the caller's own combatant, as "do Capitão Goblin",
 * "dos Goblins" (a group of NPCs a player was told about) or "da Brisa": the
 * nearest combatant before it in the order that is not defeated, going round.
 * `''` when the caller has no combatant or nobody else is in the order. */
export function playsBefore(e: Encounter): string {
  const list = e.combatants;
  const at = list.findIndex((c) => c.mine);
  if (at < 0) {
    return '';
  }
  for (let step = 1; step < list.length; step++) {
    const c = list[(at - step + list.length * 2) % list.length];
    if (c.defeated) {
      continue;
    }
    const group = npcGroupOf(e, c.id);
    const named = group ? npcPlural(group.map((m) => m.label)) : null;
    if (named) {
      return `d${named}`;
    }
    return `${article(c.label) === 'a' ? 'da' : 'do'} ${c.label}`;
  }
  return '';
}

/** "O turno passa quando você e a Brisa encerrarem.": the others who still act,
 * by name with their article ("o mestre" stands for a part the player is not
 * told the name of). */
export function passNote(others: readonly string[], masterToo: boolean): string {
  const who = ['você', ...others.map(withArticle), ...(masterToo ? ['o mestre'] : [])];
  return `O turno passa quando ${listNames(who)} encerrar${who.length > 1 ? 'em' : ''}.`;
}

/** "Turno conjunto: Brisa e Toren, iniciativa 19" or "Montado: Tor e Cavalo de guerra, iniciativa 17", for a screen reader. */
export function groupAriaLabel(item: OrderItem): string {
  return item.kind === 'group'
    ? `${item.mounted ? 'Montado' : 'Turno conjunto'}: ${listNames(item.members.map((m) => m.label))}, iniciativa ${item.total}`
    : '';
}

/** The mount whose part ends before its rider's when the rider ends the turn, or `null` once it no longer acts. Its part goes
 * first: with the rider's part ended, the mount's would no longer be the player's to end. */
export function rideToEnd(e: Encounter | null, ride: Combatant | null): Combatant | null {
  const mount = ride ? e?.combatants.find((c) => c.id === ride.id) : undefined;
  return e && mount && acts(e, mount) ? mount : null;
}

/** Whose part "Encerrar turno" ends: the caller's own while it acts (the master's, for the character he acts for), else the
 * combatant on turn. */
export function partToEnd(e: Encounter, own: Combatant | null): string {
  return own && acts(e, own) ? own.id : e.currentCombatantId;
}
