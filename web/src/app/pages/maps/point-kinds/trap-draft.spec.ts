import { clone, create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';

import { MapPointSchema, TrapState } from '../../../../gen/meurpg/maps/v1/maps_pb';
import {
  Ability,
  TrapPassOutcome,
  TrapPresetSchema,
  TrapSaveApplies,
  TrapTargets,
  TrapTrigger,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import {
  alsoFindOptions,
  blankTrapDraft,
  hasTrapErrors,
  isTrapDirty,
  newAttack,
  newCondition,
  newDamage,
  newSave,
  presetSummary,
  toggleAlsoFind,
  trapChangesOf,
  trapDraftFromPreset,
  trapDraftOf,
  trapErrors,
  trapSpecOf,
  validDice,
} from './trap-draft';

// The Agulha envenenada as the SRD gives it (effects/traps.json): 1 piercing and 2d10 poison that always land, then a
// Constitution DC 15 save whose only fail result is Poisoned for 1 hour.
const needle = create(TrapPresetSchema, {
  key: 'trap:poison-needle',
  namePt: 'Agulha envenenada',
  descriptionPt: 'Agulha na fechadura de um baú.',
  noticeDc: 0,
  findDc: 20,
  trigger: TrapTrigger.MANUAL,
  areaSize: 1,
  effect: {
    targets: TrapTargets.MANUAL,
    damage: [
      { dice: '1', damageTypeKey: 'damage-type:piercing', damageTypePt: 'perfurante' },
      { dice: '2d10', damageTypeKey: 'damage-type:poison', damageTypePt: 'veneno' },
    ],
    save: {
      ability: Ability.CONSTITUTION,
      dc: 15,
      appliesTo: TrapSaveApplies.CAUGHT,
      onFail: { condition: { conditionKey: 'condition:poisoned', durationPt: '1 hora' } },
      onPass: TrapPassOutcome.NONE,
    },
  },
});

const pit = create(TrapPresetSchema, {
  key: 'trap:hidden-pit',
  namePt: 'Fosso escondido',
  descriptionPt: 'Um fosso de 6 m.',
  noticeDc: 15,
  findDc: 15,
  trigger: TrapTrigger.ENTER,
  areaSize: 2,
  fallFt: 20,
  effect: {
    damage: [{ dice: '2d6', damageTypeKey: 'damage-type:bludgeoning', damageTypePt: 'concussão' }],
    conditions: [{ conditionKey: 'condition:prone', conditionPt: 'Derrubado' }],
  },
});

describe('a preset fills the form', () => {
  it('copies the numbers, the trigger, the area and the effect parts (the poison needle has no DC to notice)', () => {
    const d = trapDraftFromPreset(needle);
    expect(d).toMatchObject({
      presetKey: 'trap:poison-needle',
      name: 'Agulha envenenada',
      noticeDc: '',
      findDc: '20',
      areaSize: 1,
      trigger: TrapTrigger.MANUAL,
      targets: TrapTargets.MANUAL,
    });
    expect(d.damage).toEqual([
      { dice: '1', typeKey: 'damage-type:piercing' },
      { dice: '2d10', typeKey: 'damage-type:poison' },
    ]);
    expect(d.save).toMatchObject({
      ability: Ability.CONSTITUTION,
      dc: '15',
      onPass: TrapPassOutcome.NONE,
      failDamage: [],
      failCondition: { key: 'condition:poisoned', duration: '1 hora' },
    });
    expect(trapErrors(d)).toEqual({ parts: {} });
  });

  it('describes a pit by its fall and the others by their parts, for the radio', () => {
    expect(presetSummary(pit)).toBe('Queda de 6\u00a0m, 2d6');
    expect(presetSummary(needle)).toBe('1 perfurante, 2d10 veneno · resistência de Constituição');
  });

  it('starts from nothing on "Começar do zero": the square alone, fired by entering it, no effect', () => {
    const d = blankTrapDraft('Armadilha nova');
    expect(d).toMatchObject({
      presetKey: '',
      areaSize: 1,
      trigger: TrapTrigger.ENTER,
      state: TrapState.ARMED,
      attack: null,
      damage: [],
      conditions: [],
      save: null,
    });
  });
});

describe('trapSpecOf', () => {
  it('sends the whole spec: the DCs as numbers, an empty notice DC as 0, the parts as the server takes them', () => {
    const spec = trapSpecOf(trapDraftFromPreset(needle));
    expect(spec).toMatchObject({
      presetKey: 'trap:poison-needle',
      noticeDc: 0,
      findDc: 20,
      areaSize: 1,
      trigger: TrapTrigger.MANUAL,
      state: TrapState.ARMED,
    });
    expect(spec.effect?.damage).toEqual([
      { dice: '1', damageTypeKey: 'damage-type:piercing' },
      { dice: '2d10', damageTypeKey: 'damage-type:poison' },
    ]);
    expect(spec.effect?.save).toMatchObject({
      ability: Ability.CONSTITUTION,
      dc: 15,
      onFail: {
        damage: [],
        condition: { conditionKey: 'condition:poisoned', durationPt: '1 hora' },
      },
      onPass: TrapPassOutcome.NONE,
    });
    expect(spec.effect?.attack).toBeUndefined();
  });
});

describe('trapErrors', () => {
  it('asks for a name and the DC to find it', () => {
    const e = trapErrors(blankTrapDraft());
    expect(e.name).toBe('Dê um nome ao ponto.');
    expect(e.findDc).toBe('Use uma CD de 1 a 30.');
    expect(hasTrapErrors(e)).toBe(true);
  });

  it('takes an empty DC to notice, and refuses one out of 1 to 30', () => {
    const ok = { ...blankTrapDraft('A'), findDc: '15' };
    expect(trapErrors(ok).noticeDc).toBeUndefined();
    expect(trapErrors({ ...ok, noticeDc: '31' }).noticeDc).toContain('1 a 30');
    expect(trapErrors({ ...ok, noticeDc: '0' }).noticeDc).toContain('1 a 30');
  });

  it('checks the dice of each part: d4 to d12, 1 to 20 of them, or a flat 1 to 100', () => {
    for (const good of ['2d6', '1d12', '20d4', '1', '100']) {
      expect(validDice(good), good).toBe(true);
    }
    for (const bad of ['', 'd6', '2d20', '21d6', '0', '101', '2d6+1', 'abc']) {
      expect(validDice(bad), bad).toBe(false);
    }
    const d = { ...blankTrapDraft('A'), findDc: '15', damage: [{ ...newDamage(), dice: '2d20' }] };
    expect(trapErrors(d).parts['damage:0']).toContain('2d6');
  });

  it("checks an attack's bonus (0 to 20) and count (1 to 10)", () => {
    const base = { ...blankTrapDraft('A'), findDc: '15' };
    const e = trapErrors({
      ...base,
      attack: {
        ...newAttack(),
        bonus: '21',
        count: '0',
        damage: { dice: '1d4', typeKey: 'damage-type:piercing' },
      },
    });
    expect(e.attackBonus).toBe('Use de 0 a 20.');
    expect(e.attackCount).toBe('Use de 1 a 10.');
    expect(e.attackDice).toBeUndefined();
  });

  it('a save needs a DC and something to do on a failure; "metade" needs damage', () => {
    const base = { ...blankTrapDraft('A'), findDc: '15' };
    const e = trapErrors({ ...base, save: { ...newSave(), dc: '' } });
    expect(e.parts['saveDc']).toBe('Use uma CD de 1 a 30.');
    expect(e.parts['save']).toContain('o que acontece a quem falha');
    const half = trapErrors({
      ...base,
      save: {
        ...newSave(),
        dc: '12',
        failCondition: { key: 'condition:prone', duration: '' },
        onPass: TrapPassOutcome.HALF,
      },
    });
    expect(half.parts['save']).toContain('Metade do dano');
  });

  it('"quem foi atingido" needs an attack', () => {
    const base = { ...blankTrapDraft('A'), findDc: '15' };
    const e = trapErrors({
      ...base,
      save: {
        ...newSave(),
        dc: '12',
        appliesTo: TrapSaveApplies.HIT,
        failDamage: [{ dice: '1d6', typeKey: 'damage-type:fire' }],
      },
    });
    expect(e.parts['save']).toContain('ataque');
  });
});

describe('what the point carries', () => {
  const point = create(MapPointSchema, {
    id: 'p1',
    name: 'Fosso escondido',
    description: 'No corredor.',
    trap: {
      presetKey: 'trap:hidden-pit',
      noticeDc: 15,
      findDc: 15,
      areaSize: 2,
      trigger: TrapTrigger.ENTER,
      state: TrapState.ARMED,
      effect: { damage: [{ dice: '2d6', damageTypeKey: 'damage-type:bludgeoning' }] },
    },
  });

  it('is the form unchanged, and clean', () => {
    const d = trapDraftOf(point);
    expect(isTrapDirty(d, point)).toBe(false);
    expect(trapChangesOf(d, point)).toBeNull();
  });

  it('saves the whole spec when anything changes, and the name only when it changed', () => {
    const d = { ...trapDraftOf(point), findDc: '12', state: TrapState.DISARMED };
    const changes = trapChangesOf(d, point);
    expect(changes?.trap).toMatchObject({
      findDc: 12,
      state: TrapState.DISARMED,
      noticeDc: 15,
      areaSize: 2,
    });
    expect(changes?.name).toBeUndefined();
    expect(trapChangesOf({ ...d, name: ' Fosso do corredor ' }, point)?.name).toBe(
      'Fosso do corredor',
    );
  });

  it('leaves the state to the server when the master did not touch it, so a trap that fired meanwhile stays fired', () => {
    // The form was opened while the trap was armed; then it fired in play.
    const opened = trapDraftOf(point);
    const fired = clone(MapPointSchema, point);
    fired.trap!.state = TrapState.TRIGGERED;
    const edited = { ...opened, findDc: '12' };
    const changes = trapChangesOf(edited, fired, opened.state);
    expect(changes?.trap).toMatchObject({ findDc: 12, state: TrapState.UNSPECIFIED });
    // Choosing a state is a change the master made.
    expect(
      trapChangesOf({ ...edited, state: TrapState.DISARMED }, fired, opened.state)?.trap,
    ).toMatchObject({ state: TrapState.DISARMED });
    // A form nobody edited is clean even though the trap moved on.
    expect(trapChangesOf(opened, fired, opened.state)).toBeNull();
  });

  it('adding and removing a part is a change', () => {
    const d = trapDraftOf(point);
    expect(isTrapDirty({ ...d, damage: [...d.damage, newDamage()] }, point)).toBe(true);
    expect(isTrapDirty({ ...d, damage: [] }, point)).toBe(true);
  });
});

describe('"Também acham com" (PM-03c)', () => {
  it('starts empty, even for a preset, and reads the saved keys', () => {
    expect(blankTrapDraft().alsoFind).toEqual([]);
    expect(trapDraftFromPreset(needle).alsoFind).toEqual([]);
    const saved = {
      name: 'Fosso',
      description: '',
      trap: { findDc: 15, alsoFindSkillKeys: ['skill:arcana'] },
    } as never;
    expect(trapDraftOf(saved).alsoFind).toEqual(['skill:arcana']);
  });

  it('sends the keys in the whole spec, and a change of them is an edit', () => {
    const point = {
      name: 'Fosso',
      description: '',
      trap: { findDc: 15, alsoFindSkillKeys: [] },
    } as never;
    const d = { ...trapDraftOf(point), alsoFind: ['skill:arcana', 'skill:religion'] };
    expect(trapSpecOf(d).alsoFindSkillKeys).toEqual(['skill:arcana', 'skill:religion']);
    expect(isTrapDirty(d, point)).toBe(true);
    expect(trapChangesOf(d, point)?.trap?.alsoFindSkillKeys).toEqual([
      'skill:arcana',
      'skill:religion',
    ]);
    expect(isTrapDirty(trapDraftOf(point), point)).toBe(false);
  });

  it("offers every skill but the two that always find, in the list's order, and keeps the picks in it", () => {
    const all = [
      'skill:acrobatics',
      'skill:arcana',
      'skill:investigation',
      'skill:perception',
      'skill:religion',
    ].map((key) => ({ key }));
    const offered = alsoFindOptions(all).map((s) => s.key);
    expect(offered).toEqual(['skill:acrobatics', 'skill:arcana', 'skill:religion']);
    expect(toggleAlsoFind(['skill:religion'], 'skill:acrobatics', offered)).toEqual([
      'skill:acrobatics',
      'skill:religion',
    ]);
    expect(
      toggleAlsoFind(['skill:acrobatics', 'skill:religion'], 'skill:religion', offered),
    ).toEqual(['skill:acrobatics']);
  });
});

// Each error text of `trapErrors`, and the fallback of every missing part when a form is read from a half-filled preset or point.
describe('trapErrors, field by field', () => {
  const ok = { ...blankTrapDraft('A'), findDc: '15' };

  it('says what is wrong with a name: too long, or not on one line', () => {
    expect(trapErrors({ ...ok, name: 'a'.repeat(81) }).name).toBe('Use até 80 caracteres.');
    expect(trapErrors({ ...ok, name: 'a\nb' }).name).toBe('Use um nome numa linha só.');
    expect(trapErrors({ ...ok, name: ' ' }).name).toBe('Dê um nome ao ponto.');
    expect(hasTrapErrors(trapErrors(ok))).toBe(false);
  });

  it('counts characters, not code units, and limits the description', () => {
    expect(trapErrors({ ...ok, name: '🎲'.repeat(80) }).name).toBeUndefined();
    expect(trapErrors({ ...ok, description: 'x'.repeat(2001) }).description).toBe(
      'Use até 2.000 caracteres.',
    );
    expect(trapErrors({ ...ok, description: 'x'.repeat(2000) }).description).toBeUndefined();
  });

  it('limits what a condition lasts to 60 characters, on the part and on the failure', () => {
    const long = 'h'.repeat(61);
    const e = trapErrors({
      ...ok,
      conditions: [{ ...newCondition(), duration: long }, newCondition()],
      save: {
        ...newSave(),
        dc: '12',
        failDamage: [{ dice: 'x', typeKey: 'damage-type:fire' }],
        failCondition: { key: 'condition:prone', duration: long },
      },
    });
    expect(e.parts).toEqual({
      'condition:0': 'Use até 60 caracteres.',
      'fail:0': 'Use dados como 2d6 (de d4 a d12) ou um número de 1 a 100.',
      failCondition: 'Use até 60 caracteres.',
    });
  });

  it('checks the damage dice of the attack', () => {
    const e = trapErrors({
      ...ok,
      attack: { bonus: '5', count: '1', damage: { dice: 'oops', typeKey: 'damage-type:fire' } },
    });
    expect(e.attackDice).toContain('2d6');
    expect(e.attackBonus).toBeUndefined();
  });

  it('a save of "quem foi atingido" with an attack, and a clean failure, has no error', () => {
    const e = trapErrors({
      ...ok,
      attack: { bonus: '5', count: '1', damage: { dice: '1d6', typeKey: 'damage-type:fire' } },
      save: {
        ...newSave(),
        dc: '12',
        appliesTo: TrapSaveApplies.HIT,
        failDamage: [{ dice: '1d6', typeKey: 'damage-type:fire' }],
        onPass: TrapPassOutcome.HALF,
      },
    });
    expect(hasTrapErrors(e)).toBe(false);
  });
});

describe('the form from a preset or a point with parts missing', () => {
  it('fills an attack without damage with a blank bludgeoning part, and a save with every part', () => {
    const d = trapDraftFromPreset(
      create(TrapPresetSchema, {
        key: 'trap:x',
        namePt: 'X',
        noticeDc: 12,
        findDc: 14,
        areaSize: 4,
        trigger: TrapTrigger.MANUAL,
        effect: {
          targets: TrapTargets.MANUAL,
          attack: { bonus: 7, count: 2 },
          save: {
            ability: Ability.WISDOM,
            dc: 13,
            appliesTo: TrapSaveApplies.HIT,
            onPass: TrapPassOutcome.HALF,
            onFail: {
              damage: [{ dice: '1d4', damageTypeKey: 'damage-type:fire' }],
              condition: { conditionKey: 'condition:blinded', durationPt: '1 min' },
            },
          },
        },
      }),
    );
    expect(d).toMatchObject({
      noticeDc: '12',
      areaSize: 4,
      trigger: TrapTrigger.MANUAL,
      targets: TrapTargets.MANUAL,
      attack: { bonus: '7', count: '2', damage: { dice: '', typeKey: 'damage-type:bludgeoning' } },
      save: {
        ability: Ability.WISDOM,
        dc: '13',
        appliesTo: TrapSaveApplies.HIT,
        onPass: TrapPassOutcome.HALF,
        failDamage: [{ dice: '1d4', typeKey: 'damage-type:fire' }],
        failCondition: { key: 'condition:blinded', duration: '1 min' },
      },
    });
  });

  it('reads an area of 3, and falls back to a square, "ao entrar" and armed for what it does not know', () => {
    const three = trapDraftFromPreset(create(TrapPresetSchema, { areaSize: 3 }));
    expect(three.areaSize).toBe(3);
    expect(three.trigger).toBe(TrapTrigger.ENTER);
    const bare = trapDraftOf({ name: 'N', description: 'D', trap: undefined });
    expect(bare).toMatchObject({
      presetKey: '',
      findDc: '',
      noticeDc: '',
      areaSize: 1,
      trigger: TrapTrigger.ENTER,
      state: TrapState.ARMED,
      targets: TrapTargets.AREA,
      attack: null,
      save: null,
    });
    const fired = trapDraftOf(
      create(MapPointSchema, {
        name: 'N',
        trap: { findDc: 12, areaSize: 4, trigger: TrapTrigger.MANUAL, state: TrapState.TRIGGERED },
      }),
    );
    expect(fired).toMatchObject({
      areaSize: 4,
      trigger: TrapTrigger.MANUAL,
      state: TrapState.TRIGGERED,
    });
  });

  it('sends a changed description along with the spec', () => {
    const point = create(MapPointSchema, { name: 'N', description: 'old', trap: { findDc: 12 } });
    const changes = trapChangesOf({ ...trapDraftOf(point), description: 'new' }, point);
    expect(changes?.description).toBe('new');
    expect(changes?.name).toBeUndefined();
  });
});

describe('presetSummary, the other shapes', () => {
  it('names an attack with and without damage, the damage, and the ability of the save', () => {
    expect(
      presetSummary(
        create(TrapPresetSchema, {
          effect: {
            attack: { bonus: 5, damage: { dice: '1d6', damageTypePt: 'fogo' } },
            damage: [{ dice: '2d4', damageTypePt: 'veneno' }],
            save: { ability: Ability.CONSTITUTION },
          },
        }),
      ),
    ).toBe('Ataque +5, 1d6 fogo · 2d4 veneno · resistência de Constituição');
    expect(presetSummary(create(TrapPresetSchema, { effect: { attack: { bonus: 3 } } }))).toBe(
      'Ataque +3',
    );
    expect(
      presetSummary(
        create(TrapPresetSchema, { effect: { save: { ability: Ability.UNSPECIFIED } } }),
      ),
    ).toBe('Resistência de habilidade');
    expect(presetSummary(create(TrapPresetSchema, {}))).toBe('Só descrição');
  });
});
