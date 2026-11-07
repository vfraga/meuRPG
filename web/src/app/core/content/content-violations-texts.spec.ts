import { violationText } from './content-violations';

/** The sentence of every reason at a field of each shape: the branches `violationText` walks, in the order it walks them. */
const CTX = { aOne: 'uma magia' };
const CREATURE_CTX = { aOne: 'uma criatura' };
const CLASS_CTX = { aOne: 'uma classe', maxFeatures: 40, featureCount: 41 };

const CASES: Record<string, [field: string, reason: string, ctx?: typeof CLASS_CTX]> = {
  'entry limit': ['', 'limit'],
  'entry size': ['', 'size_limit'],
  'entry overlay': ['', 'overlay'],
  'entry other': ['', 'anything'],
  'name duplicate': ['table_race.name_pt', 'duplicate_name'],
  'name bad': ['table_race.name_pt', 'bad_name'],
  'name bad text': ['table_race.name_pt', 'bad_text'],
  'name reserved': ['table_race.name_pt', 'reserved_key'],
  'name bad key': ['table_race.name_pt', 'bad_key'],
  'name duplicate key': ['table_race.name_pt', 'duplicate_key'],
  'name other reason': ['table_race.name_pt', 'bad_value'],
  'name immutable': ['table_race.name_pt', 'immutable'],
  'level immutable': ['table_spell.level', 'immutable'],
  'immutable elsewhere': ['table_spell.school_key', 'immutable'],
  'size anywhere': ['table_spell.desc_pt[0]', 'size_limit'],
  'bad formula': ['table_race.traits[0].effects[0].value', 'bad_formula'],
  'forbidden effect': ['table_race.traits[0].effects[0].type', 'forbidden_effect'],
  'resource bad value': ['table_race.traits[0].effects[0].resource', 'bad_value'],
  'effect value bad value': ['table_race.traits[0].effects[0].value', 'bad_value'],
  'effect other reason': ['table_race.traits[0].effects[0].when', 'required'],
  'dangling anywhere': ['table_race.languages[0]', 'dangling_reference'],
  'dangling class key': ['table_spell.class_keys[0]', 'dangling_reference'],
  'class key twice': ['table_spell.class_keys[1]', 'duplicate_key'],
  'skills dangling': ['table_background.skills[0]', 'dangling_reference'],
  'skills twice': ['table_background.skills[0]', 'bad_value'],
  'text too long': ['table_spell.components.material_pt', 'bad_text'],
  'text with no component': ['table_spell.components.material_pt', 'bad_value'],
  'unknown path, bad text': ['table_x.unknown', 'bad_text'],
  'unknown path, limit': ['table_x.unknown', 'limit'],
  'unknown path, duplicate name': ['table_x.unknown', 'duplicate_name'],
  'unknown path, duplicate key': ['table_x.unknown', 'duplicate_key'],
  'unknown path, any reason': ['table_x.unknown', 'zzz'],
  'class level slots': ['table_class.levels[3].slots[1]', 'bad_value'],
  'class features limit': ['table_class.levels[3].features[1]', 'limit', CLASS_CTX],
  'class features limit, no count': ['table_class.levels[3].features[1]', 'limit', CTX as never],
  'subclass features limit': ['table_subclass.levels[0].features[0]', 'limit', CLASS_CTX],
  'subclass features limit, defaults': [
    'table_subclass.levels[0].features[0]',
    'limit',
    CTX as never,
  ],
  'class feature other reason': ['table_class.levels[3].features[1]', 'bad_value'],
  'feat prerequisite': ['table_feat.prerequisite.minimums.strength', 'bad_value'],
  'spell damage dice': ['table_spell.damage[0].dice', 'bad_value'],
  'creature base missing': ['table_creature.base_key', 'dangling_reference'],
  'creature base changed': ['table_creature.base_key', 'immutable'],
  'creature base other': ['table_creature.base_key', 'bad_value'],
  'creature name duplicate': ['table_creature.name_pt', 'duplicate_name'],
  'creature name bad': ['table_creature.name_pt', 'bad_name'],
  'creature mods limit': ['table_creature.damage_mods', 'limit'],
  'creature mod kind': ['table_creature.damage_mods[2].kind', 'bad_value'],
  'creature mod no types': ['table_creature.damage_mods[0].damage_types', 'bad_value'],
  'creature mod type missing': [
    'table_creature.damage_mods[0].damage_types[1]',
    'dangling_reference',
  ],
  'creature mod type twice': ['table_creature.damage_mods[0].damage_types[1]', 'bad_value'],
  'creature mod qualifier': ['table_creature.damage_mods[1].qualifier', 'bad_value'],
  'creature hit points': ['table_creature.hit_points', 'bad_value'],
  'creature armor class': ['table_creature.armor_class', 'bad_value'],
  'creature damage bonus': ['table_creature.damage_bonus', 'bad_value'],
  'creature hit points formula': ['table_creature.hit_points_formula', 'bad_formula'],
  'creature armor class formula': ['table_creature.armor_class_formula', 'bad_formula'],
  'creature damage bonus formula': ['table_creature.damage_bonus_formula', 'bad_formula'],
  'creature attack bonus formula': ['table_creature.attack_bonus_formula', 'bad_formula'],
  'creature traits limit': ['table_creature.traits', 'limit'],
  'creature trait name': ['table_creature.traits[2].name_pt', 'bad_name'],
  'creature trait text': ['table_creature.traits[0].text_pt', 'bad_text'],
  'creature attacks limit': ['table_creature.attacks', 'limit'],
  'creature attack name': ['table_creature.attacks[1].name_pt', 'bad_name'],
  'creature attack reach': ['table_creature.attacks[0].reach_ft', 'bad_value'],
  'creature attack range': ['table_creature.attacks[0].range_ft', 'bad_value'],
  'creature attack long range': ['table_creature.attacks[0].long_range_ft', 'bad_value'],
  'creature attack dice': ['table_creature.attacks[0].dice_count', 'bad_value'],
  'creature attack sides': ['table_creature.attacks[0].dice_sides', 'bad_value'],
  'creature attack damage type': ['table_creature.attacks[0].damage_type', 'dangling_reference'],
  'creature attack to hit': ['table_creature.attacks[0].to_hit', 'bad_formula'],
  'creature attack damage bonus': ['table_creature.attacks[0].damage_bonus', 'bad_formula'],
  'reroll no rolls': ['table_feat.effects[0].rolls', 'bad_value'],
  'reroll face': ['table_class_feature.effects[1].at_most', 'bad_value'],
  'reroll dangling resource': ['table_feat.effects[0].resource', 'dangling_reference'],
  'feat effect value': ['table_feat.effects[0].value', 'bad_value'],
  'feat options limit': ['table_feat.options', 'limit'],
  'feat choice with its own options and a set': ['table_feat.effects[1].set', 'bad_value'],
  'feat option reads a pick': ['table_feat.options[2].effects[0].target', 'bad_value'],
  'feat option other field': ['table_feat.options[2].effects[0].value', 'bad_value'],
  'feat pick before its choice': ['table_feat.effects[0].proficiency', 'dangling_reference'],
  'feat option name twice': ['table_feat.options[1].name_pt', 'bad_name'],
  'option offered by, dangling': ['table_option.offered_by', 'dangling_reference'],
  // The editor drops the feature with the set, so this one is defensive: a body from another client can still carry it.
  'option offered by, another set': ['table_option.offered_by', 'bad_value'],
};

const EXPECTED: Record<string, string> = {
  'feat options limit': 'O talento passou de 60 opções. Remova uma.',
  'feat choice with its own options and a set':
    'Este talento tem opções próprias: a escolha oferece elas ou um conjunto, não os dois.',
  'feat option reads a pick':
    'Uma opção do talento não usa as escolhas dele. Escolha um valor fixo.',
  'feat option other field': 'Este valor não serve para este efeito.',
  'feat pick before its choice':
    'A escolha vem depois deste efeito. Suba a escolha ou desça este efeito.',
  'feat option name twice': 'Duas opções desta característica têm o mesmo nome. Mude uma delas.',
  'option offered by, dangling':
    'Esta característica não existe mais ou não tem lista de opções. Escolha outra.',
  'option offered by, another set':
    'Só o conjunto "Opção de característica da mesa" tem característica que oferece. Tire esta.',
  'entry limit': 'A mesa já tem 1000 entradas. Arquive ou use o que já existe.',
  'entry size': 'Esta entrada passou de 64 KiB de dados. Tire um pouco de texto.',
  'entry overlay': 'O conteúdo da mesa não fecha com esta entrada. Confira o que ela usa.',
  'entry other': 'O servidor recusou esta entrada. Confira os campos.',
  'name duplicate': 'Já existe uma magia da mesa com este nome. Escolha outro.',
  'name bad': 'O nome tem de 1 a 60 letras.',
  'name bad text': 'O nome tem de 1 a 60 letras.',
  'name reserved': 'Este nome não pode ser usado. Escolha outro.',
  'name bad key': 'Este nome não pode ser usado. Escolha outro.',
  'name duplicate key': 'Este nome não pode ser usado. Escolha outro.',
  'name other reason': 'Este valor não é aceito. Confira o campo.',
  'name immutable': 'Isto não muda depois de criado.',
  'level immutable': 'Um truque não vira magia de 1º nível ou mais, nem o contrário. Crie outra.',
  'immutable elsewhere': 'Isto não muda depois de criado.',
  'size anywhere': 'Esta entrada passou de 64 KiB de dados. Tire um pouco de texto.',
  'bad formula': 'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'forbidden effect': 'Este efeito não está no menu.',
  'resource bad value':
    'Use só letras minúsculas sem acento, números e _ (de 1 a 40), como vigilia.',
  'effect value bad value': 'Este valor não serve para este efeito.',
  'effect other reason': 'Confira este campo do efeito.',
  'dangling anywhere': 'Este idioma não existe. Escolha da lista.',
  'dangling class key': 'Esta classe não existe mais. Tire-a da lista.',
  'class key twice': 'Esta classe está na lista duas vezes.',
  'skills dangling': 'Esta perícia não existe. Escolha da lista.',
  'skills twice': 'Escolha duas perícias diferentes.',
  'text too long': 'Este texto é longo demais. Encurte o parágrafo.',
  'text with no component': 'O material vai com o componente M, e só com ele.',
  'unknown path, bad text': 'Este texto é longo demais. Encurte o parágrafo.',
  'unknown path, limit': 'Isto passou do limite.',
  'unknown path, duplicate name': 'Já existe uma entrada com isto. Escolha outro.',
  'unknown path, duplicate key': 'Já existe uma entrada com isto. Escolha outro.',
  'unknown path, any reason': 'Este valor não é aceito. Confira o campo.',
  'class level slots':
    'Espaços de magia: de 0 a 9 por nível de magia, e 0 antes de a conjuração começar. Pacto: espaços de um nível só.',
  'class features limit':
    'Esta classe tem 41. Uma classe da mesa não pode ter mais de 40 características.',
  'class features limit, no count': 'Uma classe da mesa não pode ter mais de 60 características.',
  'subclass features limit':
    'Esta subclasse tem 41. Uma subclasse da mesa não pode ter mais de 40 características.',
  'subclass features limit, defaults':
    'Uma subclasse da mesa não pode ter mais de 60 características.',
  'class feature other reason': 'Confira esta característica.',
  'feat prerequisite': 'A pontuação mínima vai de 1 a 30.',
  'spell damage dice': 'Escreva o dado assim: 2d8 (de 1 a 20 dados).',
  'creature base missing': 'Esta criatura não está no SRD. Escolha outra como base.',
  'creature base changed': 'Isto não muda depois de criado.',
  'creature base other': 'Escolha uma criatura do SRD como base.',
  'creature name bad': 'O nome tem de 1 a 80 letras.',
  'creature mods limit':
    'No máximo 12 linhas de resistência, imunidade ou vulnerabilidade. Remova uma para pôr outra.',
  'creature mod kind': 'Escolha resistência, imunidade ou vulnerabilidade.',
  'creature mod no types': 'Escolha ao menos um tipo de dano.',
  'creature mod type missing': 'Este tipo de dano não está no SRD. Tire-o da linha.',
  'creature mod type twice': 'Há um tipo de dano repetido nesta linha. Tire o repetido.',
  'creature mod qualifier':
    'Só resistência ou imunidade a concussão, perfurante e cortante pode ter esse "Vale contra". Escolha "Todo dano desses tipos" ou mude a linha.',
  'creature name duplicate': 'Já existe uma criatura da mesa com este nome. Escolha outro.',
  'creature hit points': 'Os pontos de vida vão de 1 a 9.999 (vazio é o da base).',
  'creature armor class': 'A classe de armadura vai de 1 a 40 (vazio é a da base).',
  'creature damage bonus': 'O bônus no dano vai de −10 a 30.',
  'creature hit points formula':
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'creature armor class formula':
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'creature damage bonus formula':
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'creature attack bonus formula':
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'creature traits limit': 'No máximo 10 traços. Remova um para pôr outro.',
  'creature trait name':
    'Use de 1 a 80 letras numa linha, e um nome que a base e os outros traços e ataques ainda não usem. Escolha outro.',
  'creature trait text': 'O texto do traço tem de 1 a 4.000 letras.',
  'creature attacks limit': 'No máximo 8 ataques. Remova um para pôr outro.',
  'creature attack name':
    'Use de 1 a 80 letras numa linha, e um nome que a base e os outros traços e ataques ainda não usem. Escolha outro.',
  'creature attack reach':
    'O alcance de um ataque corpo a corpo vai de 0 a 9 m; vazio ou 0 é 1,5 m.',
  'creature attack range': 'O alcance de um ataque à distância vai de 0,3 m a 180 m.',
  'creature attack long range': 'O alcance longo fica vazio ou passa do alcance normal, até 360 m.',
  'creature attack dice': 'O ataque tem de 1 a 20 dados.',
  'creature attack sides': 'Escolha o dado: d4, d6, d8, d10, d12, d20 ou d100.',
  'creature attack damage type': 'Escolha um tipo de dano da lista.',
  'creature attack to hit':
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'creature attack damage bonus':
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  'reroll no rolls': 'Marque ao menos uma jogada.',
  'reroll face': 'A face vai de 1 a 20.',
  // The type-less sentence: the picker puts the words of the effect's own type in its place (EFFECT_REFUSAL_TEXTS).
  'reroll dangling resource': 'Isto aponta para algo que não existe mais. Escolha de novo.',
  'feat effect value':
    'Este valor não serve para este efeito (um aumento de habilidade soma +1 ou +2 a cada habilidade).',
};

describe('the sentence of a refusal, by the shape of its field and its reason', () => {
  it.each(Object.keys(CASES))('%s', (name) => {
    const [field, reason, ctx] = CASES[name];
    const text = violationText(
      { field, reason },
      ctx ?? (name.startsWith('creature') ? CREATURE_CTX : CTX),
    );
    expect(text).toBe(EXPECTED[name]);
  });
});
