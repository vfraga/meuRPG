import type { FullSheet } from '../../../gen/meurpg/characters/v1/characters_pb';
import { create } from '@bufbuild/protobuf';
import { DerivedSheetSchema, FeatureSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { featuresVm } from './sheet-lost';

describe('featuresVm', () => {
  it('carries the name of what offers an option to its row (board 580-05)', () => {
    const derived = create(DerivedSheetSchema, {
      features: [
        create(FeatureSchema, {
          key: 'feat:truques',
          namePt: 'Truques de lâmina',
          sourcePt: 'Talento',
        }),
        create(FeatureSchema, {
          key: 'feat:truques#asa',
          namePt: 'Asa Partida',
          sourcePt: 'Talento',
          optionOfPt: 'Truques de lâmina',
          optionOfKey: 'feat:truques',
        }),
      ],
    });
    const rows = featuresVm(
      { featKeys: [], featureChoiceKeys: [] } as unknown as FullSheet,
      derived,
    );
    expect(rows.map((r) => r.optionOf)).toEqual(['', 'Truques de lâmina']);
  });
});

describe('featuresVm, the order of the rows', () => {
  const row = (key: string, namePt: string, optionOf?: { pt: string; key: string }) =>
    create(FeatureSchema, {
      key,
      namePt,
      sourcePt: 'Guerreiro 1',
      optionOfPt: optionOf?.pt ?? '',
      optionOfKey: optionOf?.key ?? '',
    });
  const names = (features: ReturnType<typeof row>[]) =>
    featuresVm(
      { featKeys: [], featureChoiceKeys: [] } as unknown as FullSheet,
      create(DerivedSheetSchema, { features }),
    ).map((r) => r.name);

  it('puts an option right under the feature that offers it, the options in their own order', () => {
    expect(
      names([
        row('feature:estilo', 'Estilo de Luta'),
        row('feat:truques', 'Truques de lâmina'),
        row('feature:estilo-grandes', 'Estilo de Luta: Armas Grandes', {
          pt: 'Estilo de Luta',
          key: 'feature:estilo',
        }),
        row('feat:truques#asa', 'Asa Partida', { pt: 'Truques de lâmina', key: 'feat:truques' }),
        row('feature:estilo-defesa', 'Estilo de Luta: Defesa', {
          pt: 'Estilo de Luta',
          key: 'feature:estilo',
        }),
        row('feature:fúria', 'Fúria'),
      ]),
    ).toEqual([
      'Estilo de Luta',
      'Estilo de Luta: Armas Grandes',
      'Estilo de Luta: Defesa',
      'Truques de lâmina',
      'Asa Partida',
      'Fúria',
    ]);
  });

  it('keeps an option whose feature is not on the sheet where it is', () => {
    expect(
      names([
        row('a', 'A'),
        row('o', 'Opção', { pt: 'Sumiu', key: 'feature:sumiu' }),
        row('b', 'B'),
      ]),
    ).toEqual(['A', 'Opção', 'B']);
  });

  it('puts each option under the feature of its own key when two features have one name', () => {
    // A Fighter 1 / Paladin 2: both classes have a feature called "Estilo de Luta".
    expect(
      names([
        row('feature:fighter-fighting-style', 'Estilo de Luta'),
        row('feature:paladin-fighting-style', 'Estilo de Luta'),
        row('feature:fighter-fighting-style-defense', 'Estilo de Luta: Defesa', {
          pt: 'Estilo de Luta',
          key: 'feature:fighter-fighting-style',
        }),
        row('feature:fighting-style-dueling', 'Estilo de Luta: Duelismo', {
          pt: 'Estilo de Luta',
          key: 'feature:paladin-fighting-style',
        }),
      ]),
    ).toEqual([
      'Estilo de Luta',
      'Estilo de Luta: Defesa',
      'Estilo de Luta',
      'Estilo de Luta: Duelismo',
    ]);
  });
});
