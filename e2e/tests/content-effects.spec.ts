import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { createEntryRPC, entryRoute } from './content-support';
import { tableForMaps } from './maps-support';
import { newSignedInContext } from './support';

test.describe.configure({ timeout: 150_000 });

// MR-025: the editor fields of "Dano extra" (dice, type, weapon, frequency) and of "Opção de magia" (the spells it fits and its
// change). The feats are invented and come through the API, so the test proves the screen reads them back.

test(
  'o editor mostra o dano extra com dados e a opção de magia com os campos da sua mudança @MR-025',
  { tag: ['@MR-025'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const { campaignId } = await tableForMaps(m, p, `Efeitos de dano ${Date.now()}`);

      const sharpshot = await createEntryRPC(m, campaignId, 'tableFeat', {
        namePt: 'Tiro certeiro do vale',
        descPt: ['Uma vez por turno, o primeiro acerto à distância queima um pouco.'],
        effects: [
          {
            type: 'extra_damage',
            applies: 'any_target',
            dice: '1',
            diceSides: 6,
            damageType: 'damage-type:fire',
            weapon: 'ranged',
            frequency: 'once_per_turn',
          },
        ],
      });
      await m.goto(entryRoute(campaignId, sharpshot));
      await expect(m.getByLabel('Lados do dado')).toHaveValue('6');
      await expect(m.getByLabel('Dados (quantos)')).toHaveValue('1');
      // Native selects: the text of a select is every option's, so the chosen one is read by its value and its checked option.
      await expect(m.getByLabel('Tipo de dano')).toHaveValue('damage-type:fire');
      await expect(m.getByLabel('Tipo de dano').locator('option:checked')).toHaveText('fogo');
      await expect(m.getByLabel('Arma')).toHaveValue('ranged');
      await expect(m.getByLabel('Arma').locator('option:checked')).toHaveText('Ataque com arma à distância');
      await expect(m.getByLabel('Quantas vezes')).toHaveValue('once_per_turn');
      await expect(m.getByLabel('Quantas vezes').locator('option:checked')).toHaveText('Uma vez por turno');

      const tinted = await createEntryRPC(m, campaignId, 'tableFeat', {
        namePt: 'Chama tingida',
        descPt: ['O fogo das suas magias pode virar frio.'],
        effects: [
          { type: 'spell_modifier', appliesTo: 'damage_spell', spellChange: 'damage_type_swap', from: ['damage-type:fire', 'damage-type:cold'] },
        ],
      });
      await m.goto(entryRoute(campaignId, tinted));
      await expect(m.getByLabel('O que muda na magia')).toHaveValue('damage_type_swap');
      await expect(m.getByLabel('O que muda na magia').locator('option:checked')).toHaveText('Trocar o tipo de dano');
      await expect(m.getByText('Tipos de dano que troca')).toBeVisible();
      // The fields of the other changes are not asked.
      await expect(m.getByLabel('Dano fixo somado')).toHaveCount(0);
      await expect(m.getByLabel('Dados a mais por nível acima')).toHaveCount(0);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);
