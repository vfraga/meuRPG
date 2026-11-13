import { expect, type Browser, type Locator, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

import { combatRPC, startEncounterWithMonstersRPC, tableForCombat, type CombatTable, type Encounter } from './combat-support';
import { endOpenSessionRPC } from './live-session-support';
import { newSignedInContext } from './support';

// Setup for the specs of the sunlight in combat (#704, RN-20, RN-29), through the API: a combat on a map with Pensantus, the
// Capitão Goblin, two Goblins and a Drow (Sunlight Sensitivity), every initiative typed and every token placed, in SETUP. The
// specs prove what the two screens read of the sun; the table, the initiative and the squares come through the API.

/** The squares of the combat's map where the NPCs stand (columns and rows count from 0); the Drow's is lit by `LIT`. */
const SQUARES: Record<string, [number, number]> = { Drow: [12, 6], 'Capitão Goblin': [9, 4], 'Goblin 1': [9, 9], 'Goblin 2': [16, 10] };

/** A block of squares around the Drow, lit when the scope is "Onde eu pintar": 4 columns by 2 rows, 8 squares. */
export const LIT: { col: number; row: number }[] = [12, 13, 14, 15].flatMap((col) => [6, 7].map((row) => ({ col, row })));

/** A square far from the Drow and from every token, for the specs that paint one by hand. */
export const FAR = { col: 2, row: 12 };

export interface SunTable {
  m: Page;
  p: Page;
  table: CombatTable;
  campaignId: string;
  /** The combat in SETUP, with every initiative typed. */
  enc: Encounter;
  /** The Drow's label in the order. */
  drow: string;
  done: () => Promise<void>;
}

/**
 * The master's and the player's pages at `width` and a combat in SETUP: Pensantus, the Capitão, two Goblins and a Drow, with the
 * Drow first in the order, all in plain sight. Neither page navigates.
 */
export async function sunlightTable(browser: Browser, name: string, width = 1280, colorScheme: 'light' | 'dark' = 'light'): Promise<SunTable> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport, colorScheme, ...shotScale });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme, ...shotScale });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  const table = await tableForCombat(m, p, `${name} ${Date.now()}`, true, true);
  const campaignId = table.campaignId;
  const done = async () => {
    await endOpenSessionRPC(m, campaignId);
    await master.close();
    await player.close();
  };
  try {
    let enc = await startEncounterWithMonstersRPC(m, table, ['monster:drow']);
    const drow = enc.combatants.find((c) => c.label.startsWith('Drow'))!.label;
    // Totals that never tie: the Drow's face is the highest, so it plays first.
    const faces: Record<string, number> = { [drow]: 20, Pensantus: 12, 'Capitão Goblin': 9, 'Goblin 1': 6, 'Goblin 2': 3 };
    for (const c of enc.combatants) {
      enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: faces[c.label] ?? 1 });
    }
    for (const [label, [col, row]] of Object.entries({ ...SQUARES, [drow]: SQUARES.Drow })) {
      const combatantId = enc.combatants.find((c) => c.label === label)?.id;
      if (combatantId) {
        enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId, col, row });
      }
    }
    return { m, p, table, campaignId, enc, drow, done };
  } catch (err) {
    await done();
    throw err;
  }
}

/** `SetSunlight` as the master: the sun on, a scope and the squares to add or remove. Returns the combat after the change. */
export function setSunlightRPC(
  page: Page,
  t: { campaignId: string; encounterId: string },
  change: { on?: boolean; scope?: 'SUNLIGHT_SCOPE_MAP' | 'SUNLIGHT_SCOPE_SQUARES'; addSquares?: { col: number; row: number }[]; removeSquares?: { col: number; row: number }[] },
): Promise<Encounter> {
  return combatRPC(page, 'SetSunlight', { campaignId: t.campaignId, encounterId: t.encounterId, on: true, ...change });
}

/** The master's bar of the combat (setup or running). */
export const bar = (page: Page): Locator => page.locator('app-combat-bar');

/**
 * The sun switch, by its name ("Luz do sol: desligada", "...: todo o mapa", "...: onde eu pintar"): there is one in the page, in the
 * combat bar (setup, 768 and up) or, on a phone, in the combat header ("Rodada 2 · em seguida: ...") or the setup bar's head.
 */
export const sunSwitch = (page: Page, name: string | RegExp = /^Luz do sol:/): Locator => page.getByRole('switch', { name });

/** What holds the sun switch at the current width: the combat header on a phone, the bar otherwise (what the screenshots frame). */
export const sunSpot = (page: Page): Locator => page.locator('app-combat-bar, .phone__line').filter({ has: page.getByRole('switch') }).first();

/** The map card with the scope controls (the combat's map, or the setup's "Posições iniciais"). */
export const mapCard = (page: Page): Locator => page.locator('app-combat-map-card').filter({ has: page.locator('app-sunlight-controls') });

/** The paint surface over the map: it takes the arrows and Space, or a tap. */
export const paintPad = (page: Page): Locator => page.getByRole('application', { name: /^Pintar luz do sol no mapa/ });

/** The status line of paint mode ("Pintando luz do sol · 3 quadrados"). */
export const paintStatus = (page: Page): Locator => page.locator('app-sunlight-controls').getByRole('status');

/** The master's "Próximo turno": it stays in the page while the sun is painted, off (`aria-disabled`). */
export const nextTurn = (page: Page): Locator => page.getByRole('button', { name: 'Próximo turno' });

/** Where the middle of a square of the pad is, for a tap: the pad covers the whole map. */
export async function squareCenter(pad: Locator, square: { col: number; row: number }, columns: number, rows: number): Promise<{ x: number; y: number }> {
  const box = await pad.boundingBox();
  expect(box, 'the paint pad has a box').not.toBeNull();
  return { x: ((square.col + 0.5) * box!.width) / columns, y: ((square.row + 0.5) * box!.height) / rows };
}

/** The folder the screenshots go to when `E2E_SHOTS_DIR` names one; otherwise nothing is written. */
const shotsDir = process.env.E2E_SHOTS_DIR;

/** The device scale of a context that takes screenshots: 2 when they are asked for, else the browser's own. */
export const shotScale = shotsDir ? { deviceScaleFactor: 2 } : {};

/** The widths of the screens (a phone, a tablet and a laptop) and the viewport height each is judged at, never full page. */
export const WIDTHS = [390, 768, 1280] as const;
const HEIGHTS: Record<number, number> = { 390: 844, 768: 1024, 1280: 800 };

export function viewportOf(width: number): { width: number; height: number } {
  return { width, height: HEIGHTS[width] ?? 800 };
}

/** Saves the viewport as `704-<screen>-<width>-<claro|escuro>.png` when `E2E_SHOTS_DIR` is set, scrolled to `at` first. */
export async function shoot(page: Page, screen: string, width: number, colorScheme: 'light' | 'dark', at?: Locator): Promise<void> {
  if (!shotsDir) {
    return;
  }
  mkdirSync(shotsDir, { recursive: true });
  await page.setViewportSize(viewportOf(width));
  if (at) {
    await at.evaluate((el) => {
      el.style.scrollMarginTop = '140px'; // clear the sticky header of the page
      el.scrollIntoView({ block: el.getBoundingClientRect().height < innerHeight * 0.8 ? 'center' : 'start' });
    });
  } else {
    await page.evaluate(() => window.scrollTo(0, 0));
  }
  await page.screenshot({ path: join(shotsDir, `704-${screen}-${width}-${colorScheme === 'light' ? 'claro' : 'escuro'}.png`) });
}
