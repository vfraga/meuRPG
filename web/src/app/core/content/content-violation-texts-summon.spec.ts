import { placeViolations, violationText } from './content-violations';

const CTX = { aOne: 'uma classe' };

// The paths and reasons of backend/internal/rules/overlay_summon.go, under each kind of entry that has a feature.
const CASES: [field: string, reason: string, text: string][] = [
  [
    'table_class.levels[2].features[0].summon.creature_keys',
    'limit',
    'Escolha de 1 a 8 criaturas.',
  ],
  [
    'table_subclass.levels[0].features[1].summon.creature_keys[1]',
    'dangling_reference',
    'Esta criatura não existe mais. Tire-a da lista.',
  ],
  [
    'table_race.traits[0].summon.creature_keys[2]',
    'bad_value',
    'Esta criatura está na lista duas vezes. Tire a repetida.',
  ],
  [
    'table_background.feature.summon.count',
    'bad_value',
    'Escreva quantas criaturas vêm: um número ou uma fórmula do dono, como 1 ou mod("cha").',
  ],
  [
    'table_class_feature.summon.count',
    'bad_formula',
    'Esta fórmula não funciona. Use só as funções da lista e confira os parênteses.',
  ],
  [
    'table_class_feature.summon.max_live',
    'bad_value',
    'De 0 (sem limite) a 40, e nunca abaixo de quantas vêm de uma vez.',
  ],
  [
    'table_subrace.traits[1].summon.turn_mode',
    'bad_value',
    'Escolha iniciativa própria ou em grupo. Agir logo depois do dono ainda não existe.',
  ],
  [
    'table_class_feature.summon.replaces',
    'bad_value',
    'Escolha se as anteriores ficam ou se as novas tomam o lugar.',
  ],
  [
    'table_class_feature.summon.resource_key',
    'dangling_reference',
    'Nenhum efeito Recurso desta característica ou da classe tem este nome. Escolha outro recurso.',
  ],
  ['table_class_feature.summon.resource_key', 'bad_value', 'Escolha o recurso que cada uso gasta.'],
  [
    'table_class_feature.summon.resource_cost',
    'bad_value',
    'Cada invocação gasta 1 uso do recurso.',
  ],
  [
    'table_class.levels[0].features[0].summon.count_per_slot_level',
    'bad_value',
    'Esta característica não gasta espaço de magia, e a criatura invocada é sempre uma companheira.',
  ],
  [
    'table_class.levels[0].features[0].summon.role',
    'bad_value',
    'Esta característica não gasta espaço de magia, e a criatura invocada é sempre uma companheira.',
  ],
  [
    'table_class.levels[0].features[0].summon',
    'bad_value',
    'Confira as criaturas que esta característica invoca.',
  ],
  [
    'table_class_feature.summon.concentration',
    'bad_value',
    'Concentração pede uma duração por tempo.',
  ],
  ['table_class_feature.summon.duration.kind', 'bad_value', 'Escolha a duração.'],
  [
    'table_class_feature.summon.casting_time.amount',
    'bad_value',
    'O tempo vai de 1 a 60 minutos ou horas; uma ação vale 1.',
  ],
];

describe("the sentence of a refusal at a feature's summon (#578)", () => {
  it.each(CASES)('%s (%s)', (field, reason, text) => {
    expect(violationText({ field, reason }, CTX)).toBe(text);
  });
});

describe("a violation the server echoes at the summon's own text", () => {
  it("lands on the feature's text field", () => {
    const known = (path: string) =>
      path === 'table_class_feature.desc_pt' || path === 'table_class_feature.summon';
    const placed = placeViolations(
      [{ field: 'table_class_feature.summon.desc_pt[0]', reason: 'bad_text', key: '' } as never],
      known,
      CTX,
    );
    expect(placed.fields).toEqual(['table_class_feature.desc_pt']);
  });
});
