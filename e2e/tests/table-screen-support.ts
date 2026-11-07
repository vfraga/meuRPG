import { expect, type BrowserContext, type Locator, type Page } from '@playwright/test';

import { openSessionMenu, startSessionRPC, tableWithPensantus } from './live-session-support';
import { canvasPng, createMapRPC, createPointRPC, revealMapRPC, setCurrentMapRPC, uploadImageRPC } from './maps-support';
import { openSceneRPC } from './scene-support';
import { createMiraRPC, putOnStageRPC } from './stage-support';
import { callRPC } from './support';
import { combatRPC, startEncounterRPC, type CombatTable } from './combat-support';
import { type FogTable } from './fog-support';

// Setup and helpers for the Tela da mesa specs (#648): the master's control in the live session, the TV tab it opens and what that
// tab asks of the server. Data goes through the API; the control and the TV tab are what is under test.

/** The route of the TV tab, the one address the "Copiar o link" button puts on the clipboard. */
export const tvRoute = (campaignId: string): string => `/campaigns/${campaignId}/table-screen`;

export interface TvTable {
  campaignId: string;
  campaignName: string;
}

/** A campaign of Mestre Teste's with Jogador Teste (Pensantus) in it and its session open: no map, no scene, no image, so the TV is idle. */
export async function tableForTv(master: Page, player: Page, name: string): Promise<TvTable> {
  const table = await tableWithPensantus(master, player, name);
  await startSessionRPC(master, table.campaignId);
  return { campaignId: table.campaignId, campaignName: name };
}

export interface TvContent {
  mapId: string;
  /** "A carroça tombada": a revealed scene point of the map. */
  sceneId: string;
  /** The gallery image "Capitão Goblin", not shown yet. */
  imageId: string;
}

/** A revealed map "Estrada do Vale" with a scene point, and a gallery image; none of them is on the table until a test puts it there. */
export async function addTvContent(master: Page, campaignId: string): Promise<TvContent> {
  const mapImage = await uploadImageRPC(master, campaignId, 'Estrada do Vale', await canvasPng(master, 1600, 1000, 'Estrada do Vale'));
  const imageId = await uploadImageRPC(master, campaignId, 'Capitão Goblin', await canvasPng(master, 600, 800, 'Capitão Goblin', '#3a3a2a'));
  const mapId = await createMapRPC(master, campaignId, 'Estrada do Vale', mapImage);
  await revealMapRPC(master, campaignId, mapId);
  const sceneId = await createPointRPC(master, campaignId, mapId, {
    kind: 'SCENE',
    name: 'A carroça tombada',
    description: 'Uma carroça de mercador tombada na estrada. Há caixas espalhadas e rastros de botas na lama.',
    xBp: 4500,
    yBp: 4200,
    revealed: true,
  });
  return { mapId, sceneId, imageId };
}

/** The map the session has now, the way the master's "Mapa" tab sets it. */
export const putMapOnTable = setCurrentMapRPC;

/** The scene open to the whole table. */
export const putSceneOnTable = openSceneRPC;

/** The image shown to the whole table (`''` takes it off). */
export async function showImageRPC(master: Page, campaignId: string, imageId: string): Promise<void> {
  const res = await callRPC(master, 'meurpg.play.v1.PlayService/SetShownImage', { campaignId, imageId });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** A d20 rolled for the whole table, by the master. */
export async function rollForTableRPC(master: Page, campaignId: string): Promise<void> {
  const res = await callRPC(master, 'meurpg.play.v1.PlayService/RollFreeDice', { campaignId, sides: 20, count: 1, idempotencyKey: crypto.randomUUID() });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** The session bar's "Tela da mesa" button (from 1024 px; under it the "Sessão" menu keeps the line). */
export const tvButton = (page: Page): Locator => page.getByTestId('tv-button');

/** Opens the master's panel from the session bar (its button from 1024 px, the "Sessão" menu under it) and returns it. */
export async function openTvPanel(page: Page): Promise<Locator> {
  if ((page.viewportSize()?.width ?? Number.POSITIVE_INFINITY) < 1024) {
    await openSessionMenu(page);
    await page.getByTestId('tv-menu-item').click();
  } else {
    await tvButton(page).click();
  }
  const panel = page.getByRole('dialog', { name: 'Tela da mesa' });
  await expect(panel).toBeVisible();
  return panel;
}

/** Closes the panel with Escape, the way a keyboard does. */
export async function closeTvPanel(page: Page): Promise<void> {
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog', { name: 'Tela da mesa' })).toHaveCount(0);
}

/** Chooses a row of the panel (`tv-mirror`, `tv-pin-map`...) the way a person does, by its label: the radio under it is visually hidden. */
export async function chooseRow(panel: Locator, testId: string): Promise<void> {
  await panel.getByTestId(testId).locator('label').click();
  await expect(panel.getByTestId(testId).getByRole('radio')).toBeChecked();
}

/** "Abrir a Tela da mesa" of the panel: the link opens the TV in a new tab of the master's own browser, returned once it is on its route. */
export async function openTvTab(context: BrowserContext, panel: Locator, campaignId: string): Promise<Page> {
  const opened = context.waitForEvent('page');
  await panel.locator('.js-open').click();
  const tv = await opened;
  await expect(tv).toHaveURL(new RegExp(`${tvRoute(campaignId)}$`));
  return tv;
}

/** The title of the TV tab's header: the h1 names the item on the stage (the idle screen names the session). */
export const tvTitle = (tv: Page): Locator => tv.getByRole('heading', { level: 1 });

/** What a page asked of the server, in order. */
export interface Traffic {
  /** Every `/meurpg.` procedure path asked, such as `/meurpg.play.v1.TableScreenService/GetTableScreen`. */
  procedures: () => string[];
  /** The images the page asked for, as path and query. */
  images: () => string[];
}

/** Starts recording every request `page` makes; call it before the first `goto`. */
export function recordTraffic(page: Page): Traffic {
  const procedures: string[] = [];
  const images: string[] = [];
  page.on('request', (request) => {
    const url = new URL(request.url());
    if (url.pathname.startsWith('/meurpg.')) {
      procedures.push(url.pathname);
    }
    if (request.resourceType() === 'image') {
      images.push(url.pathname + url.search);
    }
  });
  return { procedures: () => [...procedures], images: () => [...images] };
}

/** Everything the page keeps in `localStorage` and `sessionStorage`, as one comparable value. */
export function storageOf(page: Page): Promise<string> {
  return page.evaluate(() => {
    const dump = (store: Storage): [string, string | null][] =>
      Object.keys(store)
        .sort()
        .map((key) => [key, store.getItem(key)]);
    return JSON.stringify({ local: dump(localStorage), session: dump(sessionStorage) });
  });
}

/** The numbers of an element's scroll: how far its content reaches past its box. */
export function overflowOf(locator: Locator): Promise<{ height: number; width: number }> {
  return locator.evaluate((el) => ({ height: el.scrollHeight - el.clientHeight, width: el.scrollWidth - el.clientWidth }));
}

/** The panel's scrolling body fits its box: nothing to scroll, either way. */
export async function expectPanelFits(panel: Locator, where: string): Promise<void> {
  const over = await overflowOf(panel.locator('.frame__body'));
  expect(over, `the panel scrolls ${where}`).toEqual({ height: 0, width: 0 });
}

/**
 * How far the TV reaches past its window, in the document and in the page's own box: the app's root clips what the page overflows
 * (the document measures 800 of 800 while the page is 3123 high), so the page's box is where an overflow shows.
 */
export function pageOverflow(tv: Page): Promise<{ height: number; width: number }> {
  return tv.evaluate(() => {
    const root = document.documentElement;
    const host = document.querySelector('app-table-screen-page');
    return {
      height: Math.max(root.scrollHeight - window.innerHeight, host ? host.scrollHeight - host.clientHeight : 0),
      width: Math.max(root.scrollWidth - window.innerWidth, host ? host.scrollWidth - host.clientWidth : 0),
    };
  });
}

/** Two story NPCs on the stage of the open scene, the first one speaking. */
export async function twoOnStageRPC(master: Page, campaignId: string): Promise<void> {
  const mira = await createMiraRPC(master, campaignId, '', 'Mira');
  const borin = await createMiraRPC(master, campaignId, '', 'Borin');
  await putOnStageRPC(master, campaignId, mira);
  await putOnStageRPC(master, campaignId, borin);
  const res = await callRPC(master, 'meurpg.play.v1.PlayService/SetSpeaker', { campaignId, characterId: mira });
  expect(res.ok(), await res.text()).toBeTruthy();
}

/** What the TV's "Ordem" card drew, measured in the page. */
export interface OrderFit {
  /** The rows drawn (not `display: none`). */
  drawn: number;
  /** The last drawn row's bottom edge is inside the card. */
  lastInsideCard: boolean;
  /** The text of the "+N" slot. */
  more: string;
  /** Rows hidden while the "+N" slot is empty: none are allowed. */
  hiddenWhileEmpty: number;
  /** Showing the first hidden row again puts it past the list's bottom edge (true when nothing is hidden): the "+N" counts rows that cannot be drawn. */
  hiddenOneDoesNotFit: boolean;
}

/** Measures the order card and, for a moment, un-hides its first hidden row; the row is hidden again before the call returns. */
export function inspectOrder(tv: Page): Promise<OrderFit> {
  return tv.evaluate(() => {
    const card = document.querySelector('.tv-order') as HTMLElement;
    const list = card.querySelector('.tv-order__list') as HTMLElement;
    const rows = [...list.querySelectorAll('.tv-order__row')] as HTMLElement[];
    const hidden = rows.filter((row) => getComputedStyle(row).display === 'none');
    const drawn = rows.filter((row) => !hidden.includes(row));
    const more = (card.querySelector('[data-testid="tv-order-more"]')?.textContent ?? '').trim();
    const last = drawn[drawn.length - 1];
    let hiddenOneDoesNotFit = true;
    const first = hidden[0];
    if (first) {
      const was = first.style.display;
      first.style.removeProperty('display');
      hiddenOneDoesNotFit = first.getBoundingClientRect().bottom > list.getBoundingClientRect().bottom + 2;
      first.style.display = was;
    }
    return {
      drawn: drawn.length,
      lastInsideCard: last.getBoundingClientRect().bottom <= card.getBoundingClientRect().bottom + 0.5,
      more,
      hiddenWhileEmpty: more === '' ? hidden.length : 0,
      hiddenOneDoesNotFit,
    };
  });
}

/** The scene card's and the stage column's boxes on the TV. */
export function sceneBoxes(tv: Page): Promise<{ whole: number; text: number }> {
  return tv.evaluate(() => {
    const scene = document.querySelector('.tv-scene') as HTMLElement;
    const style = getComputedStyle(scene);
    return {
      // The room inside the scene's own padding.
      whole: scene.clientWidth - parseFloat(style.paddingLeft) - parseFloat(style.paddingRight),
      text: (document.querySelector('.tv-scene__text') as HTMLElement).getBoundingClientRect().width,
    };
  });
}

/** A combat of the party and the Capitão with `goblins` Goblin 1 copies on the fog map: more rows than a short TV can draw. */
export async function beginCrowdedCombat(master: Page, table: FogTable, goblins: number): Promise<void> {
  const combatTable = { campaignId: table.campaignId, captainId: table.captainId, goblinId: table.goblin1Id } as unknown as CombatTable;
  let enc = await startEncounterRPC(master, combatTable, [
    { characterId: table.captainId, count: 1, hidden: false },
    { characterId: table.goblin1Id, count: goblins, hidden: false },
  ]);
  for (const c of enc.combatants) {
    const face = c.label === 'Pensantus' ? 20 : c.label === 'Toren' ? 15 : 2;
    enc = await combatRPC(master, 'SubmitInitiative', { campaignId: table.campaignId, encounterId: enc.id, combatantId: c.id, d20Face: face });
  }
  const goblinRows = enc.combatants.filter((c) => c.label.startsWith('Goblin'));
  const at: [string, number, number][] = [['Capitão Goblin', 21, 3], ...goblinRows.map((c, i): [string, number, number] => [c.label, 17 + (i % 5), 4 + Math.floor(i / 5)])];
  for (const [label, col, row] of at) {
    const id = enc.combatants.find((c) => c.label === label)!.id;
    enc = await combatRPC(master, 'MoveCombatant', { campaignId: table.campaignId, encounterId: enc.id, combatantId: id, col, row });
  }
  await combatRPC(master, 'BeginCombat', { campaignId: table.campaignId, encounterId: enc.id });
}

/**
 * The room around the speaking NPC's token inside the list that clips it, on each side, after the 3 px ring and its 2 px offset
 * (`combatant-token.scss`): a side under 0 is a ring cut by the list.
 */
export function ringRoom(tv: Page): Promise<{ left: number; top: number; right: number; bottom: number }> {
  return tv.evaluate(() => {
    const ring = 5;
    const list = (document.querySelector('.tv-scene__list') as HTMLElement).getBoundingClientRect();
    const token = (document.querySelector('.tv-scene__npc--speaking app-combatant-token') as HTMLElement).getBoundingClientRect();
    return {
      left: token.left - ring - list.left,
      top: token.top - ring - list.top,
      right: list.right - (token.right + ring),
      bottom: list.bottom - (token.bottom + ring),
    };
  });
}
