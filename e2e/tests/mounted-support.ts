import { expect, type Browser, type Page } from '@playwright/test';

import { beginAttackCombatRPC, tableForCombat, type CombatTable, type Encounter } from './combat-support';
import { listCreaturesRPC } from './creatures-support';
import { endOpenSessionRPC } from './live-session-support';
import { sq20 } from './trap-support';
import { callRPC, newSignedInContext, type CharacterBuild } from './support';

// Setup of the mounted-combat specs (#588), through the API: a paladin who has Convocar Montaria at a table with a map,
// the goblins and an open session. What is under test is what the player and the master read and press.

/** Isolda, a human Paladin 5: 2nd-level slots for Convocar Montaria and Dexterity 10, so her save to stay on is the bare d20. */
export const isolda: CharacterBuild = {
  name: 'Isolda',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:paladin',
  class: 'Paladino',
  subclassKey: 'subclass:devotion',
  subclass: 'Juramento de Devoção',
  level: 5,
  backgroundKey: 'background:acolyte',
  backgroundSkillKeys: [],
  backgroundSkills: [],
  extraSkillKeys: ['skill:athletics', 'skill:persuasion'],
  extraSkills: [],
  scores: { for: 16, des: 10, con: 14, int: 8, sab: 10, car: 15 },
};
export const isoldaSheet = { weaponKeys: ['equipment:longsword'], armorKey: 'equipment:chain-mail', preparedSpellKeys: ['spell:find-steed', 'spell:bless'] };

/** The name the app gives the steed of these specs: the form's. */
export const STEED = 'Cavalo de guerra';

export interface MountedTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  done: () => Promise<void>;
}

/** Isolda's table with the session open and no combat: the master at `masterWidth`, the player at `playerWidth`. */
export async function mountedTable(
  browser: Browser,
  name: string,
  look: { colorScheme?: 'light' | 'dark'; masterWidth?: number; playerWidth?: number; deviceScaleFactor?: number } = {},
): Promise<MountedTable> {
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: look.masterWidth ?? 1280, height: 900 }, colorScheme: look.colorScheme, deviceScaleFactor: look.deviceScaleFactor });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: look.playerWidth ?? 1280, height: 900 }, colorScheme: look.colorScheme, deviceScaleFactor: look.deviceScaleFactor });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { build: isolda, sheet: isoldaSheet });
  return {
    m,
    p,
    table,
    campaignId: table.campaignId,
    done: async () => {
      await endOpenSessionRPC(m, table.campaignId);
      await master.close();
      await player.close();
    },
  };
}

/** Convocar Montaria through the API (a 2nd-level slot): the steed's creature id. */
export async function castSteedRPC(p: Page, table: CombatTable, monsterKey = 'monster:warhorse'): Promise<string> {
  const res = await callRPC(p, 'meurpg.play.v1.PlayService/CastSummon', {
    campaignId: table.campaignId,
    characterId: table.characterId,
    spellKey: 'spell:find-steed',
    slot: { level: 2 },
    summon: { option: 0, creatureKeys: [monsterKey], creatureType: 'celestial' },
    idempotencyKey: crypto.randomUUID(),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).creatureIds[0] as string;
}

/** The master puts the steed's token on a square of the 20-column map. */
export async function placeSteedRPC(m: Page, table: CombatTable, creatureId: string, col: number, row: number): Promise<void> {
  const { xBp, yBp } = sq20(col, row);
  const res = await callRPC(m, 'meurpg.maps.v1.MapService/PlaceMapToken', { campaignId: table.campaignId, mapId: table.mapId, creatureId, xBp, yBp });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The steed Isolda already has, placed beside her (she stands on 5,7), and the combat begun with her first: Isolda 20, the steed 12, Goblin 1 beside where the pair will ride to. */
export async function beginMountedCombat(t: MountedTable, at: Record<string, [number, number]> = { 'Capitão Goblin': [15, 2], 'Goblin 1': [10, 7], 'Goblin 2': [16, 11] }): Promise<Encounter> {
  const creatures = await listCreaturesRPC(t.p, t.campaignId, t.table.characterId);
  await placeSteedRPC(t.m, t.table, creatures[0].id, 6, 7);
  return beginAttackCombatRPC(t.m, t.table, { Isolda: 20, [STEED]: 12, 'Goblin 1': 6, 'Capitão Goblin': 3, 'Goblin 2': 2 }, at);
}
