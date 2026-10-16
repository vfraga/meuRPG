import type { MonsterHp } from '../../core/combat/combat-client-types';

/** The map the fight is on (the session's current map). */
export interface CombatMapInfo {
  readonly id: string;
  readonly name: string;
  readonly image: { readonly url: string; readonly width: number; readonly height: number };
  /** 0 when the map has no grid. */
  readonly columns: number;
  readonly rows: number;
}

/** One kind of creature of a saved encounter, as the dialog lists it (read only: the encounter is the page's). */
export interface SavedMonsters {
  readonly key: string;
  readonly namePt: string;
  readonly count: number;
  /** The base name the point kept for the group (empty: the creature's Portuguese name). */
  readonly name?: string;
}

/** "Começar este combate" (MR-043): what the dialog starts with, from the encounter a battle point keeps. */
export interface SavedStart {
  /** The battle point it was kept on: the combat starts from it. */
  readonly pointId: string;
  readonly pointName: string;
  readonly groups: readonly SavedMonsters[];
  readonly hp: MonsterHp;
  readonly hidden: boolean;
}

export interface StartCombatData {
  readonly campaignId: string;
  /** `start`: "Iniciar combate"; `add`: reinforcements for a running combat. */
  readonly mode: 'start' | 'add';
  readonly map: CombatMapInfo | null;
  /** The running combat, for `add`. */
  readonly encounterId?: string;
  /** How many combatants it already has (the limit is 40 in all). */
  readonly existing?: number;
  /** "Começar este combate": the monsters of a saved encounter come in with the party. */
  readonly saved?: SavedStart;
}

/** One kind of creature picked from the bestiary inside the dialog. */
export interface PickedMonster {
  readonly key: string;
  readonly namePt: string;
  readonly count: number;
}
