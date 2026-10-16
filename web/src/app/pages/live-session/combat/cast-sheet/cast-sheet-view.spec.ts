import { create } from '@bufbuild/protobuf';

import {
  AttackOutcome,
  CombatantKind,
  DiceRollSchema,
  PendingDamageSchema,
  PreviewSpellAreaResponseSchema,
  SpellCastSchema,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  Ability,
  ActionEconomy,
  SpellDamageChoice,
  SpellRangeKind,
  type SpellDetails,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import type { SlotRow, TargetRule } from '../../../../core/combat/cast-flow';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { NO_PICKS } from '../../../../core/resources/metamagic';
import {
  areaResultLine,
  castMissing,
  castNotes,
  castRowsOf,
  castShow,
  castTargetsOf,
  castTitle,
  concentrationEnd,
  coverNoteOf,
  damageChoicesOf,
  damageTypeOf,
  dartsAfterChange,
  doneSubtitle,
  idsWhere,
  isPlainCast,
  keptTargets,
  luckyShow,
  nextRollOf,
  picksAfterTargets,
  reachFtOf,
  resumedCastOf,
  sculptRowsOf,
  sculptedIdsOf,
  shieldLostText,
  slotAfter,
  targetsMissingText,
  toggledSculpt,
} from './cast-sheet-view';

const plain = (t: string) => t.replace(/ /g, ' ');
const details = (over: Record<string, unknown> = {}) =>
  ({
    spell: { level: 1 },
    damage: [],
    healBySlotLevel: {},
    higherLevel: [],
    ...over,
  }) as unknown as SpellDetails;

const reach = (id: string, over: Record<string, unknown> = {}) => ({
  combatantId: id,
  label: id.toUpperCase(),
  distanceFt: 10,
  tooFar: false,
  ...over,
});

describe('the damage type', () => {
  it('lists the types of a spell that lets the caster pick, and none otherwise', () => {
    const types = [
      { damageTypeKey: 'damage-type:fire', damageTypePt: 'fogo' },
      { damageTypeKey: 'damage-type:radiant', damageTypePt: 'radiante' },
    ];
    expect(
      damageChoicesOf(details({ damageChoice: SpellDamageChoice.ALTERNATIVE, damage: types })),
    ).toEqual([
      { key: 'damage-type:fire', name: 'fogo' },
      { key: 'damage-type:radiant', name: 'radiante' },
    ]);
    expect(
      damageChoicesOf(details({ damageChoice: SpellDamageChoice.UNSPECIFIED, damage: types })),
    ).toEqual([]);
    expect(
      damageChoicesOf(
        details({ damageChoice: SpellDamageChoice.ALTERNATIVE, damage: types.slice(0, 1) }),
      ),
    ).toEqual([]);
    expect(damageChoicesOf(null)).toEqual([]);
  });

  it('is the picked one, else the first, else none', () => {
    const choices = [{ key: 'a' }, { key: 'b' }];
    expect(damageTypeOf(choices, 'b')).toBe('b');
    expect(damageTypeOf(choices, 'zzz')).toBe('a');
    expect(damageTypeOf([], 'a')).toBe('');
  });
});

describe('reachFtOf', () => {
  it('is the distance of a ranged spell and null for any other', () => {
    expect(reachFtOf(details({ range: { kind: SpellRangeKind.RANGED, distanceFt: 60 } }))).toBe(60);
    expect(reachFtOf(details({ range: { kind: SpellRangeKind.TOUCH, distanceFt: 5 } }))).toBeNull();
    expect(reachFtOf(details())).toBeNull();
    expect(reachFtOf(null)).toBeNull();
  });
});

describe('castRowsOf', () => {
  const targets = create(SpellTargetsSchema, { targets: [reach('s'), reach('g1')] } as never);

  it('leaves the caster out for a spell aimed at others', () => {
    expect(castRowsOf(targets, 'attack', 's', null).map((r) => r.id)).toEqual(['g1']);
  });

  it('keeps the caster for a heal and for a spell the app has no effect for', () => {
    expect(castRowsOf(targets, 'heal', 's', null).map((r) => r.id)).toEqual(['s', 'g1']);
    expect(castRowsOf(targets, 'plain', 's', null).map((r) => r.id)).toEqual(['s', 'g1']);
    expect(castRowsOf(undefined, 'plain', 's', null)).toEqual([]);
  });
});

describe('idsWhere', () => {
  it('collects the ids of the combatants that pass', () => {
    const list = [
      combatant({ id: 'a', label: 'A' }),
      combatant({ id: 'b', label: 'B', kind: CombatantKind.PLAYER }),
    ];
    expect([...idsWhere(list, (c) => c.kind === CombatantKind.PLAYER)]).toEqual(['b']);
  });
});

describe('concentrationEnd', () => {
  const spell = { casterId: 's', concentration: true, key: 'spell:entangle', name: 'Constrição' };
  const caster = (over: Record<string, unknown> = {}) =>
    combatant({
      id: 's',
      label: 'Sálvia',
      characterId: 'sc',
      concentrationSpell: 'spell:web',
      concentrationSpellNamePt: 'Teia',
      ...over,
    });
  const pet = (id: string, label: string, over: Record<string, unknown> = {}) =>
    combatant({
      id,
      label,
      kind: CombatantKind.CREATURE,
      ownerCharacterId: 'sc',
      summonGroupId: 'g',
      monsterKey: 'monster:wolf',
      monsterNamePt: label,
      ...over,
    });

  it('says nothing without a combat, a caster, concentration, or another spell held', () => {
    expect(concentrationEnd(null, spell)).toBeNull();
    expect(concentrationEnd(encounter({ combatants: [] }), spell)).toBeNull();
    expect(
      concentrationEnd(encounter({ combatants: [caster()] }), { ...spell, concentration: false }),
    ).toBeNull();
    expect(
      concentrationEnd(encounter({ combatants: [caster({ concentrationSpell: '' })] }), spell),
    ).toBeNull();
    expect(
      concentrationEnd(
        encounter({ combatants: [caster({ concentrationSpell: spell.key })] }),
        spell,
      ),
    ).toBeNull();
  });

  it('names the spell it ends, "a magia" when it has no name', () => {
    expect(concentrationEnd(encounter({ combatants: [caster()] }), spell)).toEqual({
      ends: 'Constrição encerra a concentração em Teia.',
      goes: '',
    });
    expect(
      concentrationEnd(encounter({ combatants: [caster({ concentrationSpellNamePt: '' })] }), spell)
        ?.ends,
    ).toBe('Constrição encerra a concentração em a magia.');
  });

  it('says which creatures go with it: one, in its gender, or a group', () => {
    expect(
      concentrationEnd(encounter({ combatants: [caster(), pet('w1', 'Lobo')] }), spell)?.goes,
    ).toBe('O Lobo some.');
    expect(
      concentrationEnd(encounter({ combatants: [caster(), pet('w1', 'Loba')] }), spell)?.goes,
    ).toBe('A Loba some.');
    expect(
      concentrationEnd(
        encounter({ combatants: [caster(), pet('w1', 'Lobo 1'), pet('w2', 'Lobo 2')] }),
        spell,
      )?.goes,
    ).toMatch(/^Os 2 .* somem\.$/);
    expect(
      concentrationEnd(
        encounter({ combatants: [caster(), pet('w1', 'Loba 1'), pet('w2', 'Loba 2')] }),
        spell,
      )?.goes,
    ).toMatch(/^As 2 /);
  });

  it('leaves out the creatures of another owner, the ones with no summon group, and the defeated', () => {
    const others = [
      pet('w1', 'Lobo', { ownerCharacterId: 'other' }),
      pet('w2', 'Lobo', { summonGroupId: '' }),
      pet('w3', 'Lobo', { defeated: true }),
    ];
    expect(concentrationEnd(encounter({ combatants: [caster(), ...others] }), spell)?.goes).toBe(
      '',
    );
  });
});

describe('targetsMissingText', () => {
  const rule = (kind: TargetRule['kind'], min = 1, max = 3): TargetRule => ({ kind, min, max });
  const have = (chosen: number, dealt: [string, number][] = [], darts = 3) => ({
    chosen,
    dealt: new Map(dealt),
    darts,
  });

  it('asks for the darts until all are placed', () => {
    expect(targetsMissingText(rule('darts'), 'darts', have(0, [['a', 3]]), true)).toBe('');
    expect(targetsMissingText(rule('darts'), 'darts', have(0, [['a', 1]]), true)).not.toBe('');
  });

  it('asks for one target, or at least one of several', () => {
    expect(targetsMissingText(rule('single'), 'save', have(0), true)).toBe('Escolha o alvo.');
    expect(targetsMissingText(rule('multi'), 'save', have(0), true)).toBe(
      'Escolha pelo menos um alvo.',
    );
    expect(targetsMissingText(rule('multi', 0), 'save', have(0), true)).toBe('');
    expect(targetsMissingText(rule('none', 0, 0), 'save', have(0), true)).toBe('');
  });

  it('asks for one target at a time for a spell attack with physical dice only', () => {
    expect(targetsMissingText(rule('multi'), 'attack', have(2), false)).toBe(
      'Com dados físicos, escolha um alvo por vez.',
    );
    expect(targetsMissingText(rule('multi'), 'attack', have(2), true)).toBe('');
    expect(targetsMissingText(rule('multi'), 'attack', have(1), false)).toBe('');
    expect(targetsMissingText(rule('multi'), 'save', have(2), false)).toBe('');
  });
});

describe('castMissing', () => {
  const base = {
    detailsRead: true,
    detailsFailed: false,
    level: 1,
    slotChosen: true,
    spellKey: 'spell:x',
    ability: '',
    placedArea: false,
    targets: () => '',
    metaOptions: [],
    chosenMeta: [],
    picks: NO_PICKS,
    sorceryPoints: null,
  };

  it('waits for the spell to be read, and says when it could not be', () => {
    expect(castMissing({ ...base, detailsRead: false })).toBe('Lendo a magia…');
    expect(castMissing({ ...base, detailsRead: false, detailsFailed: true })).toBe(
      'Não deu para ler a magia. Feche e tente de novo.',
    );
    expect(castMissing({ ...base, detailsFailed: true })).toBe(
      'Não deu para ler a magia. Feche e tente de novo.',
    );
  });

  it('asks for the slot of a leveled spell only', () => {
    expect(castMissing({ ...base, slotChosen: false })).toBe('Escolha o espaço de magia.');
    expect(castMissing({ ...base, slotChosen: false, level: 0 })).toBe('');
  });

  it('asks for the ability of the spell that names one', () => {
    expect(castMissing({ ...base, spellKey: 'spell:enhance-ability' })).not.toBe('');
    expect(castMissing({ ...base, spellKey: 'spell:enhance-ability', ability: 'str' })).toBe('');
  });

  it('asks the targets, unless the area is placed on the map', () => {
    expect(castMissing({ ...base, targets: () => 'Escolha o alvo.' })).toBe('Escolha o alvo.');
    expect(castMissing({ ...base, targets: () => 'Escolha o alvo.', placedArea: true })).toBe('');
  });
});

describe('Sculpt Spells', () => {
  const rows = [
    { id: 'a', label: 'A', blocked: '', sub: '', cover: '', coverMark: null },
    { id: 'b', label: 'B', blocked: 'longe', sub: '', cover: '', coverMark: null },
    { id: 'c', label: 'C', blocked: '', sub: '', cover: '', coverMark: null },
  ];

  it('offers nothing to a caster without it', () => {
    expect(sculptRowsOf(false, null, false, rows, ['a'])).toEqual([]);
  });

  it('lists the ticked creatures that can be targeted, in a list', () => {
    expect(sculptRowsOf(true, null, false, rows, ['a', 'b', 'c']).map((r) => r.id)).toEqual([
      'a',
      'c',
    ]);
  });

  it('lists the others in a placed area, allies first, and nothing before the preview', () => {
    const area = create(PreviewSpellAreaResponseSchema, {
      targets: [
        { combatantId: 'g', label: 'Goblin', ally: false },
        { combatantId: 'p', label: 'Pia', ally: true },
        { combatantId: 's', label: 'Sálvia', self: true },
      ],
    });
    expect(sculptRowsOf(true, area, true, [], []).map((r) => r.id)).toEqual(['p', 'g']);
    expect(sculptRowsOf(true, null, true, [], [])).toEqual([]);
  });

  it('sends the marked creatures that are still listed, up to the limit', () => {
    const listed = [
      { id: 'a', label: '', ally: false },
      { id: 'b', label: '', ally: false },
    ];
    expect(sculptedIdsOf(['a', 'x', 'b'], listed, 5)).toEqual(['a', 'b']);
    expect(sculptedIdsOf(['a', 'b'], listed, 1)).toEqual(['a']);
  });

  it('toggles one in while there is room and out at any time', () => {
    expect(toggledSculpt(['a'], 'b', 2)).toEqual(['a', 'b']);
    expect(toggledSculpt(['a', 'b'], 'c', 2)).toEqual(['a', 'b']);
    expect(toggledSculpt(['a', 'b'], 'a', 2)).toEqual(['b']);
  });
});

describe('the choices that follow the slot', () => {
  it('keeps the targets that still fit, up to the maximum, and says nothing when all do', () => {
    expect(keptTargets(['a', 'b', 'c'], new Set(['a', 'b', 'c']), 3)).toBeNull();
    expect(keptTargets(['a', 'b', 'c'], new Set(['a', 'b', 'c']), 2)).toEqual(['a', 'b']);
    expect(keptTargets(['a', 'x'], new Set(['a']), 3)).toEqual(['a']);
    expect(keptTargets(['a'], new Set(['a']), -1)).toEqual([]);
  });

  it('puts the darts on the only target when none is placed, takes back the ones past the total, else leaves them', () => {
    const dealt = new Map<string, number>();
    expect([...(dartsAfterChange(dealt, ['g'], 3, false) ?? [])]).toEqual([['g', 3]]);
    expect(dartsAfterChange(dealt, ['g', 'h'], 3, false)).toBeNull();
    expect(dartsAfterChange(dealt, ['g'], 3, true)).toBeNull();
    expect(dartsAfterChange(dealt, ['g'], 0, false)).toBeNull();
    expect(dartsAfterChange(new Map([['g', 2]]), ['g'], 3, false)).toBeNull();
    expect(dartsAfterChange(new Map([['g', 4]]), ['g', 'h'], 3, false)?.size).toBe(0);
  });

  it('drops a Metamagic pick that is no longer a target', () => {
    expect(
      picksAfterTargets({ twinned: 'a', careful: ['b'], heightened: 'c' }, ['a', 'c']),
    ).toEqual({
      twinned: '',
      careful: ['b'],
      heightened: 'c',
    });
    expect(picksAfterTargets({ twinned: 'x', careful: [], heightened: 'z' }, ['a'])).toEqual({
      twinned: 'x',
      careful: [],
      heightened: '',
    });
  });
});

describe('coverNoteOf', () => {
  it('explains the cover only when the preview counts it and has creatures', () => {
    const area = (coverCounts: boolean, n: number) =>
      create(PreviewSpellAreaResponseSchema, {
        coverCounts,
        targets: Array.from({ length: n }, (_, i) => ({ combatantId: `g${i}`, label: 'G' })),
      });
    expect(coverNoteOf(area(true, 1), 'Bola de Fogo')).toContain('como o da Bola de Fogo');
    expect(coverNoteOf(area(true, 1), 'Nuvem')).toContain('como o do Nuvem');
    expect(coverNoteOf(area(false, 1), 'Bola de Fogo')).toBe('');
    expect(coverNoteOf(area(true, 0), 'Bola de Fogo')).toBe('');
    expect(coverNoteOf(null, 'Bola de Fogo')).toBe('');
  });
});

describe('the subtitle and the title of a cast that is done', () => {
  const cast = (targets: Record<string, unknown>[], slot = { level: 3, pact: false }) =>
    create(SpellCastSchema, { targets, slot } as never);
  const save = details({ save: { ability: Ability.DEXTERITY } });

  it('says the circle, and for a pool spell the creatures it reached', () => {
    const c = cast([{ combatantId: 'a' }, { combatantId: 'b' }]);
    expect(
      plain(doneSubtitle({ cast: c, level: 3, kind: 'attack', placedArea: false, details: null })),
    ).toBe('3º nível');
    expect(
      plain(doneSubtitle({ cast: c, level: 3, kind: 'pool', placedArea: false, details: null })),
    ).toBe('3º nível · 2 criaturas na área');
    expect(
      plain(
        doneSubtitle({
          cast: cast([{ combatantId: 'a' }]),
          level: 3,
          kind: 'pool',
          placedArea: false,
          details: null,
        }),
      ),
    ).toBe('3º nível · 1 criatura na área');
    expect(
      plain(
        doneSubtitle({ cast: cast([]), level: 3, kind: 'pool', placedArea: false, details: null }),
      ),
    ).toBe('3º nível');
  });

  it('says "Truque" for a cantrip with no slot, and nothing for a leveled spell with none', () => {
    const none = create(SpellCastSchema, {});
    expect(
      doneSubtitle({ cast: none, level: 0, kind: 'save', placedArea: false, details: null }),
    ).toBe('Truque');
    expect(
      doneSubtitle({ cast: none, level: 2, kind: 'save', placedArea: false, details: null }),
    ).toBe('');
    expect(
      doneSubtitle({ cast: null, level: 0, kind: 'save', placedArea: false, details: null }),
    ).toBe('Truque');
  });

  it('tells what a placed area hit: the creatures, the save and its DC', () => {
    const c = cast([{ combatantId: 'a', save: { dc: 15 } }, { combatantId: 'b' }]);
    expect(plain(areaResultLine('3º nível', c, save))).toBe(
      '3º nível · 2 criaturas · teste de resistência de Destreza (CD 15)',
    );
    expect(plain(areaResultLine('3º nível', cast([{ combatantId: 'a' }]), save))).toBe(
      '3º nível · 1 criatura · teste de resistência de Destreza',
    );
    expect(plain(areaResultLine('3º nível', c, details()))).toBe('3º nível · 2 criaturas');
    expect(areaResultLine('', null, null)).toBe('0 criaturas');
    expect(
      plain(doneSubtitle({ cast: c, level: 3, kind: 'save', placedArea: true, details: save })),
    ).toContain('CD 15');
  });

  it('titles the sheet by its stage', () => {
    const base = {
      name: 'Bola de Fogo',
      resume: false,
      done: false,
      hpDone: false,
      placedArea: false,
      typing: false,
    };
    expect(castTitle({ ...base })).toBe('Conjurar Bola de Fogo');
    expect(castTitle({ ...base, typing: true })).toBe('Digite o resultado');
    expect(castTitle({ ...base, resume: true })).toBe('Role o dano de Bola de Fogo');
    expect(castTitle({ ...base, done: true })).toBe('Você conjurou Bola de Fogo');
    expect(castTitle({ ...base, done: true, placedArea: true })).toBe('Bola de Fogo conjurada');
    expect(castTitle({ ...base, name: 'Sono', done: true, hpDone: true })).toBe('Sono conjurado');
    expect(castTitle({ ...base, done: false, placedArea: true })).toBe('Conjurar Bola de Fogo');
  });
});

describe('the result of the cast', () => {
  const rows: SlotRow[] = [
    { level: 1, pact: false, free: 2, total: 4, used: 2, enabled: true, title: '', count: '' },
    { level: 1, pact: true, free: 1, total: null, used: 0, enabled: true, title: '', count: '' },
  ];

  it('counts the slots left after the cast, with the total or without', () => {
    const c = create(SpellCastSchema, { slot: { level: 1, pact: false } });
    expect(slotAfter(c, rows)).toMatchObject({ level: 1, total: 4, used: 3 });
    expect(plain(slotAfter(c, rows)?.text ?? '')).toContain('1º nível');
    expect(
      slotAfter(create(SpellCastSchema, { slot: { level: 1, pact: true } }), rows),
    ).toMatchObject({ total: null, used: 0 });
    expect(
      slotAfter(create(SpellCastSchema, { slot: { level: 9, pact: false } }), rows),
    ).toBeNull();
    expect(slotAfter(null, rows)).toBeNull();
    expect(slotAfter(create(SpellCastSchema, {}), rows)).toBeNull();
  });

  it('says Escudo is gone with the last slot', () => {
    const c = create(SpellCastSchema, { slot: { level: 1, pact: false } });
    expect(plain(shieldLostText(c, 1, 'Escudo Arcano'))).toBe(
      'Escudo Arcano indisponível: sem espaço de 1º nível.',
    );
    expect(shieldLostText(c, 2, 'Escudo Arcano')).toBe('');
    expect(shieldLostText(c, null, 'Escudo Arcano')).toBe('');
    expect(shieldLostText(null, 1, 'Escudo Arcano')).toBe('');
  });

  it('lists the notes of a cast: concentration, the ended one, the metamagic and the action', () => {
    const spell = {
      name: 'Feitiço',
      economy: ActionEconomy.ACTION,
      metaOptions: [{ key: 'm:quick', namePt: 'Acelerada', cost: 2 }] as never,
      sorceryPoints: { left: 3, total: 5 },
    };
    expect(castNotes(null, spell)).toEqual([]);
    const full = castNotes(
      create(SpellCastSchema, {
        concentrating: true,
        concentrationEndedSpellKey: 'spell:web',
        metamagicKeys: ['m:quick', 'm:gone'],
        sorceryPointsSpent: 2,
      }),
      spell,
    );
    expect(full.length).toBe(4);
    expect(full[0]).toBe('Você está concentrado em Feitiço.');
    expect(full[1]).toBe('A sua concentração anterior acabou.');
    expect(full[2]).toContain('Acelerada');
    expect(full[2]).toContain('Metamagia');
    expect(castNotes(create(SpellCastSchema, {}), spell).length).toBe(1);
  });

  it('knows a plain cast: nothing to roll, hit or save', () => {
    expect(isPlainCast(create(SpellCastSchema, { targets: [{ combatantId: 'a' }] }), 'plain')).toBe(
      true,
    );
    expect(isPlainCast(create(SpellCastSchema, { targets: [{ combatantId: 'a' }] }), 'save')).toBe(
      false,
    );
    expect(
      isPlainCast(
        create(SpellCastSchema, { targets: [{ combatantId: 'a', pendingDamageId: 'p' }] }),
        'plain',
      ),
    ).toBe(false);
    expect(
      isPlainCast(
        create(SpellCastSchema, { targets: [{ combatantId: 'a', outcome: AttackOutcome.HIT }] }),
        'plain',
      ),
    ).toBe(false);
    expect(isPlainCast(null, 'plain')).toBe(false);
  });

  it('makes the rows of a resumed damage from the damages themselves', () => {
    const missile = create(PendingDamageSchema, {
      id: 'p1',
      targetId: 'g1',
      attackKey: 'spell:magic-missile',
      diceCount: 3,
    });
    const other = create(PendingDamageSchema, {
      id: 'p2',
      targetId: 'g2',
      attackKey: 'spell:x',
      diceCount: 1,
    });
    expect(
      resumedCastOf([missile, other].values()).targets.map((t) => [
        t.combatantId,
        t.darts,
        t.pendingDamageId,
      ]),
    ).toEqual([
      ['g1', 3, 'p1'],
      ['g2', 0, 'p2'],
    ]);
  });
});

describe('nextRollOf', () => {
  const labels = new Map([['g1', { label: 'Goblin 1', state: 0 as never }]]);
  const owed = (over: Record<string, unknown> = {}) =>
    create(PendingDamageSchema, {
      id: 'p1',
      targetId: 'g1',
      diceCount: 1,
      diceSides: 8,
      bonus: 2,
      ...over,
    } as never);

  it('has none when nothing is owed', () => {
    expect(nextRollOf([], labels)).toBeNull();
  });

  it('names the dice, the range and the targets of the first group', () => {
    const roll = nextRollOf(
      [[owed(), owed({ id: 'p2', targetId: 'ghost' })], [owed({ id: 'p3' })]],
      labels,
    );
    expect(roll).toMatchObject({
      name: '1d8',
      range: { min: 1, max: 8 },
      label: 'Role 1d8 para o dano',
      what: 'Dano em Goblin 1, alvo',
      count: 2,
      crit: '',
      modifier: 2,
    });
  });

  it('says two dice, a heal, and a critical in the words of the rule or the default ones', () => {
    expect(nextRollOf([[owed({ diceCount: 2, healing: true })]], labels)).toMatchObject({
      label: 'Role 2d8 para o dano: some os dois',
      what: 'Cura em Goblin 1',
    });
    expect(nextRollOf([[owed({ critical: true })]], labels)?.crit).not.toBe('');
  });
});

describe('the dice animated', () => {
  const d20 = (face: number, over: Record<string, unknown> = {}) =>
    create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [face],
      modifier: 5,
      total: face + 5,
      ...over,
    });
  const attack = (id: string, face: number, outcome: AttackOutcome) => ({
    combatantId: id,
    outcome,
    attackRoll: d20(face),
  });

  it('shows the pool of a spell that rolls one, before any attack', () => {
    const cast = create(SpellCastSchema, {
      poolRoll: create(DiceRollSchema, { diceCount: 2, diceSides: 8, faces: [3, 4], total: 7 }),
      targets: [attack('g', 9, AttackOutcome.HIT)],
    });
    expect(castShow('Sono', cast)?.label).toBe('Sono');
  });

  it('shows one attack with its word, told or not, and several as one roll of their dice', () => {
    const told = castShow(
      'Raio',
      create(SpellCastSchema, { targets: [attack('g', 20, AttackOutcome.CRITICAL_HIT)] }),
    );
    expect(told?.outcome).toBeDefined();
    expect(told?.critical).toBe(true);
    const missed = castShow(
      'Raio',
      create(SpellCastSchema, { targets: [attack('g', 1, AttackOutcome.MISS)] }),
    );
    expect(missed?.fumble).toBe(true);
    const untold = castShow(
      'Raio',
      create(SpellCastSchema, { targets: [attack('g', 9, AttackOutcome.UNSPECIFIED)] }),
    );
    expect(untold?.outcome).toBeUndefined();
    const several = castShow(
      'Raio',
      create(SpellCastSchema, {
        targets: [attack('g', 9, AttackOutcome.HIT), attack('h', 3, AttackOutcome.MISS)],
      }),
    );
    expect(several?.dice.length).toBe(2);
    expect(several?.label).toBe('Ataque com Raio');
  });

  it('shows nothing when no roll can be drawn', () => {
    expect(
      castShow('Raio', create(SpellCastSchema, { targets: [{ combatantId: 'g' }] })),
    ).toBeNull();
    const bad = {
      combatantId: 'g',
      attackRoll: create(DiceRollSchema, { diceCount: 0, diceSides: 0 }),
    };
    expect(
      castShow('Raio', create(SpellCastSchema, { targets: [bad, { ...bad, combatantId: 'h' }] })),
    ).toBeNull();
  });

  it('shows the new face Lucky rolled, and nothing for a typed one', () => {
    const rolled = d20(14, { rerolled: [{ index: 0, from: 1, to: 14, sourceKey: 'trait:lucky' }] });
    expect(luckyShow(rolled, { inApp: true })?.line).toBe('1 → 14');
    expect(luckyShow(rolled, { inApp: true })).not.toBeNull();
    expect(luckyShow(rolled, { face: 14 })).toBeNull();
    expect(luckyShow(rolled, null)).toBeNull();
    expect(luckyShow(d20(14), { inApp: true })).toBeNull();
    expect(luckyShow(undefined, { inApp: true })).toBeNull();
  });
});

describe('castTargetsOf', () => {
  const rule = (kind: TargetRule['kind']): TargetRule => ({ kind, min: 1, max: 3 });
  const darts = () => [{ combatantId: 'g', darts: 3 }];

  it('names nobody for a placed area or a spell with no target', () => {
    expect(castTargetsOf(true, rule('multi'), darts, ['a'])).toEqual([]);
    expect(castTargetsOf(false, rule('none'), darts, ['a'])).toEqual([]);
  });

  it('names the darts for a spell with darts, else the targets ticked', () => {
    expect(castTargetsOf(false, rule('darts'), darts, ['a'])).toEqual([
      { combatantId: 'g', darts: 3 },
    ]);
    expect(castTargetsOf(false, rule('multi'), darts, ['a', 'b'])).toEqual([
      { combatantId: 'a', darts: 0 },
      { combatantId: 'b', darts: 0 },
    ]);
  });
});
