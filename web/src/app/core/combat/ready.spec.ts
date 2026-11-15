import { create } from '@bufbuild/protobuf';

import {
  AreaPlacement,
  GetTurnOptionsResponseSchema,
  ReadiedKind,
  SpellTargetsSchema,
} from '../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  ActionOptionSchema,
  AttackKind,
  DisabledReasonCode,
  TurnOptionsSchema,
  type TurnOptions,
} from '../../../gen/meurpg/rules/v1/rules_pb';
import {
  heldName,
  isReadyOption,
  readyArgs,
  readyAttackRows,
  readyKinds,
  readyMissing,
  readyOtherRows,
  readySpeed,
  readySpellRows,
  triggerState,
} from './ready';

/** The text with the no-break spaces of `tight()` written as plain ones. */
const plain = (s: string) => s.replace(/\u00a0/g, ' ');

const sword = {
  attack: {
    key: 'equipment:longsword',
    name: 'Longsword',
    namePt: 'Espada longa',
    attackBonus: 6,
    damage: '1d8+3',
    damageTypePt: 'cortante',
    kind: AttackKind.WEAPON,
    rangeFt: 5,
  },
  enabled: true,
};
const fireBolt = {
  attack: {
    key: 'spell:fire-bolt',
    name: 'Fire Bolt',
    namePt: 'Raio de Fogo',
    attackBonus: 5,
    damage: '1d10',
    damageTypePt: 'fogo',
    kind: AttackKind.SPELL,
    rangeFt: 120,
  },
  enabled: true,
};
const missiles = {
  spell: { key: 'spell:magic-missile', name: 'Magic Missile', namePt: 'Mísseis Mágicos', level: 1 },
  economy: ActionEconomy.ACTION,
  enabled: true,
  slots: [
    { level: 1, free: 2, total: 4 },
    { level: 2, free: 1, total: 2 },
  ],
};
const shield = {
  spell: { key: 'spell:shield', name: 'Shield', namePt: 'Escudo Arcano', level: 1 },
  economy: ActionEconomy.REACTION,
  enabled: false,
  slots: [{ level: 1, free: 2, total: 4 }],
};
const misty = {
  spell: { key: 'spell:misty-step', name: 'Misty Step', namePt: 'Passo Nebuloso', level: 2 },
  economy: ActionEconomy.BONUS_ACTION,
  enabled: true,
  slots: [{ level: 2, free: 1, total: 2 }],
};
const scorching = {
  spell: { key: 'spell:scorching-ray', name: 'Scorching Ray', namePt: 'Raio Ardente', level: 2 },
  economy: ActionEconomy.ACTION,
  enabled: false,
  reason: { code: DisabledReasonCode.NO_SLOT },
  slots: [],
};
const spellOf = (key: string, name: string, level = 1) => ({
  spell: { key, name, namePt: name, level },
  economy: ActionEconomy.ACTION,
  enabled: true,
  slots: [{ level, free: 2, total: 2 }],
});
const charm = spellOf('spell:charm-person', 'Enfeitiçar Pessoa');
const burning = spellOf('spell:burning-hands', 'Mãos Flamejantes');
const sleep = spellOf('spell:sleep', 'Sono');
const bless = spellOf('spell:bless', 'Bênção');
const saveCantrip = (key: string, name: string) => ({
  attack: {
    key,
    name,
    namePt: name,
    saveDc: 13,
    damage: '1d8',
    kind: AttackKind.SPELL,
    rangeFt: 60,
  },
  enabled: true,
});
const sacredFlame = saveCantrip('spell:sacred-flame', 'Chama Sagrada');
const thunderclap = saveCantrip('spell:thunderclap', 'Trovão');
const std = (key: string, namePt: string, over: Record<string, unknown> = {}) => ({
  action: { key, namePt, economy: ActionEconomy.ACTION },
  enabled: true,
  ...over,
});

function options(over: Record<string, unknown[]> = {}): TurnOptions {
  const standardActions = [
    std('standard:attack', 'Atacar'),
    std('standard:cast-a-spell', 'Conjurar'),
    std('standard:dash', 'Disparada'),
    std('standard:disengage', 'Desengajar'),
    std('standard:dodge', 'Esquivar'),
    std('standard:help', 'Ajudar'),
    std('standard:hide', 'Esconder'),
    std('standard:search', 'Procurar'),
    std('standard:use-object', 'Usar um objeto'),
    std('standard:ready', 'Preparar', { viaReadyAction: true }),
  ];
  return create(TurnOptionsSchema, {
    attacks: [sword],
    spells: [],
    standardActions,
    ...over,
  } as never);
}

describe('Ready (#582): the kinds', () => {
  it('lists Ataque, Mover and Outra for a fighter, with no Magia', () => {
    const kinds = readyKinds(options());
    expect(kinds.map((k) => k.label)).toEqual(['Ataque', 'Mover', 'Outra']);
    expect(kinds.filter((k) => k.key !== 'move').every((k) => k.blocked === '')).toBe(true);
  });

  it('puts Magia second for a caster, whether it has a spell of the list or only a cantrip that attacks', () => {
    const list = readyKinds(options({ spells: [missiles] }));
    expect(list.map((k) => k.label)).toEqual(['Ataque', 'Magia', 'Mover', 'Outra']);
    const cantrip = readyKinds(options({ attacks: [fireBolt] }));
    expect(cantrip.map((k) => k.label)).toEqual(['Ataque', 'Magia', 'Mover', 'Outra']);
    expect(cantrip[0].blocked).toBe('Você não tem um ataque para preparar.');
  });

  it('turns Mover off with its reason, whatever the map and the speed: the window does not move the token yet (#856)', () => {
    const move = readyKinds(options()).find((k) => k.key === 'move');
    expect(move?.blocked).toBe(
      'Mover pela janela ainda não dá: por enquanto o mestre move a ficha.',
    );
  });

  it('knows the Ready action by the server flag, not by its name', () => {
    const ready = options().standardActions.find((a) => a.action?.key === 'standard:ready');
    expect(isReadyOption(ready!)).toBe(true);
    expect(isReadyOption(create(ActionOptionSchema, { enabled: true }))).toBe(false);
  });
});

describe('Ready (#582): the lists', () => {
  it('shows the weapon attacks with the bonus, the damage and the reach', () => {
    const rows = readyAttackRows(options({ attacks: [sword, fireBolt] }));
    expect(rows).toHaveLength(1);
    expect(rows[0].id).toBe('equipment:longsword');
    expect(rows[0].title).toBe('Espada longa');
    expect(plain(rows[0].sub)).toBe('+6 para acertar · 1d8 + 3 cortante · corpo a corpo, 1,5 m');
  });

  it('lists a cantrip that attacks as a spell with no slot', () => {
    const rows = readySpellRows(options({ attacks: [sword, fireBolt] }));
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      id: 'spell:fire-bolt',
      level: 0,
      blocked: '',
      slots: [],
      spell: true,
    });
    expect(plain(rows[0].sub)).toBe('Truque · ataque de magia · 1d10 de fogo · alcance 36 m');
  });

  it('lists a leveled spell with the circles it can be held with and how many are free', () => {
    const [row] = readySpellRows(options({ spells: [missiles] }));
    expect(row.title).toBe('Mísseis Mágicos');
    expect(row.level).toBe(1);
    expect(row.slots).toEqual([
      { level: 1, text: '1º círculo · restam 2' },
      { level: 2, text: '2º círculo · resta 1' },
    ]);
    expect(row.blocked).toBe('');
  });

  it('writes the reason in the card of a spell that is not of 1 action, or has no slot', () => {
    const rows = readySpellRows(options({ spells: [shield, misty, scorching] }));
    expect(rows.map((r) => r.blocked)).toEqual([
      'É uma reação: só uma magia de 1 ação pode ser preparada.',
      'É uma ação bônus: só uma magia de 1 ação pode ser preparada.',
      'Sem espaços de 2º círculo.',
    ]);
  });

  it('turns off, with a reason, the spell the window cannot release: darts, a point or an area, several targets', () => {
    const targets = (spellKey: string, over: Record<string, unknown> = {}) =>
      create(SpellTargetsSchema, { spellKey, maxTargets: 1, ...over } as never);
    const rows = readySpellRows(
      options({ spells: [missiles, shield, charm, burning, sleep, bless] }),
      [
        targets('spell:magic-missile', { maxTargets: 3, darts: [{ slotLevel: 1, darts: 3 }] }),
        targets('spell:charm-person'),
        targets('spell:burning-hands', { maxTargets: 0, placement: AreaPlacement.DIRECTION }),
        targets('spell:sleep', { maxTargets: 0, placement: AreaPlacement.POINT }),
        targets('spell:bless', { maxTargets: 3 }),
      ],
    );
    const why =
      'Soltar pela janela ainda não dá para esta magia: ela pede dardos, alvos ou um ponto no mapa.';
    expect(rows.map((r) => [r.id, r.blocked])).toEqual([
      ['spell:magic-missile', why],
      ['spell:shield', 'É uma reação: só uma magia de 1 ação pode ser preparada.'],
      ['spell:charm-person', ''],
      ['spell:burning-hands', why],
      ['spell:sleep', why],
      ['spell:bless', why],
    ]);
  });

  it('turns off a save cantrip that asks for an area, and keeps the cantrip that attacks', () => {
    const rows = readySpellRows(options({ attacks: [fireBolt, sacredFlame, thunderclap] }), [
      create(SpellTargetsSchema, { spellKey: 'spell:sacred-flame', maxTargets: 1 }),
      create(SpellTargetsSchema, {
        spellKey: 'spell:thunderclap',
        maxTargets: 0,
        placement: AreaPlacement.CASTER,
      }),
    ]);
    expect(rows.map((r) => [r.id, r.blocked === ''])).toEqual([
      ['spell:fire-bolt', true],
      ['spell:sacred-flame', true],
      ['spell:thunderclap', false],
    ]);
  });

  it('leaves out a spell a feature grants: only the class spells are held', () => {
    const granted = {
      ...missiles,
      grant: { sourceKey: 'trait:x', sourceNamePt: 'X', atWill: true },
    };
    expect(readySpellRows(options({ spells: [granted] }))).toHaveLength(0);
  });

  it('offers Esquivar, Procurar and Usar um objeto, and nothing the server refuses', () => {
    const rows = readyOtherRows(options());
    expect(rows.map((r) => r.title)).toEqual(['Esquivar', 'Procurar', 'Usar um objeto']);
    expect(rows.map((r) => r.id)).toEqual([
      'standard:dodge',
      'standard:search',
      'standard:use-object',
    ]);
  });
});

describe('Ready (#582): the trigger', () => {
  it('counts the trimmed text and says nothing until the field was touched', () => {
    expect(triggerState('', false)).toEqual({ count: 0, error: '' });
    expect(triggerState('  ', true)).toEqual({ count: 0, error: 'Escreva quando a ação dispara.' });
    expect(triggerState(' Quando o zumbi passar ', true)).toEqual({ count: 21, error: '' });
  });

  it('refuses more than 200 characters at once, even before the field was touched', () => {
    expect(triggerState('a'.repeat(200), false).error).toBe('');
    expect(triggerState('a'.repeat(201), false)).toEqual({
      count: 201,
      error: 'Até 200 caracteres.',
    });
  });
});

describe('Ready (#582): what is sent', () => {
  const pick = {
    kind: 'attack' as const,
    key: 'equipment:longsword',
    slotLevel: 0,
    targetId: 'z1',
    trigger: ' Se o zumbi vier ',
  };

  it('sends an attack as ATTACK with its key, the target and the trimmed trigger', () => {
    expect(readyArgs(pick)).toEqual({
      kind: ReadiedKind.ATTACK,
      actionKey: 'equipment:longsword',
      spellKey: '',
      slotLevel: 0,
      targetCombatantId: 'z1',
      triggerText: 'Se o zumbi vier',
    });
  });

  it('sends a spell with its key and the slot level, and no action key', () => {
    expect(
      readyArgs({ ...pick, kind: 'spell', key: 'spell:magic-missile', slotLevel: 2 }),
    ).toMatchObject({
      kind: ReadiedKind.SPELL,
      actionKey: '',
      spellKey: 'spell:magic-missile',
      slotLevel: 2,
    });
  });

  it('sends a move with no key, and another action with its key', () => {
    expect(readyArgs({ ...pick, kind: 'move', key: 'ignored' })).toMatchObject({
      kind: ReadiedKind.MOVE,
      actionKey: '',
      spellKey: '',
    });
    expect(readyArgs({ ...pick, kind: 'other', key: 'standard:dodge' })).toMatchObject({
      kind: ReadiedKind.OTHER,
      actionKey: 'standard:dodge',
    });
  });

  it('says what is missing before "Preparar" works: the choice, the slot, then the trigger', () => {
    const rows = readySpellRows(options({ spells: [missiles, shield] }));
    const atk = readyAttackRows(options())[0];
    expect(readyMissing({ ...pick, key: '' }, undefined, '')).toBe('Escolha o que preparar.');
    expect(readyMissing(pick, atk, '')).toBe('');
    expect(readyMissing({ ...pick, trigger: '  ' }, atk, '')).toBe(
      'Escreva quando a ação dispara.',
    );
    expect(
      readyMissing({ ...pick, kind: 'spell', key: missiles.spell.key, slotLevel: 0 }, rows[0], ''),
    ).toBe('Escolha o espaço de magia.');
    expect(readyMissing({ ...pick, kind: 'spell', key: shield.spell.key }, rows[1], '')).toBe(
      'É uma reação: só uma magia de 1 ação pode ser preparada.',
    );
    expect(
      readyMissing(
        { ...pick, kind: 'move', key: '' },
        undefined,
        'Sem mapa neste combate: não há para onde andar.',
      ),
    ).toBe('Sem mapa neste combate: não há para onde andar.');
    expect(readyMissing({ ...pick, kind: 'move', key: '' }, undefined, '')).toBe('');
  });
});

describe('Ready (#582): the name of a held action', () => {
  it('names a move by its limit, and the others by the action and the creature', () => {
    expect(plain(heldName({ kind: ReadiedKind.MOVE, actionNamePt: '', moveFt: 25 }))).toBe(
      'Mover, até 7,5 m',
    );
    expect(
      heldName({ kind: ReadiedKind.ATTACK, actionNamePt: 'Espada longa', moveFt: 0 }, 'no Zumbi 1'),
    ).toBe('Espada longa no Zumbi 1');
    expect(heldName({ kind: ReadiedKind.OTHER, actionNamePt: 'Esquivar', moveFt: 0 })).toBe(
      'Esquivar',
    );
  });

  it('counts characters like the server (runes, not UTF-16 units): 200 emoji are allowed, 201 are not', () => {
    expect(triggerState('😀'.repeat(200), false)).toEqual({ count: 200, error: '' });
    expect(triggerState(` ${'😀'.repeat(201)} `, false).error).toBe('Até 200 caracteres.');
  });
});

describe('Ready (#582): the speed a readied move may have', () => {
  const turn = (movementFt: number | null, grappled = false) =>
    create(GetTurnOptionsResponseSchema, {
      options: create(TurnOptionsSchema, {
        economy: movementFt === null ? undefined : { movement: { speedFt: movementFt } },
      }),
      contestState: grappled ? { grappled: true } : undefined,
    } as never);

  it('does not block a swimmer or a flier with no walking speed: the server takes the most of the speeds on any surface', () => {
    expect(readySpeed(0, turn(30))).toEqual({ noSpeed: false, speedFt: 0 });
    expect(readySpeed(30, turn(30))).toEqual({ noSpeed: false, speedFt: 30 });
  });

  it('blocks it only for a grappled readier or a speed of 0 the server says', () => {
    expect(readySpeed(30, turn(30, true)).noSpeed).toBe(true);
    expect(readySpeed(30, turn(0)).noSpeed).toBe(true);
  });

  it('derives nothing from the walking speed when the options carry no movement: the server refuses', () => {
    expect(readySpeed(0, turn(null))).toEqual({ noSpeed: false, speedFt: 0 });
  });
});
