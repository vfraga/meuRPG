import {
  CombatLogKind,
  type CombatLogRound,
  CombatantKind,
  CombatantState,
  CoverDegree,
  CoverSource,
  type GetMoveOptionsResponse,
  type GetTurnOptionsResponse,
  PendingDamageStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  coverAgainst,
  creatureTrapNotes,
  deathDueOf,
  featureNote,
  initiativeLine,
  dyingOf,
  masterReach,
  offerMarks,
  opportunityAttacks,
  pendingRollOf,
  stoppedByReaction,
  trapNote,
} from './combat-derived';
import { combatant, encounter } from './combat-testing';

const turnWith = (over: object) => over as unknown as GetTurnOptionsResponse;
const target = (id: string, cover: CoverDegree, source = CoverSource.MAP) => ({
  combatantId: id,
  cover,
  coverSource: source,
});

describe('coverAgainst', () => {
  it('lists the first cover each target has, from the attacks and then the spells, and skips none or unspecified', () => {
    const opts = turnWith({
      attackTargets: [
        {
          targets: [
            target('a', CoverDegree.HALF),
            target('b', CoverDegree.NONE),
            target('c', CoverDegree.UNSPECIFIED),
          ],
        },
      ],
      spellTargets: [
        {
          targets: [
            target('a', CoverDegree.THREE_QUARTERS),
            target('d', CoverDegree.THREE_QUARTERS),
          ],
        },
      ],
    });
    expect([...coverAgainst(opts)]).toEqual([
      ['a', { cover: CoverDegree.HALF, source: CoverSource.MAP }],
      ['d', { cover: CoverDegree.THREE_QUARTERS, source: CoverSource.MAP }],
    ]);
  });

  it('is empty without options', () => {
    expect(coverAgainst(null).size).toBe(0);
    expect(coverAgainst(undefined).size).toBe(0);
  });
});

describe('offerMarks', () => {
  it('marks each placed reactor, with the square the mover left only for an offer that is for the caller', () => {
    const e = encounter({
      combatants: [
        combatant({ id: 'r1', label: 'R1', col: 3, row: 4 }),
        combatant({ id: 'r2', label: 'R2', col: 5, row: 6 }),
        combatant({ id: 'r3', label: 'R3', placed: false }),
      ],
      opportunityOffers: [
        { reactorId: 'r1', forYou: true, leftCol: 1, leftRow: 2 },
        { reactorId: 'r2', forYou: false, leftCol: 7, leftRow: 8 },
        { reactorId: 'r3', forYou: true, leftCol: 9, leftRow: 9 },
        { reactorId: 'ghost', forYou: true, leftCol: 0, leftRow: 0 },
      ] as never,
    });
    expect(offerMarks(e)).toEqual([
      { reactor: { col: 3, row: 4 }, left: { col: 1, row: 2 } },
      { reactor: { col: 5, row: 6 }, left: null },
    ]);
  });
});

describe('masterReach', () => {
  const who = combatant({ id: 'g', label: 'Goblin', col: 2, row: 3 });
  const options = {
    movementLeftDft: 250,
    reachable: [
      { col: 2, row: 4 },
      { col: 3, row: 3 },
    ],
  } as unknown as GetMoveOptionsResponse;

  it('draws the squares the server said the one on turn can reach, from where it stands', () => {
    expect(masterReach(who, true, options)).toEqual({
      origin: { col: 2, row: 3 },
      leftDft: 250,
      squares: [
        { col: 2, row: 4 },
        { col: 3, row: 3 },
      ],
    });
  });

  it('draws nothing when it is off, the combatant is not placed or not there, or the options are not read', () => {
    expect(masterReach(who, false, options)).toBeNull();
    expect(masterReach({ ...who, placed: false }, true, options)).toBeNull();
    expect(masterReach(null, true, options)).toBeNull();
    expect(masterReach(who, true, null)).toBeNull();
  });
});

describe('who the master has to answer for', () => {
  const e = encounter({
    combatants: [
      combatant({ id: 'a', label: 'A', kind: CombatantKind.PLAYER, state: CombatantState.DYING }),
      combatant({
        id: 'b',
        label: 'B',
        kind: CombatantKind.PLAYER,
        state: CombatantState.DOWN,
        deathSaveDue: true,
      }),
      combatant({ id: 'c', label: 'C', kind: CombatantKind.PLAYER, state: CombatantState.DOWN }),
      combatant({ id: 'd', label: 'D', state: CombatantState.DOWN, deathSaveDue: true }),
    ],
  });

  it('confirms the death of each character at three failures', () => {
    expect(dyingOf(e).map((c) => c.id)).toEqual(['a']);
    expect(dyingOf(null)).toEqual([]);
  });

  it('rolls the death save of a downed player character whose save is due, never an NPC’s', () => {
    expect(deathDueOf(e).map((c) => c.id)).toEqual(['b']);
    expect(deathDueOf(null)).toEqual([]);
  });
});

describe('opportunityAttacks', () => {
  const own = combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, mine: true });
  const opts = turnWith({
    options: {},
    attackTargets: [{ attackKey: 'a1', targets: [{ combatantId: 'g', inReach: true }] }],
  });

  it('is empty when the caller cannot react, has no character, used the reaction or has no options', () => {
    expect(opportunityAttacks(own, opts, false)).toEqual([]);
    expect(opportunityAttacks(null, opts, true)).toEqual([]);
    expect(opportunityAttacks({ ...own, reactionUsed: true }, opts, true)).toEqual([]);
    expect(opportunityAttacks(own, turnWith({}), true)).toEqual([]);
    expect(opportunityAttacks(own, null, true)).toEqual([]);
  });

  it('is empty for a character who is down or dead', () => {
    expect(opportunityAttacks({ ...own, state: CombatantState.DOWN }, opts, true)).toEqual([]);
    expect(opportunityAttacks({ ...own, state: CombatantState.DEAD }, opts, true)).toEqual([]);
  });
});

describe('pendingRollOf', () => {
  const pending = (id: string, attackerId: string, status: PendingDamageStatus) => ({
    id,
    attackerId,
    status,
  });

  it('finds the damage that awaits the roll of the player’s own character', () => {
    const opts = turnWith({
      pendingDamages: [
        pending('p1', 't', PendingDamageStatus.ROLLED),
        pending('p2', 'g', PendingDamageStatus.AWAITING_ROLL),
        pending('p3', 't', PendingDamageStatus.AWAITING_ROLL),
      ],
    });
    expect(pendingRollOf(opts, 't')?.id).toBe('p3');
    expect(pendingRollOf(opts, 'x')).toBeNull();
    expect(pendingRollOf(null, 't')).toBeNull();
  });
});

describe('the combat log reads', () => {
  const round = (n: number, entries: object[]) =>
    ({ round: n, entries }) as unknown as CombatLogRound;
  const caught = (targetId: string) => ({
    kind: CombatLogKind.TRAP_TRIGGERED,
    trap: {
      name: 'Fosso',
      caught: [
        { targetId, targetLabel: 'X', damages: [], attacks: [], saves: [], conditionKeys: [] },
      ],
    },
  });
  const e = encounter({
    round: 2,
    combatants: [
      combatant({ id: 't', label: 'Toren', kind: CombatantKind.PLAYER, mine: true }),
      combatant({
        id: 'w1',
        label: 'Lobo atroz 1',
        kind: CombatantKind.CREATURE,
        controlledByMe: true,
        summonGroupId: 'grp',
      }),
    ],
  });

  it('tells whether the newest attack of the one on turn was stopped by a reaction', () => {
    const rounds = [
      round(2, [{ kind: CombatLogKind.ATTACK, actorId: 'g', stoppedByReaction: true }]),
      round(1, [{ kind: CombatLogKind.ATTACK, actorId: 'g', stoppedByReaction: false }]),
    ];
    expect(stoppedByReaction(rounds, 'g')).toBe(true);
    expect(stoppedByReaction(rounds, 'other')).toBe(false);
    expect(stoppedByReaction([], 'g')).toBe(false);
  });

  it('says what a trap did to the player this round, in the first trap that caught them', () => {
    const rounds = [round(2, [caught('x'), caught('t')]), round(1, [caught('t')])];
    expect(trapNote(e, e.combatants[0]!, rounds)?.title).toBe('Você caiu na armadilha Fosso.');
    expect(trapNote(e, e.combatants[0]!, [round(1, [caught('t')])])).toBeNull();
    expect(trapNote(e, e.combatants[0]!, [round(2, [{ kind: CombatLogKind.ATTACK }])])).toBeNull();
    expect(trapNote(null, e.combatants[0]!, rounds)).toBeNull();
    expect(trapNote(e, null, rounds)).toBeNull();
  });

  it('says the same for each creature the player controls that a trap caught, by name', () => {
    const notes = creatureTrapNotes(e, false, [round(2, [caught('w1')])]);
    expect([...notes.keys()]).toEqual(['w1']);
    expect(notes.get('w1')?.title).toBe('O Lobo atroz 1 caiu na armadilha Fosso.');
    expect(creatureTrapNotes(e, true, [round(2, [caught('w1')])]).size).toBe(0);
    expect(creatureTrapNotes(null, false, []).size).toBe(0);
    expect(creatureTrapNotes(e, false, [round(1, [caught('w1')])]).size).toBe(0);
  });
});

describe('initiativeLine', () => {
  const c = (over: object) => combatant({ id: 'a', label: 'A', ...over });

  it('writes the d20, the modifier with its sign and the total', () => {
    expect(initiativeLine(c({ initiativeFace: 15, initiativeBonus: 4, initiative: 19 }))).toEqual({
      face: 15,
      line: '15 + 4 = 19',
    });
    expect(
      initiativeLine(c({ initiativeFace: 15, initiativeBonus: -2, initiative: 13 }))?.line,
    ).toBe('15 \u2212 2 = 13');
    expect(
      initiativeLine(c({ initiativeFace: 15, initiativeBonus: 0, initiative: 15 }))?.line,
    ).toBe('15 = 15');
  });

  it('is null while a number is missing, or there is no combatant', () => {
    expect(initiativeLine(c({ initiativeBonus: 4, initiative: 19 }))).toBeNull();
    expect(initiativeLine(c({ initiativeFace: 15, initiative: 19 }))).toBeNull();
    expect(initiativeLine(c({ initiativeFace: 15, initiativeBonus: 4 }))).toBeNull();
    expect(initiativeLine(undefined)).toBeNull();
  });
});

describe('featureNote', () => {
  it('says what the feature did: frenzy, Action Surge, or just that it was used', () => {
    expect(featureNote('Frenesi', 'x', true)).toBe('Frenesi: você entrou em frenesi.');
    expect(featureNote('Surto de Ação', 'action_surge', false)).toBe(
      'Surto de Ação: você tem outra ação.',
    );
    expect(featureNote('Fúria', 'rage', false)).toBe('Usou Fúria.');
  });
});
