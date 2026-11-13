import { expect, type Locator, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { createEntryRPC, creatureBody, expectCardsHoldTheirText, listEntriesJSON } from './content-support';
import { tableForCaster, listCreaturesRPC } from './creatures-support';
import { endOpenSessionRPC } from './live-session-support';
import { newSignedInContext, shellSettled } from './support';

test.describe.configure({ timeout: 180_000 });

// MR-025 and MR-037 (RN-23; #578): the table's summons. The master gives a class feature the block "Invoca criaturas" (the creatures, how many,
// the resource a use is spent from) and a spell the mechanic "Invocar"; a player whose class has the feature uses it, chooses the
// table creature in the list ("Da mesa", with the note on its numbers) and reads its page. Every test makes its own campaign.

/** The "Criaturas" search of a summon: types the name and picks the first match (the table's creatures and the SRD's share the list). */
async function addCreature(m: Page, scope: Locator, name: string): Promise<void> {
  await scope.getByRole('combobox', { name: 'Criaturas' }).fill(name);
  await m.getByRole('option').filter({ has: m.locator('.pick__pt', { hasText: new RegExp(`^${name}$`) }) }).click();
  await expect(scope.getByRole('group', { name: /^Criaturas que a/ }).getByText(name, { exact: true })).toBeVisible();
}

test(
  'o mestre salva a característica que invoca criaturas e a magia com Invocar, e o servidor guarda o que o formulário disse @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste', { viewport: { width: 1280, height: 800 } });
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const { campaignId } = await tableForCaster(m, p, `Conteúdo invoca ${Date.now()}`, 'druid');
    try {
      const wolf = await createEntryRPC(m, campaignId, 'tableCreature', creatureBody());

      // C2: the feature of the class, with a resource of its own and the block that summons.
      await m.goto(`/campaigns/${campaignId}/content/new/classfeature`);
      await m.getByLabel('Classe', { exact: true }).selectOption({ label: 'Druida' });
      await m.getByLabel('Nível na classe (1 a 20)').fill('1');
      await m.getByLabel('Nome da característica').fill('Companheiro do Vale');
      await m.getByLabel('Texto da característica').fill('Um lobo do vale atende ao seu chamado.');
      await m.getByLabel('Efeito', { exact: true }).selectOption({ label: 'Recurso' });
      await m.getByLabel('Nome do recurso').fill('chamados');
      await m.getByLabel('Usos (máximo)').fill('2');
      await m.getByLabel('Volta em').selectOption({ label: 'Descanso longo' });

      const block = m.locator('app-summon-block');
      await expect(block.getByRole('switch', { name: 'Invoca criaturas' })).toHaveAttribute('aria-checked', 'false');
      await block.getByRole('switch', { name: 'Invoca criaturas' }).click();
      await expect(block.getByRole('switch', { name: 'Invoca criaturas' })).toHaveAttribute('aria-checked', 'true');
      await addCreature(m, block, 'Lobo do Vale');
      await block.getByLabel('Quantas vêm').fill('1');
      await block.getByLabel('No máximo ao mesmo tempo').fill('2');
      await block.getByLabel('Gasta 1 uso de').selectOption({ label: 'Companheiro do Vale (2)' });
      await expectCardsHoldTheirText(m);
      await block.locator('label', { hasText: 'Em grupo' }).click();
      await block.locator('label', { hasText: 'As novas tomam o lugar' }).click();
      await m.getByRole('button', { name: 'Salvar característica' }).click();
      await expect(m).toHaveURL(/\/content\/entries\//);
      await expect(m.getByText('A característica de classe Companheiro do Vale foi salva.')).toBeVisible();

      const feature = (await listEntriesJSON(m, campaignId)).entries!.find((e) => e.namePt === 'Companheiro do Vale')!;
      expect(feature.tableClassFeature.summon).toMatchObject({
        creatureKeys: [wolf],
        count: '1',
        maxLive: 2,
        resourceKey: 'chamados',
        turnMode: 'TABLE_SUMMON_TURN_MODE_OWNER_ROLL_GROUP',
        replaces: 'TABLE_SUMMON_REPLACES_SAME_SUMMON',
      });

      // C3: a spell with "Invocar", one creature and one more for each circle above the first.
      await m.goto(`/campaigns/${campaignId}/content/new/spell`);
      await m.getByLabel('Nome', { exact: true }).fill('Chamado do Vale');
      await m.getByLabel('Descrição').fill('Um lobo do vale atravessa o véu e fica ao seu lado.');
      await m.locator('.classes label', { hasText: 'Druida' }).click();
      await m.locator('[aria-label="Mecânica"] label', { hasText: 'Invocar' }).click();
      const spell = m.locator('app-spell-summon');
      await expect(spell.getByText('Sem recurso, tempo nem duração: a magia tem os seus.')).toBeVisible();
      await addCreature(m, spell, 'Lobo do Vale');
      await spell.getByLabel('Quantas vêm').fill('1');
      await spell.getByLabel('A mais por círculo acima').fill('1');
      await spell.getByLabel('No máximo ao mesmo tempo').fill('4');
      await expect(m.getByRole('complementary', { name: 'Como os jogadores veem' }).getByText('Só quem conjura')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      await expect(m).toHaveURL(/\/content\/entries\//);
      await expect(m.getByText('A magia Chamado do Vale foi salva.')).toBeVisible();

      const saved = (await listEntriesJSON(m, campaignId)).entries!.find((e) => e.namePt === 'Chamado do Vale')!;
      expect(saved.tableSpell.summon).toMatchObject({ creatureKeys: [wolf], count: '1', countPerSlotLevel: 1, maxLive: 4 });
      expect(saved.tableSpell.target.kind).toBe('TABLE_SPELL_TARGET_KIND_SELF');
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'a jogadora usa a característica que invoca, escolhe a criatura "Da mesa" na lista e lê a ficha dela com a nota dos números @MR-037',
  { tag: ['@MR-037', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const table = await tableForCaster(m, p, `Invoca jogadora ${Date.now()}`, 'druid');
    const campaignId = table.campaignId;
    try {
      const wolf = await createEntryRPC(m, campaignId, 'tableCreature', creatureBody());
      await createEntryRPC(m, campaignId, 'tableClassFeature', {
        namePt: 'Companheiro do Vale',
        descPt: ['Um lobo do vale atende ao seu chamado.'],
        classKey: 'class:druid',
        level: 1,
        effects: [{ type: 'resource', resource: 'chamados', max: '2', recharge: 'long_rest' }],
        summon: {
          creatureKeys: [wolf, 'monster:wolf'],
          count: '1',
          maxLive: 2,
          resourceKey: 'chamados',
          turnMode: 'TABLE_SUMMON_TURN_MODE_OWN_INITIATIVE',
          replaces: 'TABLE_SUMMON_REPLACES_NONE',
        },
      });

      await p.goto(`/campaigns/${campaignId}/characters/${table.characterId}`);
      const panel = p.locator('app-creatures-panel');
      await panel.getByRole('button', { name: 'Companheiro do Vale' }).click();
      const sheet = p.getByRole('dialog', { name: 'Companheiro do Vale' }).or(p.locator('mat-bottom-sheet-container'));

      // C6: the table creature is marked and says whose numbers it gets; the SRD wolf has its own line.
      const rows = sheet.locator('app-creature-choice-list label.row');
      await expect(rows).toHaveCount(2);
      // The header says where the summon comes from and what it spends; the table's creature is first; the book's row has its numbers and its attack.
      await expect(sheet.locator('.frame__sub')).toHaveText('Druida 1 · 1 ação · gasta 1 uso (restam 2)');
      await expect(rows.first().locator('app-table-mark')).toHaveText(/Da mesa/);
      const tableRow = rows.filter({ hasText: 'Lobo do Vale' });
      await expect(tableRow.locator('app-table-mark')).toHaveText(/Da mesa/);
      await expect(tableRow).toContainText('Os números vêm da sua ficha quando ele chega.');
      // The row without the mark is the book's: exactly one, and the table's note is not on it.
      const bookRow = rows.filter({ hasNot: p.locator('app-table-mark') });
      await expect(bookRow).toHaveCount(1);
      await expect(bookRow.locator('.row__title')).toHaveText('Lobo');
      await expect(bookRow).toContainText(/ND\s*1\/4.*CA\s*13.*PV\s*11/);
      await expect(bookRow).toContainText('Mordida');
      await expect(bookRow).not.toContainText('Os números vêm da sua ficha');
      await tableRow.click();
      await sheet.getByRole('button', { name: 'Conjurar Companheiro do Vale', exact: true }).click();
      await expect(panel.getByRole('status')).toContainText('A criatura chegou.');

      // C5: the creature's page tells where it came from and whose sheet gives its numbers.
      const created = await listCreaturesRPC(p, campaignId, table.characterId);
      expect(created.map((c) => c.monsterKey)).toEqual([wolf]);
      await panel.getByRole('link', { name: /^Ver a ficha de / }).click();
      await expect(p.getByRole('heading', { level: 1, name: 'Lobo do Vale' })).toBeVisible();
      await expect(p.locator('.head__origin app-table-mark')).toHaveText(/Da mesa/);
      await expect(p.locator('.table-note')).toHaveText('Os números vêm da ficha de Sálvia quando ele chegou');
      // The page is compact (the numbers worked out for the owner, the creature's own attacks and traits), so the note that ends it is on the first screen.
      // The wolf's formula is 13 + proficiency: the page shows the owner's number, never the base's 13.
      expect(created[0].armorClass).toBeGreaterThan(13);
      await expect(p.locator('.tiles')).toContainText(new RegExp(`CA\\s*${created[0].armorClass}(?!\\d)`));
      await expect(p.locator('.attacks')).toContainText('Mordida Sombria');
      await expect(p.locator('.block').last()).toContainText('Faro do Vale');
      await expect(p.locator('.abilities')).toHaveCount(0);
      // On a phone (390x844) the whole note is on the first screen.
      // The page is loaded at that size, not resized in place: a page resized in place can still be styled for the old width
      // when it is measured (390 px wide with the 768 px styles), however many frames the test waits.
      await p.setViewportSize({ width: 390, height: 844 });
      await p.reload();
      await expect(p.locator('.table-note')).toHaveText('Os números vêm da ficha de Sálvia quando ele chegou');
      await shellSettled(p);
      // The shell's live notice sits above the page and is dismissible; whether it shows depends on when the session's hint arrives, so the page's own content is measured below it (notice and note read at the same instant).
      const { noteBottom, noticeHeight, layout } = await p.evaluate(() => ({
        noteBottom: document.querySelector('.table-note')!.getBoundingClientRect().bottom,
        noticeHeight: [...document.querySelectorAll('app-live-notice .live-notice')].reduce(
          (sum, n) => sum + n.getBoundingClientRect().height + parseFloat(getComputedStyle(n).marginBottom),
          0,
        ),
        layout: `${innerWidth} px wide, ${matchMedia('(min-width: 768px)').matches ? 'wide' : 'phone'} media`,
      }));
      expect(noteBottom - noticeHeight, `note bottom ${noteBottom}, notice ${noticeHeight}, ${layout}`).toBeLessThan(844);
    } finally {
      await endOpenSessionRPC(m, campaignId);
      await Promise.all([master.close(), player.close()]);
    }
  },
);
