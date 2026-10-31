import { expect, type Locator, type Page } from '@playwright/test';

import { callRPC, characterRpcBody, createCharacterRPC, pensantus } from './support';

// Setup for the live-session specs (Etapa 5, MR-011, MR-012, RN-02), through
// the API: these tests prove the session screens, not the campaign, invite
// and character forms other specs already cover. Every test makes its own
// campaign, so sessions never collide with another test's.

export interface LiveTable {
  campaignId: string;
  campaignName: string;
  characterId: string;
}

/**
 * A campaign of Mestre Teste's with Jogador Teste in it, playing Pensantus
 * (23 HP, Mago 3). `masterPage` and `playerPage` must be signed in as each
 * (their saved states); neither navigates.
 */
export async function tableWithPensantus(masterPage: Page, playerPage: Page, name: string): Promise<LiveTable> {
  const created = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
    name,
    xpMode: 'XP_MODE_ENEMIES',
  });
  expect(created.ok()).toBeTruthy();
  const campaignId = (await created.json()).campaign.id as string;

  const invite = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
    campaignId,
    maxUses: 1,
    expiresIn: '3600s',
  });
  expect(invite.ok()).toBeTruthy();
  const accepted = await callRPC(playerPage, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', {
    token: (await invite.json()).token,
  });
  expect(accepted.ok()).toBeTruthy();

  const character = await createCharacterRPC(playerPage, campaignId, characterRpcBody('PLAYER', pensantus));
  expect(character.ok()).toBeTruthy();
  return { campaignId, campaignName: name, characterId: (await character.json()).character.id };
}

/** Starts the campaign's session through the API; returns its ID. */
export async function startSessionRPC(masterPage: Page, campaignId: string): Promise<string> {
  const res = await callRPC(masterPage, 'meurpg.play.v1.PlayService/StartGameSession', { campaignId });
  expect(res.ok()).toBeTruthy();
  return (await res.json()).gameSession.id as string;
}

/**
 * Ends whatever session the campaign has open, if any: tests clean up after
 * themselves, so Jogador Teste doesn't pile up open sessions (each one is a
 * notice and an "Ao vivo" link on every screen of the shared account).
 */
export async function endOpenSessionRPC(masterPage: Page, campaignId: string): Promise<void> {
  const res = await callRPC(masterPage, 'meurpg.play.v1.PlayService/ListGameSessions', { campaignId });
  if (!res.ok()) {
    return;
  }
  const sessions = ((await res.json()).gameSessions ?? []) as { id: string; endedAt?: string }[];
  for (const s of sessions.filter((s) => !s.endedAt)) {
    await endSessionRPC(masterPage, campaignId, s.id);
  }
}

/** Ends the campaign's session through the API. */
export async function endSessionRPC(masterPage: Page, campaignId: string, gameSessionId: string): Promise<void> {
  const res = await callRPC(masterPage, 'meurpg.play.v1.PlayService/EndGameSession', { campaignId, gameSessionId });
  expect(res.ok()).toBeTruthy();
}

/**
 * Opens a session page and waits for it to be live. Not `networkidle`: the
 * page keeps its live stream open, so the network is never idle. Up to 30 s:
 * going live takes a few calls in a row (the sign-in check, the stream's
 * `ready`, the snapshot), and on a busy shared stack each can take seconds.
 */
export async function openSessionPage(page: Page, campaignId: string, tab: MasterTab = 'cena'): Promise<void> {
  // The master's board has the tabs "Cena", "Mapa" and "Grupo" (`?vista=`); the address picks the one the test works in. A player's page ignores it.
  await page.goto(`/campaigns/${campaignId}/session?vista=${tab}`);
  // The bar's "Ao vivo" pill leaves it under 360 px, so the title "Sessão 3" (there at every width) counts too.
  const live = page.getByText('Ao vivo', { exact: true }).or(page.getByRole('heading', { level: 1, name: /^Sessão \d+$/ }));
  await expect(live.first()).toBeVisible({ timeout: 30_000 });
}

/**
 * Opens `route` and waits for its heading (`heading`, else the h1) to be visible: the page has started to render, not
 * that it has finished loading. A page that keeps its stream open never goes idle, so `networkidle` never comes.
 * `timeout` is the expect default unless a page needs more.
 */
export async function openLive(page: Page, route: string, heading?: string, timeout?: number): Promise<void> {
  await page.goto(route);
  await expect(heading ? page.getByRole('heading', { name: heading }) : page.getByRole('heading', { level: 1 })).toBeVisible({ timeout });
}

/**
 * Waits for the notice of `campaignName`'s session 1 on the player's open
 * page and returns its "Entrar na sessão" link. Other tests start sessions
 * at the same time, so the page may show several notices (one per open
 * session, oldest first): the link's accessible name carries the campaign,
 * which tells ours apart. The poll runs every 30 s, so allow for one full
 * wait.
 */
export async function waitForNotice(page: Page, campaignName: string, timeoutMs = 45_000): Promise<Locator> {
  const link = page.getByRole('link', { name: `Entrar na sessão 1 de ${campaignName}`, exact: true });
  await expect(page.getByRole('status').getByText(`A sessão 1 de ${campaignName} começou.`)).toBeVisible({
    timeout: timeoutMs,
  });
  await expect(link).toBeVisible();
  return link;
}

/** A phone's session bar keeps "Iniciar combate" in the "Sessão" menu; a computer's has it as a button. */
const onPhone = (page: Page): boolean => (page.viewportSize()?.width ?? Number.POSITIVE_INFINITY) < 768;

/** Opens the master's "Sessão" menu of the session bar. */
export async function openSessionMenu(page: Page): Promise<void> {
  await page.getByRole('button', { name: 'Sessão', exact: true }).click();
}

/** "Iniciar combate" of the session bar, wherever the screen keeps it: opens the dialog. */
export async function clickStartCombat(page: Page): Promise<void> {
  if (onPhone(page)) {
    await openSessionMenu(page);
    await page.getByRole('menuitem', { name: 'Iniciar combate' }).click();
    return;
  }
  await page.getByRole('button', { name: 'Iniciar combate' }).click();
}

/** The master is offered "Iniciar combate" (no combat is on). */
export async function expectStartCombatOffered(page: Page): Promise<void> {
  if (onPhone(page)) {
    await openSessionMenu(page);
    await expect(page.getByRole('menuitem', { name: 'Iniciar combate' })).toBeVisible();
    await page.keyboard.press('Escape');
    return;
  }
  await expect(page.getByRole('button', { name: 'Iniciar combate' })).toBeVisible();
}

/** "Encerrar sessão" from the "Sessão" menu, then "Confirmar encerramento" in place under the bar. */
export async function endSessionFromBar(page: Page): Promise<void> {
  await openSessionMenu(page);
  await page.getByRole('menuitem', { name: 'Encerrar sessão' }).click();
  await page.getByRole('button', { name: 'Confirmar encerramento' }).click();
}

/** The tabs of the master's board outside a combat (C3, C6-05, C6-06). */
export type MasterTab = 'cena' | 'mapa' | 'grupo';

const TAB_NAMES: Record<MasterTab, RegExp> = { cena: /^Cena\b/, mapa: /^Mapa\b/, grupo: /^Grupo\b/ };

/**
 * Shows a tab of the master's board: the tab list on a computer; on a phone the pinned nav ("Cena" and "Grupo") and, for the map, the fold
 * of the scene's page. The panels of the other tabs stay mounted but hidden, so what a test reaches must be on the tab it shows.
 */
export async function showMasterTab(page: Page, tab: MasterTab): Promise<void> {
  if (onPhone(page)) {
    if (tab === 'mapa') {
      await showMasterTab(page, 'cena');
      const fold = page.getByRole('button', { name: /^Mapa ·/ });
      if ((await fold.getAttribute('aria-expanded')) !== 'true') {
        await fold.click();
      }
      return;
    }
    const nav = page.getByRole('navigation', { name: 'Seções da sessão' });
    const button = nav.getByRole('button', { name: TAB_NAMES[tab] });
    await button.click();
    await expect(button).toHaveAttribute('aria-current', 'true');
    return;
  }
  const button = page.getByRole('tab', { name: TAB_NAMES[tab] });
  if ((await button.getAttribute('aria-selected')) !== 'true') {
    await button.click();
  }
  await expect(button).toHaveAttribute('aria-selected', 'true');
}

/** The tools under the master's map: "Pontos", "Tokens", "Névoa e luz", "Armadilhas", "Tesouro" and "Encontros". Shows the "Mapa" tab first. */
export type MapTool = 'Pontos' | 'Tokens' | 'Névoa e luz' | 'Armadilhas' | 'Tesouro' | 'Encontros';

export async function showMapTool(page: Page, tool: MapTool): Promise<void> {
  await showMasterTab(page, 'mapa');
  const tab = page.getByRole('tablist', { name: 'Ferramentas do mapa' }).getByRole('tab', { name: new RegExp(`^${tool}\\b`) });
  if ((await tab.getAttribute('aria-selected')) !== 'true') {
    await tab.click();
  }
  await expect(tab).toHaveAttribute('aria-selected', 'true');
}

/**
 * Opens every roll of the open scene on the master's "Cena" ("Ações e rolagens": each roll is a chip that opens its detail under the
 * action) and returns the opened rolls, each action's newest first. `expectedRolls` is the exact number of rolls the scene has: they
 * reach this page by the stream, so it waits for that many chips before it opens any.
 */
export async function openRolls(page: Page, expectedRolls: number): Promise<Locator> {
  await showMasterTab(page, 'cena');
  const chips = page.locator('app-scene-actions .chip');
  // The rolls reach this page by the stream, after the RPCs that made them have returned: opening the chips found so far
  // would leave out the one still on its way, so wait for all of them first.
  await expect(chips).toHaveCount(expectedRolls);
  for (let i = 0; i < expectedRolls; i++) {
    const chip = chips.nth(i);
    if ((await chip.getAttribute('aria-expanded')) === 'false') {
      await chip.click();
      await expect(chip).toHaveAttribute('aria-expanded', 'true');
    }
  }
  return page.locator('app-scene-actions app-scene-roll-line');
}

/**
 * "Rolagens da mesa" on the master's page: the log closes the "Cena" tab as a fold, folded at the start; this opens it and returns its region.
 * A player's page draws the section open, so there is nothing to press there.
 */
export async function openTableRolls(page: Page): Promise<Locator> {
  const region = page.getByRole('region', { name: 'Rolagens da mesa' });
  const toggle = region.getByRole('button', { name: /^Rolagens da mesa/ });
  if ((await toggle.count()) > 0 && (await toggle.getAttribute('aria-expanded')) === 'false') {
    await toggle.click();
  }
  return region;
}

/**
 * The player's "Ficha" (C6-08, C6-09), in whatever shape the width gives it: a tab of the rail "Conversa · Ficha" from 1024 px, the
 * "Ficha" of the pinned nav on a phone (a panel inside the session), and a block under the scene between the two (nothing to open).
 * It opens the tab when there is one, and returns the place the numbers are in.
 */
export async function openSheetTab(page: Page): Promise<Locator> {
  const width = page.viewportSize()?.width ?? 1280;
  if (width < 768) {
    await page.getByRole('navigation', { name: 'Seções da sessão' }).getByRole('button', { name: 'Ficha' }).click();
    await expect(page.getByRole('navigation', { name: 'Seções da sessão' }).getByRole('button', { name: 'Ficha' })).toHaveAttribute('aria-current', 'true');
    return page.getByRole('region', { name: 'Ficha', exact: true });
  }
  if (width >= 1024) {
    const tab = railOf(page).getByRole('tab', { name: 'Ficha' });
    await tab.click();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
    return railOf(page).getByRole('tabpanel', { name: 'Ficha' });
  }
  return page.locator('.player-board__side');
}

/** Back to the scene on a phone ("Cena" of the pinned nav), or to "Conversa" in the rail from 1024 px. */
export async function openChatTab(page: Page): Promise<void> {
  const width = page.viewportSize()?.width ?? 1280;
  if (width >= 1024) {
    const tab = railOf(page).getByRole('tab', { name: /^Conversa/ });
    await tab.click();
    await expect(tab).toHaveAttribute('aria-selected', 'true');
  }
}

/** The player's right column from 1024 px: the rail named "Conversa e ficha". */
export function railOf(page: Page): Locator {
  return page.getByRole('complementary', { name: 'Conversa e ficha' });
}

/**
 * The player's whole map, from the session page: on a phone the map there is one row, "Mapa · <nome>" with "Ver mapa" (C5-01), so the
 * fog, its legend and its loading state are on the map page that link opens; from 768 px they are on the session page itself.
 * Call it on a session page already open; it follows the link on a phone and does nothing elsewhere.
 */
export async function followMapLink(page: Page): Promise<void> {
  if (!onPhone(page)) {
    return;
  }
  await page.getByRole('link', { name: 'Ver mapa' }).click();
  await expect(page).toHaveURL(/\/maps\/[^/?]+\?from=session/);
}

/** `goto` of the session's address, then `followMapLink`: the page where the player's whole map is at this width. */
export async function openPlayerMap(page: Page, sessionUrl: string): Promise<void> {
  await page.goto(sessionUrl);
  await followMapLink(page);
}
