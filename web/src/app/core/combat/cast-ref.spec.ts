import { create } from '@bufbuild/protobuf';

import {
  SpellGrantOptionSchema,
  SpellOptionSchema,
  SpellSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { SpellTargetsSchema } from '../../../gen/meurpg/play/v1/combat_pb';
import {
  castRefOf,
  castRefOfKey,
  optionOfRef,
  splitCastRef,
  targetsOfOption,
  targetsOfRef,
} from './cast-ref';
import { grantLine, spellLine } from './combat-options';

const own = create(SpellOptionSchema, {
  spell: create(SpellSchema, { key: 'spell:bless', level: 1 }),
  enabled: true,
});
const granted = create(SpellOptionSchema, {
  spell: create(SpellSchema, { key: 'spell:bless', level: 1 }),
  enabled: true,
  grant: create(SpellGrantOptionSchema, {
    sourceKey: 'feat:graca@mesa',
    sourceNamePt: 'Graça de teste',
    usesLeft: 2,
  }),
});

describe('the reference of a spell the character can cast', () => {
  it('is the key for a class spell and the key with the grant for a granted one', () => {
    expect(castRefOf(own)).toBe('spell:bless');
    expect(castRefOf(granted)).toBe('spell:bless#granted=feat:graca@mesa');
    expect(splitCastRef(castRefOf(granted))).toEqual({
      key: 'spell:bless',
      grantedBy: 'feat:graca@mesa',
    });
    expect(splitCastRef('spell:bless')).toEqual({ key: 'spell:bless', grantedBy: '' });
  });

  it('is built from a key and the granting source when only those are at hand', () => {
    expect(castRefOfKey('spell:bless', '')).toBe('spell:bless');
    expect(castRefOfKey('spell:bless', 'feat:graca@mesa')).toBe(castRefOf(granted));
    expect(splitCastRef(castRefOfKey('spell:bless', 'feat:graca@mesa'))).toEqual({
      key: 'spell:bless',
      grantedBy: 'feat:graca@mesa',
    });
  });

  it('finds the option a reference names, the same spell twice apart', () => {
    const list = [own, granted];
    expect(optionOfRef(list, 'spell:bless')).toBe(own);
    expect(optionOfRef(list, castRefOf(granted))).toBe(granted);
    expect(optionOfRef([granted], 'spell:bless')).toBeUndefined();
  });
});

describe('the line of a granted spell', () => {
  it('says the source and the uses', () => {
    expect(grantLine(granted.grant!, 1)).toBe('Graça de teste · 2 usos');
    expect(
      grantLine(create(SpellGrantOptionSchema, { sourceNamePt: 'Dom', atWill: true }), 1),
    ).toBe('Dom · À vontade');
    expect(spellLine(granted)).toBe('Graça de teste · 2 usos');
    expect(
      grantLine(
        create(SpellGrantOptionSchema, { sourceNamePt: 'Dom', usesLeft: 3, usesCost: 2 }),
        1,
      ),
    ).toBe('Dom · gasta 2 · 3 usos');
  });

  it('says the level it is cast at only when above the spell’s own', () => {
    const at = (castLevel: number, level = 1) =>
      grantLine(
        create(SpellGrantOptionSchema, { sourceNamePt: 'Legado', usesLeft: 1, castLevel }),
        level,
      );
    expect(at(2)).toBe('Legado · 1 uso · no\u00a02º\u00a0nível');
    expect(at(1)).toBe('Legado · 1 uso');
    expect(at(0, 0)).toBe('Legado · 1 uso');
    const atWill = create(SpellGrantOptionSchema, {
      sourceNamePt: 'Legado',
      atWill: true,
      castLevel: 3,
    });
    expect(grantLine(atWill, 1)).toBe('Legado · À vontade · no\u00a03º\u00a0nível');
  });
});

describe('targetsOfOption', () => {
  const opt = (key: string, over: object = {}) =>
    create(SpellOptionSchema, { spell: { key }, enabled: true, ...over } as never);
  const entry = (key: string, maxTargets: number) =>
    create(SpellTargetsSchema, { spellKey: key, maxTargets });
  const klass = opt('spell:bless');
  const grant = opt('spell:bless', { grant: { sourceKey: 'feat:graca@mesa', castLevel: 2 } });
  const other = opt('spell:fog-cloud');
  const targets = [entry('spell:bless', 3), entry('spell:fog-cloud', 0), entry('spell:bless', 4)];

  it('gives the n-th entry of the key, as the options come', () => {
    const spells = [klass, other, grant];
    expect(targetsOfOption(spells, targets, klass)?.maxTargets).toBe(3);
    expect(targetsOfOption(spells, targets, grant)?.maxTargets).toBe(4);
    expect(targetsOfRef(spells, targets, 'spell:bless#granted=feat:graca@mesa')?.maxTargets).toBe(
      4,
    );
    expect(targetsOfRef(spells, targets, 'spell:bless')?.maxTargets).toBe(3);
  });

  it('skips an option that is not enabled, which has no entry', () => {
    const off = opt('spell:bless', { enabled: false });
    const spells = [off, grant];
    expect(targetsOfOption(spells, [entry('spell:bless', 4)], grant)?.maxTargets).toBe(4);
    expect(targetsOfOption(spells, [entry('spell:bless', 4)], off)).toBeUndefined();
  });
});
