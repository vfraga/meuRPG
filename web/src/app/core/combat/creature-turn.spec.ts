import { create } from '@bufbuild/protobuf';

import { Ability } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  CreatureActionKind,
  CreatureActionSchema,
  CreatureActionStepKind,
  CreatureDamagePartSchema,
  CreatureSavePlanSchema,
  CreatureUsageKind,
  CreatureUsageSchema,
} from '../../../gen/meurpg/play/v1/creatures_pb';
import {
  actionButton,
  actionLine,
  doesNotFit,
  legendaryButton,
  partsText,
  rechargeText,
  stepArrow,
  stepWord,
  unavailableWhy,
  usageText,
} from './creature-turn';

const part = (dice: string, key: string, pt: string) =>
  create(CreatureDamagePartSchema, { dice, damageTypeKey: key, damageTypePt: pt });

const plain = (t: string): string => t.replace(/\u00a0|\u202f/g, ' ');

describe('creature turn text', () => {
  it('writes the damage parts as the table reads them', () => {
    expect(
      partsText([
        part('2d10+8', 'damage-type:piercing', 'Perfurante'),
        part('2d6', 'damage-type:fire', 'Fogo'),
      ]),
    ).toBe('2d10 + 8 perfurante e 2d6 de fogo');
  });

  it('writes an attack with its bonus, reach and damage', () => {
    const bite = create(CreatureActionSchema, {
      kind: CreatureActionKind.ATTACK,
      attackBonus: 14,
      melee: true,
      reachFt: 10,
      damageParts: [part('2d10+8', 'damage-type:piercing', 'Perfurante')],
    });
    expect(plain(actionLine(bite))).toBe('+14 para acertar · alcance 3 m · 2d10 + 8 perfurante');
    expect(actionButton(bite)).toBe('Atacar');
  });

  it('writes a cone with its saving throw and the half of a success', () => {
    const breath = create(CreatureActionSchema, {
      kind: CreatureActionKind.SAVE,
      save: create(CreatureSavePlanSchema, {
        ability: Ability.DEXTERITY,
        dc: 21,
        halfOnSuccess: true,
        damage: [part('18d6', 'damage-type:fire', 'Fogo')],
        area: { shape: 'cone', lengthFt: 60 },
      }),
    });
    expect(plain(actionLine(breath))).toBe(
      'Cone de 18 m · teste de resistência de Destreza CD 21 · 18d6 de fogo, metade se passar',
    );
    expect(actionButton(breath)).toBe('Soprar');
  });

  it('says the limit of an action and why its button is grey', () => {
    const waiting = create(CreatureActionSchema, {
      available: false,
      usage: create(CreatureUsageSchema, {
        kind: CreatureUsageKind.RECHARGE,
        rechargeMin: 5,
        recharging: true,
      }),
    });
    expect(usageText(waiting)).toBe('Recarga 5–6 · Recarregando');
    expect(unavailableWhy(waiting)).toContain('d6');
    const daily = create(CreatureActionSchema, {
      usage: create(CreatureUsageSchema, {
        kind: CreatureUsageKind.PER_DAY,
        usesMax: 3,
        usesLeft: 2,
      }),
    });
    expect(usageText(daily)).toBe('3/dia · 2 de 3');
  });

  it('names the legendary cost, the missing room and the recharge roll', () => {
    expect(legendaryButton(2)).toBe('Usar (2)');
    expect(doesNotFit(1)).toBe('Resta só 1: não cabe');
    expect(rechargeText(3, 5)).toEqual({
      word: 'Não recarregou.',
      detail: 'No começo da vez o servidor rolou 1d6: 3. A recarga é 5–6.',
    });
    expect(rechargeText(5, 5).word).toBe('Recarregou.');
  });

  it('says what a step did', () => {
    expect(stepWord(CreatureActionStepKind.VULNERABILITY)).toBe('Vulnerável: dobra');
    expect(stepArrow(17, 34)).toBe('17 → 34');
  });
});
