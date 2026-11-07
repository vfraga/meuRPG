/** What the server's reason at a field of a table creature says in Portuguese (#578): the paths and reasons of
 * backend/internal/rules/overlay_creature.go and overlay_creature_formula.go. A formula that does not compile (`bad_formula`) is
 * worded for every field alike by `violationText`, and a name's `bad_name` is worded here, for the trait's and the attack's alike. */
/** The refusal of a damage type a row holds twice; the row names the type itself (`duplicateText`). */
export const DUPLICATE_TYPE_TEXT = 'Há um tipo de dano repetido nesta linha. Tire o repetido.';

export const CREATURE_FIELD_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  {
    shape: /^table_creature\.base_key$/,
    reason: 'dangling_reference',
    text: 'Esta criatura não está no SRD. Escolha outra como base.',
  },
  { shape: /^table_creature\.base_key$/, text: 'Escolha uma criatura do SRD como base.' },
  {
    shape: /^table_creature\.hit_points$/,
    text: 'Os pontos de vida vão de 1 a 9.999 (vazio é o da base).',
  },
  {
    shape: /^table_creature\.armor_class$/,
    text: 'A classe de armadura vai de 1 a 40 (vazio é a da base).',
  },
  { shape: /^table_creature\.damage_bonus$/, text: 'O bônus no dano vai de −10 a 30.' },
  {
    shape: /^table_creature\.traits$/,
    reason: 'limit',
    text: 'No máximo 10 traços. Remova um para pôr outro.',
  },
  {
    shape: /^table_creature\.(traits|attacks)\[\]\.name_pt$/,
    text: 'Use de 1 a 80 letras numa linha, e um nome que a base e os outros traços e ataques ainda não usem. Escolha outro.',
  },
  {
    shape: /^table_creature\.traits\[\]\.text_pt$/,
    text: 'O texto do traço tem de 1 a 4.000 letras.',
  },
  {
    shape: /^table_creature\.attacks$/,
    reason: 'limit',
    text: 'No máximo 8 ataques. Remova um para pôr outro.',
  },
  {
    shape: /^table_creature\.attacks\[\]\.reach_ft$/,
    text: 'O alcance de um ataque corpo a corpo vai de 0 a 9 m; vazio ou 0 é 1,5 m.',
  },
  {
    shape: /^table_creature\.attacks\[\]\.range_ft$/,
    text: 'O alcance de um ataque à distância vai de 0,3 m a 180 m.',
  },
  {
    shape: /^table_creature\.attacks\[\]\.long_range_ft$/,
    text: 'O alcance longo fica vazio ou passa do alcance normal, até 360 m.',
  },
  {
    shape: /^table_creature\.attacks\[\]\.dice_count$/,
    text: 'O ataque tem de 1 a 20 dados.',
  },
  {
    shape: /^table_creature\.attacks\[\]\.dice_sides$/,
    text: 'Escolha o dado: d4, d6, d8, d10, d12, d20 ou d100.',
  },
  {
    shape: /^table_creature\.attacks\[\]\.damage_type$/,
    text: 'Escolha um tipo de dano da lista.',
  },
  {
    shape: /^table_creature\.damage_mods$/,
    reason: 'limit',
    text: 'No máximo 12 linhas de resistência, imunidade ou vulnerabilidade. Remova uma para pôr outra.',
  },
  {
    shape: /^table_creature\.damage_mods\[\]\.kind$/,
    text: 'Escolha resistência, imunidade ou vulnerabilidade.',
  },
  {
    shape: /^table_creature\.damage_mods\[\]\.damage_types$/,
    text: 'Escolha ao menos um tipo de dano.',
  },
  {
    shape: /^table_creature\.damage_mods\[\]\.damage_types\[\]$/,
    reason: 'dangling_reference',
    text: 'Este tipo de dano não está no SRD. Tire-o da linha.',
  },
  {
    shape: /^table_creature\.damage_mods\[\]\.damage_types\[\]$/,
    text: DUPLICATE_TYPE_TEXT,
  },
  {
    shape: /^table_creature\.damage_mods\[\]\.qualifier$/,
    text: 'Só resistência ou imunidade a concussão, perfurante e cortante pode ter esse "Vale contra". Escolha "Todo dano desses tipos" ou mude a linha.',
  },
];
