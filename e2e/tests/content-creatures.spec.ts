import { expect, type Locator, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { createEntryRPC, creatureBody, damageCreatureBody, entryRoute, listEntriesJSON } from './content-support';
import { campaignWithEmptyPlayer } from './table-rules-support';
import { callRPC, newSignedInContext } from './support';

test.describe.configure({ timeout: 120_000 });

// MR-025 (RN-23, RN-10; #578): the table's creatures. The master makes one from an SRD creature in the editor (the base, the name, four numbers
// each with a fixed value and a formula of the owner's sheet, a trait and an attack), edits its damage lines (resistances, immunities,
// vulnerabilities), archives it; on a phone he reads it and archives it from the menu; a player who holds none never receives it.
// Every test makes its own campaign through the API.

const service = 'meurpg.rules.v1.TableContentService';

/** The "Vale contra" select of the damage row `n` (1-based), and the rest of the row's controls. */
const kindOf = (m: Page, n: number): Locator => m.getByRole('combobox', { name: `Tipo da linha ${n}`, exact: true });
const typesOf = (m: Page, n: number): Locator => m.getByRole('combobox', { name: `Tipos de dano da linha ${n}`, exact: true });
const qualifierOf = (m: Page, n: number): Locator => m.getByRole('combobox', { name: `Vale contra da linha ${n}`, exact: true });

/** Opens a Material select and picks the option with this exact name; a multiple select stays open, so Escape closes it. */
async function pickOption(m: Page, select: Locator, name: string, multiple = false): Promise<void> {
  await select.click();
  await m.getByRole('option', { name, exact: true }).click();
  if (multiple) {
    await m.keyboard.press('Escape');
  }
  await expect(m.getByRole('listbox')).toHaveCount(0);
}

test(
  'o mestre cria o Lobo do Vale a partir do Lobo, com números fixos e fórmulas, um traço e um ataque, e o vê na lista @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo criatura ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content?kind=creatures`);
      await expect(m.getByRole('heading', { level: 1, name: 'Conteúdo da mesa' })).toBeVisible();
      await expect(m.getByText('Nenhuma criatura da mesa ainda.')).toBeVisible();
      await m.getByRole('link', { name: 'Nova criatura' }).click();
      await expect(m.getByRole('heading', { level: 1, name: 'Nova criatura' })).toBeVisible();

      // The base comes first, from the SRD only; the numbers and the rows wait for it.
      await expect(m.getByRole('heading', { level: 2, name: 'Números' })).toHaveCount(0);
      await m.getByLabel('Criatura do SRD que serve de base').fill('Lobo');
      await m.getByRole('option').filter({ has: m.locator('.pick__pt', { hasText: /^Lobo$/ }) }).click();
      await expect(m.getByText('Lobo, do SRD')).toBeVisible();
      await expect(m.getByLabel('Nome', { exact: true })).toBeFocused();
      await m.getByLabel('Nome', { exact: true }).fill('Lobo do Vale');

      // Four numbers: a fixed value for a creature with no owner and a formula of the owner's sheet, which wins.
      await m.getByLabel('Pontos de vida, valor fixo').fill('30');
      await m.getByLabel('Pontos de vida, fórmula com o dono').fill('4 + 4 * level()');
      await m.getByLabel('Classe de armadura, valor fixo').fill('14');
      await m.getByLabel('Classe de armadura, fórmula com o dono').fill('13 + prof()');
      await m.getByLabel('Bônus no dano, valor fixo').fill('2');
      await m.getByLabel('Bônus no dano, fórmula com o dono').fill('prof()');
      await m.getByLabel('Bônus de ataque, fórmula com o dono').fill('mod("wis") + prof()');

      // A trait (text only) and an attack that comes after the base's actions.
      await m.getByRole('button', { name: 'Adicionar traço' }).click();
      await m.getByLabel('Nome do traço').fill('Faro do Vale');
      await m.getByLabel('Texto do traço').fill('Segue o cheiro de quem cruzou o vale, mesmo na chuva.');
      await m.getByRole('button', { name: 'Adicionar ataque' }).click();
      await m.getByLabel('Nome do ataque').fill('Mordida Sombria');
      await m.getByLabel('Alcance (m)').fill('1,5');
      await m.getByLabel('Dados', { exact: true }).fill('2');
      await m.getByLabel('Dado', { exact: true }).selectOption({ label: 'd6' });
      await m.getByLabel('Tipo de dano', { exact: true }).selectOption({ label: 'Necrótico' });

      // The side panel is written from the form: the creature as the table reads it.
      const preview = m.getByRole('complementary', { name: 'Como fica na mesa' });
      await expect(preview.getByText('Lobo do Vale')).toBeVisible();
      await expect(preview.getByText('Mordida Sombria')).toBeVisible();

      await m.getByRole('button', { name: 'Salvar criatura' }).click();
      await expect(m).toHaveURL(/\/content\/entries\//);
      await expect(m.getByText('A criatura Lobo do Vale foi salva.')).toBeVisible();
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Lobo do Vale');

      // What the server stored is what the form said.
      const stored = (await listEntriesJSON(m, campaignId)).entries!.find((e) => e.namePt === 'Lobo do Vale')!;
      expect(stored.tableCreature).toMatchObject({
        namePt: 'Lobo do Vale',
        baseKey: 'monster:wolf',
        hitPoints: 30,
        hitPointsFormula: '4 + 4 * level()',
        armorClass: 14,
        armorClassFormula: '13 + prof()',
        damageBonus: 2,
        damageBonusFormula: 'prof()',
        attackBonusFormula: 'mod("wis") + prof()',
      });
      expect(stored.tableCreature.traits).toMatchObject([{ namePt: 'Faro do Vale', textPt: 'Segue o cheiro de quem cruzou o vale, mesmo na chuva.' }]);
      expect(stored.tableCreature.attacks).toMatchObject([
        { namePt: 'Mordida Sombria', melee: true, reachFt: 5, diceCount: 2, diceSides: 6, damageType: 'damage-type:necrotic' },
      ]);

      // The list has it under "Criaturas", with the count; the saved read has the numbers, the trait and the attack in their rows.
      await m.goto(`/campaigns/${campaignId}/content`);
      await expect(m.getByRole('link', { name: /Criaturas/ })).toContainText('1');
      await m.getByRole('link', { name: /Criaturas/ }).click();
      await expect(m.getByRole('heading', { level: 2, name: 'Criaturas' })).toBeVisible();
      const rows = m.locator('a.row');
      await expect(rows).toHaveCount(1);
      await expect(rows.first()).toContainText('Lobo do Vale');
      await rows.first().click();
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Lobo do Vale');
      await expect(m.getByLabel('Pontos de vida, valor fixo')).toHaveValue('30');
      await expect(m.getByLabel('Classe de armadura, fórmula com o dono')).toHaveValue('13 + prof()');
      await expect(m.getByRole('button', { name: /^Faro do Vale/ })).toBeVisible();
      await expect(m.getByRole('button', { name: /^Mordida Sombria/ })).toBeVisible();
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o mestre edita as linhas de dano da criatura: adiciona, vê a vulnerabilidade sem "Vale contra", é recusado no campo, remove e arquiva @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo linhas ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableCreature', creatureBody());

      await m.goto(entryRoute(campaignId, key));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Lobo do Vale');
      const section = m.getByRole('region', { name: 'Resistências, imunidades e vulnerabilidades' });
      await expect(section.getByText('Nenhuma linha da mesa ainda.')).toBeVisible();
      await expect(section.getByText('0 de 12')).toBeVisible();

      // A resistance to the three weapon types, with "Vale contra" open to it.
      await section.getByRole('button', { name: 'Adicionar linha' }).click();
      await expect(kindOf(m, 1)).toBeFocused();
      await pickOption(m, kindOf(m, 1), 'Resistência');
      await pickOption(m, typesOf(m, 1), 'Concussão', true);
      await expect(qualifierOf(m, 1)).not.toHaveAttribute('aria-disabled', 'true');
      await pickOption(m, qualifierOf(m, 1), 'Ataque não mágico nem prateado');

      // A vulnerability to radiant: "Vale contra" is off, and the reason is what the select is described by.
      await section.getByRole('button', { name: 'Adicionar linha' }).click();
      await pickOption(m, kindOf(m, 2), 'Vulnerabilidade');
      await pickOption(m, typesOf(m, 2), 'Radiante', true);
      await expect(qualifierOf(m, 2)).toHaveAttribute('aria-disabled', 'true');
      await expect(qualifierOf(m, 2)).toHaveAttribute('aria-describedby', 'cr-mods-info');
      await expect(m.locator('#cr-mods-info')).toContainText('Outro "Vale contra" só para resistência ou imunidade a concussão, perfurante e cortante.');

      // A third row with no damage type is refused by the server, on its select, in its words.
      await section.getByRole('button', { name: 'Adicionar linha' }).click();
      await pickOption(m, kindOf(m, 3), 'Imunidade');
      await m.getByRole('button', { name: 'Salvar criatura' }).click();
      await expect(typesOf(m, 3)).toHaveAttribute('aria-invalid', 'true');
      const refusal = section.getByText('Escolha ao menos um tipo de dano.');
      await expect(refusal).toBeVisible();
      await expect(typesOf(m, 3)).toHaveAttribute('aria-describedby', /^cr-tn-/);
      await expect(m.getByRole('alert').filter({ hasText: 'Não foi possível salvar a criatura.' })).toContainText('1 campo precisa de ajuste');
      await expect(typesOf(m, 3)).toBeFocused();
      // Nothing was saved with the refused row.
      expect((await listEntriesJSON(m, campaignId)).entries![0].tableCreature.damageMods ?? []).toEqual([]);

      // The type has no repeat to pick: the server refuses one the API is given, and says where.
      const dup = await callRPC(m, `${service}/CreateTableEntry`, {
        campaignId,
        tableCreature: damageCreatureBody('Defunto Repetido', {
          damageMods: [{ kind: 'immune', damageTypes: ['damage-type:necrotic', 'damage-type:necrotic'] }],
        }),
      });
      expect(dup.ok()).toBeFalsy();
      expect(await dup.text()).toContain('table_creature.damage_mods[0].damage_types[1]');

      // Remove the refused row from its menu; the other two save.
      await section.getByRole('button', { name: 'Mais ações da linha 3', exact: true }).click();
      await m.getByRole('menuitem', { name: 'Remover linha' }).click();
      await expect(section.getByRole('combobox', { name: 'Tipo da linha 3', exact: true })).toHaveCount(0);
      await expect(section.getByText('Escolha ao menos um tipo de dano.')).toHaveCount(0);
      await m.getByRole('button', { name: 'Salvar criatura' }).click();
      await expect(m.getByText('A criatura Lobo do Vale foi salva.')).toBeVisible();
      const stored = (await listEntriesJSON(m, campaignId)).entries![0].tableCreature;
      expect(stored.damageMods).toEqual([
        { kind: 'resist', damageTypes: ['damage-type:bludgeoning'], qualifier: 'nonmagical_unless_silvered' },
        { kind: 'vulnerable', damageTypes: ['damage-type:radiant'] },
      ]);

      // The master archives it with the question in place; the player never receives it.
      await m.getByRole('button', { name: 'Arquivar', exact: true }).click();
      const question = m.getByRole('region', { name: 'Arquivar Lobo do Vale?' });
      await expect(question).toBeVisible();
      await question.getByRole('button', { name: 'Arquivar Lobo do Vale' }).click();
      await expect(m.getByText('A criatura Lobo do Vale está arquivada.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Desarquivar' })).toBeVisible();
      expect((await listEntriesJSON(m, campaignId)).entries![0].archived).toBe(true);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o mestre no celular lê a criatura com as linhas de dano e os traços e arquiva pelo menu ⋮ @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 390, height: 844 } });
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo celular ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableCreature', damageCreatureBody());

      await m.goto(entryRoute(campaignId, key));
      await expect(m.getByRole('heading', { level: 1, name: 'Defunto do Vale' })).toBeVisible();
      // It reads and archives; the editor is on a laptop.
      await expect(m.getByText('Para criar ou editar, abra o conteúdo da mesa no notebook. Aqui você lê e arquiva.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Salvar criatura' })).toHaveCount(0);
      await expect(m.getByLabel('Nome', { exact: true })).toHaveCount(0);
      // The board: the base, the damage lines with where each comes from, and the master-only note.
      await expect(m.getByText('Zumbi (SRD)')).toBeVisible();
      await expect(m.getByText('Resistência', { exact: true })).toBeVisible();
      await expect(m.getByText(/concussão, perfurante e cortante.*\(da mesa\)/)).toBeVisible();
      await expect(m.getByText('Vulnerabilidade', { exact: true })).toBeVisible();
      await expect(m.getByText('radiante (da mesa)')).toBeVisible();
      await expect(m.getByText('Imunidade', { exact: true })).toBeVisible();
      await expect(m.getByText('necrótico (da mesa)')).toBeVisible();
      await expect(m.getByText('Só você muda estas linhas. Quem tem a criatura as lê na ficha dela.')).toBeVisible();
      // The trait in the card of traits and attacks.
      const card = m.getByRole('region', { name: 'Traços e ataques' });
      await expect(card.getByText('Cheiro de cripta')).toBeVisible();
      await expect(card.getByText('Quem está a menos de 3 m sente o cheiro de terra molhada.')).toBeVisible();
      await expect(card.getByText('Pancada (da base)')).toBeVisible();

      // Archive from the ⋮ menu: the question is a sheet.
      await m.getByRole('button', { name: 'Mais ações de Defunto do Vale' }).click();
      await m.getByRole('menuitem', { name: 'Arquivar' }).click();
      await expect(m.getByRole('heading', { name: 'Arquivar Defunto do Vale?' })).toBeVisible();
      await m.getByRole('dialog').getByRole('button', { name: 'Arquivar', exact: true }).click();
      await expect(m.getByText('A criatura Defunto do Vale está arquivada.')).toBeVisible();
      expect((await listEntriesJSON(m, campaignId)).entries![0].archived).toBe(true);
      await m.getByRole('button', { name: 'Mais ações de Defunto do Vale' }).click();
      await expect(m.getByRole('menuitem', { name: 'Desarquivar' })).toBeVisible();
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o jogador que não tem a criatura nunca recebe "Criaturas": a lista fica vazia e a rota da entrada não a mostra @RN-23',
  { tag: ['@MR-025', '@RN-23', '@RN-10'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste', { viewport: { width: 1280, height: 800 } });
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo sigilo ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableCreature', damageCreatureBody());

      // Positive control: the master has the entry and reads it.
      const masters = (await listEntriesJSON(m, campaignId)).entries!;
      expect(masters.map((e) => e.namePt)).toEqual(['Defunto do Vale']);
      expect(JSON.stringify(masters)).toContain('damageMods');
      await m.goto(`/campaigns/${campaignId}/content?kind=creatures`);
      await expect(m.locator('a.row')).toContainText('Defunto do Vale');
      await m.goto(entryRoute(campaignId, key));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Defunto do Vale');

      // The player gets none of it: the server sends no entry, no key, no damage line.
      const raw = await callRPC(p, `${service}/ListTableEntries`, { campaignId });
      expect(raw.ok(), await raw.text()).toBeTruthy();
      const text = await raw.text();
      expect(JSON.parse(text).entries ?? []).toEqual([]);
      for (const secret of ['Defunto do Vale', 'Cheiro de cripta', 'damageMods', 'tableCreature', key]) {
        expect(text).not.toContain(secret);
      }

      await p.goto(`/campaigns/${campaignId}/content`);
      await expect(p.getByRole('heading', { level: 1, name: 'Conteúdo da mesa' })).toBeVisible();
      await expect(p.getByText('O mestre ainda não criou nada para esta mesa.')).toBeVisible();
      await expect(p.getByRole('heading', { name: 'Criaturas' })).toHaveCount(0);
      await expect(p.getByText('Defunto do Vale')).toHaveCount(0);

      // A direct route to the entry finds nothing, and shows no editor.
      await p.goto(entryRoute(campaignId, key));
      await expect(p.getByRole('heading', { level: 1, name: 'Entrada não encontrada' })).toBeVisible();
      await expect(p.getByText('Defunto do Vale')).toHaveCount(0);
      await expect(p.getByText('Cheiro de cripta')).toHaveCount(0);
      await expect(p.getByRole('button', { name: 'Salvar criatura' })).toHaveCount(0);

      // A player cannot write one.
      const denied = await callRPC(p, `${service}/CreateTableEntry`, { campaignId, tableCreature: creatureBody('Lobo do Jogador') });
      expect(denied.status()).toBe(403);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);
