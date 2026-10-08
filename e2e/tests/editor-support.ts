import { expect, type Locator, type Page } from '@playwright/test';

import { setGridRPC } from './combat-support';
import { squareBp, type FogTable } from './fog-support';
import { canvasPng, createMapRPC, revealMapRPC, uploadImageRPC } from './maps-support';
import { boxOf, callRPC } from './support';

// Setup for the map editor specs (Etapa 9, slice 9.12, MR-034, MR-035, MR-036, MR-041, RN-10): a map of the
// campaign "Mirathel" to paint on, and the cave's traps, chests and light as the artboards (E9-01 and E9-02) draw
// them. What is under test is the editor's screens; the rest goes through the API.

export interface EditorMap {
  mapId: string;
  columns: number;
  rows: number;
}

/** A revealed map of a 1200 x 800 picture, with a grid of `columns` squares (none when 0). */
export async function mapToPaint(master: Page, campaignId: string, name: string, columns: number): Promise<EditorMap> {
  const image = await uploadImageRPC(master, campaignId, name, await canvasPng(master, 1200, 800, name));
  const mapId = await createMapRPC(master, campaignId, name, image);
  await revealMapRPC(master, campaignId, mapId);
  if (columns > 0) {
    await setGridRPC(master, campaignId, mapId, columns);
  }
  return { mapId, columns, rows: columns > 0 ? Math.round((columns * 800) / 1200) : 0 };
}

export function editorRoute(campaignId: string, mapId: string): string {
  return `/campaigns/${campaignId}/maps/${mapId}`;
}

/** The surface that catches the master's brush, as big as the map's picture. */
export function surfaceOf(page: Page): Locator {
  return page.getByRole('application', { name: /Área de pintura do mapa/ });
}

/** Where the middle of a square is on the screen. */
export async function squareOnScreen(page: Page, map: { columns: number; rows: number }, col: number, row: number): Promise<{ x: number; y: number }> {
  const surface = surfaceOf(page);
  await surface.scrollIntoViewIfNeeded();
  const box = await boxOf(surface);
  return { x: box.x + ((col + 0.5) / map.columns) * box.width, y: box.y + ((row + 0.5) / map.rows) * box.height };
}

/** A click on a square with the brush. */
export async function clickSquare(page: Page, map: { columns: number; rows: number }, col: number, row: number): Promise<void> {
  const at = await squareOnScreen(page, map, col, row);
  await page.mouse.click(at.x, at.y);
}

/** A drag from one square to another, pressing and releasing at the ends. */
export async function dragSquares(page: Page, map: { columns: number; rows: number }, from: [number, number], to: [number, number]): Promise<void> {
  const a = await squareOnScreen(page, map, ...from);
  const b = await squareOnScreen(page, map, ...to);
  await page.mouse.move(a.x, a.y);
  await page.mouse.down();
  await page.mouse.move(b.x, b.y, { steps: 12 });
  await page.mouse.up();
}

export interface LayerBits {
  wall: number;
  terrain: number;
  half: number;
  threeQuarters: number;
  light: number;
  columns: number;
  rows: number;
}

/** What `GetMapLayers` gives the caller, counted square by square (the packed layout of `rules/grid`). */
export async function layersOf(page: Page, campaignId: string, mapId: string): Promise<LayerBits> {
  const res = await callRPC(page, 'meurpg.maps.v1.MapService/GetMapLayers', { campaignId, mapId });
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = await res.json();
  const columns = (body.gridColumns ?? 0) as number;
  const rows = (body.gridRows ?? 0) as number;
  const bytes = (key: string) => Buffer.from(body[key] ?? '', 'base64');
  const bits = (buf: Buffer) => {
    let n = 0;
    for (let i = 0; i < columns * rows; i++) {
      n += ((buf[i >> 3] ?? 0) >> (i & 7)) & 1;
    }
    return n;
  };
  const crumbs = (buf: Buffer, value?: number) => {
    let n = 0;
    for (let i = 0; i < columns * rows; i++) {
      const v = ((buf[i >> 2] ?? 0) >> (2 * (i & 3))) & 3;
      n += value === undefined ? (v > 0 ? 1 : 0) : v === value ? 1 : 0;
    }
    return n;
  };
  return {
    wall: bits(bytes('wall')),
    terrain: bits(bytes('difficultTerrain')),
    half: crumbs(bytes('cover'), 1),
    threeQuarters: crumbs(bytes('cover'), 2),
    light: crumbs(bytes('light')),
    columns,
    rows,
  };
}

export async function getMapRPC(page: Page, campaignId: string, mapId: string, asCharacterId = ''): Promise<{ map: Record<string, unknown>; points: Record<string, unknown>[]; tokens: Record<string, unknown>[] }> {
  const res = await callRPC(page, 'meurpg.maps.v1.MapService/GetMap', { campaignId, mapId, asCharacterId });
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = await res.json();
  return { map: body.map, points: body.points ?? [], tokens: body.tokens ?? [] };
}

async function point(master: Page, table: FogTable, body: object): Promise<string> {
  const res = await callRPC(master, 'meurpg.maps.v1.MapService/CreateMapPoint', { campaignId: table.campaignId, mapId: table.mapId, ...body });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).point.id as string;
}

export interface CavePoints {
  pit: string;
  needle: string;
  chestWithNeedle: string;
  coinChest: string;
  altarLight: string;
}

/** The traps, chests and the light of E9-02 on the cave (the torch of the guard room is already there): a hidden pit across the
 * corridor, a poison needle in a chest, a chest of coins and an altar's light with custom radii. */
export async function cavePoints(master: Page, table: FogTable): Promise<CavePoints> {
  const pit = await point(master, table, {
    kind: 'MAP_POINT_KIND_TRAP',
    name: 'Fosso escondido',
    description: 'Um fosso de 6 m, coberto por lajes soltas, no meio do corredor.',
    ...squareBp(11, 7),
    trap: {
      presetKey: 'trap:hidden-pit',
      noticeDc: 15,
      findDc: 15,
      areaSize: 2,
      trigger: 'TRAP_TRIGGER_ENTER',
      effect: { damage: [{ dice: '2d6', damageTypeKey: 'damage-type:bludgeoning' }], conditions: [{ conditionKey: 'condition:prone' }], targets: 'TRAP_TARGETS_AREA' },
    },
  });
  const needle = await point(master, table, {
    kind: 'MAP_POINT_KIND_TRAP',
    name: 'Agulha envenenada',
    description: 'Uma agulha escondida na fechadura do baú.',
    ...squareBp(7, 13),
    trap: {
      presetKey: 'trap:poison-needle',
      findDc: 20,
      areaSize: 1,
      trigger: 'TRAP_TRIGGER_MANUAL',
      effect: {
        damage: [
          { dice: '1', damageTypeKey: 'damage-type:piercing' },
          { dice: '2d10', damageTypeKey: 'damage-type:poison' },
        ],
        save: { ability: 'ABILITY_CONSTITUTION', dc: 15, appliesTo: 'TRAP_SAVE_APPLIES_CAUGHT', onFail: { condition: { conditionKey: 'condition:poisoned', durationPt: '1 hora' } }, onPass: 'TRAP_PASS_OUTCOME_NONE' },
        targets: 'TRAP_TARGETS_MANUAL',
      },
    },
  });
  const chestWithNeedle = await point(master, table, { kind: 'MAP_POINT_KIND_TREASURE', name: 'Baú com agulha', description: 'Moedas de cobre.', ...squareBp(7, 13), treasureValuePo: 0 });
  const coinChest = await point(master, table, { kind: 'MAP_POINT_KIND_TREASURE', name: 'Baú de moedas', description: '250 PO e uma adaga de prata.', ...squareBp(17, 13), treasureValuePo: 250 });
  const altarLight = await point(master, table, { kind: 'MAP_POINT_KIND_LIGHT', name: 'Brasa do altar', ...squareBp(13, 12), light: { presetKey: '', brightFt: 15, dimFt: 15 } });
  return { pit, needle, chestWithNeedle, coinChest, altarLight };
}
