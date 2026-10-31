import { expect, type Page } from '@playwright/test';
import { test } from './diagnostics';

import { archiveEntryRPC, createEntryRPC, entryRoute, listEntriesJSON, raceBody, spellBody, updateEntryRPC } from './content-support';
import { pickRadio } from './move-support';
import { campaignWithEmptyPlayer } from './table-rules-support';
import { callRPC, characterRpcBody, createCharacterRPC, createCharacterViaUI, newSignedInContext, pensantus } from './support';

test.describe.configure({ timeout: 120_000 });

// MR-025 (RN-23, RN-10): the table's own content, by the master's editors and the players' reading. Every test makes its
// own campaign through the API; the master signs in with the saved state.


test(
  'o mestre cria a Lâmina de Nanquim e o Sopro de Nanquim e vê as duas na lista @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo magias ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content?kind=spells`);
      await expect(m.getByRole('heading', { level: 1, name: 'Conteúdo da mesa' })).toBeVisible();
      await expect(m.getByText('Nada cadastrado ainda.')).toBeVisible();
      await m.getByRole('link', { name: 'Nova magia' }).click();
      await expect(m.getByRole('heading', { level: 1, name: 'Nova magia' })).toBeVisible();

      // A spell with an attack: one creature, the range as a distance in metres.
      await m.getByLabel('Nome', { exact: true }).fill('Lâmina de Nanquim');
      await m.getByLabel('Distância').fill('18');
      await m.getByRole('checkbox', { name: 'M', exact: true }).check({ force: true });
      await m.getByLabel('Material').fill('uma pena molhada em tinta');
      await pickRadio(m, 'Ataque');
      await m.getByLabel('Dano', { exact: true }).fill('2d8');
      await m.getByLabel('Mais dano por nível acima do 1º').fill('1d8');
      await m.getByRole('checkbox', { name: 'Mago', exact: true }).check({ force: true });
      await m.getByLabel('Descrição').fill('Um risco de tinta negra corta o ar e rasga o alvo.');
      // The preview is written from the form as the players read it.
      const preview = m.getByRole('complementary', { name: 'Como os jogadores veem' });
      await expect(preview.getByText('Uma criatura')).toBeVisible();
      await expect(preview.getByText('Ataque de magia à distância')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      await expect(m).toHaveURL(/\/content\/entries\//);
      await expect(m.getByText('A magia Lâmina de Nanquim foi salva.')).toBeVisible();

      // An area spell: the size label follows the shape.
      await m.goto(`/campaigns/${campaignId}/content/new/spell`);
      await m.getByLabel('Nome', { exact: true }).fill('Sopro de Nanquim');
      await m.getByLabel('Alcance', { exact: true }).selectOption({ label: 'Pessoal' });
      await pickRadio(m, 'Área');
      await expect(m.getByLabel('Comprimento')).toBeVisible();
      await m.getByLabel('Forma', { exact: true }).selectOption({ label: 'Cubo' });
      await expect(m.getByLabel('Lado', { exact: true })).toBeVisible();
      await m.getByLabel('Forma', { exact: true }).selectOption({ label: 'Cone' });
      await m.getByLabel('Comprimento').fill('4,5');
      await pickRadio(m, 'Teste de resistência');
      await m.getByLabel('Dano', { exact: true }).fill('3d6');
      await m.getByRole('checkbox', { name: 'Mago', exact: true }).check({ force: true });
      await m.getByLabel('Descrição').fill('Uma lufada de tinta negra se abre à frente de quem conjura.');
      await expect(preview.getByText(/Cone de 4,5/)).toBeVisible();
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      await expect(m.getByText('A magia Sopro de Nanquim foi salva.')).toBeVisible();

      await m.goto(`/campaigns/${campaignId}/content?kind=spells`);
      const rows = m.locator('a.row');
      await expect(rows).toHaveCount(2);
      await expect(rows.nth(0)).toContainText('Lâmina de Nanquim');
      await expect(rows.nth(1)).toContainText('Sopro de Nanquim');
      await expect(m.getByRole('link', { name: /Magias/ })).toContainText('2');

      // What the server stored is what the form said.
      const stored = (await listEntriesJSON(m, campaignId)).entries!.find((e) => e.namePt === 'Sopro de Nanquim')!;
      expect(stored.tableSpell.target).toMatchObject({ kind: 'TABLE_SPELL_TARGET_KIND_AREA', shape: 'TABLE_AREA_SHAPE_CONE', sizeFt: 15 });
      expect(stored.tableSpell.range.kind).toBe('SPELL_RANGE_KIND_SELF');
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test('uma recusa do servidor volta no campo, e o que foi digitado fica @MR-025', { tag: ['@MR-025', '@RN-23'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo recusa ${Date.now()}`);
    await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));

    await m.goto(`/campaigns/${campaignId}/content/new/spell`);
    await m.getByLabel('Nome', { exact: true }).fill('Lâmina de Nanquim');
    await m.getByLabel('Distância').fill('18');
    await m.getByRole('button', { name: 'Salvar magia' }).click();
    const name = m.getByLabel('Nome', { exact: true });
    await expect(name).toHaveAttribute('aria-invalid', 'true');
    await expect(m.getByText('Já existe uma magia da mesa com este nome. Escolha outro.')).toBeVisible();
    await expect(m.getByRole('alert').filter({ hasText: 'Não foi possível salvar a magia.' })).toContainText('1 campo precisa de ajuste');
    await expect(name).toBeFocused();
    await expect(name).toHaveValue('Lâmina de Nanquim');
    await expect(m).toHaveURL(/\/content\/new\/spell$/);

    // Another name saves: the refusal is gone.
    await name.fill('Lâmina de Tinta');
    await m.getByRole('button', { name: 'Salvar magia' }).click();
    await expect(m.getByText('A magia Lâmina de Tinta foi salva.')).toBeVisible();

    // A dice the server cannot read is refused on the damage field.
    await m.goto(`/campaigns/${campaignId}/content/new/spell`);
    await m.getByLabel('Nome', { exact: true }).fill('Lâmina Torta');
    await m.getByLabel('Distância').fill('18');
    await pickRadio(m, 'Ataque');
    await m.getByLabel('Dano', { exact: true }).fill('2d8x');
    await m.getByRole('button', { name: 'Salvar magia' }).click();
    await expect(m.getByLabel('Dano', { exact: true })).toHaveAttribute('aria-invalid', 'true');
    await expect(m.getByText('Escreva o dado assim: 2d8 (de 1 a 20 dados).')).toBeVisible();
  } finally {
    await Promise.all([master.close(), player.close()]);
  }
});

test(
  'o mestre cria a raça Corujeiro com um efeito num traço e o jogador cria um personagem com ela @MR-025',
  { tag: ['@MR-025', '@RN-23', '@RN-10'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo raça ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content/new/race`);
      await m.getByLabel('Nome', { exact: true }).fill('Corujeiro');
      await m.getByLabel('Deslocamento').fill('9');
      await m.getByLabel('Visão no escuro').fill('18');
      await m.getByRole('button', { name: 'Mais Sabedoria' }).click();
      await m.getByRole('button', { name: 'Mais Sabedoria' }).click();
      await m.getByRole('button', { name: 'Mais Destreza' }).click();
      await m.getByLabel('Adicionar idioma').selectOption({ label: 'Comum' });
      await m.getByRole('button', { name: 'Adicionar traço' }).click();
      await m.getByLabel('Nome do traço').fill('Olhos de caçador');
      await m.getByLabel('Texto', { exact: true }).fill('Enxergam longe e no escuro.');
      // The effect comes from the server's menu: "Proficiência" asks for what, and nothing else is sent.
      await m.getByLabel('Efeito', { exact: true }).selectOption({ label: 'Proficiência' });
      await m.getByLabel('Proficiência em').selectOption({ label: 'Percepção' });
      await m.getByRole('button', { name: 'Salvar raça' }).click();
      await expect(m.getByText('A raça Corujeiro foi salva.')).toBeVisible();

      const stored = (await listEntriesJSON(m, campaignId)).entries![0].tableRace;
      expect(stored.abilityBonuses).toMatchObject({ wisdom: 2, dexterity: 1 });
      expect(stored.speedFt).toBe(30);
      expect(stored.darkvisionFt).toBe(60);
      expect(stored.traits[0].effects).toEqual([{ type: 'proficiency', proficiency: 'skill:perception' }]);

      // The player sees the race in the content page, in full, and makes a character with it.
      await p.goto(`/campaigns/${campaignId}/content`);
      await p.getByRole('link', { name: /Corujeiro/ }).click();
      await expect(p.getByRole('heading', { level: 1, name: 'Corujeiro' })).toBeVisible();
      await expect(p.getByText('Olhos de caçador.')).toBeVisible();
      const characterId = await createCharacterViaUI(p, campaignId, { ...pensantus, race: 'Corujeiro', raceKey: 'race:corujeiro@mesa', subrace: undefined, subraceKey: undefined });
      const sheet = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
      expect(sheet.ok(), await sheet.text()).toBeTruthy();
      expect(JSON.stringify(await sheet.json())).toContain('race:corujeiro@mesa');
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'arquivar: o jogador deixa de ver a entrada e a ficha que a usa continua funcionando @RN-23',
  { tag: ['@MR-025', '@RN-23', '@RN-10'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo arquivo ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableRace', raceBody());
      const characterId = await createCharacterViaUI(p, campaignId, { ...pensantus, race: 'Corujeiro', raceKey: key, subrace: undefined, subraceKey: undefined });

      await m.goto(entryRoute(campaignId, key));
      await expect(m.getByRole('heading', { level: 1, name: 'Corujeiro' })).toBeVisible();
      await expect(m.getByText('Em uso por 1 ficha')).toBeVisible();
      await m.getByRole('button', { name: 'Arquivar', exact: true }).click();
      const question = m.getByRole('region', { name: 'Arquivar Corujeiro?' });
      await expect(question).toBeVisible();
      await expect(question.getByText('As fichas que usam Corujeiro continuam funcionando. A entrada só deixa de aparecer para fichas novas.')).toBeVisible();
      await expect(question.getByText('1 ficha usa Corujeiro agora.')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Salvar raça' })).toHaveAttribute('aria-disabled', 'true');
      await expect(m.getByText('Responda à pergunta de arquivar para voltar a salvar.')).toBeVisible();
      await question.getByRole('button', { name: 'Arquivar Corujeiro' }).click();
      await expect(m.getByText('A raça Corujeiro está arquivada.')).toBeVisible();
      await expect(m.getByText('Arquivada · 1 ficha usa')).toBeVisible();
      await expect(m.getByRole('button', { name: 'Desarquivar' })).toBeVisible();
      await expect(m.getByRole('button', { name: /Apagar/ })).toHaveCount(0);

      // The player no longer sees it, and the server never sent it: read the JSON.
      const list = await listEntriesJSON(p, campaignId);
      expect(list.entries ?? []).toEqual([]);
      await p.goto(`/campaigns/${campaignId}/content`);
      await expect(p.getByText('O mestre ainda não criou nada para esta mesa.')).toBeVisible();
      await expect(p.getByText('Corujeiro')).toHaveCount(0);
      const content = await callRPC(p, 'meurpg.rules.v1.ContentService/ListContent', { campaignId });
      expect(JSON.stringify(await content.json())).not.toContain('corujeiro');

      // The sheet that uses it keeps working.
      const sheet = await callRPC(p, 'meurpg.characters.v1.CharacterService/GetCharacter', { campaignId, characterId });
      expect(sheet.ok(), await sheet.text()).toBeTruthy();
      await p.goto(`/campaigns/${campaignId}/characters/${characterId}`);
      await expect(p.getByRole('heading', { level: 1, name: pensantus.name })).toBeVisible();

      // "Desarquivar" brings it back, with no question.
      await m.getByRole('button', { name: 'Desarquivar' }).click();
      await expect(m.getByText('A raça Corujeiro está arquivada.')).toHaveCount(0);
      expect((await listEntriesJSON(p, campaignId)).entries).toHaveLength(1);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o jogador nunca recebe uma entrada arquivada nem a contagem de fichas @RN-23',
  { tag: ['@MR-025', '@RN-23', '@RN-10'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo contagem ${Date.now()}`);
      const raceKey = await createEntryRPC(m, campaignId, 'tableRace', raceBody());
      const spellKey = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Rascunho de Tinta'));
      await createCharacterViaUI(p, campaignId, { ...pensantus, race: 'Corujeiro', raceKey, subrace: undefined, subraceKey: undefined });
      await archiveEntryRPC(m, campaignId, spellKey);

      // The master gets both, with the counts and the mark.
      const masters = (await listEntriesJSON(m, campaignId)).entries!;
      expect(masters.map((e) => e.namePt).sort()).toEqual(['Corujeiro', 'Rascunho de Tinta']);
      expect(masters.find((e) => e.namePt === 'Corujeiro')!.charactersUsing).toBe(1);
      expect(masters.find((e) => e.namePt === 'Rascunho de Tinta')!.archived).toBe(true);

      // The player gets the race that is on and nothing else: no archived entry, no count, no mark.
      const raw = await callRPC(p, 'meurpg.rules.v1.TableContentService/ListTableEntries', { campaignId });
      const text = await raw.text();
      const players = JSON.parse(text).entries as Record<string, any>[];
      expect(players.map((e) => e.namePt)).toEqual(['Corujeiro']);
      expect(text).not.toContain('Rascunho de Tinta');
      expect(text).not.toContain('charactersUsing');
      expect(text).not.toContain('archived');

      // A player cannot write.
      const denied = await callRPC(p, 'meurpg.rules.v1.TableContentService/ArchiveTableEntry', { campaignId, key: raceKey });
      expect(denied.status()).toBe(403);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o mestre cria um antecedente e o jogador o lê com os nomes, sem chaves @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo antecedente ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content/new/background`);
      await m.getByLabel('Nome', { exact: true }).fill('Cartógrafo do Vale');
      await m.getByLabel('Primeira perícia').selectOption({ label: 'Investigação' });
      await m.getByLabel('Segunda perícia').selectOption({ label: 'Sobrevivência' });
      await m.getByLabel('Adicionar ferramenta').selectOption({ label: 'Ferramentas de ladrão' });
      await m.getByLabel('Equipamento').fill('Um estojo de mapas, tinta e 10 PO');
      await m.getByLabel('Nome da característica').fill('Mapas na memória');
      await m.getByLabel('Texto', { exact: true }).fill('Você lembra o desenho de qualquer lugar que já mapeou.');
      // The preview is what a player reads: names, not keys.
      const preview = m.getByRole('complementary', { name: 'Como os jogadores veem' });
      await expect(preview.getByText('Investigação, Sobrevivência')).toBeVisible();
      await expect(preview.getByText('Ferramentas de ladrão')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar antecedente' }).click();
      await expect(m.getByText('O antecedente Cartógrafo do Vale foi salvo.')).toBeVisible();

      const stored = (await listEntriesJSON(m, campaignId)).entries![0].tableBackground;
      expect(stored.skills).toEqual(['skill:investigation', 'skill:survival']);
      expect(stored.tools).toEqual(['proficiency:thieves-tools']);
      // A note with no text of its own takes the trait's.
      expect(stored.feature.effects ?? []).toEqual([]);

      await p.goto(`/campaigns/${campaignId}/content`);
      await p.getByRole('link', { name: /Cartógrafo do Vale/ }).click();
      await expect(p.getByRole('heading', { level: 1, name: 'Cartógrafo do Vale' })).toBeVisible();
      await expect(p.getByText('Ferramentas de ladrão')).toBeVisible();
      await expect(p.getByText('Investigação, Sobrevivência')).toBeVisible();
      await expect(p.getByText(/proficiency:|skill:|language:/)).toHaveCount(0);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o mestre cria uma sub-raça de uma raça do SRD, escolhendo a raça na lista @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo sub-raça ${Date.now()}`);
      await createEntryRPC(m, campaignId, 'tableRace', raceBody());

      await m.goto(`/campaigns/${campaignId}/content?kind=races`);
      await m.getByRole('link', { name: 'Nova sub-raça' }).click();
      await expect(m.getByRole('heading', { level: 1, name: 'Nova sub-raça' })).toBeVisible();
      await m.getByLabel('Raça da sub-raça').selectOption({ label: 'Anão' });
      await m.getByLabel('Nome', { exact: true }).fill('Anão das Brumas');
      await m.getByRole('button', { name: 'Mais Constituição' }).click();
      await m.getByRole('button', { name: 'Salvar sub-raça' }).click();
      await expect(m.getByText('A sub-raça Anão das Brumas foi salva.')).toBeVisible();

      const stored = (await listEntriesJSON(m, campaignId)).entries!.find((e) => e.namePt === 'Anão das Brumas')!;
      expect(stored.tableSubrace.raceKey).toBe('race:dwarf');
      expect(stored.tableSubrace.abilityBonuses).toMatchObject({ constitution: 1 });
      await m.goto(`/campaigns/${campaignId}/content?kind=races`);
      await expect(m.locator('a.row').filter({ hasText: 'Anão das Brumas' })).toContainText('Sub-raça de Anão');
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'dois mestres na mesma magia: o segundo salvar diz que a entrada mudou e recarrega @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo conflito ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));

      await m.goto(entryRoute(campaignId, key));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Lâmina de Nanquim');
      // Someone else saves first (another tab, the other master).
      await updateEntryRPC(m, campaignId, key, 'tableSpell', spellBody('Lâmina de Nanquim', { descPt: ['Outro texto.'] }));
      await m.getByLabel('Descrição').fill('O meu texto.');
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      const alert = m.getByRole('alert').filter({ hasText: 'Esta entrada mudou enquanto você editava.' });
      await expect(alert).toBeVisible();
      await expect(m.getByLabel('Descrição')).toHaveValue('O meu texto.');
      await alert.getByRole('button', { name: 'Recarregar' }).click();
      await expect(m.getByLabel('Descrição')).toHaveValue('Outro texto.');
      await expect(m.getByRole('alert')).toHaveCount(0);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'depois de editar, o app lista as fichas que ficaram com aviso @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Conteúdo avisos ${Date.now()}`);
      const key = await createEntryRPC(m, campaignId, 'tableSpell', spellBody('Lâmina de Nanquim'));
      // Pensantus (Mago) knows the spell, which is on the wizard list.
      const body = characterRpcBody('PLAYER', pensantus) as { sheet: { full: { knownSpellKeys?: string[] } } };
      body.sheet.full.knownSpellKeys = [...(body.sheet.full.knownSpellKeys ?? []), key];
      const created = await createCharacterRPC(p, campaignId, body);
      expect(created.ok(), await created.text()).toBeTruthy();
      const characterId = (await created.json()).character.id as string;

      await m.goto(entryRoute(campaignId, key));
      await m.getByRole('checkbox', { name: 'Mago', exact: true }).uncheck({ force: true });
      await m.getByRole('checkbox', { name: 'Clérigo', exact: true }).check({ force: true });
      await m.getByRole('button', { name: 'Salvar magia' }).click();
      await expect(m.getByText('A magia Lâmina de Nanquim foi salva.')).toBeVisible();
      const warning = m.getByRole('status').filter({ hasText: 'ficha ficou com aviso' });
      await expect(warning).toBeVisible();
      await expect(warning.getByRole('link', { name: pensantus.name })).toHaveAttribute('href', `/campaigns/${campaignId}/characters/${characterId}`);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);
