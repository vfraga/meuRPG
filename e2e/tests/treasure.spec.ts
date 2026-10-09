import { expect, test, type Page } from '@playwright/test';

import { createDungeonRPC, roomsRPC } from './dungeon-support';
import { tableForGold } from './gold-support';
import { canvasPng, createMapRPC, revealMapRPC, tableForMaps, uploadImageRPC } from './maps-support';
import { callRPC, newSignedInContext } from './support';
import { generateTreasureRPC, gridRPC, mapPointsRPC, markFoundRPC, treasureRoute } from './treasure-support';
import { getExperienceRPC } from './xp-support';

// MR-044 (the treasure generator on screen, slice 10.17c), MR-041 (the gold of a treasure becomes XP in a gold campaign), RN-09
// and RN-10 (a hidden point is the master's). Setup goes through the API; every test makes its own campaign.

const xpOf = async (page: Page, campaignId: string, characterId: string) =>
  (await getExperienceRPC(page, campaignId)).characters.find((c) => c.characterId === characterId)?.experiencePoints ?? 0;

test(
  'o mestre gera um tesouro de covil do nível 4; a mesma semente dá o mesmo tesouro, e "Ver descrição" abre o item com o texto do SRD em inglês',
  { tag: ['@MR-044', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForMaps(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;

      // The way in: the campaign page's "Mapas" panel, for the master.
      await master.goto(`/campaigns/${campaignId}`);
      await master.getByRole('link', { name: 'Gerar tesouro' }).click();
      await expect(master.getByRole('heading', { name: 'Tesouro', level: 1 })).toBeVisible();
      await expect(master.getByText('Nada gerado ainda.')).toBeVisible();

      // The party is Pensantus (level 3): the level starts there and the master raises it to 4.
      await expect(master.getByTestId('party-help')).toContainText('O grupo está no nível 3.');
      await expect(master.locator('.step__value')).toHaveText('3');
      await master.getByRole('button', { name: 'Mais Nível do grupo' }).click();
      await expect(master.locator('.step__value')).toHaveText('4');

      await master.getByRole('button', { name: 'Gerar tesouro' }).click();
      await expect(master.getByRole('heading', { name: 'Tesouro de covil · nível 4' })).toBeVisible();
      const seed = (await master.getByTestId('treasure-seed').textContent())!.trim();
      expect(seed).toMatch(/^\d+$/);

      // The same mode, level and seed give the same treasure, twice (and the page drew what the server returned).
      const first = await generateTreasureRPC(master, campaignId, { mode: 'TREASURE_MODE_HOARD', partyLevel: 4, seed });
      const second = await generateTreasureRPC(master, campaignId, { mode: 'TREASURE_MODE_HOARD', partyLevel: 4, seed });
      expect(second).toEqual(first);
      expect(String(first.seed)).toBe(seed);
      const gold = first.goldPo as number;
      await expect(master.getByTestId('treasure-gold')).toHaveText(new RegExp(`^${gold.toLocaleString('pt-BR')}\\s+PO$`));
      await expect(master.locator('#tr-coins-h')).toContainText('Moedas');

      // "Gerar outro" draws another seed.
      await master.getByRole('button', { name: 'Gerar outro' }).click();
      await expect(master.getByTestId('treasure-seed')).not.toHaveText(seed);

      // A hoard of level 4 has magic items (the table always gives some): each has a description, with the SRD's text in English.
      // The same item can come twice in a hoard (two rows of "Poção de cura"), so everything is read inside the first row.
      const firstRow = master.locator('app-treasure-item-row').first();
      const name = (await firstRow.locator('.item__name').textContent())!.replace(/^\d+ × /, '').trim();
      const openFirst = firstRow.getByRole('button', { name: `Ver descrição: ${name}` });
      await openFirst.click();
      const dialog = master.getByRole('dialog', { name });
      await expect(dialog).toContainText('Valores do SRD 5.2.1 (regras de 2024)');
      await expect(dialog.getByRole('link', { name: 'Créditos' })).toBeVisible();
      await expect(dialog.getByText('Texto do SRD 5.1, em inglês')).toBeVisible();
      await expect(dialog.locator('.text__body[lang=en]')).toBeVisible();
      await expect(dialog.locator('button[data-initial-focus]')).toBeFocused();
      await master.keyboard.press('Escape');
      await expect(dialog).toHaveCount(0);
      await expect(openFirst).toBeFocused();

      // A player has no page: a notice, and the server answers `not_found`.
      await player.goto(treasureRoute(campaignId));
      await expect(player.getByText('Só o mestre gera o tesouro da campanha.')).toBeVisible();
      const refused = await callRPC(player, 'meurpg.maps.v1.TreasureService/GenerateTreasure', { campaignId, mode: 'TREASURE_MODE_HOARD', partyLevel: 4 });
      expect(refused.status()).toBe(404);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  '"Pôr no mapa" numa sala da masmorra faz um ponto de tesouro escondido: o mestre o vê, o JSON do jogador não',
  { tag: ['@MR-044', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const table = await tableForMaps(master, player, `Mirathel ${Date.now()}`);
      const campaignId = table.campaignId;
      const mapId = await createDungeonRPC(master, campaignId, 'Masmorra de Mirathel', { seed: '11', options: { width: 31, height: 21, roomSideMin: 3, roomSideMax: 9 } });
      await revealMapRPC(master, campaignId, mapId);

      await master.goto(treasureRoute(campaignId));
      await master.getByRole('button', { name: 'Gerar tesouro' }).click();
      await expect(master.getByTestId('treasure-seed')).toBeVisible();
      const gold = Number((await master.getByTestId('treasure-gold').textContent())!.replace(/[^\d]/g, ''));
      await master.getByRole('button', { name: 'Pôr no mapa' }).click();

      const dialog = master.getByRole('dialog', { name: 'Pôr no mapa' });
      await expect(dialog.locator('select[data-field=map]')).toHaveValue(mapId);
      await expect(dialog.getByText(/Onde\s+Na Sala \d+/)).toBeVisible();
      // The rooms are radios beside the map: Sala 2, then one step east with the arrow key.
      await dialog.locator('.room', { hasText: 'Sala 2' }).click();
      await expect(dialog.getByText(/Onde\s+Na Sala 2/)).toBeVisible();
      await dialog.getByRole('group', { name: 'Quadrado do tesouro no mapa' }).focus();
      await master.keyboard.press('ArrowRight');
      await dialog.getByLabel('Nome do ponto (opcional)').fill('Cofre da cripta');
      await dialog.getByRole('button', { name: 'Pôr no mapa' }).click();
      await expect(dialog).toHaveCount(0);

      const done = master.getByTestId('treasure-placed');
      await expect(done).toContainText('Tesouro posto na Sala 2');
      await expect(done).toContainText('escondido: só você vê');
      // The treasure is on the map: "Abrir o mapa" is the main button and there is no "Pôr no mapa" left for it.
      await expect(master.getByRole('link', { name: 'Abrir o mapa' })).toHaveAttribute('href', `/campaigns/${campaignId}/maps/${mapId}`);
      await expect(master.getByRole('button', { name: 'Pôr no mapa' })).toHaveCount(0);
      await expect(master.getByRole('button', { name: 'Gerar outro' })).toBeVisible();

      // The master reads the point: a hidden TREASURE worth the gold only, never the items, on the square east of Sala 2's middle.
      const mine = await mapPointsRPC(master, campaignId, mapId);
      const point = mine.points.find((p) => p.name === 'Cofre da cripta')!;
      expect(point).toBeTruthy();
      expect(point.kind).toBe('MAP_POINT_KIND_TREASURE');
      expect(point.revealed ?? false).toBe(false);
      expect(Number(point.treasureValuePo)).toBe(gold);
      const room = ((await roomsRPC(master, campaignId, mapId)).rooms as { id: number; centerCol?: number; centerRow?: number }[]).find((r) => r.id === 2)!;
      const columns = mine.map.gridColumns as number;
      const rows = mine.map.gridRows as number;
      expect(Math.abs((point.xBp as number) - (((room.centerCol ?? 0) + 1.5) / columns) * 10000)).toBeLessThanOrEqual(2);
      expect(Math.abs((point.yBp as number) - (((room.centerRow ?? 0) + 0.5) / rows) * 10000)).toBeLessThanOrEqual(2);

      // The player's JSON never has the point, its name, its gold or what is inside.
      const theirs = await mapPointsRPC(player, campaignId, mapId);
      expect(theirs.points.find((p) => p.name === 'Cofre da cripta')).toBeUndefined();
      expect(theirs.text).not.toContain('Cofre da cripta');
      expect(theirs.text).not.toContain(point.id);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test(
  'numa campanha por ouro, o tesouro posto no mapa, achado e levado de volta à cidade dá XP só pelo ouro, e a linha da página diz o que acontece com ele',
  { tag: ['@MR-044', '@MR-041', '@RN-09'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const masterContext = await newSignedInContext(browser, 'Mestre Teste');
    const playerContext = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const master = await masterContext.newPage();
      const player = await playerContext.newPage();
      await master.goto('/');
      await player.goto('/');
      const name = `Estrada de Ouro ${Date.now()}`;
      const table = await tableForGold(master, player, name);
      await gridRPC(master, table.campaignId, table.mapId, 20);
      const before = await xpOf(master, table.campaignId, table.characterId);

      await master.goto(treasureRoute(table.campaignId));
      await master.getByRole('button', { name: 'Gerar tesouro' }).click();
      await expect(master.getByTestId('treasure-gold-line')).toHaveText(`${name} dá XP por ouro: o grupo converte isto em XP em “Voltar à cidade”.`);
      const gold = Number((await master.getByTestId('treasure-gold').textContent())!.replace(/[^\d]/g, ''));
      // A hoard's items are shown apart and are not part of the gold.
      await master.getByRole('button', { name: 'Pôr no mapa' }).click();
      const dialog = master.getByRole('dialog', { name: 'Pôr no mapa' });
      await expect(dialog.getByText('converte isto em XP em “Voltar à cidade”')).toBeVisible();
      await dialog.getByRole('button', { name: 'Pôr no mapa' }).click();
      await expect(dialog).toHaveCount(0);
      await expect(master.getByTestId('treasure-placed-line')).toContainText(`${name} dá XP por ouro: o grupo converte ${gold.toLocaleString('pt-BR')} PO em XP em “Voltar à cidade”.`);
      await expect(master.getByRole('button', { name: 'Pôr no mapa' })).toHaveCount(0);

      const placed = (await mapPointsRPC(master, table.campaignId, table.mapId)).points.find((p) => p.kind === 'MAP_POINT_KIND_TREASURE')!;
      expect(Number(placed.treasureValuePo)).toBe(gold);
      await markFoundRPC(master, table.campaignId, table.mapId, placed.id, [table.characterId]);

      await master.goto(`/campaigns/${table.campaignId}`);
      const panel = master.getByRole('region', { name: 'Experiência', exact: true });
      await panel.getByRole('button', { name: 'Voltar à cidade', exact: true }).click();
      const town = master.getByRole('dialog', { name: 'Voltar à cidade' });
      await town.getByRole('button', { name: new RegExp(`Dar ${gold.toLocaleString('pt-BR')} XP para cada`) }).click();
      await expect(town).toHaveCount(0);
      await expect(panel.getByRole('status')).toContainText(`Pensantus recebeu ${gold.toLocaleString('pt-BR')} XP. O tesouro foi convertido.`);

      // The XP counts the gold only: 1 XP per PO, never the value of the items.
      const after = await xpOf(master, table.campaignId, table.characterId);
      expect(after - before).toBe(gold);
    } finally {
      await masterContext.close();
      await playerContext.close();
    }
  },
);

test('um mapa sem grade diz "Escolha um mapa com grade" e o botão fica quieto', { tag: ['@MR-044'] }, async ({ browser }) => {
  test.setTimeout(180_000);
  const masterContext = await newSignedInContext(browser, 'Mestre Teste');
  const playerContext = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const master = await masterContext.newPage();
    const player = await playerContext.newPage();
    await master.goto('/');
    await player.goto('/');
    const table = await tableForMaps(master, player, `Mirathel ${Date.now()}`);
    const png = await canvasPng(master, 640, 400, 'Sem grade');
    const imageId = await uploadImageRPC(master, table.campaignId, 'Sem grade', png);
    await createMapRPC(master, table.campaignId, 'Mapa sem grade', imageId);

    await master.goto(treasureRoute(table.campaignId));
    await master.getByRole('button', { name: 'Gerar tesouro' }).click();
    await master.getByRole('button', { name: 'Pôr no mapa' }).click();
    const dialog = master.getByRole('dialog', { name: 'Pôr no mapa' });
    await expect(dialog.getByTestId('no-grid')).toContainText('Escolha um mapa com grade');
    await expect(dialog.getByRole('button', { name: 'Pôr no mapa' })).toHaveAttribute('aria-disabled', 'true');
  } finally {
    await masterContext.close();
    await playerContext.close();
  }
});
