import { create } from '@bufbuild/protobuf';

import {
  TableChoiceFilterSchema,
  TableEffectSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { choiceSourceMenu } from './content-testing-menu-choices';
import {
  featureLine,
  featureOffers,
  filterLine,
  filterMatches,
  toolSourceOf,
  withFilterKinds,
  withFromFeature,
  withToolSource,
} from './effect-choice-source';
import { draftToEffect, effectToDraft, emptyEffect, missingRequired } from './effect-draft';
import { visibleFields } from './effect-visible';

const menu = choiceSourceMenu();
const tool = () => ({ ...emptyEffect('choice'), choice: 'tool', count: 1 });
const feature = () => ({ ...emptyEffect('choice'), choice: 'feature', count: 1 });

describe('a choice of tools with a filter (TableEffect.filter = 96)', () => {
  it('starts with no filter and reads a stored one, copying the kinds', () => {
    expect(emptyEffect('choice')).toMatchObject({ filter: [], filterOn: false, fromFeature: '' });
    const stored = create(TableEffectSchema, {
      type: 'choice',
      choice: 'tool',
      filter: create(TableChoiceFilterSchema, { proficiencyKinds: ['weapon', 'other'] }),
    });
    const d = effectToDraft(stored);
    expect(d).toMatchObject({ filter: ['weapon', 'other'], filterOn: true });
    d.filter.push('tool');
    expect(stored.filter?.proficiencyKinds).toEqual(['weapon', 'other']);
    expect(
      effectToDraft(create(TableEffectSchema, { type: 'choice', choice: 'tool' })).filterOn,
    ).toBe(false);
  });

  it('sends the kinds as the filter, in place of the list, and nothing when none is ticked', () => {
    const d = withFilterKinds(withToolSource(tool(), 'filter'), menu, ['other', 'weapon']);
    const sent = draftToEffect(d, menu);
    expect(sent).toMatchObject({ type: 'choice', choice: 'tool', count: 1 });
    expect(sent.filter?.proficiencyKinds).toEqual(['weapon', 'other']);
    expect(sent.from).toBeUndefined();
    expect(draftToEffect(withToolSource(tool(), 'filter'), menu).filter).toBeUndefined();
  });

  it('reads the source from the switch, so a filter with no kind ticked is still the filter', () => {
    expect(toolSourceOf(tool())).toBe('list');
    expect(toolSourceOf(withToolSource(tool(), 'filter'))).toBe('filter');
  });

  it('clears the other source when the source changes', () => {
    const listed = { ...tool(), from: ['proficiency:dice-set'] };
    expect(withToolSource(listed, 'filter')).toMatchObject({
      filterOn: true,
      filter: [],
      from: [],
    });
    const filtered = withFilterKinds(withToolSource(tool(), 'filter'), menu, ['tool']);
    expect(withToolSource(filtered, 'list')).toMatchObject({
      filterOn: false,
      filter: [],
      from: [],
    });
  });

  it('orders the ticked kinds as the menu lists them', () => {
    expect(withFilterKinds(tool(), menu, ['other', 'tool', 'weapon']).filter).toEqual([
      'weapon',
      'tool',
      'other',
    ]);
  });

  it('counts what the filter matches today: the sizes of the lists of the ticked kinds', () => {
    expect(filterMatches(menu, ['weapon'])).toBe(37);
    expect(filterMatches(menu, ['weapon', 'tool'])).toBe(57);
    expect(filterLine(menu, ['weapon', 'tool'])).toBe('Oferece 57 proficiências hoje.');
    expect(filterLine(menu, ['other'])).toBe('Oferece 1 proficiência hoje.');
    expect(filterLine(menu, [])).toBe('Marque ao menos um tipo.');
  });

  it('asks for a kind before saving a filter, and not for a list', () => {
    expect(missingRequired(withToolSource(tool(), 'filter'), menu)).toEqual(['filter']);
    expect(
      missingRequired(withFilterKinds(withToolSource(tool(), 'filter'), menu, ['tool']), menu),
    ).toEqual([]);
    expect(missingRequired(tool(), menu)).toEqual([]);
  });

  it('reads the menu fields: `filter` and `from_feature` are in the choice type', () => {
    const names = visibleFields(menu.fieldsOf('choice'), tool()).map((f) => f.name);
    expect(names).toContain('filter');
    expect(names).toContain('from_feature');
  });
});

describe('a choice of features that offers the options of another feature (TableEffect.from_feature = 97)', () => {
  it('lists the SRD features with options and the table ones by name, with the invocations left out', () => {
    expect(featureOffers(menu)).toEqual(
      [
        { key: 'feature:pact-boon', label: 'Dádiva do pacto', count: 1 },
        { key: 'feature:fighting-style', label: 'Estilo de luta', count: 1 },
        {
          key: 'feature:class-guardiao-3f9a1c--truques@mesa',
          label: 'Truques de lâmina (Guarda do Corvo)',
          count: 3,
        },
      ].sort((a, b) => a.label.localeCompare(b.label, 'pt-BR')),
    );
  });

  it('writes the key picked and clears the list, the set and the filter', () => {
    const d = withFromFeature(
      { ...feature(), from: ['option:archery'], set: 'fighting_style' },
      'feature:fighting-style',
    );
    expect(d).toMatchObject({ fromFeature: 'feature:fighting-style', from: [], set: '' });
    expect(draftToEffect(d, menu)).toEqual({
      type: 'choice',
      choice: 'feature',
      count: 1,
      fromFeature: 'feature:fighting-style',
    });
    expect(withFromFeature(d, '').fromFeature).toBe('');
    expect(draftToEffect(withFromFeature(d, ''), menu).fromFeature).toBeUndefined();
  });

  it('reads a stored key', () => {
    const d = effectToDraft(
      create(TableEffectSchema, {
        type: 'choice',
        choice: 'feature',
        fromFeature: 'feature:fighting-style',
      }),
    );
    expect(d.fromFeature).toBe('feature:fighting-style');
  });

  it('says how many options the feature offers today, and what a key that left the list needs', () => {
    const offers = featureOffers(menu);
    expect(featureLine(offers, 'feature:class-guardiao-3f9a1c--truques@mesa')).toBe(
      'Oferece 3 opções hoje.',
    );
    expect(featureLine(offers, 'feature:fighting-style')).toBe('Oferece 1 opção hoje.');
    expect(featureLine(offers, 'feature:gone')).toBe(
      'Esta característica saiu da lista ou não tem mais opções. Escolha outra.',
    );
    expect(featureLine(offers, '')).toBe(
      'Vazio: a escolha usa a lista ou as opções desta característica.',
    );
  });
});
