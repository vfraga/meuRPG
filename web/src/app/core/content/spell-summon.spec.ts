import { create, fromBinary, toBinary } from '@bufbuild/protobuf';

import {
  TableContentKind,
  TableEntrySchema,
  TableEffectApplies,
  TableSpellEffectSchema,
  TableSpellSchema,
  TableSpellZoneSchema,
  TableSummonReplaces,
  TableSummonRole,
  TableSummonSchema,
  TableSummonTurnMode,
} from '../../../gen/meurpg/rules/v1/table_content_pb';
import { readEntry } from './content-read';
import { draftToSpell, spellToDraft, withMechanic, withTarget } from './spell-draft';
import { spellFieldPaths } from './spell-preview';
import { SUMMON_LOCK_PT, summonRefusals } from './spell-summon';
import { emptySpell } from './spell-types';

/** A stored summoning spell: a leveled spell on the caster alone, with a summon that carries a field this build does not know. */
function stored(summon: object = {}) {
  const bytes = toBinary(
    TableSummonSchema,
    create(TableSummonSchema, {
      creatureKeys: ['creature:lobo-do-vale@mesa', 'monster:wolf'],
      count: '1 + slot()',
      countPerSlotLevel: 2,
      maxLive: 6,
      turnMode: TableSummonTurnMode.OWNER_ROLL_GROUP,
      replaces: TableSummonReplaces.SAME_SUMMON,
      role: TableSummonRole.COMPANION,
      ...summon,
    }),
  );
  // field 99, varint 1: something a newer server adds
  const withUnknown = fromBinary(TableSummonSchema, new Uint8Array([...bytes, 0x98, 0x06, 0x01]));
  return create(TableSpellSchema, {
    namePt: 'Chamado do Vale',
    level: 3,
    schoolKey: 'school:conjuration',
    castingTime: { unit: 1, amount: 1 },
    range: { kind: 2 },
    duration: { kind: 2, amount: 1, unit: 3, upTo: true },
    concentration: true,
    target: { kind: 4 },
    summon: withUnknown,
  });
}

describe('"Invocar" in the spell draft (#578, board C3)', () => {
  it('opens a spell that summons on "Invocar", with the summon as typed', () => {
    const d = spellToDraft(stored());
    expect(d.mechanic).toBe('summon');
    expect(d.kept?.effect).toBeUndefined();
    expect(d.summon).toMatchObject({
      creatureKeys: ['creature:lobo-do-vale@mesa', 'monster:wolf'],
      count: '1 + slot()',
      perSlot: '2',
      maxLive: '6',
      turnMode: TableSummonTurnMode.OWNER_ROLL_GROUP,
      replaces: TableSummonReplaces.SAME_SUMMON,
    });
  });

  it('goes back as it was loaded, a field this build does not know included', () => {
    const s = stored();
    const sent = create(TableSpellSchema, draftToSpell(spellToDraft(s)));
    expect(sent.summon).toEqual(s.summon);
    expect(toBinary(TableSummonSchema, sent.summon!)).toEqual(
      toBinary(TableSummonSchema, s.summon!),
    );
    expect(sent.concentration).toBe(true);
  });

  it('sends what the block edits over it, and never a resource, a cost, a time, a duration or a concentration', () => {
    const d = spellToDraft(stored());
    const dirty = {
      ...d,
      summon: {
        ...d.summon!,
        count: '2',
        perSlot: '1',
        maxLive: '',
        creatureKeys: ['monster:wolf'],
        resourceKey: 'recurso',
        concentration: true,
        time: { unit: 3, amount: 10, triggerPt: '' },
        duration: { kind: 2, amount: 1, unit: 3, upTo: true },
      },
    };
    const sent = create(TableSpellSchema, draftToSpell(dirty)).summon!;
    expect(sent).toMatchObject({
      creatureKeys: ['monster:wolf'],
      count: '2',
      countPerSlotLevel: 1,
      maxLive: 0,
      resourceKey: '',
      resourceCost: 0,
      concentration: false,
    });
    expect(sent.castingTime).toBeUndefined();
    expect(sent.duration).toBeUndefined();
    expect(sent.role).toBe(TableSummonRole.COMPANION);
  });

  it('sends a number that is no whole number as -1, for the server to refuse on its field', () => {
    const d = spellToDraft(stored());
    const sent = draftToSpell({ ...d, summon: { ...d.summon!, perSlot: 'x', maxLive: '1,5' } });
    expect(sent.summon).toMatchObject({ countPerSlotLevel: -1, maxLive: -1 });
  });

  it('drops the summon when another mechanic is chosen, and gives it back when "Invocar" is chosen again', () => {
    const d = spellToDraft(stored());
    const attack = withMechanic(d, 'attack');
    expect(draftToSpell(attack).summon).toBeUndefined();
    expect(draftToSpell({ ...d, mechanic: 'text' }).summon).toBeUndefined();
    expect(draftToSpell(withMechanic(attack, 'summon')).summon?.creatureKeys).toEqual(
      d.summon!.creatureKeys,
    );
  });

  it('chooses the caster as the target (and the range with it) and leaves the level and the ritual alone', () => {
    const d = {
      ...emptySpell(),
      level: 0,
      ritual: true,
      target: 'area' as const,
      range: 'ranged' as const,
    };
    const next = withMechanic(d, 'summon');
    expect(next).toMatchObject({
      mechanic: 'summon',
      target: 'self',
      range: 'self',
      level: 0,
      ritual: true,
    });
    expect(next.summon).toMatchObject({ count: '1', perSlot: '0', maxLive: '0' });
    expect(draftToSpell(next).summon?.creatureKeys).toEqual([]);
  });

  it('sends no summon, and no damage, for a spell that is not on "Invocar"', () => {
    expect(
      draftToSpell({ ...emptySpell(), summon: spellToDraft(stored()).summon }).summon,
    ).toBeUndefined();
  });
});

describe('what the engine will refuse about a summoning spell, said before the save', () => {
  const on = (over: object) => ({ ...withMechanic(emptySpell(), 'summon'), ...over });

  it('has nothing to say for a leveled ritual-free spell on the caster, or for another mechanic', () => {
    expect(summonRefusals(on({}))).toEqual([]);
    expect(summonRefusals({ ...emptySpell(), level: 0, ritual: true })).toEqual([]);
  });

  it('puts each refusal on the field the master changes, in the order of the form', () => {
    const got = summonRefusals(on({ level: 0, target: 'creature', ritual: true }));
    expect(got.map((r) => r.path)).toEqual([
      'table_spell.level',
      'table_spell.ritual',
      'table_spell.target.kind',
    ]);
    expect(got[0].text).toContain('Um truque não invoca');
  });

  it('refuses a stored area zone beside the summon, at the summon', () => {
    const d = spellToDraft(stored());
    const withZone = { ...d, kept: { zone: create(TableSpellZoneSchema) } };
    expect(summonRefusals(withZone).map((r) => r.path)).toEqual(['table_spell.summon']);
  });

  it('refuses a stored effect on an instantaneous spell, and one that is not for all, with what to do', () => {
    const effect = create(TableSpellEffectSchema, { applies: TableEffectApplies.FAILED_SAVE });
    const d = { ...on({}), kept: { effect } };
    expect(d.duration).toBe('instant');
    const got = summonRefusals(d);
    expect(got.map((r) => r.path)).toEqual(['table_spell.duration.kind', 'table_spell.summon']);
    expect(got[1].text).toContain('Escolha outra mecânica.');
    const all = {
      ...d,
      duration: 'until_dispelled' as const,
      kept: { effect: create(TableSpellEffectSchema, { applies: TableEffectApplies.ALL }) },
    };
    expect(summonRefusals(all)).toEqual([]);
  });

  it('gives the target and the range back on leaving "Invocar" while they are still the caster\'s', () => {
    const d = { ...emptySpell(), target: 'creature' as const, range: 'touch' as const };
    const on = withMechanic(d, 'summon');
    expect(withMechanic(on, 'text')).toMatchObject({ target: 'creature', range: 'touch' });
    expect(withMechanic(on, 'text').beforeSummon).toBeUndefined();
    expect(withMechanic(withTarget(on, 'area'), 'text').target).toBe('area');
    expect(withMechanic(withMechanic(on, 'summon'), 'text')).toMatchObject({ range: 'touch' });
    expect(
      withMechanic({ ...emptySpell(), target: 'self', range: 'self' }, 'summon').beforeSummon,
    ).toBeUndefined();
  });

  it('says the rule the same as the board', () => {
    expect(SUMMON_LOCK_PT).toBe(
      '“Invocar” pede uma magia de 1º círculo ou mais, com alvo “Só quem conjura”, sem ataque, teste, dano, cura ou área, e que não seja ritual.',
    );
  });
});

describe('where a refusal of the summon lands, and what the preview and the read view say', () => {
  it('draws the summon fields only on "Invocar"', () => {
    const on = spellFieldPaths(withMechanic(emptySpell(), 'summon'));
    for (const f of [
      'creature_keys',
      'count',
      'count_per_slot_level',
      'max_live',
      'turn_mode',
      'replaces',
    ]) {
      expect(on.has(`table_spell.summon.${f}`)).toBe(true);
    }
    expect(on.has('table_spell.summon')).toBe(true);
    expect(spellFieldPaths(emptySpell()).has('table_spell.summon.count')).toBe(false);
  });

  it('says "Invoca: Lobo do Vale, Lobo." under the text', () => {
    const names: Record<string, string> = {
      'creature:lobo-do-vale@mesa': 'Lobo do Vale',
      'monster:wolf': 'Lobo',
    };
    const entry = create(TableEntrySchema, {
      kind: TableContentKind.SPELL,
      body: { case: 'tableSpell', value: { ...stored(), descPt: ['Lobos vêm.'] } },
    });
    const read = readEntry(entry, (k) => names[k] ?? k);
    expect(read.text).toEqual(['Lobos vêm.', 'Invoca: Lobo do Vale, Lobo.']);
  });
});
