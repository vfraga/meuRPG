import { create } from '@bufbuild/protobuf';

import {
  CombatLogEntrySchema,
  CombatLogKind,
  CombatLogReadiedSchema,
  ReadiedKind,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { CombatNotices } from './combat-notices';

const lost = (id: string) =>
  create(CombatLogEntrySchema, {
    id,
    kind: CombatLogKind.READIED_LOST,
    actorId: 'tor',
    readiedLost: create(CombatLogReadiedSchema, {
      readiedId: 'r1',
      kind: ReadiedKind.ATTACK,
      actionNamePt: 'Espada longa',
    }),
  });

describe('CombatNotices, the readied action that was lost (#582)', () => {
  const notices = () =>
    new CombatNotices({
      api: {} as never,
      calls: {} as never,
      campaignId: () => 'c',
      ownId: () => 'tor',
    });

  it('tells a loss that arrives after the first read, until it is touched, and a new combat clears it', () => {
    const n = notices();
    n.readForm('e1', 'tor', true, []);
    expect(n.readiedNotice()).toBeNull();
    n.readForm('e1', 'tor', true, [lost('a')]);
    expect(n.readiedNotice()?.lead).toBe('A sua ação preparada se perdeu.');
    n.readiedLost.set(null);
    expect(n.readiedNotice()).toBeNull();
    n.readForm('e1', 'tor', true, [lost('a'), lost('b')]);
    n.readForm('e2', 'tor', true, []);
    expect(n.readiedNotice()).toBeNull();
  });
});
