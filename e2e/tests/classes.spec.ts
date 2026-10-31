import { expect } from '@playwright/test';
import { test } from './diagnostics';

import { classBody, createClassRPC, halfCasterBody, listEntries } from './classes-support';
import { archiveEntryRPC, entryRoute } from './content-support';
import { pickRadio } from './move-support';
import { listSpellsRPC } from './spells-support';
import { campaignWithEmptyPlayer } from './table-rules-support';
import { characterRpcBody, createCharacterRPC, newSignedInContext, pensantus } from './support';

test.describe.configure({ timeout: 120_000 });

// MR-025 (RN-23): the class and subclass editors. Every test makes its own campaign through the API.

test(
  'o mestre cria o Guardião do Vale (meio conjurador) a partir do padrão, com uma característica @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Classes criar ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content?kind=classes`);
      await m.getByRole('link', { name: 'Nova classe' }).click();
      await expect(m.getByRole('heading', { level: 1, name: 'Nova classe' })).toBeVisible();

      await m.getByLabel('Nome', { exact: true }).fill('Guardião do Vale');
      await m.getByLabel('Dado de vida').selectOption({ label: 'd10' });
      await m.getByLabel('Teste de resistência 1').selectOption({ label: 'Força' });
      await m.getByLabel('Teste de resistência 2').selectOption({ label: 'Sabedoria' });
      for (const skill of ['Atletismo', 'Intuição', 'Natureza', 'Percepção', 'Sobrevivência']) {
        await m.getByRole('button', { name: skill, exact: true }).click();
      }
      await expect(m.getByText('Perícias que ele pode escolher (5 de 18)')).toBeVisible();
      await m.locator('app-check-row', { hasText: 'Armaduras leves' }).click();
      await m.locator('app-check-row', { hasText: 'Armas simples' }).click();
      await expect(m.getByRole('checkbox', { name: 'Armas simples' })).toBeChecked();

      // "Metade" pastes the server's table: nothing at level 1, two 1st-circle slots at level 2.
      await pickRadio(m, 'Metade');
      await m.getByLabel('Habilidade de conjuração').selectOption({ label: 'Sabedoria' });
      await m.getByLabel('Lista de magias').selectOption({ label: 'A lista do Druida' });
      await expect(m.getByLabel('Nível 2, espaços de magia de 1º nível')).toHaveValue('2');
      await expect(m.getByLabel('Nível 1, espaços de magia de 1º nível')).toHaveValue('');
      await expect(m.getByLabel('Nível 5, bônus de proficiência')).toHaveValue('+3');

      // A feature with an effect from the menu.
      await m.getByRole('button', { name: 'Adicionar característica', exact: true }).click();
      await m.getByLabel('Nome da característica').fill('Vigília');
      await m.getByLabel('Efeito', { exact: true }).selectOption({ label: 'Recurso' });
      await m.getByLabel('Usos (máximo)').fill('3');
      await m.getByLabel('Volta em').selectOption({ label: 'Descanso longo' });
      await m.getByLabel('Nome do recurso').fill('vigilia');
      await m.getByRole('button', { name: 'Salvar classe' }).click();
      await expect(m).toHaveURL(/\/content\/entries\//);
      await expect(m.getByText('A classe Guardião do Vale foi salva.')).toBeVisible();

      const stored = (await listEntries(m, campaignId)).find((e) => e.namePt === 'Guardião do Vale')!;
      expect(stored.tableClass.hitDie).toBe(10);
      expect(stored.tableClass.casting).toMatchObject({ kind: 'half', preparation: 'prepared', listFrom: 'class:druid' });
      expect(stored.tableClass.levels).toHaveLength(20);
      expect(stored.tableClass.levels[1].slots[0]).toBe(2);
      expect(stored.tableClass.levels[0].features[0]).toMatchObject({ namePt: 'Vigília' });
      expect(stored.tableClass.levels[0].features[0].effects[0]).toMatchObject({ type: 'resource', max: '3', recharge: 'long_rest', resource: 'vigilia' });
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test('uma célula recusada da tabela volta no campo dela @MR-025', { tag: ['@MR-025', '@RN-23'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Classes recusa ${Date.now()}`);
    const key = await createClassRPC(m, campaignId, halfCasterBody());

    await m.goto(entryRoute(campaignId, key));
    await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Guardião do Vale');
    // A half caster has no cantrips at level 1: the server refuses the cell, not the page.
    const cell = m.getByLabel('Nível 1, truques');
    await cell.fill('5');
    await m.getByRole('button', { name: 'Salvar classe' }).click();
    await expect(cell).toHaveAttribute('aria-invalid', 'true');
    await expect(cell).toBeFocused();
    await expect(m.getByText('Truques: de 0 a 30, e 0 antes de a conjuração começar.')).toBeVisible();
    await expect(m.getByRole('alert').filter({ hasText: 'Não foi possível salvar a classe.' })).toContainText('1 campo precisa de ajuste');
    await expect(cell).toHaveValue('5');
    await expect(m.getByRole('button', { name: /Tabela dos 20 níveis/ })).toContainText('Com erro');

    // Putting it right saves.
    await cell.fill('');
    await m.getByRole('button', { name: 'Salvar classe' }).click();
    await expect(m.getByText('A classe Guardião do Vale foi salva.')).toBeVisible();
  } finally {
    await Promise.all([master.close(), player.close()]);
  }
});

test(
  'o mestre cria a Tradição da Tinta, subclasse de um terço de uma classe do SRD, com magias sempre preparadas @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Subclasses criar ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content?kind=subclasses`);
      await m.getByRole('link', { name: 'Nova subclasse' }).click();
      await expect(m.getByRole('heading', { level: 1, name: 'Nova subclasse' })).toBeVisible();
      await m.getByLabel('Nome', { exact: true }).fill('Tradição da Tinta');
      await m.getByLabel('É subclasse de').selectOption({ label: 'Guerreiro' });

      await m.getByRole('switch', { name: 'Esta subclasse conjura' }).click();
      await m.getByLabel('Habilidade de conjuração').selectOption({ label: 'Inteligência' });
      await m.getByLabel('Lista de magias').selectOption({ label: 'A lista do Mago' });
      // The rows of the third caster start at level 3, from the server's table.
      await expect(m.getByLabel('Nível 3, espaços de magia de 1º nível')).toHaveValue('2');
      await expect(m.getByLabel('Nível 2, espaços de magia de 1º nível')).toHaveCount(0);

      await m.getByLabel('Adicionar nível').selectOption({ label: 'Nível 3' });
      await m.getByRole('group', { name: 'Nível 3' }).getByLabel('Adicionar magia').selectOption({ label: 'Detectar Magia' });
      await m.getByRole('button', { name: 'Salvar subclasse' }).click();
      await expect(m).toHaveURL(/\/content\/entries\//);
      await expect(m.getByText('A subclasse Tradição da Tinta foi salva.')).toBeVisible();

      const stored = (await listEntries(m, campaignId)).find((e) => e.namePt === 'Tradição da Tinta')!;
      expect(stored.tableSubclass.classKey).toBe('class:fighter');
      expect(stored.tableSubclass.casting).toMatchObject({ kind: 'third', preparation: 'known', listFrom: 'class:wizard', startLevel: 3 });
      expect(stored.tableSubclass.levels[0]).toMatchObject({ level: 3 });
      expect(stored.tableSubclass.alwaysPrepared).toEqual([{ classLevel: 3, spellKey: 'spell:detect-magic' }]);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test('mudar a classe lista a ficha que ficou com aviso @MR-025', { tag: ['@MR-025', '@RN-23'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Classes avisos ${Date.now()}`);
    const key = await createClassRPC(m, campaignId, classBody());
    // A sheet of the class with the two skills it asks for.
    const body = characterRpcBody('PLAYER', {
      ...pensantus,
      classKey: key,
      subclassKey: undefined,
      level: 1,
      extraSkillKeys: ['skill:athletics', 'skill:insight'],
    }) as Record<string, any>;
    const created = await createCharacterRPC(p, campaignId, body);
    expect(created.ok(), await created.text()).toBeTruthy();
    const characterId = (await created.json()).character.id as string;

    await m.goto(entryRoute(campaignId, key));
    await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Guardião do Vale');
    await m.getByRole('button', { name: 'Menos Quantas perícias o jogador escolhe' }).click();
    await m.getByRole('button', { name: 'Salvar classe' }).click();
    await expect(m.getByText('A classe Guardião do Vale foi salva.')).toBeVisible();
    const warning = m.getByRole('status').filter({ hasText: 'ficha ficou com aviso' });
    await expect(warning).toBeVisible();
    await expect(warning.getByRole('link', { name: pensantus.name })).toHaveAttribute('href', `/campaigns/${campaignId}/characters/${characterId}`);
  } finally {
    await Promise.all([master.close(), player.close()]);
  }
});

test('o jogador lê a classe por inteiro, sem contagem e sem uma arquivada @MR-025', { tag: ['@MR-025', '@RN-23'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Classes leitura ${Date.now()}`);
    const key = await createClassRPC(m, campaignId, halfCasterBody());
    const old = await createClassRPC(m, campaignId, classBody('Bardo das Cinzas'));
    await archiveEntryRPC(m, campaignId, old);

    await p.goto(`/campaigns/${campaignId}/content?kind=classes`);
    await expect(p.locator('a.row')).toHaveCount(1);
    await expect(p.locator('a.row')).toContainText('Guardião do Vale');
    await expect(p.getByText('Bardo das Cinzas')).toHaveCount(0);
    await expect(p.getByText(/Em uso por|fichas? usa/)).toHaveCount(0);
    await p.locator('a.row').click();
    await expect(p.getByRole('heading', { level: 1, name: 'Guardião do Vale' })).toBeVisible();
    // The numbers, in full: the two saving throws, the skills, the table.
    await expect(p.getByText('Testes de resistência', { exact: true })).toBeVisible();
    await expect(p.getByText('Força, Sabedoria')).toBeVisible();
    await expect(p.getByRole('heading', { name: 'Tabela dos níveis' })).toBeVisible();
    await expect(p.getByText('2 de 1º').first()).toBeVisible();
    // Nothing of the master's: no editor, no archive, no count.
    await expect(p.getByRole('button', { name: /Salvar classe|Arquivar/ })).toHaveCount(0);
    await expect(p.getByText(/Em uso por/)).toHaveCount(0);
    expect(key).toContain('class:');
  } finally {
    await Promise.all([master.close(), player.close()]);
  }
});

test('mudar a conjuração de uma tabela editada pergunta no lugar, e "Restaurar o padrão" também @MR-025', { tag: ['@MR-025', '@RN-23'] }, async ({ browser }) => {
  const master = await newSignedInContext(browser, 'Mestre Teste');
  const player = await newSignedInContext(browser, 'Jogador Teste');
  try {
    const m = await master.newPage();
    const p = await player.newPage();
    await Promise.all([m.goto('/'), p.goto('/')]);
    const campaignId = await campaignWithEmptyPlayer(m, p, `Classes pergunta ${Date.now()}`);
    const key = await createClassRPC(m, campaignId, halfCasterBody());

    await m.goto(entryRoute(campaignId, key));
    await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Guardião do Vale');
    // An edited table: another way of casting asks first, right under the control, with the focus on its title.
    await m.getByLabel('Nível 5, espaços de magia de 1º nível').fill('9');
    await pickRadio(m, 'Completa');
    const ask = m.getByRole('alertdialog', { name: 'Refazer a tabela dos 20 níveis?' });
    await expect(ask).toBeVisible();
    await expect(ask.getByText('Refazer a tabela dos 20 níveis?')).toBeFocused();
    await ask.getByRole('button', { name: 'Manter a minha tabela' }).click();
    await expect(m.getByLabel('Nível 5, espaços de magia de 1º nível')).toHaveValue('9');
    await expect(m.getByRole('alertdialog')).toHaveCount(0);

    await m.getByRole('button', { name: 'Restaurar o padrão' }).click();
    const restore = m.getByRole('alertdialog', { name: 'Restaurar o padrão?' });
    await expect(restore).toBeVisible();
    await restore.getByRole('button', { name: 'Restaurar o padrão' }).click();
    await expect(m.getByRole('alertdialog')).toHaveCount(0);
    // The full caster's table of the server: 4 first-circle slots at level 5.
    await expect(m.getByLabel('Nível 5, espaços de magia de 1º nível')).toHaveValue('4');
  } finally {
    await Promise.all([master.close(), player.close()]);
  }
});

test(
  'o mestre dá magias do SRD à lista própria da classe, e a página Magias as lista para ela @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Classes lista SRD ${Date.now()}`);
      // A half caster with a list of its own (no list_from) and a list that reuses the druid's.
      const own = halfCasterBody('Aprendiz');
      (own.casting as Record<string, unknown>).listFrom = '';
      (own.casting as Record<string, unknown>).ability = 'ABILITY_INTELLIGENCE';
      const key = await createClassRPC(m, campaignId, own);

      await m.goto(entryRoute(campaignId, key));
      await expect(m.getByLabel('Nome', { exact: true })).toHaveValue('Aprendiz');
      await expect(m.getByText('Magias do SRD na lista da classe').first()).toBeVisible();
      await expect(m.getByText('Elas se juntam às magias da mesa que citam esta classe.')).toBeVisible();
      const add = m.getByLabel('Adicionar magia do SRD');
      await add.selectOption({ label: 'Curar Ferimentos (1º nível, Evocação)' });
      await add.selectOption({ label: 'Ajuda (2º nível, Abjuração)' });
      await expect(m.getByRole('button', { name: 'Tirar Ajuda (2º nível, Abjuração)' })).toBeVisible();

      // Reusing the list of another class turns the picker off and says why.
      await m.getByLabel('Lista de magias').selectOption({ label: 'A lista do Druida' });
      await expect(add).toBeDisabled();
      await expect(m.getByText('Esta classe usa a lista de outra classe.')).toBeVisible();
      await m.getByLabel('Lista de magias').selectOption({ label: 'A própria lista da classe' });
      await expect(add).toBeEnabled();

      await m.getByRole('button', { name: 'Salvar classe' }).click();
      await expect(m.getByText('A classe Aprendiz foi salva.')).toBeVisible();
      const stored = (await listEntries(m, campaignId)).find((e) => e.namePt === 'Aprendiz')!;
      expect(stored.tableClass.casting.srdSpellKeys).toEqual(['spell:cure-wounds', 'spell:aid']);

      // The "Magias" page lists them for the class, and only them.
      const keys = (await listSpellsRPC(p, campaignId, { classKey: key })).spells.map((s) => s.key);
      expect(keys.sort()).toEqual(['spell:aid', 'spell:cure-wounds']);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o mestre marca as magias de uma subclasse como lista expandida, para escolher @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Subclasses expandida ${Date.now()}`);

      await m.goto(`/campaigns/${campaignId}/content?kind=subclasses`);
      await m.getByRole('link', { name: 'Nova subclasse' }).click();
      await m.getByLabel('Nome', { exact: true }).fill('Pacto da Tinta');
      await m.getByLabel('É subclasse de').selectOption({ label: 'Clérigo' });
      // The switch is for a subclass that names spells.
      const expanded = m.getByRole('switch', { name: 'As magias da subclasse entram na lista da classe, para escolher' });
      await expect(expanded).toHaveCount(0);
      await m.getByLabel('Adicionar nível').selectOption({ label: 'Nível 3' });
      await m.getByRole('group', { name: 'Nível 3' }).getByLabel('Adicionar magia').selectOption({ label: 'Mísseis Mágicos' });
      await expect(expanded).toHaveAttribute('aria-checked', 'false');
      await expanded.click();
      await expect(expanded).toHaveAttribute('aria-checked', 'true');
      await expect(m.getByText('A pessoa escolhe estas magias como as outras da lista, e elas contam no limite.')).toBeVisible();
      await m.getByRole('button', { name: 'Salvar subclasse' }).click();
      await expect(m.getByText('A subclasse Pacto da Tinta foi salva.')).toBeVisible();

      const stored = (await listEntries(m, campaignId)).find((e) => e.namePt === 'Pacto da Tinta')!;
      expect(stored.tableSubclass.spellsExpanded).toBe(true);
      expect(stored.tableSubclass.alwaysPrepared).toEqual([{ classLevel: 3, spellKey: 'spell:magic-missile' }]);
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);

test(
  'o mestre liga "Só onde já tem proficiência" na especialização de uma perícia @MR-025',
  { tag: ['@MR-025', '@RN-23'] },
  async ({ browser }) => {
    const master = await newSignedInContext(browser, 'Mestre Teste');
    const player = await newSignedInContext(browser, 'Jogador Teste');
    try {
      const m = await master.newPage();
      const p = await player.newPage();
      await Promise.all([m.goto('/'), p.goto('/')]);
      const campaignId = await campaignWithEmptyPlayer(m, p, `Classes especializacao ${Date.now()}`);
      const key = await createClassRPC(m, campaignId, classBody());

      await m.goto(entryRoute(campaignId, key));
      await m.getByRole('button', { name: 'Adicionar característica', exact: true }).click();
      await m.getByLabel('Nome da característica').fill('Estudo dobrado');
      await m.getByLabel('Efeito', { exact: true }).selectOption({ label: 'Proficiência' });
      await m.getByLabel('Proficiência em').selectOption({ value: 'skill:perception' });
      // The switch means something only with the level "Especialização" on a skill.
      const only = m.getByRole('checkbox', { name: 'Só onde já tem proficiência' });
      await expect(only).toHaveCount(0);
      await m.getByRole('button', { name: 'Mais opções', exact: true }).last().click();
      await m.getByLabel('Quanto').selectOption({ value: 'expertise' });
      await expect(only).not.toBeChecked();
      // The real checkbox sits under the row's drawn box: a click on its words ticks it.
      await m.getByText('Só onde já tem proficiência', { exact: true }).click();
      await expect(only).toBeChecked();
      await m.getByRole('button', { name: 'Salvar classe' }).click();
      await expect(m.getByText('A classe Guardião do Vale foi salva.')).toBeVisible();

      const stored = (await listEntries(m, campaignId)).find((e) => e.namePt === 'Guardião do Vale')!;
      const features = stored.tableClass.levels[0].features as { namePt: string; effects: unknown[] }[];
      const effect = features.find((f) => f.namePt === 'Estudo dobrado')!.effects[0];
      expect(effect).toMatchObject({ type: 'proficiency', proficiency: 'skill:perception', level: 'expertise', onlyProficient: true });
    } finally {
      await Promise.all([master.close(), player.close()]);
    }
  },
);
