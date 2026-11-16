import { create } from '@bufbuild/protobuf';

import {
  AttackKind,
  AttackOptionSchema,
  AttackSchema,
  PowerAttackOptionSchema,
  Recharge,
  VersatileGrip,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import type { DiceRoll } from '../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  isTradePart,
  poolShortMessage,
  tradeBox,
  tradeOfferSub,
  tradeWhy,
  tradePartFormula,
  tradePartNote,
  tradeRollLabel,
  tradeRollLine,
  tradeRowLine,
  tradedBonus,
  tradesOf,
} from './power-attack-trade';
import { d20Formula } from './roll-mode';

const d20 = (over: Partial<DiceRoll>): DiceRoll =>
  ({
    diceCount: 1,
    diceSides: 20,
    faces: [16],
    modifier: 1,
    total: 17,
    countedIndex: 0,
    extraDice: [],
    physical: false,
    ...over,
  }) as DiceRoll;

describe('the d20 formula with a trade', () => {
  it('writes the bonus and the penalty apart, so the sum is the total', () => {
    expect(d20Formula(d20({}), true, 5)).toBe('1d20 (16) + 6 − 5 = 17');
  });

  it('writes the penalty after the kept face of two d20', () => {
    const pair = d20({ diceCount: 2, faces: [9, 15], countedIndex: 1, total: 16 });
    expect(d20Formula(pair, true, 5)).toBe('15 + 6 − 5 = 16');
  });

  it('keeps the typed face of a physical d20 and writes the penalty after the bonus', () => {
    expect(d20Formula(d20({ physical: true, faces: [] }), true, 5)).toBe('16 + 6 − 5 = 17');
  });

  it('writes a bonus the penalty takes to zero as the penalty alone', () => {
    expect(d20Formula(d20({ modifier: -5, total: 11 }), true, 5)).toBe('1d20 (16) − 5 = 11');
  });

  it('is the old formula with no trade', () => {
    expect(d20Formula(d20({}), true)).toBe('1d20 (16) + 1 = 17');
  });
});

const heavy = create(PowerAttackOptionSchema, {
  key: 'feat:golpe-pesado@mesa',
  name: 'Golpe Pesado',
  penalty: 5,
  damageBonus: 10,
});
const montante = create(AttackSchema, {
  key: 'attack:greatsword',
  namePt: 'Montante',
  attackBonus: 6,
  damage: '2d6 + 3',
  damageTypePt: 'cortante',
  kind: AttackKind.WEAPON,
});

describe('the weapon row line', () => {
  it('says the trade with the true minus', () => {
    expect(tradeRowLine(heavy)).toBe('Golpe Pesado: −5 no ataque, +10 no dano');
  });

  it('lists the trades of the option, and none for a spell, an NPC or no option', () => {
    const option = create(AttackOptionSchema, { attack: montante, powerAttacks: [heavy] });
    expect(tradesOf(option)).toEqual([heavy]);
    expect(tradesOf(create(AttackOptionSchema, { attack: montante }))).toEqual([]);
    expect(tradesOf(undefined)).toEqual([]);
    const spell = create(AttackSchema, { key: 'spell:x', kind: AttackKind.SPELL });
    expect(tradesOf(create(AttackOptionSchema, { attack: spell, powerAttacks: [heavy] }))).toEqual(
      [],
    );
  });
});

describe('the offer under "Troca do talento"', () => {
  it('says what the trade costs and gives, and that the damage needs a hit', () => {
    expect(tradeOfferSub(heavy)).toBe('−5 no ataque, +10 no dano se acertar');
  });
});

describe('the offer of a trade that spends a pool (586-05 #3, #4)', () => {
  const spends = (uses: number) =>
    create(PowerAttackOptionSchema, {
      ...heavy,
      resourceNamePt: 'Fôlego do Atirador',
      usesLeft: uses,
      disabled: uses === 0,
    });

  it('says which pool it spends and what is left after the roll', () => {
    expect(tradeOfferSub(spends(3))).toBe(
      '−5 no ataque, +10 no dano se acertar · gasta 1 de Fôlego do Atirador · resta 2',
    );
    expect(tradeOfferSub(spends(2))).toBe(
      '−5 no ataque, +10 no dano se acertar · gasta 1 de Fôlego do Atirador · resta 1',
    );
  });

  it('leaves "resta" out when the roll takes the last use, and the pool out for a trade that spends nothing', () => {
    expect(tradeOfferSub(spends(1))).toBe(
      '−5 no ataque, +10 no dano se acertar · gasta 1 de Fôlego do Atirador',
    );
    expect(tradeOfferSub(spends(0))).toBe(
      '−5 no ataque, +10 no dano se acertar · gasta 1 de Fôlego do Atirador',
    );
    expect(tradeOfferSub(heavy)).toBe('−5 no ataque, +10 no dano se acertar');
  });

  it("says when the pool comes back, in the app's own sentences for a recharge", () => {
    const empty = (recharge: Recharge) =>
      tradeWhy(create(PowerAttackOptionSchema, { ...spends(0), recharge }));
    expect(empty(Recharge.SHORT_REST)).toBe(
      'Sem usos de Fôlego do Atirador: volta no descanso curto ou longo.',
    );
    expect(empty(Recharge.LONG_REST)).toBe(
      'Sem usos de Fôlego do Atirador: volta no descanso longo.',
    );
    expect(empty(Recharge.DAWN)).toBe('Sem usos de Fôlego do Atirador: volta ao amanhecer.');
    expect(empty(Recharge.NONE)).toBe('Sem usos de Fôlego do Atirador: só o mestre devolve.');
    expect(empty(Recharge.UNSPECIFIED)).toBe('Sem usos de Fôlego do Atirador.');
  });

  it('writes the reason of a trade with no use left with the pool, and keeps the server text without it', () => {
    expect(tradeWhy(spends(0))).toBe('Sem usos de Fôlego do Atirador.');
    expect(tradeWhy(spends(2))).toBe('');
    const bare = create(PowerAttackOptionSchema, {
      ...heavy,
      disabled: true,
      disabledReasonPt: 'Sem usos do recurso.',
    });
    expect(tradeWhy(bare)).toBe('Sem usos do recurso.');
    expect(tradeWhy(create(PowerAttackOptionSchema, { ...heavy, disabled: true }))).toBe(
      'Sem usos.',
    );
  });
});

describe('the attack bonus with the trade', () => {
  it('takes the penalty off the bonus, and leaves it alone with none picked', () => {
    expect(tradedBonus(6, heavy)).toBe(1);
    expect(tradedBonus(6, undefined)).toBe(6);
    expect(tradedBonus(2, heavy)).toBe(-3);
  });
});

describe('the before and after box', () => {
  it('writes the attack and the damage as they were and as they are', () => {
    expect(tradeBox(montante, heavy, RollMode.NORMAL)).toEqual({
      attackBefore: '1d20 + 6',
      attackAfter: '1d20 + 1',
      damageBefore: '2d6 + 3',
      damageAfter: '2d6 + 13 cortante, se acertar',
    });
  });

  it('counts two d20 with advantage and a negative bonus with a true minus', () => {
    const box = tradeBox({ ...montante, attackBonus: 2, damage: '1d8' }, heavy, RollMode.ADVANTAGE);
    expect(box.attackBefore).toBe('2d20 + 2');
    expect(box.attackAfter).toBe('2d20 − 3');
    expect(box.damageBefore).toBe('1d8');
    expect(box.damageAfter).toBe('1d8 + 10 cortante, se acertar');
  });

  it('writes the dice of the grip when a versatile weapon is held in two hands', () => {
    const twoHands = {
      ...montante,
      damage: '1d8 + 3',
      versatileDamage: '1d10 + 3',
      grip: VersatileGrip.TWO_HANDS,
    };
    const box = tradeBox(twoHands, heavy, RollMode.NORMAL);
    expect(box.damageBefore).toBe('1d10 + 3');
    expect(box.damageAfter).toBe('1d10 + 13 cortante, se acertar');
  });

  it('adds to a negative modifier of the damage', () => {
    const box = tradeBox({ ...montante, damage: '1d4 - 1' }, heavy, RollMode.NORMAL);
    expect(box.damageBefore).toBe('1d4 − 1');
    expect(box.damageAfter).toBe('1d4 + 9 cortante, se acertar');
  });
});

describe('the roll button', () => {
  it('names the d20 and the new modifier', () => {
    expect(tradeRollLabel(RollMode.NORMAL, 1)).toBe('Rolar 1d20 + 1 no app');
    expect(tradeRollLabel(RollMode.DISADVANTAGE, 1)).toBe('Rolar 2d20 + 1 no app');
    expect(tradeRollLabel(RollMode.NORMAL, -3)).toBe('Rolar 1d20 − 3 no app');
    expect(tradeRollLabel(RollMode.NORMAL, 0)).toBe('Rolar 1d20 no app');
  });
});

describe('the roll line of a trade', () => {
  it('names the trade and what it took off the roll', () => {
    expect(tradeRollLine(heavy, 'hit')).toBe('Golpe Pesado: −5 no ataque');
  });

  it('says a miss adds no damage', () => {
    expect(tradeRollLine(heavy, 'miss')).toBe(
      'Golpe Pesado: −5 no ataque · sem dano, o +10 não entra',
    );
  });

  it('says only the penalty while the result is not told', () => {
    expect(tradeRollLine(heavy, 'unknown')).toBe('Golpe Pesado: −5 no ataque');
  });
});

describe('two pickings of one pool', () => {
  it('asks to unmark one, with the pool and what is left', () => {
    expect(poolShortMessage('Reserva de Golpes', 1)).toBe(
      'Sem usos suficientes: cada um gasta 1 de Reserva de Golpes, e resta 1. Desmarque um.',
    );
    expect(poolShortMessage('Reserva de Golpes', 3)).toBe(
      'Sem usos suficientes: cada um gasta 1 de Reserva de Golpes, e restam 3. Desmarque um.',
    );
    expect(poolShortMessage('', null)).toBe(
      'Sem usos suficientes: cada um gasta 1 uso. Desmarque um.',
    );
  });
});

describe('the automatic part of the damage', () => {
  const part = { key: 'power_attack', labelPt: 'Golpe Pesado', flat: 10 };

  it('is told by its key', () => {
    expect(isTradePart(part)).toBe(true);
    expect(isTradePart({ key: 'rage' })).toBe(false);
  });

  it('writes the flat number with its sign', () => {
    expect(tradePartFormula(part)).toBe('+10');
  });

  it('says it only comes in with a hit and that a critical hit never doubles it', () => {
    expect(tradePartNote(part)).toBe(
      'Só porque acertou; num crítico, os dados dobram e o +10 não.',
    );
  });
});
