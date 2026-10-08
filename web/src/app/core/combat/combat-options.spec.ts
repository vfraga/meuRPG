import { create } from '@bufbuild/protobuf';

import {
  AttackKind,
  AttackSchema,
  DisabledReasonCode,
  DisabledReasonSchema,
  Recharge,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { attackDetail, endTurnIsPrimary, isReactionHint, reasonText } from './combat-options';

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
      [DisabledReasonCode.NOT_YOUR_TURN, 'Não é a sua vez'],
      [DisabledReasonCode.COMBAT_NOT_ACTIVE, 'O combate não está em andamento'],
      [DisabledReasonCode.COMBATANT_DOWN, 'Caído: não pode agir'],
      [DisabledReasonCode.COMBATANT_DEFEATED, 'Derrotado: fora do combate'],
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
