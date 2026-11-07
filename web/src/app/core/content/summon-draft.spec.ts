import { create } from '@bufbuild/protobuf';

import {
  CastingTimeUnit,
  SpellDurationKind,
  SpellDurationUnit,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  TableSummonReplaces,
  TableSummonRole,
  TableSummonSchema,
  TableSummonTurnMode,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { emptyEffect } from './effect-draft';
import { emptyFeature } from './feature-draft';
import {
  draftToSummon,
  emptySummon,
  followResource,
  hasFixedOff,
  resetFixed,
  resourcesOf,
  summonBlockedPaths,
  summonPaths,
  summonToDraft,
} from './summon-draft';
import {
  durationOf,
  durationOptions,
  durationValue,
  timeOf,
  timeOptions,
  timeValue,
} from './summon-options';
import { creatureNameOf, rememberCreatureName, summonLine } from './summon-names';

const stored = () =>
  create(TableSummonSchema, {
    creatureKeys: ['creature:lobo-do-vale@mesa', 'monster:wolf'],
    count: 'mod("cha") + 1',
    countPerSlotLevel: 0,
    turnMode: TableSummonTurnMode.OWNER_ROLL_GROUP,
    role: TableSummonRole.COMPANION,
    maxLive: 2,
    replaces: TableSummonReplaces.SAME_SUMMON,
    resourceKey: 'companheiro',
    resourceCost: 1,
    castingTime: { unit: CastingTimeUnit.BONUS_ACTION, amount: 1 },
    duration: {
      kind: SpellDurationKind.TIMED,
      amount: 1,
      unit: SpellDurationUnit.HOUR,
      upTo: true,
    },
    concentration: true,
  });

describe('the summon draft (#578)', () => {
  it('turns a stored summon into a draft and back to the same message', () => {
    const s = stored();
    expect(draftToSummon(summonToDraft(s))).toEqual(s);
  });

  it('keeps what the block does not edit: the role, the cost, the count per slot level and an unknown enum value', () => {
    const s = create(TableSummonSchema, {
      creatureKeys: ['monster:wolf'],
      count: '1',
      role: TableSummonRole.MOUNT,
      countPerSlotLevel: 3,
      resourceCost: 5,
      turnMode: 9 as TableSummonTurnMode,
      replaces: 7 as TableSummonReplaces,
      resourceKey: 'x',
    });
    const back = draftToSummon(summonToDraft(s));
    expect(back.role).toBe(TableSummonRole.MOUNT);
    expect(back.countPerSlotLevel).toBe(3);
    expect(back.resourceCost).toBe(5);
    expect(back.turnMode).toBe(9);
    expect(back.replaces).toBe(7);
  });

  it('keeps an empty time as empty and an empty duration as empty when nothing touches them', () => {
    const s = create(TableSummonSchema, {
      creatureKeys: ['monster:wolf'],
      count: '1',
      resourceKey: 'x',
    });
    const back = draftToSummon(summonToDraft(s));
    expect(back.castingTime).toBeUndefined();
    expect(back.duration).toBeUndefined();
    expect(timeValue(summonToDraft(s).time)).toBe(timeValue(emptySummon().time));
  });

  it('sends the formula as typed and the most at a time as a number; text that is no number is refused by the server (-1)', () => {
    const d = { ...summonToDraft(stored()), count: ' 1 + slot() ', maxLive: '12' };
    expect(draftToSummon(d).count).toBe(' 1 + slot() ');
    expect(draftToSummon(d).maxLive).toBe(12);
    expect(draftToSummon({ ...d, maxLive: '' }).maxLive).toBe(0);
    expect(draftToSummon({ ...d, maxLive: '1,5' }).maxLive).toBe(-1);
    expect(draftToSummon({ ...d, maxLive: '-2' }).maxLive).toBe(-1);
  });

  it('a new summon costs 1 use and starts at the defaults of the board', () => {
    const s = draftToSummon(emptySummon('companheiro'));
    expect(s.resourceCost).toBe(1);
    expect(s.resourceKey).toBe('companheiro');
    expect(s.turnMode).toBe(TableSummonTurnMode.OWN_INITIATIVE);
    expect(s.replaces).toBe(TableSummonReplaces.NONE);
    expect(s.castingTime?.unit).toBe(CastingTimeUnit.ACTION);
    expect(s.duration?.kind).toBe(SpellDurationKind.INSTANTANEOUS);
    expect(s.concentration).toBe(false);
  });

  it('names the paths of the inputs it draws, under the feature', () => {
    expect(summonPaths('table_race.traits[0]')).toContain('table_race.traits[0].summon.max_live');
    expect(summonPaths('table_class_feature')).toContain(
      'table_class_feature.summon.creature_keys',
    );
  });
});

describe('the resources a summon spends', () => {
  const resource = (name: string, max: string) => ({
    ...emptyEffect('resource'),
    resource: name,
    max,
  });

  it('lists the resource effects of the feature: its name and the uses when it has one, the resource name when it has several', () => {
    const one = {
      ...emptyFeature(),
      name: 'Companheiro do Vale',
      effects: [resource('companheiro', '1')],
    };
    expect(resourcesOf(one)).toEqual([{ key: 'companheiro', label: 'Companheiro do Vale (1)' }]);
    const two = {
      ...one,
      effects: [resource('a', '2'), resource('b', 'prof()'), emptyEffect('note')],
    };
    expect(resourcesOf(two).map((r) => r.label)).toEqual(['a (2)', 'b (prof())']);
    expect(resourcesOf({ ...one, effects: [resource('  ', '1')] })).toEqual([]);
  });

  it("blocks the save only while no resource key is chosen at all; the class's key is the server's to check", () => {
    const f = { ...emptyFeature(), summon: emptySummon('') };
    expect(summonBlockedPaths('table_race.traits[0]', f)).toEqual([
      'table_race.traits[0].summon.resource_key',
    ]);
    expect(summonBlockedPaths('t', { ...f, summon: emptySummon('da_classe') })).toEqual([]);
    expect(summonBlockedPaths('t', { ...f, summon: null })).toEqual([]);
  });

  it("follows the new name of the feature's one resource, and never rewrites a key the feature did not define", () => {
    const own = {
      ...emptyFeature(),
      effects: [resource('companheiro', '1')],
      summon: emptySummon('companheiro'),
    };
    const renamed = { ...own, effects: [resource('companheiro2', '1')] };
    expect(followResource(own, renamed).summon?.resourceKey).toBe('companheiro2');
    const api = { ...own, summon: emptySummon('da_classe') };
    const edited = { ...api, effects: [resource('outro', '1')] };
    expect(followResource(api, edited)).toBe(edited);
    const two = { ...own, effects: [resource('a', '1'), resource('b', '1')] };
    expect(followResource(own, two)).toBe(two);
  });

  it('an API-written summon that spends a class resource goes back unchanged through edits of the feature', () => {
    const s = create(TableSummonSchema, {
      creatureKeys: ['monster:wolf'],
      count: '1',
      resourceKey: 'da_classe',
    });
    const f = { ...emptyFeature(), name: 'x', summon: summonToDraft(s) };
    const edited = followResource(f, { ...f, effects: [resource('nova', '2')] });
    expect(edited.summon?.resourceKey).toBe('da_classe');
    expect(draftToSummon(edited.summon!).resourceKey).toBe('da_classe');
    expect(summonBlockedPaths('t', edited)).toEqual([]);
  });

  it('knows a stored role, cost or creatures per slot level the block cannot change, and resets them', () => {
    const odd = create(TableSummonSchema, {
      creatureKeys: ['monster:wolf'],
      count: '1',
      resourceKey: 'x',
      role: TableSummonRole.MOUNT,
      resourceCost: 3,
      countPerSlotLevel: 2,
    });
    const d = summonToDraft(odd);
    expect(hasFixedOff(d)).toBe(true);
    const fixed = resetFixed(d);
    expect(hasFixedOff(fixed)).toBe(false);
    const out = draftToSummon(fixed);
    expect([out.role, out.resourceCost, out.countPerSlotLevel]).toEqual([
      TableSummonRole.COMPANION,
      1,
      0,
    ]);
    expect(hasFixedOff(summonToDraft(stored()))).toBe(false);
    expect(hasFixedOff(emptySummon('x'))).toBe(false);
  });
});

const instantDuration = () => ({
  kind: SpellDurationKind.INSTANTANEOUS,
  amount: 0,
  unit: 0 as const,
  upTo: false,
});

describe('the time and duration lists', () => {
  it('offers the presets and adds a stored value they do not have', () => {
    expect(timeOptions({ unit: CastingTimeUnit.ACTION, amount: 1, triggerPt: '' })[0].label).toBe(
      '1 ação',
    );
    const odd = timeOptions({ unit: CastingTimeUnit.MINUTE, amount: 3, triggerPt: '' });
    expect(odd[odd.length - 1].label).toBe('3 minutos');
    const dur = durationOptions({
      kind: SpellDurationKind.TIMED,
      amount: 3,
      unit: SpellDurationUnit.ROUND,
      upTo: false,
    });
    expect(dur[dur.length - 1].label).toBe('3 rodadas');
  });

  it('does not hide a stored value a preset does not represent', () => {
    const upTo = {
      kind: SpellDurationKind.TIMED,
      amount: 1,
      unit: SpellDurationUnit.HOUR,
      upTo: true,
    };
    expect(durationValue(upTo)).not.toBe(durationValue({ ...upTo, upTo: false }));
    expect(durationOptions(upTo).some((o) => o.label === 'Até 1 hora')).toBe(true);
    const odd = { kind: SpellDurationKind.INSTANTANEOUS, amount: 5, unit: 0 as const, upTo: false };
    expect(durationValue(odd)).not.toBe(String(SpellDurationKind.INSTANTANEOUS));
    expect(timeValue({ unit: CastingTimeUnit.ACTION, amount: 0, triggerPt: '' })).not.toBe(
      timeValue({ unit: CastingTimeUnit.ACTION, amount: 1, triggerPt: '' }),
    );
    expect(durationOptions(instantDuration())[7].label).toBe('Até ser dissipada');
  });

  it('reads a value back, keeping a stored trigger only while the unit stays', () => {
    const reaction = { unit: CastingTimeUnit.REACTION, amount: 1, triggerPt: 'quando sofre dano' };
    expect(timeOf(timeValue(reaction), reaction).triggerPt).toBe('quando sofre dano');
    expect(timeOf(`${CastingTimeUnit.HOUR}:1`, reaction)).toEqual({
      unit: CastingTimeUnit.HOUR,
      amount: 1,
      triggerPt: '',
    });
    const custom = {
      kind: SpellDurationKind.TIMED,
      amount: 3,
      unit: SpellDurationUnit.ROUND,
      upTo: true,
    };
    expect(durationOf(durationValue(custom), custom)).toBe(custom);
    expect(durationOf(String(SpellDurationKind.INSTANTANEOUS), custom).kind).toBe(
      SpellDurationKind.INSTANTANEOUS,
    );
  });
});

describe('the names of the creatures a summon lists', () => {
  const nameOf = (k: string) => (k === 'creature:lobo-do-vale@mesa' ? 'Lobo do Vale' : k);

  it('takes a table creature from the catalog, an SRD one from what the bestiary said, else from its slug', () => {
    expect(creatureNameOf('creature:lobo-do-vale@mesa', nameOf)).toBe('Lobo do Vale');
    expect(creatureNameOf('monster:giant-wolf-spider', nameOf)).toBe('Giant wolf spider');
    rememberCreatureName('monster:giant-wolf-spider', 'Aranha-lobo gigante');
    expect(creatureNameOf('monster:giant-wolf-spider', nameOf)).toBe('Aranha-lobo gigante');
  });

  it("remembers only the SRD's names: a table creature's name is its entry's and never crosses a campaign", () => {
    rememberCreatureName('creature:outra-mesa@mesa', 'Nome de outra mesa');
    expect(creatureNameOf('creature:outra-mesa@mesa', (k) => k)).toBe('Creature:outra mesa@mesa');
  });

  it('says "Invoca: Lobo do Vale, Unheard of." and nothing for a feature with no summon', () => {
    const unheard = create(TableSummonSchema, {
      creatureKeys: ['creature:lobo-do-vale@mesa', 'monster:unheard-of'],
    });
    expect(summonLine(unheard, nameOf)).toBe('Invoca: Lobo do Vale, Unheard of.');
    expect(summonLine(undefined, nameOf)).toBe('');
    expect(summonLine({ creatureKeys: [] }, nameOf)).toBe('');
  });
});
