import { type MessageInitShape, create, equals } from '@bufbuild/protobuf';

import {
  type TableEffect,
  TableEffectSchema,
  TableFeatSchema,
  TableFeatureSchema,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { powerAttackMenu } from './content-testing-menu-power';
import { effectToDraft, draftToEffect } from './effect-draft';
import { keptLines, keptView } from './effect-kept';
import { draftToFeat, featToDraft } from './feat-draft';
import { draftToFeature, featureToDraft } from './feature-draft';

const power = (): TableEffect =>
  create(TableEffectSchema, {
    type: 'power_attack',
    attackPenalty: 5,
    damageBonus: 10,
    resource: 'resource:fury',
    weapon: 'melee',
    weaponProperties: ['weapon-property:heavy', 'weapon-property:two-handed'],
    onlyProficient: true,
    textPt: 'Troca precisão por força.',
  });

describe('a power_attack the block writes goes back as stored', () => {
  it('a feat: changing only the name sends the power_attack effect proto-equal to the loaded one', () => {
    const draft = featToDraft(
      create(TableFeatSchema, { namePt: 'Golpe', descPt: ['Texto.'], effects: [power()] }),
    );
    draft.feature = { ...draft.feature, name: 'Golpe forte' };
    const sent = draftToFeat(draft, powerAttackMenu());
    expect(sent.namePt).toBe('Golpe forte');
    expect(sent.effects).toHaveLength(1);
    expect(equals(TableEffectSchema, create(TableEffectSchema, sent.effects?.[0]), power())).toBe(
      true,
    );
  });

  it('a feature: changing only the name sends the power_attack effect proto-equal to the loaded one', () => {
    const draft = featureToDraft(
      create(TableFeatureSchema, { namePt: 'Fúria', descPt: ['Texto.'], effects: [power()] }),
    );
    draft.name = 'Fúria maior';
    const sent = draftToFeature(draft, powerAttackMenu());
    expect(sent.namePt).toBe('Fúria maior');
    expect(sent.effects).toHaveLength(1);
    expect(equals(TableEffectSchema, create(TableEffectSchema, sent.effects?.[0]), power())).toBe(
      true,
    );
  });
});

describe('a power_attack outside what the block can show is kept as stored (the safety net)', () => {
  const outside: readonly [string, MessageInitShape<typeof TableEffectSchema>][] = [
    ['an attack penalty above 10', { attackPenalty: 11 }],
    ['an attack penalty of 0', { attackPenalty: 0 }],
    ['a damage bonus above 20', { damageBonus: 21 }],
    ['a damage bonus of 0', { damageBonus: 0 }],
    ['a weapon kind the menu does not have', { weapon: 'finesse' }],
    [
      'five properties',
      {
        weaponProperties: [
          'weapon-property:heavy',
          'weapon-property:light',
          'weapon-property:reach',
          'weapon-property:finesse',
          'weapon-property:versatile',
        ],
      },
    ],
  ];

  it.each(outside)(
    '%s: the draft keeps the stored effect, and the request sends it untouched',
    (_, over) => {
      const stored = create(TableEffectSchema, {
        type: 'power_attack',
        attackPenalty: 5,
        damageBonus: 10,
        weapon: 'melee',
        onlyProficient: true,
        ...over,
      });
      const draft = effectToDraft(stored);
      expect(draft.kept).toBe(stored);
      expect(
        equals(
          TableEffectSchema,
          create(TableEffectSchema, draftToEffect(draft, powerAttackMenu())),
          stored,
        ),
      ).toBe(true);
    },
  );

  it('shows the stored numbers as lines, in words, with the note that says why nothing is editable', () => {
    const draft = effectToDraft(
      create(TableEffectSchema, {
        type: 'power_attack',
        attackPenalty: 15,
        damageBonus: 10,
        weapon: 'ranged',
        weaponProperties: ['weapon-property:heavy'],
      }),
    );
    const names = (list: string, key: string) => powerAttackMenu().nameOf(list, key);
    expect(keptLines(draft, names)).toEqual([
      ['Penalidade no ataque', '−15'],
      ['Bônus no dano', '+10'],
      ['Arma', 'à distância'],
      ['Propriedades da arma', 'Pesada'],
      ['Só com arma proficiente', 'não'],
    ]);
    expect(keptView(draft, powerAttackMenu())?.note).toContain('fora do que o editor aceita');
  });

  it("keeps nothing of a power_attack inside the ranges: every field is the block's own", () => {
    const draft = effectToDraft(power());
    expect(draft.kept).toBeUndefined();
    expect(draft.attackPenalty).toBe(5);
    expect(draft.damageBonus).toBe(10);
    expect(draft.weaponProperties).toEqual(['weapon-property:heavy', 'weapon-property:two-handed']);
  });
});
