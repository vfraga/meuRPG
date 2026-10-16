import { create } from '@bufbuild/protobuf';

import { DiceMode, DicePreference } from '../../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import {
  AreaPlacement,
  type GetTurnOptionsResponse,
  type PendingDamage,
} from '../../../../../gen/meurpg/play/v1/combat_pb';
import {
  ActionEconomy,
  AttackKind,
  AttackSchema,
  TurnOptionsSchema,
} from '../../../../../gen/meurpg/rules/v1/rules_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { combatant, encounter } from '../../../../core/combat/combat-testing';
import type { VitalsVm } from '../../../../core/play/live-session.types';
import {
  type CastSheetRequest,
  type SpellTurnOptions,
  areaSpellOf,
  castAriaLabel,
  castSheetData,
  masterAreaCastData,
  masterAreaFocus,
  opensOnMap,
  spellAttackBonus,
} from './cast-sheet-data';

const caster = combatant({ id: 'z', label: 'Zuk' });
const enc = encounter({ round: 4, combatants: [caster] });
const spellOption = (key: string, over: object = {}) => ({
  spell: { key, name: key, namePt: `${key} PT`, level: 1, concentration: true },
  economy: ActionEconomy.BONUS_ACTION,
  slots: [{ level: 1, pact: false, free: 2 }],
  metamagicOptions: [{ key: 'quickened' }],
  sculptSpells: true,
  ...over,
});

function turn(spells: object[] = [], attacks: object[] = [], spellTargets: object[] = []) {
  return {
    options: create(TurnOptionsSchema, { spells, attacks } as never),
    attackTargets: [],
    spellTargets,
    pendingDamages: [],
  } as unknown as SpellTurnOptions;
}

function request(over: Partial<CastSheetRequest> = {}): CastSheetRequest {
  return {
    key: 'spell:fire-bolt',
    campaignId: 'c',
    encounter: enc,
    caster,
    turn: turn(),
    vitals: undefined,
    master: false,
    diceMode: DiceMode.PLAYERS_CHOOSE,
    preference: DicePreference.APP,
    state: new CombatState(),
    map: null,
    ...over,
  };
}

describe('castSheetData', () => {
  it('hands the sheet a spell of the list: its name, level, concentration, economy, slots and Metamagic', () => {
    const data = castSheetData(
      request({ key: 'spell:haste', turn: turn([spellOption('spell:haste')]) }),
    );
    expect(data).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      casterId: 'z',
      round: 4,
      spellKey: 'spell:haste',
      name: 'spell:haste PT',
      level: 1,
      concentration: true,
      cantripDice: '',
      economy: ActionEconomy.BONUS_ACTION,
      sculptSpells: true,
      master: false,
      usage: [],
      pact: null,
      sorceryPoints: null,
      attackBonus: 0,
    });
    expect(data.slots).toHaveLength(1);
    expect(data.metamagic).toHaveLength(1);
  });

  it("falls back to the spell's English name, then to a cantrip's name and dice, then to the key", () => {
    const noPt = spellOption('spell:haste', {
      spell: { key: 'spell:haste', name: 'Haste', namePt: '' },
    });
    expect(castSheetData(request({ key: 'spell:haste', turn: turn([noPt]) })).name).toBe('Haste');
    const cantrip = {
      attack: create(AttackSchema, {
        key: 'spell:fire-bolt',
        name: 'Fire Bolt',
        namePt: 'Raio de Fogo',
        spellDice: '2d10',
      }),
    };
    const fire = castSheetData(request({ turn: turn([], [cantrip]) }));
    expect(fire).toMatchObject({
      name: 'Raio de Fogo',
      cantripDice: '2d10',
      level: 0,
      concentration: false,
      economy: ActionEconomy.ACTION,
      slots: [],
      metamagic: [],
      sculptSpells: false,
    });
    const english = { attack: create(AttackSchema, { key: 'spell:fire-bolt', name: 'Fire Bolt' }) };
    expect(castSheetData(request({ turn: turn([], [english]) })).name).toBe('Fire Bolt');
    expect(castSheetData(request({ key: 'spell:unknown' })).name).toBe('spell:unknown');
  });

  it("reads the caster's slots, pact slots and sorcery points from the vitals", () => {
    const vitals = {
      spellSlots: [{ level: 1, total: 4, used: 3 }],
      pactSlots: { slotLevel: 2, total: 2, used: 0 },
      resources: [{ key: 'sorcery_points', total: 5, used: 2 }],
    } as unknown as VitalsVm;
    const data = castSheetData(request({ vitals }));
    expect(data.usage).toEqual([{ level: 1, total: 4, used: 3 }]);
    expect(data.pact).toEqual({ slotLevel: 2, total: 2, used: 0 });
    expect(data.sorceryPoints).toEqual({ left: 3, total: 5 });
  });

  it('counts the free slots of Escudo, with its name; nothing without it', () => {
    const shield = spellOption('spell:shield', {
      slots: [
        { level: 1, pact: false, free: 2 },
        { level: 2, pact: false, free: 1 },
      ],
    });
    expect(castSheetData(request({ turn: turn([shield]) }))).toMatchObject({
      shieldFree: 3,
      shieldName: 'spell:shield PT',
    });
    const unnamed = spellOption('spell:shield', { spell: { key: 'spell:shield', namePt: '' } });
    expect(castSheetData(request({ turn: turn([unnamed]) })).shieldName).toBe('Escudo Arcano');
    expect(castSheetData(request()).shieldFree).toBeNull();
    expect(castSheetData(request()).shieldName).toBe('Escudo Arcano');
  });

  it('takes the targets of the spell, the damage to resume and the map as they come', () => {
    const targets = { spellKey: 'spell:fire-bolt', targets: [] };
    const resume = [{ id: 'p' }] as unknown as PendingDamage[];
    const map = { mapName: 'Cripta' } as never;
    const data = castSheetData(
      request({ turn: turn([], [], [targets, { spellKey: 'other' }]), resume, map, master: true }),
    );
    expect(data.targets).toBe(targets);
    expect(data.resume).toBe(resume);
    expect(data.map).toBe(map);
    expect(data.master).toBe(true);
    expect(castSheetData(request()).targets).toBeUndefined();
  });

  it('takes the typed d20 bonus from the spell attacks of the options', () => {
    const spellAttack = {
      attack: create(AttackSchema, { key: 'x', kind: AttackKind.SPELL, attackBonus: 7 }),
    };
    expect(castSheetData(request({ turn: turn([], [spellAttack]) })).attackBonus).toBe(7);
  });
});

describe('spellAttackBonus', () => {
  const attack = (over: object) => create(AttackSchema, over as never);

  it("is the spell attack's bonus, a save cantrip's DC minus 8, or 0", () => {
    expect(
      spellAttackBonus([
        attack({ kind: AttackKind.SPELL, saveDc: 13 }),
        attack({ kind: AttackKind.SPELL, saveDc: 0, attackBonus: 5 }),
      ]),
    ).toBe(5);
    expect(spellAttackBonus([attack({ kind: AttackKind.SPELL, saveDc: 14 })])).toBe(6);
    expect(spellAttackBonus([attack({ kind: AttackKind.WEAPON, saveDc: 0, attackBonus: 9 })])).toBe(
      0,
    );
    expect(spellAttackBonus([])).toBe(0);
  });
});

describe('opensOnMap', () => {
  const data = (targetPlacement: AreaPlacement | null, over: object = {}) =>
    ({
      map: {},
      targets: targetPlacement === null ? undefined : { placement: targetPlacement },
      ...over,
    }) as never;

  it('is true for a placed spell with a map, on its first cast', () => {
    expect(opensOnMap(data(AreaPlacement.POINT))).toBe(true);
  });

  it('is false when resuming a damage, with no map, with no targets, unplaced or placed on the caster', () => {
    expect(opensOnMap(data(AreaPlacement.POINT, { resume: [{}] }))).toBe(false);
    expect(opensOnMap(data(AreaPlacement.POINT, { map: null }))).toBe(false);
    expect(opensOnMap(data(null))).toBe(false);
    expect(opensOnMap(data(AreaPlacement.UNSPECIFIED))).toBe(false);
    expect(opensOnMap(data(AreaPlacement.CASTER))).toBe(false);
  });
});

describe('castAriaLabel', () => {
  it('says the cast, or the damage still to roll', () => {
    expect(castAriaLabel('Bola de Fogo', false)).toBe('Conjurar Bola de Fogo');
    expect(castAriaLabel('Bola de Fogo', true)).toBe('Rolar o dano de Bola de Fogo');
  });
});

describe('the master casting an NPC area spell', () => {
  const targets = { spellKey: 'spell:fireball', placement: AreaPlacement.POINT };
  const options = (level: number, slots: object[]) =>
    ({
      options: create(TurnOptionsSchema, {
        spells: [
          spellOption('spell:fireball', {
            spell: { key: 'spell:fireball', name: 'Fireball', namePt: 'Bola de Fogo', level },
            slots,
          }),
        ],
      } as never),
      attackTargets: [],
      spellTargets: [targets],
      pendingDamages: [],
    }) as unknown as GetTurnOptionsResponse;

  it('finds a spell with its targets, level and name; null when either is missing', () => {
    const found = areaSpellOf(options(3, []), 'spell:fireball');
    expect(found).toMatchObject({ level: 3, name: 'Bola de Fogo', targets });
    expect(areaSpellOf(options(3, []), 'spell:other')).toBeNull();
    expect(areaSpellOf(null, 'spell:fireball')).toBeNull();
    const noTargets = { ...options(3, []), spellTargets: [] } as unknown as GetTurnOptionsResponse;
    expect(areaSpellOf(noTargets, 'spell:fireball')).toBeNull();
    const english = options(3, []);
    english.options!.spells[0]!.spell!.namePt = '';
    expect(areaSpellOf(english, 'spell:fireball')?.name).toBe('Fireball');
  });

  const dataFor = (level: number, slots: object[]) => {
    const found = areaSpellOf(options(level, slots), 'spell:fireball')!;
    return masterAreaCastData({
      campaignId: 'c',
      encounter: enc,
      caster,
      key: 'spell:fireball',
      found,
      map: { mapName: 'Cripta' } as never,
      state: new CombatState(),
    });
  };

  it("casts with the lowest slot the options offer, or the spell's own level", () => {
    expect(
      dataFor(3, [
        { level: 4, pact: false, free: 1 },
        { level: 3, pact: false, free: 1 },
      ]).slot,
    ).toEqual({
      level: 3,
      pact: false,
    });
    expect(dataFor(3, []).slot).toEqual({ level: 3, pact: false });
  });

  it('has no slot for a cantrip, and carries the caster, the name and the economy', () => {
    const data = dataFor(0, []);
    expect(data.slot).toBeNull();
    expect(data).toMatchObject({
      encounterId: 'enc',
      caster,
      spellKey: 'spell:fireball',
      name: 'Bola de Fogo',
      level: 0,
      economy: ActionEconomy.BONUS_ACTION,
      targets,
    });
  });

  it('focuses the map unless the spell starts on the caster', () => {
    expect(masterAreaFocus({ placement: AreaPlacement.POINT } as never)).toEqual({
      focus: '[role="application"]',
    });
    expect(masterAreaFocus({ placement: AreaPlacement.CASTER } as never)).toEqual({});
  });
});
