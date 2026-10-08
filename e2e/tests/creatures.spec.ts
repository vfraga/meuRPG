import { expect, test } from '@playwright/test';

import { endOpenSessionRPC } from './live-session-support';
import { giveCreatureRPC, listCreaturesRPC, tableForCreatures } from './creatures-support';
import { afterRender, callRPC, idpOrigin, newSignedInContext } from './support';

// MR-037 (the character's creatures on the sheet), RN-20 (a creature's hit points go only to its
// owner's player and the master) and RN-18 (nothing is rolled outside a combat). Setup (campaign,
// Pensantus, the session) goes through the API; every test makes its own campaign.

/** How many slots of any circle Pensantus has used, as the session reads them. */
async function slotsUsed(page: import('@playwright/test').Page, campaignId: string): Promise<number> {
  const res = await callRPC(page, 'meurpg.play.v1.PlayService/GetLiveSession', { campaignId });
  expect(res.ok()).toBeTruthy();
  const vitals = ((await res.json()).vitals ?? []) as { spellSlots?: { used?: number }[] }[];
  return vitals.flatMap((v) => v.spellSlots ?? []).reduce((sum, s) => sum + (s.used ?? 0), 0);
}

test(
  'Pensantus conjura Convocar Familiar como ritual sem gastar espaço, dá o nome Nanquim, abre a ficha, renomeia e dispensa; depois conjura de novo',
  { tag: ['@MR-037'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCreatures(master, player, `Familiar ${Date.now()}`);
      campaignId = table.campaignId;
      const usedBefore = await slotsUsed(master, campaignId);

      await player.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      const panel = player.locator('app-creatures-panel');
      await expect(panel.getByRole('heading', { name: 'Criaturas', level: 2 })).toBeVisible();
      await expect(panel.getByText('Nenhuma criatura ainda. Use Convocar Familiar ou peça ao mestre para dar uma.')).toBeVisible();
      await expect(panel.getByText('Ritual de 1 hora: não gasta espaço de magia. Só durante uma sessão, fora de combate.')).toBeVisible();

      // The sheet: the name, then the forms; the filled button names what happens and stays off until it can.
      await panel.getByRole('button', { name: 'Convocar Familiar' }).click();
      const sheet = player.getByRole('dialog', { name: 'Convocar Familiar' });
      await expect(sheet.getByText('Magia de 1º nível · ritual · 1 hora')).toBeVisible();
      await expect(sheet.getByText('Escolha a forma e dê um nome ao familiar.')).toBeVisible();
      await sheet.getByLabel('Nome do familiar').fill('Nanquim');
      await expect(sheet.locator('.line')).toHaveText(/Escolha a forma\.$/);
      await expect(sheet.getByRole('radio')).toHaveCount(15);
      await sheet.locator('label', { hasText: /Corvo/ }).click();
      await expect(sheet.getByText('Conjurar como ritual · 1 hora · sem gastar espaço')).toBeVisible();
      await sheet.getByRole('button', { name: 'Convocar o familiar' }).click();
      await expect(sheet).toBeHidden();

      // The live region confirms and the card enters; no slot was spent.
      await expect(panel.getByRole('status').getByText('Nanquim chegou. Convocar Familiar, ritual de 1 hora. Nenhum espaço de magia foi gasto.')).toBeVisible();
      await expect(panel.getByText('1 criatura', { exact: true })).toBeVisible();
      const card = panel.locator('app-creature-card');
      await expect(card.getByRole('heading', { name: 'Nanquim' })).toBeVisible();
      await expect(card.getByText('Corvo · Miúdo · Familiar de Pensantus')).toBeVisible();
      await expect(card.locator('.tile', { hasText: 'PV' })).toContainText('1 de 1');
      await expect(card.locator('.tile', { hasText: 'Deslocamento' })).toContainText('3 m');
      expect(await slotsUsed(master, campaignId)).toBe(usedBefore);

      // The stat block: a page of its own, the book's text in English.
      await card.getByRole('link', { name: 'Ver a ficha de Nanquim' }).click();
      await expect(player.getByRole('heading', { name: 'Nanquim', level: 1 })).toBeVisible();
      await expect(player.getByText('Os textos abaixo são do livro de regras (SRD 5.1), em inglês.')).toBeVisible();
      await expect(player.locator('.entry[lang=en]').first()).toBeVisible();
      await expect(player.getByText('Como familiar, Nanquim não ataca.')).toBeVisible();
      await expect(player.getByRole('button', { name: 'Atacar' })).toHaveCount(0);

      // Rename in place.
      await player.getByRole('button', { name: 'Renomear' }).click();
      await expect(player.getByLabel('Nome da criatura')).toBeFocused();
      await player.getByLabel('Nome da criatura').fill('Tinta');
      await player.getByRole('button', { name: 'Salvar o nome' }).click();
      await expect(player.getByRole('heading', { name: 'Tinta', level: 1 })).toBeVisible();
      expect((await listCreaturesRPC(player, campaignId, table.characterId)).map((c) => c.name)).toEqual(['Tinta']);

      // Dismiss asks in place first; "Voltar" has the focus and changes nothing.
      await player.getByRole('button', { name: 'Dispensar' }).click();
      const ask = player.getByRole('alertdialog', { name: 'Dispensar Tinta?' });
      await expect(ask).toBeVisible();
      await expect(ask.getByRole('button', { name: 'Voltar' })).toBeFocused();
      await ask.getByRole('button', { name: 'Voltar' }).click();
      await expect(ask).toBeHidden();
      await expect(player.getByRole('button', { name: 'Dispensar' })).toBeFocused();
      await player.getByRole('button', { name: 'Dispensar' }).click();
      await player.getByRole('button', { name: 'Dispensar Tinta' }).click();
      await expect(player).toHaveURL(`/campaigns/${campaignId}/characters/${table.characterId}`);
      await expect(player.locator('app-creatures-panel').getByText('Nenhuma criatura ainda.')).toBeVisible();
      expect(await listCreaturesRPC(player, campaignId, table.characterId)).toEqual([]);

      // A dismissed familiar can be summoned again.
      await player.locator('app-creatures-panel').getByRole('button', { name: 'Convocar Familiar' }).click();
      const again = player.getByRole('dialog', { name: 'Convocar Familiar' });
      await again.getByLabel('Nome do familiar').fill('Pena');
      await again.locator('label', { hasText: /Coruja/ }).click();
      await again.getByRole('button', { name: 'Convocar o familiar' }).click();
      await expect(player.locator('app-creatures-panel').getByRole('heading', { name: 'Pena' })).toBeVisible();
      expect(await slotsUsed(master, campaignId)).toBe(usedBefore);
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
  'fora de uma sessão o botão da magia fica tracejado e diz o motivo, e o servidor recusa o ritual',
  { tag: ['@MR-037'] },
  async ({ browser }) => {
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    try {
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCreatures(master, player, `Sem sessão ${Date.now()}`, false);
      await player.goto(`/campaigns/${table.campaignId}/characters/${table.characterId}`);
      const button = player.locator('app-creatures-panel').getByRole('button', { name: 'Convocar Familiar' });
      await expect(button).toHaveAttribute('aria-disabled', 'true');
      await expect(player.getByText('Agora não há sessão aberta.')).toBeVisible();
      await button.click({ force: true });
      await afterRender(player);
      await expect(player.getByRole('dialog')).toHaveCount(0);
      const refused = await callRPC(player, 'meurpg.play.v1.PlayService/CastSummon', {
        campaignId: table.campaignId,
        characterId: table.characterId,
        spellKey: 'spell:find-familiar',
        ritual: true,
        summon: { option: 0, creatureKeys: ['monster:raven'], names: ['Nanquim'] },
        idempotencyKey: crypto.randomUUID(),
      });
      // The refusal is typed: failed_precondition with GameSessionBlocked and NO_OPEN_SESSION, not any 400.
      expect(refused.status()).toBe(400);
      const body = (await refused.json()) as { code: string; details?: { type: string; debug?: { reason?: string } }[] };
      expect(body.code).toBe('failed_precondition');
      expect(body.details?.[0]?.type).toBe('meurpg.play.v1.GameSessionBlocked');
      expect(body.details?.[0]?.debug?.reason).toBe('GAME_SESSION_BLOCKED_REASON_NO_OPEN_SESSION');
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'o mestre dá um Mastim, corrige os PV dele fora do combate e o jogador é avisado; o jogador também pode dispensar o que o mestre deu',
  { tag: ['@MR-037', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCreatures(master, player, `Mastim ${Date.now()}`);
      campaignId = table.campaignId;
      // The player is looking at the sheet while the master gives the creature.
      await player.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      await expect(player.getByRole('heading', { name: 'Pensantus', level: 1 })).toBeVisible();
      // The session is known to the page (the cast button is on) before the master acts.
      await expect(player.locator('app-creatures-panel .cast__btn')).not.toHaveAttribute('aria-disabled', 'true');

      await master.goto(`/campaigns/${campaignId}`);
      const row = master.locator('app-character-creatures');
      await expect(row.getByText('Nenhuma criatura')).toBeVisible();
      await row.getByRole('button', { name: 'Dar uma criatura a Pensantus' }).click();
      const dialog = master.getByRole('dialog', { name: 'Dar uma criatura a Pensantus' });
      await expect(dialog.getByLabel('Nome, em português ou inglês')).toBeFocused();
      await expect(dialog.getByText(/de 334 · em ordem de nome/)).toBeVisible();
      await dialog.getByLabel('Nome, em português ou inglês').fill('ma');
      await dialog.getByLabel('Tipo').selectOption('beast');
      await dialog.getByLabel('Nível de desafio').selectOption('1/8');
      await expect(dialog.getByText('2 de 334 · em ordem de nome')).toBeVisible();
      await dialog.locator('label', { hasText: /Mastim/ }).click();
      await expect(dialog.getByLabel('Nome da criatura')).toHaveValue('Mastim');
      await dialog.getByRole('button', { name: 'Dar Mastim a Pensantus' }).click();
      await expect(dialog).toBeHidden();
      await expect(master.getByText('Mastim dado a Pensantus.')).toBeVisible();
      await expect(row.getByText('Mastim · dado pelo mestre')).toBeVisible();
      await expect(row.getByRole('button', { name: 'Dar uma criatura a Pensantus' })).toBeFocused();

      // The player is told and the panel appears with the card: CA, PV and the speed.
      const panel = player.locator('app-creatures-panel');
      await expect(panel.getByRole('status').getByText('O mestre deu uma criatura a você: Mastim.')).toBeVisible();
      const card = panel.locator('app-creature-card');
      await expect(card.getByText('Mastim · Médio · Dado pelo mestre')).toBeVisible();
      await expect(card.locator('.tile', { hasText: 'PV' })).toContainText('5 de 5');
      // The character's player may dismiss any of their creatures, a gift included (the server decides).
      await expect(card.getByRole('button', { name: 'Dispensar' })).toHaveCount(1);

      // The master corrects the hit points outside a combat (RN-02); the player sees the new number.
      await master.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      const masterCard = master.locator('app-creatures-panel app-creature-card');
      await masterCard.getByRole('button', { name: 'Corrigir PV' }).click();
      await masterCard.getByLabel('PV de Mastim').fill('3');
      await masterCard.getByRole('button', { name: 'Corrigir os PV' }).click();
      await expect(masterCard.locator('.tile', { hasText: 'PV' })).toContainText('3 de 5');
      await expect(card.locator('.tile', { hasText: 'PV' })).toContainText('3 de 5');

      // The master dismisses what he gave, in the list, asking in place first.
      await master.goto(`/campaigns/${campaignId}`);
      await row.getByRole('button', { name: 'Dispensar Mastim' }).click();
      await expect(row.getByRole('alertdialog', { name: 'Dispensar Mastim?' })).toBeVisible();
      await row.getByRole('button', { name: 'Dispensar Mastim' }).click();
      await expect(row.getByText('Nenhuma criatura')).toBeVisible();
      expect(await listCreaturesRPC(master, campaignId, table.characterId)).toEqual([]);
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
  'uma terceira pessoa da campanha, que não é a dona, não vê o painel "Criaturas" nem a lista de Pensantus',
  { tag: ['@MR-037', '@RN-20'] },
  async ({ browser }) => {
    test.setTimeout(240_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    // A real sign-in as the third test user of devidp: the one login of this spec, in a context of its own.
    const thirdContext = await browser.newContext();
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    const third = await thirdContext.newPage();
    let campaignId = '';
    try {
      await master.goto('/');
      await player.goto('/');
      const table = await tableForCreatures(master, player, `Terceiro ${Date.now()}`);
      campaignId = table.campaignId;
      await giveCreatureRPC(master, campaignId, table.characterId, 'monster:mastiff', 'Mastim');

      await third.goto('/auth/login?return_to=/');
      await expect(third).toHaveURL((url) => url.origin === idpOrigin && url.pathname === '/authorize');
      await third.getByRole('button', { name: 'E-mail Não Verificado', exact: true }).click();
      await expect(third).toHaveURL('/');
      const invite = await callRPC(master, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId, maxUses: 1, expiresIn: '3600s' });
      expect(invite.ok()).toBeTruthy();
      const joined = await callRPC(third, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
      expect(joined.ok()).toBeTruthy();

      // The list, with its hit points, and what the sheet can summon are the owner's and the master's alone.
      for (const method of ['ListCharacterCreatures', 'GetSummonOptions']) {
        const res = await callRPC(third, `meurpg.characters.v1.CharacterService/${method}`, { campaignId, characterId: table.characterId });
        expect(res.status(), method).toBe(404);
        expect(await res.text(), method).not.toContain('hitPoints');
      }
      await third.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      await expect(third.getByText('Esse personagem não existe, ou você não pode vê-lo.')).toBeVisible();
      await expect(third.locator('app-creatures-panel')).toHaveCount(0);
    } finally {
      if (campaignId) {
        await endOpenSessionRPC(master, campaignId);
      }
      await masterContext.close();
      await playerContext.close();
      await thirdContext.close();
    }
  },
);
