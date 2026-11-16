import { expect, type Locator, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

import { callRPC } from './support';
import { combatRPC, type Encounter } from './combat-support';

// Setup for the specs of the master's Inspiração screens (#581, RN-43), through the API: the master's grant and the read, as
// the screens call them. The specs prove what the screens say of who has it; the table and the combat come from the other supports.

/** The folder the screenshots go to when `E2E_SHOTS_DIR` names one; otherwise nothing is written. */
const shotsDir = process.env.E2E_SHOTS_DIR;

/** The device scale of a context that takes screenshots: 2 when they are asked for, else the browser's own. */
export const favorShotScale = shotsDir ? { deviceScaleFactor: 2 } : {};

export const FAVOR_WIDTHS = [390, 768, 1280] as const;

/** The viewport of each width: a phone, a tablet and a laptop. */
export function favorViewport(width: number): { width: number; height: number } {
  return { width, height: width === 390 ? 844 : width === 768 ? 1024 : 800 };
}

/** `GrantFavor` as the master, with a key of its own. The session must be open. */
export async function grantFavorRPC(master: Page, campaignId: string, characterId: string): Promise<void> {
  const res = await callRPC(master, 'meurpg.play.v1.FavorService/GrantFavor', { campaignId, characterId, idempotencyKey: crypto.randomUUID() });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** `GetFavor` as `page`'s user: the master reads every holder of the campaign, a player only their own characters. */
export async function holdersRPC(page: Page, campaignId: string): Promise<string[]> {
  const res = await callRPC(page, 'meurpg.play.v1.FavorService/GetFavor', { campaignId });
  expect(res.ok(), await res.text()).toBeTruthy();
  const body = (await res.json()) as { holders?: { characterId: string }[] };
  return (body.holders ?? []).map((h) => h.characterId);
}

/**
 * Saves the viewport as `<screen>-<width>-<claro|escuro>.png` when `E2E_SHOTS_DIR` is set (a normal run writes nothing), at the
 * width the page is now, scrolled to `at` first when one is given.
 */
export async function shootFavor(page: Page, screen: string, width: number, colorScheme: 'light' | 'dark', at?: Locator): Promise<void> {
  if (!shotsDir) {
    return;
  }
  mkdirSync(shotsDir, { recursive: true });
  if (at) {
    await at.evaluate((el) => {
      el.style.scrollMarginTop = '140px'; // clear the sticky header of the page
      el.scrollIntoView({ block: el.getBoundingClientRect().height < innerHeight * 0.8 ? 'center' : 'start' });
    });
  } else {
    await page.evaluate(() => window.scrollTo(0, 0));
  }
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({ path: join(shotsDir, `${screen}-${width}-${colorScheme === 'light' ? 'claro' : 'escuro'}.png`) });
}

/** The box "Inspiração do mestre" of a roll sheet (`app-favor-spend`), inside `scope` (the sheet, or the whole page). */
export function favorBox(scope: Page | Locator): Locator {
  return scope.locator('app-favor-spend');
}

/** The checkbox "Usar a Inspiração" of the box. Dashed boxes keep it reachable (`aria-disabled`), so it is always there. */
export function favorCheck(scope: Page | Locator): Locator {
  return favorBox(scope).getByRole('checkbox', { name: /Usar a Inspiração/ });
}

/** Ticks the box and waits for the roll-mode segment's own lock word, "pela Inspiração" (a box that is dashed would not). The line under the
 * segment says "Vantagem pela Inspiração: ..." too, so the word is looked up in its own element, not by its text alone. */
export async function tickFavor(scope: Page | Locator): Promise<void> {
  await favorCheck(scope).check();
  await expect(scope.locator('.mode__lock', { hasText: 'pela Inspiração' })).toBeVisible();
}

/** `SetCombatantConditions` as the master: the keys the combatant has from now on (`[]` takes them all away). */
export async function setConditionsRPC(master: Page, campaignId: string, encounter: Encounter, label: string, keys: string[]): Promise<Encounter> {
  const combatant = encounter.combatants.find((c) => c.label === label);
  expect(combatant, `${label} must be in the combat`).toBeTruthy();
  return combatRPC(master, 'SetCombatantConditions', { campaignId, encounterId: encounter.id, combatantId: combatant!.id, conditions: { keys } });
}

/** `PassFavor` as `page`'s user, the owner of `fromCharacterId`: the ask to give the Inspiração to `toCharacterId`. Answers the pass's id. The session must be open. */
export async function passFavorRPC(page: Page, campaignId: string, fromCharacterId: string, toCharacterId: string): Promise<string> {
  const res = await callRPC(page, 'meurpg.play.v1.FavorService/PassFavor', { campaignId, fromCharacterId, toCharacterId, idempotencyKey: crypto.randomUUID() });
  expect(res.ok(), await res.text()).toBeTruthy();
  return ((await res.json()) as { pass: { id: string } }).pass.id;
}
