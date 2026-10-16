import type { CreatureSummary } from '../../../gen/meurpg/rules/v1/rules_pb';
import type { JoinSpec } from '../../core/combat/combat-client-types';
import type { RosterEntry } from '../../core/maps/roster-client';
import type { PickedMonster, SavedMonsters } from './start-combat.types';

/** The most one add of a creature takes (the combat holds 40 in all). */
export const PICK_MAX = 10;

/** The server's limit (combat.proto): 40 combatants in a combat. */
const MAX_COMBATANTS = 40;

/** What decides whether "Iniciar combate" / "Adicionar" can go. */
export interface BlockInput {
  readonly ready: boolean;
  readonly adding: boolean;
  readonly theatre: boolean;
  readonly hasMap: boolean;
  readonly hasGrid: boolean;
  readonly noGridMap: boolean;
  readonly nameOk: boolean;
  /** How many of the party are checked. */
  readonly included: number;
  readonly npcTotal: number;
  readonly monsterTotal: number;
  /** How many kinds of creature were picked from the bestiary. */
  readonly picked: number;
  readonly total: number;
}

/** The map part of the reasons: a combat on a map needs the session's map, with a grid. */
function mapBlock(i: BlockInput): string | null {
  if (i.theatre) {
    return null;
  }
  if (!i.adding && !i.hasMap) {
    return 'Escolha o mapa atual da sessão primeiro.';
  }
  return !i.hasGrid || i.noGridMap ? 'O mapa precisa de uma grade.' : null;
}

/** Who enters: the party (or a monster) to start, the NPCs (or a monster) to add. */
function entrantsBlock(i: BlockInput): string | null {
  if (i.adding) {
    return i.npcTotal === 0 && i.picked === 0 ? 'Escolha quantos NPCs entram.' : null;
  }
  if (!i.nameOk) {
    return 'Dê um nome ao combate.';
  }
  return i.included === 0 && i.monsterTotal === 0 ? 'Escolha quem do grupo entra.' : null;
}

/** Why the main button waits, or `null` when it can go. */
export function blockedReason(i: BlockInput): string | null {
  if (!i.ready) {
    return 'Carregando quem pode lutar.';
  }
  const reason = mapBlock(i) ?? entrantsBlock(i);
  if (reason) {
    return reason;
  }
  return i.total > MAX_COMBATANTS
    ? `Um combate tem no máximo ${MAX_COMBATANTS} combatentes.`
    : null;
}

/** The creature picked once more (up to {@link PICK_MAX}), or listed with one. */
export function withPicked(
  list: readonly PickedMonster[],
  c: CreatureSummary,
): readonly PickedMonster[] {
  return list.some((m) => m.key === c.key)
    ? list.map((m) => (m.key === c.key ? { ...m, count: Math.min(PICK_MAX, m.count + 1) } : m))
    : [...list, { key: c.key, namePt: c.namePt, count: 1 }];
}

/** "Goblin 1, 2 e 3". */
export function copiesText(name: string, count: number): string {
  const numbers = Array.from({ length: count }, (_, i) => i + 1);
  const last = numbers[numbers.length - 1];
  return `${name} ${numbers.length > 1 ? numbers.slice(0, -1).join(', ') + ' e ' + last : numbers[0]}`;
}

/** The monsters a start sends: the saved encounter's groups, then the ones picked here. */
export function startMonsters(saved: readonly SavedMonsters[], picked: readonly PickedMonster[]) {
  return [
    ...saved.map((g) => ({ creatureKey: g.key, count: g.count, name: g.name })),
    ...picked.map((m) => ({ creatureKey: m.key, count: m.count, name: '' })),
  ];
}

export interface JoinInput {
  readonly adding: boolean;
  readonly players: readonly RosterEntry[];
  readonly included: ReadonlySet<string>;
  readonly npcs: readonly RosterEntry[];
  readonly counts: ReadonlyMap<string, number>;
  readonly hiddenAt: ReadonlyMap<string, boolean>;
}

/** Who joins: the checked party members (not when adding), then each NPC with a count; an NPC starts hidden unless said otherwise. */
export function joinSpecs(i: JoinInput): JoinSpec[] {
  const party = i.adding
    ? []
    : i.players
        .filter((p) => i.included.has(p.id) && !p.reserved)
        .map((p) => ({ characterId: p.id }));
  const npcs = i.npcs
    .filter((n) => (i.counts.get(n.id) ?? 0) > 0)
    .map((n) => ({
      characterId: n.id,
      count: i.counts.get(n.id),
      hidden: i.hiddenAt.get(n.id) ?? true,
    }));
  return [...party, ...npcs];
}
