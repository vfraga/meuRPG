import { create } from '@bufbuild/protobuf';

import { TableEffectSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { menu } from './content-testing';
import {
  draftToEffect,
  effectToDraft,
  emptyEffect,
  missingRequired,
  rangeFeet,
  unreadableRange,
  rangeMeters,
  splitTags,
} from './effect-draft';

describe("an effect from the server's menu (ADR-0018, section 4)", () => {
  const m = menu();

  it('knows the fields of each type from the menu, in its order, and the names of its lists', () => {
    expect(m.fieldsOf('proficiency').map((f) => f.name)).toEqual([
      'proficiency',
      'level',
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
    expect(m.fromOptions('feature').map((o) => o.namePt)).toEqual(['Estilo de luta']);
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
    for (const text of ['18 m', '18m', '1.000,5', 'abc', '-3']) {
      expect(unreadableRange(text), text).toBe(true);
    }
    for (const text of ['', '  ', '0', '18', '4,5', '4.5']) {
      expect(unreadableRange(text), text).toBe(false);
    }
  });
});
