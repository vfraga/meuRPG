import { create } from '@bufbuild/protobuf';

import {
  CombatLogEntrySchema,
  CombatLogKind,
  CombatLogReadiedSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { ReadiedNotices, readiedLostNoticeText } from './ready-notice';

const lost = (id: string, over: { actor?: string; canceled?: boolean; kind?: ReadiedKind } = {}) =>
  create(CombatLogEntrySchema, {
    id,
    kind: CombatLogKind.READIED_LOST,
    actorId: over.actor ?? 'tor',
    readiedLost: create(CombatLogReadiedSchema, {
      readiedId: 'r1',
      triggerText: 'Quando o zumbi passar pela porta',
      kind: over.kind ?? ReadiedKind.ATTACK,
      actionNamePt: 'Espada longa',
      canceled: over.canceled ?? false,
    }),
  });

describe("the readier's notice when the action is lost (board 582-08, case 3)", () => {
  it('says what was lost and that the spent action does not come back', () => {
    expect(readiedLostNoticeText({ held: 'Espada longa', spell: false })).toEqual({
      lead: 'A sua ação preparada se perdeu.',
      rest: 'Espada longa. A ação gasta não volta.',
    });
    expect(readiedLostNoticeText({ held: 'Mísseis Mágicos', spell: true })).toEqual({
      lead: 'Mísseis Mágicos se perdeu.',
      rest: 'A ação preparada acabou. O espaço gasto não volta.',
    });
  });

  it('tells only the loss of the own row that is new, never what the first read already held', () => {
    const n = new ReadiedNotices();
    expect(n.read('e1', 'tor', true, [lost('a')]).notice).toBeNull();
    expect(n.read('e1', 'tor', true, [lost('a'), lost('b')]).notice).toEqual({
      held: 'Espada longa',
      spell: false,
    });
    expect(n.read('e1', 'tor', true, [lost('a'), lost('b')]).notice).toBeNull();
  });

  it('says nothing for the readier giving it up, for another creature or before the log is read', () => {
    const n = new ReadiedNotices();
    n.read('e1', 'tor', true, []);
    expect(n.read('e1', 'tor', true, [lost('c', { canceled: true })]).notice).toBeNull();
    expect(n.read('e1', 'tor', true, [lost('d', { actor: 'brisa' })]).notice).toBeNull();
    const fresh = new ReadiedNotices();
    expect(fresh.read('e1', 'tor', false, [lost('e')]).notice).toBeNull();
  });

  it('starts clean in another combat, and names a spell for what it is', () => {
    const n = new ReadiedNotices();
    n.read('e1', 'tor', true, []);
    expect(n.read('e2', 'tor', true, []).reset).toBe(true);
    expect(n.read('e2', 'tor', true, [lost('f', { kind: ReadiedKind.SPELL })]).notice).toEqual({
      held: 'Espada longa',
      spell: true,
    });
  });

  it("says nothing for a spell lost by the readier's own pass, but tells one the master or the clock passed", () => {
    const fired = (id: string, byMaster: boolean) =>
      create(CombatLogEntrySchema, {
        id,
        kind: CombatLogKind.READIED_FIRED,
        actorId: 'tor',
        byMaster,
        readiedFired: create(CombatLogReadiedSchema, {
          readiedId: 'r1',
          status: ReadiedStatus.IGNORED,
        }),
      });
    const spell = { kind: ReadiedKind.SPELL };
    const n = new ReadiedNotices();
    n.read('e1', 'tor', true, []);
    // The server writes both lines in the pass: the loss has no reason of its own, but its row has an own pass of the same id.
    expect(n.read('e1', 'tor', true, [lost('g', spell), fired('f', false)]).notice).toBeNull();
    expect(n.read('e1', 'tor', true, [lost('h', spell), fired('i', true)]).notice).toEqual({
      held: 'Espada longa',
      spell: true,
    });
  });
});
