import { create } from '@bufbuild/protobuf';

import {
  AttackTargetsSchema,
  CombatantState,
  GetTurnOptionsResponseSchema,
  ReadiedActionSchema,
  ReadiedKind,
  TargetInReachSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  AttackKind,
  AttackOptionSchema,
  TurnOptionsSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from './combat-testing';
import { readiedAim } from './readied-aim';

const row = (over: Record<string, unknown>) =>
  create(ReadiedActionSchema, {
    id: 'r',
    kind: ReadiedKind.ATTACK,
    actionKey: 'k',
    ...over,
  } as never);

const options = (kind: AttackKind, key = 'k') =>
  create(GetTurnOptionsResponseSchema, {
    options: create(TurnOptionsSchema, {
      attacks: [
        create(AttackOptionSchema, {
          enabled: true,
          attack: { key, kind, melee: true, rangeFt: 5 },
        } as never),
      ],
    }),
    attackTargets: [
      create(AttackTargetsSchema, {
        attackKey: key,
        targets: [
          create(TargetInReachSchema, { combatantId: 'z1', label: 'Zumbi 1', distanceFt: 5 }),
        ],
      }),
    ],
  });

const e = encounter({
  combatants: [
    combatant({ id: 'tor', label: 'Tor' }),
    combatant({ id: 'z1', label: 'Zumbi 1', state: CombatantState.HURT }),
  ],
} as never);

describe('readiedAim (#582: who a readied attack or spell is aimed at)', () => {
  it('requires a creature for a readied attack and chooses the one named when it is within reach', () => {
    const a = readiedAim(options(AttackKind.WEAPON), row({ targetCombatantId: 'z1' }), e, 'tor');
    expect(a.need).toBe('required');
    expect(a.chosen).toBe('z1');
    expect(a.rows.map((r) => r.id)).toEqual(['z1']);
    expect(a.legend).toBe('Em quem · 1 criatura ao alcance');
  });

  it('requires a creature for a readied spell attack (a cantrip released as an attack)', () => {
    const a = readiedAim(options(AttackKind.SPELL), row({ kind: ReadiedKind.SPELL }), e, 'tor');
    expect(a.need).toBe('required');
    expect(a.chosen).toBeNull();
  });

  it('offers the creatures of the combat for a readied spell that is not an attack, and asks for none', () => {
    const a = readiedAim(
      options(AttackKind.WEAPON, 'other'),
      row({ kind: ReadiedKind.SPELL, targetCombatantId: 'z1' }),
      e,
      'tor',
    );
    expect(a.need).toBe('optional');
    expect(a.chosen).toBe('z1');
    expect(a.rows.map((r) => [r.id, r.title])).toEqual([
      ['tor', 'Tor'],
      ['z1', 'Zumbi 1'],
    ]);
    expect(a.legend).toBe('Em quem (opcional)');
    expect(a.rows.map((r) => r.sub)).toEqual([
      'Você mesmo · Ileso',
      'O alvo que você disse · Ferido',
    ]);
    // The player's own combatant has no state word: its card says only that it is the readier, with no dangling dot.
    const own = encounter({
      combatants: [combatant({ id: 'tor', label: 'Tor', state: CombatantState.UNSPECIFIED })],
    } as never);
    expect(
      readiedAim(options(AttackKind.WEAPON, 'other'), row({ kind: ReadiedKind.SPELL }), own, 'tor')
        .rows[0].sub,
    ).toBe('Você mesmo');
  });

  it('takes a cantrip that asks for a saving throw as a spell: it has no attack roll to aim', () => {
    const res = options(AttackKind.SPELL);
    res.attackTargets = [];
    const a = readiedAim(res, row({ kind: ReadiedKind.SPELL }), e, 'tor');
    expect(a.need).toBe('optional');
  });
});
