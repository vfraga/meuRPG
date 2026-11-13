/** The labels of the effect fields, in our words (the menu names the fields by their proto name). */
export const EFFECT_FIELD_LABELS: Readonly<Record<string, string>> = {
  target: 'O que muda',
  mode: 'Como muda',
  value: 'Valor',
  when: 'Só quando',
  tags: 'Situação',
  proficiency: 'Proficiência em',
  level: 'Quanto',
  roll: 'Rolagem',
  targets: 'Em quê',
  sense: 'Sentido',
  range_ft: 'Alcance',
  resource: 'Nome do recurso',
  max: 'Usos (máximo)',
  recharge: 'Volta em',
  choice: 'O jogador escolhe',
  count: 'Quantos',
  from: 'Entre',
  economy: 'Tipo de ação',
  applies: 'Onde o dado entra',
  ability: 'Habilidade somada',
  spells: 'Magias',
  text_pt: 'Texto na ficha',
  except: 'Todos os tipos de dano, menos',
  while: 'Só enquanto o estado',
  kind: 'O que acontece',
  damage_types: 'Tipos de dano',
  conditions: 'Condições',
  against_condition: 'Só nos testes de resistência contra a condição',
  state_name: 'Nome do estado',
  duration_rounds: 'Duração (rodadas)',
  ends_on: 'Também acaba',
  weapon_name: 'Nome da arma',
  attack_ability: 'Habilidade do ataque',
  damage_ability: 'Habilidade do dano',
  attack_bonus: 'Bônus no ataque',
  damage_bonus: 'Bônus no dano',
  reach_ft: 'Alcance corpo a corpo',
  magical: 'Arma mágica',
  silvered: 'Arma de prata',
  adamantine: 'Arma de adamantina',
  attack_name: 'Nome do ataque',
  damage_dice: 'Dados de dano',
  cost: 'Custo (usos)',
  cost_max: 'Custo máximo (o jogador escolhe)',
  value_per_point: 'Por ponto gasto a mais',
  heal: 'Cura (dados)',
  temp_hp: 'Pontos de vida temporários',
  event: 'Quando acontece',
  restore: 'Devolve usos de',
  heal_percent_max: 'Cura (% do máximo)',
  ignore_cover: 'Ignora meia cobertura e três quartos',
  range_multiplier: 'Multiplica o alcance da magia por',
  no_long_range_disadvantage: 'Sem desvantagem no alcance longo',
  spell: 'Magia',
  from_level: 'A partir do nível',
  uses: 'Cada conjuração gasta um uso de',
  also_with_slot: 'Também com espaço de magia',
  ritual_only: 'Só como ritual',
  counts_as_known: 'Conta como magia conhecida',
  no_material_components: 'Sem componentes materiais',
  uses_cost: 'Usos gastos por conjuração',
  only_proficient: 'Só onde já tem proficiência',
  dice: 'Dados (quantos)',
  dice_sides: 'Lados do dado',
  damage_type: 'Tipo de dano',
  weapon: 'Arma',
  frequency: 'Quantas vezes',
  no_crit_doubling: 'Não dobra no acerto crítico',
  applies_to: 'Vale para',
  spell_change: 'O que muda na magia',
  rolls: 'Vale nas jogadas',
  at_most: 'Rola de novo se tirar até',
  cap: 'Limite',
};

/** One line under a field, where the name alone does not say. This is screen copy; the formula functions and the tag
 * prefixes it mentions come from the server's menu (`EffectMenuVm.formulaHint`, `tagHint`). */
export const EFFECT_FIELD_HINTS: Readonly<Record<string, string>> = {
  resource: 'Só letras minúsculas sem acento, números e _ (de 1 a 40).',
  state_name:
    'Só letras minúsculas sem acento, números e _ (de 1 a 40). Os outros efeitos da entrada usam este nome em "Só enquanto o estado" e em active("nome").',
  duration_rounds: 'Em rodadas (1 minuto são 10). Vazio ou 0: dura até acabar.',
  weapon_name:
    'O nome da arma em português, como na ficha (sem diferença de maiúsculas e acentos).',
  attack_bonus: 'De -5 a 10, somado à jogada de ataque.',
  damage_bonus: 'De -5 a 10, somado ao dano.',
  reach_ft: 'Em metros, de 1,5 em 1,5; só para armas de corpo a corpo.',
  magical:
    'Os ataques das armas que o filtro escolhe contam como mágicos contra as resistências a ataques não mágicos.',
  silvered:
    'Os ataques das armas que o filtro escolhe contam como de prata contra as resistências a ataques não mágicos.',
  adamantine:
    'Os ataques das armas que o filtro escolhe contam como de adamantina contra as resistências a ataques não mágicos.',
  attack_name: 'O nome do ataque na lista de ataques da ficha.',
  damage_dice: 'Dados simples, como 1d6 ou 2d8 (d4, d6, d8, d10 ou d12; até 10 dados).',
  cost: 'Quantos usos cada vez gasta; vazio é 1. Não passa do máximo do recurso.',
  cost_max:
    'Com ele, o jogador escolhe na hora quantos gastar, do custo até este valor (ou o que restar).',
  value_per_point:
    'Soma a cada ponto gasto acima do custo: dados na cura ("1d6") ou uma fórmula nos pontos temporários.',
  heal: 'Dados, como 1d6 ou 2d4+2. Nunca passa do máximo, e quem está morto não é curado.',
  temp_hp: 'Não se acumulam: vale o maior entre o que o alvo tinha e este valor.',
  restore:
    'O nome de um recurso desta entrada; devolve só o que foi gasto, então o mesmo evento duas vezes devolve uma vez.',
  heal_percent_max: 'De 1 a 100, arredondado para baixo, no lugar dos dados.',
  text_pt: 'O que a ficha mostra. {value} vira o valor com sinal.',
  ignore_cover: 'O alvo não ganha o bônus na CA. A cobertura total continua impedindo o ataque.',
  range_multiplier: 'Só para magias: 2 dobra o alcance, 3 triplica. Vazio, o alcance não muda.',
  no_long_range_disadvantage: 'Só para armas: até o alcance longo, a rolagem não tem desvantagem.',
  damage_types:
    'Escolha os tipos, ou "Todos os tipos de dano" sozinho. Use este campo ou "Todos os tipos de dano, menos".',
  except:
    'Em vez de listar os tipos: vale para todos menos este. Deixe vazio se escolheu os tipos acima.',
  while: 'Vazio, vale sempre. Em fúria, só enquanto o personagem estiver em fúria.',
  against_condition:
    'Vazio, a vantagem ou desvantagem fica como lembrete da situação. Com uma condição, o app aplica nos testes de resistência para não ficar com ela.',
  only_proficient:
    'Dobra a perícia só de quem já é proficiente nela por outra fonte. Quem não é, fica como está.',
  from_level:
    'Um número ou uma fórmula. Vazio: desde o 1º nível. Vale o nível da classe numa característica de classe ou de subclasse, o nível do personagem no resto.',
  uses: 'O nome de um recurso que esta entrada define (um efeito Recurso). Vazio: à vontade, sem limite.',
  also_with_slot: 'O personagem também pode conjurar com um espaço de magia, sem gastar usos.',
  ritual_only:
    'Só conjura como ritual (10 minutos a mais, sem espaço). Pede uma magia com a etiqueta de ritual.',
  dice: 'Uma fórmula, como 1 ou prof(). Vale junto com os lados do dado.',
  dice_sides: 'De 4 a 12: d4, d6, d8, d10 ou d12.',
  no_crit_doubling: 'Por padrão os dados dobram no acerto crítico; o dano fixo nunca dobra.',
  no_material_components:
    'A ficha e a folha de conjuração mostram a magia sem o componente material.',
  uses_cost: 'Um número inteiro, de 1 a 20. Vazio: 1 uso por conjuração.',
  counts_as_known:
    'A magia aparece também entre as conhecidas da ficha, sem gastar nenhuma escolha da classe.',
  at_most: 'De 1 a 20. Um dado com este valor ou menos pode ser rolado de novo, uma vez.',
};

/** The label or hint of a field where one type reads it differently from the rest (`ability` is a maneuver's modifier, and a granted spell's casting ability). */
export const EFFECT_TYPE_FIELD_LABELS: Readonly<Record<string, Readonly<Record<string, string>>>> =
  {
    granted_spell: { ability: 'Habilidade de conjuração' },
    extra_damage: { value: 'Dano fixo', applies: 'Contra quem' },
    resistance: { while: 'Enquanto' },
    spell_modifier: {
      from: 'Tipos de dano que troca',
      value: 'Dano fixo somado',
      count: 'Dados a mais por nível acima',
      damage_type: 'Tipo do dano somado',
    },
    modifier: { applies: 'Vale para' },
    roll_minimum: { value: 'Valor mínimo' },
    // A state's resource is spent once at each start; a pseudo-weapon names its kind, its ability and its reach in the words of an attack.
    state: { resource: 'Gasta um uso de' },
    pseudo_weapon: {
      weapon: 'Tipo de ataque',
      ability: 'Habilidade do ataque',
      reach_ft: 'Alcance de contato',
      range_ft: 'Alcance (à distância)',
    },
  };

/**
 * One line for each effect type, for the feat editor, where the menu's full text (several lines) would push the fields apart.
 * A type with no line here shows the menu's own.
 */
/** The line under a formula in the feat editor, where the menu's list of functions would run several lines. */
const FEAT_FORMULA_HINT = 'Um número ou uma fórmula, como 8 + prof().';

const SHORT_TYPE_HINTS: Readonly<Record<string, string>> = {
  modifier: 'Soma ou define um número da ficha.',
  proficiency: 'Dá proficiência numa perícia ou teste.',
  resource: 'Usos que voltam num descanso.',
  sense: 'Dá visão no escuro ou outro sentido.',
  roll_mode: 'Vantagem ou desvantagem em certas rolagens.',
  grant_action: 'Uma ação nova na vez do personagem.',
  superiority_die: 'Um dado que soma a uma rolagem.',
  extra_attack: 'Quantos ataques a ação Atacar faz.',
  extra_damage: 'Dano somado a um ataque que acertou.',
  ambush: 'Vantagem contra quem é pego de surpresa.',
  wild_shape: 'Muda a Forma Selvagem do druida.',
  slot_recovery: 'Recupera espaços de magia no descanso.',
  ranged_attack: 'Muda os ataques de longe do personagem.',
  resistance: 'Resistência ou imunidade a dano.',
  state: 'Algo que o personagem liga por um tempo.',
  weapon_rule: 'Muda as armas que o filtro escolhe.',
  pseudo_weapon: 'Dá à ficha um ataque próprio.',
  condition_immunity: 'Não fica com as condições escolhidas.',
  breathes_water: 'O personagem respira debaixo d’água.',
  granted_spell: 'Dá uma magia ao personagem.',
  on_event: 'Algo que ocorre com o dono num evento.',
  spell_modifier: 'Muda o dano de certas magias.',
  reroll: 'Rola de novo um dado que caiu baixo.',
  roll_minimum: 'Um dado baixo conta como o valor.',
  choice: 'Algo que o jogador escolhe uma vez.',
  ability_increase: 'O jogador soma um valor a habilidades.',
  note: 'Um lembrete na ficha.',
};

export const EFFECT_TYPE_FIELD_HINTS: Readonly<Record<string, Readonly<Record<string, string>>>> = {
  roll_minimum: {
    value: 'De 2 a 20. Um dado que mostrar menos conta como este valor.',
    damage_type:
      'Opcional, só com o dado de dano sozinho: o mínimo vale só nos dados deste tipo. "Qualquer tipo": em todos.',
  },
  granted_spell: {
    ability:
      'Dá o bônus de ataque da magia (proficiência + modificador) e a CD de resistência (8 + o mesmo).',
  },
};

/** The line under "Vale nas jogadas" of each type that asks it. */
export const ROLLS_NOTES: Readonly<Record<string, string>> = {
  reroll:
    'No d20 o jogador responde "rolar de novo?" antes do resultado. No dado de dano o app rola de novo sozinho, sem perguntar, e um uso cobre a jogada inteira.',
  roll_minimum:
    'Um dado de dano menor que o valor conta como o seu maior valor: um d4 com mínimo 6 conta 4.',
};

/** What "Gasta usos de" says of a reroll. The server accepts a resource of the entry, of its class or of the SRD, so the app lists the
 * entry's own and never judges the rest: a name it does not list stays a text, and the refusal is the server's. */
export const RESOURCE_PICK_COPY = {
  none: 'Nenhum: vale sempre',
  hint: 'O recurso que esta entrada (ou a classe dela) define, no efeito Recurso. Vazio: vale sempre, sem gastar.',
  dangling: 'Esta entrada não define mais este recurso. Escolha outro, ou deixe vazio.',
} as const;

/** The critical range as a modifier target: the value's own words (the server names the target and says what it is), the lock under "Como
 * muda" and the note under "Vale para". */
export const CRITICAL_COPY = {
  value: 'Crítico a partir do d20',
  valueHint:
    'De 15 a 20, um número ou uma fórmula. Com 19, o 19 e o 20 são críticos. Vale a menor margem que o personagem tiver.',
  modeLock: 'A margem de crítico só aceita Definir.',
  scopeNote:
    'Só para a margem de crítico. Crítico Aprimorado e Crítico Superior nunca valem para magia.',
} as const;

/** What a refusal says of a field that other types share, where the effect says which type it is (the server's reason has no type): the
 * words replace the general ones (`GENERIC_EFFECT_TEXTS`). Keyed by "type.field", "modifier.critical_range.field" for the critical range. */
export const EFFECT_REFUSAL_TEXTS: Readonly<Record<string, string>> = {
  'roll_minimum.value': 'O valor mínimo vai de 2 a 20.',
  'roll_minimum.damage_type':
    'O tipo de dano só vale com o dado de dano sozinho. Desmarque as outras jogadas ou escolha "Qualquer tipo".',
  'reroll.cost': 'O custo é um número de 1 em diante, e pede um recurso.',
  'reroll.resource': RESOURCE_PICK_COPY.dangling,
  'modifier.critical_range.value': 'A margem vai de 15 a 20.',
  'modifier.critical_range.mode': CRITICAL_COPY.modeLock,
};

/** The hint of an effect type: the menu's, or in the feat editor the one line of `SHORT_TYPE_HINTS` when the type has one. */
export function typeHintFor(type: string, menuHint: string, inFeat: boolean): string {
  return (inFeat ? SHORT_TYPE_HINTS[type] : undefined) ?? menuHint;
}

/** The hint of a formula: the field's own sentence, else the menu's list of functions, or in the feat editor one line. */
export function formulaHintFor(own: string, menuHint: string, inFeat: boolean): string {
  return own || (inFeat ? FEAT_FORMULA_HINT : menuHint);
}
