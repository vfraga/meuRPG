import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { cardNumbers, diceTable, freeRollsRPC, openDice, rollUntilRefused, stepModifier } from './free-dice-support';
import { endOpenSessionRPC, openTableRolls, startSessionRPC } from './live-session-support';
import { callRPC, newSignedInContext } from './support';

// MR-052: the free dice roller. A member of the table opens "Dados" in the session's header, picks the dice and
// rolls; the server rolls them. A player's roll goes to the whole table, the master's "Só eu" to nobody else
// (RN-10). Assertions on what a player does NOT see wait for a table-wide roll to reach that same screen first.
// File name: dice.spec.ts already holds the RN-18 dice-mode spec.

const rollRPC = 'meurpg.play.v1.PlayService/RollFreeDice';

test(
  'um jogador rola 2d6 + 3: lê o resultado, e o mestre o vê no registro e na animação dos dados',
  { tag: ['@MR-052', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const { m, p, done } = await diceTable(browser, 'Dados livres', { master: 1280, player: 390 });
    try {
      // The suite runs with reduced motion, which keeps the dice still and the page usable (playwright.config.ts). This test
      // is about what the master sees on the rolling overlay, so his page asks for motion, and the overlay is closed again below.
      await m.emulateMedia({ reducedMotion: 'no-preference' });
      // The overlay is a deferred part of the shell: it must be mounted before the roll arrives, or the dice have nowhere to tumble.
      await expect(m.locator('app-roll-overlay')).toBeAttached({ timeout: 20_000 });
      const sheet = await openDice(p);
      await sheet.getByRole('button', { name: '2d6', exact: true }).click();
      await stepModifier(sheet, 0, 3);
      await sheet.getByRole('button', { name: 'Rolar 2d6 + 3', exact: true }).click();

      // The result card: the total in numbers and in words, and the dice behind it.
      const card = sheet.locator('app-free-roll-card');
      await expect(card.locator('.die__box')).toHaveCount(2);
      const { faces, total } = await cardNumbers(card);
      expect(faces.every((f) => f >= 1 && f <= 6)).toBe(true);
      expect(total).toBe(faces[0] + faces[1] + 3);
      await expect(card.locator('.card__line')).toHaveText(`2d6 + 3 = ${total}`);
      await expect(card.getByRole('status')).toHaveText(`Resultado ${total}. 2d6 + 3 = ${total}. Rolagem livre, para a mesa.`);
      await expect(card).toContainText('Rolagem livre · para a mesa');
      await expect(sheet.getByRole('button', { name: 'Mudar dados' })).toBeVisible();
      await expect(sheet.getByRole('button', { name: 'Rolar 2d6 + 3 de novo' })).toBeVisible();

      // The master sees the dice on the overlay first: it covers the page, and a tap anywhere closes it, so it is read and
      // closed before the master opens anything.
      const overlay = m.locator('app-roll-overlay .result').filter({ hasText: 'Rolagem livre · 2d6 + 3' });
      await expect(overlay).toBeVisible({ timeout: 15_000 });
      await expect(overlay).toContainText(/rolou/);
      await m.locator('app-roll-overlay .scrim').click(); // "Toque para fechar"
      await expect(overlay).toBeHidden();
      // Then the master reads the same roll in "Rolagens da mesa".
      const row = (await openTableRolls(m))
        .getByRole('listitem')
        .filter({ hasText: `2d6 + 3 = ${total} (${faces[0]}, ${faces[1]})` });
      await expect(row).toHaveCount(1);
      await expect(row).toContainText('Rolagem livre ·');
    } finally {
      await done();
    }
  },
);

test(
  'a rolagem "Só eu" do mestre é só dele: o jogador não a recebe, e só se confere isso depois de ver uma rolagem da mesa',
  { tag: ['@MR-052', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(120_000);
    const { m, p, campaignId, done } = await diceTable(browser, 'Dados secretos', { master: 1280, player: 1280 });
    try {
      const sheet = await openDice(m);
      await sheet.getByRole('radiogroup', { name: 'Quem vê' }).locator('label', { hasText: 'Só eu' }).click();
      await expect(sheet.getByRole('radiogroup', { name: 'Quem vê' }).getByRole('radio', { name: 'Só eu' })).toBeChecked();
      await stepModifier(sheet, 0, 7);
      await sheet.getByRole('button', { name: 'Rolar 1d20 + 7, só eu', exact: true }).click();
      await expect(sheet.getByText('Só você viu esta rolagem.')).toBeVisible();
      await expect(sheet.locator('app-free-roll-card')).toContainText('Só você');

      // Back to the table: another roll, this one for everyone, so the player's screen has certainly heard the stream.
      await sheet.getByRole('button', { name: 'Mudar dados' }).click();
      await sheet.getByRole('radiogroup', { name: 'Quem vê' }).locator('label', { hasText: 'Para a mesa' }).click();
      await stepModifier(sheet, 7, 0);
      await sheet.getByRole('button', { name: '1d6', exact: true }).click();
      await sheet.getByRole('button', { name: 'Rolar 1d6 para a mesa', exact: true }).click();
      await expect(sheet.locator('app-free-roll-card')).toContainText('Rolagem livre · para a mesa');
      await m.keyboard.press('Escape');
      await expect(sheet).toBeHidden();

      // The master reads both in the log, the secret one with its badge.
      const masterLog = await openTableRolls(m);
      const secretRow = masterLog.getByRole('listitem').filter({ hasText: '1d20 + 7' });
      await expect(secretRow).toHaveCount(1);
      await expect(secretRow).toContainText('Só você');

      // The player reads the table's roll; only now does the absence of the other one mean anything.
      // The player's log is a fold at the end of the scene (C6-08), closed until opened.
      const playerLog = await openTableRolls(p);
      await expect(playerLog.getByRole('listitem').filter({ hasText: /: 1d6 = \d+ \(\d\)/ })).toHaveCount(1);
      await expect(playerLog.getByText('1d20 + 7')).toHaveCount(0);
      await expect(playerLog.getByText('Só você')).toHaveCount(0);
      await expect(p.getByRole('alert').filter({ hasText: /rolagem|dado/i })).toHaveCount(0);

      // The reads agree with the screen: the secret roll is in no list of the player, the text of the answer included.
      const seen = await freeRollsRPC(p, campaignId);
      expect(seen.table.map((r) => r.formula)).toEqual(['1d6']);
      expect(seen.mine).toEqual([]);
      expect(seen.text).not.toContain('1d20 + 7');
      const masters = await freeRollsRPC(m, campaignId);
      expect(masters.table.map((r) => r.formula)).toEqual(['1d6', '1d20 + 7']);
      expect(masters.mine.filter((r) => r.secret)).toHaveLength(1);

      // A player cannot ask for "Só eu" either: refused, and nothing was rolled.
      const asked = await callRPC(p, rollRPC, { campaignId, sides: 20, count: 1, secret: true, idempotencyKey: `secret-${Date.now()}` });
      expect(asked.status()).toBe(403);
      expect((await freeRollsRPC(p, campaignId)).mine).toEqual([]);
    } finally {
      await done();
    }
  },
);

test('um toque duplo em "Rolar" rola uma vez só', { tag: ['@MR-052'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const { m, p, campaignId, done } = await diceTable(browser, 'Dados em dobro', { master: 1280, player: 1280 });
  try {
    const sheet = await openDice(p);
    await sheet.getByRole('button', { name: 'Rolar 1d20', exact: true }).dblclick();
    await expect(sheet.locator('app-free-roll-card')).toBeVisible();
    await expect((await openTableRolls(m)).getByRole('listitem')).toHaveCount(1);
    const own = await freeRollsRPC(p, campaignId);
    expect(own.mine).toHaveLength(1);
    expect(own.table).toHaveLength(1);
  } finally {
    await done();
  }
});

test('"Repetir" rola de novo os mesmos dados de uma das últimas rolagens', { tag: ['@MR-052'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  const { p, campaignId, done } = await diceTable(browser, 'Dados repetidos', { master: 1280, player: 390 });
  try {
    const sheet = await openDice(p);
    await expect(sheet.getByText('Você ainda não rolou nesta sessão.')).toBeVisible();
    await sheet.getByRole('button', { name: '2d6', exact: true }).click();
    await stepModifier(sheet, 0, 3);
    await sheet.getByRole('button', { name: 'Rolar 2d6 + 3', exact: true }).click();
    await expect(sheet.locator('app-free-roll-card .card__line')).toContainText('2d6 + 3 = ');

    // Other dice in between, so "Repetir" has something to bring back.
    await sheet.getByRole('button', { name: 'Mudar dados' }).click();
    await sheet.getByRole('button', { name: '1d100', exact: true }).click();
    await sheet.getByRole('button', { name: /^Rolar 1d100/ }).click();
    const last = sheet.getByRole('region', { name: 'Suas últimas rolagens nesta sessão' });
    await expect(last.getByRole('listitem')).toHaveCount(2);

    await last.getByRole('button', { name: 'Repetir 2d6 + 3' }).click();
    await expect(sheet.locator('app-free-roll-card .card__line')).toContainText('2d6 + 3 = ');
    await expect(last.getByRole('listitem')).toHaveCount(3);
    const own = await freeRollsRPC(p, campaignId);
    expect(own.mine.map((r) => r.formula)).toEqual(['2d6 + 3', '1d100 + 3', '2d6 + 3']);
  } finally {
    await done();
  }
});

test('rolar além do limite é recusado com resource_exhausted e um tempo de espera', { tag: ['@MR-052'] }, async ({ browser }) => {
  test.setTimeout(120_000);
  // Not Jogador Teste: draining a person's allowance would make the other specs' rolls wait. E-mail Não Verificado
  // rolls here and nowhere else.
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const toren = await newSignedInContext(browser, 'E-mail Não Verificado');
  const m = await master.newPage();
  const t = await toren.newPage();
  await Promise.all([m.goto('/'), t.goto('/')]);
  const created = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/CreateCampaign', { name: `Dados limite ${Date.now()}`, xpMode: 'XP_MODE_ENEMIES' });
  expect(created.ok()).toBeTruthy();
  const campaignId = (await created.json()).campaign.id as string;
  const invite = await callRPC(m, 'meurpg.campaigns.v1.CampaignService/CreateInvite', { campaignId, maxUses: 1, expiresIn: '3600s' });
  expect(invite.ok()).toBeTruthy();
  const accepted = await callRPC(t, 'meurpg.campaigns.v1.CampaignService/AcceptInvite', { token: (await invite.json()).token });
  expect(accepted.ok()).toBeTruthy();
  try {
    await startSessionRPC(m, campaignId);

    // The burst is 20 rolls times the stack's rate multiplier: the helper sends parallel batches until one is refused.
    const { granted, refused } = await rollUntilRefused(t, campaignId);
    expect(granted).toBeGreaterThanOrEqual(20);
    expect((await refused.json()).code).toBe('resource_exhausted');
    expect(Number(refused.headers()['retry-after'])).toBeGreaterThanOrEqual(1);
  } finally {
    await endOpenSessionRPC(m, campaignId);
    await master.close();
    await toren.close();
  }
});
