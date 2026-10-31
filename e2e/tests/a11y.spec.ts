import AxeBuilder from '@axe-core/playwright';
import { expect, type Browser, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { canvasJpeg, newCampaign, uploadThroughPicker } from './gallery-support';
import { saveDocumentRPC, tableWithDocumentParts } from './document-support';
import { exportThroughScreen, joinAsPlayer, tableForPackage, zipWithManifest } from './campaign-package-support';
import { expectAligned } from './layout';
import { expectLoaded } from './loaded';
import { diceTable, openDice, stepModifier } from './free-dice-support';
import { chooseReaders, lineOf, messageBoxOf, openConversaTable, say } from './conversa-support';
import { followMapLink, openChatTab, openPlayerMap, openSheetTab, endOpenSessionRPC, endSessionRPC, openRolls, openSessionPage, showMapTool, showMasterTab, startSessionRPC, tableWithPensantus, openSessionMenu, clickStartCombat, expectStartCombatOffered, endSessionFromBar } from './live-session-support';
import { canvasPng, createMapRPC, createPointRPC, placeTokenRPC, revealMapRPC, setCurrentMapRPC, tableForMaps, uploadImageRPC } from './maps-support';
import { type Encounter, adjustVitalsRPC, attackCombatSquares, beginAttackCombatRPC, setGridRPC, combatRPC, getEncounterRPC, startEncounterRPC, endTurnOf, passTurnsTo, pensantusCasting, waitTurnLeaves, tableForCombat, toren, torenSheet } from './combat-support';
import { addActionRPC, attachTwoImagesRPC, cartActions, getOpenSceneRPC, openSceneOnlyToRPC, openSceneRPC, rollSceneRPC, sceneActionIdsRPC, setAttemptsRPC, setMayJoinRPC, setSceneImagesRPC, setShowDcRPC, tableForScenes } from './scene-support';
import { addClueRPC, cartClues, cartHooks, createNoteRPC } from './notes-support';
import { createCapitaoRPC, createMiraRPC, playedCombatRPC, putOnStageRPC, uploadPortrait } from './stage-support';
import { printRoute, tableForPrinting } from './print-support';
import { tableWithOpenChoices } from './choices-support';
import { classCard, passClassStep, tableForLevelUp } from './levelup-support';
import { aurora, auroraSheet, beginPlacedCombatRPC, nael, naelSheet, orla, orlaSheet, spendForRestRPC } from './rests-support';
import { castSheet, choiceCard, openCastOf, pickChoice, pickSlotRadio, pickTargetOf, tableForCasting } from './casting-support';
import { paintRPC, pickRadio, tapSquare } from './move-support';
import { beginFogCombat, moveTo, sessionRoute, setCarriedLightRPC, tableForFog } from './fog-support';
import { beginCreatureCombat, hitAndApply, tableForCreatureCombat } from './creatures-combat-support';
import { claimRoute, linkRPC, reservedRPC, revokeLinkRPC, rowOf } from './claim-support';
import { authStatePath, boxOf, callRPC, layoutSize, characterRpcBody, createCharacterRPC, newSignedInContext, pensantus, showAllPicks, signIn, type TestUser, openActionTab, openCombatLog, openFoldedPanel, openSpentGroup, openXpHistory, viaTargetList } from './support';
import { beginJointCombat, endPartRPC, jointTable } from './joint-turn-support';
import { tableForCaster, tableForCreatures } from './creatures-support';
import { grog, pipo, rollsTable, vex } from './combat-rolls-support';
import { alliedTable, closeSheet, contestRPC, idOf, typeD20 } from './contests-support';
import { awardXpRPC, createEnemyRPC, tableForXp, tableForXpCombat, winCombatRPC } from './xp-support';
import { tableForGold, threeTreasuresRPC, treasureFoundRPC } from './gold-support';
import { movePensantus, pensantusFirst, sq20, thirdPlayer, trapRPC, treasureRPC, type TrapTable } from './trap-support';
import { cavePoints, clickSquare, dragSquares, editorRoute, mapToPaint } from './editor-support';
import { campaignWithEmptyPlayer, factor, masterCampaign, method, setTableRulesRPC, wallSquares } from './table-rules-support';
import {
  CIPHER,
  SEQUENCE,
  closePuzzleRPC,
  createCipherRPC,
  createLightsRPC,
  createLockRPC,
  createPillarsRPC,
  createRiddleRPC,
  createSequenceRPC,
  endTable,
  moveRPC,
  playSequenceRPC,
  pickPuzzleAction,
  puzzleRoute,
  releaseHintRPC,
  revealClueRPC,
  sceneClueRPC,
  secondPlayer as puzzlesSecondPlayer,
  setDiceModeRPC,
  showPuzzleRPC,
  solveByThePathRPC,
  tableForPuzzles,
  trapPointRPC,
} from './puzzles-support';
import { createInkBladeRPC, tableForSpells } from './spells-support';
import { beginTheatreRPC, secondPlayer } from './theatre-support';
import { combatWithTwoDown, reviveRPC, tableWithDead } from './revivify-support';
import { brisa, brisaAidSheet, brisaSheet, dalila, dalilaSheet, ragna, ragnaSheet } from './combat-support';
import { archiveEntryRPC, costsRaceBody, createEntryRPC, entryRoute, raceBody, spellBody, updateEntryRPC } from './content-support';
import { generateSceneRPC, mapRoute, tableForImages } from './images-support';
import { treasureRoute } from './treasure-support';
import { classBody, createClassRPC, createSubclassRPC, halfCasterBody } from './classes-support';
import { setSwitchesRPC } from './content-options-support';
import { effectRow, effectsTable, openPanel, setExhaustionRPC } from './effects-support';
import { enterZoneRPC, zoneCard, zoneLayer, zoneOf, zoneTable, zonesPanel } from './zones-support';
import {
  changeGuardianSkillsRPC,
  createGuardianRPC,
  createInkBladeSubclassRPC,
  createOwlRaceRPC,
  createPlainSubclassRPC,
  createSheetRPC,
  emptyTable,
  icaroSheetBody,
  lockAndMilestone,
} from './table-sheet-support';
import { openCombatPanel, openMap, openOrderRow, openTurn } from './combat-layout';
import { waitForEncounterRevision } from './revision-support';

// docs/design/README.md#how-a-screen-is-made: every screen passes axe with no
// serious or critical violation of WCAG 2.1 A and AA, in the light and the
// dark theme (contrast is checked per theme), at desktop and phone widths.
//
// Each test builds its own campaign and NPC through the API, so the screens
// have real content, and reuses the saved sign-in (no /auth/login hit).

const wcag = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa'];

/** Scans the page and fails with one readable line per serious or critical
 * violation (the rule, what it means and the first few elements), then runs
 * the layout checks of layout.ts on the same screen: icons in line with
 * their words, nothing over an icon, tiles centred. */
async function expectScreenPasses(page: Page, screen: string): Promise<void> {
  // A dialog fading in, or a button whose focus ring is still fading, has
  // colours between two states: axe would judge the contrast of a frame nobody
  // stops on. Wait until every animation that ends has ended ("pending" ones,
  // a dialog's first frame, included; a looping one never ends), checked two
  // frames later, so that focus the app moves after rendering, and the
  // transition it starts, are seen too.
  await page.waitForFunction(
    () =>
      new Promise<boolean>((resolve) => {
        requestAnimationFrame(() =>
          requestAnimationFrame(() =>
            resolve(
              document
                .getAnimations()
                .every(
                  (a) =>
                    a.playState === 'finished' || a.playState === 'idle' || a.effect?.getComputedTiming().iterations === Infinity,
                ),
            ),
          ),
        );
      }),
    undefined,
    { timeout: 5_000 },
  );
  const results = await new AxeBuilder({ page }).withTags(wcag).analyze();
  const serious = results.violations
    .filter((v) => v.impact === 'serious' || v.impact === 'critical')
    .map((v) => `${screen}: ${v.id} (${v.impact}) ${v.help}: ${v.nodes.slice(0, 3).map((n) => n.target.join(' ')).join(' | ')}`);
  expect(serious).toEqual([]);
  await expectAligned(page, screen);
}

/** Opens a route and waits for its h1 and for its sections' calls to
 * finish, so axe scans the loaded screen. (A loading state must pass too:
 * a spinner needs an accessible name. This just keeps each scan about one
 * state.) */
async function open(page: Page, route: string): Promise<void> {
  await page.goto(route);
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  await expectLoaded(page);
}

/** A campaign with a full-sheet enemy, created through the API as the
 * master. */
async function campaignWithNpc(page: Page): Promise<{ campaignId: string; npcId: string }> {
  const created = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
    name: `Acessibilidade ${Date.now()}`,
    xpMode: 'XP_MODE_ENEMIES',
  });
  expect(created.ok()).toBeTruthy();
  const campaignId = (await created.json()).campaign.id as string;
  const npc = await createCharacterRPC(page, campaignId, characterRpcBody('ENEMY', { ...pensantus, name: 'Capitão Goblin' }));
  expect(npc.ok()).toBeTruthy();
  return { campaignId, npcId: (await npc.json()).character.id as string };
}

async function scanMasterScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const page = await context.newPage();
  try {
    await page.goto('/');
    const { campaignId, npcId } = await campaignWithNpc(page);
    const screens: [string, string][] = [
      ['Início', '/'],
      ['Campanha, aba Mesa', `/campaigns/${campaignId}`],
      ['Campanha, aba Preparação', `/campaigns/${campaignId}/prep`],
      ['Campanha, aba Regras', `/campaigns/${campaignId}/setup`],
      ['Ficha do NPC', `/campaigns/${campaignId}/characters/${npcId}`],
      ['Editar a ficha do NPC', `/campaigns/${campaignId}/characters/${npcId}/edit`],
      ['Novo NPC básico', `/campaigns/${campaignId}/npcs/new/minion`],
      ['Meu perfil', '/profile'],
      ['Créditos', '/credits'],
    ];
    for (const [screen, route] of screens) {
      await open(page, route);
      await expectScreenPasses(page, `${screen} (${colorScheme}, ${width}px)`);
    }
  } finally {
    await context.close();
  }
}

test('as telas do mestre passam no axe no tema claro, no desktop', { tag: '@a11y' }, async ({ browser }) => {
  await scanMasterScreens(browser, 'light', 1280);
});

test('as telas do mestre passam no axe no tema escuro, no celular', { tag: '@a11y' }, async ({ browser }) => {
  await scanMasterScreens(browser, 'dark', 390);
});

/** "Sessões" on the profile page: the count with its button, and the in-place
 * confirmation. A second, real sign-in of the master gives the page another
 * device to show (it sends no cookie, so it revokes nothing); the scan only
 * opens the confirmation and cancels, so no shared session ends. */
async function scanSessions(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const other = await browser.newContext({ colorScheme });
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  try {
    await signIn(await other.newPage(), 'Mestre Teste', '/');
    const page = await context.newPage();
    const where = `(${colorScheme}, ${width}px)`;
    await open(page, '/profile');
    const ask = page.getByRole('button', { name: 'Sair dos outros dispositivos' });
    await expect(ask).toBeVisible();
    await expectScreenPasses(page, `Perfil, sessões ${where}`);
    await ask.click();
    await expect(page.getByRole('button', { name: 'Confirmar saída' })).toBeFocused();
    await expectScreenPasses(page, `Perfil, confirmar a saída dos outros dispositivos ${where}`);
    await page.getByRole('button', { name: 'Cancelar' }).click();
    await expect(ask).toBeFocused();
  } finally {
    await context.close();
    await other.close();
  }
}

test('"Sessões" do perfil passa no axe no tema claro, no desktop', { tag: '@a11y' }, async ({ browser }) => {
  await scanSessions(browser, 'light', 1280);
});

test('"Sessões" do perfil passa no axe no tema escuro, no celular', { tag: '@a11y' }, async ({ browser }) => {
  await scanSessions(browser, 'dark', 390);
});

/** The gallery (MR-019): empty, with images, a refused upload's notice,
 * a card's delete confirmation and the lightbox open. The gallery picker
 * (shared/gallery-picker) has no screen of its own until the map form
 * (5.3) uses it; its radio-group semantics are covered by its unit tests,
 * and it joins this scan with that screen. */
async function scanGallery(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const page = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await page.goto('/');
    const campaignId = await newCampaign(page, `Acessibilidade galeria ${Date.now()}`);
    await open(page, `/campaigns/${campaignId}/gallery`);
    await expect(page.getByRole('heading', { name: 'Nenhuma imagem ainda' })).toBeVisible();
    await expectScreenPasses(page, `Galeria vazia ${where}`);

    await uploadThroughPicker(page, [
      { name: 'Taverna do Javali.jpg', mimeType: 'image/jpeg', buffer: await canvasJpeg(page) },
      { name: 'Covil dos goblins.jpg', mimeType: 'image/jpeg', buffer: await canvasJpeg(page, '#5b4834') },
      { name: 'mapa-antigo.gif', mimeType: 'image/gif', buffer: Buffer.from('GIF89a') },
    ]);
    await expect(page.getByRole('article', { name: 'Covil dos goblins', exact: true })).toBeVisible();
    await expect(page.getByRole('alert')).toBeVisible();
    await expectScreenPasses(page, `Galeria com imagens e um envio recusado ${where}`);

    const card = page.getByRole('article', { name: 'Taverna do Javali', exact: true });
    await card.getByRole('button', { name: 'Apagar Taverna do Javali' }).click();
    await expect(card.getByRole('button', { name: 'Apagar imagem' })).toBeFocused();
    await expectScreenPasses(page, `Galeria, confirmar exclusão ${where}`);
    await card.getByRole('button', { name: 'Cancelar' }).click();

    await card.getByRole('button', { name: 'Ver Taverna do Javali' }).first().click();
    await expect(page.getByRole('dialog', { name: 'Taverna do Javali' })).toBeVisible();
    await expectScreenPasses(page, `Galeria, imagem aberta ${where}`);
    await page.keyboard.press('Escape');

    await open(page, `/campaigns/${campaignId}/prep`);
    await expect(page.getByRole('link', { name: 'Enviar imagem' })).toBeVisible();
    await expectScreenPasses(page, `Campanha com o painel Galeria ${where}`);
  } finally {
    await context.close();
  }
}

// Five scans and three uploads in one test: more room than the default.
test('a galeria passa no axe no tema claro, no desktop', { tag: ['@a11y', '@MR-019'] }, async ({ browser }) => {
  test.slow();
  await scanGallery(browser, 'light', 1280);
});

test('a galeria passa no axe no tema escuro, no celular', { tag: ['@a11y', '@MR-019'] }, async ({ browser }) => {
  test.slow();
  await scanGallery(browser, 'dark', 390);
});

/** The campaign document (MR-018): read mode, edit mode (with its toolbar
 * and the preview), the map dialog and the image picker dialog. */
async function scanDocument(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const page = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await page.goto('/');
    const t = await tableWithDocumentParts(page, `Acessibilidade documento ${Date.now()}`);
    // The map dialog draws the points: one revealed, one hidden (E5-29).
    await createPointRPC(page, t.campaignId, t.mapId, { kind: 'BATTLE', name: 'Emboscada na estrada', xBp: 5000, yBp: 6500, revealed: true });
    await createPointRPC(page, t.campaignId, t.mapId, { kind: 'SUBMAP', name: 'Covil dos goblins', xBp: 2500, yBp: 3000 });
    await saveDocumentRPC(
      page,
      t.campaignId,
      `## Arco 1\n\nVeja [Mirathel e arredores](map:${t.mapId}) e [Capitão Goblin](character:${t.npcId}), com **negrito** e *itálico*.\n\n![Taverna do Javali](image:${t.imageId})\n\n## Segredos\n\n- um\n- dois`,
      0,
    );
    await open(page, `/campaigns/${t.campaignId}/document`);
    await expect(page.getByRole('button', { name: 'Mirathel e arredores' })).toBeVisible();
    await expectScreenPasses(page, `Documento, leitura ${where}`);

    await page.getByRole('button', { name: 'Mirathel e arredores' }).click();
    const dialog = page.getByRole('dialog', { name: 'Mirathel e arredores' });
    await expect(dialog.getByRole('img', { name: 'Prévia do mapa Mirathel e arredores' })).toBeVisible();
    // The names show on the picture from 520 px of map (a phone's smaller one leaves them to this list).
    await expect(dialog.getByRole('list', { name: 'Pontos deste mapa' })).toContainText('Covil dos goblins');
    await expectScreenPasses(page, `Documento, janela do mapa ${where}`);
    await page.keyboard.press('Escape');

    await page.getByRole('button', { name: 'Editar documento' }).click();
    await expect(page.getByRole('textbox', { name: 'Texto' })).toBeVisible();
    await expectScreenPasses(page, `Documento, edição ${where}`);

    await page.getByRole('button', { name: 'Imagem da galeria' }).click();
    await expect(page.getByRole('dialog', { name: 'Imagem da galeria' }).getByRole('radio').first()).toBeVisible();
    await expectScreenPasses(page, `Documento, escolher imagem ${where}`);
  } finally {
    await context.close();
  }
}

test('o documento passa no axe no tema claro, no desktop', { tag: ['@a11y', '@MR-018'] }, async ({ browser }) => {
  test.slow();
  await scanDocument(browser, 'light', 1280);
});

test('o documento passa no axe no tema escuro, no celular', { tag: ['@a11y', '@MR-018'] }, async ({ browser }) => {
  test.slow();
  await scanDocument(browser, 'dark', 390);
});

test('as telas de quem não entrou passam no axe, nos dois temas', { tag: '@a11y' }, async ({ browser }) => {
  for (const colorScheme of ['light', 'dark'] as const) {
    const context = await browser.newContext({ colorScheme });
    const page = await context.newPage();
    try {
      for (const [screen, route] of [
        ['Início', '/'],
        ['Créditos', '/credits'],
        ['Página não encontrada', '/nao-existe'],
      ]) {
        await open(page, route);
        await expectScreenPasses(page, `${screen} (${colorScheme}, sem login)`);
      }
    } finally {
      await context.close();
    }
  }
});

// docs/design/README.md#logo-icons-and-footer and #footer: the two legal pages and the footer under them, signed out, in both
// themes at desktop and phone widths. The contents list is open beside the text on a desktop and closed on a phone.
test('os termos, a privacidade e o rodapé passam no axe nos dois temas, no desktop e no celular', { tag: ['@a11y', '@legal'] }, async ({ browser }) => {
  for (const [scheme, width] of [['light', 1280], ['dark', 1280], ['light', 390], ['dark', 390]] as const) {
    const context = await browser.newContext({ colorScheme: scheme, viewport: { width, height: 900 } });
    try {
      const page = await context.newPage();
      for (const [screen, route] of [
        ['Termos de uso', '/terms'],
        ['Política de privacidade', '/privacy'],
        ['Início', '/'],
      ]) {
        await open(page, route);
        const footer = page.getByRole('navigation', { name: 'Rodapé' });
        await expect(footer.getByRole('link')).toHaveCount(3);
        await footer.scrollIntoViewIfNeeded();
        await expectScreenPasses(page, `${screen} com rodapé (${scheme}, ${width}px)`);
      }
    } finally {
      await context.close();
    }
  }
});

// docs/design/tokens.md#colour: every control that takes focus shows the same 2px
// ring. Material's buttons remove their outline in their own styles, so
// this checks them explicitly (axe does not check that a focus ring shows).
test('os botões do Material mostram o anel de foco @a11y', async ({ page }) => {
  await page.goto('/uma-pagina-que-nao-existe');
  for (const name of ['Voltar para o início', 'Minhas campanhas']) {
    const button = page.getByRole('main').getByRole('link', { name });
    await button.focus();
    const ring = await button.evaluate((el) => {
      const style = getComputedStyle(el);
      return { style: style.outlineStyle, width: style.outlineWidth };
    });
    expect(ring, name).toEqual({ style: 'solid', width: '2px' });
  }
});

// docs/design/README.md#accessibility: every page has its own
// title (WCAG 2.4.2), the first Tab stop is "Pular para o conteúdo" (2.4.1), and a
// navigation to another page moves the focus to its heading (2.4.3).
test('cada página tem o próprio título, o primeiro Tab pula para o conteúdo e navegar leva o foco ao título @a11y', async ({ page }) => {
  await page.goto('/credits');
  await expect(page).toHaveTitle('Créditos · MeuRPG');

  // The first Tab stop is the skip link, hidden until it has the focus.
  await page.keyboard.press('Tab');
  const skip = page.getByRole('link', { name: 'Pular para o conteúdo' });
  await expect(skip).toBeFocused();
  await expect(skip).toBeInViewport();
  await page.keyboard.press('Enter');
  await expect(page.getByRole('main')).toBeFocused();
  await expect(page).toHaveURL(/\/credits$/);

  // A navigation to another page: its own title, and the focus on its heading.
  await page.goto('/nao-existe');
  await expect(page).toHaveTitle('Página não encontrada · MeuRPG');
  await page.getByRole('main').getByRole('link', { name: 'Voltar para o início' }).click();
  await expect(page).toHaveTitle('Início · MeuRPG');
  await expect(page.getByRole('heading', { level: 1 }).first()).toBeFocused();
});

/**
 * The live session's screens (Etapa 5): the session page for the master and
 * for the player, the adjust sheet open (a dialog on the desktop, a bottom
 * sheet on the phone), and the link opened by someone who isn't in the
 * campaign. The session page keeps a stream open, so these wait for the
 * page's own "Ao vivo" instead of `networkidle`.
 */
async function scanLiveSessionScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(90_000);
  const options = { colorScheme, viewport: { width, height: 900 } };
  const master = await browser.newContext({ ...options, storageState: authStatePath('Mestre Teste') });
  const player = await browser.newContext({ ...options, storageState: authStatePath('Jogador Teste') });
  const masterPage = await master.newPage();
  const playerPage = await player.newPage();
  const suffix = `(${colorScheme}, ${width}px)`;
  try {
    await masterPage.goto('/');
    await playerPage.goto('/');
    const { campaignId } = await tableWithPensantus(masterPage, playerPage, `Acessibilidade ao vivo ${Date.now()}`);
    const sessionId = await startSessionRPC(masterPage, campaignId);

    await openSessionPage(masterPage, campaignId, 'grupo');
    await expectScreenPasses(masterPage, `Sessão, mestre ${suffix}`);

    await masterPage.getByRole('button', { name: 'Ajustar Pensantus' }).click();
    await expect(masterPage.getByRole('dialog', { name: 'Ajustar Pensantus' })).toBeVisible();
    // Scan the sheet once it's in place: mid-animation, its text is still
    // fading in, and axe would measure the contrast of a half-drawn frame.
    await masterPage.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(masterPage, `Ajustar PV ${suffix}`);
    await masterPage.getByRole('button', { name: 'Cancelar' }).click();

    await openSessionPage(playerPage, campaignId);
    await expectScreenPasses(playerPage, `Sessão, jogador ${suffix}`);
    // The numbers are in "Ficha" (a tab of the rail from 1024 px, a panel on a phone).
    await openSheetTab(playerPage);
    await expect(playerPage.getByRole('region', { name: 'Pensantus' })).toBeVisible();
    await expectScreenPasses(playerPage, `Ficha na sessão, jogador ${suffix}`);

    // A campaign Jogador Teste isn't in: the link says to ask for an invite.
    const closed = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
      name: `Mesa fechada ${Date.now()}`,
      xpMode: 'XP_MODE_ENEMIES',
    });
    await playerPage.goto(`/campaigns/${(await closed.json()).campaign.id}/session`);
    await expect(playerPage.getByRole('heading', { level: 1, name: 'Peça um convite ao mestre' })).toBeVisible();
    await expectScreenPasses(playerPage, `Sessão sem acesso ${suffix}`);

    await endSessionRPC(masterPage, campaignId, sessionId);
  } finally {
    await master.close();
    await player.close();
  }
}

test('as telas da sessão ao vivo passam no axe no tema claro, no desktop', { tag: ['@a11y', '@MR-012'] }, async ({ browser }) => {
  await scanLiveSessionScreens(browser, 'light', 1280);
});

test('as telas da sessão ao vivo passam no axe no tema escuro, no celular', { tag: ['@a11y', '@MR-012'] }, async ({ browser }) => {
  await scanLiveSessionScreens(browser, 'dark', 390);
});

/**
 * The maps' screens (Etapa 5, MR-008, MR-009, MR-012, MR-028): "Novo mapa",
 * the editor with a point selected, the player's map with a point's sheet
 * open, the picker dialog, and the session page with the current map and
 * with an image on show, for the master and for the player.
 */
async function scanMapScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(120_000);
  const options = { colorScheme, viewport: { width, height: 900 } };
  const master = await browser.newContext({ ...options, storageState: authStatePath('Mestre Teste') });
  const player = await browser.newContext({ ...options, storageState: authStatePath('Jogador Teste') });
  const masterPage = await master.newPage();
  const playerPage = await player.newPage();
  const suffix = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await masterPage.goto('/');
    await playerPage.goto('/');
    const table = await tableForMaps(masterPage, playerPage, `Acessibilidade mapas ${Date.now()}`, true);
    campaignId = table.campaignId;
    const worldImage = await uploadImageRPC(masterPage, campaignId, 'Mapa de Mirathel', await canvasPng(masterPage, 1200, 800, 'Mirathel'));
    const towerImage = await uploadImageRPC(masterPage, campaignId, 'Planta da torre', await canvasPng(masterPage, 800, 800, 'Torre', '#5b4834'));
    await uploadImageRPC(masterPage, campaignId, 'Capitão Goblin', await canvasPng(masterPage, 400, 500, 'Capitão Goblin', '#3a3a2a'));
    const tower = await createMapRPC(masterPage, campaignId, 'Torre de Mirathel', towerImage);
    const world = await createMapRPC(masterPage, campaignId, 'Mirathel e arredores', worldImage);
    await revealMapRPC(masterPage, campaignId, tower);
    await revealMapRPC(masterPage, campaignId, world);
    await createPointRPC(masterPage, campaignId, world, { kind: 'BATTLE', name: 'Emboscada na estrada', xBp: 3800, yBp: 6200, revealed: true });
    await createPointRPC(masterPage, campaignId, world, { kind: 'SUBMAP', name: 'Torre de Mirathel', description: 'Uma torre antiga na colina.', xBp: 7100, yBp: 2800, targetMapId: tower, revealed: true });
    await createPointRPC(masterPage, campaignId, world, { kind: 'SCENE', name: 'Ruínas élficas', xBp: 8300, yBp: 7600 });
    await placeTokenRPC(masterPage, campaignId, world, table.characterId, 5200, 5400);
    await placeTokenRPC(masterPage, campaignId, world, table.npcId!, 3700, 6000);

    await open(masterPage, `/campaigns/${campaignId}/maps/new`);
    await expectScreenPasses(masterPage, `Novo mapa ${suffix}`);
    await masterPage.getByRole('button', { name: 'Criar mapa' }).click();
    await expect(masterPage.getByText('Dê um nome ao mapa.')).toBeVisible();
    await masterPage.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(masterPage, `Novo mapa com erros ${suffix}`);

    await open(masterPage, `/campaigns/${campaignId}/maps/${world}`);
    await expectScreenPasses(masterPage, `Mapa, mestre ${suffix}`);
    // E6-27: the header renaming, and asking before deleting.
    await masterPage.getByRole('button', { name: 'Renomear' }).click();
    await expect(masterPage.getByLabel('Nome do mapa')).toBeFocused();
    await expectScreenPasses(masterPage, `Mapa, renomear ${suffix}`);
    await masterPage.getByRole('button', { name: 'Cancelar' }).click();
    await masterPage.getByRole('button', { name: 'Apagar mapa' }).click();
    await expect(masterPage.getByRole('group', { name: /^Apagar / }).getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await expectScreenPasses(masterPage, `Mapa, apagar ${suffix}`);
    await masterPage.getByRole('group', { name: /^Apagar / }).getByRole('button', { name: 'Cancelar' }).click();
    if (width >= 768) {
      await masterPage.getByRole('button', { name: 'Ruínas élficas, Cena de RP, escondido' }).click();
      await expect(masterPage.getByRole('heading', { name: 'Ruínas élficas' })).toBeVisible();
      await expectScreenPasses(masterPage, `Editor com um ponto escolhido ${suffix}`);
      // A Submapa with a destination ("Abrir <mapa>"), then "Criar mapa novo…" open in the panel.
      await masterPage.getByRole('button', { name: 'Torre de Mirathel, Submapa' }).first().click();
      await expect(masterPage.getByRole('link', { name: /^Abrir / })).toBeVisible();
      await expectScreenPasses(masterPage, `Editor, submapa com "Abrir" ${suffix}`);
      await masterPage.getByLabel('Leva para').selectOption({ label: 'Criar mapa novo…' });
      await expect(masterPage.getByRole('button', { name: 'Criar mapa e usar' })).toBeVisible();
      await masterPage.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
      await expectScreenPasses(masterPage, `Editor, "Criar mapa novo…" ${suffix}`);
    }

    await open(playerPage, `/campaigns/${campaignId}/maps/${world}`);
    await expectScreenPasses(playerPage, `Mapa, jogador ${suffix}`);
    await playerPage.getByRole('button', { name: 'Torre de Mirathel, Submapa' }).first().click();
    await expect(playerPage.getByRole('button', { name: 'Abrir Torre de Mirathel' })).toBeVisible();
    await expectScreenPasses(playerPage, `Mapa, jogador, com a ficha de um ponto ${suffix}`);

    await startSessionRPC(masterPage, campaignId);
    await setCurrentMapRPC(masterPage, campaignId, world);
    await openSessionPage(masterPage, campaignId, 'mapa');
    await showMapTool(masterPage, 'Pontos');
    await expect(masterPage.getByRole('heading', { name: 'Pontos do mapa' })).toBeVisible();
    await expectScreenPasses(masterPage, `Sessão com mapa, mestre ${suffix}`);
    // The map's tools, then "Grupo" and "Cena": every tab of the master's board is scanned.
    for (const tool of ['Pontos', 'Tokens', 'Armadilhas', 'Tesouro', 'Encontros'] as const) {
      await showMapTool(masterPage, tool);
      await expectScreenPasses(masterPage, `Mapa, ${tool}, mestre ${suffix}`);
    }
    await showMasterTab(masterPage, 'grupo');
    await expectScreenPasses(masterPage, `Grupo, mestre ${suffix}`);
    await openSessionPage(playerPage, campaignId);
    if (width < 768) {
      // A phone's map is one row: the name and "Ver mapa" (C5-01).
      await expect(playerPage.getByRole('link', { name: 'Ver mapa' })).toBeVisible();
      await expect(playerPage.getByRole('heading', { name: 'Mapa · Mirathel e arredores' })).toBeVisible();
    } else {
      await expect(playerPage.getByRole('img', { name: 'Prévia do mapa Mirathel e arredores' })).toBeVisible();
    }
    await expectScreenPasses(playerPage, `Sessão com mapa, jogador ${suffix}`);

    await showMasterTab(masterPage, 'cena');
    // The picker, then an image on show.
    await masterPage.getByRole('button', { name: 'Mostrar imagem' }).click();
    const dialog = masterPage.getByRole('dialog', { name: 'Mostrar uma imagem aos jogadores' });
    await dialog.getByRole('radio', { name: /Capitão Goblin/ }).click();
    await masterPage.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(masterPage, `Mostrar imagem, seletor ${suffix}`);
    await dialog.getByRole('button', { name: 'Mostrar aos jogadores' }).click();
    await expect(masterPage.getByRole('button', { name: 'Parar de mostrar' })).toBeVisible();
    await expectScreenPasses(masterPage, `Sessão com imagem à mostra, mestre ${suffix}`);
    await expect(playerPage.getByRole('region', { name: 'O mestre está mostrando' })).toBeVisible();
    await playerPage.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(playerPage, `Sessão com imagem à mostra, jogador ${suffix}`);

    // "Deixar com os jogadores" on, then the image left with the players.
    const keep = masterPage.getByRole('switch', { name: 'Deixar com os jogadores' });
    await keep.click();
    await expect(keep).toHaveAttribute('aria-checked', 'true');
    await expectScreenPasses(masterPage, `Sessão com "Deixar com os jogadores" ligado, mestre ${suffix}`);
    await masterPage.getByRole('button', { name: 'Parar de mostrar' }).click();
    await expect(masterPage.getByRole('button', { name: 'Tirar Capitão Goblin dos jogadores' })).toBeVisible();
    await expectScreenPasses(masterPage, `Sessão com uma imagem deixada, mestre ${suffix}`);
    const left = playerPage.getByRole('region', { name: 'Imagens que o mestre deixou' });
    await expect(left).toBeVisible();
    await playerPage.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(playerPage, `Sessão com uma imagem deixada, jogador ${suffix}`);
    if (width < 768) {
      // A phone folds it to one row; opened, the thumbnails are buttons.
      await left.getByRole('button', { name: /^Imagens que o mestre deixou/ }).click();
      await expect(left.getByRole('button', { name: 'Ver Capitão Goblin em tela cheia' })).toBeVisible();
      await expectScreenPasses(playerPage, `Sessão com uma imagem deixada, a dobra aberta, jogador ${suffix}`);
    }
  } finally {
    await endOpenSessionRPC(masterPage, campaignId);
    await master.close();
    await player.close();
  }
}

test('as telas de mapa e da imagem mostrada passam no axe no tema claro, no desktop', { tag: ['@a11y', '@MR-008', '@MR-028'] }, async ({ browser }) => {
  await scanMapScreens(browser, 'light', 1280);
});

test('as telas de mapa e da imagem mostrada passam no axe no tema escuro, no celular', { tag: ['@a11y', '@MR-009', '@MR-028'] }, async ({ browser }) => {
  await scanMapScreens(browser, 'dark', 390);
});

/** Printing a map to scale (MR-033, E8-12): the map page's entry with and
 * without a grid, and the print view in its states: the defaults, a size and
 * a paper that need 3 sheets, one that needs 36 (the amber notice, the labels
 * shrunk), 136 (the labels gone), an invalid size, and a map without a grid.
 * On a phone the setup stacks above the preview. */
async function scanPrintScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(180_000);
  const options = { colorScheme, viewport: { width, height: 900 } };
  const master = await browser.newContext({ ...options, storageState: authStatePath('Mestre Teste') });
  const player = await browser.newContext({ ...options, storageState: authStatePath('Jogador Teste') });
  const masterPage = await master.newPage();
  const playerPage = await player.newPage();
  const suffix = `(${colorScheme}, ${width}px)`;
  try {
    await masterPage.goto('/');
    const table = await tableForPrinting(masterPage, playerPage, `Acessibilidade impressão ${Date.now()}`);

    await open(masterPage, `/campaigns/${table.campaignId}/maps/${table.gridMapId}`);
    await expectScreenPasses(masterPage, `Mapa com grade, entrada de impressão ${suffix}`);
    await open(masterPage, `/campaigns/${table.campaignId}/maps/${table.plainMapId}`);
    await expect(masterPage.getByText('Defina a grade do mapa para imprimir em escala')).toBeVisible();
    await expectScreenPasses(masterPage, `Mapa sem grade, entrada de impressão ${suffix}`);

    await open(masterPage, printRoute(table.campaignId, table.gridMapId));
    await expect(masterPage.getByLabel('Tamanho do quadrado')).toHaveValue('2,54');
    await expectScreenPasses(masterPage, `Imprimir o mapa, A4 e 2,54 cm ${suffix}`);

    const square = masterPage.getByLabel('Tamanho do quadrado');
    await square.fill('2');
    await masterPage.getByRole('radio', { name: /A3/ }).check();
    await expect(masterPage.getByRole('button', { name: 'Voltar a 2,54 cm' })).toBeVisible();
    await expectScreenPasses(masterPage, `Imprimir o mapa, 2 cm em A3 ${suffix}`);

    await masterPage.getByRole('radio', { name: /A4/ }).check();
    await square.fill('5');
    await expect(masterPage.getByText('São 36 folhas.')).toBeVisible();
    await expectScreenPasses(masterPage, `Imprimir o mapa, 36 folhas e o aviso ${suffix}`);

    await square.fill('10');
    await expect(masterPage.getByText('São 136 folhas.')).toBeVisible();
    await expectScreenPasses(masterPage, `Imprimir o mapa, 136 folhas sem rótulos ${suffix}`);

    await square.fill('0,5');
    await expect(masterPage.getByRole('alert').filter({ hasText: 'Use um tamanho de 1 a 10 cm.' })).toBeVisible();
    await expectScreenPasses(masterPage, `Imprimir o mapa, tamanho inválido ${suffix}`);

    await open(masterPage, printRoute(table.campaignId, table.plainMapId));
    await expect(masterPage.getByText('Este mapa ainda não tem grade.')).toBeVisible();
    await expectScreenPasses(masterPage, `Imprimir o mapa, sem grade ${suffix}`);

    await open(playerPage, printRoute(table.campaignId, table.gridMapId));
    await expectScreenPasses(playerPage, `Imprimir o mapa, jogador ${suffix}`);
  } finally {
    await master.close();
    await player.close();
  }
}

test('a impressão do mapa passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-033'] }, async ({ browser }) => {
  await scanPrintScreens(browser, 'light', 1280);
});

test('a impressão do mapa passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-033'] }, async ({ browser }) => {
  await scanPrintScreens(browser, 'dark', 390);
});

test('a impressão do mapa passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-033'] }, async ({ browser }) => {
  await scanPrintScreens(browser, 'dark', 1024);
});

test('a impressão do mapa passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-033'] }, async ({ browser }) => {
  await scanPrintScreens(browser, 'light', 320);
});

/** The dice settings (RN-18): the master's "Dados" panel and the player's
 * "Como você rola os dados", as a choice and locked (the master decided). */
async function scanDiceScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const masterPage = await master.newPage();
  const playerPage = await player.newPage();
  try {
    await masterPage.goto('/');
    await playerPage.goto('/');
    const { campaignId } = await tableWithPensantus(masterPage, playerPage, `Acessibilidade dados ${Date.now()}`);
    const suffix = `(${colorScheme}, ${width}px)`;
    await open(masterPage, `/campaigns/${campaignId}/setup`);
    await expectScreenPasses(masterPage, `Campanha com Dados, mestre ${suffix}`);
    await open(playerPage, `/campaigns/${campaignId}`);
    await expectScreenPasses(playerPage, `Campanha com Como você rola os dados, jogador ${suffix}`);

    const set = await callRPC(masterPage, 'meurpg.campaigns.v1.CampaignService/SetCampaignDiceMode', { campaignId, mode: 'DICE_MODE_APP' });
    expect(set.ok()).toBeTruthy();
    await open(masterPage, `/campaigns/${campaignId}/setup`);
    await expectScreenPasses(masterPage, `Campanha com Dados, todos no app, mestre ${suffix}`);
    await open(playerPage, `/campaigns/${campaignId}`);
    await expectScreenPasses(playerPage, `Como você rola os dados, decidido pelo mestre ${suffix}`);
  } finally {
    await master.close();
    await player.close();
  }
}

test('as configurações de dados passam no axe no tema claro, no desktop', { tag: ['@a11y', '@RN-18'] }, async ({ browser }) => {
  await scanDiceScreens(browser, 'light', 1280);
});

test('as configurações de dados passam no axe no tema escuro, no celular', { tag: ['@a11y', '@RN-18'] }, async ({ browser }) => {
  await scanDiceScreens(browser, 'dark', 390);
});

/** The campaign page with someone waiting to create a character (MR-024):
 * the "Membros" row with its tag, and the removal confirmation open. */
async function scanPendingMembers(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  const page = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await page.goto('/');
    const created = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
      name: `Acessibilidade esperando ${Date.now()}`,
      xpMode: 'XP_MODE_ENEMIES',
    });
    const campaignId = (await created.json()).campaign.id as string;
    const invite = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
      campaignId,
      maxUses: 1,
      expiresIn: '86400s',
      requiresApproval: true,
    });
    const { token } = await invite.json();
    const playerPage = await playerContext.newPage();
    await playerPage.goto('/');
    const accepted = await callRPC(playerPage, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token });
    expect(accepted.ok()).toBeTruthy();

    await open(page, `/campaigns/${campaignId}`);
    // A phone folds "Membros" to one row.
    await openFoldedPanel(page, 'Membros');
    await expect(page.getByRole('list', { name: 'Esperando para criar o personagem' })).toBeVisible();
    await expectScreenPasses(page, `Campanha com alguém sem personagem ${where}`);
    await page.getByRole('button', { name: /^Remover .* da campanha$/ }).click();
    await expect(page.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(page, `Campanha, confirmar a remoção ${where}`);
  } finally {
    await playerContext.close();
    await context.close();
  }
}

test('quem está sem personagem passa no axe no tema claro, no desktop', { tag: ['@a11y', '@MR-024'] }, async ({ browser }) => {
  await scanPendingMembers(browser, 'light', 1280);
});

test('quem está sem personagem passa no axe no tema escuro, no celular', { tag: ['@a11y', '@MR-024'] }, async ({ browser }) => {
  await scanPendingMembers(browser, 'dark', 390);
});

/** "Pedir ajustes" (RN-15, MR-024): the master's three buttons, the form empty and with its error, the notice after the request, the
 * refusal question, and the owning player's notice and the waiting one after "Enviar de novo". */
async function scanReviewScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(120_000);
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport: { width, height: 900 } });
  const page = await context.newPage();
  const playerPage = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await page.goto('/');
    const created = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
      name: `Acessibilidade ajustes ${Date.now()}`,
      xpMode: 'XP_MODE_ENEMIES',
    });
    const campaignId = (await created.json()).campaign.id as string;
    const invite = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
      campaignId,
      maxUses: 1,
      expiresIn: '86400s',
      requiresApproval: true,
    });
    const { token } = await invite.json();
    await playerPage.goto('/');
    const accepted = await callRPC(playerPage, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token });
    expect(accepted.ok()).toBeTruthy();
    const character = await createCharacterRPC(playerPage, campaignId, characterRpcBody('PLAYER', pensantus));
    const route = `/campaigns/${campaignId}/characters/${(await character.json()).character.id as string}`;

    await open(page, route);
    await expect(page.getByRole('button', { name: 'Pedir ajustes', exact: true })).toBeVisible();
    await expectScreenPasses(page, `Pendente, os três botões ${where}`);

    await page.getByRole('button', { name: 'Pedir ajustes', exact: true }).click();
    await page.getByRole('button', { name: 'Enviar pedido' }).click();
    await expect(page.getByRole('alert').filter({ hasText: 'O pedido não vai sem motivo.' })).toBeVisible();
    await expectScreenPasses(page, `Pedir ajustes, sem motivo ${where}`);

    await page.getByLabel(/^O que .* precisa ajustar$/).fill('O antecedente não bate com a história. Também falta escolher o equipamento.');
    await expectScreenPasses(page, `Pedir ajustes, com o motivo ${where}`);
    await page.getByRole('button', { name: 'Enviar pedido' }).click();
    await expect(page.getByRole('button', { name: 'Pedir ajustes de novo' })).toBeFocused();
    await expectScreenPasses(page, `Pendente, ajustes pedidos ${where}`);

    await page.getByRole('button', { name: 'Recusar personagem' }).click();
    await expect(page.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await expectScreenPasses(page, `Recusar, a confirmação ${where}`);

    await open(playerPage, route);
    await expect(playerPage.getByRole('alert').filter({ hasText: 'O mestre pediu ajustes.' })).toBeVisible();
    await expectScreenPasses(playerPage, `O jogador, ajustes pedidos ${where}`);
    await playerPage.getByRole('button', { name: 'Enviar de novo' }).click();
    await expect(playerPage.getByRole('status').filter({ hasText: 'Ficha enviada de novo.' })).toBeVisible();
    await expectScreenPasses(playerPage, `O jogador, ficha enviada de novo ${where}`);

    await open(page, route);
    await expect(page.getByRole('list', { name: 'Estado do personagem' })).toContainText('Pendente · reenviado');
    await expectScreenPasses(page, `Pendente, reenviado ${where}`);
  } finally {
    await playerContext.close();
    await context.close();
  }
}

test('pedir ajustes passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-024', '@RN-15'] }, async ({ browser }) => {
  await scanReviewScreens(browser, 'light', 1280);
});

test('pedir ajustes passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-024', '@RN-15'] }, async ({ browser }) => {
  await scanReviewScreens(browser, 'dark', 390);
});

/**
 * The character editor's rolls and the spell "?" (MR-004, E6-20 to E6-23):
 * the "Habilidades" step with "Rolar 4d6" (half placed, and on the phone with a
 * result chosen), the rolled hit points, the "Magias" step with its search
 * fields, and the spell dialog (a bottom sheet on the phone).
 */
async function scanEditorRolls(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(90_000);
  const context = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height: 900 } });
  const page = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await page.goto('/');
    const created = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Acessibilidade rolagens ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
    expect(created.ok()).toBeTruthy();
    const campaignId = (await created.json()).campaign.id as string;
    await open(page, `/campaigns/${campaignId}/characters/new`);
    await page.getByLabel('Nome do personagem', { exact: true }).fill('Zézinho');
    const classSelect = page.getByRole('combobox', { name: 'Classe', exact: true });
    await classSelect.focus();
    await classSelect.press('Enter');
    await page.getByRole('option', { name: 'Mago', exact: true }).click();
    await page.getByLabel('Nível', { exact: true }).fill('3');

    await page.getByRole('tab', { name: 'Habilidades' }).click();
    await page.getByRole('radio', { name: /Rolar 4d6/ }).check();
    if (width < 768) {
      await page.getByRole('button', { name: /^\d+ [\d ]*Livre$/ }).first().click();
      await page.getByRole('button', { name: /^Força: colocar o/ }).click();
      await page.getByRole('button', { name: /^\d+ [\d ]*Livre$/ }).first().click();
    } else {
      await page.getByLabel('Força', { exact: true }).selectOption({ index: 1 });
    }
    await expectScreenPasses(page, `Habilidades, Rolar 4d6 ${where}`);

    await page.getByRole('radio', { name: /Rolado/ }).check();
    await page.getByRole('button', { name: 'Rolar os níveis que faltam' }).click();
    await expectScreenPasses(page, `Pontos de vida rolados ${where}`);

    await page.getByRole('tab', { name: 'Magias' }).click();
    await expectScreenPasses(page, `Magias ${where}`);
    await page.getByRole('group', { name: 'Magias conhecidas', exact: true }).getByRole('button', { name: 'Descrição de Mísseis Mágicos' }).click();
    // Portuguese first, then the English the button flips to (the choice holds for the whole app until reload).
    await expect(page.getByText('Texto do SRD 5.1', { exact: true })).toBeVisible();
    await expectScreenPasses(page, `Descrição da magia ${where}`);
    await page.getByRole('button', { name: 'Ver em inglês' }).click();
    await expect(page.getByText('Texto do SRD 5.1 (em inglês)')).toBeVisible();
    await expectScreenPasses(page, `Descrição da magia, em inglês ${where}`);
  } finally {
    await context.close();
  }
}

test('as rolagens e a descrição da magia passam no axe no tema claro, no desktop', { tag: ['@a11y', '@MR-004'] }, async ({ browser }) => {
  await scanEditorRolls(browser, 'light', 1280);
});

test('as rolagens e a descrição da magia passam no axe no tema escuro, no celular', { tag: ['@a11y', '@MR-004'] }, async ({ browser }) => {
  await scanEditorRolls(browser, 'dark', 390);
});

/** The combat (MR-013, E6-01 to E6-16): the grid page, the start dialog, the
 * initiative of the master and of the player, the running combat for each, the
 * "Mover" page in each of its states, the end confirmation and the summary.
 * The master and the player each have a page at `width`; the NPCs' initiative is
 * set through the API so the screens are the same on every run. */
async function scanCombatScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade combate ${Date.now()}`, false);
    campaignId = table.campaignId;

    await open(m, `/campaigns/${campaignId}/maps/${table.mapId}/grid?from=session`);
    await expectScreenPasses(m, `Grade do mapa sem grade ${where}`);
    await m.getByRole('button', { name: 'Salvar grade' }).click();
    await expect(m).toHaveURL(/\/session$/);

    await expectStartCombatOffered(m);
    await expectScreenPasses(m, `Sessão com o convite ao combate ${where}`);
    await clickStartCombat(m);
    await expect(m.getByRole('dialog', { name: 'Iniciar combate' })).toBeVisible();
    for (let i = 0; i < 3; i++) {
      await m.getByRole('button', { name: 'Mais um Goblin', exact: true }).click();
    }
    await m.getByRole('button', { name: 'Mais um Capitão Goblin' }).click();
    await expectScreenPasses(m, `Iniciar combate ${where}`);
    await m.getByRole('dialog').getByRole('button', { name: 'Iniciar combate' }).click();
    await expect(m.getByText('Os NPCs rolaram sozinhos.')).toBeVisible();

    // The player before rolling, then the master's initiative with a tie and a missing roll.
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Role a iniciativa' })).toBeVisible();
    await expectScreenPasses(p, `Iniciativa do jogador, antes de rolar ${where}`);
    let enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const face = async (label: string, d20Face: number) => {
      enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: id(label), d20Face });
    };
    await face('Capitão Goblin', 20);
    await face('Goblin 1', 7);
    await face('Goblin 2', 7);
    await face('Goblin 3', 3);
    await expect(m.getByText('Falta a iniciativa de Pensantus.').first()).toBeVisible();
    await expectScreenPasses(m, `Iniciativa do mestre, com empate e rolagem faltando ${where}`);
    await m.getByRole('button', { name: 'Digitar pelo jogador' }).click();
    await m.getByLabel(/Resultado do d20 de Pensantus/).fill('19');
    await expectScreenPasses(m, `Iniciativa do mestre, editando ${where}`);
    await m.getByRole('button', { name: 'Salvar', exact: true }).click();
    await expect(p.getByText('Esperando o mestre começar o combate')).toBeVisible();
    await expectScreenPasses(p, `Iniciativa do jogador, depois de rolar ${where}`);
    for (const [label, col, row] of [['Goblin 1', 9, 9], ['Goblin 2', 14, 10], ['Goblin 3', 15, 4], ['Capitão Goblin', 11, 5]] as const) {
      enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId: id(label), col, row });
    }

    // The combat runs: the master's screen, the player out of turn (the captain is hidden: "Vez do mestre").
    await m.getByRole('button', { name: 'Começar o combate' }).click();
    await expect(m.getByRole('button', { name: 'Próximo turno' })).toBeVisible();
    await expect(m.getByText('Vez do Capitão Goblin')).toBeVisible();
    await expectScreenPasses(m, `Combate do mestre ${where}`);
    await expect(p.getByRole('heading', { name: 'Vez do mestre' })).toBeVisible();
    await expectScreenPasses(p, `Combate do jogador, vez do mestre ${where}`);
    enc = await getEncounterRPC(m, campaignId);
    enc = await combatRPC(m, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: id('Capitão Goblin'), hidden: false });
    await waitForEncounterRevision([m, p], enc);
    await expect(p.getByRole('heading', { name: 'Vez do Capitão Goblin' })).toBeVisible();
    await expectScreenPasses(p, `Combate do jogador, fora da vez ${where}`);

    // The player's turn and the "Mover" page.
    await m.getByRole('button', { name: 'Próximo turno' }).click();
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await expectScreenPasses(p, `Combate do jogador, sua vez ${where}`);
    await p.getByRole('button', { name: 'Mover' }).click();
    await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await expectScreenPasses(p, `Mover, nada escolhido ${where}`);
    const map = p.getByRole('group', { name: /Mapa de batalha/ });
    const box = await boxOf(map);
    const own = (await getEncounterRPC(p, campaignId)).combatants.find((c) => c.mine)!;
    const at = (dc: number, dr: number) => ({ x: ((own.col ?? 0) + dc + 0.5) * (box.width / 20), y: ((own.row ?? 0) + dr + 0.5) * (box.height / 14) });
    await map.click({ position: at(2, 1) });
    await expect(p.getByText('Mover 3,4 m')).toBeVisible();
    await expectScreenPasses(p, `Mover, quadrado escolhido ${where}`);
    await map.click({ position: at(8, 1) });
    await expect(p.getByRole('alert').filter({ hasText: 'Longe demais' })).toBeVisible();
    await expectScreenPasses(p, `Mover, longe demais ${where}`);
    await p.getByRole('button', { name: 'Cancelar' }).click();

    // The end: the master's confirmation in place, then the summary for both.
    await m.getByRole('button', { name: 'Encerrar combate' }).click();
    await expect(m.getByRole('alertdialog', { name: 'Encerrar o combate?' })).toBeVisible();
    await expectScreenPasses(m, `Encerrar o combate, confirmação ${where}`);
    await m.getByRole('button', { name: 'Encerrar combate' }).last().click();
    await expect(m.getByRole('heading', { name: 'Combate encerrado' })).toBeVisible();
    await expect(p.getByRole('heading', { name: 'O combate acabou' })).toBeVisible(); // the player's one end-of-combat card
    await expectScreenPasses(m, `Combate encerrado, mestre ${where}`);
    await expectScreenPasses(p, `Combate encerrado, jogador ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o combate passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-013'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanCombatScreens(browser, 'light', 1280);
});

test('o combate passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-013'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanCombatScreens(browser, 'dark', 390);
});

/** The master's initiative setup at the desktop widths between a laptop and a wide screen (MR-013): beside the conversation column
 * of 1600px and more the board is narrower than at 1280, and the list must still read: a name on one line (an ellipsis, never a letter
 * a line), no horizontal scroll, the bar's help text on a measure of its own, and the roll and the buttons inside the row. */
async function scanMasterInitiativeSetup(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 941 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade iniciativa ${Date.now()}`, false);
    campaignId = table.campaignId;
    await m.goto(`/campaigns/${campaignId}/session`);
    await expectStartCombatOffered(m);
    await clickStartCombat(m);
    await expect(m.getByRole('dialog', { name: 'Iniciar combate' })).toBeVisible();
    for (let i = 0; i < 3; i++) {
      await m.getByRole('button', { name: 'Mais um Goblin', exact: true }).click();
    }
    await m.getByRole('button', { name: 'Mais um Capitão Goblin' }).click();
    await m.getByRole('dialog').getByRole('button', { name: 'Iniciar combate' }).click();
    await expect(m.getByText('Os NPCs rolaram sozinhos.')).toBeVisible();
    await expectScreenPasses(m, `Iniciativa do mestre, antes de começar ${where}`);

    // Every name keeps one line and nothing leaves its row or the page.
    const list = m.locator('app-initiative-setup');
    await expect(list.locator('.row__name').first()).toBeVisible();
    for (const name of await list.locator('.row__name').all()) {
      const box = await boxOf(name);
      expect(box.height, `um nome em uma linha ${where}`).toBeLessThan(26);
      expect(box.width, `um nome com largura ${where}`).toBeGreaterThan(60);
    }
    for (const row of await list.locator('.row').all()) {
      const inside = await row.evaluate((el) => {
        const r = el.getBoundingClientRect();
        return [...el.querySelectorAll('.row__formula, .row__actions button')].every((c) => {
          const b = c.getBoundingClientRect();
          return b.left >= r.left - 1 && b.right <= r.right + 1;
        });
      });
      expect(inside, `a rolagem e os botões dentro da linha ${where}`).toBe(true);
    }
    const lead = await boxOf(m.locator('.bar__lead'));
    expect(lead.width, `o texto de ajuda da barra com medida ${where}`).toBeGreaterThan(300);
    expect(await m.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), `sem rolagem lateral ${where}`).toBe(true);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

for (const [scheme, width] of [['light', 1440], ['dark', 1728]] as const) {
  test(`a iniciativa do mestre passa no axe e nas conferências de layout no tema ${scheme === 'light' ? 'claro' : 'escuro'}, em ${width}px`, { tag: ['@a11y', '@MR-013'] }, async ({ browser }) => {
    test.setTimeout(240_000);
    await scanMasterInitiativeSetup(browser, scheme, width);
  });
}

/** Acting in a combat (Etapa 6, slice 6.5b; E6-06, E6-07, E6-08, E6-15, E6-11):
 * the player's "Sua vez" groups, the end-turn question, every step of the
 * attack sheet (typed roll empty, wrong, valid; the result), the log sheet on a
 * phone, and the master's card with the armor class, "Aplicar" and its
 * questions, "Desfazer última ação" and "Dano/Cura". */
async function scanActionScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade ações ${Date.now()}`, true, true);
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });

    // The player's turn: the groups, the question when the action is free, and the attack.
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('button', { name: 'Atacar com Raio de Fogo' })).toBeVisible();
    await expectScreenPasses(p, `Sua vez, com os grupos de ações ${where}`);
    if (width >= 1024) {
      // From 1024 px the conversation is the rail's second tab; its panel stays mounted and the Actions tab comes back as it was.
      await p.getByRole('tab', { name: /^Conversa/ }).click();
      await expect(p.getByRole('heading', { name: 'Conversa', level: 2 })).toBeVisible();
      await expectScreenPasses(p, `Sua vez, a aba Conversa da coluna ${where}`);
      await p.getByRole('tab', { name: /^Ações/ }).first().click();
    }
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await expect(p.getByText('Ainda tem ação disponível. Encerrar mesmo?')).toBeVisible();
    await expectScreenPasses(p, `Encerrar turno com a ação livre ${where}`);
    await p.getByRole('button', { name: 'Voltar' }).click();

    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    // The target is tapped on the map first (C7-04, C7-05): the map with its rings and the bar (a card in the rail from 1024 px), then the list.
    await expect(p.getByRole('region', { name: 'Escolha do alvo' })).toBeVisible();
    await expectScreenPasses(p, `Atacar, escolher o alvo no mapa ${where}`);
    await p.getByRole('region', { name: 'Escolha do alvo' }).getByRole('button', { name: 'Escolher na lista' }).click();
    await expect(p.getByRole('heading', { name: 'Atacar com Raio de Fogo' })).toBeVisible();
    await expectScreenPasses(p, `Atacar, escolher o alvo ${where}`);
    await p.locator('label', { hasText: 'Goblin 1' }).click();
    await expect(p.getByRole('button', { name: 'Digitar o resultado' })).toBeVisible();
    await expectScreenPasses(p, `Atacar, rolar ${where}`);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expectScreenPasses(p, `Rolagem física, vazia ${where}`);
    await p.getByLabel(/Role 1d20/).fill('27');
    await expect(p.getByRole('alert').filter({ hasText: 'Digite um número de 1 a 20' })).toBeVisible();
    await expectScreenPasses(p, `Rolagem física, número fora de 1 a 20 ${where}`);
    await p.getByLabel(/Role 1d20/).fill('16');
    await expect(p.getByText('22 · dado físico').or(p.getByText('16 + 6 = 22 · dado físico'))).toBeVisible();
    await expectScreenPasses(p, `Rolagem física, número válido ${where}`);
    await p.getByRole('button', { name: 'Confirmar 16' }).click();
    await expect(p.locator('.pill', { hasText: 'Acertou' })).toBeVisible();
    await expectScreenPasses(p, `Atacar, o d20 e o dano a rolar ${where}`);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d10/).fill('9');
    await p.getByRole('button', { name: 'Confirmar 9' }).click();
    await expect(p.getByRole('button', { name: 'Voltar à sua vez' })).toBeFocused();
    await expectScreenPasses(p, `Atacar, resultado ${where}`);
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expectScreenPasses(p, `Sua vez, depois de atacar ${where}`);
    await openActionTab(p, 'Ataques');
    await expect(p.getByRole('button', { name: 'Ação · Usada', exact: true })).toHaveAttribute('aria-expanded', 'false');
    await expectScreenPasses(p, `Sua vez, o grupo da Ação usado e fechado ${where}`);
    await openSpentGroup(p, 'Ação');
    await expectScreenPasses(p, `Sua vez, o grupo da Ação usado e aberto ${where}`);
    await openCombatLog(p);
    if (width < 768) {
      await expectScreenPasses(p, `Registro do combate, folha ${where}`);
      await p.keyboard.press('Escape');
    } else {
      await expectScreenPasses(p, `Registro do combate, a dobra aberta ${where}`);
    }

    // The master: the captain's card, the roll with the armor class, the damage and the questions.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await openSessionPage(m, campaignId);
    await expect(m.getByRole('heading', { name: /Ações do Capitão Goblin|Vez do Capitão Goblin/ })).toBeVisible();
    await expectScreenPasses(m, `Cartão do mestre, antes de rolar ${where}`);
    await m.getByRole('button', { name: 'Digitar o resultado' }).click();
    await m.getByLabel(/Role 1d20/).fill('18');
    await m.getByRole('button', { name: 'Confirmar 18' }).click();
    await expect(m.getByText(/contra CA \d+ da Pensantus|contra CA \d+ do Pensantus/)).toBeVisible();
    // Pensantus can cast Escudo: a hit that is not critical waits for his reaction (E6-28b).
    await expect(m.getByRole('region', { name: 'Reações a um ataque' })).toBeVisible();
    await expect(m.getByRole('button', { name: 'Rolar dano' })).toBeDisabled();
    await expectScreenPasses(m, `Cartão do mestre, esperando a reação (Escudo) ${where}`);
    await m.getByRole('region', { name: 'Reações a um ataque' }).getByRole('button', { name: 'Deixar passar pelo jogador' }).click();
    await m.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(m.getByRole('button', { name: /Aplicar \d+ de dano/ })).toBeVisible();
    await expectScreenPasses(m, `Cartão do mestre, dano para aplicar ${where}`);
    await m.getByRole('button', { name: 'Não aplicar' }).click();
    await expect(m.getByText(/Descartar o dano de \d+\?/)).toBeVisible();
    await expect(m.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await expectScreenPasses(m, `Cartão do mestre, descartar o dano ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await m.getByRole('button', { name: 'Próximo turno' }).click();
    await expect(m.getByText('Há dano sem aplicar. Passar o turno mesmo assim?')).toBeVisible();
    await expectScreenPasses(m, `Próximo turno com dano sem aplicar ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await m.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    await expect(m.getByText(/Dano de \d+ aplicado\./).first()).toBeVisible();
    // "Mais do combate": the rows open in place in the rail.
    await openCombatPanel(m, 'Efeitos');
    await expectScreenPasses(m, `Mais do combate, Efeitos aberto ${where}`);
    await openCombatPanel(m, 'Registro');
    await expectScreenPasses(m, `Mais do combate, Registro aberto ${where}`);
    await m.getByRole('button', { name: 'Desfazer última ação' }).first().click();
    await expect(m.getByText(/Desfazer o ataque do Capitão Goblin/)).toBeVisible();
    await expectScreenPasses(m, `Desfazer a última ação ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await openOrderRow(m, 'Goblin 1');
    await m.getByRole('button', { name: 'Dano/Cura: Goblin 1' }).click();
    await expect(m.getByRole('heading', { name: 'Dano ou cura em Goblin 1' })).toBeVisible();
    await expectScreenPasses(m, `Dano/Cura de um NPC ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('agir no combate passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanActionScreens(browser, 'light', 1280);
});

test('agir no combate passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanActionScreens(browser, 'dark', 390);
});

async function checkTurnAboveTheFold(browser: Browser): Promise<void> {
  const viewport = { width: 390, height: 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Ações acima da dobra ${Date.now()}`, true, true);
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();

    // The shapes of the turn (the Ação, the Bônus, the Reação) are one compact row in the card; the attack is on the first screen.
    const shapes = p.getByRole('list', { name: 'O que tem neste turno' }).getByRole('listitem');
    await expect(shapes).toHaveCount(3);
    const row = await Promise.all((await shapes.all()).map((t) => boxOf(t)));
    expect(new Set(row.map((b) => Math.round(b.y))).size).toBe(1);
    expect(Math.max(...row.map((b) => b.height))).toBeLessThanOrEqual(80);
    await expect(p.getByRole('button', { name: 'Atacar com Raio de Fogo' })).toBeInViewport();
    // The first action row ends above the sticky bar, not behind it (8px of air).
    const firstRow = await boxOf(p.locator('app-action-row:visible').first());
    const bar = await boxOf(p.locator('app-turn-bar'));
    expect(firstRow.y + firstRow.height + 8).toBeLessThanOrEqual(bar.y);

    // The action block comes before the order of initiative.
    const actions = await boxOf(p.getByRole('heading', { name: 'O que você pode fazer' }));
    const order = await boxOf(p.getByRole('list', { name: 'Ordem de iniciativa' }));
    expect(actions.y).toBeLessThan(order.y);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('no celular de 390 x 844 as ações da vez ficam acima da dobra, logo sob o título, e a ordem vem depois', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await checkTurnAboveTheFold(browser);
});

/** The player's compact combat (MR-013, MR-014; boards C4-03, C6-07, C6-10): off turn, then on the turn the action tabs one by
 * one, the sticky bar of a phone or the turn bar of 1280 with exactly one "Mover" and one "Encerrar turno" in sight, the map's
 * "Ver mapa" icon, and the groups that are spent, folded and opened. */
async function scanCompactTurnScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade turno compacto ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Pensantus: 15, 'Capitão Goblin': 20, 'Goblin 1': 5, 'Goblin 2': 4 });

    // Off turn: the card says whose turn it is; from 1024px the rail says what the player still has, a phone has the line.
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Vez do Capitão Goblin' })).toBeVisible();
    if (width >= 1024) {
      await expect(p.getByRole('complementary', { name: 'Ações' })).toContainText('Reação · Disponível');
    } else {
      await expect(p.getByText('Sua reação: Disponível.')).toBeVisible();
    }
    await expectScreenPasses(p, `Compacto, fora da vez ${where}`);

    // The player's turn: one "Mover" and one "Encerrar turno" in sight, the tab list, and "Ver mapa" as an icon at the map's top.
    await waitForEncounterRevision(p, await passTurnsTo(m, campaignId, 'Pensantus'));
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await expect(p.getByRole('button', { name: 'Mover', exact: true })).toHaveCount(1);
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(1);
    await expect(p.getByRole('button', { name: 'Ver mapa' })).toHaveCount(1);
    await expect(p.getByRole('complementary', { name: 'Ações' })).toBeVisible();
    await expectScreenPasses(p, width >= 1024 ? `Compacto, a barra do turno ${where}` : `Compacto, a barra fixa ${where}`);
    for (const label of ['Ataques', 'Magias', 'Outras']) {
      await openActionTab(p, label);
      await expectScreenPasses(p, `Compacto, a aba ${label} ${where}`);
    }

    // The Action spent: its group is one folded line in every tab, and opens.
    await p.getByRole('button', { name: 'Disparada' }).click();
    for (const label of ['Outras', 'Magias', 'Ataques']) {
      await openActionTab(p, label);
      await expect(p.getByRole('button', { name: 'Ação · Usada', exact: true })).toHaveAttribute('aria-expanded', 'false');
      await expectScreenPasses(p, `Compacto, a aba ${label} com o grupo usado e fechado ${where}`);
      await openSpentGroup(p, 'Ação');
      await expectScreenPasses(p, `Compacto, a aba ${label} com o grupo usado e aberto ${where}`);
    }
    await expect(p.getByRole('button', { name: 'Mover', exact: true })).toHaveCount(1);
    await expect(p.getByRole('button', { name: 'Encerrar turno' })).toHaveCount(1);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

for (const [scheme, width] of [['light', 1280], ['dark', 390], ['dark', 1280], ['light', 390]] as const) {
  test(`o turno compacto do jogador passa no axe e nas conferências de layout no tema ${scheme === 'light' ? 'claro' : 'escuro'}, em ${width}px`, { tag: ['@a11y', '@MR-013', '@MR-014'] }, async ({ browser }) => {
    test.setTimeout(240_000);
    await scanCompactTurnScreens(browser, scheme, width);
  });
}

/** Moving by the circle, jumping, cover and the opportunity attacks (Etapa 9,
 * slice 9.15; E9-05, E9-06, E9-07, E9-13): the "Mover" page with nothing chosen,
 * with a cost and a warning, with a wall refused, "Saltar" (distance, then
 * height), the target list with its cover, the master's order with the mark open,
 * the master's prompt, the waiting mover, and the player's `alertdialog`. The
 * squares are chosen with the arrows under the map, which every width has. */
async function scanMoveScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  const nudge = async (name: string, times = 1) => {
    for (let i = 0; i < times; i++) {
      await p.getByRole('button', { name }).click();
    }
  };
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade movimento ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    await paintRPC(m, table, 'MAP_LAYER_DIFFICULT_TERRAIN', 1, [[4, 7]]);
    await paintRPC(m, table, 'MAP_LAYER_WALL', 1, [[5, 5]]);
    await paintRPC(m, table, 'MAP_LAYER_COVER', 1, [[7, 8]]);
    await beginAttackCombatRPC(
      m,
      table,
      { Pensantus: 20, 'Goblin 1': 15, 'Capitão Goblin': 10, 'Goblin 2': 4 },
      { 'Capitão Goblin': [9, 9], 'Goblin 1': [6, 7], 'Goblin 2': [15, 11] },
    );

    // The player's turn: "Mover" with nothing chosen, then a square with a cost and the warning.
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await p.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(p.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await expectScreenPasses(p, `Mover, nada escolhido ${where}`);
    await nudge('Um quadrado para a esquerda');
    await expect(p.getByText('Mover 3,0 m', { exact: true })).toBeVisible();
    await expect(p.getByText('Sair do alcance do Goblin 1 pode provocar um ataque de oportunidade.')).toBeVisible();
    await expectScreenPasses(p, `Mover, custo e aviso de ataque de oportunidade ${where}`);
    await nudge('Um quadrado para a direita');
    await nudge('Um quadrado para cima', 2);
    await expect(p.getByRole('alert').filter({ hasText: 'Sem caminho reto' })).toBeVisible();
    await expectScreenPasses(p, `Mover, parede recusada ${where}`);

    // Saltar: the limits and the circle, then the height.
    await pickRadio(p, 'Saltar');
    await expect(p.getByRole('heading', { name: 'Saltar Pensantus' })).toBeVisible();
    await expectScreenPasses(p, `Saltar, distância ${where}`);
    await pickRadio(p, 'Altura');
    await expect(p.getByRole('button', { name: 'Aumentar a altura em 0,3 m' })).toBeVisible();
    await expectScreenPasses(p, `Saltar, altura ${where}`);
    await pickRadio(p, 'Andar');

    // The move that provokes: the turn waits for the master, who has the prompt.
    await nudge('Um quadrado para a esquerda');
    await p.getByRole('button', { name: 'Mover para cá' }).click();
    await expect(p.getByRole('status').filter({ hasText: 'Esperando o mestre.' })).toBeVisible();
    await expectScreenPasses(p, `Sua vez, esperando a reação do mestre ${where}`);
    const card = m.getByRole('group', { name: 'Ataque de oportunidade de Goblin 1' });
    await expect(card.getByRole('button', { name: 'Não atacar' })).toBeFocused();
    await expectScreenPasses(m, `Ataque de oportunidade, a pergunta do mestre ${where}`);
    await card.getByRole('button', { name: 'Não atacar' }).click();
    await expect(card).toHaveCount(0);

    // Cover: the target list, and the master's order with the mark in place.
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    await viaTargetList(p, p.getByRole('dialog'));
    await expect(p.locator('label', { hasText: 'Capitão Goblin' })).toContainText('Meia cobertura (do mapa)');
    await expectScreenPasses(p, `Atacar, alvos com cobertura ${where}`);
    await p.getByRole('button', { name: 'Fechar' }).click();
    const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
    await openOrderRow(m, 'Capitão Goblin');
    await expect(order.getByText('Meia cobertura (do mapa) contra o Pensantus').first()).toBeVisible();
    await expectScreenPasses(m, `Ordem do mestre, com a cobertura contra quem tem a vez ${where}`);
    await openOrderRow(m, 'Capitão Goblin');
    await order.getByRole('button', { name: 'Marcar cobertura de Capitão Goblin' }).click();
    await expect(order.getByRole('radiogroup', { name: 'Cobertura marcada de Capitão Goblin' })).toBeVisible();
    await expectScreenPasses(m, `Marcar cobertura, no lugar ${where}`);
    await pickRadio(order, 'Três quartos');
    await expect(order.getByText('Três quartos (marcada pelo mestre) contra o Pensantus')).toBeVisible();
    await order.getByRole('button', { name: 'Fechar' }).click();
    await openOrderRow(m, 'Goblin 2');
    await order.getByRole('button', { name: 'Mais ações para Goblin 2' }).click();
    await m.getByRole('menuitem', { name: 'Marcar como aliado' }).click();
    await expect(order.getByText('Aliado')).toBeVisible();
    await expectScreenPasses(m, `Ordem do mestre, com "Aliado" e a marca ${where}`);

    // The player's `alertdialog`: Goblin 1 has the turn and leaves Pensantus's reach.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    // On a phone the opened rows left "Turno" for the tab "Ordem".
    await openTurn(m);
    await expect(m.getByText('Vez do Goblin 1')).toBeVisible();
    // It steps next to Pensantus (who moved away), then out of his reach again.
    const now = await getEncounterRPC(m, campaignId);
    const goblin = now.combatants.find((c) => c.label === 'Goblin 1')!.id;
    for (const col of [5, 9]) {
      await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: now.id, combatantId: goblin, col, row: 7 });
    }
    const prompt = p.getByRole('alertdialog', { name: 'Ataque de oportunidade' });
    await expect(prompt.getByRole('button', { name: 'Não atacar' })).toBeFocused();
    await expectScreenPasses(p, `Ataque de oportunidade, o aviso do jogador ${where}`);
    await expectScreenPasses(m, `Ataque de oportunidade, esperando um jogador ${where}`);
    await prompt.getByRole('button', { name: 'Não atacar' }).click();
    await expect(prompt).toHaveCount(0);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('mover, saltar, a cobertura e o ataque de oportunidade passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-034'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanMoveScreens(browser, 'light', 1280);
});

test('mover, saltar, a cobertura e o ataque de oportunidade passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-034'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanMoveScreens(browser, 'dark', 390);
});

test('mover e a pergunta do ataque de oportunidade passam no axe no tema claro, no celular de 320', { tag: ['@a11y', '@MR-034'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanMoveScreens(browser, 'light', 320);
});

/** Casting, the fallen, Escudo, conditions and the master's other amount (Etapa 6,
 * slice 6.5c; E6-09, E6-13, E6-28 to E6-31): every step of the cast sheet, the
 * Escudo Arcano prompt and its answer, the conditions dialog with its tags, the
 * player's concentration line, the opportunity-attack sheet, "Aplicar outro
 * valor" with the concentration reminder, the death saves (stable, then three
 * failures) and the question that confirms a death. */
async function scanCastingScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade conjurar ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    await setTableRulesRPC(m, campaignId, { showSaveDc: true }); // the concentration question below says the DC
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
    await adjustVitalsRPC(m, campaignId, table.characterId, { spellSlotsUsed: [{ level: 1, used: 3 }, { level: 2, used: 2 }] });
    // Pensantus stands next to Goblin 1, so the dagger reaches it for an opportunity attack.
    const begun = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: begun.id, combatantId: begun.combatants.find((c) => c.label === 'Pensantus')!.id, col: 8, row: 9 });
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    // The cast sheet: the slot and the darts, then the result and the damage to roll.
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Mísseis Mágicos' }).click();
    // The darts are tapped on the map first (C7-04): two on one creature, the third on another.
    await expect(p.getByRole('region', { name: 'Escolha do alvo' })).toBeVisible();
    await p.getByRole('button', { name: /^Escolher Capitão Goblin/ }).click();
    await p.getByRole('button', { name: /^Pôr outro dardo em Capitão Goblin/ }).click();
    await expectScreenPasses(p, `Conjurar, os dardos no mapa ${where}`);
    await p.getByRole('region', { name: 'Escolha do alvo' }).getByRole('button', { name: 'Escolher na lista' }).click();
    await expect(p.getByText('É o seu último espaço de 1º nível: depois dele, o Escudo Arcano fica sem espaço.')).toBeVisible();
    await expectScreenPasses(p, `Conjurar, o espaço e os dardos ${where}`);
    const more = (who: string) => p.getByRole('button', { name: `Pôr um dardo em ${who}` });
    await more('Capitão Goblin').click();
    await more('Capitão Goblin').click();
    await more('Goblin 1').click();
    await expect(p.getByText('3 de 3 dardos distribuídos.')).toBeVisible();
    await expectScreenPasses(p, `Conjurar, os três dardos distribuídos ${where}`);
    await p.getByRole('dialog').getByRole('button', { name: 'Conjurar Mísseis Mágicos' }).click();
    await expect(p.getByText('Falta rolar o dano.').first()).toBeVisible();
    await expectScreenPasses(p, `Conjurar, resultado com dano a rolar ${where}`);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 2d4/).fill('9');
    await expect(p.getByRole('alert').filter({ hasText: 'Digite um número de 2 a 8' })).toBeVisible();
    await expect(p.getByRole('button', { name: /Confirmar/ })).toBeInViewport({ ratio: 1 });
    await expect(p.getByLabel(/Role 2d4/)).toBeInViewport({ ratio: 1 });
    await expectScreenPasses(p, `Conjurar, rolagem do dano inválida ${where}`);
    await p.getByLabel(/Role 2d4/).fill('4');
    await p.getByRole('button', { name: 'Confirmar 4' }).click();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d4/).fill('2');
    await p.getByRole('button', { name: 'Confirmar 2' }).click();
    await expect(p.getByRole('button', { name: 'Voltar à sua vez' })).toBeFocused();
    await expectScreenPasses(p, `Conjurar, resultado final ${where}`);
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();

    // The conditions: the dialog (the master) and the tags on both screens.
    await openOrderRow(m, 'Goblin 1');
    await m.getByRole('button', { name: 'Mais ações para Goblin 1' }).click();
    await m.getByRole('menuitem', { name: 'Mudar condições' }).click();
    await expect(m.getByRole('dialog', { name: 'Condições de Goblin 1' })).toBeVisible();
    await expectScreenPasses(m, `Condições, a janela ${where}`);
    await m.getByRole('checkbox', { name: 'Envenenado' }).check();
    await m.getByRole('checkbox', { name: 'Derrubado' }).check();
    await m.getByRole('button', { name: 'Salvar condições' }).click();
    await expect(m.getByRole('list', { name: 'Condições de Goblin 1' })).toBeVisible();
    await expectScreenPasses(m, `Condições, as etiquetas na ordem do mestre ${where}`);

    // Escudo Arcano: the prompt, its answer, and the master's card meanwhile (the slots are given back: the cast took the last one).
    await adjustVitalsRPC(m, campaignId, table.characterId, { spellSlotsUsed: [{ level: 1, used: 2 }, { level: 2, used: 0 }] });
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await openTurn(m);
    const card = m.getByRole('region', { name: /Ações do Capitão Goblin|Vez do Capitão Goblin/ });
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('11');
    await card.getByRole('button', { name: 'Confirmar 11' }).click();
    const prompt = p.getByRole('alertdialog', { name: 'Você foi atingido: usar Escudo Arcano?' });
    await expect(prompt).toBeVisible();
    await expect(prompt.getByText('Capitão Goblin · Cimitarra · Rodada 1')).toBeVisible();
    await expectScreenPasses(p, `Escudo, o aviso ${where}`);
    await expectScreenPasses(m, `Escudo, o cartão do mestre esperando ${where}`);
    await prompt.getByRole('button', { name: 'Conjurar Escudo Arcano' }).click();
    await expect(prompt.getByText('O Escudo Arcano segurou o ataque do Capitão Goblin.')).toBeVisible();
    await expectScreenPasses(p, `Escudo, o resultado ${where}`);
    // On the master's row the shield's label stays on one line, whatever the width of the screen.
    await openOrderRow(m, 'Pensantus');
    const shieldChip = m.getByRole('region', { name: 'Ordem de iniciativa' }).locator('.row__effect', { hasText: 'Escudo Arcano' });
    await expect(shieldChip).toBeVisible();
    expect((await layoutSize(shieldChip)).height).toBeLessThan(30);
    await prompt.getByRole('button', { name: 'Fechar' }).click();
    // The opened row took the phone to "Ordem"; the captain's card is in "Turno".
    await openTurn(m);
    // The captain's turn goes on: a second hit (a critical one) is damage to roll and discard; then Pensantus's turn.
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20 para Cimitarra/).fill('20');
    await card.getByRole('button', { name: 'Confirmar 20' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await card.getByRole('button', { name: 'Não aplicar' }).click();
    await card.getByRole('button', { name: 'Descartar' }).click();
    await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Pensantus'));

    // Round 2: Pensantus casts Teia (through the API) and concentrates; the line and its action are his.
    const enc2 = await getEncounterRPC(m, campaignId);
    const cast = await callRPC(p, 'meurpg.play.v1.CombatService/CastSpell', {
      campaignId,
      encounterId: enc2.id,
      casterId: enc2.combatants.find((c) => c.label === 'Pensantus')!.id,
      spellKey: 'spell:web',
      slot: { level: 2 },
      targets: [],
      idempotencyKey: crypto.randomUUID(),
    });
    expect(cast.ok(), await cast.text()).toBeTruthy();
    await expect(p.getByText('Concentrado em Teia', { exact: true })).toBeVisible();
    await expect(p.getByRole('button', { name: 'Encerrar concentração' })).toBeVisible();
    await expectScreenPasses(p, `Concentrado, a linha da vez ${where}`);
    await expectScreenPasses(m, `Concentrado, a ordem do mestre ${where}`);
    // Teia used the action, so "Encerrar turno" ends the turn without asking.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await waitTurnLeaves(m, campaignId, 'Pensantus');
    await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Capitão Goblin'));

    // Off turn: "Ataque de oportunidade" opens the attack sheet with the dagger (a melee weapon).
    await expect(p.getByRole('button', { name: /^Ataque de oportunidade (com |\()Adaga\)?$/ })).toBeVisible();
    await expectScreenPasses(p, `Sua reação, com o ataque de oportunidade ${where}`);
    await p.getByRole('button', { name: /^Ataque de oportunidade (com |\()Adaga\)?$/ }).click();
    await expect(p.getByRole('dialog', { name: /Adaga/ })).toBeVisible();
    await expectScreenPasses(p, `Ataque de oportunidade, escolher o alvo ${where}`);
    await p.getByRole('dialog').getByRole('button', { name: 'Fechar' }).click();

    // Another amount, with the concentration reminder (Pensantus concentrates on Teia).
    await openTurn(m);
    const card2 = m.getByRole('region', { name: /Ações do Capitão Goblin|Vez do Capitão Goblin/ });
    await card2.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card2.getByLabel(/Role 1d20 para Cimitarra/).fill('20');
    await card2.getByRole('button', { name: 'Confirmar 20' }).click();
    await card2.getByRole('button', { name: 'Rolar dano' }).click();
    await card2.getByRole('button', { name: 'Aplicar outro valor' }).click();
    await expectScreenPasses(m, `Aplicar outro valor ${where}`);
    await card2.getByLabel('Dano a aplicar').fill('1');
    await card2.getByRole('button', { name: 'Aplicar 1 de dano' }).click();
    await expect(card2.getByText(/1 de dano aplicado/).first()).toBeVisible();
    // The damage on a concentrating character asks its owner for the save (PM-04): the prompt, the master's card, the result.
    const save = p.getByRole('alertdialog', { name: 'Concentração em risco: teste de resistência de Constituição contra CD 10' });
    await expect(save).toBeVisible();
    await expectScreenPasses(p, `Concentração em risco, o aviso ${where}`);
    await expectScreenPasses(m, `Concentração em risco, o cartão do mestre ${where}`);
    await save.getByRole('button', { name: 'Rolar no app' }).click();
    await expect(save.getByRole('button', { name: 'Fechar' })).toBeVisible();
    await expectScreenPasses(p, `Concentração em risco, o resultado ${where}`);
    await save.getByRole('button', { name: 'Fechar' }).click();

    // The fallen, first stable: three successes.
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    for (let i = 0; i < 3; i++) {
      await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Pensantus'));
      await expect(p.getByRole('heading', { name: 'Pensantus está caído' })).toBeVisible();
      if (i === 0) {
        await expectScreenPasses(p, `Caído, antes de rolar ${where}`);
      }
      await p.getByRole('button', { name: 'Digitar o resultado' }).click();
      await p.getByLabel(/Role 1d20 para o teste contra a morte/).fill('15');
      if (i === 0) {
        await expectScreenPasses(p, `Caído, rolagem física ${where}`);
      }
      await p.getByRole('button', { name: 'Confirmar 15' }).click();
      await expect(p.getByRole('status').filter({ hasText: 'Teste contra a morte' })).toBeVisible();
      if (i === 0) {
        // The result line shows while the roll call is still running (the app marks itself busy and disables every button that
        // takes a call), and axe reads a button's colour live but its disabled state from a snapshot of the page: scanned
        // mid-way, a disabled grey is judged as a live label. Wait for the busy state to end on "Encerrar turno", which is on
        // the screen for the whole turn ("Encerrar concentração" is not: the concentration can end while the call lands).
        await expect(p.getByRole('button', { name: 'Encerrar turno' })).toBeEnabled();
        await expectScreenPasses(p, `Caído, depois de rolar ${where}`);
      }
      await endTurnOf(p, m, campaignId, 'Pensantus');
    }
    await expect(p.getByText('Pensantus está estável.')).toBeVisible();
    await expectScreenPasses(p, `Estável, fora da vez ${where}`);
    await passTurnsTo(m, campaignId, 'Pensantus');
    await expect(p.getByText(/Estável: não rola mais testes contra a morte/)).toBeVisible();
    await expectScreenPasses(p, `Estável, na vez ${where}`);

    // Healed and down again: now three failures, and the master's question.
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 5 });
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    for (let i = 0; i < 2; i++) {
      await passTurnsTo(m, campaignId, 'Capitão Goblin');
      await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Pensantus'));
      await p.getByRole('button', { name: 'Digitar o resultado' }).click();
      await p.getByLabel(/Role 1d20 para o teste contra a morte/).fill(i === 0 ? '1' : '2');
      await p.getByRole('button', { name: i === 0 ? 'Confirmar 1' : 'Confirmar 2' }).click();
      await endTurnOf(p, m, campaignId, 'Pensantus');
    }
    await expect(m.getByRole('alertdialog', { name: /Confirmar a morte/ })).toBeVisible();
    await expect(m.getByRole('alertdialog', { name: /Confirmar a morte/ })).toBeInViewport({ ratio: 1 });
    await expectScreenPasses(m, `Confirmar a morte ${where}`);
    await expectScreenPasses(p, `Caído, três falhas, para o jogador ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('conjurar, cair, o Escudo e as condições passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanCastingScreens(browser, 'light', 1280);
});

test('conjurar, cair, o Escudo e as condições passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanCastingScreens(browser, 'dark', 390);
});

/** The reaction window of an NPC (PM-04): the player's wait, the master's card of the Mago for Escudo and for
 * Contramágica, and the concentration save's prompt. */
async function scanReactionScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade reações ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    // The Mago joins at the start with the lowest face (the master cannot set an initiative once the combat runs).
    const begun = await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4, Mago: 1 }, { ...attackCombatSquares, Mago: [8, 8] }, [], ['monster:mage']);
    const mage = begun.combatants.find((c) => c.label.startsWith('Mago'))!;
    let enc = begun;
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    // Escudo of the Mago: the player's wait, then the master's card.
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    const sheet = p.getByRole('dialog', { name: 'Atacar com Raio de Fogo' });
    await viaTargetList(p, sheet);
    await sheet.locator('label', { hasText: mage.label }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20 para Raio de Fogo/).fill('10');
    await p.getByRole('button', { name: 'Confirmar 10' }).click();
    await expect(p.getByText('Esperando o mestre.').first()).toBeVisible();
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect(p.getByRole('status').filter({ hasText: 'Esperando o mestre.' })).toBeVisible();
    await expectScreenPasses(p, `Esperando o mestre, a reação de um NPC ${where}`);
    const card = m.getByRole('region', { name: mage.label });
    await expect(card.getByRole('button', { name: `Usar Escudo Arcano pelo ${mage.label}` })).toBeVisible();
    await expectScreenPasses(m, `A reação do Mago, Escudo Arcano ${where}`);
    await card.getByRole('button', { name: 'Deixar passar' }).click();
    await expect(p.getByRole('status').filter({ hasText: 'Esperando o mestre.' })).toHaveCount(0);

    // Contramágica of the Mago against Pensantus's cast, on his next turn.
    enc = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.combatants.find((c) => c.label === 'Pensantus')!.id, discardPendingDamage: true });
    enc = await passTurnsTo(m, campaignId, 'Pensantus');
    await waitForEncounterRevision(m, enc);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const cast = await callRPC(p, 'meurpg.play.v1.CombatService/CastSpell', {
      campaignId,
      encounterId: enc.id,
      casterId: id('Pensantus'),
      spellKey: 'spell:magic-missile',
      slot: { level: 1 },
      targets: [{ combatantId: id('Goblin 1'), darts: 3 }],
      idempotencyKey: crypto.randomUUID(),
    });
    expect(cast.ok(), await cast.text()).toBeTruthy();
    await expect(card.getByRole('button', { name: /Usar Contramágica/ })).toBeVisible();
    await expectScreenPasses(p, `Esperando o mestre, uma conjuração ${where}`);
    await expectScreenPasses(m, `A reação do Mago, Contramágica ${where}`);
    await card.getByRole('button', { name: 'Deixar passar' }).click();
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as reações dos NPCs passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@PM-04'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanReactionScreens(browser, 'light', 1280);
});

test('as reações dos NPCs passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@PM-04'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanReactionScreens(browser, 'dark', 390);
});

/** The fighter's turn (Etapa 6, slice 6.5c): Extra Attack's "1 ataque restante", Retomar o
 * Fôlego's sheet and Surto de Ação's note. */
async function scanFighterScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade guerreiro ${Date.now()}`, true, true, { build: toren, sheet: torenSheet });
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Toren: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 20 });
    const start = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: start.id, combatantId: start.combatants.find((c) => c.label === 'Toren')!.id, col: 8, row: 9 });
    await openSessionPage(p, campaignId);
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    await expectScreenPasses(p, `Guerreiro, a vez com as habilidades ${where}`);
    await openActionTab(p, 'Guerreiro');
    await expectScreenPasses(p, `Guerreiro, a aba da classe ${where}`);
    await openActionTab(p, 'Ataques');
    await groups.getByRole('button', { name: /^Atacar com Espada/ }).click();
    const sheet = p.getByRole('dialog', { name: /Atacar com Espada/ });
    await viaTargetList(p, sheet);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20/).fill('1');
    await p.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(p.getByText('Você ainda tem 1 ataque desta ação.')).toBeVisible();
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // The card and, on a phone, the sticky bar both say it with the Ação's shape.
    await expect(p.getByRole('listitem', { name: 'Ação, 1 ataque restante' }).first()).toBeVisible();
    await expectScreenPasses(p, `Ataque Extra, um ataque restante ${where}`);
    await openActionTab(p, 'Guerreiro');
    await groups.getByRole('button', { name: 'Usar Retomar o Fôlego' }).click();
    await expectScreenPasses(p, `Retomar o Fôlego, antes de rolar ${where}`);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d10/).fill('7');
    await expectScreenPasses(p, `Retomar o Fôlego, rolagem física ${where}`);
    await p.getByRole('button', { name: 'Confirmar 7' }).click();
    await expect(p.getByText(/\d+ PV recuperados/)).toBeVisible();
    await expectScreenPasses(p, `Retomar o Fôlego, resultado ${where}`);
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await openActionTab(p, 'Guerreiro');
    await expectScreenPasses(p, `Guerreiro, a aba da classe com o Ação bônus usado ${where}`);
    await openSpentGroup(p, 'Ação bônus');
    await expectScreenPasses(p, `Guerreiro, o Ação bônus usado aberto ${where}`);
    await groups.getByRole('button', { name: 'Usar Surto de Ação' }).click();
    await expect(groups.getByText('Surto de Ação: você tem outra ação.')).toBeVisible();
    await expectScreenPasses(p, `Surto de Ação, a nota ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o guerreiro (Ataque Extra, Retomar o Fôlego, Surto de Ação) passa no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanFighterScreens(browser, 'dark', 1280);
});

test('o guerreiro (Ataque Extra, Retomar o Fôlego, Surto de Ação) passa no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanFighterScreens(browser, 'light', 390);
});

/** The XP screens (Etapa 7, MR-016, RN-12): the end of a combat with "Dar XP" in each of its states, the
 * "Dar XP" dialog or sheet with its errors, the campaign's "Experiência" with its history and the question
 * of "Desfazer", the milestone's dialog and panel, the sheet with the XP block and the tag, and the NPC's
 * ND and XP (the list open, "Usar 50 XP"). Built in one function so the sweep is one place. */
async function scanXpScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const campaigns: string[] = [];
  /** A page that keeps its stream open never goes idle: wait for its h1 instead. */
  const openLive = async (page: Page, route: string, heading?: string) => {
    await page.goto(route);
    await expect(heading ? page.getByRole('heading', { name: heading }) : page.getByRole('heading', { level: 1 })).toBeVisible({ timeout: 30_000 });
  };
  try {
    await m.goto('/');
    await p.goto('/');

    // The end of a combat, before the XP is given.
    const combat = await tableForXpCombat(m, p, `Acessibilidade XP ${Date.now()}`, { experiencePoints: 2600 });
    const campaignId = combat.table.campaignId;
    campaigns.push(campaignId);
    await winCombatRPC(m, combat);
    await openLive(m, `/campaigns/${campaignId}/session`, 'Combate encerrado');
    const block = m.getByRole('region', { name: 'Experiência do combate' });
    await expect(block.getByRole('button', { name: /^Dar 350 XP/ })).toBeVisible();
    await expectScreenPasses(m, `Fim do combate com Dar XP ${where}`);
    await block.getByRole('checkbox', { name: 'Marcar Pensantus' }).uncheck();
    await expect(block.getByText('Marque pelo menos um personagem')).toBeVisible();
    await expectScreenPasses(m, `Fim do combate, ninguém marcado ${where}`);
    await block.getByRole('checkbox', { name: 'Marcar Pensantus' }).check();

    // "Agora não": the quiet line, and "Dar XP" opened from it, with an error.
    await block.getByRole('button', { name: 'Agora não' }).click();
    await expect(block.getByRole('button', { name: /XP do combate ainda não dado/ })).toBeFocused();
    await expectScreenPasses(m, `Fim do combate, XP para depois ${where}`);
    await block.getByRole('button', { name: /XP do combate ainda não dado/ }).click();
    const dialog = m.getByRole('dialog', { name: 'Dar XP' });
    await expect(dialog.getByLabel('Motivo')).toHaveValue('Combate: Emboscada na estrada');
    await expectScreenPasses(m, `Dar XP, aberto pelo resumo ${where}`);
    await dialog.getByLabel('Motivo').fill('');
    await dialog.getByLabel('XP para o grupo').focus();
    await dialog.getByLabel('Motivo').focus();
    await dialog.getByLabel('XP para o grupo').focus();
    await expect(dialog.getByText('Escreva o motivo do XP.')).toBeVisible();
    await expectScreenPasses(m, `Dar XP, com erro ${where}`);
    await dialog.getByLabel('Motivo').fill('Combate: Emboscada na estrada');
    await dialog.getByRole('button', { name: /^Dar 350 XP/ }).click();
    await expect(block).toContainText('350 XP dados');
    await expectScreenPasses(m, `Fim do combate, XP dado ${where}`);

    // The campaign page: the panel with its history, the question, the player's view and the sheet.
    await awardXpRPC(m, campaignId, { mode: 'MANUAL', reason: 'Pela ajuda ao ferreiro', characterIds: [combat.table.characterId], amount: 40 });
    // The campaign page of a master with a session open keeps the session's stream (MR-040): not `open`.
    await openLive(m, `/campaigns/${campaignId}`);
    await expect(m.getByRole('region', { name: 'Experiência', exact: true })).toContainText('Pela ajuda ao ferreiro');
    await expectScreenPasses(m, `Campanha com Experiência, o histórico fechado, mestre ${where}`);
    await openXpHistory(m);
    await expectScreenPasses(m, `Campanha com Experiência, o histórico aberto, mestre ${where}`);
    await m.getByRole('button', { name: /^Desfazer/ }).click();
    await expect(m.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(m, `Experiência, Desfazer a pergunta ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await m.getByRole('button', { name: 'Dar XP' }).click();
    await expect(m.getByRole('dialog', { name: 'Dar XP' })).toBeVisible();
    await m.getByRole('dialog').getByLabel('Motivo').fill('Pelo resgate do mercador');
    await m.getByRole('dialog').getByLabel('XP para o grupo').fill('150');
    await expectScreenPasses(m, `Dar XP, a qualquer hora ${where}`);
    await m.getByRole('dialog').getByRole('button', { name: 'Cancelar' }).click();
    await open(p, `/campaigns/${campaignId}`);
    await expect(p.getByRole('region', { name: 'Experiência', exact: true })).toContainText('Todos da campanha veem este histórico.');
    await expectScreenPasses(p, `Campanha com Experiência, jogador ${where}`);
    await openLive(p, `/campaigns/${campaignId}/characters/${combat.table.characterId}`);
    // The level-up block says it (MR-040), not the XP block's tag.
    await expect(p.getByRole('heading', { name: 'Pensantus pode subir de nível' })).toBeVisible();
    await expectScreenPasses(p, `Ficha com XP e Pode subir de nível ${where}`);

    // Milestones: the dialog, the panel after it and the sheet with only the tag.
    const marks = await tableForXp(m, p, `Acessibilidade marcos ${Date.now()}`, 'XP_MODE_MILESTONES');
    campaigns.push(marks.campaignId);
    await startSessionRPC(m, marks.campaignId);
    await openLive(m, `/campaigns/${marks.campaignId}`);
    await expectScreenPasses(m, `Experiência por marcos, sem marcos ${where}`);
    await m.getByRole('button', { name: 'Registrar um marco fora da lista' }).click();
    const markDialog = m.getByRole('dialog', { name: 'Registrar marco' });
    await markDialog.getByLabel('O que aconteceu').fill('Marco: a ponte do rio foi salva');
    await expectScreenPasses(m, `Registrar marco ${where}`);
    await markDialog.getByRole('button', { name: 'Registrar marco' }).click();
    await expect(m.getByRole('status').filter({ hasText: 'Marco registrado' })).toBeVisible();
    await expectScreenPasses(m, `Experiência por marcos, depois do marco ${where}`);
    await openLive(p, `/campaigns/${marks.campaignId}/characters/${marks.characterId}`);
    await expect(p.getByRole('heading', { name: /pode subir de nível/ })).toBeVisible();
    await expectScreenPasses(p, `Ficha por marcos, o bloco de subir de nível ${where}`);

    // The NPC: the minion's section with the list open, "Usar 50 XP", and the enemy's header fields.
    const npc = await createEnemyRPC(m, campaignId, 'Capitão Goblin', '1', 200);
    await open(m, `/campaigns/${campaignId}/npcs/new/minion`);
    await expectScreenPasses(m, `NPC curto com Ao ser derrotado ${where}`);
    const nd = m.getByRole('combobox', { name: 'Nível de desafio (ND)' });
    await nd.click();
    await expect(m.getByRole('option', { name: /^ND 1\/4/ })).toBeVisible();
    await expectScreenPasses(m, `NPC curto, lista de ND ${where}`);
    await m.getByRole('option', { name: /^ND 1\/4/ }).click();
    await m.getByLabel('XP ao derrotar').fill('0');
    await expect(m.getByRole('button', { name: 'Usar 50 XP' })).toBeVisible();
    await expectScreenPasses(m, `NPC curto, XP zero com Usar ${where}`);
    await open(m, `/campaigns/${campaignId}/characters/${npc}/edit`);
    await expect(m.getByLabel('XP ao derrotar')).toHaveValue('200');
    await expectScreenPasses(m, `Inimigo, ND e XP no passo Básico ${where}`);
    await openLive(m, `/campaigns/${campaignId}/characters/${npc}`);
    await expectScreenPasses(m, `Ficha do inimigo com ND e XP ${where}`);
  } finally {
    for (const id of campaigns) {
      await endOpenSessionRPC(m, id);
    }
    await master.close();
    await player.close();
  }
}

test('as telas de XP passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-016'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanXpScreens(browser, 'light', 1280);
});

test('as telas de XP passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-016'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanXpScreens(browser, 'dark', 390);
});

/**
 * The RP scenes (Etapa 7, MR-015; E7-01 to E7-05): the point panel with its
 * actions (the list, the add form, the DC error, the empty state, the full
 * list; on a computer), the master's "Cena de RP" and the picker, the open
 * scene with its rolls, and the player's scene block, roll sheet in each of
 * its states (how, typed, the number out of 1 to 20, the result), the rolled
 * row and the page without a scene. The rolls of the master's screen are made
 * through the API so the screens are the same on every run.
 */
async function scanSceneScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade cenas ${Date.now()}`);
    campaignId = table.campaignId;
    await attachTwoImagesRPC(m, table, table.cartId);

    // The editor is for a computer: a phone has the lists of points instead.
    if (width >= 768) {
      await open(m, `/campaigns/${campaignId}/maps/${table.mapId}`);
      await m.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(m.getByRole('heading', { name: 'Ações da cena' })).toBeVisible();
      await expectScreenPasses(m, `Ações da cena no ponto ${where}`);
      await expect(m.getByRole('heading', { name: 'Imagens da cena' })).toBeVisible();
      await m.getByRole('heading', { name: 'Imagens da cena' }).scrollIntoViewIfNeeded();
      await expectScreenPasses(m, `Imagens da cena no ponto, com duas imagens ${where}`);
      await m.getByRole('button', { name: 'Adicionar ação' }).click();
      await expect(m.getByRole('form', { name: 'Nova ação' })).toBeVisible();
      await expectScreenPasses(m, `Nova ação ${where}`);
      await m.getByLabel('CD (opcional)').fill('31');
      await m.getByRole('form', { name: 'Nova ação' }).getByRole('button', { name: 'Adicionar ação' }).click();
      await expect(m.getByText('A CD vai de 1 a 30.')).toBeVisible();
      await expectScreenPasses(m, `Nova ação com a CD fora de 1 a 30 ${where}`);
      await m.getByRole('button', { name: 'Cancelar' }).click();
      await m.getByRole('button', { name: /^Vau do riacho, Cena de RP/ }).click();
      await expect(m.getByText('Nenhuma ação ainda')).toBeVisible();
      await expectScreenPasses(m, `Ações da cena, vazia ${where}`);
      for (let i = 0; i < 20; i++) {
        await addActionRPC(m, table, table.fordId, { key: 'skill:arcana' });
      }
      await open(m, `/campaigns/${campaignId}/maps/${table.mapId}`);
      await m.getByRole('button', { name: /^Vau do riacho, Cena de RP/ }).click();
      await expect(m.getByText('Limite de 20 ações. Remova uma para adicionar outra.')).toBeVisible();
      await expectScreenPasses(m, `Ações da cena, lista cheia ${where}`);
    }

    // The session: "Cena de RP", the picker, the open scene with its rolls.
    await openSessionPage(m, campaignId);
    await expect(m.getByRole('heading', { name: 'Cena de RP' })).toBeVisible();
    await expectScreenPasses(m, `Sessão com "Cena de RP" ${where}`);
    await m.getByRole('button', { name: 'Abrir cena', exact: true }).click();
    await expect(m.getByRole('dialog', { name: 'Abrir uma cena' })).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Abrir uma cena ${where}`);
    await m.getByRole('dialog').getByText('A carroça tombada', { exact: true }).click();
    await m.getByRole('dialog').getByRole('button', { name: 'Abrir cena', exact: true }).click();
    await expect(m.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeFocused();
    // The scene's pictures, with "Mostrar aos jogadores", then one on show.
    const sceneImages = m.getByRole('region', { name: 'Imagens da cena' });
    await expect(sceneImages).toBeVisible();
    await expectScreenPasses(m, `Cena aberta com as imagens da cena ${where}`);
    await sceneImages.getByRole('button', { name: 'Mostrar Vista da carroça aos jogadores' }).click();
    await expect(sceneImages.getByText('À mostra agora')).toBeVisible();
    await expectScreenPasses(m, `Cena aberta com uma imagem à mostra ${where}`);
    await m.getByRole('button', { name: 'Parar de mostrar', exact: true }).click();
    await expect(sceneImages.getByText('À mostra agora')).toHaveCount(0);

    // The player: the block, the roll sheet in each state, the rolled row.
    await openSessionPage(p, campaignId);
    const scene = p.getByRole('region', { name: 'Cena: A carroça tombada' });
    await expect(scene).toBeVisible();
    await expectScreenPasses(p, `Cena do jogador ${where}`);
    await scene.getByRole('button', { name: 'Rolar Procurar pistas na carroça' }).click();
    const sheet = p.getByRole('dialog', { name: 'Rolar Procurar pistas na carroça' });
    await expect(sheet.getByRole('button', { name: 'Rolar no app' })).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Rolar a ação, no app ${where}`);
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(sheet.getByRole('heading', { name: 'Digite o resultado do dado' })).toBeVisible();
    await expectScreenPasses(p, `Rolar a ação, digitando ${where}`);
    await sheet.getByLabel(/Role 1d20 para Investigação/).fill('27');
    await expect(sheet.getByRole('alert')).toBeVisible();
    await expectScreenPasses(p, `Rolar a ação, número fora de 1 a 20 ${where}`);
    await sheet.getByLabel(/Role 1d20 para Investigação/).fill('11');
    await expect(sheet.getByRole('status')).toContainText('11 + 6 = 17');
    await expectScreenPasses(p, `Rolar a ação, número valido ${where}`);
    await sheet.getByRole('button', { name: 'Confirmar 11' }).click();
    await expect(sheet.getByText('Seu total em Investigação')).toBeVisible();
    await expectScreenPasses(p, `Rolar a ação, resultado ${where}`);
    await sheet.getByRole('button', { name: 'Voltar à cena' }).click();
    await expect(scene.getByText('Rolada')).toBeVisible();
    await expectScreenPasses(p, `Cena do jogador, uma ação rolada ${where}`);

    // The compact scene (C5-01, C6-08, C6-09): the image on show first, "Ficha" inside the session, the folds.
    await sceneImages.getByRole('button', { name: 'Mostrar Vista da carroça aos jogadores' }).click();
    await expect(p.getByRole('region', { name: 'O mestre está mostrando' })).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Cena do jogador com a imagem à mostra ${where}`);
    const sheetPanel = await openSheetTab(p);
    await expect(sheetPanel.getByRole('heading', { name: 'Magias' })).toBeVisible();
    await expect(sheetPanel.getByRole('button', { name: 'Conjurar fora do combate' })).toBeVisible();
    await expectScreenPasses(p, `Ficha do jogador, ${width >= 1024 ? 'na coluna' : width >= 768 ? 'sob a cena' : 'no painel'} ${where}`);
    if (width < 768) {
      await p.getByRole('navigation', { name: 'Seções da sessão' }).getByRole('button', { name: 'Cena' }).click();
    } else if (width >= 1024) {
      await openChatTab(p);
    }
    if (width < 768) {
      // The scene's tabs (C7-07): "Ações" is open; "Mapa" shows the map card, "Ações" again for the rest of the scan.
      const sceneTabs = p.getByRole('tablist', { name: 'Seções da cena' });
      await expect(sceneTabs.getByRole('tab', { name: 'Ações' })).toHaveAttribute('aria-selected', 'true');
      await sceneTabs.getByRole('tab', { name: 'Mapa' }).click();
      await expect(p.getByRole('tabpanel', { name: 'Mapa' }).getByRole('link', { name: 'Ver mapa' })).toBeVisible();
      await expectScreenPasses(p, `Cena do jogador, aba Mapa ${where}`);
      await sceneTabs.getByRole('tab', { name: 'Ações' }).click();
    }
    const dice = p.getByRole('region', { name: 'Rolagens da mesa' });
    await dice.getByRole('button', { name: 'Rolagens da mesa' }).click();
    await expect(dice.getByRole('button', { name: 'Rolagens da mesa' })).toHaveAttribute('aria-expanded', 'true');
    await expectScreenPasses(p, `Rolagens da mesa aberta, jogador ${where}`);
    await dice.getByRole('button', { name: 'Rolagens da mesa' }).click();
    await m.getByRole('button', { name: 'Parar de mostrar', exact: true }).click();
    await expect(p.getByRole('region', { name: 'O mestre está mostrando' })).toHaveCount(0);

    // The master with the roll and one more, with and without a DC.
    const open1 = await getOpenSceneRPC(p, campaignId);
    const ids = open1.scene!.actions.map((a) => a.id);
    await rollSceneRPC(p, campaignId, ids[3], 14);
    await rollSceneRPC(p, campaignId, ids[4], 3);
    // Each roll is a chip beside its action; opening them shows the formula and the verdict.
    await openRolls(m, 3);
    await expect(m.getByText('Não passou · CD 10')).toBeVisible();
    await expect(m.getByText('Passou · CD 12')).toBeVisible();
    await expectScreenPasses(m, `Cena aberta com três rolagens ${where}`);
    // With a scene open, the points of the map say what they do with it.
    await showMapTool(m, 'Pontos');
    await expect(m.getByText('Cena aberta agora')).toBeVisible();
    await expect(m.getByRole('button', { name: 'Trocar para esta cena: Posto da guarda' })).toBeVisible();
    await expectScreenPasses(m, `Pontos do mapa com a cena aberta ${where}`);
    await showMasterTab(m, 'cena');
    await m.getByRole('button', { name: 'Trocar cena' }).click();
    await expect(m.getByRole('dialog', { name: 'Abrir uma cena' })).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Trocar cena ${where}`);
    await m.getByRole('button', { name: 'Cancelar' }).click();

    // No scene: nothing extra for the player.
    await m.getByRole('button', { name: 'Fechar cena' }).click();
    await expect(p.getByRole('region', { name: 'Cena: A carroça tombada' })).toHaveCount(0);
    await expectScreenPasses(p, `Sessão do jogador sem cena ${where}`);
    await expectScreenPasses(m, `Sessão do mestre depois de fechar a cena ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

/**
 * "Entrar na cena" (C7-06), the player outside a scene open to only some players: no scene card at all (the switch off), the card "Há uma
 * cena aberta" with the filled "Entrar na cena" (the switch on), and the scene after entering, under "Você entrou na cena". Toren is the
 * one outside; on a phone the scene comes in tabs.
 */
async function scanEnterSceneScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  let outsider: Awaited<ReturnType<typeof secondPlayer>> | undefined;
  try {
    await Promise.all([m.goto('/'), p.goto('/')]);
    const table = await tableForScenes(m, p, `Acessibilidade entrar na cena ${Date.now()}`);
    campaignId = table.campaignId;
    outsider = await secondPlayer(browser, m, campaignId, toren, {}, { storageState: authStatePath('E-mail Não Verificado'), colorScheme, viewport });
    const out = outsider.page;
    await openSceneOnlyToRPC(m, campaignId, table.cartId, 'Pensantus');
    await openSessionPage(out, campaignId);
    await expect(out.getByRole('region', { name: /A carroça tombada/ })).toHaveCount(0);
    await expectScreenPasses(out, `Entrar na cena, o interruptor desligado ${where}`);

    await setMayJoinRPC(m, campaignId, true);
    const card = out.getByRole('region', { name: 'A carroça tombada' });
    await expect(card.getByRole('button', { name: 'Entrar na cena' })).toBeVisible();
    await out.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(out, `Entrar na cena, o cartão ${where}`);

    await card.getByRole('button', { name: 'Entrar na cena' }).click();
    await expect(out.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeFocused();
    await out.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(out, `Entrar na cena, depois de entrar ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await outsider?.close();
    await master.close();
    await player.close();
  }
}

test('"Entrar na cena" passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-015', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanEnterSceneScreens(browser, 'light', 1280);
});

test('"Entrar na cena" passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-015', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanEnterSceneScreens(browser, 'dark', 390);
});

test('as cenas de RP passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSceneScreens(browser, 'light', 1280);
});

test('as cenas de RP passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSceneScreens(browser, 'dark', 390);
});

test('as cenas de RP passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSceneScreens(browser, 'dark', 1024);
});

test('as cenas de RP passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSceneScreens(browser, 'light', 320);
});

/**
 * The planned milestones (Etapa 8, MR-016, RN-12, RN-20; E8-14): the empty panel, "Adicionar marco" open with its
 * error, the list, the edit field and the removal question in place, "Marcar como alcançado" (a dialog on a
 * computer, a sheet on a phone), the reached milestone with "Dar a mais alguém"'s absence (one character only),
 * "Desfazer"'s question, and the player's panel before and after the first milestone. Built in one function so
 * the sweep is one place.
 */
async function scanMilestoneScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width <= 390 ? 700 : 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const experience = (page: Page) => page.getByRole('region', { name: 'Experiência', exact: true });
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForXp(m, p, `Acessibilidade marcos planejados ${Date.now()}`, 'XP_MODE_MILESTONES');
    const route = `/campaigns/${table.campaignId}`;

    // Nothing planned yet, and the player's empty state.
    await open(m, route);
    await expect(experience(m)).toContainText('Nenhum marco planejado');
    await expectScreenPasses(m, `Marcos, nenhum planejado ${where}`);
    await open(p, route);
    await expect(experience(p)).toContainText('Nenhum marco alcançado ainda');
    await expectScreenPasses(p, `Marcos, jogador antes do primeiro ${where}`);

    // "Adicionar marco" open, with its error.
    await experience(m).getByRole('button', { name: 'Adicionar marco', exact: true }).click();
    await experience(m).getByLabel('Nome do marco').press('Enter');
    await expect(experience(m).getByRole('alert')).toContainText('Escreva o nome do marco');
    await expectScreenPasses(m, `Marcos, Adicionar marco com erro ${where}`);
    for (const text of ['Salvar o mercador', 'Chegar ao Vale Seco', 'Derrotar o Barão Ivo']) {
      await experience(m).getByLabel('Nome do marco').fill(text);
      await experience(m).getByLabel('Nome do marco').press('Enter');
      await expect(experience(m).getByRole('button', { name: `Marcar como alcançado: “${text}”` })).toBeVisible();
      if (text !== 'Derrotar o Barão Ivo') {
        await experience(m).getByRole('button', { name: 'Adicionar marco', exact: true }).click();
      }
    }
    await expectScreenPasses(m, `Marcos, a lista planejada ${where}`);

    // Edit in place, and the removal question.
    await experience(m).getByRole('button', { name: 'Editar Salvar o mercador' }).click();
    await expect(experience(m).getByLabel('Nome do marco')).toHaveValue('Salvar o mercador');
    await expectScreenPasses(m, `Marcos, editar no lugar ${where}`);
    await experience(m).getByRole('button', { name: 'Cancelar' }).click();
    await experience(m).getByRole('button', { name: 'Remover Salvar o mercador' }).click();
    await expect(m.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(m, `Marcos, remover a pergunta ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();

    // "Marcar como alcançado".
    await experience(m).getByRole('button', { name: 'Marcar como alcançado: “Chegar ao Vale Seco”' }).click();
    await expect(m.getByRole('dialog', { name: 'Marcar “Chegar ao Vale Seco” como alcançado' })).toBeVisible();
    await expectScreenPasses(m, `Marcar como alcançado ${where}`);
    await m.getByRole('dialog').getByRole('button', { name: 'Marcar como alcançado' }).click();
    await expect(experience(m).getByRole('status').filter({ hasText: 'Marco alcançado' })).toBeVisible();
    await expectScreenPasses(m, `Marcos, depois de alcançar ${where}`);

    // "Desfazer" asks in place.
    await experience(m).getByRole('button', { name: 'Desfazer Chegar ao Vale Seco' }).click();
    await expect(m.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(m, `Marcos, desfazer a pergunta ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();

    // The player, after the first milestone: only the reached one, and the own character.
    await open(p, route);
    await expect(experience(p)).toContainText('Chegar ao Vale Seco');
    await expectScreenPasses(p, `Marcos, jogador depois do marco ${where}`);
  } finally {
    await master.close();
    await player.close();
  }
}

test('os marcos planejados passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-016'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanMilestoneScreens(browser, 'light', 1280);
});

test('os marcos planejados passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-016'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanMilestoneScreens(browser, 'dark', 390);
});

test('os marcos planejados passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-016'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanMilestoneScreens(browser, 'light', 320);
});

/**
 * Clues, hooks and the players' notes (Etapa 8, MR-029, MR-030, E8-04 to
 * E8-07): the point panel's "Pistas" (empty, the list with who has each, the
 * add form with its error, the remove question, the full list) and "Ganchos e
 * anotações"; the open scene's clues and hooks (open and, on a phone, folded);
 * "Revelar pista" with nobody checked, one checked and after revealing; the
 * player's notice and the bar's button with a new clue; the notes sheet (empty,
 * the list with a clue, the filter open, a scene with nothing, a new note, the
 * limit of 2.000, editing with its two questions); and the panel on the
 * character sheet (list, form, empty).
 */
async function scanNotesScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade pistas ${Date.now()}`, false);
    campaignId = table.campaignId;

    // The editor is for a computer: "Pistas" and "Ganchos e anotações" in the point panel.
    if (width >= 768) {
      await open(m, `/campaigns/${campaignId}/maps/${table.mapId}`);
      await m.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(m.getByText('Nenhuma pista ainda')).toBeVisible();
      await expectScreenPasses(m, `Pistas e ganchos, vazios ${where}`);
      for (const text of cartClues) {
        await addClueRPC(m, table, table.cartId, text);
      }
      await open(m, `/campaigns/${campaignId}/maps/${table.mapId}`);
      await m.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(m.getByText('3 de 30', { exact: true })).toBeVisible();
      await m.getByRole('textbox', { name: 'Ganchos e anotações' }).fill(cartHooks);
      await expectScreenPasses(m, `Pistas e ganchos, com três pistas ${where}`);
      await m.getByRole('button', { name: 'Adicionar pista' }).click();
      await expect(m.getByRole('form', { name: 'Nova pista' })).toBeVisible();
      await expectScreenPasses(m, `Nova pista ${where}`);
      await m.getByRole('form', { name: 'Nova pista' }).getByRole('button', { name: 'Adicionar pista' }).click();
      await expect(m.getByText('Escreva a pista antes de salvar.')).toBeVisible();
      await expectScreenPasses(m, `Nova pista, com o erro ${where}`);
      await m.getByRole('form', { name: 'Nova pista' }).getByLabel('Texto da pista').fill('x'.repeat(512));
      await expect(m.getByText('512 de 500')).toBeVisible();
      await expectScreenPasses(m, `Nova pista, passou de 500 caracteres ${where}`);
      await m.getByRole('button', { name: 'Cancelar' }).click();
      await m.getByRole('button', { name: 'Remover a pista 2' }).click();
      await expect(m.getByRole('alertdialog', { name: 'Remover a pista 2?' })).toBeVisible();
      await expectScreenPasses(m, `Remover a pista, a pergunta ${where}`);
      await m.getByRole('button', { name: 'Voltar' }).click();
      await m.getByRole('button', { name: /^Vau do riacho, Cena de RP/ }).click();
      for (let i = 1; i <= 30; i++) {
        await addClueRPC(m, table, table.fordId, `Pista número ${i}`);
      }
      await open(m, `/campaigns/${campaignId}/maps/${table.mapId}`);
      await m.getByRole('button', { name: /^Vau do riacho, Cena de RP/ }).click();
      await expect(m.getByText('Limite de 30 pistas. Remova uma para adicionar outra.')).toBeVisible();
      await expectScreenPasses(m, `Pistas, lista cheia ${where}`);
    } else {
      for (const text of cartClues) {
        await addClueRPC(m, table, table.cartId, text);
      }
      const hooks = await callRPC(m, 'meurpg.maps.v1.MapService/UpdateMapPoint', { campaignId, mapId: table.mapId, pointId: table.cartId, hooks: cartHooks });
      expect(hooks.ok()).toBeTruthy();
    }
    await callRPC(m, 'meurpg.maps.v1.MapService/UpdateMapPoint', { campaignId, mapId: table.mapId, pointId: table.cartId, hooks: cartHooks });

    // The player's notes before anything arrived: the empty sheet.
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Anotações', exact: true }).click();
    await expect(p.getByText('Nenhuma anotação ainda')).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Anotações, vazias ${where}`);
    await p.getByRole('button', { name: 'Fechar' }).click();

    // The open scene with its clues and hooks, and "Revelar pista".
    await m.getByRole('button', { name: 'Abrir cena', exact: true }).click();
    await m.getByRole('dialog').getByText('A carroça tombada', { exact: true }).click();
    await m.getByRole('dialog').getByRole('button', { name: 'Abrir cena', exact: true }).click();
    await expect(m.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeFocused();
    await expectScreenPasses(m, `Cena aberta com pistas e ganchos ${where}`);
    if (width < 768) {
      await m.getByRole('button', { name: 'Ganchos e anotações' }).click();
      await expect(m.getByText(cartHooks)).toBeVisible();
      await expectScreenPasses(m, `Cena aberta, ganchos abertos ${where}`);
    }
    await m.getByRole('button', { name: 'Revelar a pista 2' }).click();
    await expect(m.getByRole('dialog', { name: 'Revelar pista' })).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Revelar pista, ninguém marcado ${where}`);
    await m.getByRole('button', { name: 'Marcar todos' }).click();
    await expect(m.getByRole('button', { name: 'Revelar para Pensantus' })).toBeVisible();
    await expectScreenPasses(m, `Revelar pista, uma pessoa marcada ${where}`);
    await m.getByRole('button', { name: 'Revelar para Pensantus' }).click();
    await expect(m.getByText(/Pista revelada para todos às/)).toBeVisible();
    await expectScreenPasses(m, `Cena aberta, depois de revelar ${where}`);

    // The player: the notice, the bar's button with a new clue, the sheet.
    await expect(p.getByText('O mestre revelou uma pista para você.')).toBeVisible();
    await expect(p.getByRole('button', { name: 'Anotações, 1 nova' })).toBeVisible();
    await expectScreenPasses(p, `Sessão do jogador com a pista nova ${where}`);
    await p.getByRole('button', { name: 'Abrir anotações' }).click();
    const sheet = p.getByRole('dialog');
    await expect(sheet.getByText('Pista do mestre')).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Anotações, só a pista ${where}`);
    await sheet.getByRole('button', { name: 'Nova anotação' }).click();
    await expect(sheet.getByRole('heading', { name: 'Nova anotação' })).toBeVisible();
    await expectScreenPasses(p, `Nova anotação ${where}`);
    await sheet.getByLabel('Anotação', { exact: true }).fill('x'.repeat(2000));
    await expect(sheet.getByText('Chegou ao limite de 2.000 caracteres.')).toBeVisible();
    await expectScreenPasses(p, `Nova anotação, no limite de 2.000 ${where}`);
    await sheet.getByLabel('Anotação', { exact: true }).fill('Perguntar ao ferreiro sobre o brasão de lobo');
    await sheet.getByRole('combobox', { name: /Cena \(opcional\)/ }).click();
    await expect(p.getByRole('option', { name: 'A carroça tombada' })).toBeVisible();
    await expectScreenPasses(p, `Nova anotação, escolhendo a cena ${where}`);
    await p.getByRole('option', { name: 'A carroça tombada' }).click();
    await sheet.getByRole('button', { name: 'Cancelar' }).click();
    await expect(sheet.getByRole('alertdialog', { name: 'Descartar o que você escreveu?' })).toBeVisible();
    await expectScreenPasses(p, `Nova anotação, descartar ${where}`);
    await sheet.getByRole('button', { name: 'Continuar' }).click();
    await sheet.getByRole('button', { name: 'Salvar anotação' }).click();
    await expect(sheet.getByRole('heading', { name: 'Anotações' })).toBeVisible();
    await expectScreenPasses(p, `Anotações, uma nota e uma pista ${where}`);
    await sheet.getByRole('combobox', { name: /^Cena/ }).click();
    await expect(p.getByRole('option', { name: /Todas as anotações/ })).toBeVisible();
    await expectScreenPasses(p, `Anotações, o filtro por cena aberto ${where}`);
    await p.getByRole('option', { name: /Sem cena/ }).click();
    await expect(sheet.getByText('Nada sem cena')).toBeVisible();
    await expectScreenPasses(p, `Anotações, filtro sem resultado ${where}`);
    // Reopened only once the list that faded out is gone (see notes.spec.ts).
    await expect(p.getByRole('listbox')).toHaveCount(0);
    await sheet.getByRole('combobox', { name: /^Cena/ }).click();
    await p.getByRole('option', { name: /Todas as anotações/ }).click();
    await expect(p.getByRole('listbox')).toHaveCount(0);
    await sheet.getByRole('button', { name: /^Editar a anotação/ }).click();
    await sheet.getByRole('button', { name: 'Apagar anotação' }).click();
    await expect(sheet.getByRole('alertdialog', { name: 'Apagar esta anotação?' })).toBeVisible();
    await expectScreenPasses(p, `Editar anotação, apagar ${where}`);
    await sheet.getByRole('button', { name: 'Voltar' }).click();
    await sheet.getByRole('button', { name: 'Cancelar' }).click();
    await sheet.getByRole('button', { name: 'Fechar' }).click();

    // The panel on the player's sheet; the master's own sheet page has none.
    await createNoteRPC(p, campaignId, 'Brisa me deve 5 PO');
    // Not `open()`: with a session open the sheet follows its stream, so the network is never idle.
    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    const panel = p.getByRole('region', { name: 'Anotações' });
    // Under 1200px the notes are one "Anotações (N)" row that opens on a tap (no such button on the four-column sheet).
    await expectScreenPasses(p, `Ficha com as anotações, primeira vista ${where}`);
    for (const toggle of await panel.getByRole('button', { name: /^Anotações \(\d+\)/ }).all()) {
      await toggle.click();
    }
    await expect(panel.getByText('Brisa me deve 5 PO')).toBeVisible();
    await expectScreenPasses(p, `Ficha com as anotações ${where}`);
    await panel.getByRole('button', { name: 'Nova anotação' }).click();
    await expect(panel.getByRole('heading', { name: 'Nova anotação' })).toBeVisible();
    await expectScreenPasses(p, `Ficha, nova anotação ${where}`);
    await panel.getByRole('button', { name: 'Cancelar' }).click();
    await m.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(m.getByRole('heading', { name: 'Pensantus' }).first()).toBeVisible();
    await expect(m.getByRole('heading', { name: 'Anotações' })).toHaveCount(0);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('pistas, ganchos e anotações passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-029', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanNotesScreens(browser, 'light', 1280);
});

test('pistas, ganchos e anotações passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-029', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanNotesScreens(browser, 'dark', 390);
});

test('pistas, ganchos e anotações passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-029', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanNotesScreens(browser, 'dark', 1024);
});

test('pistas, ganchos e anotações passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-029', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanNotesScreens(browser, 'light', 320);
});

/** The player's sheet: the game numbers first on a phone and a tablet (the combat block before the saves and the
 * skills, the notes last as one row), and the four-column paper sheet from 1200px (docs/design/README.md). */
async function scanSheetOrder(browser: Browser, width: number): Promise<{ combat: number; proficiencies: number; features: number; notes: number; notesX: number; combatX: number }> {
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), viewport: { width, height: 844 } });
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), viewport: { width, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Ordem da ficha ${Date.now()}`, false);
    campaignId = table.campaignId;
    await createNoteRPC(p, campaignId, 'Brisa me deve 5 PO');
    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(p.getByRole('heading', { name: 'Características e traços' })).toBeVisible();
    const top = async (selector: string) => (await boxOf(p.locator(selector))).y;
    const abilities = await top('app-ability-medallions');
    const combat = await top('app-combat-column');
    const proficiencies = await top('app-proficiency-column');
    const features = await top('app-features-panel');
    const notes = await top('app-notes-panel');
    return {
      combat: combat - abilities,
      proficiencies: proficiencies - abilities,
      features: features - abilities,
      notes: notes - abilities,
      notesX: (await boxOf(p.locator('app-notes-panel'))).x,
      combatX: (await boxOf(p.locator('app-combat-column'))).x,
    };
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('a ficha no celular vai dos números do jogo às anotações, que ficam numa linha só', { tag: ['@a11y', '@MR-004', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const at = await scanSheetOrder(browser, 390);
  // The abilities come first, then the combat block, then the saves and skills.
  expect(at.combat).toBeGreaterThan(0);
  expect(at.combat).toBeLessThan(at.proficiencies);
  expect(at.proficiencies).toBeLessThan(at.features);
  expect(at.features).toBeLessThan(at.notes);
});

test('a ficha no tablet também põe o combate antes das anotações', { tag: ['@a11y', '@MR-004', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const at = await scanSheetOrder(browser, 768);
  expect(at.combat).toBeLessThanOrEqual(at.proficiencies);
  expect(at.features).toBeLessThan(at.notes);
});

test('a ficha no desktop de 1280 mantém as quatro colunas, com as anotações no alto da quarta', { tag: ['@a11y', '@MR-004', '@MR-030'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const at = await scanSheetOrder(browser, 1280);
  // The notes are the first block of the fourth column: level with the combat block, to its right.
  expect(Math.abs(at.notes - at.combat)).toBeLessThan(2);
  expect(at.notesX).toBeGreaterThan(at.combatX);
});

/** The joint turn (MR-013, E8-01): the master's card and boxes, the player's
 * pill, the other members' card, "Encerrar a minha parte" and its question, the
 * state after the part ended, and the goblins' turn (a group of NPCs alone). */
async function scanJointTurnScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const joint = await jointTable(m, p, `Acessibilidade turno conjunto ${Date.now()}`);
    campaignId = joint.table.campaignId;
    await beginJointCombat(m, joint);

    await m.goto(`/campaigns/${campaignId}/session`);
    await expect(m.getByRole('heading', { name: /^Turno conjunto: / })).toBeVisible();
    await expectScreenPasses(m, `Turno conjunto, visto pelo mestre ${where}`);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await expectScreenPasses(p, `Turno conjunto, a vez do jogador ${where}`);
    await p.getByRole('button', { name: 'Encerrar a minha parte' }).click();
    await expect(p.getByRole('alertdialog', { name: 'Encerrar a sua parte?' })).toBeVisible();
    await expectScreenPasses(p, `Encerrar a minha parte, com pergunta ${where}`);
    await p.getByRole('alertdialog').getByRole('button', { name: 'Encerrar a minha parte' }).click();
    await expect(p.getByRole('heading', { name: 'Você encerrou a sua parte' })).toBeVisible();
    await expectScreenPasses(p, `Turno conjunto, depois de encerrar ${where}`);
    await expectScreenPasses(m, `Turno conjunto, uma parte encerrada ${where}`);

    await endPartRPC(m, campaignId, 'Brisa');
    await expect(p.getByRole('heading', { name: 'Vez dos Goblins' })).toBeVisible();
    await expectScreenPasses(p, `Vez de um grupo de NPCs, jogador ${where}`);
    await expectScreenPasses(m, `Vez de um grupo de NPCs, mestre ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o turno conjunto passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-013'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanJointTurnScreens(browser, 'light', 1280);
});

test('o turno conjunto passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-013'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanJointTurnScreens(browser, 'dark', 390);
});

test('o turno conjunto passa no axe e nas conferências de layout no celular de 320', { tag: ['@a11y', '@MR-013'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanJointTurnScreens(browser, 'light', 320);
});


// Etapa 8.6 (MR-031, MR-032): the NPC's portrait field, the master's and the
// players' stage with its larger view, and the combat highlights.
async function scanStageScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade palco ${Date.now()}`);
    campaignId = table.campaignId;
    const miraImage = await uploadPortrait(m, campaignId, 'Retrato da Mira');
    const capitaoImage = await uploadPortrait(m, campaignId, 'Retrato do Capitão', '#6e8a52');
    const miraId = await createMiraRPC(m, campaignId, miraImage);
    const capitaoId = await createCapitaoRPC(m, campaignId, capitaoImage);
    const aldoId = await createMiraRPC(m, campaignId, '', 'Aldo');
    const ids = [miraId, capitaoId, aldoId];

    // The portrait field: with an image, the gallery picker, the question, and without.
    await open(m, `/campaigns/${campaignId}/characters/${miraId}/edit`);
    await expectScreenPasses(m, `Retrato do NPC ${where}`);
    await m.getByRole('button', { name: 'Trocar retrato' }).click();
    await expect(m.getByRole('dialog', { name: 'Escolher o retrato de Mira' })).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Escolher o retrato ${where}`);
    await m.getByRole('dialog').getByRole('radio', { name: /Retrato do Capitão/ }).click();
    await expectScreenPasses(m, `Escolher o retrato, uma imagem escolhida ${where}`);
    await m.getByRole('dialog').getByRole('button', { name: 'Cancelar' }).click();
    await m.getByRole('button', { name: 'Remover retrato' }).click();
    await expect(m.getByText('A imagem continua na galeria.')).toBeVisible();
    await expectScreenPasses(m, `Remover o retrato, a pergunta ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await open(m, `/campaigns/${campaignId}/characters/${capitaoId}/edit`);
    await expectScreenPasses(m, `Retrato do inimigo ${where}`);
    await open(m, `/campaigns/${campaignId}/characters/${capitaoId}`);
    await expectScreenPasses(m, `Ficha do inimigo com o retrato ${where}`);

    // The stage: the master's cards and list, then the players' stage.
    await openSceneRPC(m, campaignId, table.cartId);
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await expect(m.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeVisible();
    await expectScreenPasses(m, `Em cena, vazio ${where}`);
    await m.getByRole('button', { name: 'Pôr em cena', exact: true }).click();
    await expect(m.getByRole('button', { name: 'Pôr em cena: Mira' })).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Pôr em cena ${where}`);
    for (const name of ['Mira', 'Capitão Goblin']) {
      await m.getByRole('button', { name: `Pôr em cena: ${name}` }).click();
      await expect(m.getByRole('button', { name: `Tirar de cena: ${name}` }).or(m.getByText(`${name} entrou na cena.`).first()).first()).toBeVisible();
    }
    await m.getByRole('button', { name: width >= 768 ? 'Fechar' : 'Fechar', exact: true }).last().click();
    await m.getByRole('button', { name: 'Dar a fala a Capitão Goblin' }).click();
    await expect(m.getByRole('button', { name: 'Fala agora: Capitão Goblin está com a fala. Tirar a fala' })).toBeVisible();
    await expectScreenPasses(m, `Em cena, dois NPCs, um falando ${where}`);

    const stage = p.getByRole('group', { name: 'Em cena: Mira e Capitão Goblin' });
    await expect(stage).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Palco do jogador, dois NPCs ${where}`);
    await stage.getByRole('button', { name: 'Mira (ver maior)' }).click();
    await expect(p.getByRole('heading', { name: 'Mira' })).toBeFocused();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Mira maior ${where}`);
    await p.keyboard.press('Escape');
    await expect(stage.getByRole('button', { name: 'Mira (ver maior)' })).toBeFocused();

    // Four on the stage: the grid on a phone, the row on a desktop.
    await putOnStageRPC(m, campaignId, aldoId);
    const extra = await createMiraRPC(m, campaignId, '', 'Barão Ivo');
    await putOnStageRPC(m, campaignId, extra);
    await expect(p.getByRole('group', { name: /^Em cena: .*, .* e / })).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(p, `Palco do jogador, quatro NPCs ${where}`);
    await expect(m.getByText('A cena comporta 4 NPCs. Tire um para pôr outro.')).toBeVisible();
    await expectScreenPasses(m, `Em cena, quatro de quatro ${where}`);
    expect(ids).toHaveLength(3);
    await m.getByRole('button', { name: 'Fechar cena' }).click();
    await endOpenSessionRPC(m, campaignId);
    campaignId = '';
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }

  // The combat highlights, in a table of their own.
  const master2 = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player2 = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m2 = await master2.newPage();
  const p2 = await player2.newPage();
  let id2 = '';
  try {
    await m2.goto('/');
    await p2.goto('/');
    const table = await tableForCombat(m2, p2, `Acessibilidade destaques ${Date.now()}`, true, true);
    id2 = table.campaignId;
    const enc = await playedCombatRPC(m2, p2, table);
    await openSessionPage(m2, id2);
    await openSessionPage(p2, id2);
    await combatRPC(m2, 'EndEncounter', { campaignId: id2, encounterId: enc.id });
    await expect(m2.getByRole('region', { name: 'Destaques do combate' })).toBeVisible();
    await expect(p2.getByRole('region', { name: 'Destaques do combate' })).toBeVisible();
    await m2.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await p2.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m2, `Destaques do combate, o mestre ${where}`);
    await expectScreenPasses(p2, `Destaques do combate, o cartão do jogador ${where}`);
  } finally {
    if (id2) {
      await endOpenSessionRPC(m2, id2);
    }
    await master2.close();
    await player2.close();
  }
}

test('o retrato, o palco e os destaques passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-031', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanStageScreens(browser, 'light', 1280);
});

test('o retrato, o palco e os destaques passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-031', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanStageScreens(browser, 'dark', 390);
});

test('o retrato, o palco e os destaques passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-031', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanStageScreens(browser, 'dark', 1024);
});

test('o retrato, o palco e os destaques passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-031', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanStageScreens(browser, 'light', 320);
});

// MR-012, RN-10 (B2): the master's moving of tokens outside combat, in the "Tokens" tab under the map: the switch, "Pôr o grupo no mapa" with its
// dialog (a sheet on a phone), "Mover Pensantus" with the pad, the amber notice of a move that stopped before a marked square, the group.
async function scanTokenMoveScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 1000 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const pensantus = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const toren = await browser.newContext({ storageState: authStatePath('E-mail Não Verificado'), colorScheme, viewport });
  const [m, a, b] = await Promise.all([master.newPage(), pensantus.newPage(), toren.newPage()]);
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await Promise.all([m.goto('/'), a.goto('/'), b.goto('/')]);
    const table = await tableForFog(m, a, b, `Acessibilidade mover ${Date.now()}`, { noFog: true, pensantusAt: { col: 2, row: 7 }, torenAt: { col: 3, row: 9 } });
    campaignId = table.campaignId;
    const marked = await callRPC(m, 'meurpg.maps.v1.MapService/PaintMapCells', { campaignId, mapId: table.mapId, layer: 'MAP_LAYER_NO_PASS', value: 1, squares: [{ col: 9, row: 7 }] });
    expect(marked.ok(), await marked.text()).toBeTruthy();
    await m.goto(sessionRoute(campaignId));
    await showMapTool(m, 'Tokens');
    await expect(m.getByRole('switch', { name: 'Os jogadores movem os próprios personagens' })).toBeVisible();
    await expectScreenPasses(m, `Tokens, nada escolhido ${where}`);

    await m.getByRole('checkbox', { name: 'Escolher Pensantus para mover' }).check();
    await expect(m.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await expectScreenPasses(m, `Mover Pensantus, sem destino ${where}`);
    await m.getByRole('button', { name: 'Ir para a direita' }).click();
    for (let i = 0; i < 9; i++) {
      await m.getByRole('button', { name: 'Mais um quadrado' }).click();
    }
    await expect(m.getByText('Pensantus vai 10 quadrados para a direita')).toBeVisible();
    await expectScreenPasses(m, `Mover Pensantus, com destino ${where}`);
    await m.getByRole('button', { name: 'Mover Pensantus', exact: true }).click();
    await expect(m.locator('.mr-notice').getByText('O movimento parou antes.', { exact: true })).toBeVisible();
    await expectScreenPasses(m, `Mover Pensantus, parou antes ${where}`);

    await m.getByRole('checkbox', { name: 'Escolher Toren para mover' }).check();
    await expect(m.getByRole('heading', { name: '2 selecionados' })).toBeVisible();
    await expectScreenPasses(m, `Dois selecionados ${where}`);

    await m.getByRole('button', { name: 'Pôr o grupo no mapa' }).click();
    const dialog = m.getByRole('dialog', { name: 'Pôr o grupo no mapa' });
    await expect(dialog).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((x) => x.playState !== 'running'));
    await expectScreenPasses(m, `Pôr o grupo no mapa ${where}`);
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }
}

test('o mover de tokens passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-012', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanTokenMoveScreens(browser, 'light', 1280);
});

test('o mover de tokens passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-012', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanTokenMoveScreens(browser, 'dark', 390);
});

test('o mover de tokens passa no axe e nas conferências de layout no tema escuro, no tablet de 768', { tag: ['@a11y', '@MR-012', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanTokenMoveScreens(browser, 'dark', 768);
});

test('o mover de tokens passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-012', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanTokenMoveScreens(browser, 'light', 320);
});

// MR-012, RN-10 (B2, C9-01 to C9-03): the player's side of moving tokens outside combat, under the map of the scene's card: the line
// "O mestre move os tokens agora.", the notice when the master frees it, "Mover Brisa" and its panel (a sheet on a phone), the amber notice of a
// move that stopped before a marked square, and the notice when the master locks it again.
async function scanPlayerMoveScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 1000 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await Promise.all([m.goto('/'), p.goto('/')]);
    const table = await tableForScenes(m, p, `Acessibilidade jogador move ${Date.now()}`);
    campaignId = table.campaignId;
    await setGridRPC(m, campaignId, table.mapId, 24);
    const marked = await callRPC(m, 'meurpg.maps.v1.MapService/PaintMapCells', { campaignId, mapId: table.mapId, layer: 'MAP_LAYER_NO_PASS', value: 1, squares: [{ col: 8, row: 11 }] });
    expect(marked.ok(), await marked.text()).toBeTruthy();
    await openSceneRPC(m, campaignId, table.cartId);
    const switchTo = async (enabled: boolean) => {
      const res = await callRPC(m, 'meurpg.maps.v1.MapService/SetMapPlayersMove', { campaignId, mapId: table.mapId, enabled });
      expect(res.ok(), await res.text()).toBeTruthy();
    };
    await p.goto(`/campaigns/${campaignId}/session`);
    if (width < 768) {
      await p.getByRole('tab', { name: /^Mapa/ }).click();
    }
    await expect(p.getByText('O mestre move os tokens agora.')).toBeVisible();
    await expectScreenPasses(p, `Jogador, o mestre move os tokens ${where}`);

    await switchTo(true);
    await expect(p.getByText('O mestre liberou: agora você move Pensantus.').first()).toBeVisible();
    await expectScreenPasses(p, `Jogador, o mestre liberou ${where}`);
    await p.getByRole('button', { name: 'Mover Pensantus', exact: true }).click();
    await expect(p.getByRole('button', { name: 'Ir para a direita' })).toBeVisible();
    await p.waitForFunction(() => document.getAnimations().every((x) => x.playState !== 'running'));
    await expectScreenPasses(p, `Jogador, Mover Pensantus, sem plano ${where}`);
    await p.getByRole('button', { name: 'Ir para a direita' }).click();
    for (let i = 0; i < 9; i++) {
      await p.getByRole('button', { name: 'Mais um quadrado' }).click();
    }
    await expectScreenPasses(p, `Jogador, Mover Pensantus, com plano ${where}`);
    await p.getByRole('button', { name: 'Mover Pensantus', exact: true }).click();
    await expect(p.locator('.mr-notice').getByText('O movimento parou antes.', { exact: true })).toBeVisible();
    await expectScreenPasses(p, `Jogador, o movimento parou antes ${where}`);

    await switchTo(false);
    await expect(p.getByText('O mestre voltou a mover os tokens.').first()).toBeVisible();
    await expectScreenPasses(p, `Jogador, o mestre voltou a mover os tokens ${where}`);
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await master.close();
    await player.close();
  }
}

test('o lado do jogador do mover de tokens passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-012', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanPlayerMoveScreens(browser, 'light', 1280);
});

test('o lado do jogador do mover de tokens passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-012', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanPlayerMoveScreens(browser, 'dark', 390);
});

// MR-015, MR-028, MR-031, RN-10: the master's side of who sees what in a scene: the eye of a stage NPC and of an action, "Quem está
// na cena" with the players as chips and "Os jogadores podem entrar", the menu "Mostrar" of a thumbnail, who sees the image on
// display, and the chooser of players (a sheet on a phone, a dialog from a tablet up).
async function scanSceneVisibilityScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  let toren2: Awaited<ReturnType<typeof secondPlayer>> | null = null;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade quem vê ${Date.now()}`);
    campaignId = table.campaignId;
    toren2 = await secondPlayer(browser, m, campaignId, toren, torenSheet);
    const miraId = await createMiraRPC(m, campaignId);
    const capitaoId = await createCapitaoRPC(m, campaignId);
    const hall = await uploadImageRPC(m, campaignId, 'Vista do salão', await canvasPng(m, 640, 400, 'Vista do salão', '#7a5a40'));
    const cellar = await uploadImageRPC(m, campaignId, 'O porão', await canvasPng(m, 640, 400, 'O porão', '#5a4b42'));
    await setSceneImagesRPC(m, table, table.cartId, [hall, cellar]);
    await openSceneRPC(m, campaignId, table.cartId);
    await putOnStageRPC(m, campaignId, miraId);
    await putOnStageRPC(m, campaignId, capitaoId);
    await openSessionPage(m, campaignId);
    await expect(m.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeVisible();

    // The scene's chooser, then the scene open to Pensantus only, with the switch on.
    await m.getByRole('button', { name: 'Quem está na cena' }).click();
    const chooser = m.getByRole('dialog', { name: 'Quem está na cena?' });
    await expect(chooser).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Quem está na cena, o seletor ${where}`);
    await chooser.getByRole('checkbox', { name: /Toren/ }).uncheck();
    await expectScreenPasses(m, `Quem está na cena, só a Pensantus ${where}`);
    await chooser.getByRole('button', { name: 'Pronto' }).click();
    await expect(m.getByText('Toren não está nesta cena.')).toBeVisible();
    await m.getByRole('switch', { name: 'Os jogadores podem entrar' }).click();
    await expect(m.getByRole('switch', { name: 'Os jogadores podem entrar' })).toHaveAttribute('aria-checked', 'true');
    await expectScreenPasses(m, `Cena aberta só para a Pensantus, entrar ligado ${where}`);

    // The eyes: a hidden NPC and a hidden action.
    await m.getByRole('button', { name: 'Esconder Capitão Goblin dos jogadores' }).click();
    await expect(m.locator('[data-stage-card]').filter({ hasText: 'Capitão Goblin' })).toContainText('Escondida dos jogadores');
    await m.getByRole('button', { name: 'Esconder Acalmar os cavalos dos jogadores' }).click();
    await expect(m.getByRole('region', { name: 'Ações e rolagens' }).locator('.sa__item').filter({ hasText: 'Acalmar os cavalos' })).toContainText('Escondida');
    await expectScreenPasses(m, `Um NPC e uma ação escondidos ${where}`);

    // "Mostrar" on a thumbnail, then who sees the image on display.
    await m.getByRole('button', { name: 'Mostrar Vista do salão para…' }).click();
    await expect(m.getByRole('menu').getByText('Mostrar “Vista do salão” para')).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Mostrar para quem, o menu ${where}`);
    await m.getByRole('menuitem', { name: /Só quem está na cena/ }).click();
    await expect(m.getByText('Pensantus vê')).toBeVisible();
    await expectScreenPasses(m, `Imagem só para quem está na cena ${where}`);
    await m.getByRole('button', { name: 'Mudar quem vê' }).click();
    const viewers = m.getByRole('dialog', { name: 'Quem vê “Vista do salão”?' });
    await expect(viewers).toBeVisible();
    await m.waitForFunction(() => document.getAnimations().every((a) => a.playState !== 'running'));
    await expectScreenPasses(m, `Quem vê a imagem, o seletor ${where}`);
    await viewers.getByRole('button', { name: 'Cancelar' }).click();
    await expect(viewers).toBeHidden();
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await toren2?.close();
    await master.close();
    await player.close();
  }
}

test('o quem vê da cena passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-015', '@MR-028', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanSceneVisibilityScreens(browser, 'light', 1280);
});

test('o quem vê da cena passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-015', '@MR-028', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanSceneVisibilityScreens(browser, 'dark', 390);
});

test('o quem vê da cena passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-015', '@MR-028', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanSceneVisibilityScreens(browser, 'dark', 1024);
});

test('o quem vê da cena passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-015', '@MR-028', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(480_000);
  await scanSceneVisibilityScreens(browser, 'light', 320);
});

/** The spells in the session (Etapa 8, slice 8.4; E8-02, E8-03): the list with its "?" and slot
 * rows, the details sheet, the cast sheet with its "?", the details over it, the result a player
 * reads, and the master's card under Sono in the log. */
async function scanCombatDetailsScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const phone = width < 768;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade magias ${Date.now()}`, true, true);
    campaignId = table.campaignId;
    // The three around Goblin 1, so Sono placed on it catches them.
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, { 'Capitão Goblin': [9, 10], 'Goblin 1': [9, 9], 'Goblin 2': [10, 9] });
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    await openActionTab(p, 'Magias');
    await expect(p.getByRole('button', { name: 'Conjurar Sono' })).toBeVisible();
    await expectScreenPasses(p, `Magias e os espaços ${where}`);

    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Sono' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Sono' });
    // The area picker (PM-02a, PM-02b): the map before a point, then a point with nobody seen in it and its confirmation.
    const map = sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Sono' });
    await expect(map).toBeFocused();
    await expectScreenPasses(p, `Conjurar Sono, o ponto no mapa ${where}`);
    await map.press('Shift+ArrowUp');
    // The hint under the title follows the keyboard: it names the distance of the point the arrows reached, before Enter places it.
    const hint = sheet.locator('.hint');
    await expect(hint).toContainText(/Ponto a\s+\d+,\d\s+m de você/);
    const first = await hint.innerText();
    await map.press('Shift+ArrowUp');
    await expect(hint).not.toHaveText(first);
    await map.press('Enter');
    await expect(sheet.getByText(/Ponto a\s+.* de você/).first()).toBeVisible();
    await expectScreenPasses(p, `Conjurar Sono, o ponto colocado ${where}`);
    await map.press('c');
    await expect(sheet.getByRole('listbox', { name: 'Centrar em…' })).toBeFocused();
    await expectScreenPasses(p, `Conjurar Sono, "Centrar em…" ${where}`);
    await sheet.getByRole('listbox', { name: 'Centrar em…' }).press('Escape');
    await map.press('Enter');
    await expect(sheet.getByText('Ninguém que você vê está na área.')).toBeVisible();
    await expect(sheet.getByRole('button', { name: 'Mudar o local' })).toBeFocused();
    await expectScreenPasses(p, `Conjurar Sono, ninguém na área ${where}`);
    await sheet.getByRole('button', { name: 'Mudar o local' }).click();
    await sheet.getByRole('button', { name: 'Centrar em…' }).click();
    await sheet.getByRole('listbox', { name: 'Centrar em…' }).getByRole('option', { name: /Goblin 1/ }).click();
    await sheet.getByRole('button', { name: 'Confirmar local' }).click();
    await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
    await expectScreenPasses(p, `Conjurar Sono, quem está na área ${where}`);
    await sheet.getByRole('button', { name: 'Detalhes de Sono' }).click();
    await expect(p.getByRole('dialog', { name: phone ? 'Descrição de Sono' : 'Sono', exact: true })).toBeVisible();
    await expectScreenPasses(p, `Detalhes de Sono por cima de Conjurar ${where}`);
    await p.getByRole('dialog', { name: phone ? 'Descrição de Sono' : 'Sono', exact: true }).getByRole('button', { name: 'Fechar' }).last().click();
    // A dialog on a desktop is named by its title, which changes with the step: ask the page.
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 5d8/).fill('20');
    await p.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(p.getByRole('heading', { name: 'Sono conjurado' })).toBeVisible();
    await expectScreenPasses(p, `Sono conjurado, o resultado do jogador ${where}`);
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();

    // The master's log is a row of "Mais do combate", folded.
    await openCombatPanel(m, 'Registro');
    await expect(m.locator('app-pool-card')).toBeVisible({ timeout: 20_000 });
    await expectScreenPasses(m, `O cartão do Sono no registro do mestre ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as magias na sessão passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanCombatDetailsScreens(browser, 'light', 1280);
});

test('as magias na sessão passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanCombatDetailsScreens(browser, 'dark', 390);
});

/**
 * The area spell the table asks about (PM-02c 9 and 9c, PM-02b 5): the master's question with the area of the last spell
 * over his map and its legend, the question when no hidden creature was in the area ("Sem escondidas"), the line "Combate
 * atualizado agora." after a reload, and the picker's zoom on a map of 40 columns. The desktop scan hides a goblin in the
 * area; the phone scan has none in it. The table asks (`Perguntar a cada vez`), so the cast holds the turn either way.
 */
async function scanHiddenAreaScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number, hiddenInArea: boolean): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade escondidas ${Date.now()}`, true, true);
    campaignId = table.campaignId;
    await setTableRulesRPC(m, campaignId, { hiddenAreaHits: 'HIDDEN_AREA_HIT_RULE_ASK' });
    if (!hiddenInArea) {
      await setGridRPC(m, campaignId, table.mapId, 40);
    }
    // Sono's area is 4 squares around the point: Goblin 2 is in it, or far from it.
    const at: Record<string, [number, number]> = { 'Capitão Goblin': [9, 10], 'Goblin 1': [9, 9], 'Goblin 2': hiddenInArea ? [10, 9] : [20, 3] };
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, at, ['Goblin 2']);
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Sono' }).click();
    const sheet = p.getByRole('dialog', { name: 'Conjurar Sono' });
    const map = sheet.getByRole('application', { name: 'Mapa: escolha o ponto da Sono' });
    await expect(map).toBeFocused();
    if (!hiddenInArea) {
      await expect(sheet.getByRole('group', { name: 'Zoom do mapa' })).toBeVisible();
      await sheet.getByRole('button', { name: 'Aproximar o mapa' }).click();
      await expectScreenPasses(p, `Conjurar Sono, o mapa de 40 colunas aproximado ${where}`);
      await sheet.getByRole('button', { name: 'Afastar o mapa' }).click();
    }
    await map.press('c');
    await sheet.getByRole('listbox', { name: 'Centrar em…' }).getByRole('option', { name: /Goblin 1/ }).click();
    await map.press('Enter');
    await expect(sheet.getByRole('heading', { name: 'Quem está na área' })).toBeFocused();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 5d8/).fill('20');
    await p.getByRole('button', { name: 'Confirmar 20' }).click();
    await p.getByRole('button', { name: 'Voltar à sua vez' }).click();
    await expect(p.getByText('Esperando o mestre').first()).toBeVisible();
    await expectScreenPasses(p, `Esperando o mestre, a vez parada ${where}`);

    const card = m.getByRole('group', { name: hiddenInArea ? /atingiu 1 criatura escondida/ : /nenhuma criatura escondida na área/ });
    await expect(card).toBeVisible({ timeout: 20_000 });
    // On a phone the map is its own tab: the area is drawn there, the question waits in "Turno".
    await openMap(m);
    await expect(m.locator('app-area-overlay')).toBeVisible();
    await openTurn(m);
    await expectScreenPasses(m, `A pergunta ao mestre, com a área da última magia ${where}`);
    await m.reload();
    await expect(m.getByRole('status').filter({ hasText: 'Combate atualizado agora.' })).toBeVisible();
    await expect(card).toBeVisible();
    await expectScreenPasses(m, `A pergunta depois de recarregar ${where}`);
    await m.getByRole('button', { name: hiddenInArea ? 'Revelar' : 'Sem escondidas', exact: true }).click();
    await expect(card).toHaveCount(0);
    await expectScreenPasses(p, `A vez volta ao jogador ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('a pergunta das escondidas e a área da última magia passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanHiddenAreaScreens(browser, 'light', 1280, true);
});

test('a pergunta sem escondidas e o zoom do mapa grande passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014', '@RN-10'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanHiddenAreaScreens(browser, 'dark', 390, false);
});

/**
 * The options of the scene (Etapa 8, MR-015, RN-20; E8-13): the point panel with the DC switch off and on and the
 * attempts of each action (on a computer), the master's open scene with "Os jogadores veem a CD", the limits and
 * "Tentativa 1 de 3", "Dar mais uma tentativa" and its question in place and the status after it, and the
 * player's scene with the CD pill, "Passou"/"Não passou", the attempts that are left ("Restam 2 de 3 tentativas",
 * "Sem mais tentativas") and the page with the DC hidden. The rolls are made through the API, so the screens are
 * the same on every run.
 */
async function scanSceneOptionsScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade opções ${Date.now()}`);
    campaignId = table.campaignId;
    const ids = await sceneActionIdsRPC(m, table, table.cartId);
    await setAttemptsRPC(m, table, table.cartId, ids['Percepção'], 3);
    await setAttemptsRPC(m, table, table.cartId, ids['Acalmar os cavalos'], 0);

    // The editor: the switch off and on, and every action with its attempts. On a phone the map is read-only (its
    // points open nothing and the page says to edit them on a computer), so the editor is scanned at the desktop
    // widths, and the phone checks that it gets the read-only map instead.
    await open(m, `/campaigns/${campaignId}/maps/${table.mapId}`);
    const cart = m.getByRole('button', { name: /^A carroça tombada, Cena de RP/ });
    if (width < 768) {
      await expect(m.getByText('Para mudar pontos e tokens de lugar, abra o mapa no computador.')).toBeVisible();
      await expect(cart).toHaveCount(0);
    } else {
      await cart.click();
      await expect(m.getByRole('switch', { name: 'Mostrar a CD aos jogadores' })).toBeVisible();
      await expectScreenPasses(m, `Ações da cena com o interruptor da CD desligado ${where}`);
      await m.getByRole('switch', { name: 'Mostrar a CD aos jogadores' }).click();
      await expect(m.getByText('Como o jogador vê, antes e depois de rolar')).toBeVisible();
      await expectScreenPasses(m, `Ações da cena com o interruptor da CD ligado ${where}`);
      await expect(m.getByRole('status').filter({ hasText: 'Os jogadores agora veem a CD.' })).toHaveCount(1);
      await expect(m.getByText('Sem limite: o jogador rola quantas vezes quiser')).toBeVisible();
      await expectScreenPasses(m, `Ações da cena com "Sem limite" ${where}`);
    }

    // The session: the scene is open with the DC shown. The player rolls (Percepção 4 + 1 = 5, Investigação 11 + 6 = 17, Constituição 3 + 3 = 6).
    await setShowDcRPC(m, table, table.cartId, true);
    await openSceneRPC(m, campaignId, table.cartId);
    await openSessionPage(p, campaignId);
    const scene = p.getByRole('region', { name: 'Cena: A carroça tombada' });
    await expect(scene).toBeVisible();
    await expect(scene.getByText('CD 12')).toBeVisible();
    await expect(scene.getByText('Restam 3 de 3 tentativas')).toBeVisible();
    await expectScreenPasses(p, `Cena do jogador com a CD à mostra ${where}`);
    const open1 = await getOpenSceneRPC(p, campaignId);
    const actionIds = open1.scene!.actions.map((a) => a.id);
    await rollSceneRPC(p, campaignId, actionIds[3], 4);
    await rollSceneRPC(p, campaignId, actionIds[0], 11);
    await rollSceneRPC(p, campaignId, actionIds[4], 3);
    await expect(scene.getByText('Restam 2 de 3 tentativas')).toBeVisible();
    await expect(scene.getByText('Passou · CD 12')).toBeVisible();
    await expect(scene.getByText('Não passou · CD 10')).toBeVisible();
    await expect(scene.getByText('Sem mais tentativas').first()).toBeVisible();
    await expectScreenPasses(p, `Cena do jogador com Passou, Não passou e as tentativas ${where}`);

    await openSessionPage(m, campaignId);
    await expect(m.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeVisible();
    await expect(m.getByText('Os jogadores veem a CD')).toBeVisible();
    await openRolls(m, 3);
    await expect(m.getByText('Tentativa 1 de 1').first()).toBeVisible();
    await expectScreenPasses(m, `Cena aberta do mestre com os limites e as tentativas ${where}`);
    const grant = m.getByRole('button', { name: 'Dar mais uma tentativa a Pensantus em Resistir ao cheiro de fumaça' });
    await grant.click();
    const ask = m.getByRole('alertdialog', { name: 'Dar mais uma tentativa a Pensantus?' });
    await expect(ask.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await expectScreenPasses(m, `Dar mais uma tentativa, a pergunta no lugar ${where}`);
    await ask.getByRole('button', { name: 'Dar mais uma tentativa' }).click();
    await expect(m.getByRole('status').filter({ hasText: 'Mais uma tentativa dada a Pensantus' })).toBeVisible();
    await expectScreenPasses(m, `Dar mais uma tentativa, depois de dar ${where}`);
    await expect(scene.getByText('Restam 1 de 1 tentativas').or(scene.getByText('1 tentativa')).first()).toBeVisible();
    await expectScreenPasses(p, `Cena do jogador com uma tentativa a mais ${where}`);

    // The DC hidden (the default): no CD and no "Passou" for the player.
    await setShowDcRPC(m, table, table.cartId, false);
    await expect(scene.getByText('CD 12')).toHaveCount(0);
    await expectScreenPasses(p, `Cena do jogador com a CD escondida ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as opções da cena passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanSceneOptionsScreens(browser, 'light', 1280);
});

test('as opções da cena passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanSceneOptionsScreens(browser, 'dark', 390);
});

test('as opções da cena passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanSceneOptionsScreens(browser, 'dark', 1024);
});

test('as opções da cena passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-015'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanSceneOptionsScreens(browser, 'light', 320);
});

/**
 * The session summary (Etapa 8, MR-032, RN-20; E8-11 states 4 and 5): the master's "Sessão encerrada" with the
 * highlights and the table, the player's card "A sessão acabou" and the plain notice after "Fechar". The rolls are
 * made through the API, with the DC shown, so the numbers are the same on every run.
 */
async function scanSessionSummaryScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade resumo ${Date.now()}`);
    campaignId = table.campaignId;
    await setShowDcRPC(m, table, table.cartId, true);
    await openSceneRPC(m, campaignId, table.cartId);
    await openSessionPage(p, campaignId);
    const open1 = await getOpenSceneRPC(p, campaignId);
    const actionIds = open1.scene!.actions.map((a) => a.id);
    await rollSceneRPC(p, campaignId, actionIds[0], 11);
    await rollSceneRPC(p, campaignId, actionIds[4], 3);
    await openSessionPage(m, campaignId);
    await openRolls(m, 2);
    await expect(m.getByText('Passou · CD 12')).toBeVisible();

    await endSessionFromBar(m);
    await expect(m.getByRole('heading', { name: 'Sessão encerrada' })).toBeVisible();
    await expect(m.getByRole('table', { name: 'Testes passados fora do combate' })).toBeVisible();
    await expectScreenPasses(m, `Resumo da sessão do mestre ${where}`);

    const card = p.getByRole('region', { name: 'Resumo da sessão' });
    await expect(card.getByRole('heading', { name: 'A sessão acabou' })).toBeVisible();
    await expect(card.getByRole('heading', { name: 'Seu resultado, Pensantus' })).toBeVisible();
    await expectScreenPasses(p, `Cartão "A sessão acabou" do jogador ${where}`);
    await card.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(p.getByText('A sessão acabou.')).toBeVisible();
    await expectScreenPasses(p, `A sessão acabou, o aviso simples ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o resumo da sessão passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSessionSummaryScreens(browser, 'light', 1280);
});

test('o resumo da sessão passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSessionSummaryScreens(browser, 'dark', 390);
});

test('o resumo da sessão passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSessionSummaryScreens(browser, 'light', 320);
});

// The guided level-up (MR-040, E8-15): the sheet's block, every step and state of the page, the question
// before discarding, the blocked route, and the master's "O que mudou". Pensantus goes from Mago 3 to 4.
async function scanLevelUpScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `${colorScheme === 'light' ? 'no tema claro' : 'no tema escuro'}, a ${width} px`;
  const height = width === 320 ? 568 : width === 1024 ? 768 : width < 768 ? 844 : 800;
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height } });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const m = await master.newPage();
  const p = await player.newPage();
  const row = (name: string) => p.locator('.row__main, .row').filter({ hasText: name }).first();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForLevelUp(m, p, `Acessibilidade subida ${Date.now()}`);
    campaignId = table.campaignId;
    const sheet = `/campaigns/${campaignId}/characters/${table.characterId}`;

    // Not `open`: with a session open the page keeps its stream, so the network is never idle.
    await p.goto(sheet);
    await expect(p.getByRole('link', { name: 'Subir para o nível 4' })).toBeVisible();
    await expectScreenPasses(p, `A ficha que pode subir de nível ${where}`);

    await p.getByRole('link', { name: 'Subir para o nível 4' }).click();
    await expect(p.getByText('Passo 1 de 5 · Classe')).toBeVisible();
    await expectScreenPasses(p, `Subir em qual classe?, a classe que ele tem ${where}`);
    await classCard(p, /^Uma classe nova/).click();
    await expect(p.getByRole('heading', { name: 'Qual classe nova?' })).toBeVisible();
    await expectScreenPasses(p, `Subir em qual classe?, "Uma classe nova" aberta com os pré-requisitos ${where}`);
    // A new class end to end, before the level goes to the Mago he has: the Guerreiro (Destreza 16 meets "Força ou Destreza").
    await classCard(p, /^Guerreiro/).click();
    await expect(p.getByText('Pensantus · Mago 3 → Mago 3 · Guerreiro 1')).toBeAttached();
    await expectScreenPasses(p, `Subir em qual classe?, a classe nova escolhida ${where}`);
    await p.getByRole('button', { name: 'Próximo' }).click();
    await expect(p.getByRole('heading', { name: 'Pontos de vida de Guerreiro 1' })).toBeVisible();
    await expectScreenPasses(p, `Vida da classe nova ${where}`);
    await p.getByRole('button', { name: 'Próximo' }).click();
    await p.locator('#pick-feature-0').getByRole('radio').first().click();
    await expectScreenPasses(p, `Escolhas da classe nova, o Estilo de Luta ${where}`);
    await p.getByRole('button', { name: 'Próximo' }).click();
    await expect(p.getByText('Passo 4 de 4 · Resumo')).toBeVisible();
    await expectScreenPasses(p, `Resumo da classe nova ${where}`);
    await p.getByRole('button', { name: 'Confirmar o nível 4' }).click();
    await expect(p.getByRole('alertdialog', { name: 'Subir em Guerreiro 1?' })).toBeVisible();
    await expectScreenPasses(p, `A pergunta antes de acrescentar a classe ${where}`);
    await p.getByRole('button', { name: 'Voltar', exact: true }).click();
    for (let i = 0; i < 3; i++) {
      await p.getByRole('button', { name: 'Voltar', exact: true }).click();
    }
    await classCard(p, /^Mago/).click();
    await p.getByRole('button', { name: 'Trocar para o Mago' }).click();
    await expect(p.getByText('Pensantus · Mago 3 → Mago 4')).toBeAttached();
    await p.getByRole('button', { name: 'Próximo' }).click();
    await expect(p.getByText('Passo 2 de 5 · Habilidades')).toBeVisible();
    await expectScreenPasses(p, `Habilidades, com a escolha faltando ${where}`);
    await row('Inteligência').click();
    await expect(p.getByText('18 → 20')).toBeVisible();
    await expectScreenPasses(p, `Habilidades, Inteligência 20 ${where}`);
    // "Cancelar" is the first step's button; the class step is the first one now.
    await p.getByRole('button', { name: 'Voltar', exact: true }).click();
    await expect(p.getByText('Passo 1 de 5 · Classe')).toBeVisible();
    await p.getByRole('button', { name: 'Cancelar' }).click();
    await expect(p.getByText('Descartar as escolhas?')).toBeVisible();
    await expectScreenPasses(p, `A pergunta de descartar ${where}`);
    await p.getByRole('button', { name: 'Continuar escolhendo' }).click();
    await p.getByRole('button', { name: 'Próximo' }).click();
    await expect(p.getByText('Passo 2 de 5 · Habilidades')).toBeVisible();
    await p.getByRole('button', { name: 'Próximo' }).click();

    await expect(p.getByText('Passo 3 de 5 · Vida')).toBeVisible();
    await expectScreenPasses(p, `Vida, a média ${where}`);
    await p.locator('.dice-choice__card').filter({ hasText: 'Rolar 1d6' }).click();
    await expectScreenPasses(p, `Vida, rolar o dado ${where}`);
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d6/).fill('4');
    await expectScreenPasses(p, `Vida, o dado físico digitado ${where}`);
    await p.getByRole('button', { name: 'Confirmar 4' }).click();
    await expect(p.getByText('Dado físico: 4 no d6')).toBeVisible();
    await expectScreenPasses(p, `Vida, o resultado do dado ${where}`);
    await p.getByRole('button', { name: 'Próximo' }).click();

    await expect(p.getByText('Passo 4 de 5 · Magias')).toBeVisible();
    await expectScreenPasses(p, `Magias, com a escolha faltando ${where}`);
    await p.getByRole('button', { name: /Ver os outros \d+ truques/ }).click();
    await row('Prestidigitação').click();
    await p.getByLabel('Buscar magia').fill('nebuloso');
    await row('Passo Nebuloso').click();
    await p.getByLabel('Buscar magia').fill('reflexos');
    await row('Reflexos').click();
    await p.getByLabel('Buscar magia').fill('');
    await expectScreenPasses(p, `Magias, o livro completo e as preparadas faltando ${where}`);
    await p.getByRole('button', { name: 'Descrição de Passo Nebuloso' }).first().click();
    await expect(p.getByRole('dialog').first()).toBeVisible();
    await expectScreenPasses(p, `O "?" de uma magia do subir de nível ${where}`);
    await p.keyboard.press('Escape');
    const prepare = p.locator('#pick-prepared');
    await showAllPicks(prepare);
    await prepare.locator('.row__main').filter({ hasText: 'Passo Nebuloso' }).click();
    await prepare.locator('.row__main').filter({ hasText: 'Detectar Magia' }).click();
    await expectScreenPasses(p, `Magias, tudo escolhido ${where}`);
    await p.getByRole('button', { name: 'Próximo' }).click();

    await expect(p.getByText('Passo 5 de 5 · Resumo')).toBeVisible();
    await expectScreenPasses(p, `Resumo ${where}`);
    await p.getByRole('button', { name: 'Confirmar o nível 4' }).click();
    await expect(p.getByText('Pensantus subiu para o nível 4.').first()).toBeVisible();
    await expectScreenPasses(p, `A ficha depois de subir ${where}`);

    await p.goto(`${sheet}/level-up`);
    await expect(p.getByRole('heading', { name: 'Ainda não dá para subir de nível' })).toBeVisible();
    await expectScreenPasses(p, `A rota sem a marca ${where}`);

    await m.goto(`/campaigns/${campaignId}`);
    await expect(m.getByRole('status').filter({ hasText: 'Pensantus subiu para o nível 4.' })).toBeVisible();
    await expectScreenPasses(m, `O mestre, o aviso da subida ${where}`);
    await m.getByRole('button', { name: 'O que mudou: Pensantus' }).click();
    await expect(m.getByRole('region', { name: 'O que Pensantus escolheu no nível 4' })).toBeVisible();
    await expectScreenPasses(m, `O mestre, "O que mudou" aberto ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o subir de nível passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(900_000);
  await scanLevelUpScreens(browser, 'light', 1280);
});

test('o subir de nível passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(900_000);
  await scanLevelUpScreens(browser, 'dark', 390);
});

test('o subir de nível passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(900_000);
  await scanLevelUpScreens(browser, 'dark', 1024);
});

test('o subir de nível passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(900_000);
  await scanLevelUpScreens(browser, 'light', 320);
});

// The choices a locked sheet left open (PM-05): the sheet's banner, the page "Completar escolhas pendentes" with the
// choice missing, half picked and full, the save, the page with nothing left, and the master's tag on the campaign.
async function scanOpenChoicesScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `${colorScheme === 'light' ? 'no tema claro' : 'no tema escuro'}, a ${width} px`;
  const height = width === 320 ? 568 : width === 1024 ? 768 : width < 768 ? 844 : 800;
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height } });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableWithOpenChoices(m, p, `Acessibilidade escolhas ${Date.now()}`);
    campaignId = table.campaignId;
    const sheet = `/campaigns/${campaignId}/characters/${table.characterId}`;

    // Not `open`: with a session open the page keeps its stream, so the network is never idle.
    await m.goto(`/campaigns/${campaignId}`);
    await expect(m.getByText('2 escolhas em aberto')).toBeVisible();
    await expectScreenPasses(m, `O mestre, a marca de escolhas em aberto ${where}`);

    await p.goto(sheet);
    await expect(p.getByText('Esta ficha tem 2 escolhas pendentes.')).toBeVisible();
    await expectScreenPasses(p, `A ficha com escolhas pendentes ${where}`);

    await p.getByRole('link', { name: 'Completar escolhas pendentes' }).click();
    await expect(p.getByRole('heading', { level: 1, name: 'Completar escolhas pendentes' })).toBeVisible();
    const abilities = p.getByRole('region', { name: /\+1 em duas habilidades/ });
    await expect(abilities).toBeVisible();
    await expectScreenPasses(p, `Completar escolhas, com a escolha faltando ${where}`);
    await abilities.getByRole('checkbox', { name: /Destreza/ }).click();
    await expect(abilities.getByText('Destreza 16 → 17')).toBeVisible();
    await expectScreenPasses(p, `Completar escolhas, uma habilidade ${where}`);
    await abilities.getByRole('checkbox', { name: /Constituição/ }).click();
    await expect(abilities.getByText('Constituição 15 → 16')).toBeVisible();
    await expect(abilities.getByText(/Já escolheu 2/).first()).toBeVisible();
    await expectScreenPasses(p, `Completar escolhas, as duas habilidades ${where}`);

    await p.getByRole('button', { name: 'Salvar escolhas' }).click();
    await expect(p).toHaveURL(new RegExp(`${sheet}$`));
    await expect(p.getByText('escolhas pendentes')).toHaveCount(0);
    await expectScreenPasses(p, `A ficha depois de completar as escolhas ${where}`);

    await p.goto(`${sheet}/choices`);
    await expect(p.getByText('Pensantus não tem escolhas pendentes.')).toBeVisible();
    await expectScreenPasses(p, `Completar escolhas, sem nada pendente ${where}`);
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await master.close();
    await player.close();
  }
}

test('as escolhas pendentes passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-01'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanOpenChoicesScreens(browser, 'light', 1280);
});

test('as escolhas pendentes passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-01'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanOpenChoicesScreens(browser, 'dark', 390);
});

test('as escolhas pendentes passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@RN-01'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanOpenChoicesScreens(browser, 'dark', 1024);
});

test('as escolhas pendentes passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@RN-01'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanOpenChoicesScreens(browser, 'light', 320);
});

/**
 * "Voltar à cidade" and "Mais tesouro encontrado" (Etapa 9, MR-041, MR-032; E9-09): the Experiência panel of a
 * campaign by gold with its strip and the two buttons, "Dar XP" with the strip, the conversion dialog (everything
 * checked, a change, nothing checked, a refusal, nothing to convert), the history line and the question to undo,
 * the player's view, the campaign by enemies, and the session summary with the treasure block and the card.
 */
async function scanGoldScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const campaigns: string[] = [];
  const panel = (page: Page) => page.getByRole('region', { name: 'Experiência', exact: true });
  try {
    await m.goto('/');
    await p.goto('/');

    // Nothing found yet: the strip invites, and the dialog explains.
    const gold = await tableForGold(m, p, `Acessibilidade ouro ${Date.now()}`);
    campaigns.push(gold.campaignId);
    await open(m, `/campaigns/${gold.campaignId}`);
    await expect(panel(m)).toContainText('Nenhum tesouro esperando.');
    await expectScreenPasses(m, `Experiência por ouro, nada esperando ${where}`);
    await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
    const empty = m.getByRole('dialog', { name: 'Voltar à cidade' });
    await expect(empty).toContainText('Nenhum tesouro encontrado para converter.');
    await expectScreenPasses(m, `Voltar à cidade, nada para converter ${where}`);
    await empty.getByRole('button', { name: 'Fechar', exact: true }).last().click();

    // Three finds: the strip, the dialog and its calculation in each state.
    const ids = await threeTreasuresRPC(m, gold);
    await open(m, `/campaigns/${gold.campaignId}`);
    await expect(panel(m).getByText('3 tesouros · 420 PO')).toBeVisible();
    await expectScreenPasses(m, `Experiência por ouro, três tesouros esperando ${where}`);
    await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
    const town = m.getByRole('dialog', { name: 'Voltar à cidade' });
    await expect(town).toContainText('420 XP ÷ 1 = 420 XP para cada');
    await expectScreenPasses(m, `Voltar à cidade, tudo marcado ${where}`);
    await town.getByRole('checkbox', { name: 'Converter Ídolo de prata' }).uncheck({ force: true });
    await expect(town).toContainText('370 XP ÷ 1 = 370 XP para cada');
    await expectScreenPasses(m, `Voltar à cidade, sem um tesouro ${where}`);
    await town.getByRole('checkbox', { name: 'Marcar Pensantus' }).uncheck({ force: true });
    await expect(town).toContainText('Marque pelo menos um tesouro e um personagem.');
    await expectScreenPasses(m, `Voltar à cidade, ninguém marcado ${where}`);
    await town.getByRole('button', { name: 'Cancelar' }).click();

    // Converted meanwhile: the refusal stays in the dialog.
    await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
    await expect(m.getByRole('dialog', { name: 'Voltar à cidade' })).toContainText('420 XP ÷ 1 = 420 XP para cada');
    const other = await callRPC(m, 'meurpg.progression.v1.ProgressionService/AwardXP', {
      campaignId: gold.campaignId,
      mode: 'XP_AWARD_MODE_GOLD',
      reason: 'Voltar à cidade',
      characterIds: gold.characterIds,
      treasurePointIds: [ids[0]],
      idempotencyKey: crypto.randomUUID(),
    });
    expect(other.ok(), await other.text()).toBeTruthy();
    await m.getByRole('dialog', { name: 'Voltar à cidade' }).getByRole('button', { name: /^Dar 420 XP/ }).click();
    await expect(m.getByRole('dialog', { name: 'Voltar à cidade' }).getByRole('alert')).toContainText('já virou XP em outro prêmio');
    await expectScreenPasses(m, `Voltar à cidade, o erro no lugar ${where}`);
    await m.getByRole('dialog', { name: 'Voltar à cidade' }).getByRole('button', { name: /^Dar 170 XP/ }).click();
    await expect(panel(m).getByRole('status').filter({ hasText: 'foram convertidos' })).toContainText('Os 2 tesouros foram convertidos.');
    // The master's history is folded under "Histórico de XP (N)": the line of the award is in it.
    await openXpHistory(m);
    await expect(panel(m).getByText('Voltar à cidade · 170 PO em 2 tesouros')).toBeVisible();
    await expectScreenPasses(m, `Experiência, depois de converter ${where}`);
    await panel(m).getByRole('button', { name: /^Desfazer/ }).click();
    await expect(panel(m).getByRole('alertdialog')).toContainText('voltam a “encontrado, não convertido”');
    await expectScreenPasses(m, `Experiência, desfazer a conversão ${where}`);
    await panel(m).getByRole('alertdialog').getByRole('button', { name: 'Desfazer XP' }).click();
    await expect(panel(m).getByRole('status').filter({ hasText: 'XP desfeito' })).toBeVisible();
    await expectScreenPasses(m, `Experiência, conversão desfeita ${where}`);

    // "Dar XP" has the strip too, and its button.
    await panel(m).getByRole('button', { name: 'Dar XP' }).click();
    const give = m.getByRole('dialog', { name: 'Dar XP' });
    await expect(give).toContainText('ou digite o ouro');
    await expectScreenPasses(m, `Dar XP por ouro com os tesouros ${where}`);
    await give.getByRole('button', { name: 'Cancelar' }).click();

    // The player: the history line and the strip's absence.
    await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
    await m.getByRole('dialog', { name: 'Voltar à cidade' }).getByRole('button', { name: /^Dar 170 XP/ }).click();
    await expect(panel(m).getByRole('status').filter({ hasText: 'foram convertidos' })).toContainText('Os 2 tesouros foram convertidos.');
    await open(p, `/campaigns/${gold.campaignId}`);
    await expect(panel(p)).toContainText('Voltar à cidade · 170 PO em 2 tesouros');
    await expectScreenPasses(p, `Experiência por ouro, jogador ${where}`);

    // A campaign by enemies: no button, the line why.
    const enemies = await tableForGold(m, p, `Acessibilidade inimigos ${Date.now()}`, 'XP_MODE_ENEMIES');
    campaigns.push(enemies.campaignId);
    await treasureFoundRPC(m, enemies, { name: 'Baú de moedas', valuePo: 250, finders: [enemies.characterIds[0]] });
    await open(m, `/campaigns/${enemies.campaignId}`);
    await expect(panel(m)).toContainText('Esta campanha dá XP por inimigos, então o tesouro não vira XP.');
    await expectScreenPasses(m, `Experiência por inimigos, com tesouro ${where}`);

    // "Dar XP" of the live session (state 6c): the strip with the way in, then the conversion from it.
    const live = await tableForGold(m, p, `Acessibilidade ouro sessão ${Date.now()}`);
    campaigns.push(live.campaignId);
    await threeTreasuresRPC(m, live);
    await startSessionRPC(m, live.campaignId);
    await openSessionPage(m, live.campaignId, 'grupo');
    await m.getByRole('button', { name: 'Dar XP', exact: true }).click();
    const liveGive = m.getByRole('dialog', { name: 'Dar XP' });
    await expect(liveGive).toContainText('3 tesouros · 420 PO');
    await expectScreenPasses(m, `Dar XP da sessão com a faixa de tesouros ${where}`);
    await liveGive.getByRole('button', { name: 'Voltar à cidade' }).click();
    await expect(m.getByRole('dialog', { name: 'Voltar à cidade' })).toContainText('420 XP ÷ 1 = 420 XP para cada');
    await expectScreenPasses(m, `Voltar à cidade aberto do Dar XP da sessão ${where}`);
    await m.getByRole('dialog', { name: 'Voltar à cidade' }).getByRole('button', { name: 'Cancelar' }).click();
    await endOpenSessionRPC(m, live.campaignId);

    // The session summary: the master's block and the player's card.
    const summary = await tableForGold(m, p, `Acessibilidade resumo ouro ${Date.now()}`, 'XP_MODE_ENEMIES');
    campaigns.push(summary.campaignId);
    await startSessionRPC(m, summary.campaignId);
    await threeTreasuresRPC(m, summary);
    await openSessionPage(p, summary.campaignId);
    await openSessionPage(m, summary.campaignId);
    await endSessionFromBar(m);
    await expect(m.getByRole('heading', { name: 'Sessão encerrada' })).toBeVisible();
    await expect(m.getByRole('table', { name: 'Mais tesouro encontrado' })).toBeVisible();
    await expectScreenPasses(m, `Resumo da sessão com Mais tesouro encontrado, mestre ${where}`);
    const card = p.getByRole('region', { name: 'Resumo da sessão' });
    await expect(card.getByRole('heading', { name: 'A sessão acabou' })).toBeVisible();
    await expect(card).toContainText('Mais tesouro encontrado');
    await expectScreenPasses(p, `Cartão com Mais tesouro encontrado, jogador ${where}`);
  } finally {
    for (const id of campaigns) {
      await endOpenSessionRPC(m, id);
    }
    await master.close();
    await player.close();
  }
}

test('"Voltar à cidade" e o tesouro no resumo passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-041', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanGoldScreens(browser, 'light', 1280);
});

test('"Voltar à cidade" e o tesouro no resumo passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-041', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanGoldScreens(browser, 'dark', 390);
});

test('"Voltar à cidade" e o tesouro no resumo passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-041', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanGoldScreens(browser, 'dark', 1024);
});

test('"Voltar à cidade" e o tesouro no resumo passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-041', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanGoldScreens(browser, 'light', 320);
});

// Etapa 9, MR-037: the character's creatures. The sheet's "Criaturas" panel (empty, outside a
// session, with a creature), the cast sheet, the questions in place, the stat block, and the
// master's list with "Dar uma criatura" (a computer's dialog: not on a phone).
async function scanCreatureScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const height = width < 768 ? (width < 360 ? 568 : 844) : width === 1024 ? 768 : 800;
  const options = { colorScheme, viewport: { width, height } };
  const master = await newSignedInContext(browser, 'Mestre Teste', options);
  const player = await newSignedInContext(browser, 'Jogador Teste', options);
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const closed = await tableForCreatures(m, p, `Criaturas fechada ${Date.now()}`, false);
    await p.goto(`/campaigns/${closed.campaignId}/characters/${closed.characterId}`);
    await expect(p.locator('app-creatures-panel').getByRole('heading', { name: 'Criaturas' })).toBeVisible();
    await expectScreenPasses(p, `Criaturas, fora de uma sessão ${where}`);

    const table = await tableForCreatures(m, p, `Criaturas ${Date.now()}`);
    campaignId = table.campaignId;
    const panel = p.locator('app-creatures-panel');
    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(panel.getByRole('heading', { name: 'Criaturas' })).toBeVisible();
    await expectScreenPasses(p, `Criaturas, vazio ${where}`);

    await panel.getByRole('button', { name: 'Convocar Familiar' }).click();
    const sheet = p.getByRole('dialog', { name: 'Convocar Familiar' }).or(p.locator('mat-bottom-sheet-container'));
    await expect(sheet.getByText('Escolha a forma e dê um nome ao familiar.')).toBeVisible();
    await expectScreenPasses(p, `Convocar Familiar, faltando o nome ${where}`);
    await sheet.getByLabel('Nome do familiar').fill('Nanquim');
    await sheet.locator('label', { hasText: /Corvo/ }).click();
    await expect(sheet.getByText('Conjurar como ritual · 1 hora · sem gastar espaço')).toBeVisible();
    await expectScreenPasses(p, `Convocar Familiar, pronto ${where}`);
    await sheet.getByRole('button', { name: 'Convocar o familiar' }).click();
    await expect(panel.getByText('Nanquim chegou.')).toBeVisible();
    await expectScreenPasses(p, `Criaturas, com o Nanquim e o aviso ${where}`);

    await panel.getByRole('button', { name: 'Renomear' }).click();
    await expect(panel.getByLabel('Nome da criatura')).toBeFocused();
    await expectScreenPasses(p, `Criaturas, renomear ${where}`);
    await panel.getByRole('button', { name: 'Cancelar' }).click();
    await panel.getByRole('button', { name: 'Dispensar' }).click();
    await expect(panel.getByRole('button', { name: 'Voltar' })).toBeFocused();
    await expectScreenPasses(p, `Criaturas, dispensar pergunta ${where}`);
    await panel.getByRole('button', { name: 'Voltar' }).click();

    await panel.getByRole('link', { name: 'Ver a ficha de Nanquim' }).click();
    await expect(p.getByRole('heading', { name: 'Nanquim', level: 1 })).toBeVisible();
    await expectScreenPasses(p, `A ficha da criatura ${where}`);
    await p.getByRole('button', { name: 'Dispensar' }).click();
    await expect(p.getByRole('alertdialog', { name: 'Dispensar Nanquim?' })).toBeVisible();
    await expectScreenPasses(p, `A ficha da criatura, dispensar pergunta ${where}`);

    await m.goto(`/campaigns/${campaignId}`);
    const row = m.locator('app-character-creatures');
    await expect(row.locator('.item__name', { hasText: 'Nanquim' })).toBeVisible();
    await expectScreenPasses(m, `O mestre, a lista de personagens com a criatura ${where}`);
    if (width >= 768) {
      // "Dar uma criatura a Pensantus" is an item of the character's "⋮" menu.
      await m.getByRole('button', { name: 'Mais ações de Pensantus' }).click();
      await expectScreenPasses(m, `O mestre, o menu do personagem ${where}`);
      await m.getByRole('menuitem', { name: 'Dar uma criatura a Pensantus' }).click();
      const dialog = m.getByRole('dialog', { name: 'Dar uma criatura a Pensantus' });
      await expect(dialog.getByText(/de 334 · em ordem de nome/)).toBeVisible();
      await expectScreenPasses(m, `Dar uma criatura, aberta ${where}`);
      await dialog.getByLabel('Nome, em português ou inglês').fill('ma');
      await dialog.getByLabel('Tipo').selectOption('beast');
      await dialog.getByLabel('Nível de desafio').selectOption('1/8');
      await expect(dialog.getByText('2 de 334 · em ordem de nome')).toBeVisible();
      await dialog.locator('label', { hasText: /Mastim/ }).click();
      await expect(dialog.getByText(/com os PV do livro \(5\)/)).toBeVisible();
      await expectScreenPasses(m, `Dar uma criatura, o Mastim escolhido ${where}`);
      await dialog.getByRole('button', { name: 'Dar Mastim a Pensantus' }).click();
      await expect(m.getByText('Mastim dado a Pensantus.')).toBeVisible();
      await expectScreenPasses(m, `O mestre, depois de dar ${where}`);
    }
    await row.getByRole('button', { name: 'Dispensar Nanquim' }).click();
    await expect(row.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(m, `O mestre, dispensar pergunta ${where}`);
    await row.getByRole('button', { name: 'Voltar' }).click();

    await m.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    const mc = m.locator('app-creatures-panel app-creature-card').first();
    await mc.getByRole('button', { name: 'Corrigir PV' }).click();
    await expect(mc.getByLabel(/PV de /)).toBeFocused();
    await expectScreenPasses(m, `O mestre, corrigir os PV da criatura ${where}`);

    // A creature that is not there (or that the viewer may not read): the page's not-found state.
    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}/creatures/6f1c7a52-3b5e-4c55-9d0b-2a51f0c1e001`);
    await expect(p.getByRole('heading', { name: 'Criatura não encontrada', level: 1 })).toBeVisible();
    await expectScreenPasses(p, `A ficha da criatura, não encontrada ${where}`);

    // A druid: the slot picker, "Quantas criaturas", the list with "−" and "+", the warning of what a new
    // concentration ends, and a refusal of the server inside the sheet.
    const druid = await tableForCaster(m, p, `Criaturas druida ${Date.now()}`, 'druid');
    const druidCampaign = druid.campaignId;
    try {
      await p.goto(`/campaigns/${druidCampaign}/characters/${druid.characterId}`);
      const dpanel = p.locator('app-creatures-panel');
      await dpanel.getByRole('button', { name: 'Conjurar Animais' }).click();
      const cast = p.getByRole('dialog', { name: 'Conjurar Animais' }).or(p.locator('mat-bottom-sheet-container'));
      await expect(cast.getByText('Quantas criaturas')).toBeVisible();
      await expectScreenPasses(p, `Conjurar Animais, as opções e o espaço ${where}`);
      await cast.locator('label', { hasText: /2\s+feras\s+de\s+ND\s+1\s/ }).click();
      // A row not chosen yet has only "Escolher"; once chosen it has "Menos" and "Mais".
      await cast.getByRole('button', { name: 'Escolher Lobo', exact: true }).click();
      await cast.getByRole('button', { name: 'Mais Lobo', exact: true }).click();
      await expect(cast.locator('.line')).toContainText('2 criaturas · 1 ação · gasta um espaço de 3º nível');
      await expectScreenPasses(p, `Conjurar Animais, a mistura pronta ${where}`);
      await cast.getByRole('button', { name: 'Conjurar Animais', exact: true }).click();
      await expect(dpanel.getByText('2 criaturas chegaram.')).toBeVisible();

      await dpanel.getByRole('button', { name: 'Conjurar Animais' }).click();
      await expect(cast.getByText('Isso encerra Conjurar Animais e dispensa 2 criaturas')).toBeVisible();
      await expectScreenPasses(p, `Conjurar Animais, o aviso do que a concentração encerra ${where}`);
      // The session ends while the sheet is open: the server refuses, and the sheet says so, still open.
      await cast.locator('label', { hasText: /1\s+fera\s+de\s+ND\s+2\s/ }).click();
      await cast.locator('app-creature-choice-list label.row').first().click();
      await endOpenSessionRPC(m, druidCampaign);
      await cast.getByRole('button', { name: 'Conjurar Animais', exact: true }).click();
      await expect(cast.getByRole('alert')).toContainText('A sessão acabou');
      await expectScreenPasses(p, `Conjurar Animais, a recusa do servidor na folha ${where}`);
    } finally {
      await endOpenSessionRPC(m, druidCampaign);
    }
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as criaturas passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanCreatureScreens(browser, 'light', 1280);
});

test('as criaturas passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanCreatureScreens(browser, 'dark', 390);
});

test('as criaturas passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanCreatureScreens(browser, 'dark', 1024);
});

test('as criaturas passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanCreatureScreens(browser, 'light', 320);
});

// The fog of war (Etapa 9, MR-036, E9-03 and E9-04): the player's map with its legend and caption, the tiles on their way,
// what was seen before, the character who is not on the map, the carried light (the row, the sheet, the toast), the
// master's "Ver como" and "Luz dos personagens", and the familiar's eyes (the band, in and out of a combat).
async function scanFogScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width < 700 ? 800 : 900 };
  const contexts = await Promise.all(
    (['Mestre Teste', 'Jogador Teste', 'E-mail Não Verificado'] as const).map((user) =>
      browser.newContext({ storageState: authStatePath(user), colorScheme, viewport }),
    ),
  );
  const [m, p, t] = await Promise.all(contexts.map((c) => c.newPage()));
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  const loaded = async (page: Page) => {
    await expect(page.locator('app-fog-base')).toBeVisible();
    await expect(page.getByTestId('fog-loading')).toHaveCount(0);
  };
  try {
    await Promise.all([m.goto('/'), p.goto('/'), t.goto('/')]);
    const table = await tableForFog(m, p, t, `Acessibilidade névoa ${Date.now()}`, { familiar: { col: 15, row: 8 }, torenOffMap: true });
    campaignId = table.campaignId;

    // The player: the map, the legend, the caption, the row of the light; the party's chips on a phone.
    // (On a phone the session page has the map as one row: the whole map, with the fog and its legend, is the page "Ver mapa" opens.)
    await openPlayerMap(p, sessionRoute(campaignId));
    await loaded(p);
    await expectScreenPasses(p, `A névoa, a vista do jogador ${where}`);

    // The tiles on their way: still stripes with the dashed border, and the notice.
    await p.route('**/tiles/**', async (route) => {
      await new Promise((r) => setTimeout(r, 4000));
      await route.continue().catch(() => undefined);
    });
    await p.reload();
    await expect(p.getByTestId('fog-loading')).toBeVisible();
    await expectScreenPasses(p, `A névoa, carregando o mapa ${where}`);
    await p.unrouteAll({ behavior: 'ignoreErrors' });
    await loaded(p);

    // What was seen stays, darkened.
    await moveTo(m, table, table.pensantusId, 10, 13);
    await expect(p.locator('.mr-legend').getByText('Já visto', { exact: true })).toBeVisible();
    await expectScreenPasses(p, `A névoa, o que já foi visto ${where}`);

    // A character who is not on the map (Toren): the fixed notice with its icon.
    await openPlayerMap(t, sessionRoute(campaignId));
    await expect(t.getByTestId('fog-off-map')).toBeVisible();
    await expectScreenPasses(t, `A névoa, o personagem fora do mapa ${where}`);

    // The carried light: the row on the session page (the map's row on a phone), the sheet, then the toast.
    await p.goto(sessionRoute(campaignId));
    await expect(p.getByRole('button', { name: /Luz que você carrega/ })).toBeVisible();
    await expectScreenPasses(p, `A névoa, a linha da luz na sessão ${where}`);
    await p.getByRole('button', { name: /Luz que você carrega/ }).click();
    const sheet = p.getByRole('dialog', { name: 'Luz que você carrega' });
    await expect(sheet).toBeVisible();
    await expectScreenPasses(p, `Luz que você carrega, a folha ${where}`);
    await sheet.getByText('Tocha', { exact: true }).click();
    await expect(sheet.getByRole('radio', { name: /Tocha/ })).toBeChecked();
    await sheet.getByRole('button', { name: 'Pronto' }).click();
    await expect(p.getByRole('status').filter({ hasText: 'Você acendeu a tocha' })).toBeVisible();
    await expectScreenPasses(p, `Luz que você carrega, a tocha acesa ${where}`);

    // The master: "Ver como" and "Luz dos personagens", then the map as Pensantus sees it.
    await m.goto(sessionRoute(campaignId));
    await showMapTool(m, 'Névoa e luz');
    const list = m.getByRole('radiogroup', { name: 'Ver como' });
    await expect(list.getByRole('radio', { name: /Pensantus/ })).toContainText(/\d+\s+quadrados vistos/);
    await expectScreenPasses(m, `Ver como, a lista e a luz dos personagens ${where}`);
    // Nothing sticks out of the page, whatever its width (the "Ver como" and light cards at 320 px did).
    expect(await m.evaluate(() => document.documentElement.scrollWidth - window.innerWidth), `rolagem lateral do mestre ${where}`).toBeLessThanOrEqual(0);
    expect(await p.evaluate(() => document.documentElement.scrollWidth - window.innerWidth), `rolagem lateral do jogador ${where}`).toBeLessThanOrEqual(0);
    await list.getByRole('radio', { name: /Pensantus/ }).click();
    await expect(m.getByText('Você está vendo o mapa como Pensantus.', { exact: false })).toBeVisible();
    await loaded(m);
    await expectScreenPasses(m, `Ver como, o mapa de Pensantus ${where}`);

    // The familiar's eyes, out of a combat: the row under the legend, the question, then the band with the one filled button.
    const familiar = p.getByRole('region', { name: 'Seu familiar Nanquim' });
    await expect(familiar).toBeVisible();
    await expectScreenPasses(p, `Pelos olhos do Nanquim, a linha do familiar ${where}`);
    await familiar.getByRole('button', { name: 'Ver pelos olhos' }).click();
    const question = p.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' });
    await expect(question).toBeVisible();
    await expectScreenPasses(p, `Pelos olhos do Nanquim, a pergunta ${where}`);
    await question.getByRole('button', { name: 'Ver pelos olhos' }).click();
    await expect(p.getByTestId('familiar-band')).toBeVisible();
    await expectScreenPasses(p, `Pelos olhos do Nanquim, a faixa ${where}`);
    await p.getByRole('button', { name: 'Voltar aos seus olhos' }).click();
    await expect(p.getByTestId('familiar-band')).toHaveCount(0);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await Promise.all(contexts.map((c) => c.close()));
  }
}

test('a névoa de guerra passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFogScreens(browser, 'light', 1280);
});

test('a névoa de guerra passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFogScreens(browser, 'dark', 390);
});

test('a névoa de guerra passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFogScreens(browser, 'dark', 1024);
});

test('a névoa de guerra passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFogScreens(browser, 'light', 320);
});


// The fog in a combat (Etapa 9, MR-036): the combat's map with the same fog and legend, the action "Ver pelos olhos do Nanquim", the band
// "Até o começo da sua próxima vez" with the "Cego" line, and the master's "Ver como" and "Luz dos personagens" under the combat.
async function scanFogCombatScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width < 700 ? 800 : 900 };
  const contexts = await Promise.all(
    (['Mestre Teste', 'Jogador Teste', 'E-mail Não Verificado'] as const).map((user) =>
      browser.newContext({ storageState: authStatePath(user), colorScheme, viewport }),
    ),
  );
  const [m, p, t] = await Promise.all(contexts.map((c) => c.newPage()));
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await Promise.all([m.goto('/'), p.goto('/'), t.goto('/')]);
    const table = await tableForFog(m, p, t, `Acessibilidade névoa no combate ${Date.now()}`, { familiar: { col: 15, row: 8 } });
    campaignId = table.campaignId;
    await beginFogCombat(m, table);

    await p.goto(sessionRoute(campaignId));
    await expect(p.getByRole('heading', { name: /Sua vez, Pensantus/ })).toBeVisible();
    await expect(p.locator('app-fog-base')).toBeVisible();
    await expect(p.getByTestId('fog-loading')).toHaveCount(0);
    await expectScreenPasses(p, `Combate na névoa, o mapa e a luz ${where}`);

    await openActionTab(p, 'Outras');
    const action = p.locator('app-action-row', { hasText: 'Ver pelos olhos do Nanquim' });
    await expect(action).toBeVisible();
    await expectScreenPasses(p, `Combate na névoa, a ação "Ver pelos olhos do Nanquim" ${where}`);
    await action.getByRole('button', { name: 'Ver pelos olhos do Nanquim' }).click();
    const question = p.getByRole('dialog', { name: 'Ver pelos olhos do Nanquim?' });
    await expect(question).toBeVisible();
    await expectScreenPasses(p, `Combate na névoa, a pergunta com o custo da ação ${where}`);
    await question.getByRole('button', { name: 'Ver pelos olhos' }).click();
    await expect(p.getByTestId('familiar-band')).toBeVisible();
    await expect(p.getByTestId('familiar-blind')).toBeVisible();
    await expectScreenPasses(p, `Combate na névoa, a faixa e a linha "Cego" ${where}`);

    await m.goto(sessionRoute(campaignId));
    // In the combat the fog tools are a row of "Mais do combate".
    await openCombatPanel(m, 'Névoa e luz');
    await expect(m.getByRole('radiogroup', { name: 'Ver como' })).toBeVisible();
    await m.getByRole('radio', { name: /Toren/ }).click();
    await expect(m.locator('app-view-as-map')).toBeVisible();
    await expect(m.getByTestId('fog-loading')).toHaveCount(0);
    await expectScreenPasses(m, `Combate na névoa, o mestre vendo como Toren ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await Promise.all(contexts.map((c) => c.close()));
  }
}

test('a névoa no combate passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFogCombatScreens(browser, 'light', 1280);
});

test('a névoa no combate passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFogCombatScreens(browser, 'dark', 390);
});

// Traps and treasure in the session (slice 9.14, MR-035, MR-041, E9-08, E9-09): the master's cards and
// their dialogs, the damage that waits, the player's search sheet in each step, the toast and the treasure's
// sheet. Every state goes through axe and the alignment checks.
async function scanTrapScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade armadilhas ${Date.now()}`, true, true);
    campaignId = table.campaignId;
    await trapRPC(m, table, 'Fosso escondido', 6, 7, { noticeDc: 30, damage: '3' });
    await trapRPC(m, table, 'Agulha envenenada', 15, 10, { manual: true, noticeDc: 0, findDc: 20 });
    await trapRPC(m, table, 'Fosso fundo', 8, 7, { noticeDc: 30, damage: '3' });
    await treasureRPC(m, table, 'Baú de moedas', 12, 7);
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    // The master's cards, the reveal and the fire dialogs (the "Armadilhas" tool under the map).
    await showMapTool(m, 'Armadilhas');
    const panel = m.getByRole('region', { name: 'Armadilhas do mapa' });
    const card = panel.getByRole('article', { name: 'Fosso escondido' });
    await expect(card).toContainText('Quem notaria');
    await expectScreenPasses(m, `Armadilhas do mapa, o cartão aberto ${where}`);
    await card.getByRole('button', { name: 'Revelar para…' }).click();
    await expect(m.getByRole('dialog', { name: 'Revelar armadilha' })).toBeVisible();
    await expectScreenPasses(m, `Revelar armadilha ${where}`);
    await m.getByRole('dialog', { name: 'Revelar armadilha' }).getByRole('button', { name: 'Cancelar' }).click();
    await card.getByRole('button', { name: 'Disparar…' }).click();
    await expect(m.getByRole('dialog', { name: /Disparar/ })).toBeVisible();
    await expectScreenPasses(m, `Disparar a armadilha ${where}`);
    await m.getByRole('dialog', { name: /Disparar/ }).getByRole('button', { name: 'Disparar para quem está na área' }).click();
    await expect(card).toContainText('Disparada');
    await expectScreenPasses(m, `Armadilha disparada e o registro ${where}`);

    // The treasure: hidden, the form in place, found, the question in place (the "Tesouro" tool).
    await showMapTool(m, 'Tesouro');
    const chest = m.getByRole('region', { name: 'Tesouros do mapa' }).getByRole('article', { name: 'Baú de moedas' });
    await expectScreenPasses(m, `Tesouros do mapa, escondido ${where}`);
    await chest.getByRole('button', { name: 'Marcar como encontrado: Baú de moedas' }).click();
    await expectScreenPasses(m, `Marcar como encontrado no lugar ${where}`);
    await chest.locator('label', { hasText: 'Pensantus' }).click();
    await chest.getByRole('button', { name: 'Marcar como encontrado' }).click();
    await expect(chest).toContainText('Encontrado por Pensantus');
    // The toast goes away on its own: scan the player's screen before the master's.
    await expect(p.getByRole('status').filter({ hasText: 'Pensantus encontrou: Baú de moedas' })).toBeVisible();
    await expectScreenPasses(p, `Aviso do tesouro achado, jogador ${where}`);
    await expectScreenPasses(m, `Tesouro achado ${where}`);
    await chest.getByRole('button', { name: 'Desmarcar' }).click();
    await expectScreenPasses(m, `Desmarcar no lugar ${where}`);
    await chest.getByRole('alertdialog').getByRole('button', { name: 'Voltar' }).click();

    // The player: the toast, the row and sheet of the treasure, and the search in each step.
    await p.getByRole('button', { name: /^Baú de moedas\s+Tesouro/ }).click();
    await expect(p.getByRole('dialog', { name: 'Baú de moedas' })).toBeVisible();
    await expectScreenPasses(p, `Folha do tesouro, jogador ${where}`);
    await p.getByRole('dialog', { name: 'Baú de moedas' }).getByRole('button', { name: 'Fechar', exact: true }).last().click();
    await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
    const sheet = p.getByRole('dialog', { name: 'Procurar armadilhas' });
    await expectScreenPasses(p, `Procurar armadilhas, como ${where}`);
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para/).fill('2');
    await expectScreenPasses(p, `Procurar armadilhas, o dado ${where}`);
    await sheet.getByRole('button', { name: /Confirmar 2/ }).click();
    await expect(sheet).toContainText(/Você (achou|não encontrou)/);
    await expectScreenPasses(p, `Procurar armadilhas, o resultado ${where}`);
    await sheet.getByRole('button', { name: 'Fechar', exact: true }).last().click();

    // "Outra perícia…": the list of the other skills, then one picked.
    await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
    await sheet.locator('label', { hasText: 'Outra perícia…' }).click();
    await expect(sheet.getByRole('listbox', { name: 'Perícia' })).toBeVisible();
    await expectScreenPasses(p, `Procurar armadilhas, Outra perícia ${where}`);
    await sheet.getByRole('listbox', { name: 'Perícia' }).getByRole('option', { name: /^Arcanismo/ }).click();
    await expect(sheet.getByText('Escolha uma perícia')).toHaveCount(0);
    await expectScreenPasses(p, `Procurar armadilhas, a perícia escolhida ${where}`);
    await p.keyboard.press('Escape');
    await expect(sheet).toBeHidden();

    // In a combat: the damage that waits for the master, and the player's note.
    await pensantusFirst(m, table);
    await movePensantus(p, table, 9, 7);
    await openSessionPage(m, campaignId);
    await expectScreenPasses(m, `Combate com o dano de armadilha esperando ${where}`);
    await expectScreenPasses(p, `Combate, a nota da queda, jogador ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as armadilhas e os tesouros na sessão passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-035', '@MR-041'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanTrapScreens(browser, 'light', 1280);
});

test('as armadilhas e os tesouros na sessão passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-035', '@MR-041'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanTrapScreens(browser, 'dark', 390);
});

test('as armadilhas e os tesouros na sessão passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-035', '@MR-041'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanTrapScreens(browser, 'dark', 1024);
});

test('as armadilhas e os tesouros na sessão passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-035', '@MR-041'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanTrapScreens(browser, 'light', 320);
});

// ---- the creatures in combat and Wild Shape (MR-037, E9-11, E9-12) ----

/** Taps a button of the page on a phone, where the pinned turn bar can cover half the screen: it is brought up under the app bar first. */
async function tapAboveBar(button: import('@playwright/test').Locator): Promise<void> {
  await button.evaluate((el) => {
    el.scrollIntoView({ block: 'start' });
    window.scrollBy(0, -140);
  });
  await button.click();
}

async function scanCreatureCombatScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width < 700 ? 800 : 900 };
  const contexts = await Promise.all(
    (['Mestre Teste', 'Jogador Teste', 'E-mail Não Verificado'] as const).map((user) =>
      browser.newContext({ storageState: authStatePath(user), colorScheme, viewport }),
    ),
  );
  const [m, p, t] = await Promise.all(contexts.map((c) => c.newPage()));
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await Promise.all([m.goto('/'), p.goto('/'), t.goto('/')]);
    const table = await tableForCreatureCombat(m, p, t, `Acessibilidade criaturas ${Date.now()}`);
    campaignId = table.campaignId;
    await beginCreatureCombat(m, table, { Sálvia: p, Toren: t }, { Toren: 20, Sálvia: 13, 'Capitão Goblin': 5, 'Goblin 1': 4, 'Goblin 2': 4 }, { 'Capitão Goblin': [21, 3], 'Goblin 1': [6, 6], 'Goblin 2': [20, 7] });
    await passTurnsTo(m, campaignId, 'Sálvia');
    await p.goto(sessionRoute(campaignId));
    await expect(p.getByRole('heading', { name: 'Sua vez, Sálvia' })).toBeVisible();
    await expectScreenPasses(p, `A vez da Sálvia, com Forma Selvagem ${where}`);

    // Wild Shape: the list of beasts, with one chosen.
    await openActionTab(p, 'Druida');
    await tapAboveBar(p.getByRole('button', { name: 'Transformar: Forma Selvagem' }));
    const wild = p.getByRole('dialog', { name: 'Forma Selvagem' });
    await expect(wild.getByText('Escolha uma fera.')).toBeVisible();
    await expectScreenPasses(p, `Forma Selvagem, a lista ${where}`);
    await wild.getByLabel('Buscar fera').fill('lobo');
    await wild.locator('label', { hasText: '(Wolf)' }).click();
    await expect(wild.locator('.row__sub').first()).toContainText('CA 13');
    await expectScreenPasses(p, `Forma Selvagem, a fera escolhida ${where}`);
    await wild.getByRole('button', { name: 'Virar Lobo' }).click();
    await expect(p.getByText('Na forma de Lobo').first()).toBeVisible();
    await expectScreenPasses(p, `A vez como Lobo ${where}`);

    // The beast falls: the notice that stays.
    await hitAndApply(m, campaignId, 'Goblin 1', 'Sálvia', 30);
    await expect(p.getByTestId('form-ended')).toBeVisible();
    await expectScreenPasses(p, `O aviso da fera que caiu ${where}`);
    await p.getByTestId('form-ended').getByRole('button', { name: 'Entendi' }).click();
    // Next round: her action is free again.
    await endTurnOf(p, m, campaignId, 'Sálvia');
    await waitForEncounterRevision(p, await passTurnsTo(m, campaignId, 'Sálvia'));
    await expect(p.getByRole('heading', { name: 'Sua vez, Sálvia' })).toBeVisible();

    // Conjurar Animais: the sheet, then the result.
    await openActionTab(p, 'Magias');
    await tapAboveBar(p.getByRole('button', { name: 'Conjurar Conjurar Animais' }));
    // The dialog's name is its title, which changes to "Lobos atrozes conjurados" with the result.
    const sheet = p.getByRole('dialog');
    await sheet.getByText('2 feras de ND 1 ou menos').click();
    await sheet.getByRole('button', { name: 'Escolher Lobo atroz' }).click();
    await sheet.getByRole('button', { name: 'Mais Lobo atroz' }).click();
    await expectScreenPasses(p, `Conjurar Animais em combate ${where}`);
    await sheet.getByRole('button', { name: 'Digitar o d20 de um dado físico' }).click();
    await sheet.getByLabel('Role 1d20 para a iniciativa das criaturas').fill('8');
    await expectScreenPasses(p, `Conjurar Animais, o d20 digitado ${where}`);
    await sheet.getByRole('button', { name: 'Conjurar Animais', exact: true }).click();
    await expect(sheet.getByRole('heading', { name: 'Lobos atrozes conjurados' })).toBeVisible();
    await expectScreenPasses(p, `Os Lobos atrozes conjurados ${where}`);
    await sheet.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(p.getByRole('tablist', { name: 'O que você joga' })).toBeVisible();
    await expectScreenPasses(p, `As abas, a vez da Sálvia ${where}`);

    // The wolves' turn, and what Toren sees.
    await endTurnOf(p, m, campaignId, 'Sálvia');
    await expect(p.getByRole('heading', { name: 'Vez dos seus Lobos atrozes' })).toBeVisible();
    await expectScreenPasses(p, `A vez dos Lobos atrozes ${where}`);
    await p.getByRole('button', { name: 'Encerrar a parte dos Lobos' }).click();
    await expect(p.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(p, `Encerrar a parte dos Lobos, a pergunta ${where}`);
    await p.getByRole('alertdialog').getByRole('button', { name: 'Voltar' }).click();
    await t.goto(sessionRoute(campaignId));
    await expect(t.getByRole('heading', { name: 'Vez dos Lobos atrozes da Sálvia' })).toBeVisible();
    await expectScreenPasses(t, `A vez dos Lobos atrozes, vista por outro jogador ${where}`);

    // The master: the order with the group box, the legend and the concentration question.
    await m.goto(sessionRoute(campaignId));
    const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
    await openOrderRow(m, 'Sálvia');
    await expect(order.getByText('Concentração: Conjurar Animais · 2 Lobos atrozes')).toBeVisible();
    await expectScreenPasses(m, `A ordem do mestre com as criaturas ${where}`);
    await order.getByRole('button', { name: 'Perdeu a concentração' }).click();
    await expect(order.getByRole('alertdialog')).toBeVisible();
    await expectScreenPasses(m, `Perdeu a concentração, a pergunta ${where}`);
    await order.getByRole('alertdialog').getByRole('button', { name: 'Dispensar os Lobos' }).click();
    await expect(p.getByTestId('concentration-lost')).toBeVisible();
    await expectScreenPasses(p, `O aviso da concentração perdida ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await Promise.all(contexts.map((c) => c.close()));
  }
}

test('as criaturas no combate e a Forma Selvagem passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanCreatureCombatScreens(browser, 'light', 1280);
});

test('as criaturas no combate e a Forma Selvagem passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanCreatureCombatScreens(browser, 'dark', 390);
});

test('as criaturas no combate e a Forma Selvagem passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanCreatureCombatScreens(browser, 'dark', 1024);
});

test('as criaturas no combate e a Forma Selvagem passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-037'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanCreatureCombatScreens(browser, 'light', 320);
});

// The map editor (slice 9.12, MR-034, MR-035, MR-036, MR-041, E9-01 and E9-02): painting with each tool, the in-place questions, the
// fog's settings, the Luz, Armadilha and Tesouro panels in each state, the map with no grid, with a combat on it, "Ver como", and,
// on a phone, the manage view with its fixed notice. Every state goes through axe and the alignment checks.
async function scanMapEditorScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const pensantus = await newSignedInContext(browser, 'Jogador Teste');
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado');
  const [m, ap, bp] = [await master.newPage(), await pensantus.newPage(), await toren.newPage()];
  const where = `(${colorScheme}, ${width}px)`;
  const phone = width < 768;
  let campaignId = '';
  try {
    await Promise.all([m.goto('/'), ap.goto('/'), bp.goto('/')]);
    const table = await tableForFog(m, ap, bp, `Acessibilidade editor ${Date.now()}`, {});
    campaignId = table.campaignId;
    await cavePoints(m, table);
    const image = await uploadImageRPC(m, campaignId, 'Sem grade', await canvasPng(m, 960, 640, 'Sem grade'));
    const noGrid = await createMapRPC(m, campaignId, 'Mapa sem grade', image);
    const map = { columns: 24, rows: 16 };
    const route = editorRoute(campaignId, table.mapId);
    // Back to the list with nothing chosen: the editor asks before it leaves a point with unsaved changes, so these scans start again.
    const reopen = async () => {
      await m.goto(route);
      await expect(m.getByRole('radio', { name: 'Pontos' })).toBeVisible();
    };
    const list = m.getByRole('region', { name: 'Pontos do mapa' });
    const tools = m.getByRole('group', { name: 'Ferramenta de pintura' });

    if (phone) {
      await m.goto(editorRoute(campaignId, table.mapId));
      await expect(m.getByText('Pintar só no computador')).toBeVisible();
      await expectLoaded(m);
      await expectScreenPasses(m, `Mapa no celular, sem pintura ${where}`);
      await m.getByRole('button', { name: 'Esquecer o que foi visto' }).click();
      await expect(m.getByRole('heading', { name: 'Esquecer o que foi visto?' })).toBeVisible();
      await expectScreenPasses(m, `Esquecer o que foi visto, no celular ${where}`);
      await m.goto(editorRoute(campaignId, noGrid));
      await expect(m.getByText('Pintar só no computador')).toBeVisible();
      await expectLoaded(m);
      await expectScreenPasses(m, `Mapa sem grade no celular ${where}`);
      return;
    }

    // Pontos: the list, and each kind's panel.
    await m.goto(editorRoute(campaignId, table.mapId));
    await expect(m.getByRole('radio', { name: 'Pontos' })).toBeVisible();
    await expectLoaded(m);
    await expectScreenPasses(m, `Editor, Pontos, a lista ${where}`);
    await list.getByRole('button', { name: /Fosso escondido/ }).click();
    await expect(m.getByRole('heading', { name: 'Predefinições do SRD' })).toBeVisible();
    await expect(m.getByText('Percepção passiva contra a CD 15')).toBeVisible();
    await expectScreenPasses(m, `Armadilha, o formulário e Quem notaria ${where}`);
    await m.getByRole('button', { name: 'Acrescentar perícia' }).click();
    await expect(m.getByRole('listbox', { name: 'Também acham com' })).toBeVisible();
    await expectScreenPasses(m, `Armadilha, Também acham com, a lista ${where}`);
    await m.getByRole('listbox', { name: 'Também acham com' }).getByRole('option', { name: 'Arcanismo' }).click();
    await m.keyboard.press('Escape');
    await expect(m.locator('.tp__chip', { hasText: 'Arcanismo' })).toBeVisible();
    await expectScreenPasses(m, `Armadilha, Também acham com, a etiqueta ${where}`);
    await m.getByLabel('CD para achar (Investigação)').fill('40');
    await m.getByRole('button', { name: 'Salvar ponto' }).click();
    await expect(m.getByText('Use uma CD de 1 a 30.')).toBeVisible();
    await expectScreenPasses(m, `Armadilha, um campo com erro ${where}`);
    await m.getByRole('radio', { name: /Agulha envenenada/ }).click();
    await expect(m.getByRole('heading', { name: 'Teste de resistência' })).toBeVisible();
    await expectScreenPasses(m, `Armadilha, a Agulha envenenada em partes ${where}`);
    // The form has unsaved changes (the preset): going to "Pintar" asks in place.
    await m.getByRole('radio', { name: 'Pintar' }).click();
    await expect(m.getByRole('heading', { name: /Salvar as mudanças em/ })).toBeFocused();
    await expectScreenPasses(m, `Salvar as mudanças? ${where}`);
    await m.getByRole('button', { name: 'Continuar editando' }).click();
    await reopen();
    await list.getByRole('button', { name: /Brasa do altar/ }).click();
    await expect(m.getByRole('radiogroup', { name: 'Tipo de luz' })).toBeVisible();
    await expectScreenPasses(m, `Luz personalizada ${where}`);
    await m.getByRole('radio', { name: /Tocha/ }).click();
    await expectScreenPasses(m, `Luz, uma predefinição ${where}`);
    await reopen();
    // The list is drawn a moment after the page: pick the row again until the panel is there.
    await expect(async () => {
      await list.getByRole('button', { name: /Baú de moedas/ }).click();
      await expect(m.getByRole('heading', { name: 'Baú de moedas', level: 2 })).toBeVisible({ timeout: 2_000 });
    }).toPass();
    await expect(m.locator('app-treasure-point-panel').getByText('Não encontrado')).toBeVisible();
    await expectScreenPasses(m, `Tesouro escondido ${where}`);
    await m.getByRole('button', { name: 'Marcar como encontrado' }).click();
    await expect(m.getByRole('group', { name: /Quem encontrou/ })).toBeVisible();
    await expectScreenPasses(m, `Tesouro, quem encontrou ${where}`);
    await m.getByRole('group', { name: /Quem encontrou/ }).locator('label', { hasText: 'Pensantus' }).click();
    await m.getByRole('button', { name: 'Marcar como encontrado' }).last().click();
    await expect(m.getByText(/Encontrado por Pensantus/)).toBeVisible();
    await expectScreenPasses(m, `Tesouro encontrado ${where}`);
    await m.getByRole('button', { name: 'Desmarcar' }).click();
    await expect(m.getByRole('group', { name: /^Desmarcar / })).toBeVisible();
    await expectScreenPasses(m, `Tesouro, desmarcar no lugar ${where}`);
    await m.getByRole('group', { name: /^Desmarcar / }).getByRole('button', { name: 'Desmarcar' }).click();
    await reopen();

    // Ver como
    await expect(m.getByRole('heading', { name: 'Ver como' })).toBeVisible();
    await m.getByRole('radio', { name: /Toren/ }).click();
    await expect(m.getByText('Você está vendo o mapa como Toren')).toBeVisible();
    await expectLoaded(m);
    await expectScreenPasses(m, `Ver como Toren, no editor ${where}`);
    await m.getByRole('button', { name: 'Voltar à sua vista' }).click();

    // Pintar: every tool, the brush, the layers.
    await reopen();
    await m.getByRole('radio', { name: 'Pintar' }).click();
    await expect(tools).toBeVisible();
    await tools.getByRole('button', { name: 'Terreno difícil' }).click();
    await dragSquares(m, map, [4, 9], [5, 10]);
    await expect(m.locator('app-layers-panel').getByText('Tudo salvo').first()).toBeVisible();
    await expectScreenPasses(m, `Pintar, Terreno difícil ${where}`);
    await tools.getByRole('button', { name: 'Cobertura' }).click();
    await m.getByRole('radio', { name: 'Três quartos' }).click();
    await clickSquare(m, map, 20, 4);
    await expectScreenPasses(m, `Pintar, Cobertura e o grau ${where}`);
    await tools.getByRole('button', { name: 'Luz' }).click();
    await m.getByRole('radio', { name: 'Claro' }).first().click();
    await m.getByRole('radio', { name: '3×3' }).click();
    await clickSquare(m, map, 8, 13);
    await expect(m.locator('app-layers-panel').getByText('Tudo salvo').first()).toBeVisible();
    await expectScreenPasses(m, `Pintar, Luz com os glifos ${where}`);
    await tools.getByRole('button', { name: 'Apagar' }).click();
    await expectScreenPasses(m, `Pintar, Apagar ${where}`);

    // The questions in place.
    await m.getByRole('button', { name: 'Mudar a grade' }).click();
    await expect(m.getByRole('heading', { name: 'Mudar a grade?' })).toBeFocused();
    await expectScreenPasses(m, `Mudar a grade? ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await m.getByRole('button', { name: 'Esquecer o que foi visto' }).click();
    await expect(m.getByRole('heading', { name: 'Esquecer o que foi visto?' })).toBeFocused();
    await expectScreenPasses(m, `Esquecer o que foi visto? ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).click();
    await m.evaluate(() => window.scrollTo(0, 0));
    await m.getByRole('button', { name: 'Trocar imagem' }).click();
    await expect(m.getByRole('heading', { name: 'Trocar a imagem?' })).toBeFocused();
    await expectScreenPasses(m, `Trocar a imagem? ${where}`);
    await m.getByRole('button', { name: 'Voltar' }).first().click();

    // No grid, and a combat on the map.
    await m.goto(editorRoute(campaignId, noGrid));
    await m.getByRole('radio', { name: 'Pintar' }).click();
    await expect(m.getByText('Defina a grade para pintar e ligar a névoa.')).toBeVisible();
    await expectLoaded(m);
    await expectScreenPasses(m, `Mapa sem grade, Pintar ${where}`);
    await beginFogCombat(m, table);
    await m.goto(editorRoute(campaignId, table.mapId));
    await m.getByRole('radio', { name: 'Pintar' }).click();
    await expect(m.getByText('Combate em andamento')).toBeVisible();
    await expectLoaded(m);
    await expectScreenPasses(m, `Combate no mapa, Pintar ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }
}

test('o editor do mapa passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-034', '@MR-035', '@MR-036', '@MR-041'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMapEditorScreens(browser, 'light', 1280);
});

test('o editor do mapa passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-034', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanMapEditorScreens(browser, 'dark', 390);
});

test('o editor do mapa passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-034', '@MR-035', '@MR-036', '@MR-041'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMapEditorScreens(browser, 'dark', 1024);
});

test('o editor do mapa passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-034', '@MR-036'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanMapEditorScreens(browser, 'light', 320);
});

// The doors (slice 10.14a, MR-010, RN-26, RN-10; E10-05 7 to 11): the "Porta" tool in "Pintar" with its panel, its refusal and its
// question; the master's door sheet in the session (a door, a secret door and the question before revealing it); what a player's
// map says (only "Porta fechada"); and the "Mover" page after a locked door stopped the move. Every state goes through axe and the
// alignment checks. On a phone the editor is "Pintar só no computador" (the existing scans), so only the session's states run there.
async function scanDoorScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const pensantus = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado');
  const [m, ap, bp] = [await master.newPage(), await pensantus.newPage(), await toren.newPage()];
  const where = `(${colorScheme}, ${width}px)`;
  const phone = width < 768;
  let campaignId = '';
  const door = (state: string, col: number, row: number) => m.getByRole('button', { name: new RegExp(`^${state}, coluna ${col}, linha ${row}`) });
  // Opens a door's sheet by keyboard: a phone's map is zoomed on the party, so the door may be off the screen.
  const openDoor = async (state: string, col: number, row: number) => {
    await door(state, col, row).focus();
    await m.keyboard.press('Enter');
  };
  try {
    await Promise.all([m.goto('/'), ap.goto('/'), bp.goto('/')]);
    const table = await tableForFog(m, ap, bp, `Acessibilidade portas ${Date.now()}`, {});
    campaignId = table.campaignId;
    const target = { campaignId, mapId: table.mapId };
    const paint = async (layer: string, value: number, squares: [number, number][]) => {
      const res = await callRPC(m, 'meurpg.maps.v1.MapService/PaintMapCells', { ...target, layer, value, squares: squares.map(([col, row]) => ({ col, row })) });
      expect(res.ok(), await res.text()).toBeTruthy();
    };
    // The wall at (7, 9) and (7, 10) has floor on both sides: a closed and a locked door; a grade, an open door and a secret one stand on the floor.
    await paint('MAP_LAYER_WALL', 0, [[7, 9], [7, 10]]);
    await paint('MAP_LAYER_DOORS', 2, [[7, 9]]);
    await paint('MAP_LAYER_DOORS', 3, [[7, 10]]);
    await paint('MAP_LAYER_DOORS', 4, [[3, 7]]);
    await paint('MAP_LAYER_DOORS', 1, [[2, 7]]);
    await paint('MAP_LAYER_DOORS', 5, [[1, 7]]);

    if (!phone) {
      // The "Porta" tool: its panel, the kinds, a refused tap and the question before a door goes where someone stands.
      await m.goto(editorRoute(campaignId, table.mapId));
      await expect(m.getByRole('radio', { name: 'Pontos' })).toBeVisible();
      await m.getByRole('radio', { name: 'Pintar' }).click();
      const tools = m.getByRole('group', { name: 'Ferramenta de pintura' });
      await tools.getByRole('button', { name: 'Porta' }).click();
      await expect(m.getByText('Toque num quadrado para pôr a porta do tipo escolhido.')).toBeVisible();
      await expect(m.locator('app-layers-panel').getByText('Tudo salvo').first()).toBeVisible();
      await expectScreenPasses(m, `Editor, Pintar, a ferramenta Porta ${where}`);
      await m.getByRole('group', { name: 'Tipo de porta' }).getByRole('button', { name: 'Trancada' }).click();
      await expectScreenPasses(m, `Editor, Porta trancada escolhida ${where}`);
      const map = { columns: 24, rows: 16 };
      await clickSquare(m, map, 12, 12);
      await expect(m.getByRole('alert').filter({ hasText: 'uma porta precisa de chão dos dois lados' })).toBeVisible();
      await expectScreenPasses(m, `Editor, Porta: um toque que não serve ${where}`);
      // Toren stands on a wall square that has floor on both sides: a closed door there asks first.
      await moveTo(m, table, table.torenId, 7, 11);
      await paint('MAP_LAYER_WALL', 1, [[7, 11]]);
      await m.goto(editorRoute(campaignId, table.mapId));
      await m.getByRole('radio', { name: 'Pintar' }).click();
      await m.getByRole('group', { name: 'Ferramenta de pintura' }).getByRole('button', { name: 'Porta' }).click();
      await m.getByRole('group', { name: 'Tipo de porta' }).getByRole('button', { name: 'Fechada' }).click();
      await clickSquare(m, map, 7, 11);
      await expect(m.getByText('Pôr a porta onde há alguém?')).toBeVisible();
      await expectScreenPasses(m, `Editor, Porta: a pergunta de quem está no quadrado ${where}`);
      await m.getByRole('button', { name: 'Voltar' }).click();
    }

    // The session: the master's map names every door and each door has its sheet; the player's names only "Porta fechada".
    await Promise.all([m.goto(sessionRoute(campaignId)), ap.goto(sessionRoute(campaignId))]);
    await followMapLink(ap);
    await showMasterTab(m, 'mapa');
    await expect(m.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta secreta (só você vê)')).toBeVisible();
    await expect(m.getByRole('group', { name: /^Mapa A caverna/ })).toBeVisible();
    await expectScreenPasses(m, `Sessão, o mestre, o mapa com as portas e a legenda ${where}`);
    await expect(ap.getByRole('list', { name: 'Legenda do mapa' }).getByText('Porta fechada')).toBeVisible();
    await expect(ap.locator('app-fog-base').first()).toBeVisible();
    await expectScreenPasses(ap, `Sessão, o jogador, o mapa com "Porta fechada" ${where}`);
    await openDoor('Porta fechada', 8, 10);
    await expect(m.getByRole('dialog', { name: 'Porta' }).getByRole('radio', { name: /Fechada/ })).toHaveAttribute('aria-checked', 'true');
    await expectScreenPasses(m, `Sessão, a folha da porta fechada ${where}`);
    await m.keyboard.press('Escape');
    await expect(m.getByRole('dialog', { name: 'Porta' })).toHaveCount(0);
    await openDoor('Grade', 4, 8);
    await expect(m.getByRole('dialog', { name: 'Porta' }).getByRole('radio')).toHaveCount(2);
    await expectScreenPasses(m, `Sessão, a folha de uma grade ${where}`);
    await m.keyboard.press('Escape');
    await expect(m.getByRole('dialog', { name: 'Porta' })).toHaveCount(0);
    await openDoor('Porta secreta', 2, 8);
    const sheet = m.getByRole('dialog', { name: 'Porta' });
    await expect(sheet.getByRole('heading', { name: 'Porta secreta' })).toBeVisible();
    await expectScreenPasses(m, `Sessão, a folha da porta secreta ${where}`);
    await sheet.getByRole('button', { name: 'Revelar a porta secreta' }).click();
    await expect(sheet.getByText('Os jogadores vão ver a porta. Revelar?')).toBeVisible();
    await expectScreenPasses(m, `Sessão, a pergunta antes de revelar a porta secreta ${where}`);
    await sheet.getByRole('button', { name: 'Voltar' }).click();
    await m.keyboard.press('Escape');
    await expect(m.getByRole('dialog', { name: 'Porta' })).toHaveCount(0);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await Promise.all([master.close(), pensantus.close(), toren.close()]);
  }

  // The "Mover" page after a locked door stopped the move: the notice with the lock, and the map still "Porta fechada".
  const mover = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const moverPlayer = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const [mm, pp] = [await mover.newPage(), await moverPlayer.newPage()];
  let moveCampaign = '';
  try {
    await Promise.all([mm.goto('/'), pp.goto('/')]);
    const table = await tableForCombat(mm, pp, `Acessibilidade porta trancada ${Date.now()}`, true, true, { sheet: pensantusCasting });
    moveCampaign = table.campaignId;
    await paintRPC(mm, table, 'MAP_LAYER_WALL', 1, [[3, 5], [4, 5], [6, 5], [7, 5]]);
    const res = await callRPC(mm, 'meurpg.maps.v1.MapService/PaintMapCells', { campaignId: table.campaignId, mapId: table.mapId, layer: 'MAP_LAYER_DOORS', value: 3, squares: [{ col: 5, row: 5 }] });
    expect(res.ok(), await res.text()).toBeTruthy();
    await beginAttackCombatRPC(mm, table, { Pensantus: 20, 'Goblin 1': 15, 'Capitão Goblin': 10, 'Goblin 2': 4 }, { 'Capitão Goblin': [11, 9], 'Goblin 1': [14, 7], 'Goblin 2': [15, 11] });
    await openSessionPage(pp, moveCampaign);
    await expect(pp.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
    await pp.getByRole('button', { name: 'Mover', exact: true }).click();
    await expect(pp.getByRole('heading', { name: 'Mover Pensantus' })).toBeVisible();
    await tapSquare(pp, 5, 4);
    await pp.getByRole('button', { name: 'Mover para cá' }).click();
    await expect(pp.getByRole('status').filter({ hasText: 'A porta está trancada.' })).toBeVisible();
    await expectScreenPasses(pp, `Mover, uma porta trancada parou o movimento ${where}`);
  } finally {
    if (moveCampaign) {
      await endOpenSessionRPC(mm, moveCampaign);
    }
    await Promise.all([mover.close(), moverPlayer.close()]);
  }
}

test('as portas passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-010', '@RN-26'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanDoorScreens(browser, 'light', 1280);
});

test('as portas passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-010', '@RN-26'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanDoorScreens(browser, 'dark', 390);
});

test('as portas passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-010', '@RN-26'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanDoorScreens(browser, 'dark', 1024);
});

test('as portas passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-010', '@RN-26'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanDoorScreens(browser, 'light', 320);
});

/**
 * The bestiary (MR-042, E10-08): the list, loading and failing, a search with results, one with none, a
 * filter set, the Ogre's stat block, the "Criar NPC" dialog (a sheet on a phone), its empty-name error, the
 * confirmation after the NPC is made, the player's notice and the campaign page with its "Bestiário" panel.
 */
async function scanBestiaryScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const playerContext = await browser.newContext({
    storageState: authStatePath('Jogador Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const page = await context.newPage();
  const player = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const listed = (p: Page, count: string) => expect(p.locator('.list__n')).toHaveText(count);
  try {
    await page.goto('/');
    await player.goto('/');
    const { campaignId } = await tableForMaps(page, player, `Acessibilidade bestiário ${Date.now()}`);

    // Loading: the answer is held until the scan is done.
    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/meurpg.rules.v1.ContentService/ListCreatures', async (route) => {
      await held;
      await route.continue();
    });
    await page.goto(`/campaigns/${campaignId}/bestiary`);
    await expect(page.getByText('Buscando as criaturas...')).toBeVisible();
    await expectScreenPasses(page, `Bestiário, carregando ${where}`);
    release();
    await listed(page, '334 de 334 criaturas');
    await page.unroute('**/meurpg.rules.v1.ContentService/ListCreatures');
    await expectScreenPasses(page, `Bestiário, a lista ${where}`);

    // Failing: the server does not answer.
    await page.route('**/meurpg.rules.v1.ContentService/ListCreatures', (route) =>
      route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) }),
    );
    await page.getByRole('searchbox', { name: 'Nome' }).fill('lobo');
    await expect(page.getByRole('alert')).toContainText('o servidor não respondeu');
    await expectScreenPasses(page, `Bestiário, erro ${where}`);
    await page.unroute('**/meurpg.rules.v1.ContentService/ListCreatures');
    await page.getByRole('button', { name: 'Tentar de novo' }).click();
    await listed(page, '5 de 334 criaturas');
    await expectScreenPasses(page, `Bestiário, busca "lobo" ${where}`);

    await page.getByRole('searchbox', { name: 'Nome' }).fill('xyzzy');
    await expect(page.getByText('Nenhuma criatura com “xyzzy”.')).toBeVisible();
    await expectScreenPasses(page, `Bestiário, busca sem resultado ${where}`);

    await page.getByRole('button', { name: 'Limpar a busca' }).click();
    await page.locator('select[name=type]').selectOption('dragon');
    await page.locator('select[name=size]').selectOption('huge');
    await page.locator('select[name=cr]').selectOption('11-30');
    await expect(page.getByRole('button', { name: 'Limpar filtros' }).first()).toBeVisible();
    await expectScreenPasses(page, `Bestiário, filtros ligados ${where}`);

    await page.goto(`/campaigns/${campaignId}/bestiary/ogre`);
    await expect(page.getByText('Os textos abaixo são do livro de regras (SRD 5.1), em inglês.')).toBeVisible();
    await expectScreenPasses(page, `Bestiário, a ficha do Ogro ${where}`);

    await page.getByRole('button', { name: 'Criar NPC' }).click();
    const dialog = page.getByRole('dialog', { name: 'Criar NPC' });
    await expect(dialog.getByLabel('Nome do NPC')).toBeFocused();
    await expectScreenPasses(page, `Criar NPC ${where}`);

    await dialog.getByLabel('Nome do NPC').fill('');
    await dialog.getByRole('button', { name: 'Criar NPC' }).click();
    await expect(dialog.getByText('Dê um nome ao NPC.')).toBeVisible();
    await expect(dialog.getByLabel('Nome do NPC')).toBeFocused();
    await expectScreenPasses(page, `Criar NPC, nome vazio ${where}`);

    await dialog.getByLabel('Nome do NPC').fill('Capitão bandido');
    await dialog.getByRole('button', { name: 'Criar NPC' }).click();
    await expect(page.locator('.made')).toContainText('NPC criado: Capitão bandido.');
    await expectScreenPasses(page, `Criar NPC, a confirmação ${where}`);

    await page.goto(`/campaigns/${campaignId}/prep`);
    const panel = page.getByRole('link', { name: /^Bestiário/ });
    await panel.scrollIntoViewIfNeeded();
    await expect(panel).toBeVisible();
    await expectScreenPasses(page, `Campanha com o painel Bestiário ${where}`);

    // A player: no panel on the campaign page, and a calm notice on the page itself.
    await player.goto(`/campaigns/${campaignId}/bestiary`);
    await expect(player.getByText('Só o mestre usa o bestiário da campanha.')).toBeVisible();
    await expectScreenPasses(player, `Bestiário, o aviso do jogador ${where}`);
  } finally {
    await context.close();
    await playerContext.close();
  }
}

test('o bestiário passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-042'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanBestiaryScreens(browser, 'light', 1280);
});

test('o bestiário passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-042'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanBestiaryScreens(browser, 'dark', 390);
});

test('o bestiário passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-042'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanBestiaryScreens(browser, 'dark', 1024);
});

test('o bestiário passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-042'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanBestiaryScreens(browser, 'light', 320);
});

/**
 * "Regras da mesa" (MR-025, RN-24, RN-09; E10-03 states 1 to 3): the page as saved, a style chosen (the notice and
 * the "Mudou" tags, the save bar lit), the XP mode question open in place, and the rules for a table with a long
 * list of reminders. The XP question needs XP already given, so the campaign has Pensantus and an award.
 */
async function scanTableRules(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(120_000);
  const mContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height: 900 } });
  const pContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const m = await mContext.newPage();
    const p = await pContext.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const table = await tableForXp(m, p, `Acessibilidade regras ${Date.now()}`);
    await awardXpRPC(m, table.campaignId, { mode: 'MANUAL', reason: 'A porta da torre', characterIds: [table.characterId], amount: 50 });
    await setTableRulesRPC(m, table.campaignId, { houseRules: ['Beber uma poção é uma ação bônus', 'Quem cai fica caído até o fim do turno'] });
    const where = `(${colorScheme}, ${width}px)`;

    await m.goto(`/campaigns/${table.campaignId}/setup`);
    await expect(m.getByRole('heading', { level: 1 })).toBeVisible();
    await expectLoaded(m);
    await expectScreenPasses(m, `Campanha com o painel Regras da mesa ${where}`);

    await open(m, `/campaigns/${table.campaignId}/rules`);
    await expectScreenPasses(m, `Regras da mesa ${where}`);
    await pickRadio(m, /Mesa física/);
    await expect(m.getByText(/o estilo preencheu/)).toBeVisible();
    await expectScreenPasses(m, `Regras da mesa, um estilo escolhido ${where}`);
    await pickRadio(m, /Por marcos/);
    await m.getByRole('button', { name: 'Mudar para marcos' }).click();
    await expect(m.getByRole('heading', { name: 'Mudar para “por marcos”?' })).toBeVisible();
    await expectScreenPasses(m, `Regras da mesa, mudar o modo de XP ${where}`);

    // A player is told it is the master's page.
    await open(p, `/campaigns/${table.campaignId}/rules`);
    await expect(p.getByText('Só o mestre muda as regras da mesa.')).toBeVisible();
    await expectScreenPasses(p, `Regras da mesa, visto por um jogador ${where}`);
  } finally {
    await Promise.all([mContext.close(), pContext.close()]);
  }
}

test('as regras da mesa passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-24', '@RN-09'] }, async ({ browser }) => {
  await scanTableRules(browser, 'light', 1280);
});

test('as regras da mesa passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-24', '@RN-09'] }, async ({ browser }) => {
  await scanTableRules(browser, 'dark', 390);
});

test('as regras da mesa passam no axe e nas conferências de layout no tema escuro, no celular de 320', { tag: ['@a11y', '@RN-24'] }, async ({ browser }) => {
  await scanTableRules(browser, 'dark', 320);
});

/**
 * The "Habilidades" step of a player who makes a new sheet by the table's rules (E10-03 state 4): the four ways, each
 * with what it shows (the placing of the standard array, the point buy with the points left, the 4d6 the server
 * rolled and the physical dice to type, and the typed values).
 */
async function scanTableAbilities(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(180_000);
  const mContext = await newSignedInContext(browser, 'Mestre Teste');
  const pContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height: 900 } });
  try {
    const m = await mContext.newPage();
    const p = await pContext.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Acessibilidade habilidades ${Date.now()}`);
    const where = `(${colorScheme}, ${width}px)`;
    await open(p, `/campaigns/${campaignId}/characters/new`);
    await p.getByLabel('Nome do personagem', { exact: true }).fill('Ícaro');
    await p.getByRole('tab', { name: 'Habilidades' }).click();
    await expect(p.getByRole('radio', { name: 'Padrão' })).toBeChecked();
    await expectScreenPasses(p, `Habilidades, conjunto padrão ${where}`);

    await method(p, 'Pontos');
    for (let i = 0; i < 7; i++) {
      await p.getByRole('button', { name: 'Aumentar Sabedoria', exact: true }).click();
    }
    await expect(p.getByText('Restam 18 pontos')).toBeVisible();
    await expectScreenPasses(p, `Habilidades, compra por pontos ${where}`);

    await method(p, '4d6');
    await expectScreenPasses(p, `Habilidades, 4d6 ainda sem rolar ${where}`);
    await p.getByRole('button', { name: 'Rolar as habilidades' }).click();
    await expect(p.getByText(/Rolados em/)).toBeVisible();
    await expectScreenPasses(p, `Habilidades, 4d6 rolados pelo servidor ${where}`);

    await method(p, 'Digitar');
    await p.locator('input').and(p.getByLabel('Força', { exact: true })).fill('19');
    await expectScreenPasses(p, `Habilidades, digitar com um valor fora do limite ${where}`);

    // Physical dice: a second campaign where everybody rolls their own.
    const physical = await campaignWithEmptyPlayer(m, p, `Acessibilidade dados ${Date.now()}`);
    await setTableRulesRPC(m, physical, { diceMode: 'DICE_MODE_PHYSICAL' });
    await open(p, `/campaigns/${physical}/characters/new`);
    await p.getByRole('tab', { name: 'Habilidades' }).click();
    await method(p, '4d6');
    await expect(p.getByText('Digite os quatro dados de cada rolagem.')).toBeVisible();
    await expectScreenPasses(p, `Habilidades, dados físicos a digitar ${where}`);
  } finally {
    await Promise.all([mContext.close(), pContext.close()]);
  }
}

test('as habilidades por jeito passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-24'] }, async ({ browser }) => {
  await scanTableAbilities(browser, 'light', 1280);
});

test('as habilidades por jeito passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-24'] }, async ({ browser }) => {
  await scanTableAbilities(browser, 'dark', 390);
});

test('as habilidades por jeito passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@RN-24'] }, async ({ browser }) => {
  await scanTableAbilities(browser, 'light', 320);
});

/** The grid calibration (E10-03 state 5): the panel of a calibrated map, the question with "Outro", and "Mudar a grade?". */
async function scanCalibration(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(180_000);
  const context = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height: 900 } });
  try {
    const page = await context.newPage();
    await page.goto('/');
    const campaignId = await masterCampaign(page, `Acessibilidade calibração ${Date.now()}`);
    const map = await mapToPaint(page, campaignId, 'A torre em ruínas', 12);
    const painted = await callRPC(page, 'meurpg.maps.v1.MapService/PaintMapCells', { campaignId, mapId: map.mapId, layer: 'MAP_LAYER_WALL', value: 1, squares: wallSquares() });
    expect(painted.ok()).toBeTruthy();
    const where = `(${colorScheme}, ${width}px)`;
    await page.goto(editorRoute(campaignId, map.mapId));
    await expect(page.getByRole('radio', { name: 'Pintar' })).toBeVisible();
    await page.getByRole('radio', { name: 'Pintar' }).click();
    const grid = page.getByRole('region', { name: 'Grade' });
    await grid.getByRole('button', { name: 'Calibrar o quadrado' }).click();
    await expect(page.getByRole('heading', { name: 'Cada quadrado deste desenho vale' })).toBeFocused();
    await expectScreenPasses(page, `Calibração da grade, a pergunta ${where}`);
    await pickRadio(page.locator('app-calibrate-ask'), /Outro/);
    await page.getByLabel('Quanto vale o quadrado', { exact: true }).fill('4,5');
    await expect(page.getByText('o mapa terá 36 × 24 quadrados.')).toBeVisible();
    await expectScreenPasses(page, `Calibração da grade, Outro ${where}`);
    await factor(page, '3 m');
    await page.getByRole('button', { name: 'Salvar a grade' }).click();
    await expect(grid.getByText('cada um vale 3 m')).toBeVisible();
    await expectScreenPasses(page, `Calibração da grade, o mapa calibrado ${where}`);
    await grid.getByRole('button', { name: 'Calibrar o quadrado' }).click();
    await factor(page, '4,5 m');
    await page.getByRole('button', { name: 'Salvar a grade' }).click();
    await expect(page.getByRole('heading', { name: 'Mudar a grade?' })).toBeVisible();
    await expectScreenPasses(page, `Calibração da grade, Mudar a grade? ${where}`);
  } finally {
    await context.close();
  }
}

test('a calibração da grade passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-25'] }, async ({ browser }) => {
  await scanCalibration(browser, 'light', 1280);
});

test('a calibração da grade passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@RN-25'] }, async ({ browser }) => {
  await scanCalibration(browser, 'dark', 1024);
});

test('a calibração da grade passa no axe e nas conferências de layout no tema claro, no tablet de 768', { tag: ['@a11y', '@RN-25'] }, async ({ browser }) => {
  await scanCalibration(browser, 'light', 768);
});

test('a página Créditos, com a atribuição do SRD 5.2.1, passa no axe nos dois temas', { tag: ['@a11y', '@licenca'] }, async ({ browser }) => {
  for (const [scheme, width] of [['light', 1280], ['dark', 390]] as const) {
    const context = await browser.newContext({ colorScheme: scheme, viewport: { width, height: 900 } });
    try {
      const page = await context.newPage();
      await open(page, '/credits');
      await expect(page.getByText('System Reference Document 5.2.1', { exact: false }).first()).toBeVisible();
      await expectScreenPasses(page, `Créditos (${scheme}, ${width}px)`);
    } finally {
      await context.close();
    }
  }
});

// The dungeon generator (slice 10.14b, MR-010, RN-26, RN-10; E10-05 1 to 6): the "Gerar masmorra" page (the options and the server's preview,
// a refused size, the preview loading and failing, a map being created), the generated map with its rooms list (and a room chosen),
// "Redesenhar" asked in place, and the "Imagem" panel in "Pintar". On a phone the page only says it is for a computer, and the master reads
// the rooms list under the map. Every state goes through axe and the alignment checks.
async function scanDungeonScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const context = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const m = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const phone = width < 768;
  try {
    await m.goto('/');
    const created = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Acessibilidade masmorra ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
    expect(created.ok()).toBeTruthy();
    const campaignId = (await created.json()).campaign.id as string;
    const made = await callRPC(m, 'meurpg.maps.v1.DungeonService/CreateDungeonMap', { campaignId, name: 'A masmorra do teste', seed: '48213', options: { width: 31, height: 21, roomSideMin: 3, roomSideMax: 9 } });
    expect(made.ok(), await made.text()).toBeTruthy();
    const mapId = (await made.json()).map.id as string;
    const route = `/campaigns/${campaignId}/maps/dungeon`;
    const preview = m.getByRole('img', { name: /^Prévia da masmorra/ });

    await open(m, `/campaigns/${campaignId}/prep`);
    await expect(m.getByRole('link', { name: 'Gerar masmorra' })).toBeVisible();
    await expectScreenPasses(m, `Campanha, "Gerar masmorra" ao lado de "Novo mapa" ${where}`);

    if (phone) {
      await open(m, route);
      await expect(m.getByText('Gerar masmorra é no notebook.')).toBeVisible();
      await expectScreenPasses(m, `Gerar masmorra no celular ${where}`);
    } else {
      await open(m, route);
      await expect(preview).toBeVisible();
      await expectScreenPasses(m, `Gerar masmorra, as opções e a prévia ${where}`);
      await m.getByRole('radio', { name: 'Outro' }).click();
      await m.getByRole('textbox', { name: 'Quadrados no lado maior' }).fill('130');
      await expect(m.getByText('Corrija o tamanho para ver a masmorra.')).toBeVisible();
      await expectScreenPasses(m, `Gerar masmorra, um tamanho recusado ${where}`);

      // The preview loading, and failing.
      await m.route('**/*DungeonService/PreviewDungeon', (r) => r.fulfill({ status: 503, contentType: 'application/json', body: '{"code":"unavailable","message":"x"}' }));
      await m.goto(route);
      await expect(m.getByRole('button', { name: 'Tentar de novo' })).toBeVisible();
      await expectScreenPasses(m, `Gerar masmorra, a prévia com falha ${where}`);
      await m.unroute('**/*DungeonService/PreviewDungeon');
      await m.route('**/*DungeonService/PreviewDungeon', () => new Promise(() => undefined));
      await m.goto(route);
      await expect(m.getByRole('status').filter({ hasText: 'Carregando a prévia...' })).toBeVisible();
      await expectScreenPasses(m, `Gerar masmorra, carregando a prévia ${where}`);
      await m.unroute('**/*DungeonService/PreviewDungeon');

      // Creating: the server does not answer, so the steps stay on screen.
      await m.route('**/*DungeonService/CreateDungeonMap', () => new Promise(() => undefined));
      await m.goto(route);
      await expect(preview).toBeVisible();
      await m.getByRole('button', { name: 'Criar o mapa' }).click();
      await expect(m.getByText('Criando o mapa. Costuma levar poucos segundos.')).toBeVisible();
      await expectScreenPasses(m, `Gerar masmorra, criando o mapa ${where}`);
      await m.unroute('**/*DungeonService/CreateDungeonMap');
    }

    // The generated map: on a computer the editor with the rooms list; on a phone the list under the map.
    await open(m, `/campaigns/${campaignId}/maps/${mapId}`);
    await expect(m.getByRole('heading', { name: 'Salas', exact: true })).toBeVisible();
    await expectScreenPasses(m, `Mapa gerado, a lista das salas ${where}`);
    await m.locator('app-dungeon-rooms .room__head').nth(1).click();
    await expect(m.locator('app-dungeon-rooms .room--on')).toHaveCount(1);
    await expectScreenPasses(m, `Mapa gerado, uma sala escolhida ${where}`);
    if (!phone) {
      await m.getByRole('region', { name: 'Imagem' }).getByRole('button', { name: 'Redesenhar' }).click();
      await expect(m.getByRole('heading', { name: 'Redesenhar a imagem?' })).toBeVisible();
      await expectScreenPasses(m, `Mapa gerado, "Redesenhar" perguntado ${where}`);
      await m.getByRole('button', { name: 'Voltar' }).click();
      await m.getByRole('radio', { name: 'Pintar' }).click();
      await expect(m.getByRole('region', { name: 'Imagem' })).toBeVisible();
      await expect(m.locator('app-layers-panel').getByText('Tudo salvo').first()).toBeVisible();
      await expectScreenPasses(m, `Mapa gerado, "Pintar" com o painel Imagem ${where}`);
    }
  } finally {
    await context.close();
  }
}

for (const [scheme, width, label] of [
  ['light', 1280, 'tema claro, no desktop'],
  ['dark', 390, 'tema escuro, no celular'],
  ['dark', 1024, 'tema escuro, no desktop de 1024'],
  ['light', 320, 'tema claro, no celular de 320'],
] as const) {
  test(`o gerador de masmorras passa no axe e nas conferências de layout no ${label}`, { tag: ['@a11y', '@MR-010'] }, async ({ browser }) => {
    test.setTimeout(600_000);
    await scanDungeonScreens(browser, scheme, width);
  });
}


/**
 * Puzzles (Etapa 10, slice 10.15a: MR-038, RN-27, RN-10; E10-06 states 1 to 9): the master's list and the three forms, his live view of
 * each kind (the lock's solution asked for, "Recomeçar" and "Fechar" asked in place, a solved puzzle with its door) and the player's
 * notice and boards (the lights, the lock, the pillars, solved, a 7 × 7 board). `masterToo` is false on the narrowest phone, where only the
 * player's boards are the screens of the artboards (state 9).
 */
async function scanPuzzleScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number, masterToo = true): Promise<void> {
  // Not `open()`: a campaign with a session open keeps a stream going (the XP watcher's), so the network is never idle.
  const openPage = async (page: Page, route: string) => {
    await page.goto(route);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  };
  const stepPasses = async (page: Page, screen: string) => {
    await expectScreenPasses(page, screen);
  };
  const height = width < 400 ? 640 : 900;
  const masterContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height } });
  const playerContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const master = await masterContext.newPage();
  const player = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForPuzzles(master, player, `Acessibilidade quebra-cabeças ${Date.now()}`);
    campaignId = table.campaignId;
    const lights = await createLightsRPC(master, campaignId, 'O selo da Capela', 5, { clue: 'Só o selo apagado abre o caminho.', hints: ['A luz do selo responde ao toque.', 'Cada toque troca cinco luzes de uma vez.'] });
    const lock = await createLockRPC(master, campaignId, 'O cofre do Refeitório', {
      clue: 'O fogo nasce antes da lua, e a raiz vê tudo.',
      hints: ['A pista fala de três coisas da natureza.'],
      onSolve: { action: 'PUZZLE_SOLVE_ACTION_OPEN_DOOR', message: 'A porta da Capela se abriu.', door: { mapId: table.map.mapId, col: table.door.col, row: table.door.row } },
    });
    const pillars = await createPillarsRPC(master, campaignId, 'Os pilares da Galeria', { clue: 'Os pilares obedecem ao mural.' });
    const big = await createLightsRPC(master, campaignId, 'Os candelabros da cripta', 7, { clue: 'Os candelabros guardam a cripta.' });
    // A puzzle a limit stops after one move, one the master will close under the player, and pillars the table solves.
    const stopped = await createLightsRPC(master, campaignId, 'A sala dos espelhos', 3, { onWrong: { maxMoves: 1 } });
    const closing = await createLightsRPC(master, campaignId, 'O salão fechado', 3);
    const donePillars = await createPillarsRPC(master, campaignId, 'Os pilares resolvidos', { clue: 'Os pilares obedecem ao mural.' });

    if (masterToo) {
      // The list with no puzzle at all (state 1, empty): another campaign of the master's.
      const empty = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Sem quebra-cabeças ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
      expect(empty.ok()).toBeTruthy();
      await openPage(master, `${puzzleRoute(((await empty.json()).campaign as { id: string }).id)}/prep`);
      await expect(master.getByText('Nenhum quebra-cabeça ainda.')).toBeVisible();
      await stepPasses(master, `Lista de quebra-cabeças vazia ${where}`);

      // The list (state 1) and the three forms (state 2), the edit form.
      await openPage(master, `${puzzleRoute(campaignId)}/prep`);
      await expect(master.getByRole('region', { name: 'Quebra-cabeças' }).getByText('O selo da Capela')).toBeVisible();
      await stepPasses(master, `Lista de quebra-cabeças ${where}`);
      await pickPuzzleAction(master, 'O selo da Capela', 'Arquivar');
      await expect(master.getByRole('heading', { name: 'Arquivar “O selo da Capela”?' })).toBeFocused();
      await stepPasses(master, `Arquivar na lista ${where}`);
      await master.getByRole('button', { name: 'Voltar' }).click();

      await openPage(master, puzzleRoute(campaignId, 'puzzles', 'new'));
      await expect(master.getByText(/\d+ acesas?, \d+ apagadas?\./)).toBeVisible({ timeout: 30_000 });
      await stepPasses(master, `Novo quebra-cabeça: as luzes ${where}`);
      await master.getByRole('radio', { name: /^Fechadura de combinação/ }).check();
      await master.getByRole('radio', { name: 'Abrir uma porta' }).check();
      await expect(master.getByLabel('Mapa', { exact: true })).toBeVisible();
      await stepPasses(master, `Novo quebra-cabeça: a fechadura, abrir uma porta ${where}`);
      await master.getByRole('radio', { name: /^Símbolos giratórios/ }).check();
      await expect(master.getByText(/Dá para resolver em \d+ giros?, no mínimo\./)).toBeVisible({ timeout: 30_000 });
      await stepPasses(master, `Novo quebra-cabeça: os símbolos giratórios ${where}`);
      await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
      await expect(master.getByText('Dê um nome ao quebra-cabeça.')).toBeVisible();
      await stepPasses(master, `Formulário com erro ${where}`);
      await openPage(master, puzzleRoute(campaignId, 'puzzles', lock, 'edit'));
      await expect(master.getByLabel('Nome')).toHaveValue('O cofre do Refeitório');
      await stepPasses(master, `Editar a fechadura ${where}`);

      // The live view (states 3 to 5): each kind, the solution asked for, the two questions.
      await showPuzzleRPC(master, campaignId, lights);
      await showPuzzleRPC(master, campaignId, lock);
      await showPuzzleRPC(master, campaignId, pillars);
      await openSessionPage(master, campaignId);
      const panel = master.getByRole('region', { name: 'Quebra-cabeças' });
      await panel.getByRole('button', { name: 'Ver ao vivo O selo da Capela' }).click();
      await expect(master.getByRole('article', { name: 'O selo da Capela' })).toBeVisible();
      await stepPasses(master, `Sessão do mestre com quebra-cabeças ${where}`);
      // One card at a time, in the main column: "Ver ao vivo" on the row chooses it.
      await panel.getByRole('button', { name: 'Ver ao vivo O cofre do Refeitório' }).click();
      const lockCard = master.getByRole('article', { name: 'O cofre do Refeitório' });
      await lockCard.getByRole('button', { name: 'Mostrar a solução só para mim' }).click();
      await expect(lockCard.locator('.part__label', { hasText: 'A solução' })).toBeVisible();
      await lockCard.scrollIntoViewIfNeeded();
      await stepPasses(master, `Fechadura ao vivo, com a solução ${where}`);
      await panel.getByRole('button', { name: 'Ver ao vivo O selo da Capela' }).click();
      const lightsCard = master.getByRole('article', { name: 'O selo da Capela' });
      await lightsCard.getByRole('button', { name: 'Recomeçar' }).click();
      await expect(lightsCard.getByRole('heading', { name: 'Recomeçar “O selo da Capela”?' })).toBeFocused();
      await stepPasses(master, `Recomeçar perguntado na tela ${where}`);
      await lightsCard.getByRole('button', { name: 'Voltar' }).click();
      await lightsCard.getByRole('button', { name: 'Fechar' }).click();
      await expect(lightsCard.getByRole('heading', { name: 'Fechar “O selo da Capela”?' })).toBeFocused();
      await stepPasses(master, `Fechar perguntado na tela ${where}`);
      await lightsCard.getByRole('button', { name: 'Voltar' }).click();
      void panel;
    } else {
      await showPuzzleRPC(master, campaignId, lights);
      await showPuzzleRPC(master, campaignId, lock);
      await showPuzzleRPC(master, campaignId, pillars);
    }
    await showPuzzleRPC(master, campaignId, big);
    await showPuzzleRPC(master, campaignId, stopped);
    await showPuzzleRPC(master, campaignId, closing);
    await showPuzzleRPC(master, campaignId, donePillars);

    // The player: the notice on the session page (state 6), then each board.
    await openSessionPage(player, campaignId);
    await expect(player.getByText('O mestre mostrou um quebra-cabeça').first()).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `Aviso do quebra-cabeça mostrado ${where}`);
    const playTo = async (id: string, name: string) => {
      await player.goto(`/campaigns/${campaignId}/session?puzzle=${id}`);
      await expect(player.getByRole('heading', { level: 1, name })).toBeVisible({ timeout: 30_000 });
    };
    await playTo(lights, 'O selo da Capela');
    await expect(player.getByRole('button', { name: /^Luz na linha 1, coluna 1/ })).toBeVisible();
    await stepPasses(player, `As luzes (5 × 5) ${where}`);
    await playTo(big, 'Os candelabros da cripta');
    await expect(player.getByRole('button', { name: /^Luz na linha 7, coluna 7/ })).toBeVisible();
    await stepPasses(player, `As luzes (7 × 7) ${where}`);
    await playTo(pillars, 'Os pilares da Galeria');
    await expect(player.getByRole('group', { name: 'O mural' })).toBeVisible();
    await moveRPC(player, campaignId, pillars, { pillars: { pillar: 0, delta: 1 } });
    await expect(player.getByText('Os pilares 1 e 2 mudaram.')).toBeVisible();
    await stepPasses(player, `Os pilares ${where}`);
    await playTo(lock, 'O cofre do Refeitório');
    await expect(player.getByRole('group', { name: 'Roda 1: Lua' })).toBeVisible();
    await stepPasses(player, `A fechadura ${where}`);

    // A limit stopped it: the neutral line, the board frozen.
    await playTo(stopped, 'A sala dos espelhos');
    await moveRPC(player, campaignId, stopped, { lights: { row: 0, col: 0 } });
    await expect(player.getByText('O quebra-cabeça parou.')).toBeVisible();
    await stepPasses(player, `Parou por um limite ${where}`);
    // The master closed it while the player was reading.
    await playTo(closing, 'O salão fechado');
    await closePuzzleRPC(master, campaignId, closing);
    await expect(player.getByText('O mestre fechou o quebra-cabeça.')).toBeVisible();
    await stepPasses(player, `Fechado pelo mestre ${where}`);
    // The pillars, solved by the table.
    await solveByThePathRPC(master, player, campaignId, donePillars);
    await playTo(donePillars, 'Os pilares resolvidos');
    await expect(player.getByText('Resolvido', { exact: true })).toBeVisible();
    await stepPasses(player, `Os pilares resolvidos ${where}`);

    // Solved: the door opens, the page says what the master wrote.
    await moveRPC(player, campaignId, lock, { lock: { wheel: 0, delta: 1 } });
    await moveRPC(player, campaignId, lock, { lock: { wheel: 3, delta: -1 } });
    await expect(player.getByText('Resolvido', { exact: true })).toBeVisible();
    await stepPasses(player, `Resolvido ${where}`);
    if (masterToo) {
      await openSessionPage(master, campaignId);
      await master.getByRole('button', { name: 'Ver ao vivo Os pilares resolvidos' }).click();
      const pillarsCard = master.getByRole('article', { name: 'Os pilares resolvidos' });
      await expect(pillarsCard.getByText('Resolvido').first()).toBeVisible();
      await pillarsCard.scrollIntoViewIfNeeded();
      await stepPasses(master, `Pilares resolvidos, na visão do mestre ${where}`);
      await master.getByRole('button', { name: 'Ver ao vivo O cofre do Refeitório' }).click();
      const lockCard = master.getByRole('article', { name: 'O cofre do Refeitório' });
      await expect(lockCard.getByText('Uma porta se abriu.')).toBeVisible();
      await expect(lockCard.getByRole('img', { name: /A porta aberta no mapa/ })).toBeVisible();
      await lockCard.scrollIntoViewIfNeeded();
      await stepPasses(master, `Resolvido, com a porta aberta ${where}`);
    }
  } finally {
    if (campaignId) {
      await endTable(master, campaignId);
    }
    await masterContext.close();
    await playerContext.close();
  }
}

test('os quebra-cabeças passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanPuzzleScreens(browser, 'light', 1280);
});

test('os quebra-cabeças passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanPuzzleScreens(browser, 'dark', 390);
});

test('os quebra-cabeças passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanPuzzleScreens(browser, 'light', 320, false);
});

test('os quebra-cabeças passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanPuzzleScreens(browser, 'dark', 1024);
});

/**
 * More puzzles (Etapa 10, slice 10.15b: MR-038, RN-27, RN-10, RN-18; E10-12 states 1 to 10): the master's forms for the riddle, the
 * sequence and the cipher (with the skill check, the split information and "Ao errar"), his live view of each, and the player's phone in
 * every state: the riddle (a wrong answer, no attempts left, solved, stopped), the sequence (not played, playing, repeating, a wrong
 * step with the trap), the cipher (the key not found, found, a wrong message) and a hint by a skill check (the button, a typed d20, a
 * fail, a pass) with the split information. `masterToo` is false on the narrowest phone, where only the player's phone is a screen.
 */
async function scanMorePuzzleScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number, masterToo = true): Promise<void> {
  const openPage = async (page: Page, route: string) => {
    await page.goto(route);
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
  };
  const stepPasses = async (page: Page, screen: string) => {
    await expectScreenPasses(page, screen);
  };
  // The phone states are scanned at the phones' real heights: 568 px at 320, 667 at 375.
  const height = width === 320 ? 568 : width < 400 ? (width === 375 ? 667 : 640) : 900;
  const masterContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height } });
  const playerContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const master = await masterContext.newPage();
  const player = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  let toren: Awaited<ReturnType<typeof puzzlesSecondPlayer>> | undefined;
  try {
    await Promise.all([master.goto('/'), player.goto('/')]);
    const table = await tableForPuzzles(master, player, `Acessibilidade mais quebra-cabeças ${Date.now()}`);
    campaignId = table.campaignId;
    toren = await puzzlesSecondPlayer(browser, master, campaignId);
    await setCurrentMapRPC(master, campaignId, table.map.mapId);
    const trap = await trapPointRPC(master, campaignId, table.map.mapId, 'Dardos envenenados');
    const clueId = await sceneClueRPC(master, campaignId, table.map.mapId, 'A biblioteca', 'Cada letra anda três para trás.');

    const riddle = await createRiddleRPC(master, campaignId, 'A porta da Cripta pergunta', {
      clue: 'Procure no chão da Cripta.',
      hints: ['Pense no que acompanha você ao meio-dia.', 'Ela some quando a tocha apaga.'],
      hintCheck: { skillKey: 'skill:investigation', dc: 13 },
      parts: [
        { characterId: table.characterId, text: '…os tambores ecoam três vezes antes de a porta ceder.' },
        { characterId: toren.characterId, text: 'A porta ouve o que o chão esconde…' },
      ],
      onWrong: { attemptsPerPlayer: 4 },
    });
    const sequence = await createSequenceRPC(master, campaignId, 'Os sinos do Salão do trono', { clue: 'Quem toca os sinos escuta o trono.', onWrong: { trap: { mapId: table.map.mapId, pointId: trap } } });
    const cipher = await createCipherRPC(master, campaignId, 'A carta do Capitão', { onWrong: { maxMoves: 10, timeLimitSeconds: 3600 } }, clueId);
    const stopped = await createRiddleRPC(master, campaignId, 'A porta selada', { onWrong: { maxMoves: 1 } });
    const done = await createRiddleRPC(master, campaignId, 'O enigma resolvido');

    // Shown from the start: the master's live cards and the players' screens read the same three, with one wrong answer already made.
    for (const id of [riddle, sequence, cipher]) {
      await showPuzzleRPC(master, campaignId, id);
    }
    await moveRPC(player, campaignId, riddle, { riddle: { answer: 'escuridão' } });

    if (masterToo) {
      // The three forms (states 1 to 3), with "Ao errar", the skill check and the split information.
      await openPage(master, puzzleRoute(campaignId, 'puzzles', 'new'));
      await master.getByRole('radio', { name: /^Enigma/ }).check();
      await stepPasses(master, `Novo quebra-cabeça: o enigma, vazio ${where}`);
      await master.getByLabel('Nome').fill('A porta da Cripta pergunta');
      await master.getByLabel('O enigma').fill('Moro embaixo de cada passo seu, mas nunca peso nada. O que sou?');
      for (const answer of ['sombra', 'a sombra']) {
        await master.getByLabel(/^(Uma|Outra) resposta$/).fill(answer);
        await master.getByRole('button', { name: 'Adicionar', exact: true }).click();
      }
      await master.getByRole('radio', { name: 'Gastar uma tentativa do jogador' }).check();
      await master.getByRole('button', { name: 'Adicionar uma dica' }).click();
      await master.getByRole('textbox', { name: 'Dica 1' }).fill('Pense no que acompanha você ao meio-dia.');
      await master.getByLabel('Perícia', { exact: true }).selectOption('skill:investigation');
      await master.getByLabel('CD', { exact: true }).fill('13');
      await master.getByRole('button', { name: 'Adicionar parte' }).click();
      await master.getByLabel('Para quem (parte 1)').selectOption(table.characterId);
      await master.getByLabel('O que o jogador lê (parte 1)').fill('…os tambores ecoam três vezes antes de a porta ceder.');
      await stepPasses(master, `Novo quebra-cabeça: o enigma, com perícia, parte e tentativas ${where}`);
      await master.getByRole('radio', { name: 'Disparar uma armadilha do mapa' }).check();
      await expect(master.getByLabel('Armadilha do mapa', { exact: true })).toBeVisible();
      await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
      await expect(master.getByText('Escolha a armadilha do mapa que dispara.')).toBeVisible();
      await stepPasses(master, `O enigma com a armadilha por escolher ${where}`);
      await master.getByRole('radio', { name: 'Limite de jogadas ou de tempo' }).check();
      await stepPasses(master, `O enigma, com limite de jogadas ou de tempo ${where}`);

      await master.getByRole('radio', { name: /^Sequência/ }).check();
      for (const bell of ['Sino redondo', 'Sino alto', 'Sino pequeno', 'Sino redondo']) {
        await master.locator('app-bells-board').getByRole('button', { name: bell }).click();
      }
      await master.getByRole('radio', { name: 'Disparar uma armadilha do mapa' }).check();
      await master.getByLabel('Armadilha do mapa', { exact: true }).selectOption({ label: 'Dardos envenenados · A capela' });
      await stepPasses(master, `Novo quebra-cabeça: a sequência, com a armadilha ${where}`);
      await master.getByRole('button', { name: 'Tocar para testar' }).click();
      await stepPasses(master, `A sequência tocando para o teste ${where}`);

      await master.getByRole('radio', { name: /^Cifra/ }).check();
      await master.getByLabel('Mensagem', { exact: true }).fill(CIPHER.message);
      await expect(master.locator('app-cipher-form .cipher')).toHaveText(CIPHER.ciphertext, { timeout: 30_000 });
      await master.getByLabel('Qual pista guarda a chave').selectOption(clueId);
      await stepPasses(master, `Novo quebra-cabeça: a cifra, com a chave ${where}`);
      await master.getByRole('radio', { name: 'Palavra-chave' }).check();
      await master.getByRole('textbox', { name: 'Palavra-chave' }).fill('abc');
      await master.getByRole('button', { name: 'Criar quebra-cabeça' }).click();
      await expect(master.getByText('Essa palavra-chave não troca nenhuma letra.')).toBeVisible();
      await stepPasses(master, `A cifra com a palavra-chave recusada ${where}`);

      // The live view of each (state 5): one card at a time, in the main column.
      await openSessionPage(master, campaignId);
      const panel = master.getByRole('region', { name: 'Quebra-cabeças' });
      await panel.getByRole('button', { name: 'Ver ao vivo A porta da Cripta pergunta' }).click();
      const riddleCard = master.getByRole('article', { name: 'A porta da Cripta pergunta' });
      await expect(riddleCard.getByText(/Última jogada: Pensantus tentou “escuridão”: errou/)).toBeVisible();
      await riddleCard.scrollIntoViewIfNeeded();
      await stepPasses(master, `O enigma ao vivo ${where}`);
      await playSequenceRPC(master, campaignId, sequence);
      await moveRPC(player, campaignId, sequence, { sequence: { bell: 2 } }).catch(() => undefined);
      await panel.getByRole('button', { name: 'Ver ao vivo Os sinos do Salão do trono' }).click();
      const sequenceCard = master.getByRole('article', { name: 'Os sinos do Salão do trono' });
      await expect(sequenceCard.getByText('Tocar a sequência').first()).toBeVisible();
      await sequenceCard.scrollIntoViewIfNeeded();
      await stepPasses(master, `A sequência ao vivo ${where}`);
      await panel.getByRole('button', { name: 'Ver ao vivo A carta do Capitão' }).click();
      const cipherCard = master.getByRole('article', { name: 'A carta do Capitão' });
      await expect(cipherCard.getByText(`A mensagem: ${CIPHER.message}`)).toBeVisible();
      await cipherCard.scrollIntoViewIfNeeded();
      await stepPasses(master, `A cifra ao vivo ${where}`);
    }
    await showPuzzleRPC(master, campaignId, stopped);
    await showPuzzleRPC(master, campaignId, done);

    const playTo = async (id: string, name: string) => {
      await player.goto(`/campaigns/${campaignId}/session?puzzle=${id}`);
      await expect(player.getByRole('heading', { level: 1, name })).toBeVisible({ timeout: 30_000 });
    };

    // The riddle (state 6): the field, the hint button and the player's own part (states 9).
    await playTo(riddle, 'A porta da Cripta pergunta');
    await expect(player.getByRole('button', { name: 'Tentar uma dica · Investigação' })).toBeVisible();
    await stepPasses(player, `O enigma, com a dica por perícia e a parte ${where}`);
    await player.getByLabel('Sua resposta').fill('escuridão');
    await player.getByRole('button', { name: 'Responder' }).click();
    await expect(player.getByRole('alert').filter({ hasText: 'Não é isso.' })).toBeVisible();
    await stepPasses(player, `O enigma, resposta errada ${where}`);
    // A hint, with the table's dice: the app first, then the physical d20 typed (9b).
    await player.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(player.getByLabel('O d20 que você rolou')).toBeVisible();
    await player.getByLabel('O d20 que você rolou').fill('1');
    await stepPasses(player, `A dica por perícia, o d20 de um dado físico digitado ${where}`);
    await player.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(player.getByText('Não deu desta vez.')).toBeVisible();
    await stepPasses(player, `A dica por perícia que falhou ${where}`);
    await releaseHintRPC(master, campaignId, riddle);
    await expect(player.getByRole('button', { name: 'Tentar uma dica · Investigação' })).toBeVisible({ timeout: 30_000 });
    await setDiceModeRPC(master, campaignId, 'DICE_MODE_APP');
    await player.reload();
    await expect(player.getByRole('button', { name: 'Tentar uma dica · Investigação' })).toBeVisible({ timeout: 30_000 });
    await player.getByRole('button', { name: 'Tentar uma dica · Investigação' }).click();
    await expect(player.getByText('Você conseguiu.').or(player.getByText('Não deu desta vez.'))).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `A dica por perícia, rolada no app ${where}`);
    // Out of attempts: a dashed box with the reason.
    await moveRPC(player, campaignId, riddle, { riddle: { answer: 'luz' } });
    await moveRPC(player, campaignId, riddle, { riddle: { answer: 'sol' } });
    await expect(player.getByText('Você não tem mais tentativas.')).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `O enigma, sem tentativas ${where}`);

    // The sequence (state 7): not played, playing, repeating, a wrong step with the trap (state 10).
    await playTo(sequence, 'Os sinos do Salão do trono');
    await stepPasses(player, `A sequência ${where}`);
    await playSequenceRPC(master, campaignId, sequence);
    await expect(player.getByText('O mestre está tocando os sinos.')).toBeVisible({ timeout: 15_000 });
    await stepPasses(player, `A sequência tocando ${where}`);
    await expect(player.getByText('Agora é com vocês.')).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `A sequência, para repetir ${where}`);
    await moveRPC(player, campaignId, sequence, { sequence: { bell: 2 } });
    await expect(player.locator('.board-card').getByRole('alert').filter({ hasText: 'Errou o passo 1.' })).toBeVisible();
    await expect(player.getByText('A armadilha disparou:')).toBeVisible();
    await stepPasses(player, `A sequência, passo errado e a armadilha ${where}`);
    void SEQUENCE;

    // The cipher (state 8): the key not found, found, and a wrong message.
    await playTo(cipher, 'A carta do Capitão');
    await expect(player.getByText('Ainda não acharam a chave.')).toBeVisible();
    await stepPasses(player, `A cifra, sem a chave ${where}`);
    await revealClueRPC(master, campaignId, clueId, [table.characterId]);
    await expect(player.getByText('Pista achada na aventura')).toBeVisible({ timeout: 30_000 });
    await player.getByLabel('A mensagem decifrada').fill('o tesouro esta sobre o altar');
    await player.getByRole('button', { name: 'Conferir' }).click();
    await expect(player.getByRole('alert').filter({ hasText: 'Não é isso.' })).toBeVisible();
    await stepPasses(player, `A cifra, com a chave achada e uma mensagem errada ${where}`);

    // A limit stopped it, and a solved riddle.
    await playTo(stopped, 'A porta selada');
    await moveRPC(player, campaignId, stopped, { riddle: { answer: 'luz' } });
    await expect(player.getByText('O quebra-cabeça parou.')).toBeVisible();
    await stepPasses(player, `O enigma parou por um limite ${where}`);
    await playTo(done, 'O enigma resolvido');
    await moveRPC(player, campaignId, done, { riddle: { answer: 'sombra' } });
    await expect(player.getByText('O enigma foi respondido.')).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `O enigma resolvido ${where}`);

    // The master's view of the consequences: the trap that fired, and the puzzle a limit stopped.
    if (masterToo) {
      await openSessionPage(master, campaignId);
      const live = master.getByRole('region', { name: 'Quebra-cabeças' });
      await live.getByRole('button', { name: 'Ver ao vivo Os sinos do Salão do trono' }).click();
      const trapped = master.getByRole('article', { name: 'Os sinos do Salão do trono' });
      await expect(trapped.getByText('A armadilha disparou:')).toBeVisible();
      await trapped.scrollIntoViewIfNeeded();
      await stepPasses(master, `A sequência ao vivo, com a armadilha disparada ${where}`);
      await live.getByRole('button', { name: 'Ver ao vivo A porta selada' }).click();
      const halted = master.getByRole('article', { name: 'A porta selada' });
      await expect(halted.getByText('Parou').first()).toBeVisible();
      await halted.scrollIntoViewIfNeeded();
      await stepPasses(master, `O enigma parado por um limite, ao vivo ${where}`);
    }

    // The sequence and the cipher solved: the player's phone and the master's card.
    for (const bell of SEQUENCE) {
      await moveRPC(player, campaignId, sequence, { sequence: { bell } });
    }
    await moveRPC(player, campaignId, cipher, { cipher: { text: 'o tesouro esta sob o altar' } });
    await playTo(sequence, 'Os sinos do Salão do trono');
    await expect(player.getByText('Os sinos tocaram na ordem certa.')).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `A sequência resolvida ${where}`);
    await playTo(cipher, 'A carta do Capitão');
    await expect(player.getByText('A mensagem foi decifrada.')).toBeVisible({ timeout: 30_000 });
    await stepPasses(player, `A cifra resolvida ${where}`);
    if (masterToo) {
      const solvedPanel = master.getByRole('region', { name: 'Quebra-cabeças' });
      for (const [name, screen] of [
        ['Os sinos do Salão do trono', 'A sequência resolvida, ao vivo'],
        ['A carta do Capitão', 'A cifra resolvida, ao vivo'],
      ] as const) {
        await solvedPanel.getByRole('button', { name: `Ver ao vivo ${name}` }).click();
        const done = master.getByRole('article', { name });
        await expect(done.getByText(/resolveu “/)).toBeVisible({ timeout: 30_000 });
        await done.scrollIntoViewIfNeeded();
        await stepPasses(master, `${screen} ${where}`);
      }
    }

    // Toren's phone: the split information from the other side.
    await openPage(toren.page, `/campaigns/${campaignId}/session?puzzle=${riddle}`);
  } finally {
    if (toren) {
      await toren.close();
    }
    if (campaignId) {
      await endTable(master, campaignId);
    }
    await masterContext.close();
    await playerContext.close();
  }
}

test('os quebra-cabeças novos passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMorePuzzleScreens(browser, 'light', 1280);
});

test('os quebra-cabeças novos passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMorePuzzleScreens(browser, 'dark', 390);
});

test('os quebra-cabeças novos passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(500_000);
  await scanMorePuzzleScreens(browser, 'light', 320, false);
});

test('os quebra-cabeças novos passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-038'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMorePuzzleScreens(browser, 'dark', 1024);
});

/**
 * "Magias" (MR-045, E10-11): the list with the filters on, a spell open (a table spell and an SRD one), a search with no
 * result, the phone's "Filtros" sheet, the failed list and a basic sheet with "Só as que posso aprender".
 */
async function scanSpellsScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number, height = 900): Promise<void> {
  const masterContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height } });
  const context = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const master = await masterContext.newPage();
  const page = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  // Under 1100 px the filters live in a sheet (a bottom sheet on a phone, a dialog from a tablet).
  const phone = width < 1100;
  try {
    await master.goto('/');
    await page.goto('/');
    const table = await tableForSpells(master, page, `Acessibilidade magias ${Date.now()}`);
    await createInkBladeRPC(master, table.campaignId);
    const url = `/campaigns/${table.campaignId}/spells`;
    const rows = page.locator('button.row');

    // Loading: the answer is held until the scan is done.
    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/meurpg.rules.v1.ContentService/ListSpells', async (route) => {
      await held;
      await route.continue();
    });
    await page.goto(url);
    await expect(page.getByText('Buscando as magias…')).toBeAttached();
    await expectScreenPasses(page, `Magias, carregando ${where}`);
    release();
    await expect(rows.first()).toBeVisible();
    await page.unroute('**/meurpg.rules.v1.ContentService/ListSpells');
    await expectScreenPasses(page, `Magias, a lista ${where}`);

    // The filters on: the class and "Só as que posso aprender" (the panel at 1280 px, the sheet on a phone).
    await page.goto(`${url}?class=class:wizard&mine=1`);
    await expect(rows.first()).toBeVisible();
    await expectScreenPasses(page, `Magias, filtros ligados ${where}`);
    if (phone) {
      await page.getByRole('button', { name: /Filtros/ }).click();
      const sheet = page.getByRole('dialog', { name: 'Filtros' });
      await expect(sheet.getByRole('button', { name: /^Ver \d+ magias$/ })).toBeVisible();
      await expectScreenPasses(page, `Magias, a folha de filtros ${where}`);
      await sheet.getByRole('button', { name: 'Fechar' }).click();
      await expect(sheet).toBeHidden();
    }

    // A spell open: the table's own, then the SRD's.
    await page.goto(`${url}?q=nanquim`);
    await rows.first().click();
    await expect(page.locator('#spell-card-title')).toHaveText('Lâmina de Nanquim');
    await expectScreenPasses(page, `Magias, uma magia da mesa ${where}`);
    await page.goto(`${url}?q=maos`);
    await rows.first().click();
    await expect(page.locator('#spell-card-title')).toHaveText('Mãos Flamejantes');
    await expectScreenPasses(page, `Magias, uma magia do SRD ${where}`);

    // No result.
    await page.goto(`${url}?q=zzz`);
    await expect(page.getByText('Nenhuma magia com “zzz”.')).toBeVisible();
    await expectScreenPasses(page, `Magias, busca sem resultado ${where}`);

    // A basic sheet with the filter on.
    await page.route('**/meurpg.rules.v1.ContentService/ListSpells', (route) =>
      (route.request().postData() ?? '').includes('characterId')
        ? route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 'failed_precondition', message: 'x' }) })
        : route.continue(),
    );
    await page.goto(`${url}?mine=1`);
    await expect(page.getByText('Essa ficha é básica e não tem classes que conjuram', { exact: false })).toBeVisible();
    await expectScreenPasses(page, `Magias, ficha básica ${where}`);
    await page.unroute('**/meurpg.rules.v1.ContentService/ListSpells');

    // Failing: the server does not answer.
    await page.route('**/meurpg.rules.v1.ContentService/ListSpells', (route) =>
      route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) }),
    );
    await page.goto(url);
    await expect(page.getByRole('alert')).toContainText('o servidor não respondeu');
    await expectScreenPasses(page, `Magias, erro ${where}`);
    await page.unroute('**/meurpg.rules.v1.ContentService/ListSpells');
    await page.getByRole('button', { name: 'Tentar de novo' }).click();
    await expect(rows.first()).toBeVisible();

    // The campaign page with its "Magias" panel.
    await page.goto(`/campaigns/${table.campaignId}/setup`);
    const open = page.getByRole('link', { name: 'Abrir as magias' });
    await open.scrollIntoViewIfNeeded();
    await expect(open).toBeVisible();
    await expectScreenPasses(page, `Campanha com o painel Magias ${where}`);
  } finally {
    await masterContext.close();
    await context.close();
  }
}

test('as Magias passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-045'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  await scanSpellsScreens(browser, 'light', 1280);
});

test('as Magias passam no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@MR-045'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  await scanSpellsScreens(browser, 'dark', 1280);
});

test('as Magias passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-045'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  await scanSpellsScreens(browser, 'dark', 390);
});

test('as Magias passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-045'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  await scanSpellsScreens(browser, 'light', 320, 568);
});

test('as Magias passam no axe e nas conferências de layout no tema claro, no tablet de 1024 (o diálogo de filtros)', { tag: ['@a11y', '@MR-045'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  await scanSpellsScreens(browser, 'light', 1024, 768);
});

/** The combat without a map (Etapa 10, slice 10.13b; E10-04): the start dialog in both modes, the master's screen with the movement, the offer and
 * the cover, the player's turn, the "Gastar movimento" sheet, the target list, the opportunity question, the player out of turn, and Brisa down with the
 * death saves the table hides (her phone, the master's order). Both pages are at `width`. */
async function scanTheatreScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number, height = 900): Promise<void> {
  const viewport = { width, height };
  const master = await newSignedInContext(browser, 'Mestre Teste', { colorScheme, viewport });
  const player = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  let lia: Awaited<ReturnType<typeof secondPlayer>> | null = null;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade teatro ${Date.now()}`, true, false, { build: toren, sheet: torenSheet });
    campaignId = table.campaignId;
    lia = await secondPlayer(browser, m, campaignId, brisa, brisaSheet);
    await lia.page.setViewportSize(viewport);
    await lia.page.emulateMedia({ colorScheme });
    await setTableRulesRPC(m, campaignId, { combatStartsWithMap: false, deathSaves: 'DEATH_SAVE_VISIBILITY_OWNER_AND_MASTER' });

    // The dialog: "Sem mapa" with its one line, and "Com mapa" with the map.
    await openSessionPage(m, campaignId);
    await clickStartCombat(m);
    const dialog = m.getByRole('dialog', { name: 'Iniciar combate' });
    await expect(dialog.getByTestId('theatre-why')).toBeVisible();
    await expectScreenPasses(m, `Iniciar combate, sem mapa ${where}`);
    await dialog.locator('label', { hasText: 'Com mapa' }).first().click();
    await expect(dialog.getByText('Mapa do combate')).toBeVisible();
    await expectScreenPasses(m, `Iniciar combate, com mapa ${where}`);
    await dialog.locator('label', { hasText: 'Sem mapa' }).first().click();
    await dialog.getByRole('button', { name: 'Mais um Goblin', exact: true }).click();
    await dialog.getByRole('button', { name: 'Mais um Capitão Goblin' }).click();
    await dialog.getByRole('button', { name: 'Iniciar combate' }).click();
    await expect(m.getByText('Os NPCs rolaram sozinhos.')).toBeVisible();
    let enc = await getEncounterRPC(m, campaignId);
    for (const c of enc.combatants.filter((x) => x.kind === 'COMBATANT_KIND_NPC')) {
      enc = await combatRPC(m, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: c.id, hidden: false });
    }
    await openSessionPage(p, campaignId);
    await openSessionPage(lia.page, campaignId);
    const begun = await beginTheatreRPC(m, table, enc, { Toren: 20, Brisa: 18, Goblin: 12, 'Capitão Goblin': 10 });
    await waitForEncounterRevision([m, p], begun);

    // The master's screen, Toren on turn, and the cover's four rows.
    await expect(m.getByRole('heading', { name: /^(Ações do|Vez do) Toren$/ })).toBeVisible();
    await expectScreenPasses(m, `Combate sem mapa, mestre, vez de um jogador ${where}`);
    await m.getByRole('button', { name: 'Mudar a cobertura de Capitão Goblin' }).click();
    await expect(m.getByRole('heading', { name: 'Cobertura do Capitão Goblin' })).toBeVisible();
    await expectScreenPasses(m, `Cobertura do alvo ${where}`);
    await m.getByRole('button', { name: 'Cancelar' }).click();

    // The player's turn, the sheet and the target list.
    await expect(p.getByRole('heading', { name: 'Combate sem mapa' })).toBeVisible();
    await expectScreenPasses(p, `Combate sem mapa, jogador, sua vez ${where}`);
    await p.getByRole('button', { name: 'Gastar movimento' }).first().click();
    await expect(p.getByText('Você tem 9,0 m neste turno')).toBeVisible();
    await expectScreenPasses(p, `Gastar movimento ${where}`);
    for (let i = 0; i < 3; i++) {
      await p.getByRole('button', { name: 'Mais 1,5 m' }).click();
    }
    await p.getByRole('button', { name: /^Gastar 6,0/ }).click();
    await expect(p.getByRole('listitem', { name: /^Movimento: 3,0\sm$/ }).first()).toBeVisible();
    await expectScreenPasses(p, `Depois de gastar o movimento ${where}`);
    await p.getByRole('button', { name: 'Atacar com Espada longa' }).click();
    await expect(p.getByText('O mestre decide quem está ao alcance.')).toBeVisible();
    await expectScreenPasses(p, `Alvos sem distância ${where}`);
    await p.getByRole('radiogroup').getByText('Capitão Goblin', { exact: true }).click();
    await p.getByRole('button', { name: 'Rolar no app' }).click();
    await expect(p.getByText(/Acertou|Errou|Crítico/).first()).toBeVisible();
    await expectScreenPasses(p, `Resultado do ataque sem mapa ${where}`);
    await p.keyboard.press('Escape');
    // The rest of the movement: "Sem movimento", the button dashed with its reason.
    await p.getByRole('button', { name: 'Gastar movimento' }).first().click();
    await p.getByRole('button', { name: 'Mais 1,5 m' }).click();
    await p.getByRole('button', { name: /^Gastar 3,0/ }).click();
    await expect(p.getByText('Você já gastou todo o movimento deste turno.')).toBeVisible();
    await expectScreenPasses(p, `Sem movimento ${where}`);

    // The master's turn for the Goblin: the offer's form, the wait and the player's question.
    enc = await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: (await getEncounterRPC(m, campaignId)).currentCombatantId, discardPendingDamage: true });
    await waitForEncounterRevision([m, p], await passTurnsTo(m, campaignId, 'Goblin'));
    await expect(m.getByRole('heading', { name: /^(Ações do|Vez do) Goblin$/ })).toBeVisible();
    await expectScreenPasses(m, `Combate sem mapa, mestre, vez de um NPC ${where}`);
    await m.getByRole('button', { name: 'Oferecer ataque de oportunidade' }).click();
    await expect(m.getByRole('heading', { name: 'Oferecer ataque de oportunidade' })).toBeVisible();
    await expectScreenPasses(m, `Oferecer ataque de oportunidade ${where}`);
    await m.getByRole('radio', { name: /Toren/ }).check({ force: true });
    await m.getByRole('button', { name: 'Oferecer a Toren' }).click();
    const prompt = p.getByRole('alertdialog', { name: 'Ataque de oportunidade' });
    await expect(prompt).toBeVisible();
    await expectScreenPasses(p, `Pergunta do ataque de oportunidade ${where}`);
    await expect(m.getByRole('status').filter({ hasText: 'Esperando a resposta do' })).toBeVisible();
    await expectScreenPasses(m, `Esperando a resposta do jogador ${where}`);
    await prompt.getByRole('button', { name: 'Não atacar' }).click();
    await expect(prompt).toHaveCount(0);
    await expectScreenPasses(p, `Combate sem mapa, jogador, fora da vez ${where}`);

    // Brisa falls: the death saves are hers and the master's; Toren reads only "Caída".
    await adjustVitalsRPC(m, campaignId, lia.characterId, { hitPointsCurrent: 0 });
    enc = await getEncounterRPC(m, campaignId);
    await passTurnsTo(m, campaignId, 'Toren');
    await passTurnsTo(m, campaignId, 'Brisa');
    await expect(lia.page.getByTestId('death-private')).toBeVisible();
    await expectScreenPasses(lia.page, `Brisa caída, o dono ${where}`);
    await expectScreenPasses(m, `Brisa caída, o mestre ${where}`);
    await expect(p.getByText('Caída').first()).toBeVisible();
    await expectScreenPasses(p, `Brisa caída, o outro jogador ${where}`);
  } finally {
    await lia?.close();
    await master.close();
    await player.close();
    if (campaignId) {
      const cleanup = await newSignedInContext(browser, 'Mestre Teste');
      const page = await cleanup.newPage();
      await page.goto('/');
      await endOpenSessionRPC(page, campaignId);
      await cleanup.close();
    }
  }
}

for (const [scheme, width, label] of [
  ['light', 1280, 'claro, no desktop'],
  ['dark', 1280, 'escuro, no desktop'],
  ['light', 390, 'claro, no celular'],
  ['dark', 390, 'escuro, no celular'],
] as const) {
  test(`o combate sem mapa passa no axe e nas conferências de layout no tema ${label}`, { tag: ['@a11y', '@MR-025', '@RN-25'] }, async ({ browser }) => {
    test.setTimeout(300_000);
    await scanTheatreScreens(browser, scheme, width);
  });
}

for (const [scheme, label] of [
  ['light', 'claro'],
  ['dark', 'escuro'],
] as const) {
  test(`o combate sem mapa a 320 × 568 passa no axe e nas conferências de layout no tema ${label}`, { tag: ['@a11y', '@MR-025', '@RN-25'] }, async ({ browser }) => {
    test.setTimeout(300_000);
    await scanTheatreScreens(browser, scheme, 320, 568);
  });
}

/** The critical's line and the live total with physical dice, under "o máximo mais uma rolagem" (RN-24): the damage step on a phone. */
async function scanTheatreCritical(browser: Browser, colorScheme: 'light' | 'dark', width: number, height: number): Promise<void> {
  const viewport = { width, height };
  const master = await newSignedInContext(browser, 'Mestre Teste', { colorScheme, viewport });
  const player = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width} × ${height})`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade crítico ${Date.now()}`, true, false, { build: toren, sheet: torenSheet });
    campaignId = table.campaignId;
    await setTableRulesRPC(m, campaignId, { combatStartsWithMap: false, diceMode: 'DICE_MODE_PHYSICAL', critical: 'CRITICAL_RULE_MAX_PLUS_ROLL' });
    const { startTheatreRPC } = await import('./theatre-support');
    const enc0 = await startTheatreRPC(m, table, [{ characterId: table.captainId, count: 1, hidden: false }]);
    await beginTheatreRPC(m, table, enc0, { Toren: 20, 'Capitão Goblin': 5 });
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Atacar com Espada longa' }).click();
    await p.getByRole('radiogroup').getByText('Capitão Goblin', { exact: true }).click();
    await p.getByLabel(/Role 1d20 para Espada longa/).fill('20');
    await p.getByRole('button', { name: /^Confirmar/ }).click();
    await expect(p.getByText(/o máximo mais uma rolagem/).first()).toBeVisible();
    await p.getByLabel(/Role 1d8 para o dano/).fill('5');
    await expect(p.getByText('5 + 11 = 16')).toBeVisible();
    await expectScreenPasses(p, `Dano do crítico, dado físico ${where}`);
  } finally {
    await master.close();
    await player.close();
    if (campaignId) {
      const cleanup = await newSignedInContext(browser, 'Mestre Teste');
      const page = await cleanup.newPage();
      await page.goto('/');
      await endOpenSessionRPC(page, campaignId);
      await cleanup.close();
    }
  }
}

for (const [scheme, label] of [
  ['light', 'claro'],
  ['dark', 'escuro'],
] as const) {
  test(`o dano do crítico com dado físico passa no axe e nas conferências de layout no tema ${label}, a 320 × 568`, { tag: ['@a11y', '@RN-24', '@MR-025'] }, async ({ browser }) => {
    test.setTimeout(240_000);
    await scanTheatreCritical(browser, scheme, 320, 568);
  });
}

/**
 * "Conteúdo da mesa" (MR-025, RN-23; E10-01): the master's list and editors (a spell, a race, a background), the question to
 * archive, a refusal on its field; the same list on a phone with the sheet that asks to archive; and what a player reads. The
 * entries come through the API, with one archived so its state shows.
 */
async function scanTableContent(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(180_000);
  const mContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height: 900 } });
  const pContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height: 900 } });
  try {
    const m = await mContext.newPage();
    const p = await pContext.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Acessibilidade conteúdo ${Date.now()}`);
    const where = `(${colorScheme}, ${width}px)`;
    const spell = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));
    await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Sopro de Nanquim', {
      range: { kind: 'SPELL_RANGE_KIND_SELF' },
      target: { kind: 'TABLE_SPELL_TARGET_KIND_AREA', shape: 'TABLE_AREA_SHAPE_CONE', sizeFt: 15 },
      attack: '',
      save: { ability: 'ABILITY_DEXTERITY', onSuccess: 'SPELL_SAVE_SUCCESS_HALF' },
      damage: [{ damageTypeKey: 'damage-type:necrotic', dice: '3d6', perSlotLevel: '1d6' }],
    }));
    const archived = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Rascunho de Tinta'));
    const race = await createEntryRPC(m, campaignId, 'tableRace', raceBody());
    const background = await createEntryRPC(m, campaignId, 'tableBackground', {
      namePt: 'Cartógrafo do Vale',
      skills: ['skill:investigation', 'skill:survival'],
      tools: ['proficiency:thieves-tools'],
      equipmentPt: 'Um estojo de mapas, tinta e 10 PO',
      feature: { namePt: 'Mapas na memória', descPt: ['Você lembra o desenho de qualquer lugar que já mapeou.'], effects: [{ type: 'note', textPt: 'Lembra qualquer lugar mapeado.' }] },
    });
    await archiveEntryRPC(m, campaignId, archived);

    await open(m, `/campaigns/${campaignId}/content?kind=spells`);
    await expectScreenPasses(m, `Conteúdo da mesa, as magias ${where}`);
    if (width >= 768) {
      await open(m, `/campaigns/${campaignId}/content`);
      await expectScreenPasses(m, `Conteúdo da mesa, a lista inicial ${where}`);
      await open(m, entryRoute(campaignId, spell));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Lâmina de Nanquim');
      await expectScreenPasses(m, `Editor de magia ${where}`);
      await open(m, entryRoute(campaignId, race));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Corujeiro');
      await expectScreenPasses(m, `Editor de raça ${where}`);
      await m.getByRole('button', { name: 'Mais opções' }).first().click();
      await expect(m.getByRole('button', { name: 'Menos opções' }).first()).toBeVisible();
      await expectScreenPasses(m, `Editor de raça, "Mais opções" aberto ${where}`);
      await m.getByRole('button', { name: 'Arquivar', exact: true }).click();
      await expect(m.getByRole('region', { name: 'Arquivar Corujeiro?' })).toBeVisible();
      await expectScreenPasses(m, `Arquivar a raça, a pergunta no lugar ${where}`);
      await m.getByRole('button', { name: 'Arquivar Corujeiro' }).click();
      await expect(m.getByText('A raça Corujeiro está arquivada.')).toBeVisible();
      await expectScreenPasses(m, `A raça arquivada, com "Desarquivar" ${where}`);
      // It comes back at once, so the player's screens below still have it.
      await m.getByRole('button', { name: 'Desarquivar' }).click();
      await expect(m.getByText('A raça Corujeiro está arquivada.')).toHaveCount(0);
      await open(m, `/campaigns/${campaignId}/content/new/subrace`);
      await expectScreenPasses(m, `Editor de sub-raça, a raça a escolher ${where}`);
      await open(m, entryRoute(campaignId, background));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Cartógrafo do Vale');
      await expectScreenPasses(m, `Editor de antecedente ${where}`);
      await open(m, `/campaigns/${campaignId}/content/new/option`);
      await expectScreenPasses(m, `Editor de opção, o conjunto a escolher ${where}`);
      const option = await createEntryRPC(m, campaignId, 'tableOption', {
        namePt: 'Olhar do Corvo',
        descPt: ['Você enxerga o que o corvo vê.'],
        set: 'invocation',
        prerequisite: { classLevel: { classKey: 'class:warlock', level: 5 }, requiresPact: 'feature:pact-of-the-tome' },
        effects: [{ type: 'note', textPt: 'Enxerga pelos olhos de um corvo.' }],
      });
      await open(m, entryRoute(campaignId, option));
      await expect(m.getByLabel('Nome da opção')).toHaveValue('Olhar do Corvo');
      await expectScreenPasses(m, `Editor de opção ${where}`);
      await open(m, `/campaigns/${campaignId}/content/new/classfeature`);
      await expectScreenPasses(m, `Editor de característica de classe, a classe a escolher ${where}`);
      const classFeature = await createEntryRPC(m, campaignId, 'tableClassFeature', {
        namePt: 'Rastro do Corvo',
        descPt: ['Você segue o rastro de um corvo.'],
        classKey: 'class:ranger',
        level: 1,
        replacesFeatureKey: 'feature:favored-enemy-1-type',
        effects: [{ type: 'note', textPt: 'Segue o rastro de um corvo.' }],
      });
      await open(m, entryRoute(campaignId, classFeature));
      await expect(m.getByLabel('Nome da característica')).toHaveValue('Rastro do Corvo');
      await expectScreenPasses(m, `Editor de característica de classe ${where}`);
      // A trait with a pool that comes back on initiative, an action that spends 1 to 3 of it to heal, and a long-rest hook.
      const costs = await createEntryRPC(m, campaignId, 'tableRace', costsRaceBody());
      await open(m, entryRoute(campaignId, costs));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Brasa de Teste');
      await expect(m.getByLabel('Custo máximo (o jogador escolhe)')).toHaveValue('3');
      await expectScreenPasses(m, `Editor de raça, a ação com custo e cura, o recurso que volta na iniciativa e o gatilho ${where}`);
      // A refusal at the field: the most the player may spend is below the cost.
      await m.getByLabel('Custo máximo (o jogador escolhe)').fill('0');
      await m.getByRole('button', { name: 'Salvar raça' }).click();
      await expect(m.getByLabel('Custo máximo (o jogador escolhe)')).toHaveAttribute('aria-invalid', 'true');
      await expectScreenPasses(m, `Editor de raça, a recusa no custo máximo ${where}`);
      await open(m, `/campaigns/${campaignId}/content/new/spell`);
      await m.getByLabel('Nome', { exact: true }).fill('Lâmina de Nanquim');
      await m.getByLabel('Distância').fill('18');
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      await expect(m.getByText('Já existe uma magia da mesa com este nome. Escolha outro.')).toBeVisible();
      await expectScreenPasses(m, `Editor de magia, a recusa no campo ${where}`);
      // Another tab saves first: the stale alert, with "Recarregar".
      await open(m, entryRoute(campaignId, spell));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Lâmina de Nanquim');
      await updateEntryRPC(m, campaignId, spell, 'tableSpell', spellBody('Lâmina de Nanquim', { descPt: ['Outro texto.'] }));
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      await expect(m.getByRole('alert').filter({ hasText: 'Esta entrada mudou enquanto você editava.' })).toBeVisible();
      await expectScreenPasses(m, `Editor de magia, a entrada mudou enquanto se editava ${where}`);
    } else {
      await m.getByRole('button', { name: 'Arquivar Lâmina de Nanquim' }).click();
      await expect(m.getByRole('heading', { name: 'Arquivar Lâmina de Nanquim?' })).toBeVisible();
      await expectScreenPasses(m, `Arquivar, a folha de baixo ${where}`);
      await m.getByRole('button', { name: 'Voltar', exact: true }).click();
      await open(m, entryRoute(campaignId, race));
      await expectScreenPasses(m, `Raça lida pelo mestre no celular ${where}`);
    }

    await open(p, `/campaigns/${campaignId}/content`);
    await expect(p.getByText('Da mesa').first()).toBeVisible();
    await expectScreenPasses(p, `Conteúdo da mesa, visto por um jogador ${where}`);
    await open(p, entryRoute(campaignId, race));
    await expect(p.getByText('Olhos de caçador.')).toBeVisible();
    await expectScreenPasses(p, `Raça, vista por um jogador ${where}`);
    await open(p, entryRoute(campaignId, spell));
    await expectScreenPasses(p, `Magia, vista por um jogador ${where}`);
    await open(p, entryRoute(campaignId, background));
    await expect(p.getByText('Ferramentas de ladrão')).toBeVisible();
    await expectScreenPasses(p, `Antecedente, visto por um jogador ${where}`);
  } finally {
    await Promise.all([mContext.close(), pContext.close()]);
  }
}

test('o conteúdo da mesa passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanTableContent(browser, 'light', 1280);
});

test('o conteúdo da mesa passa no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-025'] }, async ({ browser }) => {
  await scanTableContent(browser, 'dark', 1024);
});

test('o conteúdo da mesa passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanTableContent(browser, 'dark', 390);
});

test('o conteúdo da mesa passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-025'] }, async ({ browser }) => {
  await scanTableContent(browser, 'light', 320);
});

// Generated images (slice 10.16, MR-039, RN-28, RN-10; E10-07): the "Gerar imagem" dialog from a map (the top and the end of its body, the
// textured map, the wait, the result with its adjustment, "Usar como imagem do mapa" asked in place, a refusal in words) and from the gallery
// (with the chain of an adjustment), and on a phone the same as a sheet with a fixed footer. The fake generator makes the pictures.
async function scanImageScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const players = [await newSignedInContext(browser, 'Jogador Teste'), await newSignedInContext(browser, 'E-mail Não Verificado')];
  const context = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const m = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const d = m.getByRole('dialog');
  const body = () => m.locator('.frame__body');
  try {
    await m.goto('/');
    const [ap, bp] = await Promise.all(players.map((c) => c.newPage()));
    await Promise.all([ap.goto('/'), bp.goto('/')]);
    const table = await tableForImages(m, ap, bp, `Acessibilidade imagens ${Date.now()}`, false);
    await generateSceneRPC(m, table.campaignId, 'Uma taverna à noite');

    // From the map: the top, the end, the textured map.
    await open(m, mapRoute(table));
    await m.getByRole('button', { name: 'Gerar imagem com IA' }).click();
    await expect(d.getByText('Quem aparece na imagem')).toBeVisible();
    await d.getByRole('checkbox', { name: /Capitão Goblin/ }).click();
    await body().evaluate((el) => (el.scrollTop = 0));
    await expectScreenPasses(m, `Gerar imagem, o topo ${where}`);
    await body().evaluate((el) => (el.scrollTop = el.scrollHeight));
    await expectScreenPasses(m, `Gerar imagem, o fim ${where}`);
    await body().evaluate((el) => (el.scrollTop = 0));
    await d.getByRole('radio', { name: /Textura|O mapa com textura/ }).click();
    await expectScreenPasses(m, `Gerar imagem, o mapa com textura ${where}`);
    await d.getByRole('radio', { name: /Cena|Arte da cena/ }).click();

    // Waiting, and the question on closing.
    // The long poll is held by the test, not by the fake's own delay: the screens are scanned for as long as it takes.
    let openGate!: () => void;
    const gate = new Promise<void>((resolve) => (openGate = resolve));
    await m.route('**/meurpg.maps.v1.ImageGenerationService/GetImageGeneration', async (route) => {
      await gate;
      await route.continue();
    });
    await d.getByRole('textbox', { name: 'Descreva o lugar' }).fill('Uma sala de guarda com tochas');
    await d.getByRole('button', { name: 'Gerar imagem' }).click();
    await expect(d.getByText('Gerando a imagem…')).toBeVisible({ timeout: 20_000 });
    await expectScreenPasses(m, `Gerar imagem, gerando ${where}`);
    await d.getByRole('button', { name: 'Fechar' }).click();
    await expect(d.getByText('Parar de esperar a imagem?')).toBeVisible();
    await expectScreenPasses(m, `Gerar imagem, parar de esperar perguntado ${where}`);
    // Let the picture through: the wait ends in the picture whether or not the master answered the question.
    openGate();

    // The result, with an adjustment typed.
    await expect(d.getByText('Guardada na galeria')).toBeVisible({ timeout: 60_000 });
    await expectScreenPasses(m, `Gerar imagem, o resultado ${where}`);
    await d.getByRole('textbox', { name: 'Pedir um ajuste' }).fill('mais escura');
    await body().evaluate((el) => (el.scrollTop = el.scrollHeight));
    await expectScreenPasses(m, `Gerar imagem, o ajuste escrito ${where}`);
    await d.getByRole('button', { name: 'Pedir o ajuste' }).click();
    await expect(d.getByText('A cadeia de ajustes')).toBeVisible({ timeout: 60_000 });
    await expectScreenPasses(m, `Gerar imagem, o resultado com a cadeia ${where}`);
    await d.getByRole('button', { name: 'Fechar' }).click();

    // The textured map: "Usar como imagem do mapa" asked in place.
    await m.getByRole('button', { name: 'Gerar imagem com IA' }).click();
    await d.getByRole('radio', { name: /Textura|O mapa com textura/ }).click();
    await d.getByRole('textbox', { name: 'Descreva o lugar' }).fill('Uma caverna de pedra clara');
    await d.getByRole('button', { name: 'Gerar imagem' }).click();
    await expect(d.getByRole('button', { name: 'Usar como imagem do mapa' })).toBeVisible({ timeout: 60_000 });
    await expectScreenPasses(m, `Gerar imagem, o mapa com textura pronto ${where}`);
    await d.getByRole('button', { name: 'Usar como imagem do mapa' }).click();
    await expect(d.getByText('Usar como imagem do mapa?')).toBeVisible();
    await expectScreenPasses(m, `Gerar imagem, "Usar como imagem do mapa" perguntado ${where}`);
    await d.getByRole('button', { name: 'Voltar' }).click();
    await d.getByRole('button', { name: 'Fechar' }).click();

    // The gallery: the tags, the chain, a refusal in words.
    await open(m, `/campaigns/${table.campaignId}/gallery`);
    await expectScreenPasses(m, `Galeria com imagens geradas ${where}`);
    await m.getByRole('button', { name: 'Gerar imagem com IA' }).click();
    await expect(d.getByRole('heading', { level: 2, name: 'Gerar imagem' })).toBeVisible();
    await expectScreenPasses(m, `Gerar imagem, da galeria ${where}`);
    await d.getByRole('textbox', { name: 'Descreva a cena' }).fill('[recusa] Uma taverna');
    await d.getByRole('button', { name: 'Gerar imagem' }).click();
    await expect(d.getByRole('alert')).toContainText('O serviço recusou', { timeout: 30_000 });
    await expectScreenPasses(m, `Gerar imagem, o serviço recusou ${where}`);
    await d.getByRole('button', { name: 'Fechar' }).click();

    // Generation off: the button stays, dashed, with the reason.
    await m.route('**/meurpg.maps.v1.ImageGenerationService/GetImageGenerationStatus', (r) =>
      r.fulfill({ contentType: 'application/json', body: JSON.stringify({ status: { enabled: false, monthlyLimit: 20, remaining: 20, month: '2026-10' } }) }),
    );
    await open(m, `/campaigns/${table.campaignId}/gallery`);
    await expect(m.getByText('A geração de imagens não está ligada neste servidor.')).toBeVisible();
    await expectScreenPasses(m, `Galeria, geração desligada ${where}`);
    await endOpenSessionRPC(m, table.campaignId).catch(() => undefined);
  } finally {
    await context.close();
    await Promise.all(players.map((c) => c.close()));
  }
}

for (const [scheme, width, label] of [
  ['light', 1280, 'tema claro, no desktop'],
  ['dark', 1024, 'tema escuro, no desktop de 1024'],
  ['dark', 390, 'tema escuro, no celular'],
  ['light', 390, 'tema claro, no celular'],
  ['dark', 320, 'tema escuro, no celular de 320'],
  ['light', 320, 'tema claro, no celular de 320'],
] as const) {
  test(`as imagens geradas passam no axe e nas conferências de layout no ${label}`, { tag: ['@a11y', '@MR-039'] }, async ({ browser }) => {
    test.setTimeout(600_000);
    await scanImageScreens(browser, scheme, width);
  });
}

/**
 * Monsters in the combat and the encounter builder (MR-042, MR-043, RN-29; E10-08 states 4 to 9, E10-09): the bestiary rows with
 * "Pôr no combate" and its sheet (with a combat in preparation, one to start, and the error), the master's order with the
 * monsters, what a player sees, the end-of-combat XP by each ND; the builder (empty, filled, above high, an NPC in the party, a
 * failed measure), "Pôr um NPC no grupo", "Gerar encontro" with "Trocar", "Guardar no ponto de batalha" (and the question in place),
 * and "Começar este combate" on the session with "Iniciar combate" filled. The sheets are drawn at 320 x 568 on the narrowest phone.
 */
async function scanMonsterScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const height = width === 320 ? 568 : 900;
  const context = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height } });
  const playerContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const m = await context.newPage();
  const p = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade monstros ${Date.now()}`);
    const campaignId = table.campaignId;
    const battle = await createPointRPC(m, campaignId, table.mapId, { kind: 'BATTLE', name: 'Emboscada na ponte', xBp: 3000, yBp: 3000, revealed: true });
    await createPointRPC(m, campaignId, table.mapId, { kind: 'BATTLE', name: 'Ruínas do forte', xBp: 7000, yBp: 3000, revealed: true });
    const orin = await createCharacterRPC(m, campaignId, {
      kind: 'CHARACTER_KIND_STORY',
      name: 'Orin, o guia',
      sheet: { basic: { hitPointsMax: 9, armorClass: 10, speedFt: 25, attackBonus: 0, damage: '1d4', description: '' } },
    });
    expect(orin.ok(), await orin.text()).toBeTruthy();

    // "Pôr no combate" from the list, with no combat open: the sheet starts one.
    await m.goto(`/campaigns/${campaignId}/bestiary`);
    await m.getByRole('searchbox', { name: 'Nome' }).fill('bandit');
    await expect(m.locator('.list__n')).toContainText('de 334 criaturas');
    await expectScreenPasses(m, `Bestiário com "Pôr no combate" ${where}`);
    await m.getByRole('button', { name: 'Pôr no combate: Bandido' }).click();
    const sheet = m.getByRole('dialog', { name: 'Pôr no combate' });
    await expect(sheet.locator('.readonly')).toHaveText('Nenhum combate aberto');
    await expectScreenPasses(m, `Pôr no combate, sem combate aberto ${where}`);
    await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
    await sheet.locator('.seg__item', { hasText: 'Rolar' }).click();
    await expectScreenPasses(m, `Pôr no combate, três e rolar ${where}`);
    await sheet.getByRole('button', { name: 'Cancelar' }).click();

    // A combat in preparation: the sheet reads it.
    // eslint-disable-next-line meurpg/rpc-revision-wait -- the master is on the bestiary, which has no combat view to show a revision: the sheet reads the combat when it opens.
    let enc = await startEncounterRPC(m, table, [{ characterId: table.goblinId, count: 1, hidden: false }]);
    await m.getByRole('button', { name: 'Pôr no combate: Bandido' }).click();
    await expect(sheet.locator('.readonly')).toHaveText('Emboscada na estrada · em preparação');
    await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
    await sheet.getByRole('button', { name: 'Mais um Bandido' }).click();
    await expectScreenPasses(m, `Pôr no combate, em preparação ${where}`);
    // A refusal says it in words.
    await m.route('**/meurpg.play.v1.CombatService/AddMonsters', (route) =>
      route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ code: 'invalid_argument', message: 'a combat has at most 40 combatants' }) }),
    );
    await sheet.getByRole('button', { name: 'Pôr 3 no combate' }).click();
    await expect(sheet.getByRole('alert')).toContainText('40 combatentes');
    await expectScreenPasses(m, `Pôr no combate, recusado ${where}`);
    await m.unroute('**/meurpg.play.v1.CombatService/AddMonsters');
    await sheet.getByRole('button', { name: 'Pôr 3 no combate' }).click();
    await expect(m.locator('.put-done')).toContainText('Entraram no combate');
    await expectScreenPasses(m, `Bestiário, o aviso dos monstros ${where}`);

    // The combat under way: the master's order with the monsters, the player's with the word of the state.
    for (const c of (await getEncounterRPC(m, campaignId)).combatants) {
      enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: 10 });
    }
    enc = await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
    const second = enc.combatants.find((c) => c.label === 'Bandido 2')!;
    await combatRPC(m, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: second.id, hidden: false });
    await openSessionPage(m, campaignId);
    await openOrderRow(m, 'Bandido 3');
    await m.getByRole('button', { name: 'Mais ações para Bandido 3' }).click();
    await expect(m.getByRole('menuitem', { name: /ND 1\/8/ })).toBeVisible();
    await m.keyboard.press('Escape');
    await expectScreenPasses(m, `Combate com monstros, a ordem do mestre ${where}`);
    await openSessionPage(p, campaignId);
    await expect(p.getByText('Bandido 2').first()).toBeVisible();
    await expectScreenPasses(p, `Combate com monstros, o jogador ${where}`);

    // They fall and the combat ends: the XP by each ND.
    const now = await getEncounterRPC(m, campaignId);
    for (const c of now.combatants.filter((x) => x.kind !== 'COMBATANT_KIND_PLAYER')) {
      await combatRPC(m, 'AdjustCombatantHitPoints', { campaignId, encounterId: enc.id, combatantId: c.id, damage: 999 });
    }
    await combatRPC(m, 'EndEncounter', { campaignId, encounterId: enc.id });
    await m.goto(`/campaigns/${campaignId}/session`);
    await expect(m.getByRole('region', { name: 'Experiência do combate' }).locator('.kind').first()).toContainText('ND 1/8');
    await expectScreenPasses(m, `Fim do combate, o XP por ND ${where}`);

    // The builder: empty, then filled, above high, with an NPC, and a failed measure.
    await m.goto(`/campaigns/${campaignId}/encounters`);
    await expect(m.locator('.head-xp')).toContainText('Baixa · 0 de');
    await expectScreenPasses(m, `Encontros, vazio ${where}`);
    const add = async (search: string, namePt: string) => {
      await m.getByRole('combobox', { name: 'Adicionar criatura' }).fill(search);
      await expect(m.getByRole('option').first()).toBeVisible();
      await expectScreenPasses(m, `Encontros, a busca de "${search}" ${where}`);
      await m.getByRole('option').filter({ has: m.locator('.pick__pt', { hasText: new RegExp(`^${namePt}$`) }) }).first().click();
    };
    await add('goblin', 'Goblin');
    await add('ogre', 'Ogro');
    await m.getByRole('button', { name: 'Mais um Ogro' }).click();
    await expect(m.locator('.total__n')).toContainText('950 XP');
    await expectScreenPasses(m, `Encontros, montado ${where}`);
    for (let i = 0; i < 4; i++) {
      await m.getByRole('button', { name: 'Mais um Ogro' }).click();
    }
    await expect(m.locator('.head-xp')).toContainText('Acima de alta');
    await expectScreenPasses(m, `Encontros, ${await m.locator('.head-xp').innerText()} ${where}`);

    await m.getByRole('button', { name: 'Pôr um NPC no grupo' }).click();
    const npc = m.getByRole('dialog', { name: 'Pôr um NPC no grupo' });
    await expect(npc.locator('.budget__n')).toContainText('Baixa');
    await expectScreenPasses(m, `Pôr um NPC no grupo ${where}`);
    await npc.getByRole('radio', { name: 'Só um nome' }).check({ force: true });
    await npc.getByRole('button', { name: 'Pôr no grupo' }).click();
    await expect(npc.getByText('Dê um nome ao NPC do grupo.')).toBeVisible();
    await expectScreenPasses(m, `Pôr um NPC no grupo, sem nome ${where}`);
    await npc.getByRole('radio', { name: /Orin, o guia/ }).check({ force: true });
    await npc.getByRole('button', { name: 'Pôr no grupo' }).click();
    await expect(m.locator('.chip', { hasText: 'Orin, o guia' })).toBeVisible();
    await expectScreenPasses(m, `Encontros, com um NPC no grupo ${where}`);

    await m.getByRole('button', { name: 'Gerar encontro' }).click();
    const gen = m.getByRole('dialog', { name: 'Gerar encontro' });
    await expect(gen.locator('.res__seed')).toContainText('Semente');
    await expectScreenPasses(m, `Gerar encontro ${where}`);
    await gen.locator('.line').first().getByRole('button', { name: /^Trocar criatura/ }).click();
    await expect(gen.locator('.swap__opt').first()).toBeVisible();
    await gen.locator('.swap__opt').first().click();
    await expectScreenPasses(m, `Trocar criatura ${where}`);
    await m.getByRole('button', { name: /^Trocar por/ }).click();
    await gen.getByRole('button', { name: 'Usar este encontro' }).click();

    // "Guardar no ponto de batalha": free, the question in place, and the empty point list of another map.
    await m.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
    const save = m.getByRole('dialog', { name: 'Guardar no ponto de batalha' });
    await expect(save.locator('.pt').first()).toBeVisible();
    await expectScreenPasses(m, `Guardar no ponto de batalha ${where}`);
    await save.locator('.pt', { hasText: 'Emboscada na ponte' }).click();
    await save.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
    await expect(m.locator('.mr-notice--success')).toContainText('Encontro guardado em “Emboscada na ponte”.');
    await expectScreenPasses(m, `Encontros, guardado ${where}`);
    await m.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
    await save.locator('.pt', { hasText: 'Emboscada na ponte' }).click();
    await save.getByRole('button', { name: 'Guardar no ponto de batalha' }).click();
    await expect(save.getByText('Trocar o encontro guardado?')).toBeVisible();
    await expectScreenPasses(m, `Guardar, a pergunta no lugar ${where}`);
    await save.getByRole('button', { name: 'Voltar' }).click();
    await save.getByRole('button', { name: 'Cancelar' }).click();

    // A failed measure.
    await m.route('**/meurpg.play.v1.EncounterService/EvaluateEncounter', (route) =>
      route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) }),
    );
    await m.locator('app-count-stepper button[aria-label^="Mais um"]').first().click();
    await expect(m.getByRole('alert').first()).toContainText('o servidor não respondeu');
    await expectScreenPasses(m, `Encontros, medição que falhou ${where}`);
    await m.unroute('**/meurpg.play.v1.EncounterService/EvaluateEncounter');

    // The session: the card of the point and "Iniciar combate" filled from it.
    await openSessionPage(m, campaignId, 'mapa');
    // The ended combat is still on the page: back to the session, where the launch and the saved encounter are.
    await m.getByRole('button', { name: 'Voltar à sessão' }).click();
    await showMapTool(m, 'Encontros');
    const card = m.locator('.enc', { hasText: 'Emboscada na ponte' });
    await expect(card).toBeVisible();
    await expectScreenPasses(m, `Sessão, o encontro guardado ${where}`);
    await card.getByRole('button', { name: 'Começar este combate' }).click();
    const start = m.getByRole('dialog', { name: 'Iniciar combate' });
    await expect(start.locator('.mon__row').first()).toBeVisible();
    await expectScreenPasses(m, `Começar este combate, o Iniciar combate preenchido ${where}`);
    await start.getByRole('button', { name: 'Cancelar' }).click();
    expect(battle).not.toBe('');

    // A player: the builder is the master's.
    await p.goto(`/campaigns/${campaignId}/encounters`);
    await expect(p.getByText('Só o mestre monta encontros.')).toBeVisible();
    await expectScreenPasses(p, `Encontros, o aviso do jogador ${where}`);
  } finally {
    await context.close();
    await playerContext.close();
  }
}

test('os monstros e os encontros passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-042', '@MR-043'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMonsterScreens(browser, 'light', 1280);
});

test('os monstros e os encontros passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-042', '@MR-043'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMonsterScreens(browser, 'dark', 390);
});

test('os monstros e os encontros passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-042', '@MR-043'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMonsterScreens(browser, 'dark', 1024);
});

test('os monstros e os encontros passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-042', '@MR-043'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMonsterScreens(browser, 'light', 320);
});

test('os monstros e os encontros passam no axe e nas conferências de layout no tema escuro, no celular de 320', { tag: ['@a11y', '@MR-042', '@MR-043'] }, async ({ browser }) => {
  test.setTimeout(600_000);
  await scanMonsterScreens(browser, 'dark', 320);
});


/**
 * The treasure generator (Etapa 10, slice 10.17c: MR-044, MR-041, RN-09, RN-10; E10-10 states 1 to 6): nothing generated, a hoard, an
 * individual treasure, a failing and a busy generator, an item's description, "Pôr no mapa" (on a map with rooms, and on a map without a
 * grid), the confirmation, a gold campaign's line and the player's notice. On a phone the item and "Pôr no mapa" are sheets.
 */
async function scanTreasureScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const context = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const playerContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await context.newPage();
  const p = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForMaps(m, p, `Acessibilidade tesouro ${Date.now()}`);
    const campaignId = table.campaignId;
    const made = await callRPC(m, 'meurpg.maps.v1.DungeonService/CreateDungeonMap', { campaignId, name: 'A masmorra do teste', seed: '48213', options: { width: 31, height: 21, roomSideMin: 3, roomSideMax: 9 } });
    expect(made.ok(), await made.text()).toBeTruthy();
    const route = treasureRoute(campaignId);
    const generate = m.getByRole('button', { name: 'Gerar tesouro' });
    const dialogOrSheet = m.locator('mat-dialog-container, mat-bottom-sheet-container').last();

    await open(m, `/campaigns/${campaignId}/prep`);
    await expect(m.getByRole('link', { name: 'Gerar tesouro' })).toBeVisible();
    await expectScreenPasses(m, `Campanha, "Gerar tesouro" no painel Mapas ${where}`);

    await open(m, route);
    await expect(m.getByText('Nada gerado ainda.')).toBeVisible();
    await expectScreenPasses(m, `Tesouro, nada gerado ${where}`);

    // Busy: the server does not answer.
    await m.route('**/*TreasureService/GenerateTreasure', () => new Promise(() => undefined));
    await generate.click();
    await expect(m.getByRole('button', { name: 'Gerando...' })).toBeVisible();
    await expectScreenPasses(m, `Tesouro, gerando ${where}`);
    await m.unroute('**/*TreasureService/GenerateTreasure');

    // Failing.
    await m.route('**/*TreasureService/GenerateTreasure', (r) => r.fulfill({ status: 503, contentType: 'application/json', body: '{"code":"unavailable","message":"x"}' }));
    await open(m, route);
    await m.getByRole('button', { name: 'Gerar tesouro' }).click();
    await expect(m.getByRole('alert')).toContainText('o servidor não respondeu');
    await expectScreenPasses(m, `Tesouro, com falha ${where}`);
    await m.unroute('**/*TreasureService/GenerateTreasure');

    await m.getByRole('button', { name: 'Tentar de novo' }).first().click();
    await expect(m.getByTestId('treasure-seed')).toBeVisible();
    await expectScreenPasses(m, `Tesouro, o covil ${where}`);

    // An item's description.
    await m.locator('.item__desc').first().click();
    await expect(m.getByText('Texto do SRD 5.1 (tradução nossa)')).toBeVisible();
    await expectScreenPasses(m, `Tesouro, a descrição de um item ${where}`);
    await m.getByRole('button', { name: 'Ver em inglês' }).click();
    await expect(m.getByText('Texto do SRD 5.1 (em inglês)')).toBeVisible();
    await expectScreenPasses(m, `Tesouro, a descrição de um item, em inglês ${where}`);
    await m.keyboard.press('Escape');

    // "Pôr no mapa", on the dungeon (rooms), then on a map without a grid.
    const png = await canvasPng(m, 640, 400, 'Sem grade');
    const imageId = await uploadImageRPC(m, campaignId, 'Sem grade', png);
    await createMapRPC(m, campaignId, 'Mapa sem grade', imageId);
    await m.getByRole('button', { name: 'Pôr no mapa' }).click();
    await expect(dialogOrSheet.getByRole('heading', { name: 'Pôr no mapa' })).toBeVisible();
    await expect(dialogOrSheet.getByText(width < 768 ? 'Em que sala?' : /Onde\s+Na Sala/)).toBeVisible();
    if (width >= 768) {
      await expect(dialogOrSheet.locator('.mv__img')).toBeVisible();
      await expect.poll(() => dialogOrSheet.locator('.mv__img').evaluate((img: HTMLImageElement) => img.complete && img.naturalWidth > 0)).toBe(true);
    }
    await expectScreenPasses(m, `Tesouro, "Pôr no mapa" ${where}`);
    await dialogOrSheet.locator('select[data-field=map]').selectOption({ label: 'Mapa sem grade · sem grade' });
    await expect(dialogOrSheet.getByTestId('no-grid')).toBeVisible();
    await expectScreenPasses(m, `Tesouro, "Pôr no mapa" num mapa sem grade ${where}`);
    await dialogOrSheet.locator('select[data-field=map]').selectOption({ label: 'A masmorra do teste' });
    await expect(dialogOrSheet.getByTestId('no-grid')).toHaveCount(0);
    await dialogOrSheet.getByRole('button', { name: 'Pôr no mapa' }).click();
    await expect(m.getByTestId('treasure-placed')).toBeVisible();
    await expectScreenPasses(m, `Tesouro, posto no mapa ${where}`);

    await m.locator('.seg__item', { hasText: 'Individual' }).first().click();
    await generate.click();
    await expect(m.getByRole('heading', { name: /Tesouro individual/ })).toBeVisible();
    await expectScreenPasses(m, `Tesouro, o individual ${where}`);

    // A gold campaign: the line says the gold is converted in "Voltar à cidade".
    const gold = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Estrada de Ouro ${Date.now()}`, xpMode: 'XP_MODE_GOLD' });
    expect(gold.ok()).toBeTruthy();
    await open(m, treasureRoute((await gold.json()).campaign.id as string));
    await m.getByRole('button', { name: 'Gerar tesouro' }).click();
    await expect(m.getByTestId('treasure-gold-line')).toContainText('converte isto em XP');
    await expectScreenPasses(m, `Tesouro, numa campanha por ouro ${where}`);

    // A player: a calm notice.
    await open(p, route);
    await expect(p.getByText('Só o mestre gera o tesouro da campanha.')).toBeVisible();
    await expectScreenPasses(p, `Tesouro, o aviso do jogador ${where}`);
  } finally {
    await context.close();
    await playerContext.close();
  }
}

for (const [scheme, width, label] of [
  ['light', 1280, 'tema claro, no desktop'],
  ['dark', 1024, 'tema escuro, no desktop de 1024'],
  ['dark', 390, 'tema escuro, no celular'],
  ['light', 320, 'tema claro, no celular de 320'],
] as const) {
  test(`o gerador de tesouro passa no axe e nas conferências de layout no ${label}`, { tag: ['@a11y', '@MR-044'] }, async ({ browser }) => {
    test.setTimeout(600_000);
    await scanTreasureScreens(browser, scheme, width);
  });
}

/**
 * The class and subclass editors (MR-025, RN-23; E10-02 states 1 to 4): the class page with its sections, the 20-level grid, a
 * feature open, the question before the table is replaced, a refused cell, the subclass of a third caster and the always-prepared
 * spells; on a phone the master only reads (the table as a list of levels). The entries come through the API.
 */
async function scanTableClasses(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(240_000);
  const mContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height: 900 } });
  const pContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height: 900 } });
  try {
    const m = await mContext.newPage();
    const p = await pContext.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Acessibilidade classes ${Date.now()}`);
    const where = `(${colorScheme}, ${width}px)`;
    const half = halfCasterBody();
    (half.levels as Record<string, unknown>[])[0].features = [
      { namePt: 'Vigília', descPt: ['Você marca um lugar e o vigia.'], effects: [{ type: 'resource', resource: 'vigilia', max: '3', recharge: 'long_rest' }] },
    ];
    const guardiao = await createClassRPC(m, campaignId, half);
    await createClassRPC(m, campaignId, classBody('Bardo das Cinzas'));
    const tinta = await createSubclassRPC(m, campaignId, {
      namePt: 'Tradição da Tinta',
      classKey: 'class:fighter',
      casting: { kind: 'third', ability: 'ABILITY_INTELLIGENCE', preparation: 'known', listFrom: 'class:wizard', startLevel: 3 },
      levels: Array.from({ length: 18 }, (_, i) => ({ level: i + 3, cantripsKnown: 2, spellsKnown: 3, slots: [i < 4 ? 2 : 3, 0, 0, 0, 0, 0, 0, 0, 0] })),
      alwaysPrepared: [{ classLevel: 3, spellKey: 'spell:detect-magic' }],
    });

    if (width >= 1024) {
      await open(m, entryRoute(campaignId, guardiao));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Guardião do Vale');
      await expectScreenPasses(m, `Editor de classe ${where}`);
      await m.getByRole('button', { name: 'Vigília' }).first().click();
      await expect(m.getByLabel('Nome da característica')).toHaveValue('Vigília');
      await expectScreenPasses(m, `Editor de classe, uma característica aberta ${where}`);
      await m.getByRole('button', { name: 'Mostrar os 9 níveis de magia' }).click();
      await expectScreenPasses(m, `Editor de classe, os 9 níveis de magia ${where}`);
      // A refused cell: a half caster has no cantrips at level 1.
      await m.getByLabel('Nível 1, truques').fill('5');
      await m.getByRole('button', { name: 'Salvar classe' }).click();
      await expect(m.getByLabel('Nível 1, truques')).toHaveAttribute('aria-invalid', 'true');
      await expectScreenPasses(m, `Editor de classe, a célula recusada ${where}`);
      await m.getByLabel('Nível 1, truques').fill('');
      // The question before an edited table is replaced.
      await m.getByLabel('Nível 5, espaços de magia de 1º nível').fill('9');
      await m.locator('label', { hasText: 'Completa' }).first().click();
      await expect(m.getByRole('alertdialog', { name: 'Refazer a tabela dos 20 níveis?' })).toBeVisible();
      await expectScreenPasses(m, `Editor de classe, a pergunta antes de refazer a tabela ${where}`);
      await m.getByRole('button', { name: 'Manter a minha tabela' }).click();
      await open(m, `/campaigns/${campaignId}/content/new/class`);
      await expectScreenPasses(m, `Editor de classe, uma classe nova ${where}`);
      await open(m, entryRoute(campaignId, tinta));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Tradição da Tinta');
      await expectScreenPasses(m, `Editor de subclasse de um terço ${where}`);
      await open(m, `/campaigns/${campaignId}/content/new/subclass`);
      await expectScreenPasses(m, `Editor de subclasse, uma nova ${where}`);
    } else {
      await open(m, entryRoute(campaignId, guardiao));
      await expect(m.getByText('Para criar ou editar, abra o conteúdo da mesa no notebook.')).toBeVisible();
      await expectScreenPasses(m, `Classe lida pelo mestre no celular ${where}`);
      await open(m, entryRoute(campaignId, tinta));
      await expectScreenPasses(m, `Subclasse lida pelo mestre no celular ${where}`);
    }

    await open(p, `/campaigns/${campaignId}/content?kind=classes`);
    await expectScreenPasses(p, `Classes, vistas por um jogador ${where}`);
    await open(p, entryRoute(campaignId, guardiao));
    await expect(p.getByText('Testes de resistência', { exact: true })).toBeVisible();
    await expectScreenPasses(p, `Classe, vista por um jogador ${where}`);
  } finally {
    await Promise.all([mContext.close(), pContext.close()]);
  }
}

test('as classes da mesa passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanTableClasses(browser, 'light', 1280);
});

test('as classes da mesa passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-025'] }, async ({ browser }) => {
  await scanTableClasses(browser, 'dark', 1024);
});

test('as classes da mesa passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanTableClasses(browser, 'dark', 390);
});

test('as classes da mesa passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-025'] }, async ({ browser }) => {
  await scanTableClasses(browser, 'light', 320);
});

// MR-025, RN-23, MR-040 (slice 10.12b): the character editor with the table's content (the "Da mesa" tags, the
// "Outro" background, a sheet of several classes and its spell step per class), the level-up with a table class
// and "A classe mudou" on the sheet, with its sheet "O que mudou".
async function scanTableSheetScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number, height = 900): Promise<void> {
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const context = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height } });
  const m = await masterContext.newPage();
  const page = await context.newPage();
  // A click on something that is not there fails in 20 s with its name, not at the end of the test.
  page.setDefaultTimeout(20_000);
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await page.goto('/');
    const table = await emptyTable(m, page, `Acessibilidade da mesa ${Date.now()}`);
    campaignId = table.campaignId;
    const guardian = await createGuardianRPC(m, campaignId);
    await createOwlRaceRPC(m, campaignId);
    await createPlainSubclassRPC(m, campaignId, 'class:wizard', 'Tradição da Tinta', 2);
    await createPlainSubclassRPC(m, campaignId, 'class:cleric', 'Domínio do Caminho', 1);
    const characterId = await createSheetRPC(page, campaignId, icaroSheetBody(guardian));

    // The editor: a table class, race and the "Outro" background.
    const pick = async (label: string, option: string | RegExp) => {
      const control = page.getByRole('combobox', { name: label, exact: true });
      await control.focus();
      await control.press('Enter');
      await page.getByRole('option', { name: option }).click();
    };
    await open(page, `/campaigns/${campaignId}/characters/new`);
    await page.getByLabel('Nome do personagem', { exact: true }).fill('Davi');
    await pick('Raça', /^Corujeiro/);
    await pick('Classe', /^Guardião do Vale/);
    await expect(page.getByText('O Guardião do Vale escolhe a subclasse no nível 3.')).toBeVisible();
    await expectScreenPasses(page, `Editor com a classe e a raça da mesa ${where}`);
    // The open list, with the tag on the table's entries.
    const classSelect = page.getByRole('combobox', { name: 'Classe', exact: true });
    await classSelect.focus();
    await classSelect.press('Enter');
    await expect(page.getByRole('option', { name: /^Guardião do Vale/ })).toContainText('Da mesa');
    await expectScreenPasses(page, `Editor, a lista de classes aberta ${where}`);
    await page.keyboard.press('Escape');
    await pick('Antecedente', 'Outro (personalizado)');
    await expect(page.getByRole('heading', { name: 'Personalizar um antecedente' })).toBeVisible();
    await expectScreenPasses(page, `Editor, o antecedente Outro ${where}`);

    // Perícias: the class's count from the server's entry.
    await page.getByRole('tab', { name: 'Perícias' }).click();
    await expect(page.getByText('O Guardião do Vale escolhe 2 perícias')).toBeVisible();
    await expectScreenPasses(page, `Editor, a perícias da classe ${where}`);

    // Several classes.
    await open(page, `/campaigns/${campaignId}/characters/new`);
    await page.getByLabel('Nome do personagem', { exact: true }).fill('Corvina');
    await pick('Classe', 'Mago');
    await page.getByLabel('Nível', { exact: true }).fill('3');
    await page.getByRole('button', { name: 'Adicionar classe' }).click();
    const second = page.locator('app-class-block').nth(1);
    const secondClass = second.getByRole('combobox', { name: 'Classe', exact: true });
    await secondClass.focus();
    await secondClass.press('Enter');
    await page.getByRole('option', { name: 'Clérigo' }).click();
    await second.getByLabel('Nível', { exact: true }).fill('1');
    await expect(page.locator('app-class-block')).toHaveCount(2);
    await expectScreenPasses(page, `Editor, dois blocos de classe ${where}`);
    const secondSub = second.getByRole('combobox', { name: 'Subclasse', exact: true });
    await secondSub.focus();
    await secondSub.press('Enter');
    await expect(page.getByRole('option', { name: /^Domínio do Caminho/ })).toContainText('Da mesa');
    await expectScreenPasses(page, `Editor, a subclasse da mesa aberta ${where}`);
    await page.keyboard.press('Escape');
    await page.getByRole('tab', { name: 'Magias' }).click();
    await expect(page.getByRole('heading', { name: 'Clérigo · até o 1º nível de magia' })).toBeVisible();
    await page.locator('.spell-section').nth(1).getByPlaceholder('Buscar magia').fill('amizade');
    await expect(page.locator('.picker__out')).toBeVisible();
    await expectScreenPasses(page, `Editor, as magias de cada classe e uma fora da lista ${where}`);

    // The level-up with the table class: the session locks the sheet. An open session keeps a stream going, so
    // the screens from here on are loaded by their heading, not by a quiet network.
    await lockAndMilestone(m, campaignId, characterId);
    const openLive = async (route: string) => {
      await page.goto(route);
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
    };
    await openLive(`/campaigns/${campaignId}/characters/${characterId}/level-up`);
    await passClassStep(page);
    await expect(page.getByText(/Passo 2 de \d · Vida/)).toBeVisible();
    await expectScreenPasses(page, `Subir de nível, a vida ${where}`);
    await page.getByRole('button', { name: 'Próximo' }).click();
    await expect(page.getByText(/Escolhas/).first()).toBeVisible();
    await page.locator('#pick-feature-0').getByRole('radio').first().click();
    await expectScreenPasses(page, `Subir de nível, as escolhas ${where}`);
    await page.getByRole('button', { name: 'Próximo' }).click();
    const prepare = page.locator('#pick-prepared');
    await expect(prepare).toBeVisible();
    await showAllPicks(prepare);
    // How many to prepare: the list's own "0 de 2", drawn with its rows (the footer's sentence follows it a repaint later).
    await expect(prepare.locator('.list__count')).toContainText(/\d+ de \d+/);
    const [had, of] = ((await prepare.locator('.list__count').textContent()) ?? '').match(/\d+/g)!.map(Number);
    for (const name of ['Amizade Animal', 'Bom Fruto', 'Criar ou Destruir Água', 'Curar Ferimentos'].slice(0, of - had)) {
      await prepare.getByRole('checkbox', { name: new RegExp(`^${name}`) }).check();
    }
    await expectScreenPasses(page, `Subir de nível, as magias ${where}`);
    await page.getByRole('button', { name: 'Próximo' }).click();
    const slots = page.getByRole('region', { name: 'O que muda', exact: true }).locator('li').filter({ hasText: 'Espaços de 1º nível' });
    await expect(slots).toContainText('Da mesa');
    await expectScreenPasses(page, `Subir de nível, o resumo com "Da mesa" ${where}`);

    // The third caster's Magias step at level 3 (the master's subclass casts from the wizard's list).
    const campaignD = (await emptyTable(m, page, `Acessibilidade do conjurador ${Date.now()}`)).campaignId;
    await createInkBladeSubclassRPC(m, campaignD);
    const fighter = icaroSheetBody('class:fighter');
    fighter.name = 'Rúnico';
    (fighter.sheet as any).full.classes = [{ classKey: 'class:fighter', level: 2 }];
    (fighter.sheet as any).full.experiencePoints = 900;
    (fighter.sheet as any).full.skillProficiencyKeys = ['skill:athletics', 'skill:perception'];
    const runico = await createSheetRPC(page, campaignD, fighter);
    await lockAndMilestone(m, campaignD, runico);
    await openLive(`/campaigns/${campaignD}/characters/${runico}/level-up`);
    await passClassStep(page);
    await page.getByRole('button', { name: 'Próximo' }).click();
    await page.locator('#pick-subclass').getByRole('radio', { name: /Lâmina de Tinta/ }).click();
    await page.getByRole('button', { name: 'Próximo' }).click();
    await expect(page.getByText(/truques de Mago/)).toBeVisible();
    await expectScreenPasses(page, `Subir de nível, as magias do conjurador de um terço ${where}`);

    // The edit of a sheet whose domain always prepares a spell: "Sempre preparadas", in the class's section.
    const campaignB = (await emptyTable(m, page, `Acessibilidade das sempre preparadas ${Date.now()}`)).campaignId;
    const domain = await createPlainSubclassRPC(m, campaignB, 'class:cleric', 'Domínio do Caminho', 1, [{ classLevel: 1, spellKey: 'spell:detect-magic' }]);
    const cleric = await createSheetRPC(page, campaignB, {
      kind: 'CHARACTER_KIND_PLAYER',
      name: 'Clara',
      sheet: { full: {
        baseScores: { strength: 10, dexterity: 12, constitution: 14, intelligence: 10, wisdom: 15, charisma: 8 },
        raceKey: 'race:gnome',
        classes: [{ classKey: 'class:cleric', level: 3, subclassKey: domain }],
        backgroundKey: 'background:acolyte',
        hitPoints: { method: 'HIT_POINTS_METHOD_AVERAGE' },
        preparedSpellKeys: ['spell:bless'],
      } },
    });
    await openLive(`/campaigns/${campaignB}/characters/${cleric}/edit`);
    await page.getByRole('tab', { name: 'Magias' }).click();
    await expect(page.locator('.granted')).toContainText('Sempre preparada');
    await expect(page.getByText(/Preparadas \d+ de \d+/)).toBeVisible();
    await expectScreenPasses(page, `Editar, as magias sempre preparadas ${where}`);

    // "A classe mudou" on the sheet, and its sheet.
    await changeGuardianSkillsRPC(m, campaignId, guardian, 1);
    await openLive(`/campaigns/${campaignId}/characters/${characterId}`);
    await expect(page.getByText('A classe mudou.')).toBeVisible();
    await expectScreenPasses(page, `Ficha com "A classe mudou" ${where}`);
    await page.getByRole('button', { name: 'Ver o que mudou' }).click();
    await expect(page.getByRole('heading', { name: 'O que mudou: Guardião do Vale' })).toBeVisible();
    await expectScreenPasses(page, `"O que mudou", a folha ${where}`);
  } finally {
    if (campaignId) await endOpenSessionRPC(m, campaignId);
    await masterContext.close();
    await context.close();
  }
}

test('o editor com a mesa, a subida de nível e "A classe mudou" passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-025', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanTableSheetScreens(browser, 'light', 1280);
});

test('o editor com a mesa, a subida de nível e "A classe mudou" passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-025', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanTableSheetScreens(browser, 'dark', 390);
});

test('o editor com a mesa, a subida de nível e "A classe mudou" passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-025', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanTableSheetScreens(browser, 'light', 320, 568);
});

test('o editor com a mesa, a subida de nível e "A classe mudou" passam no axe e nas conferências de layout no tema escuro, no celular de 320', { tag: ['@a11y', '@MR-025', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanTableSheetScreens(browser, 'dark', 320, 568);
});

test('o editor com a mesa, a subida de nível e "A classe mudou" passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-025', '@MR-040'] }, async ({ browser }) => {
  test.setTimeout(360_000);
  await scanTableSheetScreens(browser, 'dark', 1024, 768);
});

/**
 * "Opções para os jogadores" (MR-025, RN-23; E10-01 state 3): the master's switches by kind (the races with one off and one
 * used by a sheet, the subclasses of an off class, the search with nothing found, the bulk buttons), the entry's own switch in
 * the spell editor, and what a player reads when the master turned something off (the race list on a phone).
 */
async function scanOptions(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(180_000);
  const mContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport: { width, height: 900 } });
  const pContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: { width, height: 900 } });
  try {
    const m = await mContext.newPage();
    const p = await pContext.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const { campaignId } = await tableForMaps(m, p, `Acessibilidade opções ${Date.now()}`);
    const where = `(${colorScheme}, ${width}px)`;
    const spell = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));
    const race = await createEntryRPC(m, campaignId, 'tableRace', raceBody());
    await setSwitchesRPC(m, campaignId, [
      { key: 'race:tiefling', off: true },
      { key: 'class:cleric', off: true },
      { key: spell, off: true },
      { key: race, off: false },
    ]);

    await open(m, `/campaigns/${campaignId}/content/options?kind=races`);
    await expect(m.getByRole('switch', { name: 'Tiefling' })).toHaveAttribute('aria-checked', 'false');
    await expectScreenPasses(m, `Opções para os jogadores, as raças ${where}`);
    await m.getByRole('switch', { name: 'Gnomo', exact: true }).click();
    await expect(m.getByText('Tudo salvo')).toBeVisible();
    await expectScreenPasses(m, `Opções para os jogadores, uma ficha usa a raça desligada ${where}`);
    await open(m, `/campaigns/${campaignId}/content/options?kind=subclasses`);
    await expect(m.getByText('Some para os jogadores: a classe Clérigo está desligada.').first()).toBeVisible();
    await expectScreenPasses(m, `Opções para os jogadores, as subclasses de uma classe desligada ${where}`);
    await open(m, `/campaigns/${campaignId}/content/options?kind=spells`);
    if (colorScheme === 'light' && width === 1280) {
      // Once, the whole list: the 320 spells of the SRD and the table's own, as the master reads it.
      await expect(m.locator('.orow')).toHaveCount(320);
      await expectScreenPasses(m, `Opções para os jogadores, a lista inteira das magias ${where}`);
    }
    await m.getByLabel('Buscar pelo nome').fill('zzzz');
    await expect(m.getByText('Nenhuma opção com esta busca.')).toBeVisible();
    await expectScreenPasses(m, `Opções para os jogadores, a busca sem resultado ${where}`);
    await open(m, `/campaigns/${campaignId}/content`);
    await expectScreenPasses(m, `Conteúdo da mesa, com o caminho para as opções ${where}`);
    if (width >= 768) {
      await open(m, entryRoute(campaignId, spell));
      await expect(m.getByRole('switch', { name: 'Disponível para os jogadores' })).toHaveAttribute('aria-checked', 'false');
      await expectScreenPasses(m, `Editor de magia, "Disponível para os jogadores" desligado ${where}`);
    }

    // What a player reads: the race list without the Tiefling, on the screen where the master's phone is small.
    await open(p, `/campaigns/${campaignId}/content`);
    await expectScreenPasses(p, `Conteúdo da mesa do jogador, com opções desligadas ${where}`);
    await open(p, `/campaigns/${campaignId}/spells?q=nanquim`);
    await expect(p.getByText('Nenhuma magia com “nanquim”.')).toBeVisible();
    await expectScreenPasses(p, `Magias do jogador, a magia desligada não aparece ${where}`);
    await open(p, `/campaigns/${campaignId}/content/options`);
    await expect(p.getByText('Só o mestre escolhe o que os jogadores veem.')).toBeVisible();
    await expectScreenPasses(p, `Opções para os jogadores, o que um jogador vê ${where}`);
  } finally {
    await Promise.all([mContext.close(), pContext.close()]);
  }
}

test('as opções para os jogadores passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanOptions(browser, 'light', 1280);
});

test('as opções para os jogadores passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanOptions(browser, 'dark', 1024);
});

test('as opções para os jogadores passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanOptions(browser, 'dark', 390);
});

test('as opções para os jogadores passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-025', '@RN-23'] }, async ({ browser }) => {
  await scanOptions(browser, 'light', 320);
});

/** The campaign package (MR-050): the export page (idle, running through a
 * fulfilled `GetCampaignExport`, done with "Baixar de novo") and the import
 * page (choose, a file refused before upload, preview, refused, newer
 * version, done). The package is a real one, exported and fetched by the
 * test; the running export is the one state too quick to catch for real. */
async function scanPackage(browser: Browser, colorScheme: 'light' | 'dark', width: number, owner: TestUser): Promise<void> {
  const context = await browser.newContext({
    storageState: authStatePath(owner),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const page = await context.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const input = page.getByLabel('Arquivo do pacote da campanha');
  try {
    await page.goto('/');
    const table = await tableForPackage(page, `Acessibilidade pacote ${Date.now()}`);

    await open(page, `/campaigns/${table.campaignId}/setup`);
    await expect(page.getByRole('link', { name: 'Exportar campanha' })).toBeVisible();
    await expectScreenPasses(page, `Campanha com o painel Pacote da campanha ${where}`);

    await open(page, `/campaigns/${table.campaignId}/export`);
    await expect(page.getByRole('button', { name: 'Exportar campanha' })).toBeVisible();
    await expectScreenPasses(page, `Exportar campanha, antes de exportar ${where}`);

    const running = '**/meurpg.campaignpackage.v1.CampaignPackageService/GetCampaignExport';
    await page.route(running, (route) =>
      route.fulfill({
        contentType: 'application/json',
        json: { export: { id: 'a11y', state: 'CAMPAIGN_EXPORT_STATE_RUNNING', percent: 40 }, estimatedBytes: '4096' },
      }),
    );
    await page.reload();
    await expect(page.getByRole('progressbar', { name: 'Progresso da exportação' })).toBeVisible();
    await expectScreenPasses(page, `Exportar campanha, exportando ${where}`);
    await page.unroute(running);

    await exportThroughScreen(page, table.campaignId);
    await expectLoaded(page);
    await expectScreenPasses(page, `Exportar campanha, pacote pronto ${where}`);
    const href = await page.getByRole('link', { name: 'Baixar de novo' }).getAttribute('href');
    const fetched = await page.request.get(href!);
    expect(fetched.ok()).toBeTruthy();
    const zip = await fetched.body();

    await open(page, '/campaigns');
    await expect(page.getByRole('link', { name: 'Importar campanha' })).toBeVisible();
    await expectScreenPasses(page, `Minhas campanhas com Importar campanha ${where}`);

    await open(page, '/campaigns/import');
    await expect(page.getByRole('button', { name: 'Escolher o arquivo' })).toBeVisible();
    await expectScreenPasses(page, `Importar campanha, escolher o arquivo ${where}`);

    await input.setInputFiles({ name: 'notas.txt', mimeType: 'text/plain', buffer: Buffer.from('não é um pacote') });
    await expect(page.getByRole('alert').filter({ hasText: 'Esse arquivo não parece um pacote de campanha.' })).toBeVisible();
    await expectScreenPasses(page, `Importar campanha, arquivo recusado ${where}`);

    await input.setInputFiles({ name: 'falso.meurpg.zip', mimeType: 'application/zip', buffer: Buffer.from('só tem o nome') });
    await expect(page.getByRole('strong').filter({ hasText: 'Este pacote não pode ser criado.' })).toBeVisible({ timeout: 60_000 });
    await expectScreenPasses(page, `Importar campanha, pacote recusado ${where}`);

    await page.getByRole('button', { name: 'Escolher outro arquivo' }).click();
    await input.setInputFiles({ name: 'futuro.meurpg.zip', mimeType: 'application/zip', buffer: zipWithManifest({ format_version: 2 }) });
    await expect(page.getByRole('strong').filter({ hasText: 'Este pacote é de uma versão mais nova do MeuRPG' })).toBeVisible({ timeout: 60_000 });
    await expectScreenPasses(page, `Importar campanha, versão mais nova ${where}`);

    await page.getByRole('button', { name: 'Escolher outro arquivo' }).click();
    await input.setInputFiles({ name: 'pacote.meurpg.zip', mimeType: 'application/zip', buffer: zip });
    await expect(page.getByRole('button', { name: 'Criar campanha' })).toBeVisible({ timeout: 60_000 });
    await expectLoaded(page);
    await expectScreenPasses(page, `Importar campanha, prévia ${where}`);

    await page.getByRole('button', { name: 'Criar campanha' }).click();
    await expect(page.getByRole('heading', { level: 2, name: 'Campanha criada' })).toBeVisible({ timeout: 60_000 });
    await expectScreenPasses(page, `Importar campanha, campanha criada ${where}`);

    // The player's side of the export page: only the master exports.
    const playerContext = await newSignedInContext(browser, 'Mestre Teste', { colorScheme, viewport: { width, height: 900 } });
    try {
      const player = await playerContext.newPage();
      await player.goto('/');
      await joinAsPlayer(page, player, table.campaignId);
      await open(player, `/campaigns/${table.campaignId}/export`);
      await expect(player.getByText('Só o mestre exporta a campanha.')).toBeVisible();
      await expectScreenPasses(player, `Exportar campanha, vista do jogador ${where}`);
    } finally {
      await playerContext.close();
    }
  } finally {
    await context.close();
  }
}

test('o pacote da campanha passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-050'] }, async ({ browser }) => {
  test.slow();
  // One import upload per account at a time: each scan, and campaign-package.spec.ts, imports as its own account.
  await scanPackage(browser, 'light', 1280, 'Jogador Teste');
});

test('o pacote da campanha passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-050'] }, async ({ browser }) => {
  test.slow();
  await scanPackage(browser, 'dark', 390, 'E-mail Não Verificado');
});

/**
 * The reserved characters and their claim links (MR-049, PM-09): the master's list with a row in each state of the link, a
 * question asked in place, the dialog that makes the link (before and after), the editor of a reserved character, and the page
 * a player opens with the link: signed out (the same for every link), the card, the page of a link that cannot be used and the
 * master's own link.
 */
async function scanClaims(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(150_000);
  const view = { colorScheme, viewport: { width, height: 900 } } as const;
  // The master's "Copiar link" writes to the clipboard, which a headless browser allows only when it is granted.
  const mContext = await browser.newContext({ storageState: authStatePath('Mestre Teste'), permissions: ['clipboard-read', 'clipboard-write'], ...view });
  const pContext = await browser.newContext({ storageState: authStatePath('Jogador Teste'), ...view });
  const gContext = await browser.newContext({ storageState: { cookies: [], origins: [] }, ...view });
  const where = `(${colorScheme}, ${width}px)`;
  try {
    const m = await mContext.newPage();
    const p = await pContext.newPage();
    const g = await gContext.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const created = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Acessibilidade reservados ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
    expect(created.ok()).toBeTruthy();
    const campaignId = (await created.json()).campaign.id as string;
    const kai = await reservedRPC(m, campaignId, 'Kai');
    const salvia = await reservedRPC(m, campaignId, 'Sálvia');
    const ragna = await reservedRPC(m, campaignId, 'Ragna');
    const brisa = await reservedRPC(m, campaignId, 'Brisa');
    const salviaToken = await linkRPC(m, campaignId, salvia);
    await linkRPC(m, campaignId, ragna);
    await revokeLinkRPC(m, campaignId, ragna);
    const kaiToken = await linkRPC(m, campaignId, kai);
    const brisaToken = await linkRPC(m, campaignId, brisa);
    // The player takes Brisa, so the list has "Assumido por".
    await p.goto(claimRoute(brisaToken));
    await p.getByRole('button', { name: 'Assumir este personagem' }).click();
    await expect(p.getByRole('heading', { level: 1 })).toHaveText('Pronto: Brisa é seu');

    // The master's list: no link, sent, revoked and claimed, one row each.
    await open(m, `/campaigns/${campaignId}`);
    await expect(rowOf(m, 'Sálvia')).toContainText('Link enviado');
    await expect(rowOf(m, 'Ragna')).toContainText('Link revogado');
    await expect(rowOf(m, 'Brisa')).toContainText('Assumido por');
    await expectScreenPasses(m, `Personagens reservados ${where}`);
    await rowOf(m, 'Sálvia').getByRole('button', { name: /Revogar o link de Sálvia/ }).click();
    await expect(rowOf(m, 'Sálvia').getByRole('group', { name: 'Revogar o link de Sálvia?' })).toBeVisible();
    await expectScreenPasses(m, `Personagens reservados, revogar o link ${where}`);
    await rowOf(m, 'Sálvia').getByRole('button', { name: 'Cancelar' }).click();
    await rowOf(m, 'Brisa').getByRole('button', { name: /Devolver à reserva: Brisa/ }).click();
    await expect(rowOf(m, 'Brisa').getByRole('group', { name: 'Devolver Brisa à reserva?' })).toBeVisible();
    await expectScreenPasses(m, `Personagens reservados, devolver à reserva ${where}`);
    await rowOf(m, 'Brisa').getByRole('button', { name: 'Cancelar' }).click();
    await rowOf(m, 'Ragna').getByRole('button', { name: 'Excluir Ragna' }).click();
    await expect(rowOf(m, 'Ragna').getByRole('group', { name: 'Excluir Ragna?' })).toBeVisible();
    await expectScreenPasses(m, `Personagens reservados, excluir ${where}`);
    await rowOf(m, 'Ragna').getByRole('button', { name: 'Cancelar' }).click();

    // The dialog: the validity, then the link shown once.
    await rowOf(m, 'Ragna').getByRole('button', { name: /Gerar novo link/ }).click();
    const dialog = m.getByRole('dialog', { name: 'Gerar link para o jogador' });
    await expect(dialog.getByRole('radio', { name: '7 dias' })).toBeChecked();
    await expectScreenPasses(m, `Gerar link para o jogador ${where}`);
    await dialog.getByRole('button', { name: 'Gerar link' }).click();
    await expect(dialog.locator('.claim-link__field')).toBeVisible();
    await expectScreenPasses(m, `Gerar link para o jogador, o link ${where}`);
    await dialog.getByRole('button', { name: 'Copiar link' }).click();
    await expect(dialog.getByText('Link copiado.')).toBeVisible();
    await expectScreenPasses(m, `Gerar link para o jogador, link copiado ${where}`);

    await open(m, `/campaigns/${campaignId}/reserved/new`);
    await expect(m.getByText('Personagem reservado.')).toBeVisible();
    await expectScreenPasses(m, `Criar personagem para um jogador ${where}`);

    // The player's pages: the card, the page of a link that cannot be used, the master's own link and the signed-out page.
    await p.goto(claimRoute(kaiToken));
    await expect(p.getByRole('heading', { level: 1 })).toHaveText('Este personagem é seu?');
    await expectScreenPasses(p, `Link do personagem, o cartão ${where}`);
    await p.goto(claimRoute('x'));
    await expect(p.getByRole('heading', { level: 1 })).toHaveText('Este link não pode ser usado');
    await expectScreenPasses(p, `Link do personagem, o link que não vale ${where}`);
    await m.goto(claimRoute(salviaToken));
    await expect(m.getByRole('heading', { level: 1 })).toHaveText('Este link é para um jogador');
    await expectScreenPasses(m, `Link do personagem, o link do próprio mestre ${where}`);
    await g.goto(claimRoute(salviaToken));
    await expect(g.getByRole('heading', { level: 1 })).toHaveText('Assumir um personagem');
    await expect(g.getByRole('button', { name: 'Entrar com Google' })).toBeVisible();
    await expectScreenPasses(g, `Link do personagem, sem entrar ${where}`);
  } finally {
    await Promise.all([mContext.close(), pContext.close(), gContext.close()]);
  }
}

test('os reservados e os links passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-049'] }, async ({ browser }) => {
  await scanClaims(browser, 'light', 1280);
});

test('os reservados e os links passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-049'] }, async ({ browser }) => {
  await scanClaims(browser, 'dark', 1024);
});

test('os reservados e os links passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-049'] }, async ({ browser }) => {
  await scanClaims(browser, 'dark', 390);
});

test('os reservados e os links passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-049'] }, async ({ browser }) => {
  await scanClaims(browser, 'light', 320);
});

/**
 * "Sessões anteriores" (PM-01): the campaign page's panel with a long list (the master's, the player's, and a
 * player's before any session ended, where it does not exist), the summary page of the master (with a combat's
 * table, without it) and of a player, the neighbours' links, a number that does not exist, and the generic page.
 */
async function scanPastSessions(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width <= 390 ? 700 : 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableWithPensantus(m, p, `Acessibilidade sessões anteriores ${Date.now()}`);
    campaignId = table.campaignId;
    const route = `/campaigns/${campaignId}`;

    // Nothing ended yet: the master's empty box, and no panel for the player.
    await open(m, route);
    await openFoldedPanel(m, 'Sessões anteriores');
    await expect(m.getByRole('region', { name: 'Sessões anteriores' })).toContainText('Nenhuma sessão encerrada ainda');
    await expectScreenPasses(m, `Sessões anteriores, vazio do mestre ${where}`);
    await open(p, route);
    await expect(p.getByRole('region', { name: 'Sessões anteriores' })).toHaveCount(0);
    await expectScreenPasses(p, `Sessões anteriores, o jogador sem nenhuma ${where}`);

    // Six ended sessions: five show and "Mostrar as outras 1".
    for (let i = 0; i < 6; i++) {
      await endSessionRPC(m, campaignId, await startSessionRPC(m, campaignId));
    }
    await open(m, route);
    await openFoldedPanel(m, 'Sessões anteriores');
    const panel = (page: Page) => page.getByRole('region', { name: 'Sessões anteriores' });
    // A phone says "Sessões anteriores · 6"; a wider screen "6 encerradas".
    await expect(panel(m)).toContainText(width < 768 ? '· 6' : '6 encerradas');
    await expect(panel(m).getByRole('button', { name: 'Mostrar as outras 1' })).toBeVisible();
    await expectScreenPasses(m, `Sessões anteriores, lista do mestre ${where}`);
    await panel(m).getByRole('button', { name: 'Mostrar as outras 1' }).click();
    await expect(panel(m).getByRole('link', { name: 'Ver resumo da Sessão 1' })).toBeFocused();
    await expectScreenPasses(m, `Sessões anteriores, a lista inteira ${where}`);
    await open(p, route);
    await expect(panel(p)).toContainText('6 encerradas');
    await expectScreenPasses(p, `Sessões anteriores, lista do jogador ${where}`);

    // The summary page, as the master and as the player, and the pager.
    await open(m, `${route}/sessions/3`);
    await expect(m.getByRole('heading', { name: 'Em números' })).toBeVisible();
    await expect(m.getByRole('navigation', { name: 'Outras sessões' })).toContainText('Sessão 4');
    await expectScreenPasses(m, `O resumo da sessão, mestre ${where}`);
    await m.getByRole('navigation', { name: 'Outras sessões' }).getByRole('link', { name: 'Sessão 4' }).focus();
    await expectScreenPasses(m, `O resumo da sessão, foco na sessão vizinha ${where}`);
    await open(p, `${route}/sessions/3`);
    await expect(p.getByRole('heading', { name: 'A sessão acabou' })).toBeVisible();
    await expectScreenPasses(p, `O resumo da sessão, jogador ${where}`);

    // A number that does not exist, and the page for anyone who may not know.
    await open(m, `${route}/sessions/9`);
    await expect(m.getByRole('heading', { level: 1, name: 'Não há Sessão 9' })).toBeVisible();
    await expectScreenPasses(m, `O resumo da sessão, número que não existe ${where}`);
    await open(p, `${route}/sessions/abc`);
    await expect(p.getByRole('heading', { level: 1, name: 'Página não encontrada' })).toBeVisible();
    await expectScreenPasses(p, `O resumo da sessão, página genérica ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as sessões anteriores e o resumo passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanPastSessions(browser, 'light', 1280);
});

test('as sessões anteriores e o resumo passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanPastSessions(browser, 'dark', 390);
});

test('as sessões anteriores e o resumo passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanPastSessions(browser, 'light', 320);
});

/** "Reviver" (RN-03, PM-08d states 1 and 5): the master's dead page with "Reviver", the question in place, the refusal because the player
 * made another character, and the owner's dead page with "E agora?". */
async function scanReviveScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(120_000);
  const context = await browser.newContext({
    storageState: authStatePath('Mestre Teste'),
    colorScheme,
    viewport: { width, height: 900 },
  });
  const playerContext = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport: { width, height: 900 } });
  const page = await context.newPage();
  const playerPage = await playerContext.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await page.goto('/');
    const created = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', {
      name: `Acessibilidade reviver ${Date.now()}`,
      xpMode: 'XP_MODE_ENEMIES',
    });
    const campaignId = (await created.json()).campaign.id as string;
    const invite = await callRPC(page, 'meurpg.campaigns.v1.CampaignService/CreateInvite', {
      campaignId,
      maxUses: 1,
      expiresIn: '86400s',
      requiresApproval: false,
    });
    const { token } = await invite.json();
    await playerPage.goto('/');
    const accepted = await callRPC(playerPage, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token });
    expect(accepted.ok()).toBeTruthy();
    const character = await createCharacterRPC(playerPage, campaignId, characterRpcBody('PLAYER', pensantus));
    const characterId = (await character.json()).character.id as string;
    const dead = await callRPC(page, 'meurpg.characters.v1.CharacterService/MarkCharacterDead', { campaignId, characterId });
    expect(dead.status()).toBe(200);
    const route = `/campaigns/${campaignId}/characters/${characterId}`;

    await open(page, route);
    await expect(page.getByRole('button', { name: 'Reviver', exact: true })).toBeVisible();
    await expectScreenPasses(page, `Reviver, a ficha do morto ${where}`);
    await page.getByRole('button', { name: 'Reviver', exact: true }).click();
    await expect(page.getByRole('button', { name: `Reviver ${pensantus.name}` })).toBeFocused();
    await expectScreenPasses(page, `Reviver, a confirmação ${where}`);

    await open(playerPage, route);
    await expect(playerPage.getByRole('heading', { name: 'E agora?' })).toBeVisible();
    await expectScreenPasses(playerPage, `O jogador do morto, "E agora?" ${where}`);

    const replacement = await createCharacterRPC(playerPage, campaignId, characterRpcBody('PLAYER', { ...pensantus, name: 'Personagem Novo' }));
    expect(replacement.status()).toBe(200);
    await page.getByRole('button', { name: `Reviver ${pensantus.name}` }).click();
    await expect(page.getByRole('alert').filter({ hasText: 'já tem outro personagem vivo' })).toBeVisible();
    await expectScreenPasses(page, `Reviver, o jogador já tem outro personagem ${where}`);
  } finally {
    await playerContext.close();
    await context.close();
  }
}

test('reviver passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-03'] }, async ({ browser }) => {
  await scanReviveScreens(browser, 'light', 1280);
});

test('reviver passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-03'] }, async ({ browser }) => {
  await scanReviveScreens(browser, 'dark', 390);
});

/** The dead of a combat and the master's question about Revivify (RN-03, RN-10; PM-08d states 2b and 4): "Mortos nesta luta" under the
 * order with the tag, the switch off and on (the notice only the master reads), and the dialog of a cast outside a combat waiting for him. */
async function scanDeathsScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(180_000);
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade mortos ${Date.now()}`, true, true);
    const { campaignId } = table;
    const enc = await beginAttackCombatRPC(m, table, { 'Capitão Goblin': 20, 'Goblin 1': 12, 'Goblin 2': 3, Pensantus: 15 });
    const fallen = enc.combatants.find((c) => c.label === 'Goblin 1')!;
    await combatRPC(m, 'AdjustCombatantHitPoints', { campaignId, encounterId: enc.id, combatantId: fallen.id, damage: 999 });

    await openSessionPage(m, campaignId);
    const block = m.getByRole('region', { name: /^Mortos nesta luta/ });
    await expect(block).toContainText('Mortos nesta luta (1)');
    await expect(block).toContainText('Cabe em Revivificar');
    await expect(block.getByRole('button', { name: 'Reviver' })).toHaveCount(0);
    await expectScreenPasses(m, `Mortos nesta luta ${where}`);

    await block.getByRole('switch', { name: 'Revivificar não funciona nesta morte' }).click();
    await expect(block).toContainText('Revivificar não vai aparecer para ele.');
    await expect(block).not.toContainText('Cabe em Revivificar');
    await expectScreenPasses(m, `Mortos nesta luta, o interruptor ligado ${where}`);

    // A cast outside a combat waits for the master: the dialog with the focus on its first button.
    const cast = {
      requests: [
        {
          id: crypto.randomUUID(),
          status: 'REVIVIFY_REQUEST_STATUS_PENDING',
          casterName: 'Ilaria',
          targetName: 'Toren',
          createdAt: new Date().toISOString(),
        },
      ],
    };
    await m.route('**/meurpg.play.v1.RevivifyService/ListRevivifyRequests', (r) =>
      r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(cast) }),
    );
    await openSessionPage(m, campaignId);
    const ask = m.getByRole('alertdialog', { name: 'Ilaria quer conjurar Revivificar em Toren' });
    await expect(ask).toBeVisible();
    await expect(ask.getByRole('button', { name: 'Faz menos de 1 minuto' })).toBeFocused();
    await expectScreenPasses(m, `Revivificar fora de combate, a pergunta do mestre ${where}`);
    await m.unroute('**/meurpg.play.v1.RevivifyService/ListRevivifyRequests');
  } finally {
    await player.close();
    await master.close();
  }
}

test('os mortos da luta e a pergunta de Revivificar passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-03', '@RN-10'] }, async ({ browser }) => {
  await scanDeathsScreens(browser, 'light', 1280);
});

test('os mortos da luta e a pergunta de Revivificar passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-03', '@RN-10'] }, async ({ browser }) => {
  await scanDeathsScreens(browser, 'dark', 390);
});

/** Revivificar (RN-03, RN-10; PM-08d states 2, 2b and 3): the sheet's three steps in a combat (the target, the diamonds not ticked and
 * ticked, the result), outside a combat (the target with the master's confirmation, the wait, his "no") and the notice the revived
 * character's player reads. */
async function scanRevivifyScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(300_000);
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  const sheetOf = (page: Page) => page.getByRole('dialog', { name: /^Revivificar/ });
  const campaigns = [] as string[];
  try {
    await m.goto('/');
    await p.goto('/');

    // In a combat.
    const fight = await combatWithTwoDown(m, p, `Acessibilidade revivificar ${Date.now()}`);
    campaigns.push(fight.table.campaignId);
    await openSessionPage(p, fight.table.campaignId);
    await openActionTab(p, 'Magias');
    await p.getByRole('button', { name: 'Conjurar Revivificar' }).click();
    let sheet = sheetOf(p);
    await expect(sheet.getByRole('radiogroup', { name: 'Quem morreu por perto' })).toContainText('Goblin 1');
    await expectScreenPasses(p, `Revivificar, o alvo ${where}`);
    await sheet.getByRole('button', { name: 'Próximo' }).click();
    await expect(sheet.getByRole('heading', { name: 'Revivificar em Goblin 1' })).toBeFocused();
    await expectScreenPasses(p, `Revivificar, confirmar sem os diamantes ${where}`);
    await sheet.getByText('Tenho os diamantes', { exact: true }).click();
    await expectScreenPasses(p, `Revivificar, confirmar com os diamantes ${where}`);
    await sheet.getByRole('button', { name: 'Conjurar Revivificar' }).click();
    await expect(sheet.getByRole('status').filter({ hasText: 'Goblin 1 voltou à vida.' })).toBeVisible();
    await expectScreenPasses(p, `Revivificar, o resultado ${where}`);
    await endOpenSessionRPC(m, fight.table.campaignId);

    // Outside a combat.
    const dead = await tableWithDead(m, p, `Acessibilidade revivificar fora ${Date.now()}`, true);
    campaigns.push(dead.campaignId);
    await openSessionPage(m, dead.campaignId);
    await openSessionPage(p, dead.campaignId);
    await openSheetTab(p);
    await p.getByRole('button', { name: 'Revivificar', exact: true }).click();
    sheet = sheetOf(p);
    await expect(sheet).toContainText('O mestre confirma que faz menos de 1 minuto.');
    await expectScreenPasses(p, `Revivificar fora de combate, o alvo ${where}`);
    await sheet.getByRole('button', { name: 'Próximo' }).click();
    await sheet.getByText('Tenho os diamantes', { exact: true }).click();
    await sheet.getByRole('button', { name: 'Conjurar Revivificar' }).click();
    await expect(sheet.getByRole('status').filter({ hasText: 'Esperando o mestre' })).toBeVisible();
    await expectScreenPasses(p, `Revivificar fora de combate, esperando o mestre ${where}`);
    await m.getByRole('alertdialog', { name: 'Ilaria quer conjurar Revivificar em Toren' }).getByRole('button', { name: 'Já passou' }).click();
    await expect(sheet.getByRole('status').filter({ hasText: 'O mestre disse que não dá.' })).toBeVisible();
    await expectScreenPasses(p, `Revivificar fora de combate, o mestre disse que não ${where}`);
    await endOpenSessionRPC(m, dead.campaignId);

    // The notice of the one who lives again.
    const only = await tableWithDead(m, p, `Acessibilidade voltou ${Date.now()}`, false);
    campaigns.push(only.campaignId);
    await openSessionPage(p, only.campaignId);
    await reviveRPC(m, only.campaignId, only.deadId);
    await expect(p.getByRole('status').filter({ hasText: 'Você voltou à vida.' })).toContainText('Está com 1 PV.');
    await expectScreenPasses(p, `Você voltou à vida ${where}`);
  } finally {
    for (const id of campaigns) {
      await endOpenSessionRPC(m, id);
    }
    await player.close();
    await master.close();
  }
}

test('Revivificar passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-03', '@RN-10'] }, async ({ browser }) => {
  await scanRevivifyScreens(browser, 'light', 1280);
});

test('Revivificar passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-03', '@RN-10'] }, async ({ browser }) => {
  await scanRevivifyScreens(browser, 'dark', 390);
});

test('Revivificar passa no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@RN-03', '@RN-10'] }, async ({ browser }) => {
  await scanRevivifyScreens(browser, 'light', 320);
});

/** Rolls with a mode (PM-06): the attack sheet with the server's mode and its reasons, the
 * two d20, the damage extras and the parts, the master's queue of requests and his card
 * with the damage by part. */
async function scanRollModeScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const { m, p, campaignId, done } = await rollsTable(
    browser,
    `Acessibilidade vantagem ${Date.now()}`,
    vex,
    { weaponKeys: ['equipment:rapier'] },
    { Vex: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 },
    { colorScheme, playerWidth: width, masterWidth: width },
  );
  try {
    const enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toBeVisible();

    // A better mode than the suggestion is asked of the master: the pending request, on both screens.
    await p.getByRole('button', { name: /^Atacar com Rapieira/ }).click();
    await viaTargetList(p, p.getByRole('dialog'));
    await p.locator('label', { hasText: 'Goblin 1' }).click();
    await expectScreenPasses(p, `Atacar, o modo normal e o pedido ${where}`);
    await p.locator('.radio__word', { hasText: /^Vantagem$/ }).click();
    await p.getByLabel(/Motivo/).fill('Ele está distraído');
    await expectScreenPasses(p, `Atacar, pedir vantagem ao mestre ${where}`);
    await p.getByRole('button', { name: /Pedir ao mestre/ }).click();
    await expect(p.getByText(/Aguardando o mestre/)).toBeVisible();
    await expectScreenPasses(p, `Atacar, aguardando o mestre ${where}`);
    await expect(m.getByRole('button', { name: 'Aprovar' })).toBeVisible();
    await expectScreenPasses(m, `Fila do mestre, pedido de vantagem ${where}`);
    await m.getByRole('button', { name: 'Aprovar' }).click();

    // The approved mode: the two d20 are typed, the extra is marked, the parts are typed.
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expectScreenPasses(p, `Atacar, os dois d20 ${where}`);
    await p.getByLabel(/1º|Primeiro/).first().fill('4');
    await p.getByLabel(/2º|Segundo/).first().fill('17');
    await p.getByRole('button', { name: /Confirmar/ }).click();
    await expect(p.getByText(/descartado/).first()).toBeVisible();
    await expectScreenPasses(p, `Atacar, os dois d20 e o dano a rolar ${where}`);
    const extra = p.getByRole('checkbox', { name: /Ataque Furtivo/ });
    if (await extra.isEnabled()) {
      await extra.check();
    }
    await expectScreenPasses(p, `Atacar, as partes do dano ${where}`);

    // The master's card for an NPC that took the hit: the damage by part.
    await combatRPC(m, 'SetCombatantConditions', { campaignId, encounterId: enc.id, combatantId: id('Goblin 1'), conditions: { keys: ['condition:restrained'] } });
  } finally {
    await done().catch(() => undefined);
  }
}

test('rolar com vantagem passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@PM-06a', '@PM-06b'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRollModeScreens(browser, 'light', 1280);
});

test('rolar com vantagem passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@PM-06a', '@PM-06b'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRollModeScreens(browser, 'dark', 390);
});

/** The master's pending questions of the held rolls (RN-35, RN-18): in a combat, the question inside the row of the roller with
 * the chip that counts them; outside a combat, the panel above the stage. The roller answers first, so each scan waits until
 * the master's buttons are enabled (the settled state), never for a fixed time. */
async function scanHeldQuestionScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const { m, p, campaignId, done } = await rollsTable(
    browser,
    `Acessibilidade pergunta pendente ${Date.now()}`,
    pipo,
    { weaponKeys: ['equipment:battleaxe'] },
    { Pipo: 20, 'Goblin 1': 15, 'Capitão Goblin': 5, 'Goblin 2': 4 },
    { colorScheme, playerWidth: width, masterWidth: width },
  );
  try {
    const enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const swing = await callRPC(p, 'meurpg.play.v1.CombatService/RollAttack', {
      campaignId, encounterId: enc.id, attackerId: id('Pipo'), attackKey: 'equipment:battleaxe', targetId: id('Goblin 1'), idempotencyKey: crypto.randomUUID(), d20Face: 1,
    });
    expect(swing.ok(), await swing.text()).toBeTruthy();
    await openSessionPage(m, campaignId);
    const question = m.getByRole('region', { name: 'Tirou 1: rolar de novo?' });
    await expect(question.getByRole('button', { name: 'Ficar com o 1' })).toBeEnabled({ timeout: 30_000 });
    await expectScreenPasses(m, `Pergunta pendente na ordem do combate, o mestre pode responder ${where}`);
    await question.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(question.getByLabel(/Número que saiu no d20/)).toBeVisible();
    await expectScreenPasses(m, `Pergunta pendente, digitando o resultado ${where}`);
  } finally {
    await done().catch(() => undefined);
  }
}

test('as perguntas pendentes na ordem do combate passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014', '@RN-35'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanHeldQuestionScreens(browser, 'light', 1280);
});

test('as perguntas pendentes na ordem do combate passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014', '@RN-35'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanHeldQuestionScreens(browser, 'dark', 390);
});

/** The same questions outside a combat: the "Perguntas pendentes" panel above the stage (a grid on a computer, one open card and
 * a row for each other question on a phone). */
async function scanHeldPanelScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const options = { colorScheme, viewport: { width, height: 900 } };
  const master = await browser.newContext({ ...options, storageState: authStatePath('Mestre Teste') });
  const player = await browser.newContext({ ...options, storageState: authStatePath('Jogador Teste') });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForScenes(m, p, `Acessibilidade perguntas ${Date.now()}`, true, { raceKey: 'race:halfling', subraceKey: 'subrace:lightfoot-halfling' });
    campaignId = table.campaignId;
    await openSceneRPC(m, campaignId, table.cartId);
    const open = await getOpenSceneRPC(p, campaignId);
    await rollSceneRPC(p, campaignId, open.scene!.actions[0].id, 1);
    await openSessionPage(m, campaignId);
    const panel = m.getByRole('region', { name: 'Perguntas pendentes' });
    await expect(panel.getByRole('button', { name: 'Ficar com o 1' })).toBeEnabled({ timeout: 30_000 });
    await expectScreenPasses(m, `Perguntas pendentes fora do combate ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o painel de perguntas pendentes fora do combate passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014', '@MR-015', '@RN-35'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanHeldPanelScreens(browser, 'light', 1280);
});

test('o painel de perguntas pendentes fora do combate passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014', '@MR-015', '@RN-35'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanHeldPanelScreens(browser, 'dark', 390);
});

/** The rage (PM-07): the chip on the order and the sheet, the damage with its resistance step on the
 * master's card, and the question when the rage is about to end. */
async function scanRageScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const { m, p, campaignId, done } = await rollsTable(
    browser,
    `Acessibilidade fúria ${Date.now()}`,
    grog,
    { weaponKeys: ['equipment:greataxe'] },
    { Grog: 20, 'Goblin 1': 15, 'Capitão Goblin': 5, 'Goblin 2': 4 },
    { colorScheme, playerWidth: width, masterWidth: width },
  );
  try {
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Grog' })).toBeVisible();
    // Grog is a Berserker: Fúria asks about the frenzy first, in place (SRD 5.1, Frenzy).
    await openActionTab(p, 'Bárbaro');
    await p.getByRole('button', { name: /Fúria/ }).click();
    await expect(p.getByRole('group', { name: 'Entrar em frenesi?' })).toBeVisible();
    await expectScreenPasses(p, `Entrar em frenesi? ${where}`);
    await p.getByRole('button', { name: 'Só fúria' }).click();
    await expect(p.getByText('Em fúria').first()).toBeVisible();
    await expectScreenPasses(p, `Sua vez, em fúria ${where}`);

    // Ending the turn without attacking or taking damage asks before the rage ends (after the
    // question about the free action).
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    await expect(p.getByText('Ainda tem ação disponível. Encerrar mesmo?')).toBeVisible();
    await p.getByRole('button', { name: /^Encerrar/ }).last().click();
    const question = p.getByRole('alertdialog', { name: /A sua fúria vai acabar/ });
    await expect(question).toBeVisible();
    await expect(question.getByRole('button', { name: 'Voltar e atacar' })).toBeFocused();
    await expectScreenPasses(p, `A fúria vai acabar? ${where}`);
    await question.getByRole('button', { name: 'Voltar e atacar' }).click();

    // He swings, the turn passes, and the goblin's blow shows the resistance on the master's card.
    const enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const swing = await callRPC(p, 'meurpg.play.v1.CombatService/RollAttack', {
      campaignId, encounterId: enc.id, attackerId: id('Grog'), attackKey: 'equipment:greataxe', targetId: id('Goblin 1'), idempotencyKey: crypto.randomUUID(), d20Face: 2,
    });
    expect(swing.ok(), await swing.text()).toBeTruthy();
    const end = await callRPC(p, 'meurpg.play.v1.CombatService/EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: id('Grog'), idempotencyKey: crypto.randomUUID() });
    expect(end.ok(), await end.text()).toBeTruthy();
    await openSessionPage(m, campaignId);
    const card = m;
    await card.getByRole('button', { name: 'Digitar o resultado' }).click();
    await card.getByLabel(/Role 1d20/).fill('20');
    await card.getByRole('button', { name: 'Confirmar 20' }).click();
    await card.getByRole('button', { name: 'Rolar dano' }).click();
    await expect(card.getByText(/Resistência/).first()).toBeVisible();
    await expectScreenPasses(m, `Cartão do mestre, o dano com a resistência da fúria ${where}`);
  } finally {
    await done().catch(() => undefined);
  }
}

test('a fúria passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@PM-07a', '@PM-07b'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRageScreens(browser, 'light', 1280);
});

test('a fúria passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@PM-07a', '@PM-07b'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRageScreens(browser, 'dark', 390);
});

/**
 * "Descansos e dados de vida" (PM-07b 9, 10): the master's rest card (idle, the short and the long rest asking, done, and the
 * long rest's warning once one was taken), and the player's "Gastar dados de vida" sheet (the app and the typed roll, the typed
 * number out of range and valid, a die spent, and no dice left), then the "Recursos" counters on the character sheet.
 */
async function scanRestScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width <= 390 ? 700 : 900 };
  const master = await newSignedInContext(browser, 'Mestre Teste', { colorScheme, viewport });
  const player = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  m.setDefaultTimeout(20_000);
  p.setDefaultTimeout(20_000);
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForLevelUp(m, p, `Acessibilidade descansos ${Date.now()}`, { build: toren, sheet: torenSheet, milestone: false });
    campaignId = table.campaignId;
    // Toren: 10 of 46 hit points, 3 of 5 d10 spent, Second Wind and Action Surge used.
    await spendForRestRPC(m, campaignId, table.characterId);

    // The master's card: idle, then the two questions, each with the focus on the button that was tapped.
    await openSessionPage(m, campaignId, 'grupo');
    const card = m.getByRole('region', { name: 'Descanso' });
    await expect(card.getByRole('button', { name: 'Descanso curto' })).toBeVisible();
    await expectScreenPasses(m, `Descanso, o cartão do mestre ${where}`);
    await card.getByRole('button', { name: 'Descanso curto' }).click();
    const question = card.getByRole('alertdialog', { name: 'Começar o descanso curto?' });
    await expect(question).toContainText('Retomar o Fôlego: Toren 1 de 1');
    await expectScreenPasses(m, `Descanso curto, a confirmação ${where}`);
    await card.getByRole('button', { name: 'Descanso longo' }).click();
    await expect(card.getByRole('alertdialog', { name: 'Começar o descanso longo?' })).toContainText('Metade dos dados de vida');
    await expectScreenPasses(m, `Descanso longo, a confirmação ${where}`);
    await card.getByRole('button', { name: 'Cancelar' }).click();
    await expect(card.getByRole('alertdialog')).toHaveCount(0);

    // The player's sheet of hit dice: the two ways to roll, the typed number, a die spent, no dice left.
    await openSessionPage(p, campaignId);
    await openSheetTab(p);
    await p.getByRole('button', { name: 'Gastar dados de vida' }).click();
    const dice = p.getByRole('dialog', { name: 'Gastar dados de vida' });
    await expect(dice.getByRole('button', { name: 'Digitar o resultado' })).toBeVisible();
    await expectScreenPasses(p, `Gastar dados de vida, escolher como rolar ${where}`);
    await dice.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expectScreenPasses(p, `Gastar dados de vida, número a digitar ${where}`);
    await dice.getByLabel(/Role 1d10/).fill('27');
    await expect(dice.getByRole('alert').filter({ hasText: 'Digite um número de 1 a 10' })).toBeVisible();
    await expectScreenPasses(p, `Gastar dados de vida, número fora de 1 a 10 ${where}`);
    await dice.getByLabel(/Role 1d10/).fill('6');
    await dice.getByRole('button', { name: 'Confirmar 6' }).click();
    await expect(dice.getByRole('list', { name: 'Dados gastos agora' })).toContainText('Rolou 6 + 3 = 9 · recuperou 9 PV');
    await expectScreenPasses(p, `Gastar dados de vida, um dado gasto ${where}`);
    await dice.getByRole('button', { name: /^Rolar d10 no app/ }).click();
    await expect(dice.getByRole('list', { name: 'Dados gastos agora' }).getByRole('listitem')).toHaveCount(2);
    await expect(dice.getByText('Não há dados de vida para gastar.')).toBeVisible();
    await expectScreenPasses(p, `Gastar dados de vida, nenhum dado sobrando ${where}`);
    await dice.locator('button', { hasText: 'Fechar' }).click();
    await expect(p.getByRole('dialog')).toHaveCount(0);

    // The rests taken: the done line, and the long rest's warning once one was taken.
    await card.getByRole('button', { name: 'Descanso curto' }).click();
    await card.getByRole('button', { name: 'Descansar' }).click();
    await expect(card.getByText('Descanso curto feito.').first()).toBeVisible();
    await expectScreenPasses(m, `Descanso curto, feito ${where}`);
    await card.getByRole('button', { name: 'Descanso longo' }).click();
    await card.getByRole('button', { name: 'Descansar' }).click();
    await expect(card.getByText('Descanso longo feito.').first()).toBeVisible();
    await card.getByRole('button', { name: 'Descanso longo' }).click();
    await expect(card.getByText('Já houve um descanso longo nesta sessão.')).toBeVisible();
    await expectScreenPasses(m, `Descanso longo, com o aviso do outro descanso ${where}`);
    await card.getByRole('button', { name: 'Cancelar' }).click();

    // "Recursos" on the character sheet (a fighter has no spell slots), live while the session is open.
    await open(p, `/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(p.getByRole('region', { name: 'Recursos' }).getByRole('listitem').filter({ hasText: 'Retomar o Fôlego' })).toContainText('1 de 1');
    await expectScreenPasses(p, `Ficha, os recursos ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('os descansos e os dados de vida passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-012', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRestScreens(browser, 'light', 1280);
});

test('os descansos e os dados de vida passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-012', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRestScreens(browser, 'dark', 1024);
});

test('os descansos e os dados de vida passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-012', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRestScreens(browser, 'dark', 390);
});

test('os descansos e os dados de vida passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-012', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanRestScreens(browser, 'light', 320);
});

/**
 * "Cura pelas Mãos" and "Inspiração de Bardo" (PM-07c 9 and 12): the paladin's dialog (the touch, the cure, the answer), the
 * bard's gift (the list, the answer), and the question a held roll asks, "Usar a Inspiração de Bardo (d6)?". Aurora (the
 * paladin) is Jogador Teste and Orla (the bard) the other player; the master only passes the turns.
 */
async function scanLayOnHandsAndInspirationScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width <= 390 ? 700 : 900 };
  const master = await newSignedInContext(browser, 'Mestre Teste', { colorScheme, viewport });
  const player = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  m.setDefaultTimeout(20_000);
  p.setDefaultTimeout(20_000);
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  let bard: Awaited<ReturnType<typeof secondPlayer>> | null = null;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade mãos ${Date.now()}`, true, false, { build: aurora, sheet: auroraSheet });
    campaignId = table.campaignId;
    bard = await secondPlayer(browser, m, campaignId, orla, orlaSheet);
    await bard.page.setViewportSize(viewport);
    await bard.page.emulateMedia({ colorScheme });
    const b = bard.page;
    b.setDefaultTimeout(20_000);
    await beginPlacedCombatRPC(
      m,
      table,
      [table.captainId],
      { Orla: 20, Aurora: 15, 'Capitão Goblin': 5 },
      { Orla: [7, 9], Aurora: [8, 9], 'Capitão Goblin': [9, 9] },
    );
    await adjustVitalsRPC(m, campaignId, bard.characterId, { hitPointsCurrent: 3 });
    await openSessionPage(p, campaignId);
    await openSessionPage(b, campaignId);

    // Orla's turn: the gift, the list of who can be inspired, and the answer.
    await expect(b.getByRole('heading', { name: 'Sua vez, Orla' })).toBeVisible();
    await openActionTab(b, 'Bardo');
    await b.getByRole('region', { name: 'O que você pode fazer' }).getByRole('button', { name: /^Usar Inspiração de Bardo/ }).click();
    const gift = b.getByRole('dialog', { name: 'Inspiração de Bardo' });
    await expect(gift.locator('label', { hasText: 'Aurora' })).toBeVisible();
    await expectScreenPasses(b, `Inspiração de Bardo, quem inspirar ${where}`);
    await gift.locator('label', { hasText: 'Aurora' }).click();
    await gift.getByRole('button', { name: 'Inspirar Aurora' }).click();
    await expect(gift.getByText('Você inspirou Aurora.')).toBeVisible();
    await expectScreenPasses(b, `Inspiração de Bardo, dado entregue ${where}`);
    await gift.locator('button', { hasText: 'Fechar' }).click();
    await waitForEncounterRevision(p, await passTurnsTo(m, campaignId, 'Aurora'));

    // Aurora's turn: the die card, and the touch on Orla (hurt).
    await expect(p.getByRole('heading', { name: 'Sua vez, Aurora' })).toBeVisible();
    await expect(p.getByText('Inspiração de Bardo: d6').first()).toBeVisible();
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });
    await expectScreenPasses(p, `Sua vez, com o dado de Inspiração de Bardo ${where}`);
    await openActionTab(p, 'Paladino');
    await groups.getByRole('button', { name: 'Usar Cura pelas Mãos' }).click();
    const hands = p.getByRole('dialog', { name: 'Cura pelas Mãos' });
    await expect(hands.locator('label', { hasText: 'Orla' })).toBeVisible();
    await expectScreenPasses(p, `Cura pelas Mãos, restaurar PV ${where}`);
    await hands.locator('label', { hasText: 'Neutralizar um veneno' }).click();
    await expect(hands.getByRole('button', { name: /^Neutralizar o veneno/ })).toBeVisible();
    await expectScreenPasses(p, `Cura pelas Mãos, neutralizar um veneno ${where}`);
    await hands.locator('label', { hasText: 'Restaurar PV' }).click();
    await hands.locator('label', { hasText: 'Orla' }).click();
    await hands.getByRole('button', { name: 'Mais um ponto' }).click();
    await expect(hands.getByRole('button', { name: 'Curar 6 PV em Orla' })).toBeVisible();
    await expectScreenPasses(p, `Cura pelas Mãos, mais pontos ${where}`);
    await hands.getByRole('button', { name: 'Curar 6 PV em Orla' }).click();
    await expect(hands.locator('.pill')).toContainText('Curou');
    await expectScreenPasses(p, `Cura pelas Mãos, curou ${where}`);
    await hands.locator('button', { hasText: 'Fechar' }).click();

    // The next round: the attack with the die in hand asks before the result.
    await passTurnsTo(m, campaignId, 'Orla');
    await waitForEncounterRevision(p, await passTurnsTo(m, campaignId, 'Aurora'));
    await expect(p.getByRole('heading', { name: 'Sua vez, Aurora' })).toBeVisible();
    await openActionTab(p, 'Ataques');
    await groups.getByRole('button', { name: /^Atacar com Espada/ }).click();
    const attack = p.getByRole('dialog', { name: /Atacar com Espada/ });
    await viaTargetList(p, attack);
    await attack.locator('label', { hasText: 'Capitão Goblin' }).click();
    await p.getByRole('button', { name: 'Digitar o resultado' }).click();
    await p.getByLabel(/Role 1d20/).fill('1');
    await p.getByRole('button', { name: 'Confirmar 1' }).click();
    await expect(p.getByRole('heading', { name: 'Usar a Inspiração de Bardo (d6)?' }).last()).toBeVisible();
    await expectScreenPasses(p, `Usar a Inspiração de Bardo, a pergunta ${where}`);
    await p.getByRole('button', { name: 'Digitar o resultado' }).last().click();
    await expectScreenPasses(p, `Usar a Inspiração de Bardo, digitar o dado ${where}`);
    await p.getByLabel(/Role 1d6/).last().fill('4');
    await p.getByRole('button', { name: 'Confirmar 4' }).last().click();
    await expect(p.getByRole('button', { name: 'Voltar à sua vez' }).last()).toBeVisible();
    await expectScreenPasses(p, `Atacar, resultado com o dado de Inspiração de Bardo ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await bard?.close();
    await master.close();
    await player.close();
  }
}

test('a Cura pelas Mãos e a Inspiração de Bardo passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanLayOnHandsAndInspirationScreens(browser, 'light', 1280);
});

test('a Cura pelas Mãos e a Inspiração de Bardo passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanLayOnHandsAndInspirationScreens(browser, 'dark', 1024);
});

test('a Cura pelas Mãos e a Inspiração de Bardo passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanLayOnHandsAndInspirationScreens(browser, 'dark', 390);
});

test('a Cura pelas Mãos e a Inspiração de Bardo passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanLayOnHandsAndInspirationScreens(browser, 'light', 320);
});

/**
 * "Conjuração Flexível" and "Metamagia" (PM-07c 10 and 11): the sorcerer's dialog (creating a slot, converting one with the points
 * at the maximum, and the answer), the cast sheet with the Metamagic section (the options marked, the grey one with its reason),
 * and the sheet's "Recursos" and "Espaços de magia" with the slot that was created.
 */
async function scanFlexibleCastingScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width <= 390 ? 700 : 900 };
  const master = await newSignedInContext(browser, 'Mestre Teste', { colorScheme, viewport });
  const player = await newSignedInContext(browser, 'Jogador Teste', { colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  m.setDefaultTimeout(20_000);
  p.setDefaultTimeout(20_000);
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade feiticeiro ${Date.now()}`, true, false, { build: nael, sheet: naelSheet });
    campaignId = table.campaignId;
    await beginPlacedCombatRPC(m, table, [table.captainId], { Nael: 20, 'Capitão Goblin': 5 }, { Nael: [8, 9], 'Capitão Goblin': [9, 9] });
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Nael' })).toBeVisible();
    const groups = p.getByRole('region', { name: 'O que você pode fazer' });

    // The cast sheet with Metamagic: the options, one marked, and the grey one with its reason.
    await openActionTab(p, 'Magias');
    await groups.getByRole('button', { name: 'Conjurar Enfeitiçar Pessoa' }).click();
    const cast = p.getByRole('dialog', { name: 'Conjurar Enfeitiçar Pessoa' });
    await viaTargetList(p, cast);
    await cast.locator('label', { hasText: 'Capitão Goblin' }).click();
    await expect(cast.getByRole('heading', { name: /^Metamagia/ })).toBeVisible();
    await expect(cast.getByText('Pontos de Feitiçaria: 3 de 3')).toBeVisible();
    await expectScreenPasses(p, `Conjurar, a Metamagia ${where}`);
    await cast.locator('label', { hasText: 'Magia Cuidadosa' }).click();
    await expectScreenPasses(p, `Conjurar, a Magia Cuidadosa marcada ${where}`);
    await p.keyboard.press('Escape');
    await expect(p.getByRole('dialog')).toHaveCount(0);

    // Flexible Casting: nothing to convert at the maximum of points (the reason is written), then a slot is created.
    await openActionTab(p, 'Feiticeiro');
    await groups.getByRole('button', { name: 'Usar Conjuração Flexível: Criar Espaços de Magia' }).click();
    const flex = p.getByRole('dialog', { name: 'Conjuração Flexível' });
    await expect(flex).toContainText('Pontos de Feitiçaria: 3 de 3');
    await expectScreenPasses(p, `Conjuração Flexível, criar um espaço ${where}`);
    await flex.locator('label', { hasText: 'Espaço → pontos' }).click();
    await expectScreenPasses(p, `Conjuração Flexível, converter um espaço ${where}`);
    await flex.locator('label', { hasText: 'Pontos → espaço' }).click();
    await flex.getByRole('button', { name: /^Criar o espaço de/ }).click();
    await expect(flex.locator('.pill')).toBeVisible();
    await expectScreenPasses(p, `Conjuração Flexível, espaço criado ${where}`);
    await flex.locator('button', { hasText: 'Fechar' }).click();

    // The sheet's counters: Sorcery Points 1 of 3 and a created 1st-level slot.
    await open(p, `/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(p.getByRole('region', { name: 'Recursos' }).getByRole('listitem').filter({ hasText: 'Pontos de Feitiçaria' })).toContainText('1 de 3');
    await expect(p.getByRole('region', { name: 'Espaços de magia' })).toContainText('criado');
    await expectScreenPasses(p, `Ficha, os recursos e os espaços criados ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('a Conjuração Flexível, a Metamagia e os contadores da ficha passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFlexibleCastingScreens(browser, 'light', 1280);
});

test('a Conjuração Flexível, a Metamagia e os contadores da ficha passam no axe e nas conferências de layout no tema escuro, no desktop de 1024', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFlexibleCastingScreens(browser, 'dark', 1024);
});

test('a Conjuração Flexível, a Metamagia e os contadores da ficha passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFlexibleCastingScreens(browser, 'dark', 390);
});

test('a Conjuração Flexível, a Metamagia e os contadores da ficha passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanFlexibleCastingScreens(browser, 'light', 320);
});

// ---- casting outside a combat (MR-048, W7-C) ----

async function scanOutsideCastingScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: width < 700 ? 800 : 900 };
  const contexts = await Promise.all(
    (['Mestre Teste', 'Jogador Teste'] as const).map((user) =>
      browser.newContext({ storageState: authStatePath(user), colorScheme, viewport }),
    ),
  );
  const [m, p] = await Promise.all(contexts.map((c) => c.newPage()));
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await Promise.all([m.goto('/'), p.goto('/')]);
    const table = await tableForCasting(m, p, `Acessibilidade conjuração ${Date.now()}`);
    campaignId = table.campaignId;
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId, 'grupo');
    // The player's "Magias" are in "Ficha" (the rail's tab from 1024 px, the pinned nav's panel on a phone): it stays open for the whole scan.
    await openSheetTab(p);
    await expect(p.locator('app-casting-panel').getByText('Nada foi conjurado nesta sessão ainda.')).toBeVisible();
    await expectScreenPasses(p, `A sessão com as magias, vazia ${where}`);

    // The spell, the slot and the target of Armadura Arcana, then what it did.
    await p.getByRole('button', { name: 'Conjurar fora do combate', exact: true }).click();
    await expect(castSheet(p).getByText('Magia', { exact: true }).first()).toBeVisible();
    await expectScreenPasses(p, `Conjurar, a lista de magias ${where}`);
    await choiceCard(castSheet(p), 'Armadura Arcana').click();
    await expect(castSheet(p).locator('.frame__title')).toHaveText('Armadura Arcana');
    await pickSlotRadio(castSheet(p), '1º nível');
    await pickTargetOf(castSheet(p), 'Pensantus');
    await expectScreenPasses(p, `Conjurar Armadura Arcana, espaço e alvo ${where}`);
    await castSheet(p).getByRole('button', { name: 'Conjurar Armadura Arcana em Pensantus' }).click();
    await expect(castSheet(p).getByText(/CA \d+ \(13 \+ Destreza\)/)).toBeVisible();
    await expectScreenPasses(p, `Conjurar Armadura Arcana, o resultado ${where}`);
    await castSheet(p).getByRole('button', { name: 'Fechar' }).last().click();

    // The spell that lasts, and the danger confirmation of ending it.
    await expect(p.locator('app-casting-panel').getByText('dura 8 horas')).toBeVisible();
    await expectScreenPasses(p, `Magias ativas ${where}`);
    await p.locator('app-casting-panel').getByRole('button', { name: 'Encerrar' }).click();
    await expect(p.locator('app-cast-confirm-sheet').getByText('Encerrar Armadura Arcana de Pensantus?')).toBeVisible();
    await expectScreenPasses(p, `Encerrar a magia, a confirmação ${where}`);
    await p.locator('app-cast-confirm-sheet').getByRole('button', { name: 'Cancelar' }).click();

    // A ritual: the way, the time computed, and the cast waiting for the master.
    const ritual = await openCastOf(p, 'Alarme');
    await pickChoice(ritual, 'Como ritual');
    await expect(ritual.getByText('1 minuto + 10 = 11 minutos')).toBeVisible();
    await expectScreenPasses(p, `Conjurar Alarme como ritual ${where}`);
    await ritual.getByRole('button', { name: 'Começar o ritual' }).click();
    await expect(ritual.getByText(/Um ritual não gasta espaço de magia/)).toBeVisible();
    await expectScreenPasses(p, `Conjurando o ritual ${where}`);
    await ritual.getByRole('button', { name: 'Fechar' }).last().click();
    await expect(p.locator('app-casting-panel').getByText('Esperando o mestre concluir')).toBeVisible();
    await expectScreenPasses(p, `A conjuração esperando o mestre ${where}`);

    // The master's queue.
    await expect(m.locator('app-casting-panel').getByRole('button', { name: 'Concluir conjuração' })).toBeVisible();
    await expectScreenPasses(m, `A fila de conjurações do mestre ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await Promise.all(contexts.map((c) => c.close()));
  }
}

test('as conjurações fora do combate passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-048'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanOutsideCastingScreens(browser, 'light', 1280);
});

test('as conjurações fora do combate passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-048'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanOutsideCastingScreens(browser, 'dark', 390);
});

test('as conjurações fora do combate passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-048'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanOutsideCastingScreens(browser, 'light', 320);
});

// ---- Ajuda, Crítico Brutal, Talento Confiável and the jump that leaves a reach (PM-03a, PM-03b, PM-03c) ----

const sizeOf = (width: number) => ({ width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 });

// Ajuda on a character at 0 hit points: the player's two pills, the awake one with the tag and the banner, the sheet page,
// and the master's row with its menu and the question that ends the spell.
async function scanAidScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = sizeOf(width);
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  let close: () => Promise<void> = async () => undefined;
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade Ajuda ${Date.now()}`, true, true);
    campaignId = table.campaignId;
    const third = await thirdPlayer(browser, { m, p, table, campaignId, done: async () => undefined } satisfies TrapTable, brisa, brisaAidSheet);
    close = third.close;
    const square = sq20(4, 7);
    await placeTokenRPC(m, campaignId, table.mapId, third.characterId, square.xBp, square.yBp);
    await beginAttackCombatRPC(m, table, { Brisa: 20, Pensantus: 15, 'Capitão Goblin': 10, 'Goblin 1': 5, 'Goblin 2': 4 });
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);

    await expect(p.getByRole('status').getByText('Testes contra a morte: 0 sucessos, 0 falhas')).toBeVisible();
    await expectScreenPasses(p, `A 0 PV, as etiquetas Inconsciente e testes contra a morte ${where}`);

    const enc = await getEncounterRPC(m, campaignId);
    const id = (label: string) => enc.combatants.find((c) => c.label === label)!.id;
    const cast = await callRPC(third.page, 'meurpg.play.v1.CombatService/CastSpell', {
      campaignId,
      encounterId: enc.id,
      casterId: id('Brisa'),
      spellKey: 'spell:aid',
      slot: { level: 2 },
      targets: [{ combatantId: id('Pensantus') }],
      idempotencyKey: crypto.randomUUID(),
    });
    expect(cast.ok(), await cast.text()).toBeTruthy();
    await expect(p.getByRole('status').getByText('Acordado · testes contra a morte zerados')).toBeVisible();
    await expectScreenPasses(p, `Ajuda sobre quem estava a 0 PV, acordou ${where}`);

    const order = m.getByRole('region', { name: 'Ordem de iniciativa' });
    await openOrderRow(m, 'Pensantus');
    await expect(order.locator('.row__effect--aid')).toContainText('Ajuda +5 PV');
    await expectScreenPasses(m, `A ordem do mestre com Ajuda ${where}`);
    await order.getByRole('button', { name: 'Mais ações para Pensantus' }).click();
    await expect(m.getByRole('menuitem', { name: 'Encerrar Ajuda em Pensantus' })).toBeVisible();
    await expectScreenPasses(m, `O menu da linha com Encerrar Ajuda ${where}`);
    await m.getByRole('menuitem', { name: 'Encerrar Ajuda em Pensantus' }).click();
    const ask = m.getByRole('alertdialog', { name: /Encerrar a Ajuda d[oa] Pensantus/ });
    await expect(ask).toBeVisible();
    await expectScreenPasses(m, `Encerrar a Ajuda, a pergunta no lugar ${where}`);
    await ask.getByRole('button', { name: 'Cancelar' }).click();

    await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
    await expect(p.locator('app-combat-stats').getByText('5 de 28')).toBeVisible({ timeout: 30_000 });
    await expectLoaded(p);
    await expectScreenPasses(p, `A ficha com Ajuda ${where}`);
  } finally {
    await close();
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('a Ajuda a 0 PV, no jogador, na ficha e na ordem do mestre, passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanAidScreens(browser, 'light', 1280);
});

test('a Ajuda a 0 PV, no jogador, na ficha e na ordem do mestre, passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-014', '@RN-02'] }, async ({ browser }) => {
  test.setTimeout(420_000);
  await scanAidScreens(browser, 'dark', 390);
});

// Crítico Brutal: the critical's sum before the roll, the typed sheet with its total on the button, and the log line.
async function scanBrutalScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = sizeOf(width);
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade Brutal ${Date.now()}`, true, true, { build: ragna, sheet: ragnaSheet });
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Ragna: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, { 'Capitão Goblin': [11, 5], 'Goblin 1': [6, 7], 'Goblin 2': [14, 10] });
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Atacar com Machado grande' }).click();
    // The dialog's name follows its heading, which changes with each step: the one dialog on the page.
    const sheet = p.getByRole('dialog');
    await viaTargetList(p, sheet);
    await sheet.locator('label', { hasText: 'Goblin 1' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Machado grande/).fill('20');
    await sheet.getByRole('button', { name: 'Confirmar 20' }).click();
    await expect(sheet.getByText('Dano do crítico')).toBeVisible();
    await expectScreenPasses(p, `Crítico Brutal, o dano do crítico antes de rolar ${where}`);
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 3d12/).fill('22');
    await expect(sheet.getByRole('button', { name: 'Confirmar 25' })).toBeVisible();
    await expectScreenPasses(p, `Crítico Brutal, dados físicos e o total no botão ${where}`);
    await sheet.getByRole('button', { name: 'Confirmar 25' }).click();
    await expect(sheet.getByRole('button', { name: 'Voltar à sua vez' })).toBeVisible();
    await expectScreenPasses(p, `Crítico Brutal, o resultado ${where}`);
    await sheet.getByRole('button', { name: 'Voltar à sua vez' }).click();
    // On a phone the log opens from its button; from 768px it is a fold under the map.
    const log = await openCombatLog(p);
    await expect(log).toContainText('Crítico Brutal');
    await expectScreenPasses(p, `Crítico Brutal, o registro do combate ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o Crítico Brutal, antes de rolar, com dados físicos e no registro, passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-012', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanBrutalScreens(browser, 'light', 1280);
});

test('o Crítico Brutal, antes de rolar, com dados físicos e no registro, passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-012', '@MR-014'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanBrutalScreens(browser, 'dark', 390);
});

// Talento Confiável on a trap search: the preview and the result, with the rule said.
async function scanReliableTalentScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = sizeOf(width);
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade Talento ${Date.now()}`, true, true, { build: dalila, sheet: dalilaSheet });
    campaignId = table.campaignId;
    await trapRPC(m, table, 'Fosso escondido', 6, 7, { findDc: 18 });
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Procurar armadilhas' }).click();
    const sheet = p.getByRole('dialog', { name: 'Procurar armadilhas' });
    await sheet.locator('label', { hasText: 'Investigação' }).click();
    await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
    await sheet.getByLabel(/Role 1d20 para Investigação/).fill('4');
    await expect(sheet.getByText('4 → 10 (Talento Confiável)')).toBeVisible();
    await expectScreenPasses(p, `Talento Confiável, a prévia do d20 baixo ${where}`);
    await sheet.getByRole('button', { name: /Confirmar 4/ }).click();
    await expect(sheet.getByText(/d20: 4 → 10 \(Talento Confiável\)/)).toBeVisible();
    await expectScreenPasses(p, `Talento Confiável, o resultado da busca ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o Talento Confiável na busca por armadilhas passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-035'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanReliableTalentScreens(browser, 'light', 1280);
});

test('o Talento Confiável na busca por armadilhas passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-035'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanReliableTalentScreens(browser, 'dark', 390);
});

async function scanJumpWarningScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = sizeOf(width);
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade Salto ${Date.now()}`, true, true, { build: toren, sheet: torenSheet });
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Toren: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 }, { 'Capitão Goblin': [15, 3], 'Goblin 1': [4, 6], 'Goblin 2': [15, 11] });
    await openSessionPage(p, campaignId);
    await p.getByRole('button', { name: 'Mover', exact: true }).click();
    await pickRadio(p, 'Saltar');
    await expect(p.getByRole('heading', { name: 'Saltar Toren' })).toBeVisible();
    await tapSquare(p, 6, 7);
    await expect(p.getByText('Esse salto sai do alcance do Goblin 1. Ele pode atacar você de graça (ataque de oportunidade).')).toBeVisible();
    await expectScreenPasses(p, `Saltar, o aviso de que sai do alcance ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o salto que sai do alcance de um inimigo passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-034', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanJumpWarningScreens(browser, 'light', 1280);
});

test('o salto que sai do alcance de um inimigo passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-034', '@RN-21'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanJumpWarningScreens(browser, 'dark', 390);
});

// W7-X: the contests and special actions. The player's sheets (Ajudar, Esconder-se, the grapple's target, wait and result, the
// answer to an NPC's grapple) and the master's cards (the contest, Hide), on one table with a rogue and an ally.
async function scanContestScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const { m, p, campaignId, done } = await alliedTable(
    browser,
    `Acessibilidade Disputas ${Date.now()}`,
    vex,
    { weaponKeys: ['equipment:shortsword'] },
    { Vex: 20, 'Goblin 1': 5, Tavo: 4, 'Goblin 2': 3 },
    { colorScheme, masterWidth: width, playerWidth: width },
  );
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toBeVisible();

    // Ajudar: the ally, then the task (it is closed without helping, so the action stays).
    await openActionTab(p, 'Outras');
    await p.getByRole('button', { name: 'Ajudar', exact: true }).click();
    const help = p.getByRole('dialog');
    await expect(help.getByText('Tavo').first()).toBeVisible();
    await expectScreenPasses(p, `Ajudar, o aliado ${where}`);
    await help.locator('label', { hasText: 'Tavo' }).click();
    await help.getByRole('button', { name: 'Continuar' }).click();
    await expect(help.getByText(/Vantagem no próximo teste de Percepção/)).toBeVisible();
    await expectScreenPasses(p, `Ajudar, a tarefa ${where}`);
    await p.keyboard.press('Escape');
    await expect(p.getByRole('dialog')).toHaveCount(0);

    // Esconder-se (the bonus action of the Ação Ardilosa): the roll, the wait, the master's card and the result.
    await openActionTab(p, 'Ladino');
    await p.getByRole('button', { name: 'Usar Ação Ardilosa: Esconder' }).click();
    const hide = p.getByRole('dialog');
    await expect(hide.getByText('Seu teste de Furtividade')).toBeVisible();
    await expectScreenPasses(p, `Esconder-se, o teste ${where}`);
    await typeD20(hide, 20);
    await expect(hide.getByText(/Esperando o mestre/)).toBeVisible();
    await expectScreenPasses(p, `Esconder-se, esperando o mestre ${where}`);
    await expect(m.getByTestId('hide-card')).toBeVisible();
    await expectScreenPasses(m, `O cartão de Esconder do mestre ${where}`);
    await m.getByTestId('hide-card').getByRole('button', { name: /^Aplicar/ }).click();
    await expect(hide.getByText(/Você está escondid/)).toBeVisible();
    await expectScreenPasses(p, `Esconder-se, o resultado ${where}`);
    await closeSheet(hide);

    // Agarrar: the target, the wait for the master and his card, the result.
    await openActionTab(p, 'Ataques');
    await p.getByRole('button', { name: /Agarrar: escolher o alvo/ }).click();
    const grapple = p.getByRole('dialog');
    await viaTargetList(p, grapple);
    await grapple.getByText('Tenho uma mão livre').click();
    await grapple.locator('label', { hasText: 'Goblin 1' }).click();
    await expectScreenPasses(p, `Agarrar, o alvo ${where}`);
    await grapple.getByRole('button', { name: 'Rolar a disputa' }).click();
    await expect(grapple.getByText(/Seu teste de/)).toBeVisible();
    await expectScreenPasses(p, `Agarrar, a rolagem ${where}`);
    await typeD20(grapple, 20);
    await expect(grapple.getByText(/Esperando o mestre/)).toBeVisible();
    await expectScreenPasses(p, `Agarrar, esperando o mestre ${where}`);
    const card = m.getByTestId('contest-card');
    await expect(card).toBeVisible();
    await expectScreenPasses(m, `O cartão da disputa do mestre ${where}`);
    await card.locator('label', { hasText: 'Atletismo' }).click();
    await typeD20(card, 3);
    await expect(grapple.getByText('Você venceu a disputa.')).toBeVisible();
    await expectScreenPasses(p, `Agarrar, o resultado ${where}`);
    await closeSheet(grapple);

    // The goblin grapples her: she chooses the skill and rolls.
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    // The player's end of turn lands first: passing the turns before it would end Vex's turn twice.
    await expect(p.getByRole('heading', { name: 'Sua vez, Vex' })).toHaveCount(0);
    const enc = await passTurnsTo(m, campaignId, 'Goblin 2');
    await waitForEncounterRevision([m, p], enc);
    await contestRPC(m, 'StartContest', {
      campaignId,
      encounterId: enc.id,
      initiatorId: idOf(enc, 'Goblin 2'),
      targetId: idOf(enc, 'Vex'),
      purpose: 'CONTEST_PURPOSE_GRAPPLE',
      roll: { d20Faces: { faces: [18] } },
    });
    const answer = p.getByRole('dialog');
    await expect(answer).toContainText('Goblin 2 tenta agarrar você');
    await expectScreenPasses(p, `A disputa de quem é agarrado, a escolha ${where}`);
    await answer.locator('label', { hasText: 'Acrobacia' }).click();
    await typeD20(answer, 2);
    await expect(answer.getByText(/Você perdeu a disputa/)).toBeVisible();
    await expectScreenPasses(p, `A disputa de quem é agarrado, o resultado ${where}`);
  } finally {
    await done().catch(() => undefined);
  }
}

test('as disputas e as ações especiais passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanContestScreens(browser, 'light', 1280);
});

test('as disputas e as ações especiais passam no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanContestScreens(browser, 'dark', 1280);
});

test('as disputas e as ações especiais passam no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanContestScreens(browser, 'light', 390);
});

test('as disputas e as ações especiais passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanContestScreens(browser, 'dark', 390);
});

// W7-X: the group check outside a combat (the master's request and open cards, the player's sheet) and the surprise card of the
// combat's setup, with the state of the one who is surprised.
async function scanGroupCheckScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = sizeOf(width);
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade Teste em grupo ${Date.now()}`, true, true, { build: vex, sheet: { weaponKeys: ['equipment:shortsword'] } });
    campaignId = table.campaignId;
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    await m.getByRole('button', { name: 'Pedir teste em grupo' }).click();
    const ask = m.getByTestId('group-ask');
    await expect(ask).toBeVisible();
    await expectScreenPasses(m, `O pedido do teste em grupo ${where}`);
    await ask.getByLabel('Teste', { exact: true }).selectOption({ label: 'Furtividade' });
    await ask.getByLabel(/^CD \(/).fill('13');
    await ask.getByRole('button', { name: 'Pedir o teste' }).click();
    const sheet = p.getByRole('dialog');
    // A party of one: "Todos" is one character, so the request is a check of its own.
    await expect(sheet.getByText('O mestre pede um teste de Furtividade.')).toBeVisible();
    await expectScreenPasses(p, `O teste em grupo, a rolagem ${where}`);
    await typeD20(sheet, 15);
    await expect(sheet.getByText(/Esperando o mestre/)).toBeVisible();
    await expectScreenPasses(p, `O teste em grupo, esperando o mestre ${where}`);
    const open = m.getByTestId('group-open');
    await expect(open).toContainText('1 de 1 responderam');
    await expectScreenPasses(m, `O teste em grupo aberto do mestre ${where}`);
    await open.getByRole('button', { name: 'Encerrar o teste' }).click();
    await expect(sheet.getByText('O mestre encerrou o teste.')).toBeVisible();
    await expectScreenPasses(p, `O teste em grupo, o resultado ${where}`);
    await closeSheet(sheet);

    // The combat's setup: "Quem está surpreso?", then the player's own state.
    let enc = await startEncounterRPC(m, table, [{ characterId: table.goblinId, count: 2, hidden: false }]);
    await waitForEncounterRevision(m, enc);
    const card = m.getByTestId('surprise-card');
    await expect(card).toContainText('Quem está surpreso?');
    await expectScreenPasses(m, `Quem está surpreso? ${where}`);
    await card.locator('label:has(input[aria-label="Surpreso: Vex"])').click();
    await expect(card.getByLabel('Surpreso: Vex')).toBeChecked();
    for (const c of enc.combatants) {
      enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: c.label === 'Vex' ? 20 : 3 });
    }
    for (const [label, [col, row]] of Object.entries({ Vex: [8, 9], 'Goblin 1': [9, 9], 'Goblin 2': [14, 10] })) {
      enc = await combatRPC(m, 'MoveCombatant', { campaignId, encounterId: enc.id, combatantId: idOf(enc, label), col, row });
    }
    await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
    await expect(p.getByText(/Você está surpres[oa] neste turno/)).toBeVisible();
    await expectScreenPasses(p, `A surpresa do jogador ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o teste em grupo e a surpresa passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanGroupCheckScreens(browser, 'light', 1280);
});

test('o teste em grupo e a surpresa passam no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanGroupCheckScreens(browser, 'dark', 1280);
});

test('o teste em grupo e a surpresa passam no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanGroupCheckScreens(browser, 'light', 390);
});

test('o teste em grupo e a surpresa passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-34'] }, async ({ browser }) => {
  test.setTimeout(300_000);
  await scanGroupCheckScreens(browser, 'dark', 390);
});

/**
 * "Efeitos que duram" no lado do mestre (W7-E, RN-22): o painel "Efeitos em jogo" com o relógio dos turnos, a pergunta de
 * encerrar uma concentração, "Adicionar um efeito", "Mudar a duração", o cartão do que os jogadores veem, a exaustão (e a
 * pergunta do nível 6), e, fora do combate, o painel dos personagens com "Passar o tempo" e o descanso longo com "Sem comida
 * ou bebida".
 */
async function scanEffectsScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const t = await effectsTable(browser, 'Acessibilidade Efeitos', width, colorScheme);
  const { m } = t;
  const dialog = (name: string | RegExp) => m.getByRole('dialog', { name });
  try {
    const panel = await openPanel(t);
    await expectScreenPasses(m, `Efeitos em jogo, o painel e o relógio ${where}`);

    const bless = effectRow(panel, 'Bênção');
    await bless.getByRole('button', { name: /^Encerrar Bênção/ }).click();
    const ask = panel.getByRole('alertdialog', { name: /Encerrar Bênção de Pensantus\?/ });
    await expect(ask.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await expectScreenPasses(m, `Efeitos em jogo, encerrar uma concentração ${where}`);
    await ask.getByRole('button', { name: 'Cancelar' }).click();

    await panel.getByRole('button', { name: 'Adicionar efeito' }).click();
    await expect(dialog('Adicionar um efeito')).toBeVisible();
    await expectScreenPasses(m, `Adicionar um efeito ${where}`);
    await dialog('Adicionar um efeito').getByRole('button', { name: 'Cancelar' }).click();

    await bless.getByRole('button', { name: /^Mudar a duração de Bênção/ }).click();
    await expect(dialog(/^Mudar a duração de Bênção em (Pensantus, Goblin 1|Goblin 1, Pensantus)$/)).toBeVisible();
    await expectScreenPasses(m, `Mudar a duração ${where}`);
    await dialog(/^Mudar a duração de Bênção em (Pensantus, Goblin 1|Goblin 1, Pensantus)$/).getByRole('button', { name: 'Cancelar' }).click();

    await effectRow(panel, 'Derrubado').getByRole('button', { name: /Jogadores veem/ }).click();
    await expect(dialog('O que os jogadores veem de Derrubado em Goblin 2')).toBeVisible();
    await expectScreenPasses(m, `O que os jogadores veem de um efeito ${where}`);
    await dialog('O que os jogadores veem de Derrubado em Goblin 2').getByRole('switch', { name: 'Os jogadores veem este efeito' }).click();
    await expectScreenPasses(m, `O que os jogadores veem de um efeito, desligado ${where}`);
    await dialog('O que os jogadores veem de Derrubado em Goblin 2').getByRole('button', { name: 'Cancelar' }).click();

    await panel.getByRole('button', { name: 'Exaustão' }).click();
    const exhaustion = dialog(/^Exaustão/);
    await expect(exhaustion.getByRole('radio')).toHaveCount(7);
    await expectScreenPasses(m, `Exaustão, os níveis ${where}`);
    await exhaustion.getByRole('radio', { name: /^Nível 6/ }).check();
    await exhaustion.getByRole('button', { name: 'Salvar' }).click();
    await expect(exhaustion.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await expectScreenPasses(m, `Exaustão, a pergunta do nível 6 ${where}`);
    await exhaustion.getByRole('button', { name: 'Cancelar' }).click();
    await exhaustion.getByRole('button', { name: 'Cancelar' }).click();
  } finally {
    await t.done();
  }
}

async function scanEffectsOutsideScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = sizeOf(width);
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport: sizeOf(390) });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableWithPensantus(m, p, `Acessibilidade Efeitos fora ${Date.now()}`);
    campaignId = table.campaignId;
    await startSessionRPC(m, campaignId);
    await setExhaustionRPC(m, campaignId, table.characterId, 2, 0);
    await openSessionPage(m, campaignId, 'grupo');
    const panel = m.getByRole('region', { name: 'Efeitos em jogo' });
    await expect(panel.getByText('Nenhum efeito nos personagens agora.')).toBeVisible();
    await expectScreenPasses(m, `Efeitos fora do combate, o painel e "Passar o tempo" ${where}`);
    const time = m.getByRole('group', { name: 'Passar o tempo' });
    // Each button keeps its words on one line, inside it.
    for (const preset of await time.getByRole('button').all()) {
      const text = await preset.evaluate((el) => {
        const range = document.createRange();
        range.selectNodeContents(el.querySelector('.mdc-button__label') ?? el);
        const rects = [...range.getClientRects()].filter((r) => r.width > 0);
        const frame = el.getBoundingClientRect();
        return {
          lines: new Set(rects.map((r) => Math.round(r.top))).size,
          inside: rects.every((r) => r.top >= frame.top - 1 && r.bottom <= frame.bottom + 1 && r.left >= frame.left - 1 && r.right <= frame.right + 1),
        };
      });
      expect(text, `${await preset.innerText()} on one line, inside its button`).toEqual({ lines: 1, inside: true });
    }
    await time.getByRole('button', { name: 'Outro tempo' }).click();
    await expectScreenPasses(m, `Passar o tempo, "Outro tempo" ${where}`);
    await time.getByRole('button', { name: '10 min, passar 10 minutos' }).click();
    await expect(panel).toContainText('Passou 10 minutos.');
    await expectScreenPasses(m, `Passar o tempo, o resultado ${where}`);

    const rest = m.getByRole('region', { name: 'Descanso' });
    await rest.getByRole('button', { name: 'Descanso longo' }).click();
    await expect(rest.getByRole('switch', { name: 'Sem comida ou bebida' })).toBeVisible();
    await expectScreenPasses(m, `Descanso longo, "Sem comida ou bebida" ${where}`);
    await rest.getByRole('switch', { name: 'Sem comida ou bebida' }).click();
    await expectScreenPasses(m, `Descanso longo, sem comida ou bebida ligado ${where}`);
    await rest.getByRole('button', { name: 'Cancelar' }).click();

    await panel.getByRole('button', { name: 'Exaustão' }).click();
    await expect(m.getByRole('dialog', { name: /^Exaustão/ })).toBeVisible();
    await expectScreenPasses(m, `Exaustão fora do combate ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

for (const [scheme, width, words] of [
  ['light', 1280, 'no tema claro, no desktop'],
  ['dark', 390, 'no tema escuro, no celular'],
  ['light', 390, 'no tema claro, no celular'],
  ['dark', 1280, 'no tema escuro, no desktop'],
] as const) {
  test(`os efeitos que duram do mestre, no combate, passam no axe e nas conferências de layout ${words}`, { tag: ['@a11y', '@W7-E', '@RN-22'] }, async ({ browser }) => {
    test.setTimeout(300_000);
    await scanEffectsScreens(browser, scheme, width);
  });

  test(`os efeitos do mestre fora do combate, "Passar o tempo" e o descanso longo passam no axe e nas conferências de layout ${words}`, { tag: ['@a11y', '@W7-E', '@RN-22'] }, async ({ browser }) => {
    test.setTimeout(300_000);
    await scanEffectsOutsideScreens(browser, scheme, width);
  });
}

/** The effects that last on the player's screens (W7-E, RN-22): "Seus efeitos" and the exhaustion card on the live sheet,
 * the attack sheet with the d4 of Bênção typed from a physical die, the turn of a paralysed character and the saving
 * throw of the end of the turn with its three answers, its typed fields and its result. The table and the effects come
 * through the API, so every run draws the same screens. */
async function scanEffectsPlayerScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade efeitos ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    const enc = await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
    const me = enc.combatants.find((c) => c.label === 'Pensantus')!.id;
    const captain = enc.combatants.find((c) => c.label === 'Capitão Goblin')!.id;
    const add = async (catalogKey: string, casterId?: string) => {
      const res = await callRPC(m, 'meurpg.play.v1.LastingEffectService/AddLastingEffect', {
        campaignId,
        encounterId: enc.id,
        idempotencyKey: crypto.randomUUID(),
        targetIds: [me],
        catalogKey,
        ...(casterId ? { casterId } : {}),
        duration: { kind: 'EFFECT_DURATION_KIND_ROUNDS', rounds: 10 },
        playerVisible: true,
        audience: 'EFFECT_AUDIENCE_ALL',
      });
      expect(res.ok(), await res.text()).toBeTruthy();
    };

    // The live sheet: the card of an effect and the exhaustion card.
    await add('spell:bless');
    const exhaustion = await callRPC(m, 'meurpg.play.v1.LastingEffectService/SetExhaustion', {
      campaignId,
      idempotencyKey: crypto.randomUUID(),
      characterId: table.characterId,
      level: 4,
      expectedLevel: 0,
    });
    expect(exhaustion.ok(), await exhaustion.text()).toBeTruthy();
    await openSessionPage(p, campaignId);
    await expect(p.getByRole('heading', { name: 'Seus efeitos' })).toBeVisible();
    await expect(p.getByRole('group', { name: 'Nível 4' })).toBeVisible();
    await expectScreenPasses(p, `Seus efeitos e a exaustão na ficha ${where}`);

    // The attack sheet with the d4 of Bênção beside the d20.
    await p.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    // The sheet's name changes while the result is typed, and the exhaustion of level 4 makes the attack take two d20.
    const attack = p.getByRole('dialog');
    await viaTargetList(p, attack);
    await attack.locator('label', { hasText: 'Capitão Goblin' }).click();
    await attack.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expect(attack.getByLabel('Resultado do d4 (Bênção)')).toBeVisible();
    await expectScreenPasses(p, `Atacar com Bênção, o d4 dos dados físicos ${where}`);
    await attack.getByLabel('Resultado do d4 (Bênção)').fill('3');
    // Two low dice: the attack misses, so no damage waits and the turn can end below.
    await attack.getByLabel('Primeiro d20').fill('2');
    await attack.getByLabel('Segundo d20').fill('5');
    await attack.getByRole('button', { name: /^Confirmar/ }).click();
    await expect(attack).toContainText('+ 1d4 (3)');
    await expectScreenPasses(p, `Atacar com Bênção, o resultado com o d4 ${where}`);
    await p.keyboard.press('Escape');
    await expect(attack).toBeHidden();

    // The turn of a paralysed character, and the saving throw of the end of the turn.
    await add('spell:hold-person', captain);
    await expect(p.getByTestId('effect-note')).toBeVisible();
    await expectScreenPasses(p, `O turno de quem está paralisado ${where}`);
    await p.getByRole('button', { name: 'Encerrar turno' }).click();
    // The dialog of a desktop is named by its title, the bottom sheet of a phone by its label.
    const save = p.getByRole('dialog', { name: /^(Teste de resistência do fim do turno|Fim do seu turno)$/ });
    await expect(save.getByRole('button', { name: 'Rolar no app' })).toBeVisible();
    await expectScreenPasses(p, `Fim do seu turno, as três respostas ${where}`);
    await save.getByRole('button', { name: 'Digitar o resultado' }).click();
    // The exhaustion of level 4 gives disadvantage on the save, and Pensantus (a Rock Gnome: Gnome Cunning, a Wisdom
    // save against a spell) has advantage: they cancel (SRD 5.1), so one d20, and Bênção adds its d4.
    await expect(save.getByLabel(/Resultado do d20/)).toBeVisible();
    await expectScreenPasses(p, `Fim do seu turno, digitando o d20 ${where}`);
    await save.getByLabel('Resultado do d4 (Bênção)').fill('2');
    await save.getByLabel(/Resultado do d20/).fill('20');
    await save.getByRole('button', { name: /Confirmar/ }).click();
    await expect(save).toContainText('Passou');
    await expectScreenPasses(p, `Fim do seu turno, o resultado ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('os efeitos que duram, na tela do jogador, passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@W7-E', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanEffectsPlayerScreens(browser, 'light', 1280);
});

test('os efeitos que duram, na tela do jogador, passam no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@W7-E', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanEffectsPlayerScreens(browser, 'light', 390);
});

test('os efeitos que duram, na tela do jogador, passam no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@W7-E', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanEffectsPlayerScreens(browser, 'dark', 1280);
});

test('os efeitos que duram, na tela do jogador, passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@W7-E', '@RN-22'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanEffectsPlayerScreens(browser, 'dark', 390);
});

// The free dice roller (MR-052): the header's "Dados" button, and the sheet of a phone or the dialog of a desktop for
// both roles: the form, the master's "Só eu" chosen and the result card. The button off while the stream reconnects is
// covered by the header's spec: Playwright's offline mode does not end a stream that is already open.
async function scanFreeDiceScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(240_000);
  const where = `(${colorScheme}, ${width}px)`;
  const { m, p, done } = await diceTable(browser, `Acessibilidade rolar dados ${Date.now()}`, { master: width, player: width }, colorScheme);
  try {
    await expect(m.getByRole('button', { name: 'Dados', exact: true })).toBeEnabled();
    await expectScreenPasses(m, `Sessão com o botão Dados, mestre ${where}`);
    await expectScreenPasses(p, `Sessão com o botão Dados, jogador ${where}`);

    // The player: the form, then the result card of 2d6 + 3.
    const playerSheet = await openDice(p);
    await expectScreenPasses(p, `Rolar dados, jogador ${where}`);
    await playerSheet.getByRole('button', { name: '2d6', exact: true }).click();
    await stepModifier(playerSheet, 0, 3);
    await playerSheet.getByRole('button', { name: 'Rolar 2d6 + 3', exact: true }).click();
    await expect(playerSheet.locator('app-free-roll-card .card__line')).toContainText('2d6 + 3 = ');
    await expect(playerSheet.getByRole('button', { name: 'Rolar 2d6 + 3 de novo' })).toBeEnabled();
    await expectScreenPasses(p, `Rolar dados, o resultado do jogador ${where}`);

    // The master's page shows the player's roll on the dice overlay. The suite runs with reduced motion, so the dice sit
    // still in a card that leaves the page usable: scan it with the "Som dos dados" toggle (off, a 44 px target) and close it.
    const overlay = m.locator('app-roll-overlay');
    await expect(overlay.locator('.result')).toContainText('Rolagem livre · 2d6 + 3', { timeout: 15_000 });
    const sound = overlay.getByRole('button', { name: 'Som dos dados' });
    await expect(sound).toHaveAttribute('aria-pressed', 'false');
    expect((await sound.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    await expectScreenPasses(m, `Rolar dados, a animação dos dados do mestre ${where}`);
    await overlay.getByRole('button', { name: 'Fechar' }).click();
    await expect(overlay.locator('.result')).toBeHidden();

    await p.keyboard.press('Escape');
    await expect(playerSheet).toBeHidden();

    // The master: the form with "Para a mesa", with "Só eu" chosen, and the secret result with its badge.
    const masterSheet = await openDice(m);
    await expectScreenPasses(m, `Rolar dados, mestre, para a mesa ${where}`);
    await masterSheet.getByRole('radiogroup', { name: 'Quem vê' }).locator('label', { hasText: 'Só eu' }).click();
    await expect(masterSheet.getByRole('button', { name: 'Rolar 1d20, só eu', exact: true })).toBeVisible();
    await expectScreenPasses(m, `Rolar dados, mestre, só eu ${where}`);
    await masterSheet.getByRole('button', { name: 'Rolar 1d20, só eu', exact: true }).click();
    await expect(masterSheet.getByText('Só você viu esta rolagem.')).toBeVisible();
    await expectScreenPasses(m, `Rolar dados, o resultado secreto do mestre ${where}`);
    await m.keyboard.press('Escape');
    await expect(masterSheet).toBeHidden();
  } finally {
    await done();
  }
}

test('o rolador de dados livre passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-052'] }, async ({ browser }) => {
  await scanFreeDiceScreens(browser, 'light', 1280);
});

test('o rolador de dados livre passa no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@MR-052'] }, async ({ browser }) => {
  await scanFreeDiceScreens(browser, 'light', 390);
});

test('o rolador de dados livre passa no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@MR-052'] }, async ({ browser }) => {
  await scanFreeDiceScreens(browser, 'dark', 1280);
});

test('o rolador de dados livre passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-052'] }, async ({ browser }) => {
  await scanFreeDiceScreens(browser, 'dark', 390);
});

// The master's light warning (B3): on a dark map Toren (a human, no darkvision) has no light, so the session map card shows
// "No escuro: Toren ...", the badge sits beside his name in "Grupo", "Ver no mapa" marks him on the map, "Acender luz" opens the
// carried-light sheet and, once he carries a torch, the notice leaves and the status message says so. The player's own screen
// never gets any of it (RN-10). On a computer the map page shows the same warning, as one line in "Pintar".
async function scanDarkWarningScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  test.setTimeout(300_000);
  const viewport = { width, height: width < 700 ? 800 : 900 };
  const contexts = await Promise.all(
    (['Mestre Teste', 'Jogador Teste', 'E-mail Não Verificado'] as const).map((user) =>
      browser.newContext({ storageState: authStatePath(user), colorScheme, viewport }),
    ),
  );
  const [m, p, t] = await Promise.all(contexts.map((c) => c.newPage()));
  const where = `(${colorScheme}, ${width}px)`;
  try {
    await Promise.all([m.goto('/'), p.goto('/'), t.goto('/')]);
    const table = await tableForFog(m, p, t, `Acessibilidade no escuro ${Date.now()}`, {});

    // The notice, the badge and the ring: Toren is the only one in the dark (Pensantus has darkvision).
    await m.goto(sessionRoute(table.campaignId));
    // The map: a tab of the board, a fold of the scene's page on a phone.
    await showMasterTab(m, 'mapa');
    const notice = m.locator('app-session-dark .dark');
    await expect(notice).toContainText('No escuro: Toren não enxerga onde está (sem luz e sem visão no escuro).');
    await expect(m.locator('.js-dark')).toContainText('No escuro');
    await expectScreenPasses(m, `No escuro, o aviso do mestre ${where}`);

    // "Ver no mapa": the map centers and only Toren is labelled.
    await notice.getByRole('button', { name: 'Ver no mapa, Toren' }).click();
    await expect(m.locator('app-dark-marks .label')).toHaveText('Toren · no escuro');
    await expectScreenPasses(m, `No escuro, Ver no mapa ${where}`);

    // The player's screen never changes (RN-10).
    await openPlayerMap(t, sessionRoute(table.campaignId));
    await expect(t.locator('app-fog-base')).toBeVisible();
    await expect(t.locator('app-session-dark, app-dark-marks, .js-dark')).toHaveCount(0);

    // "Acender luz": the existing carried-light sheet, then the notice leaves and the status message says why.
    await notice.getByRole('button', { name: 'Acender luz de Toren' }).click();
    // A dialog named by its title on a computer, a bottom sheet named "Luz que você carrega" on a phone.
    const sheet = m.getByRole('dialog', { name: /Acender luz: Toren|Luz que você carrega/ });
    await expect(sheet.getByRole('heading', { name: 'Acender luz: Toren' })).toBeVisible();
    await expectScreenPasses(m, `No escuro, Acender luz ${where}`);
    await sheet.getByText('Tocha', { exact: true }).click();
    await expect(sheet.getByRole('radio', { name: /Tocha/ })).toBeChecked();
    await sheet.getByRole('button', { name: 'Pronto' }).click();
    await expect(m.getByRole('status').filter({ hasText: 'Toren acendeu a tocha. Não está mais no escuro.' })).toBeVisible();
    await expect(m.locator('app-session-dark .dark')).toHaveCount(0);
    await expectScreenPasses(m, `No escuro, a luz acesa ${where}`);

    // The torch goes out: the notice returns at once, with the message about the light.
    await setCarriedLightRPC(m, table.campaignId, table.mapId, table.torenId, '');
    await expect(m.getByRole('status').filter({ hasText: 'A tocha de Toren apagou. A luz de Toren ficou “Nenhuma”.' })).toBeVisible();
    await expect(notice).toBeVisible();
    await expectScreenPasses(m, `No escuro, a luz apagou ${where}`);

    // The map page (a computer): the notice over the map, one line in "Pintar".
    if (width >= 768) {
      await m.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
      await expect(m.locator('app-editor-dark .dark')).toContainText('No escuro: Toren não enxerga onde está');
      await expectScreenPasses(m, `No escuro, a página do mapa ${where}`);
      await m.getByRole('radio', { name: 'Pintar' }).click();
      await expect(m.locator('app-editor-dark .dark--line')).toContainText('No escuro: Toren.');
      await expectScreenPasses(m, `No escuro, a página do mapa pintando ${where}`);
    }
  } finally {
    await Promise.all(contexts.map((c) => c.close()));
  }
}

test('o aviso de luz do mestre passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@B3'] }, async ({ browser }) => {
  await scanDarkWarningScreens(browser, 'light', 1280);
});

test('o aviso de luz do mestre passa no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@B3'] }, async ({ browser }) => {
  await scanDarkWarningScreens(browser, 'light', 390);
});

test('o aviso de luz do mestre passa no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@B3'] }, async ({ browser }) => {
  await scanDarkWarningScreens(browser, 'dark', 1280);
});

test('o aviso de luz do mestre passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@B3'] }, async ({ browser }) => {
  await scanDarkWarningScreens(browser, 'dark', 390);
});

// The scene's conversation (MR-051, RN-36): the empty panel, the lines (a narration, a whisper, an NPC), the "Para"
// chooser open (a dialog on a computer, a bottom sheet on a phone) and, on a phone, the "Conversa" tab with its unread
// count and the full-screen view. Each scan waits for the state it names: a line, the tab's name, the chooser's line.
async function scanConversationScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const table = await openConversaTable(browser, `Acessibilidade conversa ${Date.now()}`, { width, colorScheme });
  const { master: m, pensantus: p } = table;
  const where = `(${colorScheme}, ${width}px)`;
  const phone = width < 768;
  const openChat = async (page: Page) => {
    if (phone) {
      await page.getByRole('button', { name: /^Conversa( \d+, \d+ mensage(?:m|ns) novas?)?$/ }).click();
    }
    // The empty feed is not drawn; the panel is there once its message box is.
    await expect(messageBoxOf(page)).toBeVisible();
  };
  try {
    await openChat(m);
    await expect(m.getByText('A conversa começa quando o mestre narrar a cena.')).toBeVisible();
    await expectScreenPasses(m, `Conversa vazia, do mestre ${where}`);

    // The chooser, with two ways to choose and its live line.
    await m.getByLabel('Para', { exact: true }).click();
    const chooser = m.getByRole('dialog', { name: 'Quem lê esta mensagem?' });
    await chooser.locator('label', { hasText: 'Só algumas pessoas' }).click();
    await chooser.getByRole('checkbox', { name: /^Pensantus/ }).check();
    await expect(chooser.getByRole('status')).toContainText('Pensantus lê esta mensagem.');
    await expectScreenPasses(m, `Conversa, quem lê esta mensagem ${where}`);
    await chooser.getByRole('button', { name: 'Pronto' }).click();
    await expect(chooser).toBeHidden();

    const whisper = `Um sussurro só para Pensantus ${Date.now()}`;
    await say(m, whisper);
    await expect(lineOf(m, whisper)).toContainText('só para Pensantus');
    await chooseReaders(m, []);
    const narration = `A taverna fica em silêncio quando você entra ${Date.now()}`;
    await say(m, narration);
    await expect(lineOf(m, narration)).toBeVisible();
    await m.getByRole('combobox', { name: 'Falar como' }).click();
    await m.getByRole('option', { name: /^Capitão Goblin/ }).click();
    const npc = `Quem ousa entrar no meu covil? ${Date.now()}`;
    await say(m, npc);
    await expect(lineOf(m, npc)).toContainText('Capitão Goblin');
    await expectScreenPasses(m, `Conversa, do mestre com as linhas ${where}`);

    // The player: on a phone the tab says there is news, then the full-screen view.
    if (phone) {
      await expect(p.getByRole('button', { name: /^Conversa \d+, \d+ mensage(?:m|ns) novas?$/ })).toBeVisible();
      // The notice at the top of the page says the same, in view without scrolling; it is awaited by its words.
      const notice = p.getByRole('button', { name: /^Conversa: \d+ mensage(?:m|ns) novas? Abrir$/ });
      await expect(notice).toBeVisible();
      await expect(notice).toBeInViewport();
      await expectScreenPasses(p, `Aviso de mensagens novas no topo e aba Conversa com mensagens novas ${where}`);
    }
    await openChat(p);
    await expect(lineOf(p, npc)).toBeVisible();
    await expect(lineOf(p, whisper)).toContainText('Sussurro do mestre · só você lê');
    await expectScreenPasses(p, `Conversa, do jogador ${where}`);
  } finally {
    await table.close();
  }
}

test('a conversa da cena passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-051', '@RN-36'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanConversationScreens(browser, 'light', 1280);
});

test('a conversa da cena passa no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@MR-051', '@RN-36'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanConversationScreens(browser, 'dark', 1280);
});

test('a conversa da cena passa no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@MR-051', '@RN-36'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanConversationScreens(browser, 'light', 390);
});

test('a conversa da cena passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-051', '@RN-36'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanConversationScreens(browser, 'dark', 390);
});

/**
 * The spell zones (RN-37): the combat screen with a zone on the map and "Zonas ativas" for the master and for a player
 * (a zone of another caster with its details hidden), the question that ends a zone, the move picker and the saving throw
 * a zone asks of the player's character. Each scan waits for the state it judges: the label on the map, the heading's count
 * and the dialog, so axe never reads the screen before the zone arrived.
 */
async function scanZoneScreens(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const where = `(${colorScheme}, ${width}px)`;
  const t = await zoneTable(browser, `Acessibilidade zonas ${Date.now()}`, width, colorScheme);
  const { m, p, campaignId } = t;
  try {
    await openSessionPage(m, campaignId);
    await openSessionPage(p, campaignId);

    const masterTitle = zonesPanel(m).getByRole('heading', { level: 2 });
    await expect(masterTitle).toHaveText('Zonas ativas (3)');
    await expect(zoneLayer(m)).toContainText('Teia · Pensantus');
    await expect(zoneCard(m, 'Raio Lunar · Sombra')).toContainText('Só você vê');
    await expectScreenPasses(m, `Zonas no mapa e a lista, o mestre ${where}`);

    // The player: Teia is the last cast, so the list is whole once it shows; the details of another caster's zone are not there.
    await expect(zonesPanel(p).getByRole('heading', { level: 2 })).toHaveText('Zonas ativas (2)');
    await expect(zoneCard(p, 'Teia · Pensantus')).toBeVisible();
    await expect(zoneCard(p, 'Raio Lunar · Druida')).not.toContainText('Constituição');
    await expectScreenPasses(p, `Zonas no mapa e a lista, o jogador com detalhes ocultos ${where}`);

    // The end: the question in place, under the zone's card.
    const web = zoneCard(m, 'Teia · Pensantus');
    await web.getByRole('button', { name: 'Encerrar zona' }).click();
    const ask = web.getByRole('group', { name: 'Encerrar a zona?' });
    await expect(ask).toBeVisible();
    await expectScreenPasses(m, `Encerrar a zona, a pergunta no lugar ${where}`);
    await ask.getByRole('button', { name: 'Não' }).click();
    await expect(ask).toHaveCount(0);

    // The move sheet: the map picker with a square picked and its preview.
    await zoneCard(m, 'Raio Lunar · Druida').getByRole('button', { name: 'Mover', exact: true }).click();
    const sheet = m.getByRole('dialog', { name: 'Mover o Raio Lunar' });
    await expect(sheet).toBeVisible();
    const map = sheet.getByRole('group', { name: /Mapa de batalha/ });
    await expect(map).toBeVisible();
    await expectScreenPasses(m, `Mover a zona, o mapa antes de escolher ${where}`);
    await map.focus();
    await map.press('ArrowRight');
    await map.press('ArrowRight');
    await expect(sheet.getByRole('button', { name: 'Mover a zona' })).not.toHaveAttribute('aria-disabled', 'true');
    await expectScreenPasses(m, `Mover a zona, o quadrado escolhido ${where}`);
    await sheet.getByRole('button', { name: 'Cancelar' }).click();
    await expect(sheet).toHaveCount(0);

    // The player's saving throw, for stepping into the zone: no DC.
    await enterZoneRPC(t, await zoneOf(m, campaignId, 'spell:moonbeam', 'Druida'));
    const save = p.getByRole('dialog', { name: 'Raio Lunar de Druida' });
    await expect(save).toBeVisible();
    await expect(save).toContainText('Você entrou na zona.');
    await expectScreenPasses(p, `O teste de resistência de uma zona, o jogador ${where}`);
  } finally {
    await t.done();
  }
}

test('as zonas de magia passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@RN-37'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanZoneScreens(browser, 'light', 1280);
});

test('as zonas de magia passam no axe e nas conferências de layout no tema escuro, no desktop', { tag: ['@a11y', '@RN-37'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanZoneScreens(browser, 'dark', 1280);
});

test('as zonas de magia passam no axe e nas conferências de layout no tema claro, no celular', { tag: ['@a11y', '@RN-37'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanZoneScreens(browser, 'light', 390);
});

test('as zonas de magia passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@RN-37'] }, async ({ browser }) => {
  test.setTimeout(400_000);
  await scanZoneScreens(browser, 'dark', 390);
});

/**
 * The session bar and the pinned nav (compact screens): the bar is one row of 56 px for the master and the player, the
 * master's "Sessão" menu and the end asked in place under it pass the scans, and on a phone the nav sits at the bottom of the screen.
 */
async function scanSessionBar(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const phone = width < 768;
  const viewport = { width, height: width >= 768 ? 900 : width <= 320 ? 568 : 844 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableWithPensantus(m, p, `Acessibilidade barra ${Date.now()}`);
    campaignId = table.campaignId;
    await startSessionRPC(m, campaignId);
    await openSessionPage(p, campaignId);
    await openSessionPage(m, campaignId);

    for (const [who, page] of [['mestre', m], ['jogador', p]] as const) {
      const bar = page.locator('app-session-header .bar');
      await expect(bar).toBeVisible();
      const box = await bar.boundingBox();
      // 60 px on a computer, 56 px on a phone (C6-10).
      const barHeight = phone ? 56 : 60;
      expect(box?.height).toBeGreaterThanOrEqual(barHeight - 1);
      expect(box?.height).toBeLessThanOrEqual(barHeight + 1);
      await expect(page.getByRole('link', { name: /^Voltar para a campanha/ })).toBeVisible();
      await expect(page.getByRole('button', { name: /^Dados|^Rolando/ })).toBeVisible();
      await expectScreenPasses(page, `Barra da sessão, ${who} ${where}`);
      if (phone) {
        const nav = page.getByRole('navigation', { name: 'Seções da sessão' });
        await expect(nav).toBeVisible();
        const navBox = await nav.boundingBox();
        expect(Math.round((navBox?.y ?? 0) + (navBox?.height ?? 0))).toBe(viewport.height);
        // The open section is a 48 px pill, and nothing sits under the nav: the footer's links are not drawn on a live phone screen.
        const pill = await nav.locator('[aria-current="true"]').boundingBox();
        expect(Math.round(pill?.height ?? 0)).toBe(48);
        await expect(page.getByRole('link', { name: 'Termos de uso' })).toBeHidden();
      }
    }

    // The master's menu, then the end asked in place with "Cancelar" focused.
    await openSessionMenu(m);
    await expect(m.getByRole('menuitem', { name: 'Copiar link da sessão' })).toBeVisible();
    await expectScreenPasses(m, `Menu Sessão aberto ${where}`);
    await m.getByRole('menuitem', { name: 'Encerrar sessão' }).click();
    const ask = m.getByRole('group', { name: /Encerrar a sessão \d+\? Ela não reabre depois\./ });
    await expect(ask).toBeVisible();
    await expect(ask.getByRole('button', { name: 'Cancelar' })).toBeFocused();
    await expectScreenPasses(m, `Encerrar a sessão perguntado no lugar ${where}`);
    await ask.getByRole('button', { name: 'Cancelar' }).click();
    await expect(ask).toHaveCount(0);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('a barra da sessão, o menu "Sessão" e a navegação fixa passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-011', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSessionBar(browser, 'light', 1280);
});

test('a barra da sessão, o menu "Sessão" e a navegação fixa passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-011', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSessionBar(browser, 'dark', 390);
});

test('a barra da sessão, o menu "Sessão" e a navegação fixa passam no axe e nas conferências de layout no tema claro, no celular de 320', { tag: ['@a11y', '@MR-011', '@MR-032'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanSessionBar(browser, 'light', 320);
});

/** The master acts for a player's character (MR-053, C8-01 to C8-04): the character's card with "Agir por", the rail while acting, the target
 * asked for the character, and the card of a downed character, whose death save leads the turn. */
async function scanMasterActs(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade mestre age ${Date.now()}`, true, true, { sheet: pensantusCasting });
    campaignId = table.campaignId;
    await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });

    await openSessionPage(m, campaignId);
    await expect(m.getByRole('button', { name: 'Agir por Pensantus' })).toBeVisible();
    await expectScreenPasses(m, `O mestre, a vez de Pensantus com "Agir por" ${where}`);
    await m.getByRole('button', { name: 'Agir por Pensantus' }).click();
    await expect(m.getByText('Você está agindo por Pensantus')).toBeVisible();
    await expectScreenPasses(m, `O mestre age por Pensantus ${where}`);
    await m.getByRole('button', { name: 'Atacar com Raio de Fogo' }).click();
    await expect(m.getByRole('region', { name: 'Escolha do alvo' })).toBeVisible();
    await expectScreenPasses(m, `O mestre escolhe o alvo de Pensantus no mapa ${where}`);
    await m.getByRole('region', { name: 'Escolha do alvo' }).getByRole('button', { name: 'Cancelar' }).click();
    await m.getByRole('button', { name: 'Voltar ao mestre' }).click();

    // A downed character: the death save leads the turn, "Próximo turno" is the dashed button.
    await adjustVitalsRPC(m, campaignId, table.characterId, { hitPointsCurrent: 0 });
    const enc = await getEncounterRPC(m, campaignId);
    await combatRPC(m, 'EndTurn', { campaignId, encounterId: enc.id, expectedCombatantId: enc.currentCombatantId });
    await waitForEncounterRevision(m, await passTurnsTo(m, campaignId, 'Pensantus'));
    await expect(m.getByRole('heading', { name: 'Pensantus está caído: teste contra a morte' })).toBeVisible();
    await expectScreenPasses(m, `O mestre, o teste contra a morte de Pensantus ${where}`);
    await m.getByRole('button', { name: 'Digitar o resultado' }).click();
    await expectScreenPasses(m, `O mestre digita o teste contra a morte ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('o mestre agir por um personagem passa no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-053'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanMasterActs(browser, 'light', 1280);
});

test('o mestre agir por um personagem passa no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-053'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanMasterActs(browser, 'dark', 390);
});

/** The master's creature actions (W7-M, MR-012): a hell hound's NPC card with its "Outras ações" area (a breath weapon with the targets
 * named in the mind's theatre), the result of the save action under the list, and the offer of a ghoul's claw rider after the hit. */
async function scanCreatureActions(browser: Browser, colorScheme: 'light' | 'dark', width: number): Promise<void> {
  const viewport = { width, height: 900 };
  const master = await browser.newContext({ storageState: authStatePath('Mestre Teste'), colorScheme, viewport });
  const player = await browser.newContext({ storageState: authStatePath('Jogador Teste'), colorScheme, viewport });
  const m = await master.newPage();
  const p = await player.newPage();
  const where = `(${colorScheme}, ${width}px)`;
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForCombat(m, p, `Acessibilidade ações ${Date.now()}`, false, false, { build: vex, sheet: { weaponKeys: ['equipment:rapier'] } });
    campaignId = table.campaignId;
    const started = await callRPC(m, 'meurpg.play.v1.CombatService/StartEncounter', {
      campaignId,
      idempotencyKey: crypto.randomUUID(),
      name: 'Emboscada na estrada',
      participants: [],
      monsters: [
        { creatureKey: 'monster:hell-hound', count: 1 },
        { creatureKey: 'monster:ghoul', count: 1 },
      ],
      mode: 'ENCOUNTER_MODE_THEATRE',
    });
    expect(started.ok(), await started.text()).toBeTruthy();
    let enc = (await started.json()).encounter as Encounter;
    const hound = enc.combatants.find((c) => c.label === 'Cão infernal')!;
    const ghoul = enc.combatants.find((c) => c.label === 'Carniçal')!;
    const faces = new Map([[hound.id, 20], [ghoul.id, 19]]);
    for (const c of enc.combatants) {
      enc = await combatRPC(m, 'SubmitInitiative', { campaignId, encounterId: enc.id, combatantId: c.id, d20Face: faces.get(c.id) ?? 1 });
    }
    enc = await combatRPC(m, 'BeginCombat', { campaignId, encounterId: enc.id });
    for (const c of [hound, ghoul]) {
      enc = await combatRPC(m, 'SetCombatantHidden', { campaignId, encounterId: enc.id, combatantId: c.id, hidden: false });
    }
    await openSessionPage(m, campaignId);

    // Both monsters share the turn (the master plays one part after the other): the ghoul's claw hits and offers its rider.
    const ghoulCard = m.getByRole('region', { name: 'Vez do Carniçal' });
    await ghoulCard.locator('label', { hasText: 'Garras' }).click();
    await ghoulCard.getByRole('button', { name: 'Digitar o resultado' }).click();
    await ghoulCard.getByLabel(/Role 1d20/).fill('15');
    await ghoulCard.getByRole('button', { name: 'Confirmar 15' }).click();
    await ghoulCard.getByRole('button', { name: 'Rolar dano' }).click();
    await ghoulCard.getByRole('button', { name: /Aplicar \d+ de dano/ }).click();
    await expect(m.getByTestId('hit-rider').first()).toBeVisible();
    await expectScreenPasses(m, `A ficha do mestre, a oferta do efeito das garras ${where}`);
    await m.getByTestId('hit-rider').first().getByRole('button', { name: /^Aplicar / }).click();
    // The apply settles before the part ends, as a master does it: the offer is gone once the answer is on screen.
    await expect(m.getByTestId('hit-rider')).toHaveCount(0);
    // The page shows the server's latest combat (the rider's answer) before the part ends, so the end acts on what the page shows.
    await waitForEncounterRevision(m, await getEncounterRPC(m, campaignId), 'the rider answer');
    await waitForEncounterRevision(m, await endPartRPC(m, campaignId, 'Carniçal'), 'the end of the ghoul\'s part');

    // The hell hound's card: the actions area, then the result of the breath's saving throw.
    const card = m.getByRole('region', { name: 'Vez do Cão infernal' });
    await expect(card.getByRole('button', { name: /^Usar Sopro de fogo/ })).toBeVisible();
    await expectScreenPasses(m, `A ficha do mestre, a vez do Cão infernal com "Outras ações" ${where}`);
    await card.getByRole('button', { name: /^Usar Sopro de fogo/ }).click();
    await expect(card.getByTestId('creature-action-result')).toBeVisible();
    await expectScreenPasses(m, `A ficha do mestre, o resultado da salvaguarda da ação ${where}`);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
}

test('as ações de criatura do mestre passam no axe e nas conferências de layout no tema claro, no desktop', { tag: ['@a11y', '@MR-012'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanCreatureActions(browser, 'light', 1280);
});

test('as ações de criatura do mestre passam no axe e nas conferências de layout no tema escuro, no celular', { tag: ['@a11y', '@MR-012'] }, async ({ browser }) => {
  test.setTimeout(240_000);
  await scanCreatureActions(browser, 'dark', 390);
});
