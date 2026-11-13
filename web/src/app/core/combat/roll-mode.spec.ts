import { create } from '@bufbuild/protobuf';

import { DiceRollSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import { ExtraDieSchema } from '../../../gen/meurpg/play/v1/lasting_effects_pb';
import {
  AdvantageSourceKind,
  DamageStepKind,
  RollMode,
  RollModeRequestStatus,
} from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  d20Count,
  d20Faces,
  d20Formula,
  isBetter,
  liveRequest,
  modeStatus,
  modeWord,
  reasonValid,
  requestLine,
  sourceLine,
  sourceWord,
  stepLine,
} from './roll-mode';

const request = (status: RollModeRequestStatus, extra: Record<string, unknown> = {}) =>
  ({
    id: 'r1',
    combatantId: 'a',
    targetId: 'b',
    attackNamePt: 'Espada curta',
    suggestedMode: RollMode.NORMAL,
    requestedMode: RollMode.ADVANTAGE,
    reason: 'estou nas costas dele',
    status,
    ...extra,
  }) as never;

describe('roll mode words and rules', () => {
  it('names the three modes, an unset one as normal', () => {
    expect(modeWord(RollMode.ADVANTAGE)).toBe('Vantagem');
    expect(modeWord(RollMode.DISADVANTAGE)).toBe('Desvantagem');
    expect(modeWord(RollMode.NORMAL)).toBe('Normal');
    expect(modeWord(RollMode.UNSPECIFIED)).toBe('Normal');
  });

  it('rolls one d20 for a normal roll and two for advantage or disadvantage', () => {
    expect(d20Count(RollMode.NORMAL)).toBe(1);
    expect(d20Count(RollMode.ADVANTAGE)).toBe(2);
    expect(d20Count(RollMode.DISADVANTAGE)).toBe(2);
  });

  it('knows a mode better than the suggestion', () => {
    expect(isBetter(RollMode.ADVANTAGE, RollMode.NORMAL)).toBe(true);
    expect(isBetter(RollMode.NORMAL, RollMode.DISADVANTAGE)).toBe(true);
    expect(isBetter(RollMode.DISADVANTAGE, RollMode.NORMAL)).toBe(false);
    expect(isBetter(RollMode.NORMAL, RollMode.NORMAL)).toBe(false);
  });

  it('takes a reason of 1 to 120 characters on one line', () => {
    expect(reasonValid('')).toBe(false);
    expect(reasonValid('   ')).toBe(false);
    expect(reasonValid('a')).toBe(true);
    expect(reasonValid('a'.repeat(120))).toBe(true);
    expect(reasonValid('a'.repeat(121))).toBe(false);
    expect(reasonValid('duas\nlinhas')).toBe(false);
  });

  it('words a source as Vantagem or Desvantagem', () => {
    const src = (effect: RollMode) =>
      ({ kind: AdvantageSourceKind.PRONE_TARGET, effect, textPt: 'x' }) as never;
    expect(sourceWord(src(RollMode.ADVANTAGE))).toBe('Vantagem');
    expect(sourceWord(src(RollMode.DISADVANTAGE))).toBe('Desvantagem');
  });

  it('writes the sunlight sources with the server sentence, as any other source', () => {
    for (const kind of [
      AdvantageSourceKind.SUNLIGHT_ATTACK,
      AdvantageSourceKind.SUNLIGHT_PERCEPTION,
      AdvantageSourceKind.SUNLIGHT_SAVE,
    ]) {
      const source = {
        kind,
        effect: RollMode.DISADVANTAGE,
        textPt: 'Luz do sol (Sensibilidade à Luz do Sol)',
      } as never;
      expect(sourceLine(source)).toBe('Desvantagem: Luz do sol (Sensibilidade à Luz do Sol)');
    }
  });
});

describe('modeStatus', () => {
  const none = null;

  it('lets the suggestion roll with no reason', () => {
    expect(modeStatus('request', RollMode.ADVANTAGE, RollMode.ADVANTAGE, '', none)).toBe('ready');
  });

  it('asks for a reason for any change, disadvantage included', () => {
    expect(modeStatus('request', RollMode.NORMAL, RollMode.DISADVANTAGE, '', none)).toBe(
      'needs-reason',
    );
    expect(modeStatus('request', RollMode.NORMAL, RollMode.DISADVANTAGE, 'cego', none)).toBe(
      'ready',
    );
  });

  it('sends a player to the master for a better mode, once the reason is there', () => {
    expect(modeStatus('request', RollMode.NORMAL, RollMode.ADVANTAGE, '', none)).toBe(
      'needs-reason',
    );
    expect(modeStatus('request', RollMode.NORMAL, RollMode.ADVANTAGE, 'costas', none)).toBe(
      'needs-request',
    );
  });

  it('lets the master pick any mode with its reason', () => {
    expect(modeStatus('free', RollMode.NORMAL, RollMode.ADVANTAGE, 'costas', none)).toBe('ready');
    expect(modeStatus('free', RollMode.NORMAL, RollMode.ADVANTAGE, '', none)).toBe('needs-reason');
  });

  it('blocks a better mode where a player cannot ask', () => {
    expect(modeStatus('blocked', RollMode.NORMAL, RollMode.ADVANTAGE, 'x', none)).toBe('blocked');
    expect(modeStatus('blocked', RollMode.NORMAL, RollMode.DISADVANTAGE, 'x', none)).toBe('ready');
  });

  it('waits while the request is pending and rolls once it is answered', () => {
    expect(
      modeStatus(
        'request',
        RollMode.NORMAL,
        RollMode.ADVANTAGE,
        'x',
        request(RollModeRequestStatus.PENDING),
      ),
    ).toBe('pending');
    expect(
      modeStatus(
        'request',
        RollMode.NORMAL,
        RollMode.ADVANTAGE,
        'x',
        request(RollModeRequestStatus.ANSWERED),
      ),
    ).toBe('ready');
  });
});

describe('requests and rolls', () => {
  it('finds a request that is still open', () => {
    const open = request(RollModeRequestStatus.PENDING);
    const closed = request(RollModeRequestStatus.CLOSED, { id: 'r2' });
    expect(liveRequest([open, closed], 'r1')).toBe(open);
    expect(liveRequest([open, closed], 'r2')).toBeNull();
    expect(liveRequest([open], 'zz')).toBeNull();
  });

  it("writes the master's line", () => {
    const labels: Record<string, string> = { a: 'Toren', b: 'Goblin' };
    expect(requestLine(request(RollModeRequestStatus.PENDING), (id) => labels[id])).toBe(
      'Pedido de Vantagem: Toren → Goblin (Espada curta) — “estou nas costas dele”',
    );
  });

  it('marks the counted d20 of a pair and none for a single one', () => {
    const pair = create(DiceRollSchema, { diceCount: 2, faces: [14, 7], countedIndex: 0 });
    expect(d20Faces(pair)).toEqual([
      { face: 14, counted: true },
      { face: 7, counted: false },
    ]);
    const one = create(DiceRollSchema, { diceCount: 1, faces: [9], countedIndex: 0 });
    expect(d20Faces(one)).toEqual([{ face: 9, counted: true }]);
  });

  it('writes the formula of the d20 that counts', () => {
    const pair = create(DiceRollSchema, {
      diceCount: 2,
      diceSides: 20,
      faces: [14, 7],
      countedIndex: 1,
      modifier: 5,
      total: 12,
    });
    expect(d20Formula(pair)).toBe('7 + 5 = 12');
    const one = create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [9],
      countedIndex: 0,
      modifier: 5,
      total: 14,
    });
    expect(d20Formula(one)).toBe('1d20 (9) + 5 = 14');
  });

  it('names the extra dice of a d20 when asked, for one die and for the pair', () => {
    const extraDice = [
      create(ExtraDieSchema, {
        sourceKey: 'm',
        sourceNamePt: 'Golpe Certeiro',
        faces: 8,
        sign: 1,
        face: 5,
      }),
    ];
    const one = create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [12],
      countedIndex: 0,
      modifier: 6,
      total: 23,
      extraDice,
    });
    expect(d20Formula(one)).toBe('1d20 (12) + 6 + 1d8 (5) = 23');
    expect(d20Formula(one, true)).toBe('1d20 (12) + 6 + Golpe Certeiro 1d8 (5) = 23');
    const pair = create(DiceRollSchema, { ...one, diceCount: 2, faces: [12, 3], countedIndex: 0 });
    expect(d20Formula(pair, true)).toBe('12 + 6 + Golpe Certeiro 1d8 (5) = 23');
  });

  it('writes a resistance step', () => {
    expect(
      stepLine({
        kind: DamageStepKind.RESISTANCE,
        labelPt: 'Resistência a fogo (tiefling)',
        before: 10,
        after: 5,
      } as never),
    ).toBe('Resistência a fogo (tiefling): 10 → 5');
  });
});
