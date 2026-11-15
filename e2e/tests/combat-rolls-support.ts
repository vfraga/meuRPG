import type { Browser, BrowserContext, Page } from '@playwright/test';

import { beginAttackCombatRPC, tableForCombat, type CombatTable } from './combat-support';
import { placeTokenRPC } from './maps-support';
import { sq20, thirdPlayer, type ThirdPlayer } from './trap-support';
import { endOpenSessionRPC } from './live-session-support';
import { newSignedInContext, type CharacterBuild } from './support';

// The table of the attack-roll specs: a player's character beside the goblins, adjacent to
// Goblin 1, with the combat begun. The characters come through the API.

export const vex: CharacterBuild = {
  name: 'Vex',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:rogue',
  class: 'Ladino',
  subclassKey: 'subclass:thief',
  subclass: 'Ladrão',
  level: 3,
  background: 'Criminoso',
  backgroundSkillKeys: ['skill:stealth', 'skill:deception'],
  backgroundSkills: ['Furtividade', 'Enganação'],
  extraSkillKeys: ['skill:acrobatics', 'skill:perception', 'skill:investigation', 'skill:sleight-of-hand'],
  extraSkills: ['Acrobacia', 'Percepção', 'Investigação', 'Prestidigitação'],
  scores: { for: 10, des: 16, con: 14, int: 12, sab: 12, car: 10 },
};

export const grog: CharacterBuild = {
  name: 'Grog',
  raceKey: 'race:human',
  race: 'Humano',
  classKey: 'class:barbarian',
  class: 'Bárbaro',
  subclassKey: 'subclass:berserker',
  subclass: 'Caminho do Furioso',
  level: 3,
  background: 'Forasteiro',
  backgroundSkillKeys: ['skill:athletics', 'skill:survival'],
  backgroundSkills: ['Atletismo', 'Sobrevivência'],
  extraSkillKeys: ['skill:perception', 'skill:intimidation'],
  extraSkills: ['Percepção', 'Intimidação'],
  scores: { for: 16, des: 14, con: 15, int: 8, sab: 12, car: 10 },
};

/** Grog's build as a lightfoot halfling: the race's Lucky (Sortudo) asks before the result when a d20 shows a 1. */
export const pipo: CharacterBuild = {
  ...grog,
  name: 'Pipo',
  raceKey: 'race:halfling',
  race: 'Halfling',
  subraceKey: 'subrace:lightfoot-halfling',
  subrace: 'Halfling Pés-Leves',
};

export interface RollsTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  /** The second player's character when the table was asked for one (a player's character to hit). */
  ally?: ThirdPlayer;
  done: () => Promise<void>;
}

/** What a table may add to the attacker's: content made before the character (it returns the build), and a second player's character on the map beside it. */
export interface RollsExtras {
  before?: (campaignId: string, master: Page) => Promise<CharacterBuild>;
  ally?: { build: CharacterBuild; col: number; row: number };
}

export async function rollsTable(
  browser: Browser,
  name: string,
  who: CharacterBuild,
  sheet: Record<string, unknown>,
  faces: Record<string, number>,
  look: { colorScheme?: 'light' | 'dark'; masterWidth?: number; playerWidth?: number; deviceScaleFactor?: number } = {},
  extras: RollsExtras = {},
): Promise<RollsTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: look.masterWidth ?? 1280, height: 900 }, colorScheme: look.colorScheme, deviceScaleFactor: look.deviceScaleFactor });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: look.playerWidth ?? 390, height: 844 }, colorScheme: look.colorScheme, deviceScaleFactor: look.deviceScaleFactor });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, { build: who, sheet, before: extras.before });
  const ally = extras.ally ? await thirdPlayer(browser, { m, p, table, campaignId: table.campaignId, done: async () => undefined }, extras.ally.build, {}, { colorScheme: look.colorScheme, width: look.playerWidth, deviceScaleFactor: look.deviceScaleFactor }) : undefined;
  if (ally && extras.ally) {
    const at = sq20(extras.ally.col, extras.ally.row);
    await placeTokenRPC(m, table.campaignId, table.mapId, ally.characterId, at.xBp, at.yBp);
  }
  await beginAttackCombatRPC(m, table, faces, { 'Capitão Goblin': [11, 5], 'Goblin 1': [9, 9], 'Goblin 2': [14, 10], [who.name]: [8, 9], ...(extras.ally ? { [extras.ally.build.name]: [extras.ally.col, extras.ally.row] } : {}) });
  return {
    m,
    p,
    table,
    campaignId: table.campaignId,
    ally,
    done: async () => {
      await endOpenSessionRPC(m, table.campaignId);
      await ally?.close();
      await master.close();
      await player.close();
    },
  };
}
