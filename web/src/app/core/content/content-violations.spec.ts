import { create } from '@bufbuild/protobuf';

import { TableContentViolationSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import {
  inputFor,
  parentOf,
  pathAfterRowRemoved,
  placeViolations,
  refusalSummary,
  shapeOf,
  violationText,
} from './content-violations';

const v = (field: string, reason: string, key = '') =>
  create(TableContentViolationSchema, { field, reason, key });
const ctx = {
  aOne: 'uma magia',
  nameOf: (k: string) => (k === 'race:corujeiro@mesa' ? 'Corujeiro' : ''),
};

describe('the refusals of the server, back on the fields (E10-01 state 5)', () => {
  it('reads the path of a violation: its shape and its parent', () => {
    expect(shapeOf('table_race.traits[1].effects[0].value')).toBe(
      'table_race.traits[].effects[].value',
    );
    expect(parentOf('table_race.traits[1].effects[0].value')).toBe(
      'table_race.traits[1].effects[0]',
    );
    expect(parentOf('table_race.traits[1].effects[0]')).toBe('table_race.traits[1].effects');
    expect(parentOf('table_race.traits[1]')).toBe('table_race.traits');
    expect(parentOf('table_race')).toBe('');
  });

  it('finds the nearest input at or above a path', () => {
    const known = new Set(['table_race.traits[1].effects[0]', 'table_race.traits[1].name_pt']);
    expect(inputFor('table_race.traits[1].effects[0].value', (p) => known.has(p))).toBe(
      'table_race.traits[1].effects[0]',
    );
    expect(inputFor('table_race.traits[1].name_pt', (p) => known.has(p))).toBe(
      'table_race.traits[1].name_pt',
    );
    expect(inputFor('table_race.size', (p) => known.has(p))).toBe('');
  });

  // Copy of the Go table `entryViolationRows` in backend/internal/rules/overlay_paths_test.go (TestEntryViolationPaths): the
  // path and the reason the server sends for each field of a spell, a race, a sub-race and a background. Change a row there
  // and here together. `text` is what the app says.
  const SERVER_ROWS: readonly (readonly [string, string, string])[] = [
    ['table_spell.level', 'bad_value', 'Escolha um nível de magia de 0 (truque) a 9.'],
    ['table_spell.school_key', 'dangling_reference', 'Escolha a escola da magia.'],
    [
      'table_spell.range.kind',
      'bad_value',
      'Esse alcance não combina com o alvo. “Só quem conjura” pede Pessoal; criaturas escolhidas pedem distância ou Toque.',
    ],
    [
      'table_spell.range.distance_ft',
      'limit',
      'O alcance vai de 1,5 m a 1.584 m, em passos de 1,5 m.',
    ],
    [
      'table_spell.target.kind',
      'bad_value',
      'Este alvo não aceita os outros campos preenchidos. Confira o alvo.',
    ],
    [
      'table_spell.target.shape',
      'bad_value',
      'Escolha a forma da área: cone, cubo, cilindro, linha ou esfera.',
    ],
    ['table_spell.target.size_ft', 'limit', 'A área vai de 1,5 m a 90 m, em passos de 1,5 m.'],
    [
      'table_spell.target.count',
      'limit',
      'Escreva um número de criaturas: 2 ou mais, e até 10 a mais por nível de espaço.',
    ],
    [
      'table_spell.target.per_slot_level',
      'limit',
      'Escreva um número de criaturas: 2 ou mais, e até 10 a mais por nível de espaço.',
    ],
    ['table_spell.casting_time.unit', 'bad_value', 'Escolha o tempo de conjuração.'],
    [
      'table_spell.casting_time.amount',
      'limit',
      'O tempo vai de 1 a 60 minutos ou horas; uma ação vale 1.',
    ],
    ['table_spell.casting_time.trigger_pt', 'bad_value', 'Só uma reação tem gatilho.'],
    ['table_spell.duration.kind', 'bad_value', 'Escolha a duração.'],
    ['table_spell.duration.amount', 'limit', 'A duração vai de 1 a 999.'],
    ['table_spell.duration.unit', 'bad_value', 'Escolha a unidade da duração.'],
    ['table_spell.concentration', 'bad_value', 'Concentração pede uma duração por tempo.'],
    ['table_spell.ritual', 'bad_value', 'Um truque não é um ritual.'],
    [
      'table_spell.components.material_pt',
      'bad_value',
      'O material vai com o componente M, e só com ele.',
    ],
    [
      'table_spell.attack',
      'bad_value',
      'Uma magia de área não tem ataque. Escolha um teste de resistência, ou outro alvo.',
    ],
    ['table_spell.save', 'bad_value', 'Uma magia tem ataque ou teste de resistência, não os dois.'],
    ['table_spell.save.ability', 'bad_value', 'Escolha a habilidade do teste de resistência.'],
    [
      'table_spell.save.on_success',
      'bad_value',
      'Escolha o que acontece ao passar: metade do dano (que pede dano) ou nada.',
    ],
    ['table_spell.damage', 'limit', 'Uma magia tem até 4 tipos de dano.'],
    ['table_spell.damage[0].dice', 'bad_value', 'Escreva o dado assim: 2d8 (de 1 a 20 dados).'],
    ['table_spell.damage[1].dice', 'bad_value', 'Escreva o dado assim: 2d8 (de 1 a 20 dados).'],
    [
      'table_spell.damage[0].per_slot_level',
      'bad_value',
      'Escreva o dado assim: 2d8 (de 1 a 20 dados).',
    ],
    ['table_spell.damage[0].per_tier', 'bad_value', 'Escreva o dado assim: 2d8 (de 1 a 20 dados).'],
    [
      'table_spell.damage[0].damage_type_key',
      'dangling_reference',
      'Escolha um tipo de dano da lista.',
    ],
    ['table_spell.heal.dice', 'bad_value', 'Escreva o dado assim: 2d8 (de 1 a 20 dados).'],
    [
      'table_spell.heal.per_slot_level',
      'bad_value',
      'Escreva o dado assim: 2d8 (de 1 a 20 dados).',
    ],
    ['table_spell.heal', 'bad_value', 'Um truque não cura.'],
    ['table_spell.desc_pt[1]', 'bad_text', 'Este texto é longo demais. Encurte o parágrafo.'],
    [
      'table_spell.class_keys[0]',
      'dangling_reference',
      'Esta classe não existe mais. Tire-a da lista.',
    ],
    ['table_spell.class_keys[1]', 'bad_value', 'Esta classe está na lista duas vezes.'],
    ['table_race.size', 'bad_value', 'Escolha o tamanho.'],
    ['table_race.speed_ft', 'limit', 'O deslocamento vai de 1,5 m a 36 m, em passos de 1,5 m.'],
    [
      'table_race.darkvision_ft',
      'limit',
      'A visão no escuro é 0 ou vai de 1,5 m a 36 m, em passos de 1,5 m.',
    ],
    ['table_race.ability_bonuses.wisdom', 'limit', 'O bônus vai de −4 a +4.'],
    ['table_race.choice_bonuses', 'limit', 'No máximo 6 bônus à escolha.'],
    ['table_race.choice_bonuses[1]', 'limit', 'Cada bônus à escolha vai de +1 a +4.'],
    ['table_race.language_choices', 'limit', 'Idiomas à escolha: de 0 a 4.'],
    ['table_race.languages[1]', 'dangling_reference', 'Este idioma não existe. Escolha da lista.'],
    ['table_race.traits[0].effects[0].roll', 'bad_value', 'Este valor não serve para este efeito.'],
    [
      'table_subrace.race_key',
      'dangling_reference',
      'A raça desta sub-raça não existe mais. Crie a sub-raça numa raça que exista.',
    ],
    ['table_subrace.ability_bonuses.dexterity', 'limit', 'O bônus vai de −4 a +4.'],
    ['table_background.skills', 'bad_value', 'Escolha duas perícias diferentes.'],
    [
      'table_background.skills[1]',
      'dangling_reference',
      'Esta perícia não existe. Escolha da lista.',
    ],
    ['table_background.tools', 'limit', 'No máximo 4 ferramentas.'],
    [
      'table_background.tools[0]',
      'dangling_reference',
      'Esta não é uma ferramenta do SRD. Escolha da lista.',
    ],
    ['table_background.language_choices', 'limit', 'Idiomas à escolha: de 0 a 4.'],
    ['table_background.equipment_pt', 'bad_text', 'O equipamento é longo demais. Encurte o texto.'],
    [
      'table_background.feature.effects[0].type',
      'forbidden_effect',
      'Este efeito não está no menu.',
    ],
    [
      'table_spell.name_pt',
      'duplicate_name',
      'Já existe uma magia da mesa com este nome. Escolha outro.',
    ],
    ['table_spell.name_pt', 'bad_name', 'O nome tem de 1 a 60 letras.'],
    [
      'table_race.traits[1].effects[0].value',
      'bad_formula',
      'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
    ],
    [
      'table_race.traits[0].effects[0].resource',
      'bad_value',
      'Use só letras minúsculas sem acento, números e _ (de 1 a 40), como vigilia.',
    ],
    [
      'table_spell.level',
      'immutable',
      'Um truque não vira magia de 1º nível ou mais, nem o contrário. Crie outra.',
    ],
    ['', 'limit', 'A mesa já tem 1000 entradas. Arquive ou use o que já existe.'],
    ['', 'size_limit', 'Esta entrada passou de 64 KiB de dados. Tire um pouco de texto.'],
  ];

  // The class and the subclass (slice 10.12): the paths and reasons of backend/internal/rules/overlay_class.go (`bad(...)` and
  // `asiLevels`, `minimums`). Change a row there and here together.
  const CLASS_ROWS: readonly (readonly [string, string, string])[] = [
    ['table_class.hit_die', 'bad_value', 'O dado de vida é d6, d8, d10 ou d12.'],
    ['table_class.saving_throws', 'bad_value', 'Escolha dois testes de resistência diferentes.'],
    ['table_class.saving_throws[1]', 'bad_value', 'Escolha dois testes de resistência diferentes.'],
    [
      'table_class.skill_choose',
      'bad_value',
      'Quantas perícias a classe dá: de 0 até o número de perícias da lista.',
    ],
    [
      'table_class.skill_from[2]',
      'dangling_reference',
      'Esta perícia não existe. Escolha da lista.',
    ],
    [
      'table_class.proficiencies[0]',
      'dangling_reference',
      'Escolha uma armadura, arma ou ferramenta da lista.',
    ],
    [
      'table_class.multiclass_proficiencies[1]',
      'dangling_reference',
      'Escolha uma armadura, arma ou ferramenta da lista.',
    ],
    [
      'table_class.multiclass_skill_choose',
      'bad_value',
      'As perícias de quem vem de outra classe saem da lista da classe: de 0 até o número dela.',
    ],
    ['table_class.minimums.wisdom', 'bad_value', 'O valor mínimo vai de 1 a 30.'],
    ['table_class.any_of.strength', 'bad_value', 'O valor mínimo vai de 1 a 30.'],
    [
      'table_class.subclass_level',
      'bad_value',
      'Esta classe não tem nível de subclasse entre 1 e 20.',
    ],
    [
      'table_class.asi_levels[1]',
      'bad_value',
      'Os níveis de incremento no valor de habilidade são diferentes, de 1 a 20.',
    ],
    ['table_class.levels', 'bad_table', 'A tabela dos níveis precisa ter os 20 níveis.'],
    [
      'table_subclass.levels',
      'bad_table',
      'Uma subclasse que conjura precisa de uma linha em cada nível, do começo da conjuração ao 20.',
    ],
    [
      'table_class.minimums',
      'bad_value',
      'O pré-requisito de multiclasse precisa de uma habilidade.',
    ],
    [
      'table_class.levels[4].prof_bonus',
      'bad_value',
      'O bônus de proficiência vai de +1 a +12 (vazio é o do SRD).',
    ],
    [
      'table_class.levels[4].cantrips_known',
      'bad_table',
      'Truques: de 0 a 30, e 0 antes de a conjuração começar.',
    ],
    [
      'table_class.levels[4].spells_known',
      'bad_table',
      'Magias conhecidas: de 0 a 200, e 0 antes de a conjuração começar.',
    ],
    [
      'table_class.levels[4].slots[2]',
      'bad_table',
      'Espaços de magia: de 0 a 9 por nível de magia, e 0 antes de a conjuração começar. Pacto: espaços de um nível só.',
    ],
    [
      'table_class.levels[4].slots',
      'bad_table',
      'Quem conjura precisa de espaços de magia neste nível.',
    ],
    [
      'table_class.levels[19].features[0]',
      'limit',
      'Uma classe da mesa não pode ter mais de 60 características.',
    ],
    [
      'table_class.levels[0].features[0].effects[0].value',
      'bad_formula',
      'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
    ],
    ['table_class.casting.kind', 'bad_casting', 'Escolha como conjura.'],
    ['table_class.casting.ability', 'bad_casting', 'Escolha a habilidade de conjuração.'],
    [
      'table_class.casting.preparation',
      'bad_casting',
      'Escolha se as magias são preparadas ou conhecidas.',
    ],
    ['table_class.casting.prepared_max', 'bad_casting', 'Só quem prepara magias tem este número.'],
    [
      'table_class.casting.list_from',
      'dangling_reference',
      'Esta classe não existe mais. Escolha outra lista.',
    ],
    [
      'table_class.casting.list_from',
      'bad_casting',
      'Escolha uma classe que tenha lista de magias própria (uma subclasse que conjura precisa de uma).',
    ],
    [
      'table_class.casting.start_level',
      'bad_casting',
      'A conjuração começa num nível de 1 a 20, e uma subclasse não conjura antes de ser escolhida.',
    ],
    [
      'table_class.casting.srd_spell_keys[1]',
      'dangling_reference',
      'Esta magia não está no SRD. Tire-a da lista.',
    ],
    ['table_class.casting.srd_spell_keys[1]', 'bad_value', 'Esta magia está na lista duas vezes.'],
    [
      'table_class.casting.srd_spell_keys',
      'bad_casting',
      'Só uma classe com lista própria tem magias do SRD. Escolha "A própria lista da classe".',
    ],
    ['table_class.casting.srd_spell_keys', 'limit', 'A lista passou de 400 magias do SRD.'],
    [
      'table_subclass.spells_expanded',
      'bad_value',
      'Esta marca precisa de magias na subclasse. Adicione magias ou desligue.',
    ],
    [
      'table_class.levels[0].features[0].effects[0].only_proficient',
      'bad_value',
      'Só vale para a especialização numa perícia.',
    ],
    ['table_subclass.class_key', 'dangling_reference', 'A classe desta subclasse não existe mais.'],
    ['table_subclass.level', 'bad_value', 'A subclasse é escolhida no nível da classe.'],
    [
      'table_subclass.casting',
      'bad_casting',
      'Uma subclasse só conjura se a classe dela não conjura. Uma classe sem conjuração não tem lista de magias.',
    ],
    [
      'table_subclass.levels[3].level',
      'bad_table',
      'Os níveis da subclasse vão de 1 a 20, em ordem.',
    ],
    [
      'table_subclass.levels[3].features',
      'bad_table',
      'Uma subclasse só ganha características a partir do nível em que se escolhe.',
    ],
    [
      'table_subclass.levels[3].slots[0]',
      'bad_table',
      'Espaços de magia: de 0 a 9 por nível de magia, e 0 antes de a conjuração começar. Pacto: espaços de um nível só.',
    ],
    [
      'table_subclass.levels[3].features[1]',
      'limit',
      'Uma subclasse da mesa não pode ter mais de 60 características.',
    ],
    [
      'table_subclass.always_prepared',
      'bad_casting',
      'Magias sempre preparadas pedem uma classe ou subclasse que conjura.',
    ],
    [
      'table_subclass.always_prepared[0].class_level',
      'bad_value',
      'O nível da classe vai de 1 a 20.',
    ],
    [
      'table_subclass.always_prepared[0].spell_key',
      'dangling_reference',
      'Esta magia não existe mais. Escolha outra.',
    ],
    [
      'table_subclass.always_prepared[0].spell_key',
      'bad_value',
      'Uma magia sempre preparada é de 1º nível ou mais, nunca um truque.',
    ],
    ['table_class.any_of.dexterity', 'bad_value', 'O valor mínimo vai de 1 a 30.'],
    ['table_class.levels[3].features[1].name_pt', 'bad_name', 'O nome tem de 1 a 60 letras.'],
    ['table_class.class_key', 'immutable', 'Isto não muda depois de criado.'],
    [
      'table_class.name_pt',
      'duplicate_name',
      'Já existe uma classe da mesa com este nome. Escolha outro.',
    ],
  ];

  it.each(CLASS_ROWS)('writes the class path %s (%s) in Portuguese', (field, reason, text) => {
    expect(violationText({ field, reason }, { aOne: 'uma classe' })).toBe(text);
  });

  it("counts the features of a class in the limit sentence, with the menu's limit", () => {
    expect(
      violationText(
        { field: 'table_class.levels[19].features[0]', reason: 'limit' },
        { aOne: 'uma classe', maxFeatures: 60, featureCount: 61 },
      ),
    ).toBe('Esta classe tem 61. Uma classe da mesa não pode ter mais de 60 características.');
    expect(
      violationText(
        { field: 'table_subclass.levels[1].features[3]', reason: 'limit' },
        { aOne: 'uma subclasse', maxFeatures: 60, featureCount: 61 },
      ),
    ).toBe('Esta subclasse tem 61. Uma subclasse da mesa não pode ter mais de 60 características.');
  });

  it.each(SERVER_ROWS)(
    'writes %s (%s) in Portuguese from the code and the path, never the message',
    (field, reason, text) => {
      expect(violationText({ field, reason }, ctx)).toBe(text);
    },
  );

  it("never gives a spell's saving throw a class's field, and never says the generic line for a path of the table", () => {
    for (const [field, reason] of SERVER_ROWS.filter(([f]) => f !== '')) {
      expect(field).not.toContain('saving_throws');
      expect(violationText({ field, reason }, ctx)).not.toBe(
        'Este valor não é aceito. Confira o campo.',
      );
    }
  });

  it('puts each violation on its input, in the order the server listed them, and counts them', () => {
    const known = new Set([
      'table_spell.name_pt',
      'table_spell.range.distance_ft',
      'table_spell.damage[0].dice',
    ]);
    const placed = placeViolations(
      [
        v('table_spell.name_pt', 'duplicate_name'),
        v('table_spell.range.distance_ft', 'limit'),
        v('table_spell.damage[0].dice', 'bad_value'),
      ],
      (p) => known.has(p),
      ctx,
    );
    expect(placed.fields).toEqual([
      'table_spell.name_pt',
      'table_spell.range.distance_ft',
      'table_spell.damage[0].dice',
    ]);
    expect(placed.byField.get('table_spell.name_pt')?.[0].text).toContain(
      'Já existe uma magia da mesa',
    );
    expect(placed.top).toEqual([]);
    expect(placed.count).toBe(3);
    expect(refusalSummary('a magia', placed.count)).toBe(
      'Não foi possível salvar a magia. 3 campos precisam de ajuste. Nada foi salvo; o que você digitou continua aqui.',
    );
    expect(refusalSummary('a raça', 1)).toContain('1 campo precisa de ajuste');
  });

  it('takes a path with no input to the nearest one above, and a violation with no field or about another entry to the top', () => {
    const known = new Set(['table_race.traits[1].effects[0]']);
    const placed = placeViolations(
      [
        v('table_race.traits[1].effects[0].value', 'bad_formula'),
        v('', 'limit'),
        v('table_race.name_pt', 'duplicate_name', 'race:corujeiro@mesa'),
        v('table_race.size', 'bad_value'),
      ],
      (p) => known.has(p),
      { aOne: 'uma raça', nameOf: ctx.nameOf },
    );
    expect(placed.fields).toEqual(['table_race.traits[1].effects[0]']);
    expect(placed.top.map((t) => t.text)).toEqual([
      'A mesa já tem 1000 entradas. Arquive ou use o que já existe.',
      'Sobre Corujeiro: Já existe uma raça da mesa com este nome. Escolha outro.',
      'Escolha o tamanho.',
    ]);
  });

  it('says the same sentence once on one input', () => {
    const placed = placeViolations(
      [v('a.b', 'bad_value'), v('a.b', 'bad_value')],
      (p) => p === 'a.b',
      ctx,
    );
    expect(placed.byField.get('a.b')).toHaveLength(1);
  });
});

describe("the refusals of a feat, on the feat editor's fields", () => {
  const feat = { aOne: 'um talento' };

  it('writes the prerequisite and the ability increase in Portuguese, by their paths', () => {
    expect(violationText(v('table_feat.prerequisite.level', 'bad_value'), feat)).toBe(
      'O nível vai de 1 a 20 (vazio não pede nada).',
    );
    expect(violationText(v('table_feat.prerequisite.minimums.strength', 'bad_value'), feat)).toBe(
      'A pontuação mínima vai de 1 a 30.',
    );
    expect(violationText(v('table_feat.prerequisite.race_key', 'dangling_reference'), feat)).toBe(
      'Esta raça ou sub-raça não existe mais. Escolha outra.',
    );
    expect(violationText(v('table_feat.prerequisite.races[1]', 'dangling_reference'), feat)).toBe(
      'Esta raça ou sub-raça não existe mais. Escolha outra.',
    );
    expect(violationText(v('table_feat.prerequisite.races[2]', 'bad_value'), feat)).toBe(
      'Cada raça ou sub-raça entra uma vez só.',
    );
    expect(violationText(v('table_feat.prerequisite.races', 'bad_value'), feat)).toBe(
      'Use a lista de raças ou a raça única, não as duas.',
    );
    expect(violationText(v('table_feat.prerequisite.races', 'limit'), feat)).toBe(
      'Uma lista de raças vai até 20.',
    );
    expect(violationText(v('table_feat.prerequisite.size', 'bad_value'), feat)).toBe(
      'Escolha um tamanho: Miúdo, Pequeno, Médio ou Grande.',
    );
    expect(
      violationText(v('table_feat.prerequisite.lacks_feature_key', 'dangling_reference'), feat),
    ).toBe('Este traço, característica ou talento não existe. Escolha outro.');
    expect(violationText(v('table_feat.prerequisite.lacks_feature_key', 'bad_value'), feat)).toBe(
      'Um talento não pode pedir que ele mesmo não esteja na ficha.',
    );
    expect(violationText(v('table_feat.effects[1]', 'limit'), feat)).toBe(
      'Um talento tem só um aumento de habilidade.',
    );
    expect(violationText(v('table_feat.name_pt', 'duplicate_name'), feat)).toBe(
      'Já existe um talento da mesa com este nome. Escolha outro.',
    );
  });
});

describe('pathAfterRowRemoved (#578)', () => {
  it('drops a path inside the removed row, moves the rows below up and leaves the rest', () => {
    const at = (path: string) => pathAfterRowRemoved(path, 'a.rows', 1);
    expect(at('a.rows[1]')).toBeNull();
    expect(at('a.rows[1].name')).toBeNull();
    expect(at('a.rows[2].name')).toBe('a.rows[1].name');
    expect(at('a.rows[12].x[3]')).toBe('a.rows[11].x[3]');
    expect(at('a.rows[0].name')).toBe('a.rows[0].name');
    expect(at('a.rows')).toBe('a.rows');
    expect(at('a.other[2]')).toBe('a.other[2]');
  });
});

describe('the refusals of a trait with options and of the traits a subrace replaces', () => {
  const race = { aOne: 'uma raça' };

  it('says a trait with options is already the choice, under the effect select', () => {
    const field = 'table_race.traits[2].effects[0].choice';
    const text = violationText(v(field, 'bad_value'), race);
    expect(text).toBe(
      'Um traço com opções já é a escolha: troque este efeito por “Só texto” ou outro.',
    );
    const placed = placeViolations(
      [v(field, 'bad_value')],
      (p) => p === 'table_race.traits[2].effects[0].type',
      {
        ...race,
        redirect: (f) => f.replace(/\.choice$/, '.type'),
      },
    );
    expect(placed.fields).toEqual(['table_race.traits[2].effects[0].type']);
  });

  it('says two options of one trait have the same name, not that the name is too long', () => {
    expect(violationText(v('table_subrace.traits[0].options[1].name_pt', 'bad_name'), race)).toBe(
      'Duas opções deste traço têm o mesmo nome. Mude uma delas.',
    );
    expect(
      violationText(v('table_class.levels[2].features[0].options[1].name_pt', 'bad_name'), {
        aOne: 'uma classe',
      }),
    ).toBe('Duas opções desta característica têm o mesmo nome. Mude uma delas.');
    expect(violationText(v('table_race.traits[0].name_pt', 'bad_name'), race)).toBe(
      'O nome tem de 1 a 60 letras.',
    );
  });

  it("says a replaced trait is not the race's, or is marked twice, and the limit", () => {
    expect(
      violationText(v('table_subrace.replaces_trait_keys[0]', 'dangling_reference'), race),
    ).toBe('Este traço não é da raça da sub-raça. Desmarque para salvar.');
    expect(violationText(v('table_subrace.replaces_trait_keys[1]', 'bad_value'), race)).toBe(
      'Este traço está marcado duas vezes. Desmarque um.',
    );
    expect(violationText(v('table_subrace.replaces_trait_keys', 'limit'), race)).toBe(
      'Uma sub-raça substitui até 60 traços.',
    );
  });
});
