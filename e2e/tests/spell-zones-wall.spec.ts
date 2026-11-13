import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { passTurnsTo } from './combat-support';
import { openSessionPage } from './live-session-support';
import { pickRadio } from './move-support';
import {
  castZoneRPC,
  expectOnMap,
  expectWallCalculated,
  nextStep,
  openWallSheet,
  tapWallSquare,
  wallTable,
  WALL_SPELL,
  zoneAction,
  zoneBadges,
  zoneCard,
  zoneLayer,
  zonesPanel,
  zonesRPC,
} from './zones-support';

// Wall of Fire (RN-37, #640) on the two screens: the player lays it from the cast sheet's "Onde fica" step (a line, a ring, the
// side that burns), the zone is a letter in the list and a burning side on the map, a player reads of another caster's wall only
// where he sees now, and the master ends it from "Mais ações". The table comes through the API (a level 9 wizard who prepared
// the spell, and an NPC wizard); what is under test is the sheet and what each screen reads.

const title = (page: Parameters<typeof zonesPanel>[0]) => zonesPanel(page).getByRole('heading', { level: 2 });

test(
  'a muralha em linha: o começo, a direção e o comprimento, o lado que queima e "Próximo"; a zona aparece na lista com a letra e o lado que queima no mapa',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await wallTable(browser, 'Muralha em linha');
    try {
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);
      const { sheet, map } = await openWallSheet(t.p);

      // Step 1: "Onde fica". Nothing is placed, so nothing can go on, and the line is the shape.
      await expect(sheet.getByRole('radio', { name: 'Linha reta' })).toBeChecked();
      await expect(nextStep(sheet)).toHaveAttribute('aria-disabled', 'true');
      await expect(sheet.getByText('Toque no mapa para marcar onde a muralha fica.')).toBeVisible();

      // The start at (11, 3), the direction by the pad, the length by the stepper: 6 squares to the south.
      await tapWallSquare(map, 11, 3);
      await sheet.getByRole('button', { name: 'Para o sul' }).click();
      await expect(sheet.getByRole('group', { name: 'Direção da muralha: sul' }).getByRole('button', { name: 'Para o sul' })).toHaveAttribute('aria-pressed', 'true');
      for (let i = 0; i < 6; i++) {
        await sheet.getByRole('button', { name: 'Diminuir o comprimento em 1,5 m' }).click();
      }
      await expect(sheet.getByRole('group', { name: 'Comprimento' }).getByRole('status')).toHaveText('9 m');
      await expectWallCalculated(sheet);

      // The side is not preselected: the two sides are named by where they lie on the screen, and "Próximo" waits for a pick.
      await expect(sheet.getByRole('radio', { name: 'À direita' })).not.toBeChecked();
      await expect(sheet.getByRole('radio', { name: 'À esquerda' })).not.toBeChecked();
      await expect(sheet.getByText('Escolha o lado', { exact: true })).toBeVisible();
      await expect(nextStep(sheet)).toHaveAttribute('aria-disabled', 'true');
      await pickRadio(sheet, 'À direita');
      await expect(sheet.getByRole('radio', { name: 'À direita' })).toBeChecked();
      await expect(nextStep(sheet)).not.toHaveAttribute('aria-disabled', 'true');
      await nextStep(sheet).click();

      // Step 2: who is in the area (nobody), then the cast.
      await expect(sheet.getByRole('heading', { name: 'Quem está nela' })).toBeVisible();
      await expect(sheet.getByText('Ninguém que você vê está na área.')).toBeVisible();
      await sheet.getByRole('button', { name: 'Conjurar mesmo assim' }).click();
      const done = t.p.getByRole('dialog', { name: 'Muralha de Fogo conjurada' });
      await expect(done).toContainText('Você está concentrado em Muralha de Fogo.');
      await done.getByRole('button', { name: 'Voltar à sua vez' }).click();

      // The zone: a letter in the list for the two screens, and the wall with its burning side on the map.
      const label = 'Muralha de Fogo · Pensantus';
      for (const page of [t.p, t.m]) {
        await expect(title(page)).toHaveText('Zonas ativas (1)');
        await expect(zoneCard(page, label)).toBeVisible();
        await expectOnMap(page, label);
        await expect(zoneLayer(page).locator('.zl__burn')).toHaveCount(1);
      }
      const [zone] = await zonesRPC(t.m, t.campaignId);
      expect(zone.spellKey).toBe(WALL_SPELL);
      expect(zone.origin).toEqual({ col: 11, row: 3 });
      expect(zone.squares).toEqual([3, 4, 5, 6, 7, 8].map((row) => ({ col: 11, row })));
      expect(zone.wallSide).toBe('WALL_SIDE_LEFT');
      // To the south, the left side is the east: the burning squares lie on the columns east of the wall.
      expect(zone.burningSquares?.length).toBeGreaterThan(0);
      expect(zone.burningSquares?.every((s) => (s.col ?? 0) > 11)).toBe(true);
    } finally {
      await t.done();
    }
  },
);

test(
  'o anel: o centro, o diâmetro de 3 m ou 6 m e dentro ou fora; a zona é um anel na lista e no mapa',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await wallTable(browser, 'Muralha em anel');
    try {
      await openSessionPage(t.p, t.campaignId);
      await openSessionPage(t.m, t.campaignId);
      const { sheet, map } = await openWallSheet(t.p);

      await pickRadio(sheet, 'Anel');
      await expect(sheet.getByRole('radio', { name: 'Anel' })).toBeChecked();
      // A ring has no direction, a diameter of 3 m or 6 m and the sides "Dentro" and "Fora".
      await expect(sheet.getByRole('group', { name: /^Direção da muralha/ })).toHaveCount(0);
      await expect(sheet.getByRole('group', { name: 'Diâmetro' }).getByRole('status')).toHaveText('6 m');
      await expect(sheet.getByRole('radio', { name: 'Dentro' })).toBeVisible();
      await expect(sheet.getByRole('radio', { name: 'Fora' })).toBeVisible();
      await expect(nextStep(sheet)).toHaveAttribute('aria-disabled', 'true');

      await tapWallSquare(map, 11, 5);
      await expect(sheet.getByRole('application', { name: 'Mapa: escolha o centro da Muralha de Fogo' })).toBeVisible();
      await expectWallCalculated(sheet);
      // The ring's card, from the server's preview: the diameter, the squares and who it names (nobody here).
      await expect(sheet.getByText(/^Anel de\s6\sm\s·\s16\squadrados$/)).toBeVisible();
      await expect(sheet.getByText('Ninguém que você vê está na muralha agora.')).toBeVisible();
      await pickRadio(sheet, 'Dentro');
      await expect(sheet.getByRole('radio', { name: 'Dentro' })).toBeChecked();
      await expect(nextStep(sheet)).not.toHaveAttribute('aria-disabled', 'true');
      await nextStep(sheet).click();
      await expect(sheet.getByRole('heading', { name: 'Quem está nela' })).toBeVisible();
      await sheet.getByRole('button', { name: 'Conjurar mesmo assim' }).click();
      await t.p.getByRole('dialog', { name: 'Muralha de Fogo conjurada' }).getByRole('button', { name: 'Voltar à sua vez' }).click();

      const label = 'Muralha de Fogo · Pensantus';
      for (const page of [t.p, t.m]) {
        await expect(title(page)).toHaveText('Zonas ativas (1)');
        await expectOnMap(page, label);
        await expect(zoneLayer(page).locator('.zl__burn')).toHaveCount(1);
      }
      const [zone] = await zonesRPC(t.m, t.campaignId);
      expect(zone.origin).toEqual({ col: 11, row: 5 });
      expect(zone.wallSide).toBe('WALL_SIDE_INNER');
      // A ring of 6 m across is the squares 10 ft from its centre: 16 of them.
      expect(zone.squares).toHaveLength(16);
    } finally {
      await t.done();
    }
  },
);

test(
  '"Próximo" fica desligado até um lado ser escolhido, e virar a muralha tira a escolha de volta',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await wallTable(browser, 'Muralha sem lado');
    try {
      await openSessionPage(t.p, t.campaignId);
      const { sheet, map } = await openWallSheet(t.p);
      await tapWallSquare(map, 11, 3);
      await sheet.getByRole('button', { name: 'Para o sul' }).click();
      await expectWallCalculated(sheet);

      // Placed, with a way, and still waiting: the footer says what is missing and "Próximo" does not go on.
      await expect(sheet.getByText('Escolha o lado que queima.')).toBeVisible();
      await expect(nextStep(sheet)).toHaveAttribute('aria-disabled', 'true');
      await expect(sheet.getByText('Escolha o lado', { exact: true })).toBeVisible();

      await pickRadio(sheet, 'À direita');
      await expect(nextStep(sheet)).not.toHaveAttribute('aria-disabled', 'true');
      await expect(sheet.getByText('Escolha o lado que queima.')).toHaveCount(0);

      // Turning the wall swaps what each side means on the screen, so the pick is taken back.
      await sheet.getByRole('button', { name: 'Para o leste' }).click();
      await expectWallCalculated(sheet);
      await expect(sheet.getByRole('radio', { name: 'Acima' })).not.toBeChecked();
      await expect(sheet.getByRole('radio', { name: 'Abaixo' })).not.toBeChecked();
      await expect(nextStep(sheet)).toHaveAttribute('aria-disabled', 'true');
      await expect(sheet.getByText('Escolha o lado que queima.')).toBeVisible();
    } finally {
      await t.done();
    }
  },
);

test(
  'a muralha de outro conjurador: o mestre lê a muralha inteira e o lado que queima, o jogador só o que vê agora',
  { tag: ['@RN-37', '@RN-10'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await wallTable(browser, 'Muralha vista', { fog: true });
    try {
      await passTurnsTo(t.m, t.campaignId, 'Mago');
      // Mago's wall runs south from (11, 3) and burns to the east, behind the wall for Pensantus, who stands to its west.
      await castZoneRPC(t.m, t, t.id('Mago'), WALL_SPELL, { col: 11, row: 3 }, 4, { direction: { dx: 0, dy: 1 }, side: 'WALL_SIDE_LEFT', lengthFt: 30 });
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);

      const label = 'Muralha de Fogo · Mago';
      await expect(title(t.m)).toHaveText('Zonas ativas (1)');
      await expect(title(t.p)).toHaveText('Zonas ativas (1)');
      await expectOnMap(t.m, label);
      await expectOnMap(t.p, label);
      // Both maps draw the wall and a burning side: the master's is the whole one, the player's what he sees (below).
      await expect(zoneLayer(t.m).locator('.zl__burn')).toHaveCount(1);
      await expect(zoneLayer(t.p).locator('.zl__burn')).toHaveCount(1);

      const [whole] = await zonesRPC(t.m, t.campaignId);
      const [seen] = await zonesRPC(t.p, t.campaignId);
      const key = (sq: { col?: number; row?: number }) => `${sq.col},${sq.row}`;
      expect(whole.squares).toHaveLength(6);
      expect(whole.burningSquares).toHaveLength(20);
      // The face of the wall that is turned to Pensantus is his; the burning squares beside the wall (rows 3 to 8, east of it) are
      // behind it, so he reads only the ones past its ends, and never a square the master does not have.
      expect((seen.squares ?? []).map(key)).toEqual((whole.squares ?? []).map(key));
      const burning = new Set((whole.burningSquares ?? []).map(key));
      expect((seen.burningSquares ?? []).length).toBeGreaterThan(0);
      expect((seen.burningSquares ?? []).length).toBeLessThan(burning.size);
      expect((seen.burningSquares ?? []).every((sq) => burning.has(key(sq)))).toBe(true);
      expect((seen.burningSquares ?? []).filter((sq) => (sq.row ?? 0) >= 3 && (sq.row ?? 0) <= 8)).toEqual([]);
    } finally {
      await t.done();
    }
  },
);

test(
  'o mestre encerra a muralha por "Mais ações", depois de uma pergunta no lugar; as duas telas acompanham',
  { tag: ['@RN-37'] },
  async ({ browser }) => {
    test.setTimeout(300_000);
    const t = await wallTable(browser, 'Encerrar muralha');
    try {
      await passTurnsTo(t.m, t.campaignId, 'Mago');
      await castZoneRPC(t.m, t, t.id('Mago'), WALL_SPELL, { col: 11, row: 3 }, 4, { direction: { dx: 0, dy: 1 }, side: 'WALL_SIDE_LEFT', lengthFt: 30 });
      await openSessionPage(t.m, t.campaignId);
      await openSessionPage(t.p, t.campaignId);
      const label = 'Muralha de Fogo · Mago';
      await expect(title(t.m)).toHaveText('Zonas ativas (1)');
      await expect(title(t.p)).toHaveText('Zonas ativas (1)');

      // A wall is not moved: its menu has only "Encerrar zona", and a player has no menu at all.
      await expect(zoneCard(t.p, label).getByRole('button')).toHaveCount(0);
      await zoneAction(t.m, label, 'Encerrar zona');
      const ask = zoneCard(t.m, label).getByRole('group', { name: 'Encerrar a zona?' });
      await expect(ask).toContainText(`${label} some do mapa e da lista de todo mundo.`);
      await ask.getByRole('button', { name: 'Sim, encerrar' }).click();

      await expect(zonesPanel(t.m)).toHaveCount(0);
      await expect(zonesPanel(t.p)).toHaveCount(0);
      await expect(zoneBadges(t.m)).toHaveCount(0);
      await expect(zoneBadges(t.p)).toHaveCount(0);
      expect(await zonesRPC(t.m, t.campaignId)).toHaveLength(0);
    } finally {
      await t.done();
    }
  },
);
