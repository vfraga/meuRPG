import { expect, type Page } from '@playwright/test';

import { adjustVitalsRPC, beginAttackCombatRPC, tableForCombat, type CombatTable } from './combat-support';
import { callRPC, type CharacterBuild } from './support';

// Setup for the specs of the table die on a weapon attack (Golpe Certeiro, #643), through the API: a barbarian of level 3 whose
// subclass is the table's own, with a pool of dice ("Reserva de Golpes") and one or two `superiority_die` effects that apply
// to an attack. The specs prove the screens, not the table editor.

/** Brutus, a human Barbarian 3 with a greataxe, whose subclass the table replaces with the one of `maneuverTable`. */
export const brutus: CharacterBuild = {
  name: 'Brutus',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:barbarian',
  class: 'Bárbaro',
  subclassKey: 'subclass:berserker',
  subclass: 'Caminho do Berserker',
  level: 3,
  background: 'Soldado',
  backgroundSkillKeys: ['skill:athletics', 'skill:intimidation'],
  backgroundSkills: ['Atletismo', 'Intimidação'],
  extraSkillKeys: ['skill:perception', 'skill:survival'],
  extraSkills: ['Percepção', 'Sobrevivência'],
  scores: { for: 16, des: 13, con: 14, int: 10, sab: 10, car: 8 },
};

/** The pool the dice spend from; `max` is its uses (the same pool for both dice). */
const POOL = 'reserva_de_golpes';

export interface ManeuverTable extends CombatTable {
  /** The dice of the table's subclass: one (Golpe Certeiro d8) or two (and Golpe Firme d6). */
  dice: string[];
}

/**
 * A table ready for a combat (`tableForCombat`: the grid, the Capitão and two Goblins) in which Brutus has the table's die:
 * "Golpe Certeiro" (d8) and, with `dice` 2, "Golpe Firme" (d6) from the same pool of two uses. Master and player pages are signed in.
 */
export async function maneuverTable(master: Page, player: Page, name: string, dice: 1 | 2 = 1): Promise<ManeuverTable> {
  const table = await tableForCombat(master, player, name, true, true, { build: brutus, sheet: { weaponKeys: ['equipment:greataxe'] } });
  const features = [
    { namePt: 'Reserva de Golpes', descPt: ['Dados de golpe.'], effects: [{ type: 'resource', resource: POOL, max: '2', recharge: 'short_rest' }] },
    { namePt: 'Golpe Certeiro', descPt: ['Um dado a mais no ataque.'], effects: [{ type: 'superiority_die', applies: 'attack', resource: POOL, value: '8' }] },
    ...(dice === 2
      ? [{ namePt: 'Golpe Firme', descPt: ['Um dado menor, mais firme.'], effects: [{ type: 'superiority_die', applies: 'attack', resource: POOL, value: '6' }] }]
      : []),
  ];
  const made = await callRPC(master, 'meurpg.rules.v1.TableContentService/CreateTableEntry', {
    campaignId: table.campaignId,
    tableSubclass: { namePt: 'Caminho do Golpe', classKey: 'class:barbarian', descPt: ['Dados.'], levels: [{ level: 3, features }] },
  });
  expect(made.ok(), await made.text()).toBeTruthy();
  const subclassKey = (await made.json()).entry.key as string;
  // The master edits the locked sheet: the character gets the table's subclass.
  const got = await callRPC(master, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId: table.campaignId, characterId: table.characterId });
  expect(got.ok(), await got.text()).toBeTruthy();
  const character = (await got.json()).character as { name: string; revision: number; sheet: { full: { classes: object[] } & Record<string, unknown> } };
  const full = { ...character.sheet.full, classes: [{ classKey: 'class:barbarian', level: 3, subclassKey }] };
  const updated = await callRPC(master, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
    campaignId: table.campaignId,
    characterId: table.characterId,
    revision: character.revision,
    name: character.name,
    sheet: { full },
  });
  expect(updated.ok(), await updated.text()).toBeTruthy();
  return { ...table, dice: dice === 2 ? ['Golpe Certeiro', 'Golpe Firme'] : ['Golpe Certeiro'] };
}

/** The combat of the maneuver specs: Brutus first, the Goblins next to him, begun. Goblin 1 stands at his side. */
export const brutusFirst = { Brutus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 };

/** Begins the combat with Brutus on turn: Goblin 1 stands at his side, the captain and Goblin 2 far. */
export function beginManeuverCombatRPC(master: Page, table: ManeuverTable, faces: Record<string, number> = brutusFirst) {
  return beginAttackCombatRPC(master, table, faces, { Brutus: [3, 7], 'Goblin 1': [4, 7], 'Capitão Goblin': [14, 4], 'Goblin 2': [15, 11] });
}

/** The master's correction that leaves the pool without a use. */
export async function spendPoolRPC(master: Page, table: ManeuverTable): Promise<void> {
  await adjustVitalsRPC(master, table.campaignId, table.characterId, { resourcesUsed: [{ key: POOL, used: 2 }] });
}
