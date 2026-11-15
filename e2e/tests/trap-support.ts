import { expect, type Browser, type BrowserContext, type Page } from '@playwright/test';

import { type CombatTable, combatRPC, getEncounterRPC, tableForCombat, beginAttackCombatRPC, type Encounter } from './combat-support';
import { endOpenSessionRPC } from './live-session-support';
import { callRPC, characterRpcBody, createCharacterRPC, newSignedInContext, type CharacterBuild } from './support';

// Setup for the trap and treasure specs (Etapa 9, slice 9.14, MR-035, MR-041, RN-10, RN-02): a table
// with the map on a 20 x 14 grid (squares of 100 px of the 2000 x 1400 image), Pensantus's token on
// (5, 7), and traps and treasures created through the API. What is under test is the screens.

export interface TrapTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  done: () => Promise<void>;
}

/** The middle of a square of the 20 x 14 grid, in basis points. */
export function sq20(col: number, row: number): { xBp: number; yBp: number } {
  return { xBp: Math.round(((col + 0.5) * 10000) / 20), yBp: Math.round(((row + 0.5) * 10000) / 14) };
}

export async function trapTable(browser: Browser, name: string, character: { build?: CharacterBuild; sheet?: Record<string, unknown> } = {}): Promise<TrapTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 1000 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 1000 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, character);
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

export interface TrapSpecBody {
  noticeDc?: number;
  findDc?: number;
  areaSize?: number;
  manual?: boolean;
  /** A flat damage, so the number is known ("3" is 3 de concussão). */
  damage?: string;
  /** The skills besides Percepção and Investigação that find it ("Também acham com"), as content keys. */
  alsoFind?: string[];
}

/** A trap on the map (born hidden); returns its point's ID. */
export async function trapRPC(m: Page, table: CombatTable, name: string, col: number, row: number, spec: TrapSpecBody = {}): Promise<string> {
  const res = await callRPC(m, 'meurpg.maps.v1.MapService/CreateMapPoint', {
    campaignId: table.campaignId,
    mapId: table.mapId,
    kind: 'MAP_POINT_KIND_TRAP',
    name,
    description: 'No corredor',
    ...sq20(col, row),
    trap: {
      noticeDc: spec.noticeDc ?? 30,
      findDc: spec.findDc ?? 5,
      alsoFindSkillKeys: spec.alsoFind ?? [],
      areaSize: spec.areaSize ?? 1,
      trigger: spec.manual ? 'TRAP_TRIGGER_MANUAL' : 'TRAP_TRIGGER_ENTER',
      effect: { damage: [{ dice: spec.damage ?? '3', damageTypeKey: 'damage-type:bludgeoning' }] },
    },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).point.id as string;
}

export async function treasureRPC(m: Page, table: CombatTable, name: string, col: number, row: number, valuePo = 250): Promise<string> {
  const res = await callRPC(m, 'meurpg.maps.v1.MapService/CreateMapPoint', {
    campaignId: table.campaignId,
    mapId: table.mapId,
    kind: 'MAP_POINT_KIND_TREASURE',
    name,
    description: '250 PO e uma adaga de prata.',
    ...sq20(col, row),
    treasureValuePo: valuePo,
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).point.id as string;
}

/** The names of the points of the map as this page's user is sent them (RN-10: a trap they do not know is not there). */
export async function pointNames(page: Page, table: CombatTable): Promise<string[]> {
  const res = await callRPC(page, 'meurpg.maps.v1.MapService/GetMap', { campaignId: table.campaignId, mapId: table.mapId });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (((await res.json()).points ?? []) as { name: string }[]).map((x) => x.name);
}

/** A combat where Pensantus acts first, begun; returns it. */
export async function pensantusFirst(m: Page, table: CombatTable): Promise<Encounter> {
  return beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
}

/** The player's own move, through the API (the move page is 9.15's). */
export async function movePensantus(p: Page, table: CombatTable, col: number, row: number): Promise<Encounter> {
  const enc = await getEncounterRPC(p, table.campaignId);
  const me = enc.combatants.find((c) => c.label === 'Pensantus')!;
  return combatRPC(p, 'MoveCombatant', { campaignId: table.campaignId, encounterId: enc.id, combatantId: me.id, col, row });
}

export interface ThirdPlayer {
  page: Page;
  characterId: string;
  close: () => Promise<void>;
}

/**
 * A second player with a character in the table (RN-03 allows one living character each, so the first account's Pensantus and this one's
 * Toren): the third devidp account "E-mail Não Verificado", from the session `auth.setup.ts` saves.
 */
export async function thirdPlayer(
  browser: Browser,
  t: TrapTable,
  build: CharacterBuild,
  sheet: Record<string, unknown> = {},
  look: { colorScheme?: 'light' | 'dark'; width?: number; deviceScaleFactor?: number } = {},
): Promise<ThirdPlayer> {
  const context = await newSignedInContext(browser, 'E-mail Não Verificado', {
    viewport: { width: look.width ?? 1280, height: look.width === undefined ? 1000 : 844 },
    colorScheme: look.colorScheme,
    deviceScaleFactor: look.deviceScaleFactor,
  });
  const page = await context.newPage();
  await page.goto('/');
  const invite = await callRPC(t.m, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId: t.campaignId, maxUses: 1, expiresIn: '3600s' });
  expect(invite.ok(), await invite.text()).toBeTruthy();
  const joined = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
  expect(joined.ok(), await joined.text()).toBeTruthy();
  const body = characterRpcBody('PLAYER', build) as { sheet: { full: object } };
  body.sheet.full = { ...body.sheet.full, ...sheet };
  const made = await createCharacterRPC(page, t.campaignId, body);
  expect(made.ok(), await made.text()).toBeTruthy();
  return { page, characterId: (await made.json()).character.id as string, close: () => context.close() };
}
