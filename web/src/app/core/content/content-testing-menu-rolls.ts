import { create } from '@bufbuild/protobuf';

import {
  EffectMenuListSchema,
  EffectMenuTypeSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { menuResponse } from './content-testing-menu';
import { EffectMenuVm } from './effect-draft';

/**
 * The menu with the real one's reroll, roll_minimum and critical-range modifier (backend/internal/rules/tablemenu.go and
 * rolleffects.go): the rolls list, the critical range target that takes only "set" from 15 to 20, its scopes, and the entry's
 * own "Recurso". Names are the server's.
 */
export function rollsMenu(): EffectMenuVm {
  const res = menuResponse();
  const modifier = res.types.find((t) => t.type === 'modifier')!;
  modifier.fields = [
    { name: 'target', kind: 'choice', required: true, list: 'modifier_targets' },
    { name: 'mode', kind: 'choice', required: true, list: 'modifier_modes' },
    { name: 'value', kind: 'formula', required: true },
    { name: 'applies', kind: 'choice', list: 'critical_scopes' },
    { name: 'when', kind: 'condition' },
    { name: 'text_pt', kind: 'text' },
  ] as never;
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'reroll',
      namePt: 'Rolar de novo',
      hintPt: 'Rola de novo um dado que caiu baixo. A nova rolagem vale, mesmo se for pior.',
      fields: [
        { name: 'rolls', kind: 'choices', required: true, list: 'roll_applies' },
        { name: 'at_most', kind: 'number', required: true, min: 1, max: 20 },
        { name: 'resource', kind: 'resource_ref' },
        { name: 'cost', kind: 'formula' },
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
    create(EffectMenuTypeSchema, {
      type: 'roll_minimum',
      namePt: 'Valor mínimo no dado',
      hintPt: 'Um dado que mostrar menos que o valor escolhido conta como esse valor.',
      fields: [
        { name: 'rolls', kind: 'choices', required: true, list: 'roll_applies' },
        { name: 'value', kind: 'text', required: true },
        { name: 'damage_type', kind: 'choice', list: 'damage_types' },
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
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
  );
  const targets = res.lists.find((l) => l.name === 'modifier_targets')!;
  targets.values.push({
    key: 'critical_range',
    namePt: 'Margem de acerto crítico',
    hintPt: 'O menor d20 natural que é acerto crítico.',
    onlyModes: ['set'],
    min: 15,
    max: 20,
  } as never);
  const modes = res.lists.find((l) => l.name === 'modifier_modes')!;
  modes.values.push({ key: 'set', namePt: 'Definir' } as never);
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'roll_applies',
      values: [
        { key: 'attack', namePt: 'Jogada de ataque (o d20)' },
        { key: 'check', namePt: 'Teste de habilidade (o d20)' },
        { key: 'save', namePt: 'Teste de resistência (o d20)' },
        { key: 'damage', namePt: 'Dado de dano (de arma ou magia)' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'critical_scopes',
      values: [
        { key: 'weapon', namePt: 'Ataques com arma', hintPt: 'Só os ataques com arma (o padrão).' },
        { key: 'spell', namePt: 'Ataques com magia', hintPt: 'Só as jogadas de ataque de magia.' },
        {
          key: 'both',
          namePt: 'Armas e magias',
          hintPt: 'Os ataques com arma e as jogadas de ataque de magia.',
        },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'damage_types',
      values: [
        { key: 'damage-type:cold', namePt: 'frio' },
        { key: 'damage-type:fire', namePt: 'fogo' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'recharges',
      values: [
        { key: 'short_rest', namePt: 'Descanso curto' },
        { key: 'long_rest', namePt: 'Descanso longo' },
      ],
    }),
  );
  return new EffectMenuVm(res);
}
