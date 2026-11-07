import { create } from '@bufbuild/protobuf';

import {
  TableEffectSchema,
  TableFeatSchema,
  TableClassFeatureSchema,
  TableOptionSchema,
  TableContentKind,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { readEntry } from './content-read';
import { effectLine } from './content-read-parts';
import { rollEffectItem } from './content-read-rolls';
import { catalog, entry } from './content-testing';
import { rollsMenu } from './content-testing-menu-rolls';
import {
  draftToEffect,
  effectToDraft,
  emptyEffect,
  missingRequired,
  unreadableNumberPaths,
} from './effect-draft';
import { EFFECT_WRITERS } from './effect-writers';
import { neverFolded, visibleFields } from './effect-visible';

const menu = rollsMenu();
const effect = (init: Parameters<typeof create<typeof TableEffectSchema>>[1]) =>
  create(TableEffectSchema, init);
const visible = (d: Parameters<typeof visibleFields>[1]) =>
  visibleFields(menu.fieldsOf(d.type), d).map((f) => f.name);

describe('reroll and roll_minimum in the draft (TableEffect.rolls = 77, at_most = 78)', () => {
  it('starts empty and reads a stored effect, copying the list', () => {
    expect(emptyEffect('reroll')).toMatchObject({ rolls: [], atMost: 0 });
    const stored = effect({
      type: 'reroll',
      rolls: ['attack', 'save'],
      atMost: 2,
      resource: 'sorte',
      cost: '1',
    });
    const d = effectToDraft(stored);
    expect(d).toMatchObject({ rolls: ['attack', 'save'], atMost: 2, resource: 'sorte', cost: '1' });
    d.rolls.push('damage');
    expect(stored.rolls).toEqual(['attack', 'save']);
  });

  it("sends a reroll's rolls, at_most, resource and cost, and nothing it does not read", () => {
    const d = {
      ...emptyEffect('reroll'),
      rolls: ['attack', 'save'],
      atMost: 2,
      resource: 'sorte',
      cost: ' 1 ',
      value: '5',
      damageType: 'damage-type:fire',
    };
    expect(draftToEffect(d, menu)).toEqual({
      type: 'reroll',
      rolls: ['attack', 'save'],
      atMost: 2,
      resource: 'sorte',
      cost: '1',
    });
  });

  it("sends a minimum's rolls, value and damage type, and leaves out an empty list, a 0 or an unreadable face", () => {
    const d = {
      ...emptyEffect('roll_minimum'),
      rolls: ['damage'],
      value: '2',
      damageType: 'damage-type:fire',
    };
    expect(draftToEffect(d, menu)).toEqual({
      type: 'roll_minimum',
      rolls: ['damage'],
      value: '2',
      damageType: 'damage-type:fire',
    });
    expect(EFFECT_WRITERS.get('rolls')!(emptyEffect('reroll'))).toEqual({});
    expect(EFFECT_WRITERS.get('at_most')!(emptyEffect('reroll'))).toEqual({});
    expect(EFFECT_WRITERS.get('at_most')!({ ...emptyEffect('reroll'), atMost: NaN })).toEqual({});
    expect(EFFECT_WRITERS.get('at_most')!({ ...emptyEffect('reroll'), atMost: 20 })).toEqual({
      atMost: 20,
    });
  });

  it('copies the list it sends, so a later change of the draft never changes the request', () => {
    const d = { ...emptyEffect('reroll'), rolls: ['attack'], atMost: 1 };
    const sent = draftToEffect(d, menu);
    d.rolls.push('save');
    expect(sent.rolls).toEqual(['attack']);
  });

  it('says which required fields are empty', () => {
    expect(missingRequired(emptyEffect('reroll'), menu)).toEqual(['rolls', 'at_most']);
    expect(missingRequired({ ...emptyEffect('reroll'), rolls: ['save'], atMost: 2 }, menu)).toEqual(
      [],
    );
    expect(missingRequired(emptyEffect('roll_minimum'), menu)).toEqual(['rolls', 'value']);
  });

  it('keeps an unreadable face from being saved as "not set"', () => {
    const d = { ...emptyEffect('reroll'), rolls: ['save'], atMost: NaN };
    expect(unreadableNumberPaths('table_feat.effects', [d], menu)).toEqual([
      'table_feat.effects[0].at_most',
    ]);
  });
});

describe('which fields a reroll, a minimum and a modifier show', () => {
  it("shows a reroll's cost only with a resource, or when it holds a value", () => {
    expect(visible(emptyEffect('reroll'))).toEqual([
      'rolls',
      'at_most',
      'resource',
      'when',
      'text_pt',
    ]);
    expect(visible({ ...emptyEffect('reroll'), resource: 'sorte' })).toContain('cost');
    expect(visible({ ...emptyEffect('reroll'), cost: '2' })).toContain('cost');
  });

  it("shows a minimum's damage type only when damage is the only roll, or when it holds a value", () => {
    const base = emptyEffect('roll_minimum');
    expect(visible(base)).not.toContain('damage_type');
    expect(visible({ ...base, rolls: ['damage'] })).toContain('damage_type');
    expect(visible({ ...base, rolls: ['attack', 'damage'] })).not.toContain('damage_type');
    expect(visible({ ...base, rolls: ['check'] })).not.toContain('damage_type');
    expect(
      visible({ ...base, rolls: ['attack', 'damage'], damageType: 'damage-type:fire' }),
    ).toContain('damage_type');
  });

  it('shows "applies" only for the critical range', () => {
    const base = { ...emptyEffect('modifier'), mode: 'add' };
    expect(visible({ ...base, target: 'ac' })).not.toContain('applies');
    expect(visible({ ...base, target: '' })).not.toContain('applies');
    expect(visible({ ...base, target: 'critical_range' })).toContain('applies');
  });

  it('names the optional fields that are never folded', () => {
    expect(neverFolded('reroll', 'resource')).toBe(true);
    expect(neverFolded('reroll', 'when')).toBe(false);
    expect(neverFolded('roll_minimum', 'damage_type')).toBe(true);
    expect(neverFolded('modifier', 'applies')).toBe(true);
    expect(neverFolded('sense', 'applies')).toBe(false);
  });

  it('writes the critical range with its scope, and no scope for another target', () => {
    const d = {
      ...emptyEffect('modifier'),
      target: 'critical_range',
      mode: 'set',
      value: '19',
      applies: 'both',
    };
    expect(draftToEffect(d, menu)).toEqual({
      type: 'modifier',
      target: 'critical_range',
      mode: 'set',
      value: '19',
      applies: 'both',
    });
    expect(draftToEffect({ ...d, target: 'ac' }, menu)).not.toHaveProperty('applies');
  });
});

describe('"Como os jogadores veem" for the effects that change a roll', () => {
  const cat = catalog();
  const nameOf = (k: string) => cat.nameOf(k);

  it('says a reroll with its rolls, face and cost', () => {
    expect(
      rollEffectItem(
        effect({
          type: 'reroll',
          rolls: ['attack', 'save'],
          atMost: 2,
          resource: 'sorte',
          cost: '1',
        }),
        nameOf,
      ),
    ).toEqual({
      title: 'Rolar de novo',
      text: 'jogada de ataque e teste de resistência, com 2 ou menos; gasta 1 uso.',
    });
    expect(
      rollEffectItem(
        effect({ type: 'reroll', rolls: ['attack', 'check', 'damage'], atMost: 1 }),
        nameOf,
      )?.text,
    ).toBe('jogada de ataque, teste de habilidade e dado de dano, com 1 ou menos.');
    expect(
      rollEffectItem(
        effect({ type: 'reroll', rolls: ['save'], atMost: 3, resource: 'sorte', cost: '2' }),
        nameOf,
      )?.text,
    ).toBe('teste de resistência, com 3 ou menos; gasta 2 usos.');
    expect(
      rollEffectItem(
        effect({ type: 'reroll', rolls: ['save'], atMost: 3, resource: 'sorte' }),
        nameOf,
      )?.text,
    ).toBe('teste de resistência, com 3 ou menos; gasta 1 uso.');
    expect(
      rollEffectItem(
        effect({ type: 'reroll', rolls: ['save'], atMost: 3, resource: 'sorte', cost: 'prof()' }),
        nameOf,
      )?.text,
    ).toBe('teste de resistência, com 3 ou menos; gasta usos.');
  });

  it('says a minimum with the damage type when it has one', () => {
    expect(
      rollEffectItem(
        effect({
          type: 'roll_minimum',
          rolls: ['damage'],
          value: '2',
          damageType: 'damage-type:fire',
        }),
        nameOf,
      ),
    ).toEqual({ title: 'Valor mínimo no dado', text: 'dado de dano de fogo, no mínimo 2.' });
    expect(
      rollEffectItem(
        effect({ type: 'roll_minimum', rolls: ['attack', 'damage'], value: '10' }),
        nameOf,
      )?.text,
    ).toBe('jogada de ataque e dado de dano, no mínimo 10.');
  });

  it('says the critical range with its scope, weapons being the default', () => {
    const crit = (applies: string) =>
      rollEffectItem(
        effect({ type: 'modifier', target: 'critical_range', mode: 'set', value: '19', applies }),
        nameOf,
      );
    expect(crit('both')).toEqual({
      title: 'Margem de crítico',
      text: '19 a 20, em armas e magias.',
    });
    expect(crit('spell')?.text).toBe('19 a 20, em magias.');
    expect(crit('weapon')?.text).toBe('19 a 20, em armas.');
    expect(crit('')?.text).toBe('19 a 20, em armas.');
  });

  it('says nothing of what is not complete or not a roll effect', () => {
    for (const e of [
      effect({ type: 'reroll', rolls: [], atMost: 2 }),
      effect({ type: 'reroll', rolls: ['save'] }),
      effect({ type: 'roll_minimum', rolls: ['damage'], value: 'x' }),
      effect({ type: 'modifier', target: 'critical_range', value: 'prof()' }),
      effect({ type: 'modifier', target: 'ac', value: '1' }),
      effect({ type: 'sense' }),
    ]) {
      expect(rollEffectItem(e, nameOf)).toBeNull();
    }
  });

  it('puts the line in the trait of a race (effectLine) and under the text of a feat and a class feature', () => {
    const reroll = effect({ type: 'reroll', rolls: ['save'], atMost: 2 });
    expect(effectLine(reroll, nameOf)).toBe('Rolar de novo: teste de resistência, com 2 ou menos.');
    const feat = readEntry(
      entry(TableContentKind.FEAT, 'Sorte', {
        body: {
          case: 'tableFeat',
          value: create(TableFeatSchema, {
            namePt: 'Sorte',
            descPt: ['Você insiste.'],
            effects: [reroll],
          }),
        },
      }),
      nameOf,
    );
    expect(feat.text).toEqual(['Você insiste.']);
    expect(feat.effects).toEqual([
      { title: 'Rolar de novo', text: 'teste de resistência, com 2 ou menos.' },
    ]);
    const classFeature = readEntry(
      entry(TableContentKind.CLASS_FEATURE, 'Olho', {
        body: {
          case: 'tableClassFeature',
          value: create(TableClassFeatureSchema, {
            namePt: 'Olho',
            effects: [
              effect({ type: 'modifier', target: 'critical_range', value: '19', applies: 'both' }),
            ],
          }),
        },
      }),
      nameOf,
    );
    expect(classFeature.effects?.[0].text).toBe('19 a 20, em armas e magias.');
  });

  it('adds the pool as "Usos: 3, descanso longo." after the roll lines, in a feat, an option and a class feature', () => {
    const effects = [
      effect({ type: 'resource', resource: 'sorte', max: '3', recharge: 'long_rest' }),
      effect({ type: 'reroll', rolls: ['save'], atMost: 2, resource: 'sorte' }),
    ];
    const lines = [
      { title: 'Rolar de novo', text: 'teste de resistência, com 2 ou menos; gasta 1 uso.' },
      { title: 'Usos', text: '3, descanso longo.' },
    ];
    const kinds = [
      [
        TableContentKind.FEAT,
        { case: 'tableFeat', value: create(TableFeatSchema, { namePt: 'A', effects }) },
      ],
      [
        TableContentKind.OPTION,
        { case: 'tableOption', value: create(TableOptionSchema, { namePt: 'A', effects }) },
      ],
      [
        TableContentKind.CLASS_FEATURE,
        {
          case: 'tableClassFeature',
          value: create(TableClassFeatureSchema, { namePt: 'A', effects }),
        },
      ],
    ] as const;
    for (const [kind, body] of kinds) {
      expect(readEntry(entry(kind, 'A', { body } as never), nameOf).effects).toEqual(lines);
    }
    const pool = (max: string, recharge: string) =>
      readEntry(
        entry(TableContentKind.FEAT, 'A', {
          body: {
            case: 'tableFeat',
            value: create(TableFeatSchema, {
              namePt: 'A',
              effects: [effect({ type: 'resource', resource: 'x', max, recharge })],
            }),
          },
        }),
        nameOf,
      ).effects;
    expect(pool('1', 'weird')).toEqual([{ title: 'Usos', text: '1.' }]);
    expect(pool('prof()', 'long_rest')).toBeUndefined();
  });
});
