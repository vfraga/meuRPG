import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, startSessionRPC } from './live-session-support';
import {
  callRPC,
  newSignedInContext,
  openXpHistory,
} from './support';
import { awardXpRPC, createEnemyRPC, getExperienceRPC, tableForXp, tableForXpCombat, winCombatRPC } from './xp-support';

// MR-016 (XP: by defeated enemies, by gold, by milestones, whenever the master
// wants, with a history), RN-09 (the campaign's mode), RN-12 (the warning when a
// character can level up). The data comes through the API; the screens are what
// is under test. Every test makes its own campaign.

const panel = (page: Page) => page.getByRole('region', { name: 'Experiência', exact: true });
const sheetOf = (page: Page, campaignId: string, characterId: string) => `/campaigns/${campaignId}/characters/${characterId}`;

/** Waits until the page's session stream is open, so a later `xp_changed` reaches it. */
async function streamOpen(page: Page): Promise<void> {
  await page.waitForResponse((r) => r.url().includes('PlayService/WatchGameSession'), { timeout: 45_000 });
}

test(
  'o XP de um combate: o mestre dá pelo resumo, a ficha do jogador sobe ao vivo e o histórico guarda',
  { tag: ['@MR-016', '@RN-09', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      // Pensantus is 100 XP short of level 4 (2.700); the Capitão (200) and three Goblins (50 each) fall: 350 XP.
      const combat = await tableForXpCombat(m, p, `XP do combate ${Date.now()}`, { experiencePoints: 2600 });
      campaignId = combat.table.campaignId;
      await winCombatRPC(m, combat);

      // The player reads the sheet (a block, not a field) and waits for news from the session's stream.
      const stream = streamOpen(p);
      await p.goto(sheetOf(p, campaignId, combat.table.characterId));
      const xp = p.locator('app-xp-block');
      await expect(xp).toContainText('2.600 XP');
      await expect(xp).toContainText('2.600 de 2.700 XP para o nível 4. Faltam 100 XP.');
      await expect(xp.getByRole('textbox')).toHaveCount(0);
      await expect(p.getByText('Pode subir de nível')).toHaveCount(0);
      await stream;

      // The summary: what the defeated are worth, who receives, the division, and one filled button.
      await m.goto(`/campaigns/${campaignId}/session?vista=grupo`);
      await expect(m.getByRole('heading', { name: 'Combate encerrado' })).toBeVisible();
      const block = m.getByRole('region', { name: 'Experiência do combate' });
      await expect(block).toContainText('Total dos 4 derrotados');
      await expect(block).toContainText('200 + 50 + 50 + 50');
      await expect(block.getByRole('checkbox', { name: 'Marcar Pensantus' })).toBeChecked();
      await expect(block.locator('.split__big')).toHaveText('350 XP para cada');
      await expect(m.getByRole('button', { name: 'Voltar à sessão' })).toHaveClass(/mat-mdc-outlined-button/);

      await block.getByRole('button', { name: 'Dar 350 XP a cada um' }).click();
      const done = block.getByRole('status').filter({ hasText: '350 XP dados' });
      await expect(done).toBeFocused();
      await expect(done).toContainText('350 XP dados: 350 para cada.');
      await expect(block).toContainText('2.600 + 350 = 2.950 XP');
      await expect(block.getByText('Pode subir de nível')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Voltar à sessão' })).toHaveClass(/mat-mdc-unelevated-button/);

      // The player's sheet changes by itself, no reload, and says what to do (RN-12).
      await expect(xp).toContainText('2.950 XP');
      // Since MR-040 the owner of the locked sheet levels up by the block under the header (it says it, once).
      await expect(p.getByRole('heading', { name: 'Pensantus pode subir de nível' })).toBeVisible();
      await expect(xp).toContainText('Chegou aos 2.700 XP do nível 4. Suba o nível pelo botão abaixo.');

      // The campaign page: the history, and the tag in the group list.
      await m.goto(`/campaigns/${campaignId}`);
      const history = panel(m);
      await expect(history).toContainText('Combate: Emboscada na estrada');
      await expect(history).toContainText('Por inimigos');
      await expect(history).toContainText('350 XP para cada');
      await expect(history).toContainText('Total de 350 XP');
      await expect(m.getByRole('region', { name: 'Personagens' }).getByText('Pode subir de nível')).toBeVisible();

      // Giving it again is refused, in words (the combat was paid).
      const again = await callRPC(m, 'meurpg.progression.v1.ProgressionService/AwardXP', {
        campaignId,
        idempotencyKey: crypto.randomUUID(),
        mode: 'XP_AWARD_MODE_ENEMIES',
        reason: 'de novo',
        encounterId: (await (await callRPC(m, 'meurpg.play.v1.CombatService/GetEncounter', { campaignId })).json()).encounter.id,
        characterIds: [combat.table.characterId],
      });
      expect(again.status()).toBe(400);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await master.close();
      await player.close();
    }
  },
);

test(
  '"Agora não" deixa o XP do combate para depois, e a linha abre "Dar XP" com o motivo e o total',
  { tag: ['@MR-016', '@RN-09'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const combat = await tableForXpCombat(m, p, `XP depois ${Date.now()}`);
      campaignId = combat.table.campaignId;
      await winCombatRPC(m, combat);
      await m.goto(`/campaigns/${campaignId}/session?vista=grupo`);
      const block = m.getByRole('region', { name: 'Experiência do combate' });

      await block.getByRole('button', { name: 'Agora não' }).click();
      const later = block.getByRole('button', { name: /XP do combate ainda não dado/ });
      await expect(later).toBeFocused();
      await expect(m.getByRole('button', { name: 'Voltar à sessão' })).toHaveClass(/mat-mdc-unelevated-button/);
      await expect(m.getByRole('button', { name: /^Dar \d+ XP/ })).toHaveCount(0);

      await later.click();
      const dialog = m.getByRole('dialog', { name: 'Dar XP' });
      await expect(dialog.getByLabel('Motivo')).toHaveValue('Combate: Emboscada na estrada');
      await expect(dialog.getByLabel('XP para o grupo')).toHaveValue('350');
      await dialog.getByRole('button', { name: 'Dar 350 XP a cada um' }).click();
      await expect(dialog).toHaveCount(0);
      await expect(block).toContainText('350 XP dados');

      // It is still the combat's award: the history says "Por inimigos", and it cannot be given twice.
      await m.goto(`/campaigns/${campaignId}`);
      await expect(panel(m)).toContainText('Por inimigos');
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await master.close();
      await player.close();
    }
  },
);

test('um prêmio avulso, a qualquer hora: os erros aparecem ao sair do campo e o histórico mostra o prêmio', { tag: ['@MR-016', '@RN-09'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  const m = await master.newPage();
  const p = await player.newPage();
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForXp(m, p, `XP avulso ${Date.now()}`, 'XP_MODE_ENEMIES', { experiencePoints: 2600 });
    await m.goto(`/campaigns/${table.campaignId}`);
    await expect(panel(m)).toContainText('Ninguém recebeu XP ainda');
    await expect(panel(m)).toContainText('0 de 2.700 XP'.replace('0 de', '2.600 de'));

    await panel(m).getByRole('button', { name: 'Dar XP' }).click();
    const dialog = m.getByRole('dialog', { name: 'Dar XP' });
    await expect(dialog.getByLabel('Motivo')).toBeFocused();
    const give = dialog.getByRole('button', { name: 'Dar XP', exact: true }).last();
    await expect(give).toHaveAttribute('aria-disabled', 'true');

    // Typing shows no error; leaving the field does.
    await dialog.getByLabel('XP para o grupo').fill('0');
    await expect(dialog.getByText('Digite um valor de 1 a 1.000.000.')).toHaveCount(0);
    await dialog.getByLabel('Motivo').focus();
    await expect(dialog.getByText('Digite um valor de 1 a 1.000.000.')).toBeVisible();
    await dialog.getByLabel('Motivo').fill('Pelo resgate do mercador');
    await expect(dialog).toContainText('24 de 120');
    await dialog.getByLabel('XP para o grupo').fill('150');
    await expect(dialog.locator('.split__sum')).toHaveText('150 XP ÷ 1 = 150');

    await dialog.getByRole('button', { name: 'Dar 150 XP a cada um' }).click();
    await expect(dialog).toHaveCount(0);
    // The button that opened the dialog has the focus back, and the news is said.
    await expect(panel(m).getByRole('button', { name: 'Dar XP' })).toBeFocused();
    await expect(panel(m).getByRole('status').filter({ hasText: '150 XP dados' })).toBeVisible();
    await expect(panel(m)).toContainText('2.750 de 2.700 XP');
    await expect(panel(m)).toContainText('Pelo resgate do mercador');
    await expect(panel(m)).toContainText('Avulso');
    await expect(panel(m).getByText('Pode subir de nível')).toBeVisible();

    // The player reads the same history, without actions.
    await p.goto(`/campaigns/${table.campaignId}`);
    await expect(panel(p)).toContainText('Pelo resgate do mercador');
    await expect(panel(p)).toContainText('Todos da campanha veem este histórico.');
    await expect(panel(p).getByRole('button')).toHaveCount(0);
  } finally {
    await master.close();
    await player.close();
  }
});

test('por ouro: o mestre digita as peças de ouro e cada um recebe a sua parte', { tag: ['@MR-016', '@RN-09'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  const m = await master.newPage();
  const p = await player.newPage();
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForXp(m, p, `XP ouro ${Date.now()}`, 'XP_MODE_GOLD');
    await m.goto(`/campaigns/${table.campaignId}`);
    await expect(panel(m)).toContainText('XP por ouro encontrado.');

    await panel(m).getByRole('button', { name: 'Dar XP' }).click();
    const dialog = m.getByRole('dialog', { name: 'Dar XP' });
    await expect(dialog).toContainText('Campanha por ouro: 1 XP por peça de ouro (PO).');
    // The sheet puts the focus on "Motivo" when its opening animation ends:
    // typing before that, the late focus can land "120" in the wrong field.
    await expect(dialog.getByLabel('Motivo')).toBeFocused();
    await dialog.getByLabel('Motivo').fill('O baú do Capitão');
    await dialog.getByLabel('Ouro encontrado (PO)').fill('120');
    await expect(dialog).toContainText('Vale 1 XP por PO: 120 PO são 120 XP.');
    await dialog.getByRole('button', { name: 'Dar 120 XP a cada um' }).click();

    await expect(panel(m)).toContainText('O baú do Capitão');
    await expect(panel(m)).toContainText('Por ouro');
    await expect(panel(m)).toContainText('120 de 2.700 XP');
  } finally {
    await master.close();
    await player.close();
  }
});

test('por marcos: o marco marca quem pode subir de nível, sem nenhum número de XP, e a marca some quando o mestre sobe o nível', { tag: ['@MR-016', '@RN-09', '@RN-12'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
  const m = await master.newPage();
  const p = await player.newPage();
  let campaignId = '';
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForXp(m, p, `XP marcos ${Date.now()}`, 'XP_MODE_MILESTONES');
    campaignId = table.campaignId;
    await startSessionRPC(m, campaignId);

    await m.goto(`/campaigns/${campaignId}`);
    await expect(panel(m)).toContainText('Campanha por marcos');
    await expect(panel(m)).toContainText('Nenhum marco planejado');
    await expect(panel(m).getByRole('button', { name: 'Dar XP' })).toHaveCount(0);

    await panel(m).getByRole('button', { name: 'Registrar um marco fora da lista' }).click();
    const dialog = m.getByRole('dialog', { name: 'Registrar marco' });
    await expect(dialog.getByLabel('O que aconteceu')).toBeFocused();
    await expect(dialog).not.toContainText(/\d\s*XP/);
    await expect(dialog).toContainText('1 personagem pode subir de nível');
    await dialog.getByLabel('O que aconteceu').fill('Marco: a ponte do rio foi salva');
    await dialog.getByRole('button', { name: 'Registrar marco' }).click();

    await expect(panel(m).getByRole('status').filter({ hasText: 'Marco registrado' })).toContainText('todos podem subir de nível');
    await expect(panel(m).getByRole('button', { name: 'Registrar um marco fora da lista' })).toBeFocused();
    await expect(panel(m)).toContainText('Marco: a ponte do rio foi salva');
    await expect(panel(m).getByText('Pode subir de nível')).toBeVisible();
    await expect(panel(m)).not.toContainText(/\d\s*XP/);
    await expect(m.getByRole('region', { name: 'Personagens' }).getByText('Pode subir de nível')).toBeVisible();

    // The player's sheet: no XP anywhere, only the tag.
    await p.goto(sheetOf(p, campaignId, table.characterId));
    // The block of the level-up (MR-040) says it, not the header's tag.
    await expect(p.getByRole('heading', { name: 'Pensantus pode subir de nível' })).toBeVisible();
    await expect(p.locator('app-xp-block')).toHaveCount(0);
    await expect(p.locator('app-sheet-header')).not.toContainText('XP');

    // The master raises the level on the sheet: the mark goes away.
    await m.goto(`/campaigns/${campaignId}/characters/${table.characterId}/edit`);
    await m.getByLabel('Nível', { exact: true }).fill('4');
    await m.getByRole('button', { name: 'Salvar ficha' }).click();
    await expect(m).toHaveURL(sheetOf(m, campaignId, table.characterId));
    await expect(m.locator('app-sheet-header').getByText('Mago 4')).toBeVisible();
    await expect(m.getByText('Pode subir de nível')).toHaveCount(0);
    await p.reload();
    await expect(p.getByRole('heading', { name: 'Pensantus' })).toBeVisible();
    await expect(p.getByText('Pode subir de nível')).toHaveCount(0);
  } finally {
    if (campaignId) {
      await endOpenSessionRPC(m, campaignId);
    }
    await master.close();
    await player.close();
  }
});

test('desfazer o último prêmio: a pergunta fica no lugar, o foco vai para "Voltar", e o histórico guarda o desfazer', { tag: ['@MR-016'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  const m = await master.newPage();
  const p = await player.newPage();
  try {
    await m.goto('/');
    await p.goto('/');
    const table = await tableForXp(m, p, `XP desfazer ${Date.now()}`, 'XP_MODE_ENEMIES', { experiencePoints: 2600 });
    await awardXpRPC(m, table.campaignId, { mode: 'MANUAL', reason: 'Pelo resgate do mercador', characterIds: [table.characterId], amount: 40 });
    await awardXpRPC(m, table.campaignId, { mode: 'MANUAL', reason: 'Pela ajuda ao ferreiro', characterIds: [table.characterId], amount: 150 });
    await m.goto(`/campaigns/${table.campaignId}`);

    // The history is folded in the master's panel: "Histórico de XP (2)" opens it.
    await openXpHistory(m);

    // Only the last award has "Desfazer".
    await expect(panel(m).getByRole('button', { name: /^Desfazer/ })).toHaveCount(1);
    await expect(panel(m).getByText('Só o último prêmio pode ser desfeito.')).toBeVisible();
    await panel(m).getByRole('button', { name: /^Desfazer/ }).click();

    const question = panel(m).getByRole('alertdialog');
    await expect(question).toContainText('Desfazer o XP de “Pela ajuda ao ferreiro”?');
    await expect(question).toContainText('Pensantus perde 150 XP.');
    await expect(question).toContainText('Pensantus volta para 2.640 XP e deixa de poder subir de nível.');
    await expect(question.getByRole('button', { name: 'Voltar' })).toBeFocused();

    // "Voltar" puts the focus back on "Desfazer" and sends nothing.
    await question.getByRole('button', { name: 'Voltar' }).click();
    await expect(panel(m).getByRole('alertdialog')).toHaveCount(0);
    await expect(panel(m).getByRole('button', { name: /^Desfazer/ })).toBeFocused();
    await expect(panel(m)).toContainText('2.790 de 2.700 XP');

    // Esc does the same.
    await panel(m).getByRole('button', { name: /^Desfazer/ }).click();
    await expect(panel(m).getByRole('alertdialog').getByRole('button', { name: 'Voltar' })).toBeFocused();
    await m.keyboard.press('Escape');
    await expect(panel(m).getByRole('alertdialog')).toHaveCount(0);
    await expect(panel(m).getByRole('button', { name: /^Desfazer/ })).toBeFocused();

    // Someone gave another award meanwhile: the question is stale, the call says so and the page reads again.
    await panel(m).getByRole('button', { name: /^Desfazer/ }).click();
    await awardXpRPC(m, table.campaignId, { mode: 'MANUAL', reason: 'Por falar com o mercador', characterIds: [table.characterId], amount: 10 });
    await panel(m).getByRole('alertdialog').getByRole('button', { name: 'Desfazer XP' }).click();
    await expect(panel(m).getByRole('status').filter({ hasText: 'O histórico mudou' })).toBeVisible();
    await expect(panel(m)).toContainText('Por falar com o mercador');
    await expect((await getExperienceRPC(m, table.campaignId)).characters[0].experiencePoints).toBe(2800);

    // Now it is the last one: undo it for real. The line stays, with "Desfeito".
    await panel(m).getByRole('button', { name: /^Desfazer/ }).click();
    await panel(m).getByRole('alertdialog').getByRole('button', { name: 'Desfazer XP' }).click();
    await expect(panel(m).getByRole('status').filter({ hasText: 'XP desfeito' })).toBeVisible();
    await expect(panel(m).getByText('Desfeito', { exact: true })).toBeVisible();
    await expect(panel(m)).toContainText('Por falar com o mercador');
    await expect(panel(m)).toContainText('2.790 de 2.700 XP');
    // The one before it is the last that stands now.
    await expect(panel(m).getByRole('button', { name: /^Desfazer/ })).toHaveCount(1);
  } finally {
    await master.close();
    await player.close();
  }
});

test('o XP que o NPC dá: o ND preenche o XP, o valor digitado fica e "Usar" volta ao da tabela', { tag: ['@MR-016', '@RN-20'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const m = await master.newPage();
  try {
    await m.goto('/');
    const created = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `XP do NPC ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
    const campaignId = (await created.json()).campaign.id as string;

    // A minion: a short sheet with "Ao ser derrotado".
    await m.goto(`/campaigns/${campaignId}/npcs/new/minion`);
    await m.getByLabel('Nome do personagem').fill('Goblin');
    await expect(m.getByRole('heading', { name: 'Ao ser derrotado' })).toBeVisible();
    const xp = m.getByLabel('XP ao derrotar');
    await expect(xp).toHaveValue('10');

    const nd = m.getByRole('combobox', { name: 'Nível de desafio (ND)' });
    await nd.click();
    await expect(m.getByRole('option', { name: /^ND 1\/4/ })).toContainText('50 XP');
    await m.getByRole('option', { name: /^ND 1\/4/ }).click();
    await expect(xp).toHaveValue('50');
    await expect(m.getByText('Da tabela: 50 XP. O mestre pode digitar outro valor.')).toBeVisible();

    // A typed value stays while the ND does not change, and "Usar 50 XP" brings the table's back.
    await xp.fill('0');
    await expect(m.getByText('Este NPC não dá XP. Da tabela: 50 XP.')).toBeVisible();
    await m.getByRole('button', { name: 'Usar 50 XP' }).click();
    await expect(xp).toHaveValue('50');
    await expect(xp).toBeFocused();

    // Changing the ND always refills it, even over a typed value.
    await xp.fill('70');
    await nd.click();
    await m.getByRole('option', { name: /^ND 1\/2/ }).click();
    await expect(xp).toHaveValue('100');

    // "Remover ataque" is a text button, not a bin beside the fields.
    await m.getByRole('button', { name: 'Adicionar ataque' }).click();
    await expect(m.getByRole('button', { name: 'Remover ataque 1' })).toHaveText('Remover ataque');

    await m.getByLabel('Nome do ataque').fill('Cimitarra');
    // The attack's fields sit in a compact grid where a click on the select can land on its label: the keyboard opens it.
    const damageType = m.getByRole('combobox', { name: 'Tipo de dano' });
    await damageType.focus();
    await damageType.press('Enter');
    await m.getByRole('option', { name: 'Cortante' }).click();
    await m.getByRole('button', { name: 'Criar NPC' }).click();
    await expect(m).toHaveURL(/\/characters\/(?!new)[^/]+$/);

    // The master reads what it gives on the sheet; the editor still has it (the save kept both).
    await expect(m.locator('app-sheet-header')).toContainText('ND 1/2');
    await expect(m.locator('app-sheet-header')).toContainText('100 XP');
    await m.goto(`${m.url()}/edit`);
    await expect(m.getByLabel('XP ao derrotar')).toHaveValue('100');
    await m.getByLabel('Nome do personagem').fill('Goblin batedor');
    await m.getByRole('button', { name: 'Salvar ficha' }).click();
    await expect(m.locator('app-sheet-header')).toContainText('100 XP');

    // An enemy (a full sheet): the same two fields in the first step.
    const captain = await createEnemyRPC(m, campaignId, 'Capitão Goblin', '1', 200);
    await m.goto(`/campaigns/${campaignId}/characters/${captain}/edit`);
    await expect(m.getByLabel('XP ao derrotar')).toHaveValue('200');
    await m.getByRole('combobox', { name: 'Nível de desafio (ND)' }).click();
    await m.getByRole('option', { name: /^ND 2$|^ND 2 / }).first().click();
    await expect(m.getByLabel('XP ao derrotar')).toHaveValue('450');
    await m.getByRole('button', { name: 'Salvar ficha' }).click();
    await expect(m.locator('app-sheet-header')).toContainText('450 XP');

    // A player character never has them.
    await m.goto(`/campaigns/${campaignId}/characters/new`);
    await expect(m.getByLabel('Nome do personagem')).toBeVisible();
    await expect(m.getByLabel('XP ao derrotar')).toHaveCount(0);
  } finally {
    await master.close();
  }
});
