import { create } from '@bufbuild/protobuf';

import { TableEffectSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { powerAttackMenu } from './content-testing-menu-power';
import { menu } from './content-testing-menu';
import {
  draftToEffect,
  effectToDraft,
  emptyEffect,
  onlyProficientApplies,
  unreadableNumberPaths,
} from './effect-draft';
import { withNumber } from './effect-number';
import { POWER_ATTACK_LIMITS, powerAttackIssues } from './power-attack-limits';

const m = powerAttackMenu();
const trade = (over: Partial<ReturnType<typeof emptyEffect>> = {}) => ({
  ...emptyEffect('power_attack'),
  attackPenalty: 5,
  damageBonus: 10,
  ...over,
});

describe('power_attack: the effect a request carries', () => {
  it('sends the two numbers and nothing else when only they are set', () => {
    expect(draftToEffect(trade(), m)).toEqual({
      type: 'power_attack',
      attackPenalty: 5,
      damageBonus: 10,
    });
  });

  it('sends the weapon kind, the properties, the proficiency switch, the resource, the condition and the text', () => {
    const d = trade({
      weapon: 'melee',
      weaponProperties: ['weapon-property:heavy', 'weapon-property:two-handed'],
      onlyProficient: true,
      resource: 'folego',
      when: 'level() >= 3',
      textPt: ' Troca precisão por força. ',
    });
    expect(draftToEffect(d, m)).toEqual({
      type: 'power_attack',
      attackPenalty: 5,
      damageBonus: 10,
      weapon: 'melee',
      weaponProperties: ['weapon-property:heavy', 'weapon-property:two-handed'],
      onlyProficient: true,
      resource: 'folego',
      when: 'level() >= 3',
      textPt: 'Troca precisão por força.',
    });
  });

  it('sends no weapon for "any weapon" (the empty draft) and no list for no property', () => {
    const sent = draftToEffect(trade({ weapon: '' }), m);
    expect('weapon' in sent).toBe(false);
    expect('weaponProperties' in sent).toBe(false);
    expect('onlyProficient' in sent).toBe(false);
  });

  it('does not send a number it cannot read (a NaN) or one that is not set', () => {
    const sent = draftToEffect(trade({ attackPenalty: Number.NaN, damageBonus: 0 }), m);
    expect('attackPenalty' in sent).toBe(false);
    expect('damageBonus' in sent).toBe(false);
  });

  it('reads a stored one back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'power_attack',
      attackPenalty: 3,
      damageBonus: 7,
      weapon: 'ranged',
      weaponProperties: ['weapon-property:ammunition'],
      onlyProficient: true,
      resource: 'folego',
      textPt: 'Mira firme.',
    });
    const draft = effectToDraft(stored);
    expect(draft.kept).toBeUndefined();
    expect(draftToEffect(draft, m)).toEqual({
      type: 'power_attack',
      attackPenalty: 3,
      damageBonus: 7,
      weapon: 'ranged',
      weaponProperties: ['weapon-property:ammunition'],
      onlyProficient: true,
      resource: 'folego',
      textPt: 'Mira firme.',
    });
  });

  it('copies the property list: the draft never shares the stored array', () => {
    const stored = create(TableEffectSchema, {
      type: 'power_attack',
      attackPenalty: 1,
      damageBonus: 1,
      weaponProperties: ['weapon-property:heavy'],
    });
    const draft = effectToDraft(stored);
    draft.weaponProperties.push('weapon-property:light');
    expect(stored.weaponProperties).toEqual(['weapon-property:heavy']);
  });

  it('refuses to save a number that reads as no whole number, at the field of the block', () => {
    expect(
      unreadableNumberPaths('table_feat.effects', [trade({ damageBonus: Number.NaN })], m),
    ).toEqual(['table_feat.effects[0].damage_bonus']);
  });
});

describe('onlyProficientApplies: where "Só onde já tem proficiência" means something', () => {
  it('holds for a power_attack with no level and no skill, and still for the expertise on a skill', () => {
    expect(onlyProficientApplies({ type: 'power_attack', level: '', proficiency: '' })).toBe(true);
    expect(
      onlyProficientApplies({
        type: 'proficiency',
        level: 'expertise',
        proficiency: 'skill:stealth',
      }),
    ).toBe(true);
    expect(onlyProficientApplies({ type: 'proficiency', level: '', proficiency: '' })).toBe(false);
    expect(onlyProficientApplies({ type: 'modifier', level: '', proficiency: '' })).toBe(false);
  });

  it('is not sent for another type, as before', () => {
    expect(
      'onlyProficient' in
        draftToEffect({ ...emptyEffect('proficiency'), onlyProficient: true }, menu()),
    ).toBe(false);
  });
});

describe('the types the editor offers', () => {
  it('lists power_attack once, in the feature editor and in the feat editor', () => {
    for (const featEditor of [false, true]) {
      expect(m.typesFor(featEditor).filter((t) => t.type === 'power_attack')).toHaveLength(1);
    }
  });
});

describe('withNumber: what a number field does to the draft as it is typed', () => {
  it('holds the number, and the text only when it is not how the number is written back', () => {
    const typed = withNumber(trade(), 'attack_penalty', '007', false);
    expect(typed.attackPenalty).toBe(7);
    expect(typed.typed).toEqual({ attack_penalty: '007' });
    const plain = withNumber(typed, 'attack_penalty', '7', false);
    expect(plain.attackPenalty).toBe(7);
    expect(plain.typed).toBeUndefined();
  });

  it('turns a text that is no whole number into NaN and an empty field into 0', () => {
    expect(withNumber(trade(), 'damage_bonus', 'dez', false).damageBonus).toBeNaN();
    expect(withNumber(trade(), 'damage_bonus', '', false).damageBonus).toBe(0);
  });

  it('keeps what was typed in the other number fields', () => {
    const a = withNumber(trade(), 'attack_penalty', '+5', false);
    const b = withNumber(a, 'damage_bonus', '010', false);
    expect(b.typed).toEqual({ attack_penalty: '+5', damage_bonus: '010' });
  });
});

describe('powerAttackIssues: what the editor says before the server does', () => {
  const limits = POWER_ATTACK_LIMITS;

  it('says nothing about numbers inside their ranges, empty ones or unreadable ones', () => {
    expect(powerAttackIssues(trade(), limits)).toEqual({});
    expect(powerAttackIssues(trade({ attackPenalty: 0, damageBonus: 0 }), limits)).toEqual({});
    expect(powerAttackIssues(trade({ attackPenalty: Number.NaN }), limits)).toEqual({});
    expect(powerAttackIssues(trade({ attackPenalty: 1, damageBonus: 20 }), limits)).toEqual({});
  });

  it('gives the range of a number outside it, in the words of the block', () => {
    expect(powerAttackIssues(trade({ attackPenalty: 12, damageBonus: 21 }), limits)).toEqual({
      attack_penalty: 'De 1 a 10.',
      damage_bonus: 'De 1 a 20.',
    });
    expect(powerAttackIssues(trade({ attackPenalty: -2 }), limits)).toEqual({
      attack_penalty: 'De 1 a 10.',
    });
  });

  it('says it of a 0 the person typed, not of an empty field', () => {
    expect(
      powerAttackIssues(trade({ damageBonus: 0, typed: { damage_bonus: '0' } }), limits),
    ).toEqual({ damage_bonus: 'De 1 a 20.' });
  });

  it('says "Até 4 propriedades." when there are more, and which are the ones over', () => {
    const five = [
      'weapon-property:heavy',
      'weapon-property:light',
      'weapon-property:reach',
      'weapon-property:finesse',
      'weapon-property:versatile',
    ];
    expect(powerAttackIssues(trade({ weaponProperties: five.slice(0, 4) }), limits)).toEqual({});
    expect(powerAttackIssues(trade({ weaponProperties: five }), limits)).toEqual({
      weapon_properties: 'Até 4 propriedades.',
    });
  });

  it("uses the limits it is given: the menu's, when the server says others", () => {
    expect(
      powerAttackIssues(trade({ attackPenalty: 6 }), {
        ...limits,
        penalty: { min: 1, max: 5 },
      }),
    ).toEqual({ attack_penalty: 'De 1 a 5.' });
  });
});
