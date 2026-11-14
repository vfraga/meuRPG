import { create } from '@bufbuild/protobuf';

import {
  EffectMenuListSchema,
  EffectMenuTypeSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { EffectMenuVm } from './effect-draft';
import { menuResponse } from './content-testing-menu';

/** The menu with the real one's power_attack (backend/internal/rules/tablemenu.go), its weapon kinds and the SRD weapon properties,
 * for its own specs. */
export function powerAttackMenu(): EffectMenuVm {
  const res = menuResponse();
  res.types.push(
    // The pool a power attack may spend, as the real menu has it.
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
      type: 'power_attack',
      namePt: 'Ataque poderoso',
      hintPt:
        'Uma troca que o jogador faz ao atacar com arma: um número tirado da jogada de ataque.',
      fields: [
        { name: 'attack_penalty', kind: 'number', required: true, min: 1, max: 10 },
        { name: 'damage_bonus', kind: 'number', required: true, min: 1, max: 20 },
        { name: 'weapon', kind: 'choice', list: 'extra_damage_weapons', alwaysShown: true },
        {
          name: 'weapon_properties',
          kind: 'choices',
          list: 'weapon_properties',
          alwaysShown: true,
        },
        { name: 'only_proficient', kind: 'yes_no', alwaysShown: true },
        { name: 'resource', kind: 'resource_ref' },
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
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'extra_damage_weapons',
      values: [
        { key: 'any', namePt: 'Qualquer ataque com arma' },
        { key: 'melee', namePt: 'Ataque com arma corpo a corpo' },
        { key: 'ranged', namePt: 'Ataque com arma à distância' },
      ],
    }),
    create(EffectMenuListSchema, {
      name: 'weapon_properties',
      values: [
        { key: 'weapon-property:ammunition', namePt: 'Munição' },
        { key: 'weapon-property:finesse', namePt: 'Acuidade' },
        { key: 'weapon-property:heavy', namePt: 'Pesada' },
        { key: 'weapon-property:light', namePt: 'Leve' },
        { key: 'weapon-property:reach', namePt: 'Alcance' },
        { key: 'weapon-property:two-handed', namePt: 'Duas mãos' },
        { key: 'weapon-property:versatile', namePt: 'Versátil' },
      ],
    }),
  );
  return new EffectMenuVm(res);
}
