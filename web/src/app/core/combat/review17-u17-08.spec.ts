import { create } from '@bufbuild/protobuf';

import { SpellTargetsSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { targetRule } from './cast-flow';

// Finding U17-8 (review/unit-17-contract.md)
describe('Review17 U17-8: the cast flow ignores targets_per_level', () => {
  it('a 3-target spell taking 2 more per slot level allows 5 targets one level up', () => {
    // What the server sends: max_targets 3, extra_target_per_level true, targets_per_level 2.
    const st = create(SpellTargetsSchema, {
      spellKey: 'spell:table',
      maxTargets: 3,
      extraTargetPerLevel: true,
      targetsPerLevel: 2,
    });
    const spellLevel = 2;
    const slotLevel = 3;
    const contractMax = st.maxTargets + st.targetsPerLevel * (slotLevel - spellLevel); // 5
    expect(targetRule(st, 'me', spellLevel, slotLevel).max).toBe(contractMax);
  });
});
