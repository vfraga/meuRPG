/** The field texts of a race, a subrace and a background (their sizes, speeds, bonuses, languages, skills, tools and equipment);
 * `FIELD_TEXTS` spreads them at the end of its ordered list, where the first row that matches wins. */
export const RACE_FIELD_TEXTS: readonly { shape: RegExp; reason?: string; text: string }[] = [
  { shape: /\.size$/, text: 'Escolha o tamanho.' },
  { shape: /\.speed_ft$/, text: 'O deslocamento vai de 1,5 m a 36 m, em passos de 1,5 m.' },
  {
    shape: /\.darkvision_ft$/,
    text: 'A visão no escuro é 0 ou vai de 1,5 m a 36 m, em passos de 1,5 m.',
  },
  { shape: /\.ability_bonuses\.[a-z]+$/, text: 'O bônus vai de −4 a +4.' },
  { shape: /\.choice_bonuses$/, text: 'No máximo 6 bônus à escolha.' },
  { shape: /\.choice_bonuses\[\]$/, text: 'Cada bônus à escolha vai de +1 a +4.' },
  {
    shape: /\.choice_bonus_sets$/,
    text: 'No máximo 4 grupos de bônus, e a raça usa grupos ou uma lista só, nunca os dois.',
  },
  {
    shape: /\.choice_bonus_sets\[\]\.bonuses$/,
    text: 'Cada grupo tem de 1 a 6 bônus, e dois grupos não podem ser iguais.',
  },
  { shape: /\.choice_bonus_sets\[\]\.bonuses\[\]$/, text: 'Cada bônus de um grupo é +1 ou +2.' },
  { shape: /\.sizes$/, text: 'O tamanho da raça precisa estar entre os que o jogador escolhe.' },
  { shape: /\.sizes\[\]$/, text: 'Escolha Pequeno ou Médio, cada um uma vez.' },
  { shape: /\.language_choices$/, text: 'Idiomas à escolha: de 0 a 4.' },
  { shape: /\.languages\[\]$/, text: 'Este idioma não existe. Escolha da lista.' },
  {
    shape: /\.race_key$/,
    text: 'A raça desta sub-raça não existe mais. Crie a sub-raça numa raça que exista.',
  },
  { shape: /\.skills$/, text: 'Escolha duas perícias diferentes.' },
  {
    shape: /\.skills\[\]$/,
    reason: 'dangling_reference',
    text: 'Esta perícia não existe. Escolha da lista.',
  },
  { shape: /\.skills\[\]$/, text: 'Escolha duas perícias diferentes.' },
  { shape: /\.tools$/, text: 'No máximo 4 ferramentas.' },
  { shape: /\.tools\[\]$/, text: 'Esta não é uma ferramenta do SRD. Escolha da lista.' },
  { shape: /\.equipment_pt$/, text: 'O equipamento é longo demais. Encurte o texto.' },
];
