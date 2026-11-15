import { create } from '@bufbuild/protobuf';

import {
  AttackOutcome,
  CombatantSide,
  CombatantState,
  PendingDamageSchema,
  PendingDamageStatus,
  TargetInReachSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  missMark,
  defaultTarget,
  isHostile,
  nearestInReach,
  outcomeWord,
  stageAfterRoll,
  steps,
  targetAfter,
  targetRows,
} from './attack-flow';
import { CoverDegree, CoverSource } from '../../../gen/meurpg/play/v1/combat_pb';

const awaiting = create(PendingDamageSchema, { status: PendingDamageStatus.AWAITING_ROLL });

describe('the attack sheet steps', () => {
  it('marks the done steps, the current one and the rest', () => {
    expect(steps('target').map((s) => s.state)).toEqual(['current', 'todo', 'todo']);
    expect(steps('roll').map((s) => s.state)).toEqual(['done', 'current', 'todo']);
    expect(steps('damage').map((s) => s.state)).toEqual(['done', 'done', 'current']);
    expect(steps('done').map((s) => s.state)).toEqual(['done', 'done', 'done']);
  });

  it('goes on to the damage after a hit, and finishes after a miss', () => {
    expect(stageAfterRoll(AttackOutcome.HIT, awaiting)).toBe('damage');
    expect(stageAfterRoll(AttackOutcome.CRITICAL_HIT, awaiting)).toBe('damage');
    expect(stageAfterRoll(AttackOutcome.MISS, undefined)).toBe('done');
    expect(stageAfterRoll(AttackOutcome.HIT, undefined)).toBe('done');
  });

  it('names the outcome', () => {
    expect(
      [AttackOutcome.HIT, AttackOutcome.CRITICAL_HIT, AttackOutcome.MISS].map(outcomeWord),
    ).toEqual(['Acertou', 'Crítico', 'Errou']);
  });
});

describe('the natural 1 of a miss', () => {
  const roll = (outcome: AttackOutcome, face: number) =>
    ({ outcome, d20: { diceSides: 20, faces: [face], countedIndex: 0 } }) as never;

  it('marks a miss on a natural 1 only', () => {
    expect(missMark(roll(AttackOutcome.MISS, 1))).toBe('1 natural');
    expect(missMark(roll(AttackOutcome.MISS, 7))).toBe('');
    // A natural 20 is the critical hit's business, and a hit has no miss to explain.
    expect(missMark(roll(AttackOutcome.CRITICAL_HIT, 20))).toBe('');
    expect(missMark(roll(AttackOutcome.HIT, 1))).toBe('');
    expect(missMark({ outcome: AttackOutcome.MISS, d20: undefined })).toBe('');
  });

  it('follows the counted die of advantage', () => {
    const adv = {
      outcome: AttackOutcome.MISS,
      d20: { diceSides: 20, faces: [1, 1], countedIndex: 1 },
    } as never;
    expect(missMark(adv)).toBe('1 natural');
    const higher = {
      outcome: AttackOutcome.MISS,
      d20: { diceSides: 20, faces: [1, 9], countedIndex: 1 },
    } as never;
    expect(missMark(higher)).toBe('');
  });
});

describe('the targets', () => {
  it('writes the state word and the distance, and blocks one that is too far', () => {
    const rows = targetRows(
      [
        create(TargetInReachSchema, {
          combatantId: 'c',
          label: 'Capitão Goblin',
          state: CombatantState.HURT,
          distanceFt: 25,
        }),
        create(TargetInReachSchema, {
          combatantId: 'g',
          label: 'Goblin 2',
          state: CombatantState.UNHURT,
          distanceFt: 40,
          tooFar: true,
        }),
      ],
      120,
    );
    const plain = (t: string) => t.replace(/\u00a0/g, ' ');
    expect(rows.map((r) => [plain(r.sub), plain(r.blocked)])).toEqual([
      ['Ferido · a 7,5 m', ''],
      ['Ileso · a 12,0 m', 'Longe demais: alcance de 36 m'],
    ]);
    expect(rows[1].blocked).toContain('alcance de\u00a036\u00a0m');
  });

  it('names the cover of each target with its source, disables a mark of total cover and leaves out a wall (E9-07)', () => {
    const rows = targetRows(
      [
        create(TargetInReachSchema, {
          combatantId: 'g2',
          label: 'Goblin 2',
          distanceFt: 25,
          cover: CoverDegree.HALF,
          coverSource: CoverSource.MAP,
        }),
        create(TargetInReachSchema, {
          combatantId: 'c',
          label: 'Capitão Goblin',
          distanceFt: 35,
          cover: CoverDegree.THREE_QUARTERS,
          coverSource: CoverSource.MAP,
        }),
        create(TargetInReachSchema, {
          combatantId: 'g1',
          label: 'Goblin 1',
          distanceFt: 30,
          cover: CoverDegree.TOTAL,
          coverSource: CoverSource.MAP,
          untargetable: true,
        }),
        create(TargetInReachSchema, {
          combatantId: 'g3',
          label: 'Goblin 3',
          distanceFt: 20,
          cover: CoverDegree.TOTAL,
          coverSource: CoverSource.MARK,
          untargetable: true,
        }),
        create(TargetInReachSchema, { combatantId: 'g4', label: 'Goblin 4', distanceFt: 20 }),
      ],
      120,
    );
    expect(rows.map((r) => r.id)).toEqual(['g2', 'c', 'g3', 'g4']);
    expect(rows.map((r) => [r.cover, r.coverMark, r.blocked])).toEqual([
      ['Meia cobertura (do mapa)', 'half', ''],
      ['Três quartos (do mapa)', 'three', ''],
      ['', null, 'Cobertura total (marcada pelo mestre): não pode ser alvo'],
      ['', null, ''],
    ]);
  });

  it('says what happened to the target', () => {
    const defeated = create(PendingDamageSchema, {
      status: PendingDamageStatus.APPLIED,
      targetDefeated: true,
    });
    expect(targetAfter('Goblin 2', false, defeated, 'Derrotado')).toBe('Goblin 2 derrotado');
    const rolled = create(PendingDamageSchema, { status: PendingDamageStatus.ROLLED });
    expect(targetAfter('Toren', true, rolled, '')).toBe('Esperando o mestre aplicar o dano');
    const applied = create(PendingDamageSchema, { status: PendingDamageStatus.APPLIED });
    expect(targetAfter('Capitão Goblin', false, applied, 'Ferido')).toBe('Capitão Goblin: ferido');
  });
});

describe('the damage that waits for the master', () => {
  it('counts the temporary hit points first, and never goes below 0', async () => {
    const { hitPointsAfter, hitPointsLine } = await import('./attack-flow');
    expect(hitPointsAfter(26, 0, 5)).toBe(21);
    expect(hitPointsAfter(26, 3, 5)).toBe(24);
    expect(hitPointsAfter(4, 0, 9)).toBe(0);
    expect(hitPointsLine('Toren', 26, 31, 21)).toBe('Toren: 26 de 31 PV, depois 21');
  });

  it('says what is owed: to apply first, then to roll, else nothing', async () => {
    const { pendingNote } = await import('./attack-flow');
    const rolled = create(PendingDamageSchema, { status: PendingDamageStatus.ROLLED, amount: 5 });
    const waiting = create(PendingDamageSchema, { status: PendingDamageStatus.AWAITING_ROLL });
    const applied = create(PendingDamageSchema, { status: PendingDamageStatus.APPLIED, amount: 2 });
    const sum = create(PendingDamageSchema, { status: PendingDamageStatus.AWAITING_GROUP_SUM });
    expect(pendingNote([waiting, rolled])).toBe('Falta aplicar 5 de dano');
    expect(pendingNote([waiting])).toBe('Falta rolar o dano');
    expect(pendingNote([sum])).toBe('Falta rolar o dano');
    expect(pendingNote([sum], () => 'Jabuti')).toBe('Falta a soma de Jabuti');
    expect(pendingNote([applied])).toBeNull();
  });

  it('keeps only the damages that wait: the master also lists the applied one a take-out could undo', async () => {
    const { openPendingDamages } = await import('./attack-flow');
    const of = (status: PendingDamageStatus) =>
      create(PendingDamageSchema, { id: `${status}`, status });
    const all = [
      PendingDamageStatus.AWAITING_ROLL,
      PendingDamageStatus.ROLLED,
      PendingDamageStatus.APPLIED,
      PendingDamageStatus.DISCARDED,
      PendingDamageStatus.AWAITING_REACTION,
      PendingDamageStatus.AWAITING_GROUP_SUM,
      PendingDamageStatus.UNSPECIFIED,
    ].map(of);
    expect(openPendingDamages(all).map((p) => p.status)).toEqual([
      PendingDamageStatus.AWAITING_ROLL,
      PendingDamageStatus.ROLLED,
      PendingDamageStatus.AWAITING_REACTION,
      PendingDamageStatus.AWAITING_GROUP_SUM,
    ]);
  });
});

describe('the target an attack starts on', () => {
  const at = (id: string, distanceFt: number | undefined, extra: object = {}) =>
    create(TargetInReachSchema, { combatantId: id, label: id, distanceFt, ...extra });

  it('is the nearest creature in reach, not the first of the order when that one is out of reach', () => {
    const list = [at('toren', 10, { tooFar: true }), at('pensantus', 5), at('brisa', 5)];
    expect(nearestInReach(list)).toBe('pensantus');
  });

  it('is the nearest of those in reach, and the first of the order among equals', () => {
    const list = [at('a', 15), at('b', 5), at('c', 5), at('d', 10)];
    expect(nearestInReach(list)).toBe('b');
  });

  it('skips a creature behind total cover the player cannot target', () => {
    const list = [at('hidden', 5, { untargetable: true }), at('open', 10)];
    expect(nearestInReach(list)).toBe('open');
  });

  it('puts a creature with no distance (no map) after those it can measure', () => {
    const list = [at('unknown', undefined), at('near', 15)];
    expect(nearestInReach(list)).toBe('near');
  });

  it('with nothing in reach starts on the first of the order, and with no one on none', () => {
    const list = [at('far1', 20, { tooFar: true }), at('far2', 25, { tooFar: true })];
    expect(nearestInReach(list)).toBe('far1');
    expect(nearestInReach([])).toBe('');
  });
});

describe('the target an attack starts on, by who is hostile (G3)', () => {
  const at = (id: string, distanceFt: number | undefined, extra: object = {}) =>
    create(TargetInReachSchema, { combatantId: id, label: id, distanceFt, ...extra });
  const foes =
    (...ids: string[]) =>
    (id: string) =>
      ids.includes(id);

  it('is the nearest hostile creature in reach, never the ally standing closer', () => {
    // The armor's own ally at 1,5 m comes first in the list; the party is at 1,5 m too.
    const list = [at('espada', 5), at('toren', 5), at('brisa', 10)];
    expect(defaultTarget(list, foes('toren', 'brisa'))).toBe('toren');
  });

  it('is the nearest hostile out of reach when none of them is in reach', () => {
    const list = [
      at('espada', 5),
      at('toren', 80, { tooFar: true }),
      at('brisa', 60, { tooFar: true }),
    ];
    expect(defaultTarget(list, foes('toren', 'brisa'))).toBe('brisa');
  });

  it('skips a hostile creature the attack cannot target while another stands in reach', () => {
    const list = [at('toren', 5, { untargetable: true }), at('brisa', 15), at('espada', 5)];
    expect(defaultTarget(list, foes('toren', 'brisa'))).toBe('brisa');
  });

  it('falls back to the nearest of the others only when no hostile creature exists', () => {
    const list = [at('far', 20, { tooFar: true }), at('near', 5)];
    expect(defaultTarget(list, foes())).toBe('near');
    expect(defaultTarget([], foes())).toBe('');
  });
});

describe('who is hostile', () => {
  it('is the other side of the party and the enemies, and nobody else', () => {
    expect(isHostile(CombatantSide.ENEMY, CombatantSide.PARTY)).toBe(true);
    expect(isHostile(CombatantSide.PARTY, CombatantSide.ENEMY)).toBe(true);
    expect(isHostile(CombatantSide.ENEMY, CombatantSide.ENEMY)).toBe(false);
    expect(isHostile(CombatantSide.PARTY, CombatantSide.PARTY)).toBe(false);
    expect(isHostile(CombatantSide.UNSPECIFIED, CombatantSide.PARTY)).toBe(false);
  });
});
