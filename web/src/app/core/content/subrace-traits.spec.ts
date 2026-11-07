import { create } from '@bufbuild/protobuf';

import { TableContentKind, TableRaceSchema } from '../../../gen/meurpg/rules/v1/table_content_pb';
import { entry, feature } from './content-testing';
import {
  parentFeaturesOf,
  parentTraitsOf,
  parentViewOf,
  replacedNames,
  replacesLine,
  srdParentTraits,
  traitRows,
  traitSummary,
  withReplaced,
} from './subrace-traits';

describe('the traits a subrace replaces', () => {
  const nameOf = (key: string) => ({ 'skill:perception': 'Percepção' })[key] ?? key;
  const withOptions = create(TableRaceSchema, {
    traits: [
      {
        key: 'trait:sentidos@mesa',
        namePt: 'Sentidos',
        options: [{ namePt: 'A' }, { namePt: 'B' }],
      },
    ],
  }).traits[0];
  const race = entry(TableContentKind.RACE, 'Corujeiro', {
    key: 'race:corujeiro@mesa',
    body: {
      case: 'tableRace',
      value: create(TableRaceSchema, {
        traits: [
          feature('Olhos', [{ type: 'proficiency', proficiency: 'skill:perception' }]),
          feature('Planar'),
          { ...feature('Novo'), key: '' },
        ],
      }),
    },
  });

  it('says what a trait does in one line', () => {
    expect(
      traitSummary(
        feature('Olhos', [{ type: 'proficiency', proficiency: 'skill:perception' }]),
        nameOf,
      ),
    ).toBe('Proficiência em Percepção');
    expect(traitSummary(feature('Planar'), nameOf)).toBe('Só texto');
    expect(traitSummary(feature('Pele', [{ type: 'modifier' }]), nameOf)).toBe('Com efeito');
    expect(traitSummary(withOptions, nameOf)).toBe('Escolha entre 2 opções');
  });

  it("reads a table race's saved traits, and nothing for a race the web has no traits of", () => {
    expect(parentFeaturesOf([race], 'race:corujeiro@mesa')?.map((t) => t.namePt)).toEqual([
      'Olhos',
      'Planar',
    ]);
    expect(parentFeaturesOf([race], 'race:human')).toBeNull();
    expect(parentTraitsOf(parentFeaturesOf([race], 'race:corujeiro@mesa')!, nameOf)[1]).toEqual({
      key: 'feature:Planar',
      name: 'Planar',
      summary: 'Só texto',
    });
  });

  it("gives a reader the race's traits it has, and no trait for a race it has not", () => {
    const view = parentViewOf([race]);
    expect(view('race:corujeiro@mesa').traits.map((t) => t.namePt)).toEqual(['Olhos', 'Planar']);
    expect(view('race:human').traits).toEqual([]);
  });

  it('lists every parent trait, then a marked key that is no parent trait as a stale row', () => {
    const parent = parentTraitsOf(parentFeaturesOf([race], 'race:corujeiro@mesa')!, nameOf);
    const rows = traitRows(parent, ['feature:Olhos', 'trait:sumiu@mesa']);
    expect(rows.map((r) => [r.key, r.checked, r.stale])).toEqual([
      ['feature:Olhos', true, false],
      ['feature:Planar', false, false],
      ['trait:sumiu@mesa', true, true],
    ]);
  });

  it('marks and unmarks a key once, keeping the order', () => {
    expect(withReplaced(['a'], 'b', true)).toEqual(['a', 'b']);
    expect(withReplaced(['a', 'b'], 'a', true)).toEqual(['b', 'a']);
    expect(withReplaced(['a', 'b'], 'a', false)).toEqual(['b']);
  });

  it('names only the replaced traits the reader can see', () => {
    const parent = [{ key: 'a', name: 'Olhos de caçador', summary: '' }];
    expect(replacedNames(parent, ['a', 'hidden'])).toEqual(['Olhos de caçador']);
    expect(replacedNames([], ['hidden'])).toEqual([]);
  });

  it("writes the master's line of what is replaced", () => {
    expect(replacesLine(['Olhos de caçador'], 'Corujeiro')).toBe(
      'Olhos de caçador e os bônus de Corujeiro',
    );
    expect(replacesLine(['A', 'B'], 'Corujeiro')).toBe('A, B e os bônus de Corujeiro');
    expect(replacesLine(['A', 'B'], '')).toBe('A e B');
    expect(replacesLine([], 'Corujeiro')).toBe('os bônus de Corujeiro');
    expect(replacesLine([], '')).toBe('');
  });

  it("takes an SRD race's traits from the menu's race_traits, only the ones of that race", () => {
    const list = [
      { key: 'trait:darkvision', namePt: 'Visão', hintPt: 'Vê no escuro.', group: 'race:elf' },
      { key: 'trait:darkvision', namePt: 'Visão', hintPt: 'Vê no escuro.', group: 'race:gnome' },
      { key: 'trait:lucky', namePt: 'Sortudo', hintPt: '', group: 'race:halfling' },
    ];
    expect(srdParentTraits(list, 'race:elf')).toEqual([
      { key: 'trait:darkvision', name: 'Visão', summary: 'Vê no escuro.' },
    ]);
    expect(srdParentTraits(list, 'race:human')).toEqual([]);
    const rows = traitRows(srdParentTraits(list, 'race:elf'), ['trait:darkvision', 'trait:gone']);
    expect(rows.map((r) => [r.key, r.checked, r.stale])).toEqual([
      ['trait:darkvision', true, false],
      ['trait:gone', true, true],
    ]);
  });
});
