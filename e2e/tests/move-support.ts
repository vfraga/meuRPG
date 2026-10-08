import { expect, type Browser, type BrowserContext, type Locator, type Page } from '@playwright/test';

import { beginAttackCombatRPC, tableForCombat, type CombatTable } from './combat-support';
import { endOpenSessionRPC } from './live-session-support';
import { boxOf, callRPC, newSignedInContext, type CharacterBuild } from './support';

// Setup for the movement specs (Etapa 9, slice 9.15, MR-034, RN-21): a combat
// on a 20 x 14 grid with the layers painted through `PaintMapCells`, and the
// two screens (the master's and the player's, both desktop-wide so the whole
// map is in view).

export interface MoveTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  done: () => Promise<void>;
}

/** A layer of the map's grid, as the Connect JSON names it. */
export type Layer = 'MAP_LAYER_DIFFICULT_TERRAIN' | 'MAP_LAYER_WALL' | 'MAP_LAYER_COVER';

/** Paints squares of one layer: terrain and wall take 0 or 1, cover 1 (half) or 2 (three-quarters). */
export async function paintRPC(page: Page, table: CombatTable, layer: Layer, value: number, squares: [number, number][]): Promise<void> {
  const res = await callRPC(page, 'meurpg.maps.v1.MapService/PaintMapCells', {
    campaignId: table.campaignId,
    mapId: table.mapId,
    layer,
    value,
    squares: squares.map(([col, row]) => ({ col, row })),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/**
 * A combat with the player's character (Pensantus unless `character` says
 * otherwise), the Capitão and two Goblins, begun with the given d20 faces. The
 * player's screen is 1280 wide, so the "Mover" page has the whole map on it.
 * `paint` runs before the combat begins, so the layers are on the map from the
 * first read.
 */
export async function movingTable(
  browser: Browser,
  name: string,
  faces: Record<string, number>,
  options: {
    at?: Record<string, [number, number]>;
    hidden?: string[];
    character?: { build?: CharacterBuild; sheet?: Record<string, unknown> };
    paint?: (m: Page, table: CombatTable) => Promise<void>;
    viewport?: { width: number; height: number };
  } = {},
): Promise<MoveTable> {
  const master: BrowserContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
  const player: BrowserContext = await newSignedInContext(browser, 'Jogador Teste', { viewport: options.viewport ?? { width: 1280, height: 900 } });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true, options.character ?? {});
  await options.paint?.(m, table);
  await beginAttackCombatRPC(m, table, faces, options.at, options.hidden);
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

/** Taps a square of the "Mover" page's map (20 x 14 squares, the map fills its box). */
export async function tapSquare(page: Page, col: number, row: number): Promise<void> {
  const map: Locator = page.getByRole('group', { name: /Mapa de batalha/ });
  await expect(map).toBeVisible();
  const box = await boxOf(map);
  await map.click({ position: { x: ((col + 0.5) * box.width) / 20, y: ((row + 0.5) * box.height) / 14 } });
}

/** Picks a radio the app draws as a label over a hidden input (the "Andar | Saltar" segments, the cover mark's choices). */
export async function pickRadio(scope: Page | Locator, text: string | RegExp): Promise<void> {
  await scope.locator('label', { hasText: text }).first().click();
}
