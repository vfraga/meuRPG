import { REFERENCE_FIELDS } from './effect-choices';

/** The fields of an effect that may hold a pick of the feat's named choice (`pickFields` of the server). */
const PICK_FIELDS = REFERENCE_FIELDS.filter((f) => f.pick)
  .map((f) => f.name)
  .join('|');

/** What the server's refusal of a pick that points below its field says; the editor says it too, before the server is asked. */
export const LATER_TEXT = 'A escolha vem depois deste efeito. Suba a escolha ou desça este efeito.';
/**
 * What the server's reasons say at a feat's option list and at its choice of options.
 * The paths and reasons are those of `rules/overlay_features.go` and `rules/picks.go`.
 * `FIELD_TEXTS` spreads these rows, where the first row that matches wins.
 */
export const FEAT_OPTION_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  {
    shape: /^table_feat\.options$/,
    reason: 'limit',
    text: 'O talento passou de 60 opções. Remova uma.',
  },
  {
    shape: /^table_feat\.effects\[\]\.set$/,
    reason: 'bad_value',
    text: 'Este talento tem opções próprias: a escolha oferece elas ou um conjunto, não os dois.',
  },
  {
    shape: new RegExp(`^table_feat\\.options\\[\\]\\.effects\\[\\]\\.(${PICK_FIELDS})$`),
    reason: 'bad_value',
    text: 'Uma opção do talento não usa as escolhas dele. Escolha um valor fixo.',
  },
  {
    shape: new RegExp(`^table_feat\\.effects\\[\\]\\.(${PICK_FIELDS})$`),
    reason: 'dangling_reference',
    text: LATER_TEXT,
  },
];

/** What the server's refusals say at the class level, the pact, the spell and the options a feat or an option asks (#580, S1). */
export const PREREQUISITE_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  {
    shape: /^(table_(option|feat)|.+\.options\[\])\.prerequisite\.class_level$/,
    text: 'A classe e o nível vão juntos: preencha os dois ou nenhum.',
  },
  {
    shape: /^(table_(option|feat)|.+\.options\[\])\.prerequisite\.class_level\.class_key$/,
    text: 'Esta classe não existe mais. Escolha outra.',
  },
  {
    shape: /^(table_(option|feat)|.+\.options\[\])\.prerequisite\.class_level\.level$/,
    text: 'O nível vai de 1 a 20.',
  },
  {
    shape: /^table_feat\.prerequisite\.requires_option_keys(\[\])?$/,
    text: 'Escolha opções que existem, no máximo 10.',
  },
  {
    shape: /^table_option\.prerequisite\.requires_option_keys(\[\])?$/,
    text: 'Escolha opções que existem, sem a própria opção, e no máximo 10.',
  },
  // An option of a class, subclass, race or subrace feature: the save refuses the asks of two options that wait for each other (each at
  // its own ask, with bad_value), an option that is gone (dangling_reference) and too many (limit).
  {
    shape: /^.+\.options\[\]\.prerequisite\.requires_option_keys(\[\])?$/,
    reason: 'bad_value',
    text: 'Esta opção e a que ela pede esperam uma pela outra: nenhuma das duas poderia ser escolhida. Tire um dos pedidos.',
  },
  {
    shape: /^.+\.options\[\]\.prerequisite\.requires_option_keys(\[\])?$/,
    reason: 'dangling_reference',
    text: 'Uma opção pedida aqui não existe mais. Tire-a ou escolha outra.',
  },
  {
    shape: /^.+\.options\[\]\.prerequisite\.requires_option_keys(\[\])?$/,
    reason: 'limit',
    text: 'Uma opção pede no máximo 10 outras.',
  },
  {
    shape: /^(table_(option|feat)|.+\.options\[\])\.prerequisite\.requires_pact$/,
    text: 'Escolha uma dádiva do pacto da lista.',
  },
  {
    shape: /^(table_(option|feat)|.+\.options\[\])\.prerequisite\.requires_spell_key$/,
    text: 'Esta magia não existe mais. Escolha outra.',
  },
];
