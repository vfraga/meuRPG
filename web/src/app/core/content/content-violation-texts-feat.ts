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
