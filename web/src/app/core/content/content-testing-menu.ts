import { create } from '@bufbuild/protobuf';

import {
  EffectMenuListSchema,
  EffectMenuTypeSchema,
  type GetEffectMenuResponse,
  GetEffectMenuResponseSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { EffectMenuVm } from './effect-draft';

/** The lowest and highest cap a score modifier may write (backend MaxNormalScore to its ceiling). */
const SCORE_CAP_MIN = 20;
const SCORE_CAP_MAX = 30;

/** The menu: the real one's types and field order (backend/internal/rules/tablemenu.go), with a few of its lists. */
export function menuResponse(): GetEffectMenuResponse {
  const field = (name: string, kind: string, required = false, list = '', min = 0, max = 0) => ({
    name,
    kind,
    required,
    list,
    min,
    max,
  });
  return create(GetEffectMenuResponseSchema, {
    types: [
      {
        type: 'modifier',
        namePt: 'Modificador',
        hintPt: 'Soma, define ou limita um número da ficha.',
        fields: [
          field('target', 'choice', true, 'modifier_targets'),
          field('mode', 'choice', true, 'modifier_modes'),
          field('value', 'formula', true),
          field('cap', 'number', false, '', SCORE_CAP_MIN, SCORE_CAP_MAX),
          field('when', 'condition'),
          field('tags', 'tags', false, 'tag_prefixes'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'proficiency',
        namePt: 'Proficiência',
        hintPt: 'Dá proficiência.',
        fields: [
          field('proficiency', 'choice', true, 'proficiency_targets'),
          field('level', 'choice', false, 'proficiency_levels'),
          field('only_proficient', 'yes_no'),
          field('when', 'condition'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'sense',
        namePt: 'Sentido',
        hintPt: 'Visão no escuro.',
        fields: [
          field('sense', 'choice', true, 'senses'),
          field('range_ft', 'number', true, '', 1),
          field('when', 'condition'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'grant_action',
        namePt: 'Ação',
        hintPt: 'Uma ação que o personagem passa a ter.',
        fields: [
          field('economy', 'choice', true, 'economies'),
          field('resource', 'resource_ref'),
          field('when', 'condition'),
          field('tags', 'tags', false, 'tag_prefixes'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'superiority_die',
        namePt: 'Dado de manobra',
        hintPt: 'Um dado que uma opção soma a uma rolagem.',
        fields: [
          field('applies', 'choice', true, 'maneuver_applies'),
          field('resource', 'resource_ref', true),
          field('value', 'formula', true),
          field('ability', 'choice', false, 'abilities'),
          field('targets', 'choices', false, 'roll_die_targets'),
          field('economy', 'choice', false, 'economies'),
          field('when', 'condition'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'choice',
        namePt: 'Escolha',
        hintPt: 'Algo que o jogador escolhe.',
        fields: [
          field('choice', 'choice', true, 'choice_kinds'),
          field('count', 'number', true, '', 1),
          field('name', 'text'),
          field('set', 'text'),
          field('from', 'choices'),
          field('when', 'condition'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'ability_increase',
        namePt: 'Aumento de habilidade',
        hintPt: 'O jogador escolhe habilidades de uma lista.',
        featOnly: true,
        fields: [
          field('count', 'number', true, '', 1),
          field('from', 'choices', true, 'abilities'),
          field('value', 'choice', true, 'ability_increase_amounts'),
          field('text_pt', 'text'),
        ],
      },
      {
        type: 'note',
        namePt: 'Nota e magia concedida',
        hintPt: 'Um lembrete.',
        fields: [
          field('text_pt', 'text'),
          field('value', 'formula'),
          field('spells', 'choices'),
          field('tags', 'tags', false, 'tag_prefixes'),
          field('when', 'condition'),
        ],
      },
    ],
    lists: [
      {
        name: 'modifier_targets',
        values: [
          { key: 'speed.walk', namePt: 'Deslocamento' },
          { key: 'ac', namePt: 'Classe de Armadura' },
        ],
      },
      { name: 'modifier_modes', values: [{ key: 'add', namePt: 'Somar' }] },
      {
        name: 'proficiency_targets',
        values: [
          { key: 'skill:perception', namePt: 'Percepção' },
          { key: 'save.wis', namePt: 'Teste de Sabedoria' },
        ],
      },
      {
        name: 'proficiency_levels',
        values: [
          { key: 'half', namePt: 'Metade' },
          { key: 'full', namePt: 'Completa' },
          { key: 'expertise', namePt: 'Especialização' },
        ],
      },
      { name: 'senses', values: [{ key: 'darkvision', namePt: 'Visão no escuro' }] },
      {
        name: 'choice_kinds',
        values: [
          { key: 'skill', namePt: 'Perícias' },
          { key: 'feature', namePt: 'Uma opção de uma lista' },
          { key: 'feat', namePt: 'Talentos' },
        ],
      },
      {
        name: 'abilities',
        values: [
          { key: 'strength', namePt: 'Força' },
          { key: 'dexterity', namePt: 'Destreza' },
          { key: 'constitution', namePt: 'Constituição' },
        ],
      },
      {
        name: 'ability_increase_amounts',
        values: [
          { key: '1', namePt: '+1' },
          { key: '2', namePt: '+2' },
        ],
      },
      {
        name: 'skills',
        values: [
          { key: 'skill:arcana', namePt: 'Arcanismo' },
          { key: 'skill:perception', namePt: 'Percepção' },
        ],
      },
      {
        name: 'languages',
        values: [
          { key: 'language:common', namePt: 'Comum' },
          { key: 'language:primordial', namePt: 'Primordial' },
        ],
      },
      {
        name: 'feat_option_sets',
        values: [
          { key: 'invocation', namePt: 'Invocação mística' },
          {
            key: 'fighting_style',
            namePt: 'Estilo de luta',
            hintPt: 'Os estilos de luta do guerreiro, do paladino e do patrulheiro.',
          },
        ],
      },
      {
        name: 'option_set_names',
        values: [
          {
            key: 'invocation',
            namePt: 'Invocação mística',
            hintPt: 'Uma opção da Invocação mística do bruxo.',
          },
          { key: 'fighting_style', namePt: 'Estilo de luta' },
          {
            key: 'table_feature',
            namePt: 'Opção de característica da mesa',
            hintPt: 'As opções de uma característica de classe ou subclasse da mesa.',
          },
        ],
      },
      {
        name: 'tools',
        values: [{ key: 'proficiency:cartographers-tools', namePt: 'Ferramentas de cartógrafo' }],
      },
      {
        name: 'tag_prefixes',
        values: [
          { key: 'against:', namePt: 'Contra…' },
          { key: 'about:', namePt: 'Sobre…' },
        ],
      },
      {
        name: 'lackable_features',
        values: [
          { key: 'feat:grappler', namePt: 'Agarrador', hintPt: 'talento do SRD' },
          { key: 'feature:monk-grip', namePt: 'Agarrar com firmeza', hintPt: 'Monge, nível 6' },
          { key: 'trait:darkvision', namePt: 'Visão no Escuro', hintPt: 'traço do SRD' },
        ],
      },
      {
        name: 'race_traits',
        values: [
          {
            key: 'trait:keen-senses',
            namePt: 'Sentidos Aguçados',
            hintPt: 'Você tem proficiência na perícia Percepção.',
            group: 'race:elf',
          },
          {
            key: 'trait:trance',
            namePt: 'Transe',
            hintPt: 'Elfos não precisam dormir.',
            group: 'race:elf',
          },
          { key: 'trait:lucky', namePt: 'Sortudo', hintPt: '', group: 'race:halfling' },
        ],
      },
    ],
    helpers: [
      { call: 'mod("<habilidade>")', returns: 'number', hintPt: 'O modificador.' },
      { call: 'prof()', returns: 'number', hintPt: 'O bônus de proficiência.' },
    ],
    optionSets: [
      {
        key: 'feature:fighting-style',
        namePt: 'Estilo de luta',
        choose: 1,
        options: [{ key: 'option:archery', namePt: 'Arquearia' }],
      },
      {
        key: 'feature:pact-boon',
        namePt: 'Dádiva do pacto',
        choose: 1,
        options: [{ key: 'feature:pact-of-the-tome', namePt: 'Dádiva do pacto: Pacto do Tomo' }],
      },
    ],
    classIndexes: ['wizard', 'cleric'],
    maxFeaturesPerClass: 60,
    maxEffectsPerFeature: 4,
    maxTagsPerEffect: 4,
    extraAttackMin: 2,
    extraAttackMax: 4,
  });
}

export function menu(): EffectMenuVm {
  return new EffectMenuVm(menuResponse());
}

/** The menu with the real one's resistance effect (damage types or every type but one, its kind as cards), for its own specs. */
export function resistanceMenu(): EffectMenuVm {
  const res = menuResponse();
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'resistance',
      namePt: 'Resistência, imunidade ou vulnerabilidade a dano',
      hintPt: 'Resistência, imunidade ou vulnerabilidade aos tipos de dano escolhidos.',
      fields: [
        { name: 'kind', kind: 'cards', list: 'resistance_kinds', alwaysShown: true },
        {
          name: 'damage_types',
          kind: 'choices',
          list: 'resistance_damage_types',
          alwaysShown: true,
        },
        { name: 'except', kind: 'choice', list: 'damage_types' },
        { name: 'while', kind: 'state_ref', list: 'resistance_conditions' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
  );
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'damage_types',
      values: [{ key: 'damage-type:psychic', namePt: 'Psíquico' }],
    }),
    create(EffectMenuListSchema, {
      name: 'resistance_damage_types',
      values: [
        { key: 'all', namePt: 'Todos os tipos de dano' },
        {
          key: 'nonmagical_bps',
          namePt: 'Contundente, perfurante e cortante não mágicos',
          hintPt: 'Pancada, perfuração e corte de ataques que não são mágicos.',
        },
        { key: 'damage-type:cold', namePt: 'Frio' },
        { key: 'damage-type:poison', namePt: 'Veneno' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'resistance_kinds',
      values: [
        { key: 'resist', namePt: 'Resistência', hintPt: 'Sofre metade do dano.' },
        { key: 'immune', namePt: 'Imunidade', hintPt: 'Não sofre dano.' },
        { key: 'vulnerable', namePt: 'Vulnerabilidade', hintPt: 'Sofre o dobro do dano.' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'resistance_conditions',
      values: [{ key: 'rage', namePt: 'Em fúria' }],
    }),
  );
  return new EffectMenuVm(res);
}

/** The menu with the real one's condition_immunity and the roll mode against a condition, for their own specs. */
export function conditionMenu(): EffectMenuVm {
  const res = menuResponse();
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'condition_immunity',
      namePt: 'Imunidade a condição',
      hintPt: 'O personagem não fica com as condições escolhidas.',
      fields: [
        { name: 'conditions', kind: 'choices', required: true, list: 'conditions' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
    create(EffectMenuTypeSchema, {
      type: 'roll_mode',
      namePt: 'Vantagem ou desvantagem',
      hintPt: 'Vantagem ou desvantagem em certas rolagens.',
      fields: [
        { name: 'roll', kind: 'choice', required: true, list: 'roll_modes' },
        { name: 'targets', kind: 'choices', required: true, list: 'roll_targets' },
        { name: 'against_condition', kind: 'choice', list: 'conditions' },
        { name: 'tags', kind: 'tags', list: 'tag_prefixes' },
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
  );
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'conditions',
      values: [
        { key: 'condition:frightened', namePt: 'Amedrontado' },
        { key: 'condition:poisoned', namePt: 'Envenenado' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'roll_modes',
      values: [
        { key: 'advantage', namePt: 'Vantagem' },
        { key: 'disadvantage', namePt: 'Desvantagem' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'roll_targets',
      values: [
        { key: 'save.wis', namePt: 'Teste de resistência de Sabedoria' },
        { key: 'save.all', namePt: 'Todos os testes de resistência' },
      ],
    }),
  );
  return new EffectMenuVm(res);
}
