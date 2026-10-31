import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { openSessionPage } from './live-session-support';
import { enterZoneRPC, firstFootprint, zoneCard, zoneLayer, zoneOf, zoneTable, zonesPanel } from './zones-support';

// The areas that spells keep on the map (RN-37), on the two screens: the master's map and "Zonas ativas" panel, the player's
// (name, area and caster only, a hidden caster's zone not there at all: RN-10), the move picker, the question that ends a
// zone, and the saving throw the zone asks of a player's character. The table, the casts and the character's step into a
// zone come through the API; what is under test is what each screen reads.

const title = (page: Parameters<typeof zonesPanel>[0]) => zonesPanel(page).getByRole('heading', { level: 2 });

test(
  'o mestre vê as zonas no mapa e na lista com o teste, o dano e o tempo; o jogador lê só o nome, a área e quem conjurou',
  { tag: ['@RN-37', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await zoneTable(browser, 'Zonas de magia');
    try {
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);

      // The master: three zones, on the map ("Teia · Pensantus") and in the list with the numbers.
      await expect(title(t.m)).toHaveText('Zonas ativas (3)');
      for (const label of ['Raio Lunar · Druida', 'Raio Lunar · Sombra', 'Teia · Pensantus']) {
        await expect(zoneLayer(t.m)).toContainText(label);
      }
      const beam = zoneCard(t.m, 'Raio Lunar · Druida');
      await expect(beam).toContainText('Cilindro de 1,5 m de raio');
      await expect(beam).toContainText(/Constituição CD\s\d+/);
      await expect(beam).toContainText(/2d10 .*metade se passar/);
      await expect(beam).toContainText(/restam \d+ min/);
      await expect(beam).toContainText('Concentração de Druida');
      const web = zoneCard(t.m, 'Teia · Pensantus');
      await expect(web).toContainText('Cubo de 6 m');
      await expect(web).toContainText(/CD\s\d+/);
      await expect(web).toContainText('contido');
      // The hidden caster's zone is the master's alone, and the card says so.
      await expect(zoneCard(t.m, 'Raio Lunar · Sombra')).toContainText('Só você vê');

      // The player: Teia (his own, with its numbers) has come, so the list already read everything cast before it.
      await expect(title(t.p)).toHaveText('Zonas ativas (2)');
      await expect(zoneLayer(t.p)).toContainText('Teia · Pensantus');
      await expect(zoneCard(t.p, 'Teia · Pensantus')).toContainText(/CD\s\d+/);
      // Another caster's zone: the name, the area and the caster, with no save, damage, time or buttons (`details_hidden`).
      const bare = zoneCard(t.p, 'Raio Lunar · Druida');
      await expect(bare).toContainText('Cilindro de 1,5 m de raio');
      await expect(bare).not.toContainText(/Constituição|\bCD\b|2d10|radiante|restam|Concentração|Aqui/);
      await expect(bare.getByRole('button')).toHaveCount(0);
      await expect(zoneCard(t.p, 'Teia · Pensantus').getByRole('button')).toHaveCount(0);
      await expect(zoneLayer(t.p)).toContainText('Raio Lunar · Druida');
      // The hidden caster's zone is absent from the list and from the map, and nothing names it.
      await expect(zonesPanel(t.p)).not.toContainText('Sombra');
      await expect(zoneLayer(t.p)).not.toContainText('Sombra');
      await expect(zonesPanel(t.p)).not.toContainText('Só você vê');
    } finally {
      await t.done();
    }
  },
);

test(
  'o mestre move a zona que anda por ação e encerra outra depois de uma pergunta no lugar; as duas telas acompanham',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await zoneTable(browser, 'Mover e encerrar zona');
    try {
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);
      await expect(title(t.m)).toHaveText('Zonas ativas (3)');
      await expect(title(t.p)).toHaveText('Zonas ativas (2)');

      // Raio Lunar moves by an action ("Mover"); Teia stays where it was put.
      const beam = zoneCard(t.m, 'Raio Lunar · Druida');
      await expect(zoneCard(t.m, 'Teia · Pensantus').getByRole('button', { name: 'Mover', exact: true })).toHaveCount(0);
      const masterBefore = await firstFootprint(t.m).getAttribute('d');
      const playerBefore = await firstFootprint(t.p).getAttribute('d');
      await beam.getByRole('button', { name: 'Mover', exact: true }).click();

      // The move sheet: the map picks a square (the arrows start from the zone's own point), "Mover a zona" waits for one.
      const sheet = t.m.getByRole('dialog', { name: 'Mover o Raio Lunar' });
      await expect(sheet).toBeVisible();
      const move = sheet.getByRole('button', { name: 'Mover a zona' });
      await expect(move).toHaveAttribute('aria-disabled', 'true');
      const map = sheet.getByRole('group', { name: /Mapa de batalha/ });
      await map.focus();
      await map.press('ArrowRight');
      await map.press('ArrowRight');
      await expect(sheet.locator('.summary strong')).toBeVisible();
      await expect(move).not.toHaveAttribute('aria-disabled', 'true');
      await move.click();
      await expect(sheet).toHaveCount(0);

      // The footprint changed on the master's map and on the player's.
      await expect(firstFootprint(t.m)).not.toHaveAttribute('d', masterBefore ?? '');
      await expect(firstFootprint(t.p)).not.toHaveAttribute('d', playerBefore ?? '');
      await expect(title(t.m)).toHaveText('Zonas ativas (3)');

      // Teia ends after a question in place: "Não" leaves it, "Sim, encerrar" takes it off both screens.
      const web = zoneCard(t.m, 'Teia · Pensantus');
      await web.getByRole('button', { name: 'Encerrar zona' }).click();
      const ask = web.getByRole('group', { name: 'Encerrar a zona?' });
      await expect(ask).toContainText('Teia · Pensantus some do mapa e da lista de todo mundo.');
      await ask.getByRole('button', { name: 'Não' }).click();
      await expect(ask).toHaveCount(0);
      await expect(title(t.m)).toHaveText('Zonas ativas (3)');
      await web.getByRole('button', { name: 'Encerrar zona' }).click();
      await ask.getByRole('button', { name: 'Sim, encerrar' }).click();
      await expect(title(t.m)).toHaveText('Zonas ativas (2)');
      await expect(zoneLayer(t.m)).not.toContainText('Teia · Pensantus');
      await expect(title(t.p)).toHaveText('Zonas ativas (1)');
      await expect(zoneLayer(t.p)).not.toContainText('Teia · Pensantus');
      await expect(zoneLayer(t.p)).toContainText('Raio Lunar · Druida');
    } finally {
      await t.done();
    }
  },
);

test(
  'o jogador que entra numa zona recebe o teste de resistência dela, com a habilidade e a metade do dano, sem a CD',
  { tag: ['@RN-37', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await zoneTable(browser, 'Teste de uma zona');
    try {
      await openSessionPage(t.p, t.campaignId);
      await expect(title(t.p)).toHaveText('Zonas ativas (2)');
      // Pensantus is put on the point of Druida's Raio Lunar: the zone catches him as one who came in.
      await enterZoneRPC(t, await zoneOf(t.m, t.campaignId, 'spell:moonbeam', 'Druida'));

      const sheet = t.p.getByRole('dialog', { name: 'Raio Lunar de Druida' });
      await expect(sheet).toBeVisible();
      await expect(sheet).toContainText('Magia que ficou no mapa');
      await expect(sheet).toContainText('Você entrou na zona.');
      await expect(sheet).toContainText('Teste de resistência de Constituição');
      await expect(sheet).toContainText(/Se falhar: sofre dano radiante\. Se passar: metade do dano\./);
      // The DC is the caster's and the master's: the target's player reads no number and no gap where it would be.
      await expect(sheet).not.toContainText(/\bCD\b/);
      await expect(sheet.getByRole('button', { name: 'Deixar o mestre rolar por mim' })).toBeVisible();
    } finally {
      await t.done();
    }
  },
);
