import { create } from '@bufbuild/protobuf';

import {
  AttackKind,
  AttackOptionSchema,
  AttackSchema,
  BonusAttackRule,
  DisabledReasonCode,
  DisabledReasonSchema,
  Recharge,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  attackDetail,
  beamSpentText,
  beamsLeftLine,
  bonusAttackLine,
  bonusSpentText,
  endTurnIsPrimary,
  featureUsesText,
  isBonusAttack,
  isReactionHint,
  reasonText,
} from './combat-options';

function reason(code: DisabledReasonCode, extra: { minLevel?: number; recharge?: Recharge } = {}) {
  return create(DisabledReasonSchema, { code, ...extra });
}

describe('the reasons an option is disabled', () => {
  it('says each code in Portuguese', () => {
    const cases: [DisabledReasonCode, string][] = [
      [DisabledReasonCode.ACTION_USED, 'Ação já usada'],
      [DisabledReasonCode.BONUS_ACTION_USED, 'Ação bônus já usada'],
      [DisabledReasonCode.REACTION_USED, 'Reação já usada'],
      [DisabledReasonCode.REACTION_ONLY_WHEN_HIT, 'Só fora da sua vez'],
      [DisabledReasonCode.ATTACK_ACTION_FIRST, 'Só depois de atacar com a ação'],
      [DisabledReasonCode.NOT_YOUR_TURN, 'Não é a sua vez'],
      [DisabledReasonCode.COMBAT_NOT_ACTIVE, 'O combate não está em andamento'],
      [DisabledReasonCode.COMBATANT_DOWN, 'Caído: não pode agir'],
      [DisabledReasonCode.COMBATANT_DEFEATED, 'Derrotado: fora do combate'],
      // W7-X: a surprised combatant's options are all off for this reason.
      [DisabledReasonCode.SURPRISED, 'Surpresa'],
      [DisabledReasonCode.ARMOR_NOT_PROFICIENT, 'Armadura ou escudo sem proficiência'],
      [DisabledReasonCode.WALL_PLACEMENT_REQUIRED, 'Falta dizer onde fica a muralha'],
    ];
    for (const [code, text] of cases) {
      expect(reasonText(reason(code))).toBe(text);
    }
  });

  it('says "Sem espaço" and when the uses come back', () => {
    // The slot rows above the list say which circles are out, so the reason stays short.
    expect(reasonText(reason(DisabledReasonCode.NO_SLOT, { minLevel: 2 }))).toBe('Sem espaço');
    expect(reasonText(reason(DisabledReasonCode.NO_SLOT))).toBe('Sem espaço');
    expect(reasonText(reason(DisabledReasonCode.NO_USES, { recharge: Recharge.SHORT_REST }))).toBe(
      'Sem usos: volta num descanso curto',
    );
  });

  it('says a once-per-turn feature was already used in the turn', () => {
    expect(reasonText(reason(DisabledReasonCode.ALREADY_USED_THIS_TURN))).toBe(
      'Já usado neste turno',
    );
  });

  it('reads the sentence a spell reason carries, which names the spell, and the short word without one', () => {
    const own = create(DisabledReasonSchema, {
      code: DisabledReasonCode.BONUS_ACTION_SPELL_LIMIT,
      textPt:
        'Você já conjurou uma magia de ação bônus neste turno: Curar Ferimentos não cabe, só um truque de 1 ação.',
    });
    expect(reasonText(own)).toBe(own.textPt);
    expect(reasonText(own, true)).toBe(own.textPt);
    expect(reasonText(reason(DisabledReasonCode.NO_SLOT))).toBe('Sem espaço');
  });

  it('says a bonus action spell limits the turn’s other spells', () => {
    expect(reasonText(reason(DisabledReasonCode.BONUS_ACTION_SPELL_LIMIT))).toBe(
      'Só um truque junto com uma magia de ação bônus',
    );
  });

  it('has a word for every code, and an empty one for none', () => {
    expect(reasonText(undefined)).toBe('');
    for (const code of Object.values(DisabledReasonCode).filter(
      (c) => typeof c === 'number' && c > 0,
    )) {
      expect(reasonText(reason(code as DisabledReasonCode))).not.toBe('Indisponível agora');
    }
  });

  it('treats a reaction that waits to be hit as a hint, not a block', () => {
    expect(isReactionHint(reason(DisabledReasonCode.REACTION_ONLY_WHEN_HIT))).toBe(true);
    expect(isReactionHint(reason(DisabledReasonCode.ACTION_USED))).toBe(false);
  });
});

const plain = (text: string) => text.replace(/\u00a0/g, ' ');

describe('the line under an attack', () => {
  it('writes a cantrip and a close weapon', () => {
    const fireBolt = create(AttackSchema, {
      name: 'Fire Bolt',
      namePt: 'Raio de Fogo',
      attackBonus: 6,
      damage: '1d10',
      damageTypePt: 'fogo',
      kind: AttackKind.SPELL,
      rangeFt: 120,
    });
    expect(plain(attackDetail(fireBolt))).toBe('+6 para acertar · 1d10 de fogo · alcance 36 m');
    const dagger = create(AttackSchema, {
      name: 'Dagger',
      namePt: 'Adaga',
      attackBonus: 4,
      damage: '1d4+2',
      damageTypePt: 'perfurante',
      kind: AttackKind.WEAPON,
      rangeFt: 5,
      longRangeFt: 0,
    });
    expect(plain(attackDetail(dagger))).toBe(
      '+4 para acertar · 1d4 + 2 perfurante · corpo a corpo',
    );
    // The number, its unit and "alcance" never split across lines.
    expect(attackDetail(fireBolt)).toContain('alcance\u00a036\u00a0m');
    expect(plain(attackDetail(dagger, true))).toBe(
      '+4 para acertar · 1d4 + 2 perfurante · corpo a corpo, 1,5 m',
    );
  });
});

describe('"Encerrar turno"', () => {
  it('is outlined while the action or the bonus action is available, filled when both are used', () => {
    expect(endTurnIsPrimary({ actionUsed: false, bonusActionUsed: false })).toBe(false);
    expect(endTurnIsPrimary({ actionUsed: true, bonusActionUsed: false })).toBe(false);
    expect(endTurnIsPrimary({ actionUsed: true, bonusActionUsed: true })).toBe(true);
  });
});

describe('the bonus action attacks', () => {
  const option = (over: {
    enabled?: boolean;
    attack?: { key: string; namePt: string };
    beamsLeft?: number;
    bonusRule?: BonusAttackRule;
    bonusAttacksLeft?: number;
    bonusDropsModifier?: boolean;
  }) => create(AttackOptionSchema, { enabled: true, ...over });

  it('tells which attacks are made with the bonus action', () => {
    expect(isBonusAttack(option({}))).toBe(false);
    expect(isBonusAttack(option({ bonusRule: BonusAttackRule.OFF_HAND }))).toBe(true);
  });

  it('says why in one line: the off hand with or without the modifier, Artes Marciais, Rajada de Golpes counting down', () => {
    expect(
      bonusAttackLine(option({ bonusRule: BonusAttackRule.OFF_HAND, bonusDropsModifier: true })),
    ).toBe('Ataque com a outra mão, sem o modificador no dano');
    expect(bonusAttackLine(option({ bonusRule: BonusAttackRule.OFF_HAND }))).toBe(
      'Ataque com a outra mão',
    );
    expect(bonusAttackLine(option({ bonusRule: BonusAttackRule.MARTIAL_ARTS }))).toBe(
      'Golpe desarmado das Artes Marciais',
    );
    const flurry = (bonusAttacksLeft: number) =>
      bonusAttackLine(option({ bonusRule: BonusAttackRule.FLURRY_OF_BLOWS, bonusAttacksLeft }));
    expect(flurry(2)).toBe('Rajada de Golpes: 2 golpes restantes');
    expect(flurry(1)).toBe('Rajada de Golpes: 1 golpe restante');
  });

  it('adds no line to an attack of the action, or to one that cannot be made', () => {
    expect(bonusAttackLine(option({}))).toBe('');
    expect(bonusAttackLine(option({ enabled: false, bonusRule: BonusAttackRule.OFF_HAND }))).toBe(
      '',
    );
  });

  it('says what the attack spent once it is made', () => {
    expect(bonusSpentText(BonusAttackRule.OFF_HAND)).toBe('Sua ação bônus foi usada.');
    expect(bonusSpentText(BonusAttackRule.MARTIAL_ARTS)).toBe('Sua ação bônus foi usada.');
    expect(bonusSpentText(BonusAttackRule.FLURRY_OF_BLOWS, 2)).toBe(
      'Rajada de Golpes: 1 golpe restante.',
    );
    expect(bonusSpentText(BonusAttackRule.FLURRY_OF_BLOWS, 1)).toBe(
      'Rajada de Golpes: acabaram os golpes.',
    );
    expect(bonusSpentText(BonusAttackRule.UNSPECIFIED)).toBe('');
  });
});

describe('the beams of a cantrip cast', () => {
  const blast = (over: { enabled?: boolean; beamsLeft?: number }) =>
    create(AttackOptionSchema, {
      attack: { key: 'spell:eldritch-blast', namePt: 'Rajada Mística' },
      enabled: true,
      ...over,
    });

  it('counts the beams still to fire in one line, down to the last', () => {
    expect(beamsLeftLine(blast({ beamsLeft: 2 }))).toBe('Rajada Mística: 2 raios restantes');
    expect(beamsLeftLine(blast({ beamsLeft: 1 }))).toBe('Rajada Mística: 1 raio restante');
  });

  it('says nothing before the first beam or once the cantrip is off', () => {
    expect(beamsLeftLine(blast({ beamsLeft: 0 }))).toBe('');
    expect(beamsLeftLine(blast({ beamsLeft: 1, enabled: false }))).toBe('');
  });

  it('says what a beam spent: the beams left, or the action with the last one', () => {
    expect(beamSpentText('Rajada Mística', 3)).toBe('Rajada Mística: 2 raios restantes.');
    expect(beamSpentText('Rajada Mística', 2)).toBe('Rajada Mística: 1 raio restante.');
    expect(beamSpentText('Rajada Mística', 1)).toBe('Sua ação foi usada.');
  });
});

describe('what a feature row says about its resource', () => {
  it('says 1 ki and the ki left for the monk\'s bonus actions, not "N usos"', () => {
    expect(featureUsesText('ki', 3)).toBe('Gasta 1 ki · restam 3');
    expect(featureUsesText('ki', 1)).toBe('Gasta 1 ki · restam 1');
    expect(featureUsesText('ki', 0)).toBe('Gasta 1 ki · sem ki');
  });

  it('keeps the uses of any other resource', () => {
    expect(featureUsesText('second_wind', 1)).toBe('1 uso');
    expect(featureUsesText('rage', 2)).toBe('2 usos');
    expect(featureUsesText('', 0)).toBe('');
  });
});
