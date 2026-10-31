import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { callRPC, newSignedInContext } from './support';
import { tableForXp } from './xp-support';

// MR-016 (planned milestones), RN-09 (milestones campaigns), RN-12 (who can
// level up), RN-20 (a player never sees what is only planned). The data comes
// through the API; the screens are what is under test. The e2e table has one
// player with one living character, so "Dar a mais alguém" (it needs someone
// who was left out) is proved by the server's and the Vitest tests, which have
// three characters. Every test makes its own campaign.

const panel = (page: Page) => page.getByRole('region', { name: 'Experiência', exact: true });
const progression = 'meurpg.progression.v1.ProgressionService';

async function plan(page: Page, campaignId: string, text: string): Promise<string> {
  const res = await callRPC(page, `${progression}/AddMilestone`, { campaignId, text });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).milestone.id as string;
}

async function reach(page: Page, campaignId: string, milestoneId: string, characterId: string): Promise<void> {
  const res = await callRPC(page, `${progression}/MarkMilestoneReached`, {
    campaignId,
    milestoneId,
    characterIds: [characterId],
    idempotencyKey: crypto.randomUUID(),
  });
  expect(res.ok(), await res.text()).toBeTruthy();
}

test(
  'o mestre planeja os marcos, marca um como alcançado e o personagem escolhido pode subir de nível',
  { tag: ['@MR-016', '@RN-09', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForXp(m, p, `Marcos planejados ${Date.now()}`, 'XP_MODE_MILESTONES');
      await m.goto(`/campaigns/${table.campaignId}`);

      // Nothing planned: the panel says what to write.
      await expect(panel(m)).toContainText('Nenhum marco planejado');
      await expect(panel(m)).toContainText('Só você vê esta lista.');

      // Add three, in place: the field takes the focus, Enter adds, an empty name says so under the field.
      for (const text of ['Salvar o mercador', 'Chegar ao Vale Seco', 'Derrotar o Barão Ivo']) {
        await panel(m).getByRole('button', { name: 'Adicionar marco', exact: true }).click();
        const field = panel(m).getByLabel('Nome do marco');
        await expect(field).toBeFocused();
        if (text === 'Salvar o mercador') {
          await field.press('Enter');
          await expect(panel(m).getByRole('alert')).toContainText('Escreva o nome do marco');
          await expect(field).toBeFocused();
        }
        await field.fill(text);
        await field.press('Enter');
        await expect(panel(m).getByLabel('Nome do marco')).toHaveCount(0);
        await expect(panel(m).getByRole('button', { name: `Marcar como alcançado: “${text}”` })).toBeVisible();
      }
      await expect(panel(m)).toContainText('3 marcos · só você vê');

      // Order: the last goes up, and the focus stays on the button that was pressed.
      await panel(m).getByRole('button', { name: 'Subir Derrotar o Barão Ivo' }).click();
      await expect(panel(m).getByRole('button', { name: 'Subir Derrotar o Barão Ivo' })).toBeFocused();
      const names = panel(m).locator('app-planned-milestones .row__name');
      await expect(names).toHaveText(['Salvar o mercador', 'Derrotar o Barão Ivo', 'Chegar ao Vale Seco']);

      // Remove asks in place, with "Voltar" in focus.
      await panel(m).getByRole('button', { name: 'Remover Salvar o mercador' }).click();
      await expect(m.getByRole('alertdialog')).toContainText('Remover o marco Salvar o mercador?');
      await expect(m.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await m.getByRole('button', { name: 'Voltar' }).click();
      await expect(panel(m).getByRole('button', { name: 'Remover Salvar o mercador' })).toBeFocused();

      // Mark "Chegar ao Vale Seco" reached, out of order.
      await panel(m).getByRole('button', { name: 'Marcar como alcançado: “Chegar ao Vale Seco”' }).click();
      const dialog = m.getByRole('dialog', { name: 'Marcar “Chegar ao Vale Seco” como alcançado' });
      await expect(dialog).toContainText('1 personagem pode subir de nível');
      await dialog.getByRole('button', { name: 'Marcar como alcançado' }).click();
      await expect(panel(m).getByRole('status').filter({ hasText: 'Marco alcançado' })).toContainText('Pensantus pode subir de nível');
      await expect(panel(m).locator('app-reached-milestones')).toContainText('Chegar ao Vale Seco');
      await expect(panel(m).locator('app-reached-milestones')).toContainText('marcou Pensantus');
      await expect(panel(m).locator('app-level-up-tag')).toBeVisible();
      await expect(panel(m)).toContainText('2 marcos · só você vê');
      await expect(panel(m)).not.toContainText(/\d\s*XP/);

      // The player's sheet carries the tag.
      await p.goto(`/campaigns/${table.campaignId}/characters/${table.characterId}`);
      await expect(p.locator('app-sheet-header').getByText('Pode subir de nível')).toBeVisible();

      // Undo: the milestone is planned again.
      await m.reload();
      await panel(m).getByRole('button', { name: 'Desfazer Chegar ao Vale Seco' }).click();
      await expect(m.getByRole('alertdialog')).toContainText('volta para “Marcos planejados”');
      await m.getByRole('alertdialog').getByRole('button', { name: 'Desfazer marco' }).click();
      await expect(panel(m).getByRole('button', { name: 'Marcar como alcançado: “Chegar ao Vale Seco”' })).toBeVisible();
      await expect(panel(m)).toContainText('3 marcos · só você vê');
      await expect(panel(m)).toContainText('Nenhum marco alcançado ainda');
    } finally {
      await master.close();
      await player.close();
    }
  },
);

test(
  'o jogador vê só os marcos alcançados, nunca os planejados',
  { tag: ['@MR-016', '@RN-20', '@RN-12'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    try {
      await m.goto('/');
      await p.goto('/');
      const table = await tableForXp(m, p, `Marcos do jogador ${Date.now()}`, 'XP_MODE_MILESTONES');
      const vale = await plan(m, table.campaignId, 'Chegar ao Vale Seco');
      await plan(m, table.campaignId, 'Revelar o traidor da guilda');

      // Before the first milestone: the empty state, no character row, no hint of the planned ones.
      await p.goto(`/campaigns/${table.campaignId}`);
      await expect(panel(p)).toContainText('Nenhum marco alcançado ainda. Quando o grupo cumprir um, ele aparece aqui.');
      await expect(panel(p)).not.toContainText(/planejad/i);
      await expect(p.locator('body')).not.toContainText('Revelar o traidor da guilda');
      await expect(panel(p).locator('app-milestone-characters')).toHaveCount(0);

      // After: only the reached one, with who levelled, the character and "Abrir a ficha".
      await reach(m, table.campaignId, vale, table.characterId);
      await p.reload();
      await expect(panel(p)).toContainText('Chegar ao Vale Seco');
      await expect(panel(p)).toContainText('Subiu de nível: Pensantus');
      await expect(panel(p)).not.toContainText(/planejad/i);
      await expect(p.locator('body')).not.toContainText('Revelar o traidor da guilda');
      await expect(panel(p).locator('app-level-up-tag')).toBeVisible();
      await panel(p).getByRole('link', { name: 'Abrir a ficha' }).click();
      await expect(p).toHaveURL(`/campaigns/${table.campaignId}/characters/${table.characterId}`);

      // The server agrees: a player's list has the reached milestone only.
      const res = await callRPC(p, `${progression}/ListMilestones`, { campaignId: table.campaignId });
      const body = await res.text();
      expect(body).toContain('Chegar ao Vale Seco');
      expect(body).not.toContain('Revelar o traidor da guilda');
    } finally {
      await master.close();
      await player.close();
    }
  },
);
