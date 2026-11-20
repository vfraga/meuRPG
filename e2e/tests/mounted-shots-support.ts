import { mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, type Browser, type Locator, type Page } from '@playwright/test';

import { castSheet, openCastOf } from './casting-support';
import { combatRPC, endTurnOf, getEncounterRPC, passTurnsToAndWait, type Encounter } from './combat-support';
import { openOrderRow, openTurn } from './combat-layout';
import { closeSheet, contestRPC, idOf } from './contests-support';
import { openSessionPage } from './live-session-support';
import { beginMountedCombat, castSteedRPC, isoldaHalfling, mountedTable, STEED, type MountedTable } from './mounted-support';
import { tapSquare } from './move-support';
import { waitForEncounterRevision } from './revision-support';
import { openActionTab, openCombatLog } from './support';

// The screens of mounted combat and Convocar Montaria for a11y.spec.ts: each state is judged by `scan` at one width and
// theme, and photographed when `E2E_SHOTS_DIR` names a folder (a normal run writes nothing).

const shotsDir = process.env.E2E_SHOTS_DIR;

/** The device scale of a context that takes screenshots: 2 when they are asked for, else the browser's own. */
export const mountedShotScale = shotsDir ? { deviceScaleFactor: 2 } : {};

export const MOUNTED_VIEWPORTS: Record<number, number> = { 320: 700, 390: 844, 768: 1024, 1280: 800 };

export type Scan = (page: Page, screen: string) => Promise<void>;

interface Look {
  scheme: 'light' | 'dark';
  width: number;
  scan: Scan;
}

const fileOf = (screen: string, width: number | 'crop', scheme: 'light' | 'dark'): string => {
  mkdirSync(shotsDir!, { recursive: true });
  return join(shotsDir!, `588-${screen}-${width}-${scheme === 'light' ? 'claro' : 'escuro'}.png`);
};

/** Saves the viewport as `588-<screen>-<width>-<claro|escuro>.png`; `at` is brought into view first (centered, or at the top when it is tall). */
async function shoot(page: Page, screen: string, l: Look, at?: Locator, width = l.width): Promise<void> {
  if (!shotsDir) {
    return;
  }
  if (at) {
    await at.evaluate((el) => el.scrollIntoView({ block: el.getBoundingClientRect().height < innerHeight * 0.8 ? 'center' : 'start' }));
  }
  await page.screenshot({ path: fileOf(screen, width, l.scheme) });
}

/** A close-up of one element, as `588-<name>-crop-<claro|escuro>.png`. */
async function crop(at: Locator, name: string, l: Look): Promise<void> {
  if (!shotsDir) {
    return;
  }
  await at.scrollIntoViewIfNeeded();
  await at.screenshot({ path: fileOf(`${name}-${l.width}`, 'crop', l.scheme) });
}

/** Judges the screen and photographs it; the page must not scroll sideways. */
async function see(page: Page, screen: string, l: Look, at?: Locator): Promise<void> {
  const where = `Montaria, ${screen} (${l.scheme}, ${l.width}px)`;
  await l.scan(page, where);
  const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(wide, `${where}: a página rola de lado em ${wide}px`).toBeLessThanOrEqual(0);
  await shoot(page, screen, l, at);
}

/** Every child of `box` that is drawn lies inside it, side to side: nothing is cut or pushed out (the three pair buttons at 320). */
async function expectInside(box: Locator, items: Locator, what: string): Promise<void> {
  const outer = await box.evaluate((el) => el.getBoundingClientRect().toJSON() as { left: number; right: number });
  const inner = await items.evaluateAll((all) => all.map((el) => ({ left: el.getBoundingClientRect().left, right: el.getBoundingClientRect().right, cut: el.scrollWidth - el.clientWidth })));
  expect(inner.length, `${what}: nothing to measure`).toBeGreaterThan(0);
  for (const [i, b] of inner.entries()) {
    expect(b.left, `${what}, item ${i + 1}: left`).toBeGreaterThanOrEqual(outer.left - 0.5);
    expect(b.right, `${what}, item ${i + 1}: right`).toBeLessThanOrEqual(outer.right + 0.5);
    expect(b.cut, `${what}, item ${i + 1}: its text is cut by ${b.cut}px`).toBeLessThanOrEqual(1);
  }
}

/** The summon sheet, whichever container it is in (a dialog from a tablet up, a bottom sheet on a phone). */
const summonSheet = (p: Page): Locator => p.locator('app-summon-sheet');

/** Convocar Montaria out of combat, the steed's card, its questions and the row of a dismissed steed. */
async function steedScreens(t: MountedTable, l: Look): Promise<void> {
  const { p, campaignId, table } = t;
  await openSessionPage(p, campaignId);
  const cast = await openCastOf(p, 'Convocar Montaria');
  await cast.getByRole('button', { name: 'Escolher a criatura' }).click();
  const summon = summonSheet(p);
  await expect(summon.getByText('Escolha a forma e o tipo do corcel.')).toBeVisible();
  await see(p, 'convocar-vazio', l);
  await summon.locator('label', { hasText: STEED }).click();
  await summon.getByRole('radiogroup', { name: 'Tipo do corcel' }).getByText('Celestial').click();
  await expect(summon.getByRole('button', { name: `Convocar ${STEED}` })).toBeEnabled();
  await see(p, 'convocar-primeira', l);
  await summon.getByRole('button', { name: `Convocar ${STEED}` }).click();
  await expect(summon).toBeHidden();
  if (l.width >= 768) {
    await expect(castSheet(p).getByText(new RegExp(`${STEED} chegou\\.`))).toBeVisible();
    await see(p, 'convocar-resultado', l);
  }
  // On a phone the summon sheet takes the place of the cast sheet (one bottom sheet at a time), so its result line is not
  // drawn there: as with Convocar Familiar, the phone reads the spent slot under it.

  // The steed on the sheet, under "Criaturas".
  await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
  const panel = p.locator('app-creatures-panel');
  const card = panel.locator('app-creature-card');
  await expect(card.getByRole('heading', { name: STEED })).toBeVisible();
  await expectInside(card.locator('.card'), card.locator('.head .mr-tag'), `a etiqueta "Montaria vinculada" no cartão (${l.scheme}, ${l.width}px)`);
  await see(p, 'cartao', l, card);
  await crop(card, 'cartao', l);
  await card.getByRole('button', { name: `Mais ações de ${STEED}` }).click();
  await expect(p.getByRole('menuitem', { name: new RegExp(`Libertar ${STEED}`) })).toBeVisible();
  await see(p, 'cartao-menu', l);
  await p.getByRole('menuitem', { name: new RegExp(`Libertar ${STEED}`) }).click();
  const ask = card.locator('app-steed-ask');
  await expect(ask.getByText(`Libertar ${STEED}?`)).toBeVisible();
  await see(p, 'libertar', l, ask);
  await ask.getByRole('button', { name: 'Voltar' }).click();
  await card.getByRole('button', { name: /Dispensar/ }).click();
  await expect(ask.getByText(`Dispensar ${STEED}?`)).toBeVisible();
  await see(p, 'dispensar', l, ask);
  // The server does not answer: the error stays in the question, in its live region, with both buttons back.
  const dismissCall = '**/meurpg.characters.v1.CharacterService/DismissCreature';
  await p.route(dismissCall, (r) => r.fulfill({ status: 503, contentType: 'application/json', body:'{"code":"unavailable","message":"x"}' }));
  await ask.getByRole('button', { name: new RegExp(`Dispensar\\s+${STEED}`) }).click();
  await expect(ask.locator('.mr-live[role=alert] .err')).toBeVisible();
  await expect(ask.getByRole('button', { name: 'Voltar' })).toBeEnabled();
  await see(p, 'dispensar-erro', l, ask);
  await p.unroute(dismissCall);
  await ask.getByRole('button', { name: new RegExp(`Dispensar\\s+${STEED}`) }).click();

  // Dismissed, the bond stays: the row "fora da mesa" and the same steed in the sheet of the next cast.
  const away = panel.locator('app-steed-away');
  await expect(away.getByRole('heading', { name: STEED })).toBeVisible();
  await see(p, 'fora', l, away);
  await crop(away, 'fora', l);
  await panel.getByRole('button', { name: /Convocar Montaria/ }).first().click();
  const again = summonSheet(p);
  await expect(again.getByText('Mesmo corcel')).toBeVisible();
  await expect(again.getByRole('button', { name: `Convocar ${STEED}` })).toBeEnabled();
  await see(p, 'convocar-mesmo', l);
  await again.getByRole('button', { name: `Convocar ${STEED}` }).click();
  await expect(card.getByRole('heading', { name: STEED })).toBeVisible();
}

/** The goblin's shove on the horse, through the API: the master's dice are not what these screens show. */
async function shoveSteedRPC(t: MountedTable, enc: Encounter, outcome: 'SHOVE_OUTCOME_PUSH' | 'SHOVE_OUTCOME_PRONE', waitForSheet = false): Promise<void> {
  const { m, p, campaignId } = t;
  const started = await contestRPC(m, 'StartContest', {
    campaignId,
    encounterId: enc.id,
    initiatorId: idOf(enc, 'Goblin 1'),
    targetId: idOf(enc, STEED),
    purpose: 'CONTEST_PURPOSE_SHOVE',
    roll: { d20Faces: { faces: [20] } },
  });
  const contestId = (started.contest as { id: string }).id;
  if (waitForSheet) {
    // From a tablet up the contest sheet opens on the player's screen when the shove starts; answering before it is there would leave it out.
    await expect(p.getByRole('dialog', { name: /Goblin 1 tenta empurrar/ })).toBeVisible();
  }
  await contestRPC(m, 'RespondContest', { campaignId, encounterId: enc.id, contestId, skill: 'CONTEST_SKILL_ATHLETICS', roll: { d20Faces: { faces: [1] } } });
  await contestRPC(m, 'ResolveShove', { campaignId, encounterId: enc.id, contestId, outcome });
}

/**
 * The combat: the master's "Montaria" select, the "Montaria" panel of "Mover", the pair's card, token
 * and row, riding and picking where to get off, and the save to stay on with the master's wait, its result and the log.
 */
async function rideScreens(t: MountedTable, l: Look): Promise<void> {
  const { m, p, campaignId } = t;
  let enc = await beginMountedCombat(t);
  await openSessionPage(m, campaignId);
  await openSessionPage(p, campaignId);
  await waitForEncounterRevision([m, p], enc);

  // The master marks a creature as a mount: "Mais" › "Montaria…" in its opened row.
  await openOrderRow(m, 'Goblin 2');
  await m.getByRole('button', { name: 'Mais ações para Goblin 2' }).click();
  await m.getByRole('menuitem', { name: /Montaria…/ }).click();
  const select = m.getByRole('combobox', { name: 'Montaria: Goblin 2' });
  await expect(select).toBeVisible();
  await see(m, 'mestre-montaria', l, select);
  await select.click();
  const options = m.getByRole('listbox');
  await expect(options.getByRole('option').first()).toBeVisible();
  await see(m, 'mestre-montaria-opcoes', l);
  await m.keyboard.press('Escape');
  await expect(options).toBeHidden();
  await openTurn(m);

  // Isolda's turn: "Mover" lists the horse beside her.
  await expect(p.getByRole('heading', { name: 'Sua vez, Isolda' })).toBeVisible();
  await p.getByRole('button', { name: 'Mover', exact: true }).click();
  const panel = p.getByRole('region', { name: 'Montaria' });
  await expect(panel.getByRole('button', { name: 'Montar' })).toBeVisible();
  await see(p, 'mover-montaria', l, panel);

  // A mount that cannot be mounted now: the master puts the horse four squares away. Its button stays, dashed and off, with the reason linked.
  const horse = idOf(enc, STEED);
  await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId: horse, col: 6, row: 11, forced: true });
  const row = panel.getByRole('listitem').filter({ hasText: STEED });
  await expect(row.getByText(/chegue ao lado/)).toBeVisible();
  const refusedGo = row.getByRole('button', { name: 'Montar' });
  await expect(refusedGo).toBeDisabled();
  await expect(refusedGo).toHaveAttribute('aria-describedby', /mp-line-/);
  await see(p, 'mover-recusa', l, panel);
  await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId: horse, col: 6, row: 7, forced: true });
  await expect(row.getByText('Ao lado')).toBeVisible();
  await panel.getByRole('button', { name: 'Montar' }).click();
  await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === 'Isolda')?.mountedOnId).toBe(idOf(enc, STEED));

  // Mounted: "Cavalgar" is on, and "Desmontar" says why it waits (she got on in this move).
  const modes = p.getByRole('radiogroup').filter({ hasText: 'Cavalgar' });
  await expect(modes).toBeVisible();
  await expect(p.getByText('Você já montou ou desmontou neste movimento.')).toBeVisible();
  await see(p, 'mover-montado', l, modes);
  await tapSquare(p, 9, 7);
  await expect(p.getByRole('button', { name: 'Mover para cá' })).toBeVisible();
  await see(p, 'mover-cavalgar', l);
  await p.getByRole('button', { name: 'Mover para cá' }).click();
  await expect.poll(async () => (await getEncounterRPC(m, campaignId)).combatants.find((c) => c.label === STEED)?.col).toBe(9);

  // The pair's card in her turn: the horse's three actions, which fit at 320 too.
  const pair = p.locator('app-pair-card');
  await expect(pair.getByRole('button', { name: `Disparada do ${STEED}` })).toBeVisible();
  await see(p, 'par-turno', l, pair);
  await expectInside(pair.locator('.pair'), pair.locator('.pair__act'), `os três botões do par (${l.scheme}, ${l.width}px)`);
  await crop(pair, 'par-cartao', l);
  if (l.width === 390) {
    await p.setViewportSize({ width: 320, height: MOUNTED_VIEWPORTS[320] });
    await expect(pair.getByRole('button', { name: `Esquivar do ${STEED}` })).toBeVisible();
    await l.scan(p, `Montaria, par-turno (${l.scheme}, 320px)`);
    await expectInside(pair.locator('.pair'), pair.locator('.pair__act'), `os três botões do par (${l.scheme}, 320px)`);
    await shoot(p, 'par-turno', l, pair, 320);
    await p.setViewportSize({ width: 390, height: MOUNTED_VIEWPORTS[390] });
  }

  // The pair on the master's screen: one token with the rider's seal on the map, and the mount's row under the rider's.
  const order = m.locator('app-order-list');
  const orderTab = m.getByRole('tab', { name: /^Ordem/ });
  if (await orderTab.isVisible()) {
    await orderTab.click();
  }
  await expect(order.getByText(STEED).first()).toBeVisible();
  await see(m, 'mestre-ordem', l, order);
  await crop(order, 'mestre-ordem', l);
  const mapTab = m.getByRole('tab', { name: /^Mapa/ });
  if (await mapTab.isVisible()) {
    await mapTab.click();
  }
  const rider = m.locator('app-combat-map .cm__rider').first();
  await expect(rider).toBeVisible();
  await see(m, 'mestre-mapa', l, rider);
  await crop(rider.locator('xpath=..'), 'mapa-par', l);
  await openTurn(m);

  // Goblin 1 pushes the horse: the save to stay on, on her screen, and the master's wait.
  await endTurnOf(p, m, campaignId, 'Isolda');
  enc = await passTurnsToAndWait(m, campaignId, 'Goblin 1', [m, p]);
  await shoveSteedRPC(t, enc, 'SHOVE_OUTCOME_PUSH', l.width >= 768);
  const save = p.getByRole('alertdialog', { name: /Você cai da montaria\?/ });
  await expect(save.getByText(`O ${STEED} sofreu um empurrão.`)).toBeVisible();
  await see(p, 'teste', l);
  const wait = m.getByTestId('mount-save-wait');
  await expect(wait).toBeVisible();
  await see(m, 'mestre-espera', l, wait);
  await save.getByRole('button', { name: 'Digitar o resultado' }).click();
  await save.getByLabel('Face do d20 (1 a 20)').fill('4');
  await see(p, 'teste-digitado', l);
  await save.getByRole('button', { name: /^Confirmar/ }).click();
  await expect(save.locator('.res')).toContainText('4: você caiu Derrubado');
  await see(p, 'teste-resultado', l);
  await closeSheet(save);
  // From a tablet up the shove's own sheet is under it (the horse is the player's); a phone holds one sheet at a time.
  if (l.width >= 768) {
    const shove = p.getByRole('dialog', { name: /Goblin 1 tenta empurrar/ });
    await expect(shove).toBeVisible();
    await closeSheet(shove);
  }
  await expect(p.getByText('Derrubado').first()).toBeVisible();
  const log = await openCombatLog(p);
  await expect(log.getByText(/caiu/).first()).toBeVisible();
  await see(p, 'registro', l, log);

  // Out of her turn the card's "Dispensar" is refused (it spends the action): off, with the reason linked under the actions.
  await p.goto(`/campaigns/${campaignId}/characters/${t.table.characterId}`);
  const card = p.locator('app-creatures-panel app-creature-card');
  await expect(card.getByRole('heading', { name: STEED })).toBeVisible();
  await card.getByRole('button', { name: /Dispensar/ }).click();
  const ask = card.locator('app-steed-ask');
  await ask.getByRole('button', { name: new RegExp(`Dispensar\\s+${STEED}`) }).click();
  const off = card.getByRole('button', { name: /Dispensar/ });
  await expect(off).toHaveAttribute('aria-disabled', 'true');
  await expect(off).toHaveAttribute('aria-describedby', /steed-why-/);
  await expect(card.getByText(/Só na vez de .*: dispensar gasta a ação\./)).toBeVisible();
  await see(p, 'cartao-recusa', l, card);
}

/**
 * A second table, mounted from the start, with a halfling rider: the save's held-roll question and its "no longer owed" state,
 * "também" on a spell of the rider's own and the reaction that gets her off on her feet when the horse is knocked prone.
 */
async function riderScreens(t: MountedTable, l: Look): Promise<void> {
  const { m, p, campaignId, table } = t;
  await castSteedRPC(p, table);
  let enc = await beginMountedCombat(t);
  enc = await combatRPC(p, 'MountCombatant', { campaignId, encounterId: enc.id, riderId: idOf(enc, 'Isolda'), mountId: idOf(enc, STEED) });
  await openSessionPage(m, campaignId);
  await openSessionPage(p, campaignId);
  await waitForEncounterRevision([m, p], enc);
  await expect(p.getByRole('heading', { name: 'Sua vez, Isolda' })).toBeVisible();

  // Goblin 1 pushes the horse, and Isolda (a halfling) types a 1: Lucky's question comes before the result, in the save's sheet.
  await endTurnOf(p, m, campaignId, 'Isolda');
  enc = await passTurnsToAndWait(m, campaignId, 'Goblin 1', [m, p]);
  await shoveSteedRPC(t, enc, 'SHOVE_OUTCOME_PUSH', l.width >= 768);
  const held = p.getByRole('alertdialog', { name: /Você cai da montaria\?/ });
  await held.getByRole('button', { name: 'Digitar o resultado' }).click();
  await held.getByLabel('Face do d20 (1 a 20)').fill('1');
  await held.getByRole('button', { name: /^Confirmar/ }).click();
  await expect(held.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeVisible();
  await see(p, 'teste-pergunta', l);
  // The master answers for her: a new d20 of 15 keeps her on the horse, and her sheet says the save is no longer hers to roll.
  const wait = m.getByTestId('mount-save-wait');
  await expect(wait).toBeVisible();
  await wait.getByRole('button', { name: 'Rolar por Isolda' }).click();
  const answer = m.locator('app-mount-save-sheet');
  // The server lets the master answer only after the player's first seconds: the sheet counts them down and keeps its buttons off.
  const typeIt = answer.getByRole('button', { name: 'Digitar o resultado' });
  await expect(typeIt).toBeEnabled({ timeout: 30_000 });
  await typeIt.click();
  await answer.getByLabel(/Role 1d20 de novo/).fill('15');
  await answer.getByRole('button', { name: 'Confirmar 15' }).click();
  await expect(held.getByText('Este teste não espera mais a sua rolagem: o mestre rolou, ou ele deixou de ser preciso.')).toBeVisible();
  await see(p, 'teste-encerrado', l);
  await closeSheet(held);
  await closeSheet(answer);
  if (l.width >= 768) {
    // The shove's own sheet is under it from a tablet up.
    await closeSheet(p.getByRole('dialog', { name: /Goblin 1 tenta empurrar/ }));
  }
  await passTurnsToAndWait(m, campaignId, 'Isolda', [m, p]);
  await expect(p.getByRole('heading', { name: 'Sua vez, Isolda' })).toBeVisible();

  await openActionTab(p, 'Magias');
  await p.getByRole('button', { name: 'Conjurar Auxílio Divino' }).click();
  const sheet = p.getByRole('dialog');
  const also = sheet.getByRole('checkbox', { name: new RegExp(`${STEED} também`) });
  // On from the start: the caster takes it off, never has to find it.
  await expect(also).toBeChecked();
  await see(p, 'conjurar-tambem', l, sheet.locator('app-steed-also'));
  await sheet.getByRole('button', { name: /^Conjurar/ }).last().click();
  await expect(sheet.locator('.steed')).toBeVisible();
  await see(p, 'conjurar-tambem-resultado', l);
  await closeSheet(sheet);

  // The next round: "Desmontar" shows the squares beside the horse.
  await endTurnOf(p, m, campaignId, 'Isolda');
  await passTurnsToAndWait(m, campaignId, 'Isolda', [m, p]);
  await p.getByRole('button', { name: 'Mover', exact: true }).click();
  const modes = p.getByRole('radiogroup').filter({ hasText: 'Cavalgar' });
  await modes.getByText('Desmontar').click();
  await expect(p.getByText(/Desmontar custa/)).toBeVisible();
  await see(p, 'mover-desmontar', l);
  await p.getByRole('button', { name: 'Cancelar' }).click();

  await endTurnOf(p, m, campaignId, 'Isolda');
  enc = await passTurnsToAndWait(m, campaignId, 'Goblin 1', [m, p]);
  await shoveSteedRPC(t, enc, 'SHOVE_OUTCOME_PRONE');
  const off = p.getByRole('alertdialog').filter({ has: p.getByRole('heading', { name: 'A montaria caiu' }) });
  await expect(off.getByRole('button', { name: 'Desmontar de pé' })).toBeVisible();
  await see(p, 'desmontar-reacao', l);
  await see(m, 'mestre-espera-reacao', l);
  await off.getByRole('button', { name: 'Desmontar de pé' }).click();
  await expect(off).toBeHidden();
}

/** The screens of one theme and width, in the order the table gives them. */
export async function mountedScreens(browser: Browser, scheme: 'light' | 'dark', width: number, scan: Scan): Promise<void> {
  const l: Look = { scheme, width, scan };
  const t = await mountedTable(browser, 'Montaria telas', { colorScheme: scheme, masterWidth: width, playerWidth: width, ...mountedShotScale });
  try {
    await t.m.setViewportSize({ width, height: MOUNTED_VIEWPORTS[width] ?? 800 });
    await t.p.setViewportSize({ width, height: MOUNTED_VIEWPORTS[width] ?? 800 });
    await steedScreens(t, l);
    await rideScreens(t, l);
  } finally {
    await t.done().catch(() => undefined);
  }
  const again = await mountedTable(browser, 'Montaria telas 2', { colorScheme: scheme, masterWidth: width, playerWidth: width, build: isoldaHalfling, ...mountedShotScale });
  try {
    await again.m.setViewportSize({ width, height: MOUNTED_VIEWPORTS[width] ?? 800 });
    await again.p.setViewportSize({ width, height: MOUNTED_VIEWPORTS[width] ?? 800 });
    await riderScreens(again, l);
  } finally {
    await again.done().catch(() => undefined);
  }
}
