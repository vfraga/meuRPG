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
  blankTrapDraft,
  hasTrapErrors,
  isTrapDirty,
  newAttack,
  newDamage,
  newSave,
  presetSummary,
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
