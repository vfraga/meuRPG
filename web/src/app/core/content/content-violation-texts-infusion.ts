/**
 * What the server's refusals say at the infusion data of a class or subclass feature and of a table option.
 * The paths and reasons are those of `rules/overlay_infusion.go`; the editor has no screen for the data yet, so these
 * sentences are what an import or another client's body shows.
 * `FIELD_TEXTS` spreads these rows, where the first row that matches wins.
 */
const LEVELS_TEXT = 'Os níveis dos passos sobem, cada um de 1 a 20.';

export const INFUSION_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  {
    shape: /\.infusion$/,
    text: 'Uma infusão é uma opção de uma característica que tem regra de infusões. Ponha a regra na característica ou tire a infusão.',
  },
  {
    shape: /\.infusion_rule$/,
    text: 'Só uma característica de classe ou subclasse que tem opções pode ter regra de infusões.',
  },
  {
    shape: /\.infusion_rule\.limit_steps(\[\])?$/,
    text: 'A regra precisa de pelo menos um passo.',
  },
  { shape: /\.infusion_rule\.limit_steps\[\]\.class_level$/, text: LEVELS_TEXT },
  {
    shape: /\.infusion_rule\.limit_steps\[\]\.active$/,
    text: 'Cada passo permite pelo menos 1 infusão viva.',
  },
  {
    shape: /\.infusion\.accepts$/,
    text: 'Escolha o tipo de peça. O efeito escolhido só vale em certos tipos: munição repetida pede arma de disparo, retorno pede arma de arremesso, foco pede foco de conjuração e resistência pede armadura. O bônus de item vale em arma, armadura, escudo, armadura ou escudo, arma de arremesso e arma de disparo.',
  },
  {
    shape: /\.infusion\.effect$/,
    text: 'Escolha o que o app calcula, ou nenhum efeito para uma infusão só de texto.',
  },
  {
    shape: /\.infusion\.bonus_steps(\[\])?$/,
    text: 'Um bônus de item ou de foco precisa de passos por nível. Os outros efeitos não têm passos.',
  },
  { shape: /\.infusion\.bonus_steps\[\]\.class_level$/, text: LEVELS_TEXT },
  { shape: /\.infusion\.bonus_steps\[\]\.bonus$/, text: 'O bônus vai de +1 a +3.' },
  {
    shape: /\.infusion\.damage_types\[\]$/,
    reason: 'dangling_reference',
    text: 'Este tipo de dano não existe. Escolha outro.',
  },
  {
    shape: /\.infusion\.damage_types(\[\])?$/,
    text: 'Uma infusão de resistência lista os tipos de dano que quem a recebe escolhe, cada um uma vez. Os outros efeitos não têm tipos de dano.',
  },
];
