/** The text of the swap levels of a feature that cannot allow swaps (`…swap_levels`): it offers the options of another feature
 * (`from_feature`), or has no options of its own. `FIELD_TEXTS` spreads it at the end of its ordered list. */
export const SWAP_FIELD_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  {
    shape: /\.swap_levels$/,
    reason: 'bad_value',
    text: 'Uma característica que oferece as opções de outra, ou que não tem opções, não pode permitir trocas. Tire as trocas ou escolha opções próprias.',
  },
];
