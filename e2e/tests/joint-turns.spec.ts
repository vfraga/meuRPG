import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { endPartRPC, beginJointCombat, jointTable } from './joint-turn-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { newSignedInContext } from './support';

// The joint turn on screen (MR-013, RN-19, RN-20): combatants adjacent in the
// order with the same initiative total take one turn together. Pensantus (the
// player's) and Brisa (an NPC ally) tie; the goblins tie too, as a group of NPCs
// alone. The totals are typed so they tie on every run.

test(
  'Pensantus e Brisa, com a mesma iniciativa, dividem um turno; cada um encerra a sua parte e o turno passa depois da segunda',
  { tag: ['@MR-013', '@RN-19'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const joint = await jointTable(m, p, `Turno conjunto ${Date.now()}`);
      campaignId = joint.table.campaignId;
      await beginJointCombat(m, joint);

      // The master: one card for the group, a block per member, no "end the group's turn".
      await m.goto(`/campaigns/${campaignId}/session`);
      await expect(m.getByRole('heading', { name: /^Turno conjunto: / })).toBeVisible();
      const card = m.getByRole('region', { name: /^Turno conjunto: / });
      await expect(card.getByText('Falta')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Próximo turno' })).toHaveAttribute('aria-disabled', 'true');
      await expect(m.getByRole('group', { name: /^Turno conjunto: .*iniciativa \d+$/ })).toHaveCount(2);

      // The player: "Sua vez", the pill, the other member's state, and the footer.
      await openSessionPage(p, campaignId);
      await expect(p.getByRole('heading', { name: 'Sua vez, Pensantus' })).toBeVisible();
      await expect(p.getByText('Turno conjunto com Brisa')).toBeVisible();
      await expect(p.getByText('O turno passa quando você e a Brisa encerrarem.').locator('visible=true')).toHaveCount(1);

      // "Encerrar a minha parte" asks first, with "Voltar" first.
      await p.getByRole('button', { name: 'Encerrar a minha parte' }).click();
      const question = p.getByRole('alertdialog', { name: 'Encerrar a sua parte?' });
      await expect(question).toBeVisible();
      await expect(question.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await question.getByRole('button', { name: 'Voltar' }).click();
      await expect(p.getByRole('alertdialog')).toHaveCount(0);
      await p.getByRole('button', { name: 'Encerrar a minha parte' }).click();
      await p.getByRole('alertdialog').getByRole('button', { name: 'Encerrar a minha parte' }).click();
      await expect(p.getByRole('heading', { name: 'Você encerrou a sua parte' })).toBeVisible();
      await expect(p.getByText('Falta a Brisa.')).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Neste turno conjunto' })).toBeVisible();

      // The master ends Brisa's part: the turn passes to the goblins, and the player is told.
      await m.getByRole('button', { name: 'Encerrar a parte da Brisa' }).click();
      await expect(m.getByRole('heading', { name: /^Turno conjunto: Goblin \d e Goblin \d$/ })).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Vez dos Goblins' })).toBeVisible();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId).catch(() => undefined);
      }
      await master.close();
      await player.close();
    }
  },
);

test(
  'um grupo só de NPCs com a mesma iniciativa é do mestre: o jogador não vê caixa nem total',
  { tag: ['@MR-013', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 900 } });
    const m = await master.newPage();
    const p = await player.newPage();
    let campaignId = '';
    try {
      await m.goto('/');
      await p.goto('/');
      const joint = await jointTable(m, p, `Grupo de NPCs ${Date.now()}`);
      campaignId = joint.table.campaignId;
      await beginJointCombat(m, joint);
      await openSessionPage(p, campaignId);
      // The player's order has Pensantus and Brisa in a box; the goblins are plain rows.
      await expect(p.getByRole('group', { name: /^Turno conjunto: / })).toHaveCount(1);

      await endPartRPC(m, campaignId, 'Pensantus');
      await endPartRPC(m, campaignId, 'Brisa');
      await expect(p.getByRole('heading', { name: 'Vez dos Goblins' })).toBeVisible();
      // Never a box, a total or one goblin picked.
      await expect(p.getByRole('group', { name: /^Turno conjunto: Goblin/ })).toHaveCount(0);
      await expect(p.getByText('iniciativa 5')).toHaveCount(0);
      await expect(p.getByRole('heading', { name: /Vez do Goblin/ })).toHaveCount(0);
      // The master has the box.
      await m.goto(`/campaigns/${campaignId}/session`);
      await expect(m.getByRole('group', { name: /^Turno conjunto: Goblin \d e Goblin \d, iniciativa 5$/ })).toBeVisible();
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(m, campaignId).catch(() => undefined);
      }
      await master.close();
      await player.close();
    }
  },
);
