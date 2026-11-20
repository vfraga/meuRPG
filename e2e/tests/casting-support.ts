import { expect, type Locator, type Page } from '@playwright/test';

import { brisa, brisaSheet } from './combat-support';
import { openSheetTab, startSessionRPC } from './live-session-support';
import { tableForMaps, type MapsTable } from './maps-support';
import { setTableRulesRPC } from './table-rules-support';
import { pensantus } from './support';

// Setup for the casting specs (MR-048), through the API: these tests prove the casting screens, not the
// campaign and character forms other specs cover. Every test makes its own campaign.

export interface CastingTable extends MapsTable {
  sessionId: string;
}

/**
 * Pensantus at level 5 with Armadura Arcana and Detectar Magia prepared and Alarme in the spellbook only (a ritual
 * the wizard may cast without preparing it), and an open session: the casts happen during one.
 */
export async function tableForCasting(master: Page, player: Page, name: string): Promise<CastingTable> {
  const base = await tableForMaps(
    master,
    player,
    name,
    false,
    {
      cantripKeys: ['spell:fire-bolt'],
      knownSpellKeys: ['spell:mage-armor', 'spell:alarm', 'spell:detect-magic', 'spell:magic-missile'],
      preparedSpellKeys: ['spell:mage-armor', 'spell:detect-magic', 'spell:magic-missile'],
    },
    { ...pensantus, level: 5 },
  );
  return { ...base, sessionId: await startSessionRPC(master, base.campaignId) };
}

/**
 * Brisa, a level 5 Cleric (two 3rd-level slots) with Curar Ferimentos prepared, at a table whose dice are all physical, and an
 * open session: the heal's dice are typed as a sum.
 */
export async function tableForHealing(master: Page, player: Page, name: string): Promise<CastingTable> {
  const base = await tableForMaps(master, player, name, false, brisaSheet, { ...brisa, level: 5 });
  await setTableRulesRPC(master, base.campaignId, { diceMode: 'DICE_MODE_PHYSICAL' });
  return { ...base, sessionId: await startSessionRPC(master, base.campaignId) };
}

/** The cast sheet, whichever container it is in (a dialog from a tablet up, a bottom sheet on a phone). */
export function castSheet(page: Page): Locator {
  return page.locator('app-cast-out-sheet');
}

/** Opens "Conjurar fora do combate" in the player's "Ficha" (a tab of the rail, a panel on a phone), picks the spell and waits for its step. */
export async function openCastOf(page: Page, spell: string): Promise<Locator> {
  await openSheetTab(page);
  await page.getByRole('button', { name: 'Conjurar fora do combate', exact: true }).click();
  const sheet = castSheet(page);
  await expect(sheet.getByText('Magia', { exact: true }).first()).toBeVisible();
  await choiceCard(sheet, spell).click();
  await expect(sheet.locator('.frame__title')).toHaveText(spell);
  return sheet;
}

/**
 * The card of a choice in the open step, found by the role and name of its input (the accessible name starts with the
 * card's title: "1º nível", "Pensantus (você)", "Armadura Arcana"). The input is hidden under its label, as the app draws it
 * everywhere, so a test taps the label that wraps it.
 */
export function choiceCard(sheet: Locator, name: string): Locator {
  const input = new RegExp(`^${name}`);
  return sheet
    .getByRole('radio', { name: input })
    .or(sheet.getByRole('checkbox', { name: input }))
    .locator('xpath=ancestor::label');
}

/** Picks the card whose title starts with `name`, and waits until it shows as chosen. */
export async function pickChoice(sheet: Locator, name: string): Promise<void> {
  const card = choiceCard(sheet, name);
  await card.click();
  await expect(card.locator('input')).toBeChecked();
}

/** Picks the slot of the given level in the open step ("1º nível"). */
export async function pickSlotRadio(sheet: Locator, circle: string): Promise<void> {
  await pickChoice(sheet, circle);
}

/** Picks the target by name in the open step (a radio for a spell with one target, a check box for more). */
export async function pickTargetOf(sheet: Locator, name: string): Promise<void> {
  await pickChoice(sheet, name);
}
