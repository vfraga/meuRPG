import { expect, test, type Page } from '@playwright/test';

import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { addActionRPC, rollTyped, sceneActionIdsRPC, setAttemptsRPC, tableForScenes } from './scene-support';
import { orla, orlaSheet } from './rests-support';
import { callRPC, newSignedInContext } from './support';
import { secondPlayer } from './theatre-support';

// MR-015 (the actions of an RP scene), through the screens: the master picks
// the actions on the map point, opens the scene in the session, each player
// sees their own bonuses and rolls (RN-18: in the app or with a physical die),
// and the master sees every roll with whether it passed (RN-20: the DC and the
// result stay with the master). Setup (campaigns, maps, points, the session)
// goes through the API; every test makes its own campaign.

/** Picks a check in the add form: the kind, then the check's name in the list. */
async function addAction(
  master: Page,
  action: { kind: 'Perícia' | 'Teste de habilidade' | 'Teste de resistência'; check: string; name?: string; dc?: string },
): Promise<void> {
  await master.getByRole('button', { name: 'Adicionar ação' }).click();
  const form = master.getByRole('form', { name: 'Nova ação' });
  await expect(form.getByRole('radio', { name: 'Perícia' })).toBeFocused();
  await form.getByRole('radio', { name: action.kind }).check();
  await form.getByRole('combobox').selectOption({ label: action.check });
  if (action.name) {
    await form.getByLabel('Nome (opcional)').fill(action.name);
  }
  if (action.dc) {
    await form.getByLabel('CD (opcional)').fill(action.dc);
  }
  await form.getByRole('button', { name: 'Adicionar ação' }).click();
  await expect(form).toBeHidden();
  await expect(master.getByRole('button', { name: 'Adicionar ação' })).toBeFocused();
}

/** The master's "Abrir cena" picker: picks the point and confirms. */
async function openScene(master: Page, point: string): Promise<void> {
  await master.getByRole('button', { name: 'Abrir cena', exact: true }).click();
  const picker = master.getByRole('dialog', { name: 'Abrir uma cena' });
  await expect(picker.getByRole('heading', { name: 'Abrir uma cena' })).toBeFocused();
  // A person clicks the scene's words (the radio itself is drawn as the whole row).
  await picker.getByText(point, { exact: true }).click();
  await expect(picker.getByRole('radio', { name: new RegExp(point) })).toBeChecked();
  await picker.getByRole('button', { name: 'Abrir cena', exact: true }).click();
  await expect(picker).toBeHidden();
}

test(
  'o mestre escolhe as ações no ponto de cena, abre a cena na sessão, o jogador rola uma no app e uma com o dado físico e o mestre vê as duas com Passou e Não passou',
  { tag: ['@MR-015', '@RN-18', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Cena ${Date.now()}`, false);
      campaignId = table.campaignId;

      // The editor: three actions on the point, each saved at once.
      await master.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('heading', { name: 'Ações da cena' })).toBeVisible();
      await expect(master.getByText('Nenhuma ação ainda')).toBeVisible();

      await addAction(master, { kind: 'Perícia', check: 'Investigação', name: 'Procurar pistas na carroça', dc: '12' });
      await addAction(master, { kind: 'Perícia', check: 'Percepção' });
      // A DC outside 1 to 30 is said under its field, and the field takes focus.
      await master.getByRole('button', { name: 'Adicionar ação' }).click();
      const form = master.getByRole('form', { name: 'Nova ação' });
      await form.getByRole('radio', { name: 'Teste de resistência' }).check();
      await form.getByRole('combobox').selectOption({ label: 'Constituição' });
      await form.getByLabel('Nome (opcional)').fill('Resistir ao cheiro de fumaça');
      await form.getByLabel('CD (opcional)').fill('31');
      await form.getByRole('button', { name: 'Adicionar ação' }).click();
      await expect(form.getByText('A CD vai de 1 a 30. Digite outro número ou deixe em branco.')).toBeVisible();
      await expect(form.getByLabel('CD (opcional)')).toBeFocused();
      await form.getByLabel('CD (opcional)').fill('10');
      await form.getByRole('button', { name: 'Adicionar ação' }).click();
      await expect(form).toBeHidden();

      await expect(master.getByText('3 de 20', { exact: true })).toBeVisible();
      const list = master.getByRole('list').filter({ has: master.getByRole('button', { name: 'Subir Percepção' }) });
      await expect(list.getByRole('listitem')).toHaveCount(3);
      // The first ↑ and the last ↓ are quiet; a move saves at once and keeps focus on the same button.
      await expect(master.getByRole('button', { name: 'Subir Procurar pistas na carroça' })).toHaveAttribute('aria-disabled', 'true');
      await master.getByRole('button', { name: 'Subir Percepção' }).click();
      await expect(list.getByRole('listitem').first()).toContainText('Percepção');
      await expect(master.getByRole('button', { name: 'Subir Percepção' })).toBeFocused();
      await master.getByRole('button', { name: 'Descer Percepção' }).click();
      await expect(list.getByRole('listitem').nth(1)).toContainText('Percepção');
      // It survives a reload (the server has it).
      await master.reload();
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(list.getByRole('listitem').nth(1)).toContainText('Percepção');
      await expect(list.getByRole('listitem').nth(0)).toContainText('CD 12');

      // The session: "Abrir cena".
      await openSessionPage(master, table.campaignId);
      await expect(master.getByRole('heading', { name: 'Cena de RP' })).toBeVisible();
      await openScene(master, 'A carroça tombada');
      await expect(master.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeFocused();
      await expect(master.getByRole('heading', { name: 'Cena de RP' })).toBeHidden();

      // The player: own bonuses, passive values, no DC anywhere.
      await openSessionPage(player, table.campaignId);
      const scene = player.getByRole('region', { name: 'Cena: A carroça tombada' });
      await expect(scene).toBeVisible();
      await expect(scene.getByText('Uma carroça de mercador tombada na estrada.')).toBeVisible();
      const rows = scene.getByRole('listitem');
      await expect(rows).toHaveCount(3);
      await expect(rows.nth(0)).toContainText('+6');
      await expect(rows.nth(0)).toContainText('Investigação passiva 16');
      await expect(rows.nth(1)).toContainText('Percepção passiva 11');
      await expect(rows.nth(2)).toContainText('+3');
      await expect(scene).not.toContainText(/\bCD\b|passou/i);

      // One roll in the app (Investigação, CD 12: the d20 is random, so the pill reads either way).
      await scene.getByRole('button', { name: 'Rolar Procurar pistas na carroça' }).click();
      const sheet = player.getByRole('dialog', { name: 'Rolar Procurar pistas na carroça' });
      await expect(sheet.getByRole('heading', { name: 'Procurar pistas na carroça' })).toBeFocused();
      await expect(sheet.getByText('bônus +6')).toBeVisible();
      await sheet.getByRole('button', { name: 'Rolar no app' }).click();
      await expect(sheet.getByText('Seu total em Investigação')).toBeVisible();
      await expect(sheet.getByText('O mestre vê o resultado.')).toBeVisible();
      await expect(sheet).not.toContainText(/\bCD\b|passou/i);
      await expect(sheet.getByRole('button', { name: 'Voltar à cena' })).toBeFocused();
      await sheet.getByRole('button', { name: 'Voltar à cena' }).click();
      await expect(rows.nth(0)).toContainText('Rolada');
      await expect(rows.nth(0)).toBeFocused();
      await expect(rows.nth(0).getByRole('button')).toHaveCount(0);

      // One with a physical die: Constituição typed 3 is 6, under the DC of 10. A typed number out of 1 to 20 is refused.
      await scene.getByRole('button', { name: 'Rolar Resistir ao cheiro de fumaça' }).click();
      const typed = player.getByRole('dialog', { name: 'Rolar Resistir ao cheiro de fumaça' });
      await typed.getByRole('button', { name: 'Digitar o resultado' }).click();
      await expect(typed.getByRole('heading', { name: 'Digite o resultado do dado' })).toBeVisible();
      await typed.getByLabel(/Role 1d20 para Teste de resistência de Constituição/).fill('27');
      await expect(typed.getByRole('alert')).toContainText('Digite um número de 1 a 20');
      await expect(typed.getByRole('button', { name: 'Confirmar' })).toHaveAttribute('aria-disabled', 'true');
      await typed.getByLabel(/Role 1d20 para Teste de resistência de Constituição/).fill('3');
      await expect(typed.getByRole('status')).toContainText('3 + 3 = 6 · dado físico');
      await typed.getByRole('button', { name: 'Confirmar 3' }).click();
      await expect(typed.getByText('Seu total em Teste de resistência de Constituição')).toBeVisible();
      await typed.getByRole('button', { name: 'Voltar à cena' }).click();
      await expect(rows.nth(2)).toContainText('Rolada');
      // A second roll of the same action is not offered.
      await expect(scene.getByRole('button', { name: /^Rolar / })).toHaveCount(1);
      await expect(scene.getByRole('button', { name: 'Rolar Percepção' })).toBeVisible();

      // The master sees both, newest first, with the formula and Passou / Não passou.
      const rolls = master.getByRole('heading', { name: 'Rolagens' }).locator('xpath=ancestor::section[1]');
      await expect(rolls.getByRole('listitem')).toHaveCount(2);
      const newest = rolls.getByRole('listitem').nth(0);
      await expect(newest).toContainText('Pensantus');
      await expect(newest).toContainText('Resistir ao cheiro de fumaça · dado físico');
      await expect(newest).toContainText('3 + 3 = 6');
      await expect(newest).toContainText('Não passou · CD 10');
      const oldest = rolls.getByRole('listitem').nth(1);
      await expect(oldest).toContainText('Procurar pistas na carroça');
      await expect(oldest).toContainText(/1d20 \(\d+\) \+ 6 = \d+/);
      await expect(oldest).toContainText(/(Passou|Não passou)\s·\sCD\s12/);
      // The master hears the new roll in a polite live region.
      await expect(master.getByRole('status').filter({ hasText: 'Pensantus: Resistir ao cheiro de fumaça, 6, não passou' })).toHaveCount(1);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o jogador rola cada ação uma vez; o mestre fecha a cena sem pergunta e, ao abrir de novo, as ações voltam a poder ser roladas',
  { tag: ['@MR-015', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Cena de novo ${Date.now()}`);
      campaignId = table.campaignId;
      await openSessionPage(master, campaignId);
      await openScene(master, 'A carroça tombada');
      await openSessionPage(player, campaignId);
      const scene = player.getByRole('region', { name: 'Cena: A carroça tombada' });
      await expect(scene.getByRole('button', { name: /^Rolar / })).toHaveCount(5);

      await scene.getByRole('button', { name: 'Rolar Percepção' }).click();
      const sheet = player.getByRole('dialog', { name: 'Rolar Percepção' });
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Percepção/).fill('14');
      await sheet.getByRole('button', { name: 'Confirmar 14' }).click();
      await expect(sheet.getByText('Seu total em Percepção')).toBeVisible();
      await sheet.getByRole('button', { name: 'Voltar à cena' }).click();
      // That action is a result now; the other four still roll.
      await expect(scene.getByRole('button', { name: /^Rolar / })).toHaveCount(4);
      await expect(scene.getByRole('button', { name: 'Rolar Percepção' })).toHaveCount(0);
      await expect(scene.getByText('Rolada')).toHaveCount(1);

      // The master closes: no question, "Cena de RP" is back with focus on "Abrir cena".
      await master.getByRole('button', { name: 'Fechar cena' }).click();
      await expect(master.getByRole('dialog')).toHaveCount(0);
      await expect(master.getByRole('heading', { name: 'Cena de RP' })).toBeVisible();
      await expect(master.getByRole('button', { name: 'Abrir cena', exact: true })).toBeFocused();
      // The player's block goes away and the live region says so.
      await expect(player.getByRole('region', { name: 'Cena: A carroça tombada' })).toHaveCount(0);
      await expect(player.getByText('O mestre fechou a cena.')).toBeAttached();

      // Opened again, the rolls start afresh.
      await openScene(master, 'A carroça tombada');
      await expect(player.getByText('O mestre abriu uma cena: A carroça tombada.')).toBeAttached();
      await expect(scene.getByRole('button', { name: /^Rolar / })).toHaveCount(5);
      await expect(scene.getByText('Rolada')).toHaveCount(0);
      await expect(master.getByText('Ninguém rolou ainda.')).toBeVisible();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre abre uma cena escondida: os jogadores veem a cena e o ponto continua escondido no mapa deles',
  { tag: ['@MR-015', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Cena escondida ${Date.now()}`);
      campaignId = table.campaignId;
      await openSessionPage(master, campaignId);

      // The picker lists the three scene points; every one opens, even the one with no actions (question 63).
      await master.getByRole('button', { name: 'Abrir cena', exact: true }).click();
      const picker = master.getByRole('dialog', { name: 'Abrir uma cena' });
      await expect(picker.getByRole('radio', { name: /Vau do riacho/ })).toBeEnabled();
      await expect(picker.getByText('Sem ações')).toHaveCount(0);
      await expect(picker.getByText('Escondido no mapa · 3 ações')).toBeVisible();
      await picker.getByRole('button', { name: 'Cancelar' }).click();
      await expect(picker).toBeHidden();
      await expect(master.getByRole('button', { name: 'Abrir cena', exact: true })).toBeFocused();

      await openScene(master, 'Posto da guarda');
      await openSessionPage(player, campaignId);
      const scene = player.getByRole('region', { name: 'Cena: Posto da guarda' });
      await expect(scene).toBeVisible();
      await expect(scene.getByRole('listitem')).toHaveCount(3);
      // The point stays hidden on the player's map; the revealed one is there.
      await player.getByRole('link', { name: /Ver mapa/ }).first().click();
      await expect(player.getByRole('button', { name: /^A carroça tombada, Cena de RP/ })).toBeVisible();
      await expect(player.getByRole('button', { name: /Posto da guarda/ })).toHaveCount(0);
      // And still hidden for the master too (nothing was revealed).
      await master.goto(`/campaigns/${campaignId}/maps/${table.mapId}`);
      await expect(master.getByRole('button', { name: /^Posto da guarda, Cena de RP, escondido/ })).toBeVisible();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre abre a cena pelo ponto na lista da sessão, e troca para outra pelo ponto dela',
  { tag: ['@MR-015'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Cena pelo ponto ${Date.now()}`);
      campaignId = table.campaignId;
      await openSessionPage(master, campaignId);
      await openSessionPage(player, campaignId);

      // The list of points: every scene opens, even the one with no actions (question 63).
      const points = master.getByRole('list', { name: 'Pontos do mapa' });
      await expect(points.getByText('Sem ações')).toHaveCount(0);
      await expect(points.getByRole('button', { name: 'Abrir cena Vau do riacho' })).toBeVisible();
      await points.getByRole('button', { name: 'Abrir cena A carroça tombada' }).click();
      await expect(master.getByRole('heading', { name: 'Cena: A carroça tombada' })).toBeFocused();
      await expect(player.getByRole('region', { name: 'Cena: A carroça tombada' })).toBeVisible();

      // The point that is open says so; the other offers to swap.
      await expect(points.getByText('Cena aberta agora')).toHaveCount(1);
      await points.getByRole('button', { name: 'Trocar para a cena Posto da guarda' }).click();
      await expect(master.getByRole('heading', { name: 'Cena: Posto da guarda' })).toBeFocused();
      await expect(player.getByRole('region', { name: 'Cena: Posto da guarda' })).toBeVisible();
      await expect(player.getByRole('region', { name: 'Cena: A carroça tombada' })).toHaveCount(0);
      // The hidden point opened this way is still hidden on the player's map.
      await expect(points.getByText('Escondido').first()).toBeVisible();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre liga a CD da cena e dá 3 tentativas a uma ação; o jogador rola duas vezes e vê Passou e Não passou com as tentativas que restam; depois da última, o mestre dá mais uma',
  { tag: ['@MR-015', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Cena com CD ${Date.now()}`, false);
      campaignId = table.campaignId;
      await addActionRPC(master, table, table.cartId, { key: 'skill:investigation', name: 'Procurar pistas na carroça', dc: 12 });
      await addActionRPC(master, table, table.cartId, { key: 'skill:perception', name: 'Ouvir passos' });

      // The editor: the switch is off at first; it and the attempts save at once.
      await master.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      const dcSwitch = master.getByRole('switch', { name: 'Mostrar a CD aos jogadores' });
      await expect(dcSwitch).toHaveAttribute('aria-checked', 'false');
      await expect(master.getByText('Desligado: só você vê a CD. Os jogadores veem só o resultado.')).toBeVisible();
      await expect(master.getByLabel('Tentativas por jogador')).toHaveCount(2);
      await expect(master.getByLabel('Tentativas por jogador').first()).toHaveValue('1');
      await master.getByLabel('Tentativas por jogador').first().selectOption('3');
      await expect(master.getByRole('status').filter({ hasText: 'Procurar pistas na carroça: 3 tentativas por jogador.' })).toHaveCount(1);
      await dcSwitch.click();
      await expect(dcSwitch).toHaveAttribute('aria-checked', 'true');
      await expect(master.getByText('Ligado: cada jogador vê a CD na ação e, depois de rolar, se passou ou não.')).toBeVisible();
      await expect(master.getByText('Como o jogador vê, antes e depois de rolar')).toBeVisible();
      // It saved at once, like the attempts: "Salvar ponto" has nothing to send.
      await expect(master.getByRole('status').filter({ hasText: 'Os jogadores agora veem a CD.' })).toHaveCount(1);
      await expect(master.getByRole('button', { name: 'Salvar ponto' })).toBeDisabled();
      // Both survive a reload (the server has them).
      await master.reload();
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('switch', { name: 'Mostrar a CD aos jogadores' })).toHaveAttribute('aria-checked', 'true');
      await expect(master.getByLabel('Tentativas por jogador').first()).toHaveValue('3');

      await openSessionPage(master, campaignId);
      await openScene(master, 'A carroça tombada');
      await expect(master.getByText('Os jogadores veem a CD')).toBeVisible();
      await expect(master.getByText('3 tentativas por jogador')).toBeVisible();

      // The player sees the DC and the attempts before rolling.
      await openSessionPage(player, campaignId);
      const scene = player.getByRole('region', { name: 'Cena: A carroça tombada' });
      const clues = scene.getByRole('listitem').filter({ hasText: 'Procurar pistas na carroça' });
      const steps = scene.getByRole('listitem').filter({ hasText: 'Ouvir passos' });
      await expect(clues).toContainText('CD 12');
      await expect(clues).toContainText('Restam 3 de 3 tentativas');
      await expect(steps).toContainText('1 tentativa');
      await expect(steps).not.toContainText('CD');

      // First roll: 3 + 6 = 9 under the DC of 12. The sheet and the row both say "Não passou".
      await scene.getByRole('button', { name: 'Rolar Procurar pistas na carroça' }).click();
      const sheet = player.getByRole('dialog', { name: 'Rolar Procurar pistas na carroça' });
      await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
      await sheet.getByLabel(/Role 1d20 para Investigação/).fill('3');
      await sheet.getByRole('button', { name: 'Confirmar 3' }).click();
      await expect(sheet.getByText('Não passou · CD 12')).toBeVisible();
      await sheet.getByRole('button', { name: 'Voltar à cena' }).click();
      await expect(clues).toContainText('Não passou · CD 12');
      await expect(clues).toContainText('Restam 2 de 3 tentativas');
      // Another try: 10 + 6 = 16 passes. The last result stays and the counter goes down.
      await rollTyped(player, scene, 'Procurar pistas na carroça', 10);
      await expect(clues).toContainText('Passou · CD 12');
      await expect(clues).toContainText('Restam 1 de 3 tentativas');
      await expect(clues.getByRole('button', { name: 'Rolar Procurar pistas na carroça' })).toBeVisible();
      // The last: 2 + 6 = 8. No more attempts, no "Rolar", and the last result is kept.
      await rollTyped(player, scene, 'Procurar pistas na carroça', 2);
      await expect(clues).toContainText('Não passou · CD 12');
      await expect(clues).toContainText('Sem mais tentativas');
      await expect(clues.getByRole('button')).toHaveCount(0);

      // The master reads which attempt each roll was, and "Dar mais uma tentativa" is only on the last, failed, one.
      const rolls = master.getByRole('heading', { name: 'Rolagens' }).locator('xpath=ancestor::section[1]');
      await expect(rolls.getByRole('listitem')).toHaveCount(3);
      await expect(rolls.getByRole('listitem').nth(0)).toContainText('Tentativa 3 de 3');
      await expect(rolls.getByRole('listitem').nth(2)).toContainText('Tentativa 1 de 3');
      const grant = master.getByRole('button', { name: 'Dar mais uma tentativa a Pensantus em Procurar pistas na carroça' });
      await expect(grant).toHaveCount(1);
      await expect(rolls.getByRole('listitem').nth(0).getByRole('button')).toHaveCount(1);

      // It asks in place first: "Voltar" is focused and goes back with no change.
      await grant.click();
      const ask = master.getByRole('alertdialog', { name: 'Dar mais uma tentativa a Pensantus?' });
      await expect(ask).toContainText('Em “Procurar pistas na carroça”. Passa de 3 para 4 tentativas');
      await expect(ask.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await ask.getByRole('button', { name: 'Voltar' }).click();
      await expect(ask).toBeHidden();
      await expect(grant).toBeFocused();
      await expect(clues.getByRole('button')).toHaveCount(0);

      await grant.click();
      await master.getByRole('alertdialog').getByRole('button', { name: 'Dar mais uma tentativa' }).click();
      await expect(master.getByRole('status').filter({ hasText: /Mais uma tentativa dada a Pensantus às \d\d:\d\d\./ })).toBeVisible();
      await expect(rolls.getByRole('listitem').nth(0)).toContainText('Tentativa 3 de 4');
      await expect(grant).toHaveCount(0);

      // The player has it back at once: the row, the button and the live region.
      await expect(clues).toContainText('Restam 1 de 3 tentativas');
      await expect(clues.getByRole('button', { name: 'Rolar Procurar pistas na carroça' })).toBeVisible();
      await expect(player.getByText('O mestre deu mais uma tentativa em Procurar pistas na carroça.')).toBeAttached();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'com a CD desligada o jogador não vê CD nem Passou; o mestre vê o resultado e pode dar mais uma tentativa a quem usou todas, mesmo se passou',
  { tag: ['@MR-015', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Cena sem CD ${Date.now()}`, false);
      campaignId = table.campaignId;
      await addActionRPC(master, table, table.cartId, { key: 'skill:investigation', name: 'Procurar pistas na carroça', dc: 12 });
      const ids = await sceneActionIdsRPC(master, table, table.cartId);
      await setAttemptsRPC(master, table, table.cartId, ids['Procurar pistas na carroça'], 2);
      await openSessionPage(master, campaignId);
      await openScene(master, 'A carroça tombada');
      await expect(master.getByText('Só você vê a CD')).toBeVisible();

      await openSessionPage(player, campaignId);
      const scene = player.getByRole('region', { name: 'Cena: A carroça tombada' });
      const row = scene.getByRole('listitem').filter({ hasText: 'Procurar pistas na carroça' });
      await expect(row).toContainText('Restam 2 de 2 tentativas');
      await expect(scene).not.toContainText(/\bCD\b/);
      // 15 + 6 = 21 would pass a DC of 12, but the player is never told.
      await rollTyped(player, scene, 'Procurar pistas na carroça', 15);
      await rollTyped(player, scene, 'Procurar pistas na carroça', 14);
      await expect(row).toContainText('Sem mais tentativas');
      await expect(row).toContainText('Rolada');
      await expect(scene).not.toContainText(/\bCD\b|passou/i);

      // The master sees both results, and with the DC hidden any exhausted roll may be given another try.
      const rolls = master.getByRole('heading', { name: 'Rolagens' }).locator('xpath=ancestor::section[1]');
      await expect(rolls.getByRole('listitem')).toHaveCount(2);
      await expect(rolls.getByRole('listitem').nth(0)).toContainText('Passou · CD 12');
      await expect(rolls.getByRole('listitem').nth(0)).toContainText('Tentativa 2 de 2');
      await expect(master.getByRole('button', { name: /^Dar mais uma tentativa a Pensantus/ })).toHaveCount(1);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre edita a verificação, o nome e a CD de uma ação pelo lápis: o formulário abre preenchido, só salva com uma mudança, e a linha volta com o foco no lápis',
  { tag: ['@MR-015'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      const table = await tableForScenes(master, player, `Editar ação ${Date.now()}`);
      campaignId = table.campaignId;
      await master.goto(`/campaigns/${table.campaignId}/maps/${table.mapId}`);
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('heading', { name: 'Ações da cena' })).toBeVisible();

      const pencil = master.getByRole('button', { name: 'Editar Procurar pistas na carroça' });
      await pencil.click();
      const form = master.getByRole('form', { name: 'Editar ação' });
      // Filled in with today's values, the focus on the first choice; one form at a time.
      await expect(form.getByRole('radio', { name: 'Perícia' })).toBeChecked();
      await expect(form.getByRole('radio', { name: 'Perícia' })).toBeFocused();
      await expect(form.getByRole('combobox')).toHaveValue('skill:investigation');
      await expect(form.getByLabel('Nome (opcional)')).toHaveValue('Procurar pistas na carroça');
      await expect(form.getByLabel('CD (opcional)')).toHaveValue('12');
      // Nothing changed: "Salvar ação" is quiet and says why.
      const save = form.getByRole('button', { name: 'Salvar ação' });
      await expect(save).toHaveAttribute('aria-disabled', 'true');
      await expect(form.getByText('Mude um campo para salvar.')).toBeVisible();

      // A DC out of 1 to 30 is said under its field and nothing is sent.
      await form.getByLabel('CD (opcional)').fill('35');
      await save.click();
      await expect(form.getByText('A CD vai de 1 a 30. Digite outro número ou deixe em branco.')).toBeVisible();
      await expect(form.getByLabel('CD (opcional)')).toBeFocused();

      await form.getByLabel('Nome (opcional)').fill('Procurar rastros na carroça');
      await form.getByLabel('CD (opcional)').fill('14');
      await form.getByRole('combobox').selectOption({ label: 'Percepção' });
      await save.click();
      await expect(form).toBeHidden();
      const row = master.getByRole('listitem').filter({ hasText: 'Procurar rastros na carroça' });
      await expect(row).toContainText('Percepção');
      await expect(row).toContainText('CD 14');
      await expect(master.getByRole('button', { name: 'Editar Procurar rastros na carroça' })).toBeFocused();
      await expect(master.getByRole('status').filter({ hasText: 'Ação salva: Procurar rastros na carroça.' })).toBeAttached();

      // The server has it.
      await master.reload();
      await master.getByRole('button', { name: /^A carroça tombada, Cena de RP/ }).click();
      await expect(master.getByRole('listitem').filter({ hasText: 'Procurar rastros na carroça' })).toContainText('CD 14');
    } finally {
      await endOpenSessionRPC(master, campaignId);
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o halfling tira 1 num teste de cena: "Tirou 1: rolar de novo?", o novo d20 digitado e "Sortudo: 1 → 15"',
  { tag: ['@MR-014', '@MR-015', '@RN-35', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    try {
      await master.goto('/');
      // Pensantus as a lightfoot halfling: the race's Lucky (Sortudo) asks before the result when a d20 shows a 1.
      const { campaignId } = await tableForScenes(master, player, `Sortudo na cena ${Date.now()}`, true, {
        raceKey: 'race:halfling',
        subraceKey: 'subrace:lightfoot-halfling',
      });
      // The session opened below is always ended, and with no conditional in the test (playwright/no-conditional-in-test).
      try {
        await openSessionPage(master, campaignId);
        await openScene(master, 'A carroça tombada');
        await openSessionPage(player, campaignId);
        const scene = player.getByRole('region', { name: 'Cena: A carroça tombada' });
        await scene.getByRole('button', { name: 'Rolar Procurar pistas na carroça' }).click();
        const sheet = player.getByRole('dialog', { name: 'Rolar Procurar pistas na carroça' });
        await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
        await sheet.getByLabel(/Role 1d20 para Investigação/).fill('1');
        await sheet.getByRole('button', { name: 'Confirmar 1' }).click();

        // A 1 on the d20 (SRD 5.1, Halfling, Lucky): the question comes before the result, and nothing is written yet.
        await expect(sheet.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeVisible();
        await expect(sheet.getByRole('button', { name: 'Ficar com o 1' })).toBeVisible();
        await expect(sheet.getByText('Seu total em Investigação')).toBeHidden();
        await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
        await sheet.getByLabel(/Role 1d20 de novo/).fill('15');
        await sheet.getByRole('button', { name: 'Confirmar 15' }).click();

        // The new roll stands: the total is told with it, and the sheet says what Lucky did.
        await expect(sheet.getByText('Seu total em Investigação')).toBeVisible();
        await expect(sheet.getByText('Sortudo: 1 → 15')).toBeVisible();
        await expect(sheet.getByRole('heading', { name: 'Tirou 1: rolar de novo?' })).toBeHidden();
      } finally {
        await endOpenSessionRPC(master, campaignId);
      }
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o dado de Inspiração de Bardo num teste de cena: a pergunta vem depois do d20 e antes do total, o dado digitado entra na soma e o dado some',
  { tag: ['@MR-014', '@MR-015', '@RN-18'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let bard: Awaited<ReturnType<typeof secondPlayer>> | undefined;
    try {
      await master.goto('/');
      const { campaignId, characterId } = await tableForScenes(master, player, `Dado na cena ${Date.now()}`, true);
      // The session opened below is always ended, and with no conditional in the test (playwright/no-conditional-in-test).
      try {
        // Orla, a bard (a d6), gives Pensantus the die between the fights: the answer to it is what is under test.
        bard = await secondPlayer(browser, master, campaignId, orla, orlaSheet);
        await openSessionPage(master, campaignId);
        const gift = await callRPC(bard.page, 'meurpg.play.v1.ResourceService/GiveBardicInspirationOutside', {
          campaignId,
          targetCharacterId: characterId,
          idempotencyKey: crypto.randomUUID(),
        });
        expect(gift.ok(), await gift.text()).toBeTruthy();
        await openScene(master, 'A carroça tombada');
        await openSessionPage(player, campaignId);
        await expect(player.getByText(/Você tem uma Inspiração de Bardo \(d6\) de Orla/)).toBeVisible();
        const scene = player.getByRole('region', { name: 'Cena: A carroça tombada' });
        await scene.getByRole('button', { name: 'Rolar Procurar pistas na carroça' }).click();
        const sheet = player.getByRole('dialog', { name: 'Rolar Procurar pistas na carroça' });
        await sheet.getByRole('button', { name: 'Digitar o resultado' }).click();
        await sheet.getByLabel(/Role 1d20 para Investigação/).fill('14');
        await sheet.getByRole('button', { name: 'Confirmar 14' }).click();

        // SRD 5.1, Bard: the die is decided on after the d20 and before the result, so the total is not told yet.
        await expect(sheet.getByRole('heading', { name: 'Usar a Inspiração de Bardo (d6)?' })).toBeVisible();
        await expect(sheet.getByText('O resultado só aparece depois da sua escolha.')).toBeVisible();
        await expect(sheet.getByText('Seu total em Investigação')).toBeHidden();
        await sheet.getByRole('button', { name: 'Digitar o dado' }).click();
        await sheet.getByLabel(/Número que saiu no d6/).fill('4');
        await sheet.getByRole('button', { name: 'Usar o dado digitado' }).click();

        // The roll is written with the die: 14 + Pensantus's +6 in Investigação + the 4 of the d6.
        await expect(sheet.getByText('Seu total em Investigação')).toBeVisible();
        await expect(sheet.locator('.res__sum')).toHaveText('24');
        await expect(sheet.getByRole('heading', { name: 'Usar a Inspiração de Bardo (d6)?' })).toBeHidden();

        // The die is spent: Pensantus holds none after a reload.
        await player.reload();
        await expect(player.getByText(/Você tem uma Inspiração de Bardo/)).toBeHidden();
      } finally {
        await endOpenSessionRPC(master, campaignId);
      }
    } finally {
      await bard?.close();
      await masterContext.close();
      await playerContext.close();
    }
  },
);
