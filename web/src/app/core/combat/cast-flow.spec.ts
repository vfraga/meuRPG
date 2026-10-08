import { create } from '@bufbuild/protobuf';

import {
  AttackOutcome,
  CombatantState,
  type PendingDamage,
  PendingDamageStatus,
  SaveOutcome,
  SpellCastSchema,
  SpellEffectKind,
  SpellEffectOutcome,
  SpellEffectReason,
  SpellTargetsSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { SlotChoiceSchema, SpellHitPointEffectKind } from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  castRows,
  castTargetRows,
  dartLines,
  dartsStatus,
  dartTargets,
  dealOne,
  defaultSlot,
  effectSentence,
  freeText,
  lastSlotWarning,
  rollGroups,
  slotRows,
  spellKind,
  targetRule,
  toggled,
} from './cast-flow';

const choice = (level: number, free: number, pact = false) =>
  create(SlotChoiceSchema, { level, free, pact });
const usage = [
  { level: 1, total: 4, used: 3 },
  { level: 2, total: 2, used: 2 },
];

describe('the slot step (E6-09)', () => {
  it("lists every circle from the spell's own up, the full one disabled", () => {
    const rows = slotRows(1, [choice(1, 1)], usage);
    expect(rows.map((r) => [r.title, r.count, r.enabled])).toEqual([
      ['1º\u00a0nível', '1 livre de 4', true],
      ['2º\u00a0nível', '0 livres de 2', false],
    ]);
    expect(defaultSlot(rows)?.level).toBe(1);
  });

  it('a spell of 2nd level does not list the 1st circle', () => {
    expect(slotRows(2, [], usage).map((r) => r.level)).toEqual([2]);
  });

  it('says "livre" and "livres", with or without the total', () => {
    expect(freeText(1, 4)).toBe('1 livre de 4');
    expect(freeText(2, null)).toBe('2 livres');
  });

  it('warns about the last slot, and about Escudo only when it is the last one it has', () => {
    const [first] = slotRows(1, [choice(1, 1)], usage);
    expect(lastSlotWarning(first, 1)).toBe(
      'É o seu último espaço de 1º\u00a0nível: depois dele, o Escudo Arcano fica sem espaço.',
    );
    expect(lastSlotWarning(first, 3)).toBe('É o seu último espaço de 1º\u00a0nível.');
    expect(
      lastSlotWarning(slotRows(1, [choice(1, 2)], [{ level: 1, total: 4, used: 2 }])[0], 2),
    ).toBe('');
  });
});

describe('the target step', () => {
  const st = (over: object) => create(SpellTargetsSchema, { spellKey: 'spell:x', ...over });

  it('adds the number of targets per slot level the server sends, not always one', () => {
    const table = st({ maxTargets: 3, extraTargetPerLevel: true, targetsPerLevel: 2 });
    expect(targetRule(table, 'me', 2, 2).max).toBe(3);
    expect(targetRule(table, 'me', 2, 3).max).toBe(5);
    expect(targetRule(table, 'me', 2, 5).max).toBe(9);
    expect(targetRule(st({ maxTargets: 3, targetsPerLevel: 0 }), 'me', 2, 4).max).toBe(3);
  });

  it('knows who takes one target, several, an area, only the caster and the darts', () => {
    const self = [{ combatantId: 'me' }];
    expect(targetRule(st({ maxTargets: 1 }), 'me', 1, 1)).toEqual({
      kind: 'single',
      max: 1,
      min: 1,
    });
    expect(targetRule(st({ maxTargets: 1, extraTargetPerLevel: true }), 'me', 1, 3)).toEqual({
      kind: 'multi',
      max: 3,
      min: 1,
    });
    expect(targetRule(st({ maxTargets: 0, targets: [{ combatantId: 'g' }] }), 'me', 1, 1)).toEqual({
      kind: 'multi',
      max: 10,
      min: 0,
    });
    expect(targetRule(st({ maxTargets: 0, targets: self }), 'me', 2, 2).kind).toBe('none');
    expect(
      targetRule(
        st({
          maxTargets: 3,
          darts: [
            { slotLevel: 1, darts: 3 },
            { slotLevel: 2, darts: 4 },
          ],
        }),
        'me',
        1,
        2,
      ),
    ).toEqual({ kind: 'darts', max: 4, min: 1 });
    expect(targetRule(undefined, 'me', 1, 1).kind).toBe('none');
  });

  it('a single choice replaces, several are limited', () => {
    expect(toggled({ kind: 'single', max: 1, min: 1 }, ['a'], 'b')).toEqual(['b']);
    expect(toggled({ kind: 'multi', max: 2, min: 1 }, ['a', 'b'], 'c')).toEqual(['a', 'b']);
    expect(toggled({ kind: 'multi', max: 2, min: 1 }, ['a', 'b'], 'a')).toEqual(['b']);
  });

  it('writes the distance and why a target cannot be chosen', () => {
    const rows = castTargetRows(
      [
        { combatantId: 'me', label: 'Pensantus', state: CombatantState.UNSPECIFIED, tooFar: false },
        {
          combatantId: 'g',
          label: 'Goblin 1',
          state: CombatantState.HURT,
          distanceFt: 25,
          tooFar: true,
        },
      ] as never,
      'me',
      120,
    );
    expect(rows[0].label).toBe('Pensantus (você)');
    expect(rows[1].sub).toBe('Ferido\u00a0· a\u00a07,5\u00a0m');
    expect(rows[1].blocked).toBe('Longe demais: alcance de\u00a036\u00a0m');
  });
});

describe("Magic Missile's darts", () => {
  it('deals the darts, one at a time, never past the total or below 0', () => {
    let d = dealOne(new Map(), 'a', 1, 3);
    d = dealOne(d, 'a', 1, 3);
    d = dealOne(d, 'b', 1, 3);
    expect(dartTargets(d)).toEqual([
      { combatantId: 'a', darts: 2 },
      { combatantId: 'b', darts: 1 },
    ]);
    expect([...dealOne(d, 'c', 1, 3)]).toEqual([...d]); // all three placed
    expect(dealOne(d, 'b', -1, 3).get('b')).toBe(0);
    expect(dartTargets(dealOne(d, 'b', -1, 3))).toEqual([{ combatantId: 'a', darts: 2 }]);
    expect(dealOne(new Map(), 'a', -1, 3).size).toBe(0);
  });

  it('counts them out loud', () => {
    expect(dartsStatus(3, new Map([['a', 2]]))).toBe('Distribua todos os dardos: 2 de 3.');
    expect(dartsStatus(3, new Map([['a', 3]]))).toBe('3 de 3 dardos distribuídos.');
  });

  it('writes each dart: the face plus the bonus of one dart', () => {
    expect(
      dartLines(
        {
          diceCount: 2,
          diceSides: 4,
          faces: [3, 2],
          modifier: 2,
          total: 7,
          physical: false,
        } as never,
        2,
      ),
    ).toEqual(['Dardo 1: 1d4 (3) + 1 = 4', 'Dardo 2: 1d4 (2) + 1 = 3']);
    expect(
      dartLines(
        { diceCount: 2, diceSides: 4, faces: [], modifier: 2, total: 7, physical: true } as never,
        2,
      )[0],
    ).toMatch(/dado físico/);
  });
});

describe('the result of a cast', () => {
  it('reads what the spell is by what it asks for', () => {
    expect(spellKind('spell:magic-missile', null)).toBe('darts');
    expect(spellKind('spell:x', null)).toBe('plain');
    // The spells that read hit points say so in their own details: Sono rolls a pool, Palavra de Poder only names who it touches.
    const fx = (kind: SpellHitPointEffectKind) => ({ hitPointEffect: { kind } }) as never;
    expect(spellKind('spell:sleep', fx(SpellHitPointEffectKind.POOL))).toBe('pool');
    expect(spellKind('spell:power-word-stun', fx(SpellHitPointEffectKind.THRESHOLD))).toBe('hp');
    expect(spellKind('spell:spare-the-dying', fx(SpellHitPointEffectKind.ZERO_HP))).toBe('hp');
    expect(spellKind('spell:heal', fx(SpellHitPointEffectKind.FLAT_HEAL))).toBe('hp');
    expect(spellKind('spell:x', { attackType: 3 } as never)).toBe('attack');
    expect(
      spellKind('spell:x', { attackType: 1, save: { ability: 2 }, healBySlotLevel: {} } as never),
    ).toBe('save');
    expect(
      spellKind('spell:x', { attackType: 1, healBySlotLevel: { 1: '1d8 + MOD' } } as never),
    ).toBe('heal');
  });

  it('writes the save, the damage and who is still to roll', () => {
    const cast = create(SpellCastSchema, {
      targets: [
        { combatantId: 'g1', save: { outcome: SaveOutcome.FAILED, dc: 14 }, pendingDamageId: 'p1' },
        { combatantId: 'g2', save: { outcome: SaveOutcome.SAVED, dc: 14 }, pendingDamageId: 'p2' },
        { combatantId: 'g3', outcome: AttackOutcome.MISS },
      ],
    });
    const rolled = (id: string, amount: number, half: boolean) =>
      ({
        id,
        status: PendingDamageStatus.APPLIED,
        amount,
        half,
        damageTypePt: 'fogo',
        roll: { diceCount: 3, diceSides: 6, faces: [4, 4, 2], modifier: 0, total: 10 },
      }) as unknown as PendingDamage;
    const labels = new Map([
      ['g1', { label: 'Goblin 1', state: CombatantState.DEFEATED }],
      ['g2', { label: 'Goblin 2', state: CombatantState.BADLY_HURT }],
      ['g3', { label: 'Goblin 3', state: CombatantState.UNHURT }],
    ]);
    const rows = castRows(
      cast,
      new Map([
        ['p1', rolled('p1', 10, false)],
        ['p2', rolled('p2', 5, true)],
      ]),
      labels,
    );
    expect(rows[0]).toMatchObject({ word: 'Falhou', summary: '10 de fogo', state: 'Derrotado' });
    expect(rows[1]).toMatchObject({
      word: 'Resistiu: metade',
      summary: '5 de fogo',
      state: 'Muito ferido',
    });
    expect(rows[2]).toMatchObject({ word: 'Errou', tone: 'bad', summary: '' });
    const owed = castRows(
      cast,
      new Map([['p1', { id: 'p1', status: PendingDamageStatus.AWAITING_ROLL } as PendingDamage]]),
      labels,
    );
    expect(owed[0].owed).toBe(true);
  });

  it("rolls one group for an area spell and one for each of Magic Missile's targets", () => {
    const p = (id: string, castId: string) =>
      ({ id, castId, status: PendingDamageStatus.AWAITING_ROLL }) as PendingDamage;
    expect(
      rollGroups([p('a', 'c1'), p('b', 'c1'), p('c', ''), p('d', '')]).map((g) =>
        g.map((x) => x.id),
      ),
    ).toEqual([['a', 'b'], ['c'], ['d']]);
    expect(
      rollGroups([{ id: 'x', castId: '', status: PendingDamageStatus.APPLIED } as PendingDamage]),
    ).toEqual([]);
  });
});

describe('what a spell that reads hit points did (E8-03)', () => {
  const labels = new Map([
    ['g1', { label: 'Goblin 1', state: CombatantState.UNHURT }],
    ['cap', { label: 'Capitão Goblin', state: CombatantState.UNHURT }],
    ['b', { label: 'Brisa', state: CombatantState.DOWN }],
  ]);
  const npcs = new Set(['g1', 'cap']);
  const isNpc = (id: string) => npcs.has(id);
  const sleep = create(SpellCastSchema, {
    spellKey: 'spell:sleep',
    effectKind: SpellEffectKind.POOL,
    effectConditionKey: 'condition:unconscious',
    targets: [
      { combatantId: 'g1', effect: { outcome: SpellEffectOutcome.AFFECTED } },
      { combatantId: 'cap', effect: { outcome: SpellEffectOutcome.NOT_AFFECTED } },
    ],
  });

  it('tells a player who fell asleep and who was not affected, in words and an icon', () => {
    const rows = castRows(sleep, new Map(), labels);
    expect(rows.map((r) => [r.label, r.word, r.icon])).toEqual([
      ['Goblin 1', 'Adormeceu', 'bedtime'],
      ['Capitão Goblin', 'Não foi afetado', 'block'],
    ]);
    expect(effectSentence(sleep, labels, isNpc)).toBe(
      'O Goblin 1 adormeceu. O Capitão Goblin não foi afetado.',
    );
  });

  it('shows a player no enemy hit points, no order of the pool and no reason', () => {
    // What the server sends a player: only the outcome (the proto's comments say who gets what).
    const rows = castRows(sleep, new Map(), labels);
    const everything = JSON.stringify(rows) + effectSentence(sleep, labels, isNpc);
    expect(everything).not.toMatch(/PV|restam|restantes|\d{2,}/);
  });

  it("names a player's character without an article, and feminine words agree", () => {
    const spare = create(SpellCastSchema, {
      spellKey: 'spell:spare-the-dying',
      effectKind: SpellEffectKind.ZERO_HP,
      targets: [{ combatantId: 'b', effect: { outcome: SpellEffectOutcome.AFFECTED } }],
    });
    expect(effectSentence(spare, labels, isNpc)).toBe('Brisa ficou estável.');
    const missed = create(SpellCastSchema, {
      spellKey: 'spell:spare-the-dying',
      effectKind: SpellEffectKind.ZERO_HP,
      targets: [
        {
          combatantId: 'b',
          effect: {
            outcome: SpellEffectOutcome.NOT_AFFECTED,
            reason: SpellEffectReason.NOT_AT_ZERO,
          },
        },
      ],
    });
    expect(effectSentence(missed, labels, isNpc)).toBe('Brisa não foi afetada.');
  });

  it('writes the heal as an amount only when the server sent it, never on an NPC for the caster', () => {
    const heal = (healed?: number) =>
      create(SpellCastSchema, {
        spellKey: 'spell:heal',
        effectKind: SpellEffectKind.FLAT_HEAL,
        targets: [{ combatantId: 'b', effect: { outcome: SpellEffectOutcome.AFFECTED, healed } }],
      });
    expect(castRows(heal(70), new Map(), labels)[0].lines).toEqual(['70 PV recuperados']);
    expect(castRows(heal(), new Map(), labels)[0]).toMatchObject({ word: 'Foi curada', lines: [] });
  });

  it('a threshold that kills says "Morreu"; one with a condition says which', () => {
    const kill = create(SpellCastSchema, {
      effectKind: SpellEffectKind.THRESHOLD,
      targets: [{ combatantId: 'cap', effect: { outcome: SpellEffectOutcome.AFFECTED } }],
    });
    expect(castRows(kill, new Map(), labels)[0].word).toBe('Morreu');
    const stun = create(SpellCastSchema, {
      effectKind: SpellEffectKind.THRESHOLD,
      effectConditionKey: 'condition:stunned',
      targets: [{ combatantId: 'cap', effect: { outcome: SpellEffectOutcome.AFFECTED } }],
    });
    expect(castRows(stun, new Map(), labels)[0].word).toBe('Ficou atordoado');
    expect(effectSentence(stun, labels, isNpc)).toBe('O Capitão Goblin ficou atordoado.');
  });

  it('is empty for a spell that does not read hit points', () => {
    const missile = create(SpellCastSchema, { targets: [{ combatantId: 'g1', darts: 2 }] });
    expect(effectSentence(missile, labels, isNpc)).toBe('');
    expect(castRows(missile, new Map(), labels)[0].icon).toBeUndefined();
  });
});
