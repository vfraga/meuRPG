import { expect, type Locator, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { catalogJSON, catalogText, optionSwitchesJSON, raceOptions, setSwitchesRPC } from './content-options-support';
import { createEntryRPC, entryRoute, spellBody } from './content-support';
import { startSessionRPC, endOpenSessionRPC } from './live-session-support';
import { tableForMaps } from './maps-support';
import { listSpellsRPC } from './spells-support';
import { newSignedInContext } from './support';

test.describe.configure({ timeout: 150_000 });

// MR-025 and MR-045 (RN-23, RN-10): "Opções para os jogadores", the master's switches, on screen. Every test makes its own
// campaign through the API (a player with Pensantus, a gnome Mago of the Escola de Evocação).

/** Clicks a switch and waits for the call to answer: "Tudo salvo" shows at rest too, so the answer is what proves the save. */
async function turned(page: Page, sw: Locator): Promise<void> {
  const answered = page.waitForResponse((r) => r.url().includes('SetOptionSwitches'));
  await sw.click();
  await answered;
  await expect(page.getByText('Tudo salvo')).toBeVisible();
}

const optionsRoute = (campaignId: string, kind: string) => `/campaigns/${campaignId}/content/options?kind=${kind}`;

test(
  'o mestre desliga o Tiefling: a lista de raças do jogador não o tem, na tela e no JSON, e a do mestre ainda o tem, sinalizado @MR-025 @RN-23',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const { campaignId } = await tableForMaps(m, p, `Opções Tiefling ${Date.now()}`);

      // Before: the player's own list has the Tiefling.
      await p.goto(`/campaigns/${campaignId}/characters/new`);
      expect(await raceOptions(p)).toContain('Tiefling');

      await m.goto(optionsRoute(campaignId, 'races'));
      await expect(m.getByRole('heading', { level: 1, name: 'Opções para os jogadores' })).toBeVisible();
      const counter = m.locator('.counter strong');
      await expect(counter).toHaveText(/Raças: 9 de 9 ligadas/);
      const tiefling = m.getByRole('switch', { name: 'Tiefling' });
      await expect(tiefling).toHaveAttribute('aria-checked', 'true');
      await turned(m, tiefling);
      await expect(counter).toHaveText(/Raças: 8 de 9 ligadas/);
      await expect(tiefling).toHaveAttribute('aria-checked', 'false');
      await expect(m.locator('.orow', { hasText: 'Tiefling' })).toContainText('Desligada');
      // Saved at once: a reload shows the same.
      await m.reload();
      await expect(m.getByRole('switch', { name: 'Tiefling' })).toHaveAttribute('aria-checked', 'false');

      // The master's own reads still have it, flagged.
      const row = (await optionSwitchesJSON(m, campaignId)).find((o) => o.key === 'race:tiefling')!;
      expect(row).toMatchObject({ off: true, hidden: true });
      expect((await catalogJSON(m, campaignId)).races!.map((r) => r.key)).toContain('race:tiefling');

      // The player's: not in the JSON, not on the screen.
      // Nowhere in what the player receives: not as a key, not as a name.
      expect((await catalogText(p, campaignId)).toLowerCase()).not.toContain('tiefling');
      await p.goto(`/campaigns/${campaignId}/characters/new`);
      const names = await raceOptions(p);
      expect(names).not.toContain('Tiefling');
      expect(names).toContain('Gnomo');
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'uma classe desligada esconde as subclasses dela do jogador, e o mestre vê o motivo @MR-025 @RN-23',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const { campaignId } = await tableForMaps(m, p, `Opções classe ${Date.now()}`);
      const before = await optionSwitchesJSON(m, campaignId);
      const children = before.filter((o) => o.parentKey === 'class:cleric');
      expect(children.length).toBeGreaterThan(0);
      expect((await catalogJSON(p, campaignId)).subclasses!.some((s) => s.classKey === 'class:cleric')).toBe(true);

      await m.goto(optionsRoute(campaignId, 'classes'));
      await turned(m, m.getByRole('switch', { name: 'Clérigo' }));

      // The player's catalog has neither the class nor its subclasses.
      const catalog = await catalogJSON(p, campaignId);
      expect(catalog.classes!.map((c) => c.key)).not.toContain('class:cleric');
      expect(catalog.subclasses!.filter((s) => s.classKey === 'class:cleric')).toEqual([]);

      // The master's list keeps each subclass's own switch on, and says why it is hidden.
      const after = (await optionSwitchesJSON(m, campaignId)).filter((o) => o.parentKey === 'class:cleric');
      expect(after.every((o) => !o.off && o.hidden)).toBe(true);
      await m.getByRole('link', { name: /^Subclasses/ }).click();
      const first = m.locator('.orow', { hasText: 'Subclasse de Clérigo' }).first();
      await expect(first).toContainText('Some para os jogadores: a classe Clérigo está desligada.');
      await expect(first.getByRole('switch')).toHaveAttribute('aria-checked', 'true');

      // Turning the class back on gives the subclasses back as they were.
      await m.getByRole('link', { name: /^Classes/ }).click();
      await turned(m, m.getByRole('switch', { name: 'Clérigo' }));
      expect((await catalogJSON(p, campaignId)).subclasses!.some((s) => s.classKey === 'class:cleric')).toBe(true);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'uma ficha que usa uma opção desligada continua funcionando, e a tela diz quantas fichas usam @MR-025 @RN-23',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const { campaignId, characterId } = await tableForMaps(m, p, `Opções em uso ${Date.now()}`);

      await m.goto(optionsRoute(campaignId, 'races'));
      const gnome = m.locator('.orow').filter({ has: m.getByRole('switch', { name: 'Gnomo', exact: true }) });
      await expect(gnome).toContainText('1 ficha usa');
      await turned(m, m.getByRole('switch', { name: 'Gnomo', exact: true }));
      await expect(gnome).toContainText('A ficha que a usa continua funcionando.');
      await expect(m.getByText('Gnomo: desligada para os jogadores. 1 ficha usa e continua funcionando.')).toBeAttached();

      // The player's sheet still opens with the race it has.
      await p.goto(`/campaigns/${campaignId}/characters/${characterId}`);
      await expect(p.getByRole('heading', { level: 1, name: 'Pensantus' })).toBeVisible();
      await expect(p.getByText('Gnomo').first()).toBeVisible();
      // An unrelated edit of that sheet saves: the switch never refuses what is not a new choice of the option.
      await p.goto(`/campaigns/${campaignId}/characters/${characterId}/edit`);
      await p.getByRole('tab', { name: /Equipamento/ }).click();
      await p.getByLabel('Itens de equipamento', { exact: true }).fill('Grimório\nAdaga');
      await p.getByRole('button', { name: 'Salvar ficha' }).click();
      await expect(p).toHaveURL(new RegExp(`/characters/${characterId}$`));
      await expect(p.getByRole('heading', { level: 1, name: 'Pensantus' })).toBeVisible();
      await expect(p.getByText('Grimório', { exact: true })).toBeVisible();
      // But a new choice of it is not offered.
      await p.goto(`/campaigns/${campaignId}/characters/new`);
      expect(await raceOptions(p)).not.toContain('Gnomo');
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'com o editor de personagem aberto, o mestre desliga uma raça e a tela do jogador se atualiza sem recarregar @MR-025 @RN-23 @RN-10',
  { tag: ['@MR-025', '@RN-23', '@RN-10'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      ({ campaignId } = await tableForMaps(m, p, `Opções ao vivo ${Date.now()}`));
      // The hint travels on the session's stream: the session is open before the player's page is.
      await startSessionRPC(m, campaignId);

      await p.goto(`/campaigns/${campaignId}/characters/new`);
      await p.getByLabel('Nome do personagem', { exact: true }).fill('Ícaro');
      expect(await raceOptions(p)).toContain('Meio-orc');

      await setSwitchesRPC(m, campaignId, [{ key: 'race:half-orc', off: true }]);
      // No reload: the list is read again, and what was typed is still there.
      await expect(p.getByText('O mestre mudou as opções da mesa.')).toBeVisible({ timeout: 15_000 });
      expect(await raceOptions(p)).not.toContain('Meio-orc');
      await expect(p.getByLabel('Nome do personagem', { exact: true })).toHaveValue('Ícaro');

      // The master's own options page, open in another tab, follows too.
      await m.goto(optionsRoute(campaignId, 'races'));
      await expect(m.getByRole('switch', { name: 'Meio-orc' })).toHaveAttribute('aria-checked', 'false');
      await setSwitchesRPC(m, campaignId, [{ key: 'race:half-orc', off: false }]);
      await expect(m.getByRole('switch', { name: 'Meio-orc' })).toHaveAttribute('aria-checked', 'true', { timeout: 15_000 });
    } finally {
      if (campaignId) {
        const m = await master.newPage();
        await endOpenSessionRPC(m, campaignId);
      }
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'a página aberta antes da sessão também segue a dica: ela conecta em até 30 s e lê na primeira conexão @MR-025 @RN-23 @RN-10',
  { tag: ['@MR-025', '@RN-23', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(180_000);
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    let campaignId = '';
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      ({ campaignId } = await tableForMaps(m, p, `Opções antes da sessão ${Date.now()}`));
      // The player's page is up first: no session, so no stream.
      await p.goto(`/campaigns/${campaignId}/characters/new`);
      expect(await raceOptions(p)).toContain('Draconato');
      // The master opens the session and switches a race off before the page's stream has connected.
      await startSessionRPC(m, campaignId);
      await setSwitchesRPC(m, campaignId, [{ key: 'race:dragonborn', off: true }]);
      // Within the 30 s of the poll of open sessions plus the connection, the first `ready` reads the lists.
      await expect(p.getByText('O mestre mudou as opções da mesa.')).toBeVisible({ timeout: 75_000 });
      expect(await raceOptions(p)).not.toContain('Draconato');
    } finally {
      if (campaignId) {
        const m = await master.newPage();
        await endOpenSessionRPC(m, campaignId);
      }
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'a chave "Disponível para os jogadores" do editor liga e desliga uma magia da mesa, e a página "Magias" do jogador a segue @MR-025 @MR-045 @RN-23',
  { tag: ['@MR-025', '@MR-045', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const { campaignId } = await tableForMaps(m, p, `Opções magia ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));

      await m.goto(entryRoute(campaignId, key));
      const sw = m.getByRole('switch', { name: 'Disponível para os jogadores' });
      await expect(sw).toHaveAttribute('aria-checked', 'true');
      expect((await listSpellsRPC(p, campaignId)).spells.map((s) => s.key)).toContain(key);
      await sw.click();
      await expect(sw).toHaveAttribute('aria-checked', 'false');
      await expect(m.getByText('Desligada para os jogadores').first()).toBeVisible();
      expect((await optionSwitchesJSON(m, campaignId)).find((o) => o.key === key)).toMatchObject({ off: true });
      // The master's list still has the spell; the player's has not.
      expect((await listSpellsRPC(m, campaignId)).spells.map((s) => s.key)).toContain(key);
      expect((await listSpellsRPC(p, campaignId)).spells.map((s) => s.key)).not.toContain(key);
      await p.goto(`/campaigns/${campaignId}/spells?q=nanquim`);
      await expect(p.getByRole('heading', { level: 1, name: 'Magias' })).toBeVisible();
      await expect(p.getByText('Nenhuma magia com “nanquim”.')).toBeVisible();
      // And on again: it is back.
      await sw.click();
      await expect(sw).toHaveAttribute('aria-checked', 'true');
      await p.reload();
      await expect(p.getByRole('button', { name: /Lâmina de Nanquim/ })).toBeVisible();
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);
