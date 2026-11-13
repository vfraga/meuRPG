import { create } from '@bufbuild/protobuf';

import {
  EffectMenuListSchema,
  EffectMenuTypeSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { menuResponse } from './content-testing-menu';
import { EffectMenuVm } from './effect-draft';

/**
 * The menu with the real one's cost, healing and hook fields (backend/internal/rules/tablemenu.go): the action with its
 * cost, cost_max, heal, ability, temp_hp and value_per_point, the on_event hook, the resource with the recharge list and the
 * event list. Invented names only; the hints are the server's.
 */
export function costsMenu(): EffectMenuVm {
  const res = menuResponse();
  const action = res.types.find((t) => t.type === 'grant_action')!;
  action.fields = [
    { name: 'economy', kind: 'choice', required: true, list: 'economies' },
    { name: 'resource', kind: 'resource_ref' },
    { name: 'cost', kind: 'formula' },
    { name: 'cost_max', kind: 'formula' },
    { name: 'heal', kind: 'dice' },
    { name: 'ability', kind: 'choice', list: 'abilities' },
    { name: 'temp_hp', kind: 'formula' },
    { name: 'value_per_point', kind: 'formula' },
    { name: 'when', kind: 'condition' },
    { name: 'text_pt', kind: 'text' },
  ] as never;
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'resource',
      namePt: 'Recurso',
      hintPt: 'Uma reserva de usos.',
      fields: [
        { name: 'resource', kind: 'text', required: true },
        { name: 'max', kind: 'formula', required: true },
        { name: 'recharge', kind: 'choice', required: true, list: 'recharges' },
      ],
    }),
    create(EffectMenuTypeSchema, {
      type: 'on_event',
      namePt: 'Gatilho',
      hintPt: 'Algo que acontece com o dono quando um evento ocorre.',
      fields: [
        { name: 'event', kind: 'choice', required: true, list: 'events' },
        { name: 'resource', kind: 'resource_ref' },
        { name: 'cost', kind: 'formula' },
        { name: 'heal', kind: 'dice' },
        { name: 'ability', kind: 'choice', list: 'abilities' },
        { name: 'heal_percent_max', kind: 'number', min: 1, max: 100 },
        { name: 'temp_hp', kind: 'formula' },
        { name: 'restore', kind: 'resource_ref' },
        { name: 'value', kind: 'formula' },
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
  );
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'recharges',
      values: [
        { key: 'short_rest', namePt: 'Descanso curto' },
        { key: 'long_rest', namePt: 'Descanso longo' },
        {
          key: 'initiative',
          namePt: 'Ao rolar iniciativa',
          hintPt: 'Volta um uso quando o personagem rola iniciativa, mas só se não restar nenhum.',
        },
        {
          key: 'restore_one_on_short_rest',
          namePt: 'Um uso por descanso curto',
          hintPt: 'Um descanso curto devolve um uso; um descanso longo devolve todos.',
        },
        {
          key: 'restore_one_on_long_rest',
          namePt: 'Um uso por descanso longo',
          hintPt: 'Um descanso longo devolve um uso; um descanso curto não devolve nenhum.',
        },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'events',
      values: [
        {
          key: 'short_rest',
          namePt: 'Ao terminar um descanso curto',
          hintPt: 'Quando o dono termina um descanso curto.',
        },
        {
          key: 'long_rest',
          namePt: 'Ao terminar um descanso longo',
          hintPt: 'Quando o dono termina um descanso longo.',
        },
        {
          key: 'death_save_failed',
          namePt: 'Ao falhar num teste contra a morte',
          hintPt: 'Quando o dono falha num teste contra a morte.',
        },
        {
          key: 'drop_to_zero',
          namePt: 'Ao cair a 0 pontos de vida',
          hintPt: 'Quando o dono cai a 0 pontos de vida.',
        },
      ],
    }),
  );
  return new EffectMenuVm(res);
}

/** The menu with the real one's granted_spell and breathes_water effects, for their own specs. */
export function grantedSpellMenu(): EffectMenuVm {
  const res = menuResponse();
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'granted_spell',
      namePt: 'Magia concedida',
      hintPt: 'Uma magia que a entrada dá ao personagem fora das listas das classes dele.',
      fields: [
        { name: 'spell', kind: 'choice', required: true },
        { name: 'ability', kind: 'choice', required: true, list: 'granted_spell_abilities' },
        { name: 'from_level', kind: 'formula' },
        { name: 'uses', kind: 'resource_ref' },
        { name: 'uses_cost', kind: 'number', min: 1, max: 20 },
        { name: 'also_with_slot', kind: 'yes_no', alwaysShown: true },
        { name: 'ritual_only', kind: 'yes_no', alwaysShown: true },
        { name: 'counts_as_known', kind: 'yes_no', alwaysShown: true },
        { name: 'no_material_components', kind: 'yes_no', alwaysShown: true },
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
    create(EffectMenuTypeSchema, {
      type: 'breathes_water',
      namePt: 'Respira na água',
      hintPt: "O personagem respira debaixo d'água.",
      fields: [
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
  );
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'granted_spell_abilities',
      values: [
        { key: 'int', namePt: 'Inteligência' },
        { key: 'wis', namePt: 'Sabedoria' },
        { key: 'cha', namePt: 'Carisma' },
        { key: 'choice', namePt: 'O jogador escolhe' },
      ],
    }),
  );
  return new EffectMenuVm(res);
}
