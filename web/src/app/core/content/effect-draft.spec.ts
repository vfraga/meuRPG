import { create } from '@bufbuild/protobuf';

import { TableEffectSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { conditionMenu, menu, rangedAttackMenu, resistanceMenu } from './content-testing-menu';
import { extraDamageMenu, spellModifierMenu } from './content-testing-menu-damage';
import { grantedSpellMenu } from './content-testing-menu-costs';
import {
  draftToEffect,
  effectToDraft,
  emptyEffect,
  missingRequired,
  rangeFeet,
  unreadableRange,
  rangeMeters,
  splitTags,
  type EffectDraft,
} from './effect-draft';
import { visibleFields } from './effect-visible';

describe("an effect from the server's menu (ADR-0018, section 4)", () => {
  const m = menu();

  it('sends a maneuver die with where it goes and what it spends, and asks for both', () => {
    const d = {
      ...emptyEffect('superiority_die'),
      applies: 'damage',
      resource: 'dados_de_golpe',
      value: '8',
      textPt: 'O alvo faz um teste.',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'superiority_die',
      applies: 'damage',
      resource: 'dados_de_golpe',
      value: '8',
      textPt: 'O alvo faz um teste.',
    });
    expect(missingRequired({ ...d, applies: '' }, m)).toContain('applies');
  });

  it('knows the fields of each type from the menu, in its order, and the names of its lists', () => {
    expect(m.fieldsOf('proficiency').map((f) => f.name)).toEqual([
      'proficiency',
      'level',
      'only_proficient',
      'when',
      'text_pt',
    ]);
    expect(m.typeOf('sense')?.namePt).toBe('Sentido');
    expect(m.nameOf('senses', 'darkvision')).toBe('Visão no escuro');
    expect(m.nameOf('senses', 'x')).toBe('x');
    expect(m.maxEffects).toBe(4);
  });

  it('sends only the fields of the chosen type', () => {
    // A draft that still holds what an old type had: nothing of it goes.
    const d = {
      ...emptyEffect('proficiency'),
      proficiency: 'skill:perception',
      level: '',
      target: 'ac',
      mode: 'add',
      value: '2',
      rangeM: '18',
      sense: 'darkvision',
      count: 3,
      from: ['skill:arcana'],
      textPt: '  ',
    };
    expect(draftToEffect(d, m)).toEqual({ type: 'proficiency', proficiency: 'skill:perception' });
  });

  it('sends a modifier with its formula, its condition and its tags', () => {
    const d = {
      ...emptyEffect('modifier'),
      target: 'speed.walk',
      mode: 'add',
      value: ' 5 ',
      when: 'level() >= 5',
      tags: 'against:magic, about:items ,',
      proficiency: 'skill:perception',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'modifier',
      target: 'speed.walk',
      mode: 'add',
      value: '5',
      when: 'level() >= 5',
      tags: ['against:magic', 'about:items'],
    });
  });

  it('sends a sense with its range in feet, a choice with its list, a note with the text and the granted spells', () => {
    expect(
      draftToEffect({ ...emptyEffect('sense'), sense: 'darkvision', rangeM: '18' }, m),
    ).toEqual({ type: 'sense', sense: 'darkvision', rangeFt: 60 });
    expect(
      draftToEffect(
        {
          ...emptyEffect('choice'),
          choice: 'skill',
          count: 2,
          from: ['skill:arcana'],
          level: 'half',
        },
        m,
      ),
    ).toEqual({ type: 'choice', choice: 'skill', count: 2, from: ['skill:arcana'] });
    expect(
      draftToEffect({ ...emptyEffect('note'), textPt: 'Cai devagar.', spells: ['spell:light'] }, m),
    ).toEqual({ type: 'note', textPt: 'Cai devagar.', spells: ['spell:light'] });
  });

  it('reads a stored effect back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'modifier',
      target: 'ac',
      mode: 'add',
      value: '1',
      tags: ['against:magic'],
    });
    const d = effectToDraft(stored);
    expect(d.tags).toBe('against:magic');
    expect(draftToEffect(d, m)).toEqual({
      type: 'modifier',
      target: 'ac',
      mode: 'add',
      value: '1',
      tags: ['against:magic'],
    });
  });

  it('keeps a stored range as the metres it shows, and sends what was typed once, in feet', () => {
    const d = effectToDraft(
      create(TableEffectSchema, { type: 'sense', sense: 'darkvision', rangeFt: 60 }),
    );
    expect(d.rangeM).toBe('18');
    expect(draftToEffect(d, m)).toMatchObject({ rangeFt: 60 });
    // 4 m is not a whole number of feet at the table's rate: it is rounded once, on sending.
    expect(draftToEffect({ ...d, rangeM: '4' }, m)).toMatchObject({ rangeFt: 13 });
  });

  it('offers what a choice may list, by the kind of choice', () => {
    expect(m.fromOptions('skill').map((o) => o.key)).toEqual(['skill:arcana', 'skill:perception']);
    expect(m.fromOptions('language').map((o) => o.namePt)).toEqual(['Comum', 'Primordial']);
    expect(m.fromOptions('feature').map((o) => o.namePt)).toEqual([
      'Estilo de luta',
      'Dádiva do pacto',
    ]);
    m.classNamePt = (k) => (k === 'class:wizard' ? 'Mago' : k);
    expect(m.fromOptions('spell').map((o) => o.namePt)).toEqual(['Mago', 'class:cleric']);
    expect(m.fromOptions('')).toEqual([]);
  });

  it('says which required field is still empty', () => {
    expect(missingRequired(emptyEffect('modifier'), m)).toEqual(['target', 'mode', 'value']);
    expect(
      missingRequired({ ...emptyEffect('sense'), sense: 'darkvision', rangeM: '18' }, m),
    ).toEqual([]);
  });

  it('reads metres as feet in steps the server checks, and back', () => {
    expect(rangeFeet('18')).toBe(60);
    expect(rangeFeet('4,5')).toBe(15);
    expect(rangeFeet('')).toBe(0);
    expect(rangeFeet('abc')).toBe(0);
    expect(rangeMeters(60)).toBe('18');
    expect(rangeMeters(15)).toBe('4,5');
    expect(rangeMeters(0)).toBe('');
    expect(splitTags(' a, ,b ')).toEqual(['a', 'b']);
  });

  it('tells a range field it cannot read from an empty one', () => {
    for (const text of [
      '18 m',
      '18m',
      '1.000,5',
      'abc',
      '-3',
      '1e9',
      '999999999',
      '0x10',
      '1e3',
      '+3',
      '4,5,6',
    ]) {
      expect(unreadableRange(text), text).toBe(true);
    }
    for (const text of ['', '  ', '0', '18', '4,5', '4.5']) {
      expect(unreadableRange(text), text).toBe(false);
    }
  });
});

describe('the effects only a feat has (MR-025)', () => {
  const m = menu();

  it('leaves "ability_increase" out of every editor but the feat\'s', () => {
    expect(m.typesFor(false).map((t) => t.type)).not.toContain('ability_increase');
    expect(m.typesFor(true).map((t) => t.type)).toContain('ability_increase');
  });

  it('sends how many, from which abilities and how much, and asks for all three', () => {
    const d = {
      ...emptyEffect('ability_increase'),
      count: 2,
      from: ['strength', 'dexterity'],
      value: '1',
      target: 'ac',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'ability_increase',
      count: 2,
      from: ['strength', 'dexterity'],
      value: '1',
    });
    expect(missingRequired(emptyEffect('ability_increase'), m)).toEqual(['count', 'value']);
  });

  it("offers the table's feats to a choice of feats, and none by itself", () => {
    expect(m.fromOptions('feat')).toEqual([]);
    m.featOptions = [{ key: 'feat:mestre@mesa', namePt: 'Mestre', hintPt: '' }];
    expect(m.fromOptions('feat').map((o) => o.namePt)).toEqual(['Mestre']);
  });
});

describe("ranged_attack: a table feat's switches and spell range multiplier", () => {
  const m = rangedAttackMenu();

  it('sends the switches that are on and the multiplier that is set, and asks for which attacks', () => {
    const spell = {
      ...emptyEffect('ranged_attack'),
      applies: 'spell',
      ignoreCover: true,
      rangeMultiplier: 2,
    };
    expect(draftToEffect(spell, m)).toEqual({
      type: 'ranged_attack',
      applies: 'spell',
      ignoreCover: true,
      rangeMultiplier: 2,
    });
    const weapon = {
      ...emptyEffect('ranged_attack'),
      applies: 'weapon',
      noLongRangeDisadvantage: true,
    };
    expect(draftToEffect(weapon, m)).toEqual({
      type: 'ranged_attack',
      applies: 'weapon',
      noLongRangeDisadvantage: true,
    });
    expect(missingRequired({ ...spell, applies: '' }, m)).toEqual(['applies']);
  });

  it('reads a stored one back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'ranged_attack',
      applies: 'weapon',
      ignoreCover: true,
      noLongRangeDisadvantage: true,
    });
    expect(draftToEffect(effectToDraft(stored), m)).toEqual({
      type: 'ranged_attack',
      applies: 'weapon',
      ignoreCover: true,
      noLongRangeDisadvantage: true,
    });
  });

  it('never sends a switch on another type', () => {
    const d = {
      ...emptyEffect('modifier'),
      target: 'ac',
      mode: 'add',
      value: '1',
      ignoreCover: true,
      rangeMultiplier: 2,
    };
    const sent = draftToEffect(d, m);
    expect(sent).not.toHaveProperty('ignoreCover');
    expect(sent).not.toHaveProperty('rangeMultiplier');
  });
});

describe('resistance: the damage types it covers, its kind and when it holds', () => {
  const m = resistanceMenu();

  it('asks for nothing alone: the server says which of the types or the exception is missing', () => {
    expect(missingRequired(emptyEffect('resistance'), m)).toEqual([]);
  });

  it('sends the types, the kind and the condition, and only what is set', () => {
    const d = {
      ...emptyEffect('resistance'),
      damageTypes: ['damage-type:cold', 'nonmagical_bps'],
      kind: 'immune',
      while: 'rage',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'resistance',
      damageTypes: ['damage-type:cold', 'nonmagical_bps'],
      kind: 'immune',
      while: 'rage',
    });
    expect(draftToEffect({ ...emptyEffect('resistance'), damageTypes: ['all'] }, m)).toEqual({
      type: 'resistance',
      damageTypes: ['all'],
    });
  });

  it('still sends the type it leaves out, as before', () => {
    const d = { ...emptyEffect('resistance'), except: 'damage-type:psychic', while: 'rage' };
    expect(draftToEffect(d, m)).toEqual({
      type: 'resistance',
      except: 'damage-type:psychic',
      while: 'rage',
    });
  });

  it('reads a stored one back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'resistance',
      damageTypes: ['damage-type:poison'],
      kind: 'vulnerable',
    });
    expect(draftToEffect(effectToDraft(stored), m)).toEqual({
      type: 'resistance',
      damageTypes: ['damage-type:poison'],
      kind: 'vulnerable',
    });
  });
});

describe('condition immunity and the roll mode against a condition', () => {
  const m = conditionMenu();

  it('asks for the conditions and sends them', () => {
    expect(missingRequired(emptyEffect('condition_immunity'), m)).toEqual(['conditions']);
    const d = { ...emptyEffect('condition_immunity'), conditions: ['condition:poisoned'] };
    expect(draftToEffect(d, m)).toEqual({
      type: 'condition_immunity',
      conditions: ['condition:poisoned'],
    });
  });

  it('sends the condition of a roll mode only when one is picked', () => {
    const base = { ...emptyEffect('roll_mode'), roll: 'advantage', targets: ['save.wis'] };
    expect(draftToEffect(base, m)).toEqual({
      type: 'roll_mode',
      roll: 'advantage',
      targets: ['save.wis'],
    });
    expect(draftToEffect({ ...base, againstCondition: 'condition:frightened' }, m)).toEqual({
      type: 'roll_mode',
      roll: 'advantage',
      targets: ['save.wis'],
      againstCondition: 'condition:frightened',
    });
  });

  it('reads a stored one back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'roll_mode',
      roll: 'advantage',
      targets: ['save.wis'],
      againstCondition: 'condition:frightened',
    });
    expect(draftToEffect(effectToDraft(stored), m)).toEqual({
      type: 'roll_mode',
      roll: 'advantage',
      targets: ['save.wis'],
      againstCondition: 'condition:frightened',
    });
  });
});

describe('extra_damage: dice, type, weapon and frequency', () => {
  const m = extraDamageMenu();

  it('sends only what is set, the dice with their sides', () => {
    const d = {
      ...emptyEffect('extra_damage'),
      applies: 'any_target',
      dice: '1',
      diceSides: 6,
      damageType: 'damage-type:fire',
      weapon: 'ranged',
      frequency: 'each_hit',
      noCritDoubling: true,
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'extra_damage',
      applies: 'any_target',
      dice: '1',
      diceSides: 6,
      damageType: 'damage-type:fire',
      weapon: 'ranged',
      frequency: 'each_hit',
      noCritDoubling: true,
    });
    expect(
      draftToEffect({ ...emptyEffect('extra_damage'), applies: 'any_target', value: '2' }, m),
    ).toEqual({
      type: 'extra_damage',
      applies: 'any_target',
      value: '2',
    });
  });

  it('reads a stored one back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'extra_damage',
      applies: 'any_target',
      dice: '2',
      diceSides: 8,
      weapon: 'ranged',
    });
    expect(draftToEffect(effectToDraft(stored), m)).toEqual({
      type: 'extra_damage',
      applies: 'any_target',
      dice: '2',
      diceSides: 8,
      weapon: 'ranged',
    });
  });
});

describe('granted_spell: a spell the entry gives, cast with no slot', () => {
  const m = grantedSpellMenu();

  it('sends the spell, the ability, the level, what it spends and the switches that are on', () => {
    const d = {
      ...emptyEffect('granted_spell'),
      spell: 'spell:light',
      ability: 'choice',
      fromLevel: ' 3 ',
      uses: 'usos_de_teste',
      alsoWithSlot: true,
      countsAsKnown: true,
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'granted_spell',
      spell: 'spell:light',
      ability: 'choice',
      fromLevel: '3',
      uses: 'usos_de_teste',
      alsoWithSlot: true,
      countsAsKnown: true,
    });
  });

  it('sends nothing for an empty level, an empty resource (at will) and the switches that are off', () => {
    const d = { ...emptyEffect('granted_spell'), spell: 'spell:light', ability: 'wis' };
    expect(draftToEffect(d, m)).toEqual({
      type: 'granted_spell',
      spell: 'spell:light',
      ability: 'wis',
    });
  });

  it('asks for the spell and the ability', () => {
    expect(missingRequired(emptyEffect('granted_spell'), m)).toEqual(['spell', 'ability']);
  });

  it('reads a stored one back and sends it unchanged', () => {
    const stored = create(TableEffectSchema, {
      type: 'granted_spell',
      spell: 'spell:detect-magic',
      ability: 'int',
      uses: 'at_will',
      ritualOnly: true,
    });
    expect(draftToEffect(effectToDraft(stored), m)).toEqual({
      type: 'granted_spell',
      spell: 'spell:detect-magic',
      ability: 'int',
      uses: 'at_will',
      ritualOnly: true,
    });
  });
});

describe('spell_modifier: the change decides the fields', () => {
  const m = spellModifierMenu();
  const base = { ...emptyEffect('spell_modifier'), appliesTo: 'damage_spell' };
  const names = (d: EffectDraft) =>
    visibleFields(m.fieldsOf('spell_modifier'), d).map((f) => f.name);

  it('shows the swap the types, the bonus its amount and type, the dice their count, and none before a change is picked', () => {
    expect(names(base)).toEqual(['applies_to', 'spell_change', 'resource', 'when', 'text_pt']);
    expect(names({ ...base, spellChange: 'damage_type_swap' })).toContain('from');
    expect(names({ ...base, spellChange: 'damage_type_swap' })).not.toContain('value');
    expect(names({ ...base, spellChange: 'bonus_damage' })).toEqual(
      expect.arrayContaining(['value', 'damage_type']),
    );
    expect(names({ ...base, spellChange: 'extra_dice_per_slot_over' })).toContain('count');
    expect(names({ ...base, spellChange: 'extra_dice_per_slot_over' })).not.toContain('from');
  });

  it('sends only the fields of the chosen change, even if another was typed before', () => {
    const d = {
      ...base,
      spellChange: 'bonus_damage',
      value: '1',
      damageType: 'damage-type:fire',
      from: ['damage-type:cold'],
      count: 2,
      resource: 'tintas',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'spell_modifier',
      appliesTo: 'damage_spell',
      spellChange: 'bonus_damage',
      value: '1',
      damageType: 'damage-type:fire',
      resource: 'tintas',
    });
    expect(draftToEffect({ ...d, spellChange: 'damage_type_swap' }, m)).toEqual({
      type: 'spell_modifier',
      appliesTo: 'damage_spell',
      spellChange: 'damage_type_swap',
      from: ['damage-type:cold'],
      resource: 'tintas',
    });
  });

  it('asks for the spells and the change, and for nothing else', () => {
    expect(missingRequired(emptyEffect('spell_modifier'), m)).toEqual([
      'applies_to',
      'spell_change',
    ]);
    expect(missingRequired({ ...base, spellChange: 'bonus_damage' }, m)).toEqual([]);
  });
});

describe("superiority_die on the character's own rolls", () => {
  const m = menu();
  const die = { ...emptyEffect('superiority_die'), resource: 'dados', value: '6' };
  const names = (applies: string) =>
    visibleFields(m.fieldsOf('superiority_die'), { ...die, applies }).map((f) => f.name);

  it('lets only a check list targets, only a save name an ability, and none of them an action', () => {
    expect(names('check')).toContain('targets');
    expect(names('check')).not.toContain('ability');
    expect(names('check')).not.toContain('economy');
    expect(names('save')).toContain('ability');
    expect(names('save')).not.toContain('targets');
    expect(names('initiative')).not.toEqual(expect.arrayContaining(['ability']));
    expect(names('initiative')).not.toContain('targets');
    expect(names('initiative')).not.toContain('economy');
    // The older uses keep their fields, and never a targets list.
    expect(names('damage')).toEqual(expect.arrayContaining(['ability', 'economy']));
    expect(names('damage')).not.toContain('targets');
  });

  it('sends the targets of a check and drops what a change of roll left behind', () => {
    const d = {
      ...die,
      applies: 'check',
      targets: ['skill.stealth'],
      ability: 'dex',
      economy: 'bonus_action',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'superiority_die',
      applies: 'check',
      resource: 'dados',
      value: '6',
      targets: ['skill.stealth'],
    });
    expect(draftToEffect({ ...d, applies: 'damage' }, m)).not.toHaveProperty('targets');
  });
});
describe('breathes_water: no field of its own', () => {
  const m = grantedSpellMenu();

  it('sends the type and the condition only', () => {
    const d = {
      ...emptyEffect('breathes_water'),
      when: 'level() >= 3',
      textPt: ' Respira na água. ',
    };
    expect(draftToEffect(d, m)).toEqual({
      type: 'breathes_water',
      when: 'level() >= 3',
      textPt: 'Respira na água.',
    });
    expect(missingRequired(emptyEffect('breathes_water'), m)).toEqual([]);
  });
});
