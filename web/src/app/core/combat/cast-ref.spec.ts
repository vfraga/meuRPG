import { create } from '@bufbuild/protobuf';

import {
  SpellGrantOptionSchema,
  SpellOptionSchema,
  SpellSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { castRefOf, castRefOfKey, optionOfRef, splitCastRef } from './cast-ref';
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
    expect(grantLine(granted.grant!)).toBe('Graça de teste · 2 usos');
    expect(grantLine(create(SpellGrantOptionSchema, { sourceNamePt: 'Dom', atWill: true }))).toBe(
      'Dom · À vontade',
    );
    expect(spellLine(granted)).toBe('Graça de teste · 2 usos');
    expect(
      grantLine(create(SpellGrantOptionSchema, { sourceNamePt: 'Dom', usesLeft: 3, usesCost: 2 })),
    ).toBe('Dom · gasta 2 · 3 usos');
  });
});
