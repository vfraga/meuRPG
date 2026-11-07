import { create } from '@bufbuild/protobuf';

import {
  EffectMenuListSchema,
  EffectMenuTypeSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { EffectMenuVm } from './effect-draft';
import { menuResponse } from './content-testing-menu';

/** The menu with the real one's ranged_attack (a table feat's switches and a spell range multiplier), for its own specs. */
export function rangedAttackMenu(): EffectMenuVm {
  const res = menuResponse();
  res.types.push(
    create(EffectMenuTypeSchema, {
      type: 'ranged_attack',
      namePt: 'Ataque à distância',
      hintPt: 'Muda os ataques de longe do personagem.',
      fields: [
        { name: 'applies', kind: 'choice', required: true, list: 'ranged_attack_applies' },
        { name: 'ignore_cover', kind: 'yes_no', alwaysShown: true },
        { name: 'range_multiplier', kind: 'number', min: 2, max: 3, alwaysShown: true },
        { name: 'no_long_range_disadvantage', kind: 'yes_no', alwaysShown: true },
        { name: 'when', kind: 'condition' },
        { name: 'text_pt', kind: 'text' },
      ],
    }),
  );
  res.lists.push(
    create(EffectMenuListSchema, {
      name: 'ranged_attack_applies',
      values: [
        { key: 'spell', namePt: 'Magias com jogada de ataque' },
        { key: 'weapon', namePt: 'Ataques com arma à distância' },
      ],
    }),
  );
  return new EffectMenuVm(res);
}
