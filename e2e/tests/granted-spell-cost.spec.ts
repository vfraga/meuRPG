import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { beginAttackCombatRPC, pensantusCasting, tableForCombat } from './combat-support';
import { castSheet, openCastOf, tableForCasting } from './casting-support';
import { createEntryRPC, entryRoute } from './content-support';
import { endOpenSessionRPC, openSessionPage } from './live-session-support';
import { callRPC, newSignedInContext, openActionTab, viaTargetList } from './support';

test.describe.configure({ timeout: 180_000 });

// MR-025: a feat's granted spell that costs more than one use per cast and waives the material components. The feat is
// invented; the spell is the SRD's Bless (V, S, M). The editor reads the cost and the switch back; the cast sheet says
// what the cast costs and its "?" lists the components without the M.
test(
  'a magia concedida que gasta dois usos e dispensa o material: o editor a lê de volta e a folha de conjuração mostra o custo e os componentes sem o M @MR-025',
  { tag: ['@MR-025'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    let campaignId = '';
    const m = await master.newPage();
    try {
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const table = await tableForCombat(m, p, `Magia concedida ${Date.now()}`, true, true, { sheet: pensantusCasting });
      campaignId = table.campaignId;

      const feat = await createEntryRPC(m, campaignId, 'tableFeat', {
        namePt: 'Graça de teste',
        descPt: ['Um dom de teste.'],
        effects: [
          { type: 'resource', resource: 'usos_da_graca', max: '3', recharge: 'long_rest' },
          { type: 'granted_spell', spell: 'spell:bless', ability: 'cha', uses: 'usos_da_graca', usesCost: 2, noMaterialComponents: true },
        ],
      });
      await giveFeat(m, campaignId, table.characterId, feat);

      // The editor reads the cost and the switch back.
      await m.goto(entryRoute(campaignId, feat));
      await expect(m.getByLabel('Usos gastos por conjuração')).toHaveValue('2');
      await expect(m.getByLabel('Sem componentes materiais')).toBeChecked();
      await expect(m.getByLabel('Só como ritual')).not.toBeChecked();

      await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
      await openSessionPage(p, campaignId);
      await openActionTab(p, 'Magias');
      await p.getByRole('button', { name: 'Conjurar Bênção' }).click();
      const sheet = p.getByRole('dialog', { name: 'Conjurar Bênção' });
      await viaTargetList(p, sheet);
      await expect(sheet.getByText('Graça de teste · gasta 2 · 3 usos')).toBeVisible();

      await sheet.getByRole('button', { name: 'Detalhes de Bênção' }).click();
      const details = p.getByRole('dialog', { name: /Bênção/ }).last();
      await expect(details.getByText('Componentes')).toBeVisible();
      await expect(details.getByText('V, S · sem material (Graça de teste)', { exact: true })).toBeVisible();
      await expect(details.getByText('V, S, M')).toHaveCount(0);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

// #881: a granted spell cast above its own level. The feat is invented; the spell is the SRD's Bless (1st level), granted at
// will and cast as a 2nd-level spell. The sheet row, the combat cast sheet and the cast sheet outside a combat say the level.
async function featCastAtSecondLevel(master: Page, campaignId: string, characterId: string): Promise<void> {
  const feat = await createEntryRPC(master, campaignId, 'tableFeat', {
    namePt: 'Graça de teste',
    descPt: ['Um dom de teste.'],
    effects: [{ type: 'granted_spell', spell: 'spell:bless', ability: 'cha', uses: 'at_will', castLevel: 2 }],
  });
  await giveFeat(master, campaignId, characterId, feat);
}

test(
  'a linha da magia concedida na ficha diz que ela é conjurada no 2º nível',
  { tag: ['@881'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    let campaignId = '';
    const m = await master.newPage();
    try {
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const table = await tableForCasting(m, p, `Nível concedido ${Date.now()}`);
      campaignId = table.campaignId;
      await featCastAtSecondLevel(m, campaignId, table.characterId);

      await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      const row = p.locator('.granted__item', { hasText: 'Bênção' });
      await expect(row.getByText('À vontade', { exact: true })).toBeVisible();
      await expect(row.getByText('Conjurada no\u00a02º\u00a0nível', { exact: true })).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'a folha de conjuração fora do combate diz que a magia concedida é conjurada no 2º nível',
  { tag: ['@881'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    let campaignId = '';
    const m = await master.newPage();
    try {
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const table = await tableForCasting(m, p, `Nível fora ${Date.now()}`);
      campaignId = table.campaignId;
      await featCastAtSecondLevel(m, campaignId, table.characterId);

      await openSessionPage(p, campaignId);
      const sheet = await openCastOf(p, 'Bênção');
      await expect(castSheet(p)).toBeVisible();
      await expect(sheet.getByText('À vontade · Carisma · CD 12 · ataque +4 · no\u00a02º\u00a0nível', { exact: true })).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

test(
  'a folha de conjuração do combate diz que a magia concedida é conjurada no 2º nível',
  { tag: ['@881'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 900 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 390, height: 844 } });
    let campaignId = '';
    const m = await master.newPage();
    try {
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const table = await tableForCombat(m, p, `Nível no combate ${Date.now()}`, true, true, { sheet: pensantusCasting });
      campaignId = table.campaignId;
      await featCastAtSecondLevel(m, campaignId, table.characterId);

      await beginAttackCombatRPC(m, table, { Pensantus: 20, 'Capitão Goblin': 15, 'Goblin 1': 5, 'Goblin 2': 4 });
      await openSessionPage(p, campaignId);
      await openActionTab(p, 'Magias');
      await p.getByRole('button', { name: 'Conjurar Bênção' }).click();
      const sheet = p.getByRole('dialog', { name: 'Conjurar Bênção' });
      await viaTargetList(p, sheet);
      await expect(sheet.getByText('Graça de teste · À vontade · no\u00a02º\u00a0nível')).toBeVisible();
      await expect(sheet.getByText('Escolha até 4 alvos')).toBeVisible();
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await master.close();
      await player.close();
    }
  },
);

async function giveFeat(page: Page, campaignId: string, characterId: string, feat: string): Promise<void> {
  const got = await callRPC(page, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
  expect(got.ok(), await got.text()).toBeTruthy();
  const c = (await got.json()).character as { id: string; name: string; revision: number; sheet: { full: Record<string, unknown> } };
  const featKeys = [...((c.sheet.full.featKeys as string[] | undefined) ?? []), feat];
  const gave = await callRPC(page, 'meurpg.characters.v1.CharacterService/UpdateCharacter', {
    campaignId,
    characterId: c.id,
    revision: c.revision,
    name: c.name,
    sheet: { full: { ...c.sheet.full, featKeys } },
  });
  expect(gave.ok(), await gave.text()).toBeTruthy();
}
