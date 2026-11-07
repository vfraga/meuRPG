/** What the server's reason at a field of a feature's summon says in Portuguese (#578): the paths and reasons of
 * backend/internal/rules/overlay_summon.go and overlay_featuresummon.go, under `<feature>.summon` for a class, subclass, race,
 * background or class-feature entry alike (`table_class.levels[].features[].summon`, `table_race.traits[].summon`,
 * `table_background.feature.summon`, `table_class_feature.summon`). A formula that does not compile (`bad_formula`) is worded for
 * every field alike by `violationText`; the casting time, the duration and the concentration use the spell's own rows. */
export const SUMMON_FIELD_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  {
    shape: /\.summon\.creature_keys$/,
    text: 'Escolha de 1 a 8 criaturas.',
  },
  {
    shape: /\.summon\.creature_keys\[\]$/,
    reason: 'dangling_reference',
    text: 'Esta criatura não existe mais. Tire-a da lista.',
  },
  {
    shape: /\.summon\.creature_keys\[\]$/,
    text: 'Esta criatura está na lista duas vezes. Tire a repetida.',
  },
  {
    shape: /\.summon\.count$/,
    text: 'Escreva quantas criaturas vêm: um número ou uma fórmula do dono, como 1 ou mod("cha").',
  },
  {
    shape: /\.summon\.max_live$/,
    text: 'De 0 (sem limite) a 40, e nunca abaixo de quantas vêm de uma vez.',
  },
  {
    shape: /\.summon\.turn_mode$/,
    text: 'Escolha iniciativa própria ou em grupo. Agir logo depois do dono ainda não existe.',
  },
  {
    shape: /\.summon\.replaces$/,
    text: 'Escolha se as anteriores ficam ou se as novas tomam o lugar.',
  },
  {
    shape: /\.summon\.resource_key$/,
    reason: 'dangling_reference',
    text: 'Nenhum efeito Recurso desta característica ou da classe tem este nome. Escolha outro recurso.',
  },
  {
    shape: /\.summon\.resource_key$/,
    text: 'Escolha o recurso que cada uso gasta.',
  },
  {
    shape: /\.summon\.resource_cost$/,
    text: 'Cada invocação gasta 1 uso do recurso.',
  },
  {
    shape: /\.summon\.(count_per_slot_level|role)$/,
    text: 'Esta característica não gasta espaço de magia, e a criatura invocada é sempre uma companheira.',
  },
  {
    shape: /\.summon$/,
    text: 'Confira as criaturas que esta característica invoca.',
  },
];
