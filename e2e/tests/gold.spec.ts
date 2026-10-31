import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC, openSessionPage, startSessionRPC, endSessionFromBar } from './live-session-support';
import { callRPC, newSignedInContext, openXpHistory } from './support';
import { tableForGold, threeTreasuresRPC, treasureFoundRPC } from './gold-support';
import { getExperienceRPC } from './xp-support';

// MR-041 ("Voltar à cidade": the treasures the party found, converted into one XP
// award of a campaign by gold; the history line and "Desfazer") and MR-032 ("Mais
// tesouro encontrado" in the session summary), RN-09, RN-10. Marking a treasure
// found has no screen yet (slice 9.14): the tests do it through the API, and the
// screens under test are the campaign's "Experiência", "Dar XP" and the summary.

const panel = (page: Page) => page.getByRole('region', { name: 'Experiência', exact: true });

test(
  'Voltar à cidade: três tesouros viram um prêmio só; o histórico escreve a linha, o jogador lê só a contagem e o desfazer devolve os tesouros',
  { tag: ['@MR-041', '@RN-09'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForGold(m, p, `Estrada de Ouro ${Date.now()}`);
      await threeTreasuresRPC(m, table);
      await m.goto(`/campaigns/${table.campaignId}`);

      // The strip says what waits; "Voltar à cidade" sits beside "Dar XP", both outlined.
      await expect(panel(m).getByText('Encontrado, ainda não convertido')).toBeVisible();
      await expect(panel(m).getByText('3 tesouros · 420 PO')).toBeVisible();
      await expect(panel(m)).toContainText('Baú de moedas, 250 PO, de Pensantus');
      await expect(panel(m)).toContainText('Bolsa do capitão, 120 PO, de Pensantus');
      await expect(panel(m).getByRole('button', { name: 'Dar XP' })).toBeVisible();
      await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();

      // The dialog: everything checked, the sum and the split written out.
      const dialog = m.getByRole('dialog', { name: 'Voltar à cidade' });
      await expect(dialog).toContainText('Converte o ouro encontrado em XP, 1 XP por PO, num prêmio só. Dá para desfazer.');
      // Three treasures and the one living character (RN-03: a player has one, and the suite has one player).
      await expect(dialog.getByRole('checkbox')).toHaveCount(4);
      for (const box of await dialog.getByRole('checkbox').all()) {
        await expect(box).toBeChecked();
      }
      await expect(dialog).toContainText('420 PO em 3 tesouros = 420 XP');
      await expect(dialog).toContainText('420 XP ÷ 1 = 420 XP para cada');
      await expect(dialog).toContainText('Sobra 0 XP.');

      // Unchecking a treasure changes the line on the spot; checking it again puts it back.
      await dialog.getByRole('checkbox', { name: 'Converter Ídolo de prata' }).uncheck({ force: true });
      await expect(dialog).toContainText('370 PO em 2 tesouros = 370 XP');
      await expect(dialog).toContainText('370 XP ÷ 1 = 370 XP para cada');
      await dialog.getByRole('checkbox', { name: 'Converter Ídolo de prata' }).check({ force: true });
      await expect(dialog).toContainText('420 XP ÷ 1 = 420 XP para cada');

      await dialog.getByRole('button', { name: /Dar 420 XP para cada/ }).click();
      await expect(dialog).toHaveCount(0);

      // The master reads what happened; the strip empties; the one character has 420 XP more (RN-03: the suite has
      // one player, so one character; the split among several is the Vitest's `town-sheet.spec.ts`).
      await expect(panel(m).getByRole('status')).toContainText(
        'Voltar à cidade: Pensantus recebeu 420 XP. Os 3 tesouros foram convertidos.',
      );
      await expect(panel(m)).toContainText('Nenhum tesouro esperando.');
      await expect(panel(m)).toContainText('Voltar à cidade · 420 PO em 3 tesouros');
      await expect(panel(m)).toContainText('Por ouro');
      await expect(panel(m)).toContainText('420 XP para cada');
      await expect(panel(m)).toContainText('Total de 420 XP');
      const after = await getExperienceRPC(m, table.campaignId);
      expect(after.characters.map((c) => c.experiencePoints)).toEqual([420]);

      // A player reads the line and the numbers, with no list of treasures and no buttons.
      await p.goto(`/campaigns/${table.campaignId}`);
      await expect(panel(p)).toContainText('Voltar à cidade · 420 PO em 3 tesouros');
      await expect(panel(p).getByRole('button')).toHaveCount(0);
      await expect(panel(p)).not.toContainText('Baú de moedas');
      await expect(panel(p)).not.toContainText('Encontrado, ainda não convertido');
      const listed = await callRPC(p, 'meurpg.progression.v1.ProgressionService/ListXPAwards', { campaignId: table.campaignId });
      const award = (await listed.json()).awards[0] as Record<string, unknown>;
      expect(award.treasureCount).toBe(3);
      expect(award.gold).toBe(420);
      expect(award.treasures).toBeUndefined();
      const tryList = await callRPC(p, 'meurpg.progression.v1.ProgressionService/ListTreasuresToConvert', { campaignId: table.campaignId });
      expect(tryList.status()).toBe(403);

      // "Desfazer" asks in place and says where the treasures go; the answer frees them. The history is folded
      // in the master's panel: "Histórico de XP (1)" opens it.
      await openXpHistory(m);
      await panel(m).getByRole('button', { name: /^Desfazer/ }).click();
      const question = panel(m).getByRole('alertdialog');
      await expect(question).toContainText('Desfazer o XP de “Voltar à cidade”?');
      await expect(question).toContainText('Pensantus perde 420 XP.');
      await expect(question).toContainText('Os 3 tesouros (420 PO) voltam a “encontrado, não convertido”.');
      await expect(question.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await question.getByRole('button', { name: 'Desfazer XP' }).click();
      await expect(panel(m).getByRole('status').filter({ hasText: 'XP desfeito' })).toContainText(
        'os 3 tesouros voltaram a “encontrado, não convertido”',
      );
      await expect(panel(m).getByText('3 tesouros · 420 PO')).toBeVisible();
      await expect(panel(m).getByText('Desfeito', { exact: true })).toBeVisible();
      expect((await getExperienceRPC(m, table.campaignId)).characters.map((c) => c.experiencePoints ?? 0)).toEqual([0]);

      // And they convert again: one award, the same numbers.
      await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
      await m.getByRole('dialog', { name: 'Voltar à cidade' }).getByRole('button', { name: /Dar 420 XP para cada/ }).click();
      await expect(panel(m).getByRole('status').filter({ hasText: 'foram convertidos' })).toContainText('Os 3 tesouros foram convertidos.');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'Voltar à cidade também sai do "Dar XP"; o tesouro achado fora de uma sessão se diz, e nada esperando explica em vez de abrir uma lista vazia',
  { tag: ['@MR-041', '@RN-09'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForGold(m, p, `Ouro Dar XP ${Date.now()}`);
      await m.goto(`/campaigns/${table.campaignId}`);

      // Nothing found yet: the button stays and the dialog explains.
      await expect(panel(m)).toContainText('Nenhum tesouro esperando. Os que o grupo encontrar aparecem aqui.');
      await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
      const empty = m.getByRole('dialog', { name: 'Voltar à cidade' });
      await expect(empty).toContainText('Nenhum tesouro encontrado para converter.');
      await expect(empty.getByRole('button', { name: /^Dar / })).toHaveCount(0);
      await empty.getByRole('button', { name: 'Fechar', exact: true }).last().click();
      await expect(empty).toHaveCount(0);

      // Found with no session open: it can be converted, and the dialog says it counts in no summary.
      await treasureFoundRPC(m, table, { name: 'Anel de jade', valuePo: 90, finders: [table.characterIds[0]] });
      await m.reload();
      await panel(m).getByRole('button', { name: 'Dar XP' }).click();
      const give = m.getByRole('dialog', { name: 'Dar XP' });
      await expect(give).toContainText('1 tesouro · 90 PO');
      await expect(give).toContainText('ou digite o ouro');
      await give.getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
      await expect(give).toHaveCount(0);
      const town = m.getByRole('dialog', { name: 'Voltar à cidade' });
      await expect(town).toContainText('Anel de jade');
      await expect(town).toContainText('Encontrado por Pensantus');
      await expect(town).toContainText('Um tesouro achado fora de uma sessão não conta em nenhum resumo de sessão.');
      await expect(town).toContainText('fora de uma sessão');
      await expect(town).toContainText('90 XP ÷ 1 = 90 XP para cada');
      // Without the characters nothing goes: the filled button waits and says why.
      await town.getByRole('checkbox', { name: 'Marcar Pensantus' }).uncheck({ force: true });
      await expect(town).toContainText('Marque pelo menos um tesouro e um personagem.');
      await town.getByRole('button', { name: 'Cancelar' }).click();
      await expect(town).toHaveCount(0);
      expect((await getExperienceRPC(m, table.campaignId)).characters.map((c) => c.experiencePoints ?? 0)).toEqual([0]);
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'um tesouro convertido enquanto o mestre olhava: o diálogo diz, lê a lista de novo e deixa o resto da escolha',
  { tag: ['@MR-041'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForGold(m, p, `Ouro corrida ${Date.now()}`);
      const [chest, purse] = await threeTreasuresRPC(m, table);
      await m.goto(`/campaigns/${table.campaignId}`);
      await panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
      const town = m.getByRole('dialog', { name: 'Voltar à cidade' });
      await expect(town).toContainText('420 XP ÷ 1 = 420 XP para cada');

      // Meanwhile another tab converts the chest and the purse.
      const other = await callRPC(m, 'meurpg.progression.v1.ProgressionService/AwardXP', {
        campaignId: table.campaignId,
        mode: 'XP_AWARD_MODE_GOLD',
        reason: 'Voltar à cidade',
        characterIds: table.characterIds,
        treasurePointIds: [chest, purse],
        idempotencyKey: crypto.randomUUID(),
      });
      expect(other.ok(), await other.text()).toBeTruthy();

      await town.getByRole('button', { name: /Dar 420 XP para cada/ }).click();
      await expect(town.getByRole('alert')).toContainText('já virou XP em outro prêmio');
      // The list was read again: only the idol is left, still checked, and the dialog stays open.
      await expect(town.getByRole('checkbox', { name: /^Converter / })).toHaveCount(1);
      await expect(town).toContainText('Ídolo de prata');
      await expect(town).toContainText('50 XP ÷ 1 = 50 XP para cada');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'campanha por inimigos: não há botão, uma linha diz por quê, e o tesouro continua contando no resumo',
  { tag: ['@MR-041', '@MR-032', '@RN-09'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForGold(m, p, `Mirathel ouro ${Date.now()}`, 'XP_MODE_ENEMIES');
      await m.goto(`/campaigns/${table.campaignId}`);
      // Nothing found: no strip at all.
      await expect(panel(m).getByRole('button', { name: 'Dar XP' })).toBeVisible();
      await expect(panel(m)).not.toContainText('Tesouro encontrado');

      const chest = await treasureFoundRPC(m, table, { name: 'Baú de moedas', valuePo: 250, finders: [table.characterIds[0]] });
      await m.reload();
      await expect(panel(m).getByText('1 tesouro · 250 PO')).toBeVisible();
      await expect(panel(m)).toContainText('Esta campanha dá XP por inimigos, então o tesouro não vira XP. Ele aparece no resumo de cada sessão.');
      await expect(panel(m).getByRole('button', { name: 'Voltar à cidade', exact: true })).toHaveCount(0);
      await panel(m).getByRole('button', { name: 'Dar XP' }).click();
      const give = m.getByRole('dialog', { name: 'Dar XP' });
      await expect(give.getByRole('button', { name: 'Voltar à cidade', exact: true })).toHaveCount(0);
      await give.getByRole('button', { name: 'Cancelar' }).click();

      // The server refuses the conversion in this mode, in the treasure's words (the API, as the screen has no way in).
      const refused = await callRPC(m, 'meurpg.progression.v1.ProgressionService/AwardXP', {
        campaignId: table.campaignId,
        mode: 'XP_AWARD_MODE_GOLD',
        reason: 'Voltar à cidade',
        characterIds: table.characterIds,
        treasurePointIds: [chest],
        idempotencyKey: crypto.randomUUID(),
      });
      expect(refused.status()).toBe(400);
      const refusal = await refused.json();
      expect(refusal.code).toBe('failed_precondition');
      expect(refusal.details[0].debug.reason).toBe('XP_BLOCKED_REASON_MODE_NOT_ALLOWED');

      // A player never sees the strip.
      await p.goto(`/campaigns/${table.campaignId}`);
      await expect(panel(p)).not.toContainText('Tesouro encontrado');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'o resumo da sessão tem "Mais tesouro encontrado": o mestre vê quem achou quanto, o jogador vê a categoria no cartão; o que foi achado fora da sessão não conta',
  { tag: ['@MR-032', '@MR-041'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForGold(m, p, `Resumo ouro ${Date.now()}`, 'XP_MODE_ENEMIES');
      campaignId = table.campaignId;
      const [pensantus] = table.characterIds;
      // Found before the session: it counts in no summary.
      await treasureFoundRPC(m, table, { name: 'Moeda antiga', valuePo: 999, finders: [pensantus] });
      await startSessionRPC(m, campaignId);
      await threeTreasuresRPC(m, table);

      await openSessionPage(p, campaignId);
      await openSessionPage(m, campaignId);
      await endSessionFromBar(m);
      await expect(m.getByRole('heading', { name: 'Sessão encerrada' })).toBeVisible();

      // The master: one row for everyone who found something, with the PO (the find before the session is out).
      const block = m.getByRole('table', { name: 'Mais tesouro encontrado' });
      await expect(m.getByText('Só conta o que foi marcado durante a sessão.')).toBeVisible();
      await expect(block.getByRole('row')).toHaveCount(2);
      await expect(block.getByRole('row').nth(1)).toContainText('Pensantus');
      await expect(block.getByRole('row').nth(1)).toContainText('420 PO');
      await expect(block).not.toContainText('999');
      await expect(m.getByText('Um tesouro encontrado por duas pessoas divide o valor, arredondado para baixo.')).toBeVisible();
      await expect(m.getByText('Nenhum tesouro registrado nesta sessão.')).toHaveCount(0);

      // The player: the category is a tile of their card, with the winner.
      const card = p.getByRole('region', { name: 'Resumo da sessão' });
      await expect(card.getByRole('heading', { name: 'A sessão acabou' })).toBeVisible();
      const tile = card.getByRole('listitem').filter({ hasText: 'Mais tesouro encontrado' });
      await expect(tile).toContainText('420 PO');
      await expect(tile).toContainText('Pensantus');
      await expect(card.getByRole('button')).toHaveCount(2); // the ✕ and "Fechar"
      await expect(card.getByRole('table')).toHaveCount(0);
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
  'Voltar à cidade também sai do "Dar XP" da sessão ao vivo: a faixa já traz o que espera, a conversão abre e a confirmação fica na sessão',
  { tag: ['@MR-041', '@RN-09'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForGold(m, p, `Ouro sessão ${Date.now()}`);
      campaignId = table.campaignId;
      await threeTreasuresRPC(m, table);
      await startSessionRPC(m, campaignId);
      await openSessionPage(m, campaignId, 'grupo');

      await m.getByRole('button', { name: 'Dar XP', exact: true }).click();
      const give = m.getByRole('dialog', { name: 'Dar XP' });
      // The strip says what waits (never a false "nenhum" while it reads) and has the way in.
      await expect(give).toContainText('3 tesouros · 420 PO');
      await expect(give).not.toContainText('Nenhum tesouro esperando');
      await give.getByRole('button', { name: 'Voltar à cidade' }).click();
      const town = m.getByRole('dialog', { name: 'Voltar à cidade' });
      await expect(town).toContainText('420 XP ÷ 1 = 420 XP para cada');
      await town.getByRole('button', { name: /Dar 420 XP para cada/ }).click();

      await expect(m.getByRole('status').filter({ hasText: 'foram convertidos' })).toContainText(
        'Voltar à cidade: Pensantus recebeu 420 XP. Os 3 tesouros foram convertidos.',
      );
      expect((await getExperienceRPC(m, campaignId)).characters.map((c) => c.experiencePoints)).toEqual([420]);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId);
      }
      await master.close();
      await player.close();
    }
  },
);
