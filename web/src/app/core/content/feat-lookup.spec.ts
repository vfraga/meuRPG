import { create } from '@bufbuild/protobuf';

import {
  TableBackgroundSchema,
  TableClassFeatureSchema,
  TableContentKind,
  TableFeatSchema,
  TableRaceSchema,
  TableSubclassSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { catalog, entry } from './content-testing';
import { menu } from './content-testing-menu';
import { lackChips, lackOptions, pickableLacks, pickableRaces, raceChips } from './feat-lookup';
import { askedRaces } from './feat-text';

const nameOf = catalog().nameOf;

function tableEntries() {
  const feat = entry(TableContentKind.FEAT, 'Pegada Firme', {
    body: { case: 'tableFeat', value: create(TableFeatSchema, { namePt: 'Pegada Firme' }) },
  });
  const old = entry(TableContentKind.FEAT, 'Pegada Velha', {
    archived: true,
    body: { case: 'tableFeat', value: create(TableFeatSchema, { namePt: 'Pegada Velha' }) },
  });
  const cf = entry(TableContentKind.CLASS_FEATURE, 'Golpe Mestre', {
    body: {
      case: 'tableClassFeature',
      value: create(TableClassFeatureSchema, {
        namePt: 'Golpe Mestre',
        classKey: 'class:wizard',
        level: 3,
      }),
    },
  });
  const race = entry(TableContentKind.RACE, 'Corujeiro', {
    body: {
      case: 'tableRace',
      value: create(TableRaceSchema, {
        namePt: 'Corujeiro',
        traits: [{ key: 'trait:olhos@mesa', namePt: 'Olhos de coruja' }],
      }),
    },
  });
  const bg = entry(TableContentKind.BACKGROUND, 'Cartógrafo', {
    body: {
      case: 'tableBackground',
      value: create(TableBackgroundSchema, {
        namePt: 'Cartógrafo',
        feature: { key: 'bgfeature:mapas@mesa', namePt: 'Mapas' },
      }),
    },
  });
  const sub = entry(TableContentKind.SUBCLASS, 'Caminho', {
    body: {
      case: 'tableSubclass',
      value: create(TableSubclassSchema, {
        namePt: 'Caminho',
        levels: [{ level: 5, features: [{ key: 'feature:passo@mesa', namePt: 'Passo' }] }],
      }),
    },
  });
  return [feat, old, cf, race, bg, sub];
}

describe('the features a feat may ask not to have', () => {
  const all = () => lackOptions(menu().list('lackable_features'), tableEntries(), nameOf);

  it("groups the SRD values by their origin and adds the table's own entries", () => {
    const byName = (n: string) => all().find((o) => o.label === n)!;
    expect(byName('Agarrador').group).toBe('feat');
    expect(byName('Agarrar com firmeza').group).toBe('feature');
    expect(byName('Agarrar com firmeza').hint).toBe('Monge, nível 6');
    expect(byName('Visão no Escuro').group).toBe('trait');
    expect(byName('Pegada Firme')).toMatchObject({ group: 'feat', hint: 'talento da mesa' });
    expect(byName('Golpe Mestre')).toMatchObject({
      group: 'feature',
      hint: 'Mago da mesa, nível 3',
    });
    expect(byName('Olhos de coruja')).toMatchObject({ group: 'trait', hint: 'traço de Corujeiro' });
    expect(byName('Mapas').group).toBe('feature');
    expect(byName('Passo').hint).toBe('Caminho da mesa, nível 5');
  });

  it('offers neither the archived, nor the feat being edited, nor the one already chosen', () => {
    const labels = (own: string, chosen: string) =>
      pickableLacks(all(), own, chosen).map((o) => o.label);
    expect(labels('', '')).not.toContain('Pegada Velha');
    expect(labels('', '')).toContain('Pegada Firme');
    expect(labels('feat:pegada-firme@mesa', '')).not.toContain('Pegada Firme');
    expect(labels('', 'feat:grappler')).not.toContain('Agarrador');
  });

  it('writes the chip with the name and the origin of a listed feature', () => {
    expect(lackChips('feat:grappler', all(), nameOf)).toEqual([
      {
        value: 'feat:grappler',
        label: 'Agarrador',
        hint: 'talento do SRD',
        tag: '',
        invalid: false,
      },
    ]);
    expect(lackChips('', all(), nameOf)).toEqual([]);
  });

  it('keeps the name of an archived feature on its chip, with the origin', () => {
    expect(lackChips('feat:pegada-velha@mesa', all(), nameOf)[0]).toMatchObject({
      label: 'Pegada Velha',
      hint: 'talento da mesa',
    });
  });

  it('names a key no list knows by the catalog, with no origin, and marks a refused one', () => {
    const [chip] = lackChips('feat:gone', all(), (k) => `Nome de ${k}`, true);
    expect(chip).toMatchObject({ label: 'Nome de feat:gone', hint: '', invalid: true });
  });

  it('groups an SRD value by the prefix of its key, whatever its hint says', () => {
    const odd = lackOptions(
      [
        { key: 'feat:x', namePt: 'X', hintPt: 'qualquer coisa' },
        { key: 'trait:y', namePt: 'Y', hintPt: '' },
        { key: 'feature:z', namePt: 'Z', hintPt: 'talento do SRD' },
      ],
      [],
      nameOf,
    );
    expect(odd.map((o) => o.group)).toEqual(['feat', 'trait', 'feature']);
  });
});

describe('the races a feat asks for', () => {
  it('reads the single race and the list as one, each once', () => {
    expect(askedRaces('race:human', ['race:elf', 'race:human'])).toEqual([
      'race:human',
      'race:elf',
    ]);
    expect(askedRaces('', ['race:elf'])).toEqual(['race:elf']);
  });

  it('marks a race the master switched off or archived "Desligada"', () => {
    const off = entry(TableContentKind.RACE, 'Corujeiro');
    off.off = true;
    const archived = entry(TableContentKind.RACE, 'Antiga', { archived: true });
    const on = entry(TableContentKind.RACE, 'Ativa');
    const chips = raceChips(
      [off.key, archived.key, on.key, 'race:human'],
      [off, archived, on],
      (k) => k.replace(/^race:/, ''),
    );
    expect(chips.map((c) => c.tag)).toEqual(['Desligada', 'Desligada', '', '']);
  });

  it('marks the chips whose refusal is known, with the path it lands on', () => {
    const chips = raceChips(
      ['race:a', 'race:b'],
      [],
      (k) => k,
      new Map([['race:b', 'p.races[1]']]),
    );
    expect(chips.map((c) => [c.invalid, c.path])).toEqual([
      [false, undefined],
      [true, 'p.races[1]'],
    ]);
  });

  it("offers the catalog's races and sub-races without the ones already asked", () => {
    const choices = [
      { value: 'race:human', label: 'Humano' },
      { value: 'subrace:x', label: 'X (sub-raça)' },
    ];
    expect(pickableRaces(choices, ['race:human']).map((o) => o.value)).toEqual(['subrace:x']);
  });
});
