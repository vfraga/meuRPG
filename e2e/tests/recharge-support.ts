import { randomUUID } from 'node:crypto';
import { expect, type Browser, type Locator, type Page } from '@playwright/test';
import { mkdirSync } from 'node:fs';
import { join } from 'node:path';

import { combatRPC, getEncounterRPC, passTurnsTo, tableForCombat, type Encounter } from './combat-support';
import { vex } from './combat-rolls-support';
import { openSessionPage } from './live-session-support';
import { callRPC, newSignedInContext } from './support';

// The master's Recharge and uses-per-day screens (#556): a Vrock (Spores, Recharge 6; Stunning Screech, 1/day) against Vex, in the
// theatre (the master names the targets), the d20s typed so the order never depends on a roll. The d6 of a Recharge is random, so the
// screens that follow it hold either result and the checks read what the app says of whichever it was.

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

/** Saves the viewport as `556-<screen>-<width>-<claro|escuro>.png` when `E2E_SHOTS_DIR` is set, scrolled to `at` first. */
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
  await page.screenshot({ path: join(shotsDir, `556-${screen}-${width}-${colorScheme === 'light' ? 'claro' : 'escuro'}.png`) });
}

export interface VrockFight {
  campaignId: string;
  enc: Encounter;
  vrock: string;
  label: string;
  /** The card of the Vrock's turn. */
  card: Locator;
  /** Ends the Vrock's turn and passes the others' until it comes again. */
  nextTurn: () => Promise<void>;
}

/** The combat with a Vrock first in the order and its turn open on both pages; the master's page and the player's. */
export async function beginVrockFight(master: Page, player: Page, name: string): Promise<VrockFight> {
  const table = await tableForCombat(master, player, `${name} ${Date.now()}`, false, false, { build: vex, sheet: { weaponKeys: ['equipment:rapier'] } });
  const campaignId = table.campaignId;
  const started = await callRPC(master, 'meurpg.play.v1.CombatService/StartEncounter', {
    campaignId,
    idempotencyKey: crypto.randomUUID(),
    name: 'Emboscada do demônio',
    participants: [],
    monsters: [{ creatureKey: 'monster:vrock', count: 1 }],
    mode: 'ENCOUNTER_MODE_THEATRE',
  });
  expect(started.ok(), await started.text()).toBeTruthy();
  let enc = (await started.json()).encounter as Encounter;
  const monsterIds = new Set(enc.combatants.filter((c) => (c as { bestiaryCreatureKey?: string }).bestiaryCreatureKey).map((c) => c.id));
  for (const c of enc.combatants) {
    enc = await combatRPC(master, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: monsterIds.has(c.id) ? 20 : 1 });
  }
  enc = await combatRPC(master, 'BeginCombat', { campaignId, encounterId: enc.id });
  const vrock = enc.combatants.find((c) => monsterIds.has(c.id))!;
  // A monster starts hidden, and the players read nothing of a hidden creature's turn: show it, as the master does at the ambush.
  await combatRPC(master, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: vrock.id, hidden: false });
  await passTurnsTo(master, campaignId, vrock.label);
  await openSessionPage(player, campaignId);
  await openSessionPage(master, campaignId);
  const card = master.getByRole('region', { name: new RegExp(`^(Ações|Vez) d[ao] ${vrock.label}$`) });
  return {
    campaignId,
    enc,
    vrock: vrock.id,
    label: vrock.label,
    card,
    nextTurn: async () => {
      const now = await getEncounterRPC(master, campaignId);
      await combatRPC(master, 'EndTurn', { campaignId, encounterId: now.id, expectedCombatantId: now.currentCombatantId, discardPendingDamage: true });
      await passTurnsTo(master, campaignId, vrock.label);
      await master.reload();
      await openSessionPage(master, campaignId);
    },
  };
}

/** The two signed-in pages of a Vrock fight at one width and color scheme. */
/** A time zone where it is 14h now, whatever the hour of the machine: the player's page then reads "desde 14:MM" in the session bar, a
 * text with "de 1" in it (as one in 10h to 19h), so a search of the whole page for a use count would match the clock and fail by the hour. */
function afternoonZone(): string {
  const offset = 14 - new Date().getUTCHours(); // -9 to +14 hours from UTC
  return offset === 0 ? 'UTC' : `Etc/GMT${offset > 0 ? '-' : '+'}${Math.abs(offset)}`;
}

export async function pagesFor(browser: Browser, colorScheme: 'light' | 'dark', width: number) {
  const viewport = viewportOf(width);
  const master = await newSignedInContext(browser, 'Mestre Teste', { viewport, colorScheme, ...shotScale });
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport, colorScheme, timezoneId: afternoonZone(), ...shotScale });
  const m = await master.newPage();
  const p = await player.newPage();
  await m.goto('/');
  await p.goto('/');
  return { m, p, close: async () => { await master.close(); await player.close(); } };
}

/** The master uses an action of the Vrock through the API: `spores` or `stunning-screech`, on Vex in the theatre. */
export async function useCreatureActionRPC(master: Page, fight: VrockFight, action: 'spores' | 'stunning-screech'): Promise<void> {
  const enc = await getEncounterRPC(master, fight.campaignId);
  const vex = enc.combatants.find((c) => c.label === 'Vex')!;
  const res = await callRPC(master, 'meurpg.play.v1.CombatService/UseCreatureAction', {
    campaignId: fight.campaignId,
    encounterId: enc.id,
    combatantId: fight.vrock,
    actionKey: `monster:vrock#${action}`,
    targetIds: [vex.id],
    idempotencyKey: randomUUID(),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}
