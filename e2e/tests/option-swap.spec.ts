import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endOpenSessionRPC } from './live-session-support';
import { markMilestoneRPC, passClassStep, tableForLevelUp } from './levelup-support';
import { allowRulePickSwapRPC, doran, postureSheetRPC } from './option-swap-support';
import { newSignedInContext } from './support';

// #715: a table feature's options may be swapped at the level-up where its `swap_levels` say so, whatever the table's swap
// rule is and at a level with no ability step. The panel "Trocar escolhas" is then one more panel of the "Escolhas" step.

test(
  'Doran, Guerreiro 4 do Mestre das Posturas, troca a Postura da Garça pela do Lobo no nível 5, que não tem habilidades: o painel está em Escolhas e a ficha mostra a postura nova',
  { tag: ['@MR-040', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 800 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Troca de postura ${Date.now()}`, {
        build: doran,
        prepare: (id) => postureSheetRPC(m, id, 4, [5]),
      });
      campaignId = table.campaignId;
      const sheet = `/campaigns/${campaignId}/characters/${table.characterId}`;
      await p.goto(`${sheet}/level-up`);

      // Classe, Vida, Escolhas, Resumo: the level has no ability step, and the swap is the one panel of Escolhas.
      await expect(p.getByRole('heading', { name: 'Subir para o nível 5' })).toBeVisible();
      await expect(p.getByText(/Passo 1 de 4 · Classe/)).toBeVisible();
      await passClassStep(p);
      await expect(p.getByText('Passo 2 de 4 · Vida')).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();
      await expect(p.getByText('Passo 3 de 4 · Escolhas')).toBeVisible();
      const panel = p.getByRole('region', { name: 'Trocar escolhas' });
      await expect(panel).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Trocar escolhas' })).toHaveCount(1);
      await expect(panel.getByRole('radio', { name: 'Não trocar' })).toBeChecked();
      await panel.getByText('Posturas', { exact: true }).click();
      await expect(panel.getByRole('radio', { name: 'Posturas' })).toBeChecked();
      await panel.getByRole('radiogroup', { name: 'Abrir mão de' }).getByText('Postura da Garça').click();
      await expect(panel.getByRole('status')).toContainText('Falta escolher. Posturas: escolha de que abrir mão e o que pegar no lugar.');
      await expect(panel.getByText('Trocas escolhidas')).toHaveCount(0);
      await panel.getByRole('radiogroup', { name: 'Pegar no lugar' }).getByText('Postura do Lobo').click();
      await expect(panel.getByRole('status')).toHaveText('Trocar Postura da Garça por Postura do Lobo');
      await expect(panel.getByRole('list', { name: 'Trocas escolhidas' }).getByRole('listitem')).toHaveText(['Posturas: Postura da Garça → Postura do Lobo']);
      await expect(panel.getByText('Vale ao confirmar o nível.')).toBeVisible();
      await p.getByRole('button', { name: 'Próximo' }).click();

      await expect(p.getByText('Passo 4 de 4 · Resumo')).toBeVisible();
      await expect(p.getByRole('listitem').filter({ hasText: 'Posturas' }).filter({ hasText: 'Postura da Garça' }).filter({ hasText: 'Postura do Lobo' })).toBeVisible();
      await p.getByRole('button', { name: 'Confirmar o nível 5' }).click();

      // The sheet: the Lobo is picked and the Garça is not.
      await expect(p).toHaveURL(sheet);
      await expect(p.getByText('Doran subiu para o nível 5.').first()).toBeVisible();
      await expect(p.getByText('Postura do Lobo').first()).toBeVisible();
      await expect(p.getByText('Postura do Touro').first()).toBeVisible();
      await expect(p.getByText('Postura da Garça')).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'Doran deixa a troca de postura pela metade e aperta Próximo: o passo não avança, "Falta escolher." aparece, o foco vai ao conjunto pela metade, e "Não trocar" libera o passo',
  { tag: ['@MR-040', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(150_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 800 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Troca pela metade ${Date.now()}`, { build: doran, prepare: (id) => postureSheetRPC(m, id, 4, [5]) });
      campaignId = table.campaignId;
      const sheet = `/campaigns/${campaignId}/characters/${table.characterId}`;
      await p.goto(`${sheet}/level-up`);
      await passClassStep(p);
      await p.getByRole('button', { name: 'Próximo' }).click();
      await expect(p.getByText('Passo 3 de 4 · Escolhas')).toBeVisible();
      const panel = p.getByRole('region', { name: 'Trocar escolhas' });
      await panel.getByText('Posturas', { exact: true }).click();
      await panel.getByRole('radiogroup', { name: 'Abrir mão de' }).getByText('Postura da Garça').click();
      await expect(panel.getByText('Trocas escolhidas')).toHaveCount(0);

      // Half a swap is a choice left half done, as any other of the level-up: "Próximo" waits, a tap does not advance.
      const next = p.getByRole('button', { name: 'Próximo' });
      await expect(next).toHaveClass(/next--off/);
      await expect(p.locator('#foot-reason')).toContainText('Falta escolher o que trocar em Posturas.');
      // "Próximo" is aria-disabled but still answers a press, as on main.
      await next.press('Enter');
      await expect(p.getByText('Passo 3 de 4 · Escolhas')).toBeVisible();
      await expect(panel.getByRole('status')).toContainText('Falta escolher.');
      await expect(panel.getByRole('radio', { name: /Posturas/ })).toBeFocused();
      await expect(panel).toHaveAttribute('data-attn', '');

      // "Não trocar" clears it: the step goes on, the summary has no row, the sheet keeps the Garça.
      await panel.getByText('Não trocar', { exact: true }).click();
      await expect(next).not.toHaveClass(/next--off/);
      await next.click();
      await expect(p.getByText('Passo 4 de 4 · Resumo')).toBeVisible();
      await expect(p.getByText('Postura do Lobo')).toHaveCount(0);
      await expect(p.getByRole('listitem').filter({ hasText: 'Posturas' })).toHaveCount(0);
      await p.getByRole('button', { name: 'Confirmar o nível 5' }).click();
      await expect(p).toHaveURL(sheet);
      await expect(p.getByText('Postura da Garça').first()).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'Doran troca uma opção de cada conjunto no nível 6: a lista "Trocas escolhidas" traz as duas, e as duas são linhas de "O que muda até aqui" e do Resumo',
  { tag: ['@MR-040', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 800 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForLevelUp(m, p, `Duas trocas ${Date.now()}`, { build: doran, prepare: (id) => postureSheetRPC(m, id, 4, [5, 6]) });
      campaignId = table.campaignId;
      const sheet = `/campaigns/${campaignId}/characters/${table.characterId}`;
      await p.goto(`${sheet}/level-up`);
      await passClassStep(p);
      await p.getByRole('button', { name: 'Próximo' }).click();
      await p.getByRole('button', { name: 'Próximo' }).click();
      await p.getByRole('button', { name: 'Confirmar o nível 5' }).click();
      await expect(p.getByText('Doran subiu para o nível 5.').first()).toBeVisible();

      await allowRulePickSwapRPC(m, campaignId);
      await markMilestoneRPC(m, campaignId, 'Chegar ao Vale Seco 2', [table.characterId]);
      await p.goto(`${sheet}/level-up`);
      await passClassStep(p);
      await expect(p.getByText('Passo 2 de 4 · Habilidades')).toBeVisible();
      await p.locator('label.row').filter({ hasText: 'Força' }).click();
      // The preview's answer fills "O que muda com Força" above the panel and pushes it down: wait for it, so no click
      // reaches the panel while it moves.
      await expect(p.getByText('Escolha uma habilidade para ver o que muda.')).toBeHidden();
      const panel = p.getByRole('region', { name: 'Trocar escolhas' });
      // The panel comes after "O que muda com o aumento".
      const order = await p.locator('h2').allTextContents();
      expect(order.indexOf('Trocar escolhas')).toBeGreaterThan(order.findIndex((t) => t.startsWith('O que muda com')));
      await panel.locator('label.opt').filter({ hasText: 'Posturas' }).click();
      await expect(panel.getByRole('radio', { name: /Posturas/ })).toBeChecked();
      await panel.getByRole('radiogroup', { name: 'Abrir mão de' }).getByText('Postura do Touro').click();
      await panel.getByRole('radiogroup', { name: 'Pegar no lugar' }).getByText('Postura do Urso').click();
      await panel.getByText('Estilo de luta', { exact: true }).click();
      await panel.getByRole('radiogroup', { name: 'Abrir mão de' }).getByText('Defesa').click();
      await panel.getByRole('radiogroup', { name: 'Pegar no lugar' }).getByText('Duelismo').click();
      const list = panel.getByRole('list', { name: 'Trocas escolhidas' });
      await expect(list.getByRole('listitem')).toHaveText(['Estilo de luta: Defesa → Duelismo', 'Posturas: Postura do Touro → Postura do Urso']);
      const side = p.getByRole('region', { name: 'O que muda até aqui' });
      await expect(side.getByRole('listitem').filter({ hasText: 'Posturas' })).toContainText('Postura do Touro');
      await expect(side.getByRole('listitem').filter({ hasText: 'Estilo de luta' })).toContainText('Duelismo');
      await p.getByRole('button', { name: 'Próximo' }).click();
      await p.getByRole('button', { name: 'Próximo' }).click();
      await expect(p.getByText('Passo 4 de 4 · Resumo')).toBeVisible();
      await expect(p.getByRole('listitem').filter({ hasText: 'Posturas' }).filter({ hasText: 'Postura do Urso' })).toBeVisible();
      await expect(p.getByRole('listitem').filter({ hasText: 'Estilo de luta' }).filter({ hasText: 'Duelismo' })).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);
