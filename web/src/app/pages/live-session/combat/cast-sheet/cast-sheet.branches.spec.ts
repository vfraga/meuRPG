// The branches of `cast-sheet.ts` the other specs of the cast sheet never took (found with the coverage report): the page
// before the combat is read, what "Conjurar" waits for, the targets and darts that follow the slot, the result's notes,
// the damage still to roll, and the questions a cast can answer with. They read the sheet through its protected members,
// as the specs next to this one do, and were written green on the code as it was before it was split.
import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { MatDialog } from '@angular/material/dialog';
import { MAT_DIALOG_DATA, MatDialogRef } from '@angular/material/dialog';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  AttackOutcome,
  CombatantKind,
  DiceRollSchema,
  LuckyOfferSchema,
  OutsideInspirationOfferSchema,
  PendingDamageSchema,
  PendingDamageStatus,
  PreviewSpellAreaResponseSchema,
  SpellCastSchema,
  SpellTargetsSchema,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import { RollMode } from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import {
  Ability,
  ActionEconomy,
  SpellAreaShape,
  SpellAttackType,
  SpellHitPointEffectKind,
  SpellRangeKind,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatClient } from '../../../../core/combat/combat-client';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import { LuckyQuestion } from '../../../../core/combat/lucky-question';
import { SpellCatalog } from '../../../../core/combat/spell-catalog';
import { InspirationQuestion } from '../../../../core/resources/bardic-inspiration';
import { ResourceClient } from '../../../../core/resources/resources-client';
import { CastSheet, type CastSheetData } from './cast-sheet';
import { resumedCastOf } from './cast-sheet-view';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

/** The sheet seen from a spec: protected members and all. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type Sheet = any;

/** The parts the sheet hands its members to: a name a spec reads that the sheet no longer holds is found in one of them. */
const PARTS = ['s', 'ch', 'ar', 'md', 'tx', 'questions', 'damage', 'caller'];
/** The members that changed their name on the way. */
const RENAMED: Record<string, string> = {
  castRolled: 'caller.cast',
  castPooled: 'caller.cast',
  setMode: 'md.set',
  modeState: 'md.state',
  areaRows: 'ar.rows',
  areaCount: 'ar.count',
  areaWarning: 'ar.warning',
};

/** The sheet as the specs below read it, the way it was before it handed its members to plain classes. */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function seen(cmp: any): Sheet {
  return new Proxy(cmp, {
    get(target, key: string) {
      if (key === 'castPair') {
        return (faces: number[]) => target.caller.cast({ faces });
      }
      if (key === 'resumedCast') {
        return () => resumedCastOf(target.damage.pendings().values());
      }
      const found = key in target ? '' : (PARTS.find((p) => key in target[p]) ?? '');
      const path = RENAMED[key] ?? (found ? `${found}.${key}` : '');
      if (!path) {
        return target[key];
      }
      const steps = path.split('.');
      const owner = steps.slice(0, -1).reduce((o, k) => o[k], target);
      const name = steps[steps.length - 1];
      const value = owner[name];
      // A signal is a field of its owner and stays as it is; a method is bound to it.
      return typeof value === 'function' && !Object.hasOwn(owner, name) ? value.bind(owner) : value;
    },
  });
}

/** The sheet writes circles with a non-breaking space. */
const plain = (text: string) => text.replace(/\u00a0/g, ' ');

const salvia = (over: Record<string, unknown> = {}) =>
  combatant({
    id: 's',
    label: 'Sálvia',
    kind: CombatantKind.PLAYER,
    characterId: 'sc',
    mine: true,
    ...over,
  });
const goblin = (n: number, over: Record<string, unknown> = {}) =>
  combatant({ id: `g${n}`, label: `Goblin ${n}`, ...over });

/** A target the caster can reach. */
const reach = (id: string, over: Record<string, unknown> = {}) => ({
  combatantId: id,
  label: id.toUpperCase(),
  distanceFt: 10,
  tooFar: false,
  rollMode: RollMode.NORMAL,
  sources: [],
  ...over,
});

const targetsOf = (over: Record<string, unknown> = {}) =>
  create(SpellTargetsSchema, {
    spellKey: 'spell:x',
    maxTargets: 2,
    targets: [reach('g1'), reach('g2')],
    ...over,
  } as never);

const slot1 = { level: 1, free: 2, pact: false };
const slot2 = { level: 2, free: 1, pact: false };

interface Over {
  data?: Record<string, unknown>;
  details?: unknown;
  combatants?: ReturnType<typeof combatant>[];
  noEncounter?: boolean;
  client?: Record<string, unknown>;
  resources?: Record<string, unknown>;
}

async function open(over: Over = {}) {
  TestBed.resetTestingModule();
  const apply = vi.fn();
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    casterId: 's',
    round: 2,
    spellKey: 'spell:x',
    name: 'Feitiço',
    level: 1,
    concentration: false,
    cantripDice: '',
    economy: ActionEconomy.ACTION,
    slots: [slot1, slot2],
    usage: [
      { level: 1, total: 4, used: 2 },
      { level: 2, total: 2, used: 1 },
    ],
    pact: null,
    targets: undefined,
    shieldFree: null,
    shieldName: '',
    attackBonus: 5,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state: {
      encounter: signal(
        over.noEncounter
          ? null
          : encounter({ combatants: over.combatants ?? [salvia(), goblin(1)] }),
      ),
      apply,
    },
    ...over.data,
  } as unknown as CastSheetData;
  const close = vi.fn();
  const client = { castSpell: vi.fn(), rollDamage: vi.fn(), ...over.client };
  const resources = {
    answerOutsideLucky: vi.fn(),
    answerOutsideInspiration: vi.fn(),
    ...over.resources,
  };
  const details =
    over.details === undefined
      ? { spell: { level: 1 }, damage: [], healBySlotLevel: {}, higherLevel: [] }
      : over.details;
  TestBed.configureTestingModule({
    providers: [
      { provide: CombatClient, useValue: fakeCombatClient(client) },
      { provide: ResourceClient, useValue: resources },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(details) } },
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close } },
    ],
  });
  const fixture = TestBed.createComponent(CastSheet);
  fixture.detectChanges();
  await fixture.whenStable();
  fixture.detectChanges();
  return {
    fixture,
    sheet: seen(fixture.componentInstance),
    el: fixture.nativeElement as HTMLElement,
    client,
    resources,
    apply,
    close,
    settle: async () => {
      fixture.detectChanges();
      await fixture.whenStable();
      fixture.detectChanges();
    },
  };
}

beforeEach(() => {
  // The sheet scrolls its error into view; the test DOM has no scroll.
  Element.prototype.scrollTo = vi.fn();
});

describe('CastSheet: before the combat is read', () => {
  it('has no combatants, names, creatures or concentration to show', async () => {
    const { sheet } = await open({ noEncounter: true });
    expect(sheet.combatants()).toEqual([]);
    expect(sheet.casterLabel()).toBe('');
    expect(sheet.npcs().size).toBe(0);
    expect(sheet.creatures().size).toBe(0);
    expect(sheet.labels().size).toBe(0);
    expect(sheet.endsConcentration()).toBeNull();
    expect(sheet.areaRows()).toEqual([]);
    expect(plain(sheet.areaCount())).toBe('0 criaturas');
    expect(sheet.areaWarning()).toBeNull();
    expect(sheet.coverNote()).toBe('');
  });
});

describe('CastSheet: the other concentration it ends', () => {
  const holding = (over: Record<string, unknown>) =>
    salvia({ concentrationSpell: 'spell:web', concentrationSpellNamePt: 'Teia', ...over });
  const creature = (id: string, label: string) =>
    combatant({
      id,
      label,
      kind: CombatantKind.CREATURE,
      characterId: '',
      ownerCharacterId: 'sc',
      summonGroupId: 'cast',
      monsterKey: 'monster:dire-wolf',
      monsterNamePt: 'Lobo atroz',
    });

  it('names the one creature that goes with it, with the article of its name', async () => {
    const he = await open({
      data: { concentration: true },
      combatants: [holding({}), creature('w1', 'Lobo atroz')],
    });
    expect(he.sheet.endsConcentration().goes).toBe('O Lobo atroz some.');
    const she = await open({
      data: { concentration: true },
      combatants: [holding({}), creature('w1', 'Loba atroz')],
    });
    expect(she.sheet.endsConcentration().goes).toBe('A Loba atroz some.');
  });

  it('puts a group of creatures in the plural, and calls the other spell "a magia" when it has no name', async () => {
    const { sheet } = await open({
      data: { concentration: true },
      combatants: [
        holding({ concentrationSpellNamePt: '' }),
        creature('w1', 'Lobo atroz 1'),
        creature('w2', 'Lobo atroz 2'),
      ],
    });
    expect(sheet.endsConcentration()).toMatchObject({
      ends: 'Feitiço encerra a concentração em a magia.',
      goes: expect.stringContaining('somem.'),
    });
  });

  it('counts no creature that is defeated', async () => {
    const { sheet } = await open({
      data: { concentration: true },
      combatants: [holding({}), creature('w1', 'Lobo atroz 1')].map((c, i) =>
        i === 1 ? ({ ...c, defeated: true } as typeof c) : c,
      ),
    });
    expect(sheet.endsConcentration().goes).toBe('');
  });
});

describe('CastSheet: what "Conjurar" waits for', () => {
  it('asks for a slot when the spell has none to pick', async () => {
    const { sheet } = await open({ data: { slots: [], usage: [] } });
    expect(sheet.missing()).toBe('Escolha o espaço de magia.');
  });

  it('says what a spell with darts still lacks, and is ready when every dart is placed', async () => {
    const darts = targetsOf({
      spellKey: 'spell:magic-missile',
      darts: [{ slotLevel: 1, darts: 3 }],
      targets: [reach('g1'), reach('g2')],
    });
    const { sheet } = await open({
      data: { spellKey: 'spell:magic-missile', targets: darts },
      combatants: [salvia(), goblin(1), goblin(2)],
    });
    expect(sheet.dartsLine()).not.toBe('');
    expect(sheet.missing()).toBe(sheet.dartsLine());
    sheet.deal({ id: 'g1', delta: 1 });
    sheet.deal({ id: 'g1', delta: 1 });
    sheet.deal({ id: 'g2', delta: 1 });
    expect(sheet.missing()).toBe('');
  });

  it('asks for one target, or at least one of several', async () => {
    const one = await open({ data: { targets: targetsOf({ maxTargets: 1 }) } });
    expect(one.sheet.missing()).toBe('Escolha o alvo.');
    const many = await open({ data: { targets: targetsOf({ maxTargets: 2 }) } });
    expect(many.sheet.missing()).toBe('Escolha pelo menos um alvo.');
  });

  it('asks for one target at a time for a spell attack with physical dice', async () => {
    const details = {
      spell: { level: 1 },
      attackType: SpellAttackType.RANGED,
      damage: [],
      healBySlotLevel: {},
      higherLevel: [],
    };
    const { sheet } = await open({
      data: { targets: targetsOf({ maxTargets: 2 }), diceMode: DiceMode.PHYSICAL },
      details,
      combatants: [salvia(), goblin(1), goblin(2)],
    });
    sheet.toggle('g1');
    sheet.toggle('g2');
    expect(sheet.missing()).toBe('Com dados físicos, escolha um alvo por vez.');
  });
});

describe('CastSheet: the targets that follow the slot', () => {
  it('trims the choice when a lower slot takes fewer targets', async () => {
    const { sheet, settle } = await open({
      data: {
        targets: targetsOf({ maxTargets: 1, extraTargetPerLevel: true, targetsPerLevel: 1 }),
      },
      combatants: [salvia(), goblin(1), goblin(2)],
    });
    sheet.pickSlot(sheet.rows().find((r: { level: number }) => r.level === 2));
    sheet.toggle('g1');
    sheet.toggle('g2');
    expect(sheet.chosen()).toEqual(['g1', 'g2']);
    sheet.pickSlot(sheet.rows().find((r: { level: number }) => r.level === 1));
    await settle();
    expect(sheet.chosen().length).toBe(1);
  });

  it('places every dart on the only target it can reach, and takes them back when the slot takes fewer', async () => {
    const darts = targetsOf({
      spellKey: 'spell:magic-missile',
      darts: [
        { slotLevel: 1, darts: 3 },
        { slotLevel: 2, darts: 4 },
      ],
      targets: [reach('g1')],
    });
    const { sheet, settle } = await open({
      data: { spellKey: 'spell:magic-missile', targets: darts, slots: [slot1, slot2] },
      combatants: [salvia(), goblin(1)],
    });
    await settle();
    expect(sheet.dealt().get('g1')).toBe(3);
    sheet.pickSlot(sheet.rows().find((r: { level: number }) => r.level === 2));
    sheet.deal({ id: 'g1', delta: 1 });
    expect(sheet.dealt().get('g1')).toBe(4);
    sheet.pickSlot(sheet.rows().find((r: { level: number }) => r.level === 1));
    await settle();
    // The extra dart is taken back, and the only target gets the three of the lower slot again.
    expect(sheet.dealt().get('g1')).toBe(3);
  });

  it('keeps the other picks of Metamagic that still stand when a target leaves', async () => {
    const { sheet } = await open({
      data: { targets: targetsOf({ maxTargets: 2 }) },
      combatants: [salvia(), goblin(1), goblin(2)],
    });
    sheet.toggle('g1');
    sheet.toggle('g2');
    sheet.setPicks({ twinned: 'g1', careful: 'g2', heightened: 'g2' });
    sheet.toggle('g2');
    expect(sheet.picks()).toEqual({ twinned: '', careful: 'g2', heightened: '' });
  });
});

describe('CastSheet: what it says about the reach and the roll mode', () => {
  it('knows how far a ranged spell reaches, and nothing for any other', async () => {
    const ranged = await open({
      details: {
        spell: { level: 1 },
        range: { kind: SpellRangeKind.RANGED, distanceFt: 60 },
        damage: [],
        healBySlotLevel: {},
        higherLevel: [],
      },
    });
    expect(ranged.sheet.reachFt()).toBe(60);
    const touch = await open({
      details: {
        spell: { level: 1 },
        range: { kind: SpellRangeKind.TOUCH, distanceFt: 5 },
        damage: [],
        healBySlotLevel: {},
        higherLevel: [],
      },
    });
    expect(touch.sheet.reachFt()).toBeNull();
  });

  it('suggests the mode of the single target it attacks, with its sources', async () => {
    const targets = targetsOf({
      maxTargets: 2,
      targets: [reach('g1', { rollMode: RollMode.ADVANTAGE, sources: ['x'] }), reach('g2')],
    });
    const { sheet } = await open({
      data: { targets },
      combatants: [salvia(), goblin(1), goblin(2)],
    });
    sheet.toggle('g1');
    expect(sheet.suggested()).toBe(RollMode.ADVANTAGE);
    expect(sheet.sources()).toEqual(['x']);
    sheet.toggle('g2');
    expect(sheet.mixed()).toBe(true);
    expect(sheet.modeState()).toBe('ready');
    expect(sheet.sources()).toEqual([]);
    sheet.setMode(RollMode.DISADVANTAGE);
    expect(sheet.combine()).toBe('lower');
  });
});

describe('CastSheet: Sculpt Spells', () => {
  const evocation = {
    sculptSpells: true,
    targets: targetsOf({ maxTargets: 3, targets: [reach('g1'), reach('g2'), reach('g3')] }),
  };
  const three = [salvia(), goblin(1), goblin(2), goblin(3)];

  it('lists the creatures ticked in the list, and spares at most the limit', async () => {
    const { sheet } = await open({ data: evocation, combatants: three });
    sheet.toggle('g1');
    sheet.toggle('g2');
    sheet.toggle('g3');
    expect(sheet.sculptRows().map((r: { id: string }) => r.id)).toEqual(['g1', 'g2', 'g3']);
    expect(sheet.sculptLimit()).toBe(2);
    sheet.toggleSculpt('g1');
    sheet.toggleSculpt('g2');
    sheet.toggleSculpt('g3');
    expect(sheet.sculpted()).toEqual(['g1', 'g2']);
    sheet.toggleSculpt('g1');
    expect(sheet.sculpted()).toEqual(['g2']);
  });

  it('lists nothing for a caster without Sculpt Spells', async () => {
    const { sheet } = await open({ data: { targets: evocation.targets }, combatants: three });
    sheet.toggle('g1');
    expect(sheet.sculptRows()).toEqual([]);
  });
});

describe('CastSheet: the title and the subtitle', () => {
  const pool = {
    spell: { level: 1 },
    hitPointEffect: { kind: 1 },
    damage: [],
    healBySlotLevel: {},
    higherLevel: [],
  };

  it('counts the creatures in the area of a spell that rolls a pool', async () => {
    const { sheet } = await open({ details: pool });
    const kind = sheet.kind();
    const cast = create(SpellCastSchema, {
      targets: [{ combatantId: 'g1' }, { combatantId: 'g2' }],
      slot: { level: 1, pact: false },
    });
    sheet.cast.set(cast);
    if (kind === 'pool') {
      expect(plain(sheet.subtitle())).toBe('1º nível · 2 criaturas na área');
    } else {
      expect(plain(sheet.subtitle())).toBe('1º nível');
    }
  });

  it('says a cantrip\'s circle as "Truque" once cast', async () => {
    const { sheet } = await open({ data: { level: 0, slots: [], usage: [] } });
    sheet.cast.set(create(SpellCastSchema, {}));
    expect(sheet.subtitle()).toBe('Truque');
    expect(sheet.title()).toBe('Você conjurou Feitiço');
  });

  it('says what was cast, in the gender of its name, and the damage still to roll when it resumes', async () => {
    const resumed = await open({
      data: { resume: [create(PendingDamageSchema, { id: 'p1', targetId: 'g1' })] },
    });
    expect(resumed.sheet.title()).toBe('Role o dano de Feitiço');
  });

  it('asks to type the result while typing, with the round', async () => {
    const { sheet } = await open();
    sheet.typing.set(true);
    expect(sheet.title()).toBe('Digite o resultado');
    expect(sheet.subtitle()).toBe('Feitiço · Rodada 2');
  });
});

describe('CastSheet: the result', () => {
  it('lists the notes: the concentration, the one that ended, the metamagic spent and the action', async () => {
    const { sheet } = await open({
      data: {
        metamagic: [{ key: 'metamagic:quickened', namePt: 'Magia Acelerada', cost: 2 }],
        sorceryPoints: { current: 3, max: 5 },
      },
    });
    sheet.cast.set(
      create(SpellCastSchema, {
        concentrating: true,
        concentrationEndedSpellKey: 'spell:web',
        metamagicKeys: ['metamagic:quickened', 'metamagic:unknown'],
        sorceryPointsSpent: 2,
      }),
    );
    const notes: string[] = sheet.notes();
    expect(notes[0]).toBe('Você está concentrado em Feitiço.');
    expect(notes[1]).toBe('A sua concentração anterior acabou.');
    expect(notes[2]).toContain('Magia Acelerada');
    expect(notes[2]).toContain('Metamagia');
    expect(notes.length).toBe(4);
  });

  it('has no notes before the cast, and no pool text without a pool roll', async () => {
    const { sheet } = await open();
    expect(sheet.notes()).toEqual([]);
    expect(sheet.poolText()).toBe('');
    expect(sheet.sentence()).toBe('');
    expect(sheet.plain()).toBe(false);
  });

  it('counts the slots left when the table does not know the total, and says Escudo is gone with the last one', async () => {
    const { sheet } = await open({
      data: {
        slots: [{ level: 1, free: 1, pact: false }],
        usage: [],
        shieldFree: 1,
        shieldName: 'Escudo Arcano',
      },
    });
    sheet.cast.set(create(SpellCastSchema, { slot: { level: 1, pact: false } }));
    expect(sheet.after()).toMatchObject({ level: 1, used: 0 });
    expect(plain(sheet.shieldLost())).toBe('Escudo Arcano indisponível: sem espaço de 1º nível.');
  });

  it('is plain only for a spell with nothing to roll, hit or save', async () => {
    const { sheet } = await open();
    sheet.cast.set(create(SpellCastSchema, { targets: [{ combatantId: 'g1' }] }));
    expect(sheet.plain()).toBe(true);
    sheet.cast.set(
      create(SpellCastSchema, { targets: [{ combatantId: 'g1', outcome: AttackOutcome.HIT }] }),
    );
    expect(sheet.plain()).toBe(false);
  });
});

describe('CastSheet: the damage still to roll', () => {
  const owed = (over: Record<string, unknown> = {}) =>
    create(PendingDamageSchema, {
      id: 'p1',
      targetId: 'g1',
      status: PendingDamageStatus.AWAITING_ROLL,
      diceCount: 1,
      diceSides: 8,
      castId: 'cast',
      ...over,
    } as never);

  it('says one die, or "some os dois" for two, and a heal as a cure', async () => {
    const single = await open({ data: { resume: [owed()] } });
    expect(single.sheet.nextRoll()).toMatchObject({
      label: 'Role 1d8 para o dano',
      what: 'Dano em Goblin 1',
      crit: '',
    });
    const heal = await open({ data: { resume: [owed({ diceCount: 2, healing: true })] } });
    expect(heal.sheet.nextRoll()).toMatchObject({
      label: expect.stringContaining('some os dois'),
      what: 'Cura em Goblin 1',
    });
    expect(heal.sheet.nextRoll().label).not.toBe(single.sheet.nextRoll().label);
  });

  it('names an unknown target "alvo" and has no roll when none is owed', async () => {
    const unknown = await open({ data: { resume: [owed({ targetId: 'ghost' })] } });
    expect(unknown.sheet.nextRoll().what).toBe('Dano em alvo');
    const none = await open();
    expect(none.sheet.nextRoll()).toBeNull();
  });

  it('says a critical hit asks double dice when the rule says nothing', async () => {
    const { sheet } = await open({ data: { resume: [owed({ critical: true })] } });
    expect(sheet.nextRoll().crit).not.toBe('');
  });

  it('rolls them all in the app, one call each, and stops at the first that fails', async () => {
    const settled = (id: string) => ({
      encounter: encounter({ combatants: [salvia()] }),
      pending: owed({ id, status: PendingDamageStatus.ROLLED, damageTypePt: 'fogo' }),
      cast: [],
    });
    const rollDamage = vi
      .fn()
      .mockResolvedValueOnce(settled('p1'))
      .mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({
      data: {
        resume: [owed(), owed({ id: 'p2', castId: 'other' }), owed({ id: 'p3', castId: 'third' })],
      },
      client: { rollDamage },
    });
    await sheet.rollAll();
    expect(rollDamage).toHaveBeenCalledTimes(2);
    expect(sheet.error()).not.toBe('');
    expect(sheet.pendings().get('p1').status).toBe(PendingDamageStatus.ROLLED);
  });

  it('rolls the typed sum of the first group, and stops typing once it worked', async () => {
    const rollDamage = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      pending: owed({
        status: PendingDamageStatus.ROLLED,
        roll: create(DiceRollSchema, { diceCount: 1, diceSides: 8, faces: [5], total: 5 }),
      }),
      cast: [owed({ id: 'p9', targetId: 'g2', status: PendingDamageStatus.ROLLED })],
    });
    const { sheet, apply } = await open({ data: { resume: [owed()] }, client: { rollDamage } });
    sheet.typing.set(true);
    await sheet.rollTyped(5);
    expect(rollDamage).toHaveBeenCalledWith('c', 'enc', 'p1', { sum: 5 }, expect.any(String));
    expect(apply).toHaveBeenCalled();
    expect(sheet.typing()).toBe(false);
    expect(sheet.pendings().has('p9')).toBe(true);
    const idle = await open();
    await idle.sheet.rollTyped(3);
  });

  it('keeps typing when the typed roll is refused, and sends the same key for the same die', async () => {
    const rollDamage = vi.fn().mockRejectedValue(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ data: { resume: [owed()] }, client: { rollDamage } });
    sheet.typing.set(true);
    await sheet.rollTyped(5);
    await sheet.rollTyped(5);
    expect(sheet.typing()).toBe(true);
    expect(rollDamage.mock.calls[0][4]).toBe(rollDamage.mock.calls[1][4]);
  });

  it('ignores a second roll while one is in the air', async () => {
    const { sheet } = await open({ data: { resume: [owed()] } });
    sheet.busy.set(true);
    expect(await sheet.damage.rollGroup([owed()], { inApp: true })).toBe(false);
  });
});

describe('CastSheet: casting', () => {
  it('asks in place before casting a list spell at nobody, then casts', async () => {
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {}),
    });
    const { sheet } = await open({
      data: { targets: targetsOf({ maxTargets: 0, targets: [reach('g1'), reach('s')] }) },
      client: { castSpell },
    });
    await sheet.castNow();
    expect(sheet.listAsking()).toBe(true);
    expect(castSpell).not.toHaveBeenCalled();
    await sheet.castNow();
    expect(castSpell).toHaveBeenCalledTimes(1);
  });

  it('casts anyway for a list spell, and does nothing to confirm an area it has no flow for', async () => {
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {}),
    });
    const { sheet } = await open({ client: { castSpell } });
    await sheet.castAnyway();
    expect(castSpell).toHaveBeenCalledTimes(1);
    expect(await sheet.confirmArea()).toBe(false);
    sheet.changeArea();
  });

  it('sends a spell that rolls a pool as rolled in the app, and a pair of physical d20 as typed', async () => {
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {}),
    });
    const { sheet } = await open({ client: { castSpell } });
    await sheet.castPair([3, 14]);
    expect(castSpell.mock.calls[0][6]).toEqual({ faces: [3, 14] });
    await sheet.castPooled({ inApp: true });
    expect(castSpell.mock.calls[1][6]).toEqual({ inApp: true });
    await sheet.castRolled({ face: 11 });
    expect(castSpell.mock.calls[2][6]).toEqual({ face: 11 });
  });

  it('ignores a cast while one is in the air', async () => {
    const castSpell = vi.fn();
    const { sheet } = await open({ client: { castSpell } });
    sheet.busy.set(true);
    await sheet.castAnyway();
    expect(castSpell).not.toHaveBeenCalled();
    sheet.close();
  });
});

describe('CastSheet: the d20 of a spell attack held for a question', () => {
  const d20 = (face: number) =>
    create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [face],
      modifier: 5,
      total: face + 5,
    });
  const luckyOffer = create(LuckyOfferSchema, { holdId: 'h1', d20: d20(1), luckyIndex: 0 });
  const dieOffer = create(OutsideInspirationOfferSchema, { holdId: 'h2', d20: d20(9) });
  const enc = () => encounter({ combatants: [salvia()] });

  it('shows the Lucky question when a cast answers with it, and the Bardic Inspiration one the same way', async () => {
    const castSpell = vi
      .fn()
      .mockRejectedValueOnce(new LuckyQuestion(luckyOffer, enc()))
      .mockRejectedValueOnce(new InspirationQuestion(dieOffer, enc()));
    const { sheet, apply } = await open({ client: { castSpell } });
    await sheet.castAnyway();
    expect(sheet.luckyOffer()).toBe(luckyOffer);
    expect(sheet.error()).toBe('');
    await sheet.castAnyway();
    expect(sheet.offer()).toBe(dieOffer);
    expect(apply).toHaveBeenCalledTimes(2);
  });

  it('shows a question whose d20 cannot be animated, without a roll to tumble', async () => {
    const castSpell = vi
      .fn()
      .mockRejectedValue(new LuckyQuestion(create(LuckyOfferSchema, { holdId: 'h' }), enc()));
    const { sheet } = await open({ client: { castSpell } });
    await sheet.castAnyway();
    expect(sheet.luckyOffer()).not.toBeNull();
  });

  it('answers Lucky with the cast, or with the next question on the new roll', async () => {
    const answered = create(SpellCastSchema, { targets: [] });
    const answerOutsideLucky = vi
      .fn()
      .mockResolvedValueOnce({
        result: { case: 'spellCast', value: { encounter: enc(), cast: answered } },
      })
      .mockResolvedValueOnce({
        result: { case: 'spellCast', value: { encounter: enc(), inspirationOffer: dieOffer } },
      });
    const { sheet } = await open({ resources: { answerOutsideLucky } });
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(true, { inApp: true });
    expect(sheet.cast()).toBe(answered);
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(true, { inApp: true });
    expect(sheet.offer()).toBe(dieOffer);
    expect(sheet.luckyOffer()).toBeNull();
  });

  it('ignores a Lucky answer without a question, or while busy', async () => {
    const answerOutsideLucky = vi.fn();
    const { sheet } = await open({ resources: { answerOutsideLucky } });
    await sheet.answerLucky(false, null);
    sheet.luckyOffer.set(luckyOffer);
    sheet.busy.set(true);
    await sheet.answerLucky(false, null);
    expect(answerOutsideLucky).not.toHaveBeenCalled();
  });

  it("says so when Lucky's answer brings no cast, or fails", async () => {
    const answerOutsideLucky = vi
      .fn()
      .mockResolvedValueOnce({ result: { case: undefined } })
      .mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ resources: { answerOutsideLucky } });
    sheet.luckyOffer.set(luckyOffer);
    await sheet.answerLucky(true, { face: 4 });
    expect(sheet.error()).not.toBe('');
    sheet.error.set('');
    await sheet.answerLucky(true, { face: 4 });
    expect(sheet.error()).not.toBe('');
  });

  it('answers the die with the cast, ignores an answer without a question, and says when it fails', async () => {
    const answered = create(SpellCastSchema, {});
    const answerOutsideInspiration = vi
      .fn()
      .mockResolvedValueOnce({
        result: { case: 'spellCast', value: { encounter: enc(), cast: answered } },
      })
      .mockResolvedValueOnce({ result: { case: undefined } })
      .mockRejectedValueOnce(new ConnectError('x', Code.Unavailable));
    const { sheet } = await open({ resources: { answerOutsideInspiration } });
    await sheet.answerDie(true, null);
    expect(answerOutsideInspiration).not.toHaveBeenCalled();
    sheet.offer.set(dieOffer);
    await sheet.answerDie(true, null);
    expect(sheet.cast()).toBe(answered);
    sheet.offer.set(dieOffer);
    await sheet.answerDie(false, null);
    expect(sheet.error()).not.toBe('');
    sheet.error.set('');
    await sheet.answerDie(false, null);
    expect(sheet.error()).not.toBe('');
    sheet.busy.set(true);
    await sheet.answerDie(false, null);
  });
});

describe('CastSheet: the dice animated after the cast', () => {
  const attack = (id: string, face: number, outcome: AttackOutcome) => ({
    combatantId: id,
    outcome,
    attackRoll: create(DiceRollSchema, {
      diceCount: 1,
      diceSides: 20,
      faces: [face],
      modifier: 5,
      total: face + 5,
    }),
  });
  const play = async (cast: ReturnType<typeof create<typeof SpellCastSchema>>) => {
    const castSpell = vi
      .fn()
      .mockResolvedValue({ encounter: encounter({ combatants: [salvia()] }), cast });
    const { sheet } = await open({ client: { castSpell } });
    const animate = vi.spyOn(sheet.animator, 'play');
    await sheet.castAnyway();
    return animate;
  };

  it('animates a single spell attack with its word, and the dice of several as one roll', async () => {
    const one = await play(
      create(SpellCastSchema, { targets: [attack('g1', 20, AttackOutcome.CRITICAL_HIT)] }),
    );
    expect(one).toHaveBeenCalledTimes(1);
    const miss = await play(
      create(SpellCastSchema, { targets: [attack('g1', 2, AttackOutcome.MISS)] }),
    );
    expect(miss).toHaveBeenCalledTimes(1);
    const several = await play(
      create(SpellCastSchema, {
        targets: [attack('g1', 12, AttackOutcome.HIT), attack('g2', 3, AttackOutcome.MISS)],
      }),
    );
    expect(several).toHaveBeenCalledTimes(1);
  });

  it('animates nothing when the cast rolled no attack', async () => {
    const none = await play(create(SpellCastSchema, { targets: [{ combatantId: 'g1' }] }));
    expect(none).not.toHaveBeenCalled();
    const unreadable = await play(
      create(SpellCastSchema, {
        targets: [
          { combatantId: 'g1', attackRoll: create(DiceRollSchema, { diceCount: 0, diceSides: 0 }) },
          { combatantId: 'g2', attackRoll: create(DiceRollSchema, { diceCount: 0, diceSides: 0 }) },
        ],
      }),
    );
    expect(unreadable).not.toHaveBeenCalled();
  });
});

describe('CastSheet: the details, the description and closing', () => {
  it('says the spell could not be described when its details are gone', async () => {
    const { sheet } = await open({ details: null });
    expect(sheet.detailsFailed()).toBe(true);
    expect(sheet.missing()).toBe('Não deu para ler a magia. Feche e tente de novo.');
    expect(() => sheet.describe()).not.toThrow();
  });

  it("resumes a magic missile's damage as darts, and a plain one as none", async () => {
    const missile = create(PendingDamageSchema, {
      id: 'p1',
      targetId: 'g1',
      attackKey: 'spell:magic-missile',
      diceCount: 3,
      status: PendingDamageStatus.AWAITING_ROLL,
    });
    const { sheet } = await open({
      data: {
        resume: [
          missile,
          create(PendingDamageSchema, { id: 'p2', targetId: 'g2', attackKey: 'spell:x' }),
        ],
      },
    });
    const resumed = sheet.resumedCast();
    expect(resumed.targets.map((t: { darts: number }) => t.darts)).toEqual([3, 0]);
  });

  it('closes with whether the cast is over and nothing is owed, and not while busy', async () => {
    const { sheet, close } = await open();
    sheet.busy.set(true);
    sheet.close();
    expect(close).not.toHaveBeenCalled();
    sheet.busy.set(false);
    sheet.close();
    expect(close).toHaveBeenCalledWith(false);
  });

  it('knows the ability it is cast for', async () => {
    const { sheet } = await open({
      details: {
        spell: { level: 1 },
        save: { ability: Ability.DEXTERITY },
        damage: [],
        healBySlotLevel: {},
        higherLevel: [],
      },
    });
    expect(sheet.kind()).toBe('save');
  });
});

const POOL_DETAILS = {
  spell: { level: 1 },
  hitPointEffect: {
    kind: SpellHitPointEffectKind.POOL,
    poolDiceCount: 5,
    poolDicePerLevel: 2,
    poolDiceSides: 8,
  },
  damage: [],
  healBySlotLevel: {},
  higherLevel: [],
};

describe('CastSheet: a spell that rolls a pool', () => {
  it('counts the creatures it touched in the subtitle, in the singular too, and tells the roll of the pool', async () => {
    const { sheet } = await open({ details: POOL_DETAILS });
    expect(sheet.kind()).toBe('pool');
    expect(sheet.pool()).toEqual({ count: 5, sides: 8 });
    sheet.cast.set(
      create(SpellCastSchema, {
        targets: [{ combatantId: 'g1' }],
        slot: { level: 1, pact: false },
        poolRoll: create(DiceRollSchema, {
          diceCount: 5,
          diceSides: 8,
          faces: [1, 2, 3, 4, 5],
          total: 15,
        }),
      }),
    );
    expect(plain(sheet.subtitle())).toBe('1º nível · 1 criatura na área');
    expect(sheet.poolText()).not.toBe('');
  });

  it('rolls the pool in the app when nothing was typed', async () => {
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {
        poolRoll: create(DiceRollSchema, {
          diceCount: 5,
          diceSides: 8,
          faces: [1, 2, 3, 4, 5],
          total: 15,
        }),
      }),
    });
    const { sheet } = await open({ details: POOL_DETAILS, client: { castSpell } });
    await sheet.castAnyway();
    expect(castSpell.mock.calls[0][6]).toEqual({ inApp: true });
  });
});

describe('CastSheet: a spell attack with nobody to attack yet', () => {
  it('has no target to take the mode from', async () => {
    const attack = {
      spell: { level: 1 },
      attackType: SpellAttackType.RANGED,
      damage: [],
      healBySlotLevel: {},
      higherLevel: [],
    };
    const { sheet } = await open({ details: attack });
    expect(sheet.kind()).toBe('attack');
    expect(sheet.suggested()).toBe(RollMode.NORMAL);
    expect(sheet.sources()).toEqual([]);
  });
});

describe('CastSheet: Metamagic picks that stay', () => {
  it('keeps the creature with disadvantage while it is still a target', async () => {
    const { sheet } = await open({
      data: { targets: targetsOf({ maxTargets: 2 }) },
      combatants: [salvia(), goblin(1), goblin(2)],
    });
    sheet.toggle('g1');
    sheet.toggle('g2');
    sheet.setPicks({ twinned: '', careful: '', heightened: 'g1' });
    sheet.toggle('g2');
    expect(sheet.picks().heightened).toBe('g1');
  });
});

describe('CastSheet: a feminine group of creatures that goes with the concentration', () => {
  it('writes "As" for creatures with a feminine name', async () => {
    const wolf = (id: string) =>
      combatant({
        id,
        label: `Loba atroz ${id}`,
        kind: CombatantKind.CREATURE,
        characterId: '',
        ownerCharacterId: 'sc',
        summonGroupId: 'cast',
        monsterKey: 'monster:dire-wolf',
        monsterNamePt: 'Loba atroz',
      });
    const { sheet } = await open({
      data: { concentration: true },
      combatants: [
        salvia({ concentrationSpell: 'spell:web', concentrationSpellNamePt: 'Teia' }),
        wolf('w1'),
        wolf('w2'),
      ],
    });
    expect(sheet.endsConcentration().goes).toMatch(/^As 2 /);
  });
});

describe('CastSheet: a placed area', () => {
  const circle = create(SpellTargetsSchema, {
    spellKey: 'spell:x',
    placement: AreaPlacement.POINT,
    areaShape: SpellAreaShape.SPHERE,
    areaSizeFt: 20,
    rangeFt: 60,
    targets: [reach('g1')],
  });
  const map = {
    image: { url: 'cave.png', width: 1200, height: 800 },
    mapName: 'A caverna',
    columns: 12,
    rows: 8,
    layers: null,
    fog: null,
  };
  const withSave = {
    spell: { level: 3 },
    save: { ability: Ability.DEXTERITY },
    damage: [],
    healBySlotLevel: {},
    higherLevel: [],
  };
  const area = (over: Record<string, unknown> = {}) =>
    open({
      data: { targets: circle, map, level: 3, slots: [{ level: 3, free: 1, pact: false }] },
      combatants: [salvia(), goblin(1)],
      details: withSave,
      ...over,
    });

  it('says what the area hit once cast: the circle, the creatures, the save and its DC', async () => {
    const { sheet } = await area();
    sheet.cast.set(
      create(SpellCastSchema, {
        slot: { level: 3, pact: false },
        targets: [{ combatantId: 'g1', save: { dc: 15 } }, { combatantId: 'g2' }],
      }),
    );
    expect(plain(sheet.subtitle())).toBe(
      '3º nível · 2 criaturas · teste de resistência de Destreza (CD 15)',
    );
  });

  it('says it without a DC when no target carries one, and for one creature in the singular', async () => {
    const { sheet } = await area();
    sheet.cast.set(
      create(SpellCastSchema, {
        slot: { level: 3, pact: false },
        targets: [{ combatantId: 'g1' }],
      }),
    );
    expect(plain(sheet.subtitle())).toBe(
      '3º nível · 1 criatura · teste de resistência de Destreza',
    );
  });

  it('says it without a save for a spell that asks none', async () => {
    const { sheet } = await area({
      details: { spell: { level: 3 }, damage: [], healBySlotLevel: {}, higherLevel: [] },
    });
    sheet.cast.set(create(SpellCastSchema, { slot: { level: 3, pact: false }, targets: [] }));
    expect(plain(sheet.subtitle())).toBe('3º nível · 0 criaturas');
  });

  it('lists nobody for a cantrip with nobody inside, and spares the creatures of the placed area', async () => {
    const { sheet } = await open({
      data: { targets: circle, map, level: 0, slots: [], usage: [], sculptSpells: true },
      combatants: [salvia(), goblin(1)],
    });
    expect(sheet.sculptRows()).toEqual([]);
    expect(sheet.nobodyLine()).toBeDefined();
    sheet.flow.preview.set(
      create(PreviewSpellAreaResponseSchema, {
        coverCounts: false,
        targets: [
          { combatantId: 'g1', label: 'Goblin 1', ally: false },
          { combatantId: 's', label: 'Sálvia', self: true },
        ],
      }),
    );
    expect(sheet.sculptRows().map((r: { id: string }) => r.id)).toEqual(['g1']);
    expect(sheet.coverNote()).toBe('');
  });

  it('explains the cover only when the preview counts it', async () => {
    const { sheet } = await area();
    sheet.flow.preview.set(
      create(PreviewSpellAreaResponseSchema, {
        coverCounts: true,
        targets: [{ combatantId: 'g1', label: 'Goblin 1' }],
      }),
    );
    expect(sheet.coverNote()).toContain('A cobertura é medida do ponto de origem');
  });

  it('goes on to the roll, not the cast, for a spell attack with nobody in the area', async () => {
    const castSpell = vi.fn();
    const { sheet } = await area({
      details: {
        spell: { level: 3 },
        attackType: SpellAttackType.RANGED,
        damage: [],
        healBySlotLevel: {},
        higherLevel: [],
      },
      client: { castSpell },
    });
    await sheet.castAnyway();
    expect(sheet.flow.nobodyAccepted()).toBe(true);
    expect(castSpell).not.toHaveBeenCalled();
  });
});

describe('CastSheet: darts cast', () => {
  it('names the darts placed on each target in the cast', async () => {
    const darts = targetsOf({
      spellKey: 'spell:magic-missile',
      darts: [{ slotLevel: 1, darts: 3 }],
      targets: [reach('g1')],
    });
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {}),
    });
    const { sheet, settle } = await open({
      data: { spellKey: 'spell:magic-missile', targets: darts },
      combatants: [salvia(), goblin(1)],
      client: { castSpell },
    });
    await settle();
    await sheet.castAnyway();
    expect(castSpell.mock.calls[0][5]).toEqual([{ combatantId: 'g1', darts: 3 }]);
  });
});

describe('CastSheet: the die Lucky rolled again', () => {
  it("tumbles the new face alone when the die's question comes next", async () => {
    const lucky = (face: number) =>
      create(DiceRollSchema, {
        diceCount: 1,
        diceSides: 20,
        faces: [face],
        modifier: 5,
        total: face + 5,
        rerolled: [{ index: 0, from: 1, to: face, sourceKey: 'trait:lucky' }],
      });
    const offer = create(OutsideInspirationOfferSchema, { holdId: 'h', d20: lucky(14) });
    const answerOutsideLucky = vi.fn().mockResolvedValue({
      result: {
        case: 'spellCast',
        value: { encounter: encounter({ combatants: [salvia()] }), inspirationOffer: offer },
      },
    });
    const { sheet } = await open({ resources: { answerOutsideLucky } });
    const play = vi.spyOn(sheet.animator, 'play');
    sheet.luckyOffer.set(create(LuckyOfferSchema, { holdId: 'h', d20: lucky(1), luckyIndex: 0 }));
    await sheet.answerLucky(true, { inApp: true });
    expect(sheet.offer()).toBe(offer);
    expect(play).toHaveBeenCalled();
  });

  it('tumbles nothing for a face the player typed', async () => {
    const offer = create(OutsideInspirationOfferSchema, { holdId: 'h' });
    const answerOutsideLucky = vi.fn().mockResolvedValue({
      result: {
        case: 'spellCast',
        value: { encounter: encounter({ combatants: [salvia()] }), inspirationOffer: offer },
      },
    });
    const { sheet } = await open({ resources: { answerOutsideLucky } });
    const play = vi.spyOn(sheet.animator, 'play');
    sheet.luckyOffer.set(create(LuckyOfferSchema, { holdId: 'h', luckyIndex: 0 }));
    await sheet.answerLucky(true, { face: 14 });
    expect(sheet.offer()).toBe(offer);
    expect(play).not.toHaveBeenCalled();
  });
});

describe('CastSheet: the told attack and the closed cast', () => {
  it('animates a single attack that is not told yet without its word', async () => {
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {
        targets: [
          {
            combatantId: 'g1',
            outcome: AttackOutcome.UNSPECIFIED,
            attackRoll: create(DiceRollSchema, {
              diceCount: 1,
              diceSides: 20,
              faces: [9],
              modifier: 5,
              total: 14,
            }),
          },
        ],
      }),
    });
    const { sheet } = await open({ client: { castSpell } });
    const play = vi.spyOn(sheet.animator, 'play');
    await sheet.castAnyway();
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('animates the pool and nothing else when the cast rolled one', async () => {
    const castSpell = vi.fn().mockResolvedValue({
      encounter: encounter({ combatants: [salvia()] }),
      cast: create(SpellCastSchema, {
        poolRoll: create(DiceRollSchema, { diceCount: 2, diceSides: 8, faces: [3, 4], total: 7 }),
        targets: [{ combatantId: 'g1' }],
      }),
    });
    const { sheet } = await open({ details: POOL_DETAILS, client: { castSpell } });
    const play = vi.spyOn(sheet.animator, 'play');
    await sheet.castAnyway();
    expect(play).toHaveBeenCalledTimes(1);
  });

  it('closes a finished cast with nothing owed as done', async () => {
    const { sheet, close } = await open();
    sheet.cast.set(create(SpellCastSchema, {}));
    sheet.close();
    expect(close).toHaveBeenCalledWith(true);
  });
});

describe('CastSheet: the description of the spell', () => {
  it('loads the description for the dialog, and fails when the details are gone', async () => {
    const dialog = { open: vi.fn() };
    TestBed.resetTestingModule();
    const first = await openWithDialog(dialog, {
      spell: { level: 1, key: 'spell:x' },
      damage: [],
      healBySlotLevel: {},
      higherLevel: [],
    });
    first.describe();
    const data = dialog.open.mock.calls[0][1].data;
    expect(data.namePt).toBe('Feitiço');
    await expect(data.load()).resolves.toBeDefined();

    const missing = await openWithDialog(dialog, null);
    missing.describe();
    await expect(dialog.open.mock.calls[1][1].data.load()).rejects.toThrow(
      'spell details unavailable',
    );
  });
});

async function openWithDialog(
  dialog: { open: ReturnType<typeof vi.fn> },
  details: unknown,
): Promise<Sheet> {
  TestBed.resetTestingModule();
  const data = {
    campaignId: 'c',
    encounterId: 'enc',
    casterId: 's',
    round: 2,
    spellKey: 'spell:x',
    name: 'Feitiço',
    level: 1,
    concentration: false,
    cantripDice: '',
    economy: ActionEconomy.ACTION,
    slots: [slot1],
    usage: [],
    pact: null,
    targets: undefined,
    shieldFree: null,
    shieldName: '',
    attackBonus: 5,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state: { encounter: signal(encounter({ combatants: [salvia()] })), apply: vi.fn() },
  } as unknown as CastSheetData;
  TestBed.configureTestingModule({
    providers: [
      { provide: CombatClient, useValue: {} },
      { provide: SpellCatalog, useValue: { details: () => Promise.resolve(details) } },
      { provide: MatDialog, useValue: dialog },
      { provide: MAT_DIALOG_DATA, useValue: data },
      { provide: MatDialogRef, useValue: { close: vi.fn() } },
    ],
  });
  const fixture = TestBed.createComponent(CastSheet);
  fixture.detectChanges();
  await fixture.whenStable();
  return seen(fixture.componentInstance);
}
