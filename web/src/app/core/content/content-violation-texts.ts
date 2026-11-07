import { RACE_FIELD_TEXTS } from './content-violation-texts-race';

/** What the server's reason at a field says in Portuguese: the table `violationText` looks a violation up in (the rest of its words are there). */

import { CREATURE_FIELD_TEXTS } from './content-violation-texts-creature';
import { SPELL_SUMMON_TEXTS, SUMMON_FIELD_TEXTS } from './content-violation-texts-summon';

const DICE_TEXT = 'Escreva o dado assim: 2d8 (de 1 a 20 dados).';
export const FORMULA_TEXT =
  'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.';
/** What a refusal of an effect's field says when nothing more specific is known: the picker puts the words of the effect's type in
 * their place (`EFFECT_REFUSAL_TEXTS`). */
export const BAD_EFFECT_VALUE = 'Este valor não serve para este efeito.';
export const DANGLING_TEXT = 'Isto aponta para algo que não existe mais. Escolha de novo.';
export const FEAT_VALUE_TEXT =
  'Este valor não serve para este efeito (um aumento de habilidade soma +1 ou +2 a cada habilidade).';
export const GENERIC_EFFECT_TEXTS: ReadonlySet<string> = new Set([
  BAD_EFFECT_VALUE,
  DANGLING_TEXT,
  FEAT_VALUE_TEXT,
]);
export const LONG_TEXT = 'Este texto é longo demais. Encurte o parágrafo.';

/** What the server's reason at a field says in Portuguese, from the shape of the path (the indexes are `[]`). A row with no
 * reason answers any reason. The rows are ordered: the first that matches wins. The paths and reasons are the server's
 * (rules.TestEntryViolationPaths pins them). */
export const FIELD_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  // A race or subrace trait with options, and a subrace's replaced traits: backend/internal/rules/overlay_features.go.
  {
    shape: /^table_(race|subrace)\.traits\[\]\.effects\[\]\.choice$/,
    reason: 'bad_value',
    text: 'Um traço com opções já é a escolha: troque este efeito por “Só texto” ou outro.',
  },
  {
    shape: /^table_(race|subrace)\.traits\[\]\.options\[\]\.name_pt$/,
    reason: 'bad_name',
    text: 'Duas opções deste traço têm o mesmo nome. Mude uma delas.',
  },
  {
    shape: /\.options\[\]\.name_pt$/,
    reason: 'bad_name',
    text: 'Duas opções desta característica têm o mesmo nome. Mude uma delas.',
  },
  {
    shape: /^table_subrace\.replaces_trait_keys$/,
    reason: 'limit',
    text: 'Uma sub-raça substitui até 60 traços.',
  },
  {
    shape: /^table_subrace\.replaces_trait_keys\[\]$/,
    reason: 'dangling_reference',
    text: 'Este traço não é da raça da sub-raça. Desmarque para salvar.',
  },
  {
    shape: /^table_subrace\.replaces_trait_keys\[\]$/,
    text: 'Este traço está marcado duas vezes. Desmarque um.',
  },
  // A class and a subclass (E10-02): the paths and reasons of backend/internal/rules/overlay_class.go.
  { shape: /^table_class\.hit_die$/, text: 'O dado de vida é d6, d8, d10 ou d12.' },
  {
    shape: /^table_class\.saving_throws(\[\])?$/,
    text: 'Escolha dois testes de resistência diferentes.',
  },
  {
    shape: /^table_class\.skill_choose$/,
    text: 'Quantas perícias a classe dá: de 0 até o número de perícias da lista.',
  },
  { shape: /^table_class\.skill_from\[\]$/, text: 'Esta perícia não existe. Escolha da lista.' },
  {
    shape: /^table_class\.(multiclass_)?proficiencies\[\]$/,
    text: 'Escolha uma armadura, arma ou ferramenta da lista.',
  },
  {
    shape: /^table_class\.multiclass_skill_choose$/,
    text: 'As perícias de quem vem de outra classe saem da lista da classe: de 0 até o número dela.',
  },
  { shape: /^table_class\.(minimums|any_of)\.[a-z]+$/, text: 'O valor mínimo vai de 1 a 30.' },
  {
    shape: /^table_class\.(minimums|any_of)$/,
    text: 'O pré-requisito de multiclasse precisa de uma habilidade.',
  },
  {
    shape: /^table_class\.subclass_level$/,
    text: 'Esta classe não tem nível de subclasse entre 1 e 20.',
  },
  {
    shape: /^table_class\.asi_levels\[\]$/,
    text: 'Os níveis de incremento no valor de habilidade são diferentes, de 1 a 20.',
  },
  { shape: /^table_class\.levels$/, text: 'A tabela dos níveis precisa ter os 20 níveis.' },
  {
    shape: /^table_subclass\.levels$/,
    text: 'Uma subclasse que conjura precisa de uma linha em cada nível, do começo da conjuração ao 20.',
  },
  {
    shape: /^table_(class|subclass)\.levels\[\]\.level$/,
    text: 'Os níveis da subclasse vão de 1 a 20, em ordem.',
  },
  {
    shape: /^table_class\.levels\[\]\.prof_bonus$/,
    text: 'O bônus de proficiência vai de +1 a +12 (vazio é o do SRD).',
  },
  {
    shape: /^table_(class|subclass)\.levels\[\]\.cantrips_known$/,
    text: 'Truques: de 0 a 30, e 0 antes de a conjuração começar.',
  },
  {
    shape: /^table_(class|subclass)\.levels\[\]\.spells_known$/,
    text: 'Magias conhecidas: de 0 a 200, e 0 antes de a conjuração começar.',
  },
  {
    shape: /^table_(class|subclass)\.levels\[\]\.slots\[\]$/,
    text: 'Espaços de magia: de 0 a 9 por nível de magia, e 0 antes de a conjuração começar. Pacto: espaços de um nível só.',
  },
  {
    shape: /^table_(class|subclass)\.levels\[\]\.slots$/,
    text: 'Quem conjura precisa de espaços de magia neste nível.',
  },
  { shape: /^table_class\.levels\[\]\.features\[\]$/, reason: 'limit', text: 'LIMIT_CLASS' },
  { shape: /^table_subclass\.levels\[\]\.features\[\]$/, reason: 'limit', text: 'LIMIT_SUBCLASS' },
  {
    shape: /^table_subclass\.levels\[\]\.features$/,
    text: 'Uma subclasse só ganha características a partir do nível em que se escolhe.',
  },
  { shape: /^table_(class|subclass)\.casting\.kind$/, text: 'Escolha como conjura.' },
  {
    shape: /^table_(class|subclass)\.casting\.ability$/,
    text: 'Escolha a habilidade de conjuração.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.preparation$/,
    text: 'Escolha se as magias são preparadas ou conhecidas.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.prepared_max$/,
    text: 'Só quem prepara magias tem este número.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.list_from$/,
    reason: 'dangling_reference',
    text: 'Esta classe não existe mais. Escolha outra lista.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.list_from$/,
    text: 'Escolha uma classe que tenha lista de magias própria (uma subclasse que conjura precisa de uma).',
  },
  {
    shape: /^table_class\.casting\.srd_spell_keys\[\]$/,
    reason: 'dangling_reference',
    text: 'Esta magia não está no SRD. Tire-a da lista.',
  },
  {
    shape: /^table_class\.casting\.srd_spell_keys\[\]$/,
    text: 'Esta magia está na lista duas vezes.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.srd_spell_keys$/,
    reason: 'limit',
    text: 'A lista passou de 400 magias do SRD.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.srd_spell_keys$/,
    text: 'Só uma classe com lista própria tem magias do SRD. Escolha "A própria lista da classe".',
  },
  {
    shape: /^table_subclass\.spells_expanded$/,
    text: 'Esta marca precisa de magias na subclasse. Adicione magias ou desligue.',
  },
  {
    shape: /\.effects\[\]\.only_proficient$/,
    text: 'Só vale para a especialização numa perícia.',
  },
  {
    shape: /^table_(class|subclass)\.casting\.start_level$/,
    text: 'A conjuração começa num nível de 1 a 20, e uma subclasse não conjura antes de ser escolhida.',
  },
  {
    shape: /^table_(class|subclass)\.casting$/,
    text: 'Uma subclasse só conjura se a classe dela não conjura. Uma classe sem conjuração não tem lista de magias.',
  },
  { shape: /^table_subclass\.class_key$/, text: 'A classe desta subclasse não existe mais.' },
  { shape: /^table_subclass\.level$/, text: 'A subclasse é escolhida no nível da classe.' },
  {
    shape: /^table_subclass\.always_prepared$/,
    text: 'Magias sempre preparadas pedem uma classe ou subclasse que conjura.',
  },
  {
    shape: /^table_subclass\.always_prepared\[\]\.class_level$/,
    text: 'O nível da classe vai de 1 a 20.',
  },
  {
    shape: /^table_subclass\.always_prepared\[\]\.spell_key$/,
    reason: 'dangling_reference',
    text: 'Esta magia não existe mais. Escolha outra.',
  },
  {
    shape: /^table_subclass\.always_prepared\[\]\.spell_key$/,
    text: 'Uma magia sempre preparada é de 1º nível ou mais, nunca um truque.',
  },
  {
    shape: /^table_(class|subclass)\.levels\[\]\.features\[\]$/,
    text: 'Confira esta característica.',
  },
  // A feat (MR-025): the paths and reasons of backend/internal/rules/overlay_feat.go.
  {
    shape: /^table_feat\.prerequisite\.(minimums|any_of)(\.[a-z]+)?$/,
    text: 'A pontuação mínima vai de 1 a 30.',
  },
  {
    shape: /^table_feat\.prerequisite\.proficiency_key$/,
    text: 'Escolha uma proficiência da lista do SRD.',
  },
  {
    shape: /^table_feat\.prerequisite\.race_key$/,
    text: 'Esta raça ou sub-raça não existe mais. Escolha outra.',
  },
  {
    shape: /^table_feat\.prerequisite\.races$/,
    reason: 'limit',
    text: 'Uma lista de raças vai até 20.',
  },
  {
    shape: /^table_feat\.prerequisite\.races$/,
    text: 'Use a lista de raças ou a raça única, não as duas.',
  },
  {
    shape: /^table_feat\.prerequisite\.races\[\]$/,
    reason: 'dangling_reference',
    text: 'Esta raça ou sub-raça não existe mais. Escolha outra.',
  },
  {
    shape: /^table_feat\.prerequisite\.races\[\]$/,
    text: 'Cada raça ou sub-raça entra uma vez só.',
  },
  {
    shape: /^table_feat\.prerequisite\.size$/,
    text: 'Escolha um tamanho: Miúdo, Pequeno, Médio ou Grande.',
  },
  {
    shape: /^table_feat\.prerequisite\.lacks_feature_key$/,
    reason: 'dangling_reference',
    text: 'Este traço, característica ou talento não existe. Escolha outro.',
  },
  {
    shape: /^table_feat\.prerequisite\.lacks_feature_key$/,
    text: 'Um talento não pode pedir que ele mesmo não esteja na ficha.',
  },
  {
    shape: /^table_feat\.prerequisite\.level$/,
    text: 'O nível vai de 1 a 20 (vazio não pede nada).',
  },
  {
    shape: /^table_feat\.effects\[\]$/,
    reason: 'limit',
    text: 'Um talento tem só um aumento de habilidade.',
  },
  {
    shape: /^table_feat\.effects\[\]\.count$/,
    reason: 'bad_value',
    text: 'Quantos o jogador escolhe: de 1 até o número de itens da lista (em um aumento de habilidade, as habilidades da lista).',
  },
  {
    shape: /^table_feat\.effects\[\]\.from$/,
    reason: 'bad_value',
    text: 'Liste as opções entre as quais o jogador escolhe, sem repetir (em um aumento de habilidade, as habilidades).',
  },
  {
    shape: /^table_feat\.effects\[\]\.from$/,
    reason: 'dangling_reference',
    text: 'Este talento não existe mais. Tire-o da lista.',
  },
  {
    shape: /^table_feat\.effects\[\]\.value$/,
    reason: 'bad_value',
    text: FEAT_VALUE_TEXT,
  },
  // reroll and roll_minimum (rolleffects.go): their own fields.
  { shape: /\.effects\[\]\.rolls$/, text: 'Marque ao menos uma jogada.' },
  { shape: /\.effects\[\]\.at_most$/, text: 'A face vai de 1 a 20.' },
  // A table option for an SRD option set (MR-025): the paths and reasons of backend/internal/rules/overlay_option.go.
  {
    shape: /^table_option\.set$/,
    text: 'Escolha um conjunto da lista. O conjunto de uma opção salva não muda.',
  },
  {
    shape: /^table_option\.offered_by$/,
    reason: 'bad_value',
    text: 'Só o conjunto "Opção de característica da mesa" tem característica que oferece. Tire esta.',
  },
  {
    shape: /^table_option\.offered_by$/,
    text: 'Esta característica não existe mais ou não tem lista de opções. Escolha outra.',
  },
  {
    shape: /^table_option\.prerequisite\.class_level$/,
    text: 'A classe e o nível vão juntos: preencha os dois ou nenhum.',
  },
  {
    shape: /^table_option\.prerequisite\.class_level\.class_key$/,
    text: 'Esta classe não existe mais. Escolha outra.',
  },
  {
    shape: /^table_option\.prerequisite\.class_level\.level$/,
    text: 'O nível vai de 1 a 20.',
  },
  {
    shape: /^table_option\.prerequisite\.requires_option_keys(\[\])?$/,
    text: 'Escolha opções que existem, sem a própria opção, e no máximo 10.',
  },
  {
    shape: /^table_option\.prerequisite\.requires_pact$/,
    text: 'Escolha uma dádiva do pacto da lista.',
  },
  {
    shape: /^table_option\.prerequisite\.requires_spell_key$/,
    text: 'Esta magia não existe mais. Escolha outra.',
  },
  {
    shape: /^table_option\.effects\[\]\.type$/,
    text: 'Uma opção não pede outra escolha: use outro tipo de efeito.',
  },
  // A table class feature (MR-025): the paths and reasons of backend/internal/rules/overlay_classfeature.go.
  {
    shape: /^table_class_feature\.class_key$/,
    text: 'Esta classe não existe mais. Escolha outra.',
  },
  {
    shape: /^table_class_feature\.level$/,
    text: 'O nível vai de 1 a 20.',
  },
  {
    shape: /^table_class_feature\.replaces_feature_key$/,
    text: 'A classe não tem essa característica neste nível ou depois dele. Uma característica que o personagem já tinha antes não pode ser trocada.',
  },
  {
    shape: /^table_class_feature\.effects(\[\])?$/,
    text: 'Esta característica tem efeitos demais. Tire alguns.',
  },
  ...CREATURE_FIELD_TEXTS,
  ...SPELL_SUMMON_TEXTS,
  ...SUMMON_FIELD_TEXTS,
  { shape: /\.level$/, text: 'Escolha um nível de magia de 0 (truque) a 9.' },
  { shape: /\.school_key$/, text: 'Escolha a escola da magia.' },
  {
    shape: /\.range\.kind$/,
    text: 'Esse alcance não combina com o alvo. “Só quem conjura” pede Pessoal; criaturas escolhidas pedem distância ou Toque.',
  },
  { shape: /\.range\.distance_ft$/, text: 'O alcance vai de 1,5 m a 1.584 m, em passos de 1,5 m.' },
  {
    shape: /\.target\.kind$/,
    text: 'Este alvo não aceita os outros campos preenchidos. Confira o alvo.',
  },
  {
    shape: /\.target\.shape$/,
    text: 'Escolha a forma da área: cone, cubo, cilindro, linha ou esfera.',
  },
  { shape: /\.target\.size_ft$/, text: 'A área vai de 1,5 m a 90 m, em passos de 1,5 m.' },
  {
    shape: /\.target\.(count|per_slot_level)$/,
    text: 'Escreva um número de criaturas: 2 ou mais, e até 10 a mais por nível de espaço.',
  },
  { shape: /\.casting_time\.unit$/, text: 'Escolha o tempo de conjuração.' },
  {
    shape: /\.casting_time\.amount$/,
    text: 'O tempo vai de 1 a 60 minutos ou horas; uma ação vale 1.',
  },
  {
    shape: /\.casting_time\.trigger_pt$/,
    reason: 'bad_text',
    text: 'O gatilho é uma linha só, de até 200 letras.',
  },
  { shape: /\.casting_time\.trigger_pt$/, text: 'Só uma reação tem gatilho.' },
  { shape: /\.duration\.kind$/, text: 'Escolha a duração.' },
  { shape: /\.duration\.amount$/, text: 'A duração vai de 1 a 999.' },
  { shape: /\.duration\.unit$/, text: 'Escolha a unidade da duração.' },
  { shape: /\.concentration$/, text: 'Concentração pede uma duração por tempo.' },
  { shape: /\.ritual$/, text: 'Um truque não é um ritual.' },
  { shape: /\.components\.material_pt$/, reason: 'bad_text', text: LONG_TEXT },
  { shape: /\.components\.material_pt$/, text: 'O material vai com o componente M, e só com ele.' },
  {
    shape: /\.attack$/,
    text: 'Uma magia de área não tem ataque. Escolha um teste de resistência, ou outro alvo.',
  },
  { shape: /\.save$/, text: 'Uma magia tem ataque ou teste de resistência, não os dois.' },
  { shape: /\.save\.ability$/, text: 'Escolha a habilidade do teste de resistência.' },
  {
    shape: /\.save\.on_success$/,
    text: 'Escolha o que acontece ao passar: metade do dano (que pede dano) ou nada.',
  },
  { shape: /\.damage$/, reason: 'limit', text: 'Uma magia tem até 4 tipos de dano.' },
  { shape: /\.damage\[\]\.damage_type_key$/, text: 'Escolha um tipo de dano da lista.' },
  {
    shape: /\.(damage\[\]\.(dice|per_slot_level|per_tier)|heal\.(dice|per_slot_level))$/,
    text: DICE_TEXT,
  },
  { shape: /\.heal$/, text: 'Um truque não cura.' },
  { shape: /\.(desc_pt|higher_level_pt)\[\]$/, text: LONG_TEXT },
  { shape: /\.(desc_pt|higher_level_pt)$/, text: 'O texto tem parágrafos demais.' },
  {
    shape: /\.class_keys\[\]$/,
    reason: 'dangling_reference',
    text: 'Esta classe não existe mais. Tire-a da lista.',
  },
  { shape: /\.class_keys\[\]$/, text: 'Esta classe está na lista duas vezes.' },
  ...RACE_FIELD_TEXTS,
];
