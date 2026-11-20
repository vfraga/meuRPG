import { create } from '@bufbuild/protobuf';

import {
  ActionEconomy,
  type SpellDetails,
  SpellDamageChoice,
  SpellDetailsSchema,
  type SpellOption,
  SpellOptionSchema,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import { asksDamageType, castChoiceOf, readyChoices } from './ready-choices';
import { readySpellRows } from './ready';
import { TurnOptionsSchema } from '../../../gen/meurpg/rules/v1/rules_pb';

const spell = (key: string, over: Record<string, unknown> = {}): SpellOption =>
  create(SpellOptionSchema, {
    spell: { key, name: key, namePt: key, level: 1 },
    economy: ActionEconomy.ACTION,
    enabled: true,
    slots: [{ level: 1, free: 2, total: 2 }],
    ...over,
  } as never);

const details = (damageChoice: SpellDamageChoice, types: string[]): SpellDetails =>
  create(SpellDetailsSchema, {
    damageChoice,
    damage: types.map((t) => ({ damageTypeKey: t, damageTypePt: t })),
  } as never);

describe('Ready (#857): the spells that ask a choice the release does not send yet', () => {
  it('knows a damage type is asked when the spell lets the caster pick among two or more', () => {
    expect(asksDamageType(details(SpellDamageChoice.ALTERNATIVE, ['fire', 'radiant']))).toBe(true);
    expect(asksDamageType(details(SpellDamageChoice.UNSPECIFIED, ['fire', 'radiant']))).toBe(false);
    expect(asksDamageType(details(SpellDamageChoice.ALTERNATIVE, ['fire']))).toBe(false);
    expect(asksDamageType(null)).toBe(false);
  });

  it('names the choice by what the cast sheet asks: an ability, a summon, a damage type, in that order', () => {
    const none = new Set<string>();
    expect(castChoiceOf('spell:enhance-ability', null, none)).toBe('ability');
    expect(castChoiceOf('spell:conjure-animals', null, new Set(['spell:conjure-animals']))).toBe(
      'summon',
    );
    expect(
      castChoiceOf(
        'spell:flame-strike',
        details(SpellDamageChoice.ALTERNATIVE, ['fire', 'radiant']),
        none,
      ),
    ).toBe('damage');
    expect(
      castChoiceOf('spell:bless', details(SpellDamageChoice.UNSPECIFIED, []), none),
    ).toBeNull();
    expect(castChoiceOf('spell:bless', null, none)).toBeNull();
  });

  it('reads each held-able spell once and keeps the ones that ask, leaving out the spells of a feature and the ones that are off', async () => {
    const read = vi.fn(async (key: string) =>
      key === 'spell:flame-strike'
        ? details(SpellDamageChoice.ALTERNATIVE, ['fire', 'radiant'])
        : details(SpellDamageChoice.UNSPECIFIED, []),
    );
    const choices = await readyChoices(
      [
        spell('spell:enhance-ability'),
        spell('spell:conjure-animals'),
        spell('spell:flame-strike'),
        spell('spell:bless'),
        spell('spell:shield', { economy: ActionEconomy.REACTION }),
        spell('spell:hold-person', { enabled: false }),
      ],
      { details: read, summons: async () => new Set(['spell:conjure-animals']) },
    );
    expect([...choices]).toEqual([
      ['spell:enhance-ability', 'ability'],
      ['spell:conjure-animals', 'summon'],
      ['spell:flame-strike', 'damage'],
    ]);
    // The ability spell needs no read (the cast sheet knows it by its key), and a spell that cannot be held is not read.
    expect(read.mock.calls.map(([k]) => k).sort()).toEqual([
      'spell:bless',
      'spell:conjure-animals',
      'spell:flame-strike',
    ]);
  });

  it('keeps going when a read fails: the spell stays as the server will judge it', async () => {
    const choices = await readyChoices(
      [spell('spell:flame-strike'), spell('spell:conjure-animals')],
      {
        details: async () => {
          throw new Error('down');
        },
        summons: async () => {
          throw new Error('down');
        },
      },
    );
    expect(choices.size).toBe(0);
  });

  it('turns each kind off in the sheet with its own reason, and leaves a plain spell on', () => {
    const rows = readySpellRows(
      create(TurnOptionsSchema, {
        spells: [
          spell('spell:enhance-ability'),
          spell('spell:conjure-animals'),
          spell('spell:flame-strike'),
          spell('spell:bless'),
        ],
      } as never),
      new Map(),
      new Map([
        ['spell:enhance-ability', 'ability'],
        ['spell:conjure-animals', 'summon'],
        ['spell:flame-strike', 'damage'],
      ]),
    );
    expect(rows.map((r) => [r.id, r.blocked])).toEqual([
      [
        'spell:enhance-ability',
        'Soltar pela janela ainda não dá para esta magia: ela pede a habilidade ao conjurar.',
      ],
      [
        'spell:conjure-animals',
        'Soltar pela janela ainda não dá para esta magia: ela pede as criaturas ao conjurar.',
      ],
      [
        'spell:flame-strike',
        'Soltar pela janela ainda não dá para esta magia: ela pede o tipo de dano ao conjurar.',
      ],
      ['spell:bless', ''],
    ]);
  });
});
