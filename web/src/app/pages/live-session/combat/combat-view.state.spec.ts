import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';

import { DeathSaveVisibility } from '../../../../gen/meurpg/campaigns/v1/campaigns_pb';
import { CharacterKind, CreatureSource } from '../../../../gen/meurpg/characters/v1/characters_pb';
import {
  CombatLogKind,
  CombatLogRoundSchema,
  CombatantKind,
  CombatantSide,
  CombatantState,
  CoverDegree,
  type Encounter,
  EncounterMode,
  EncounterStatus,
  type GetTurnOptionsResponse,
  type OpportunityOffer,
  PendingDamageStatus,
  ReactionKind,
  ReactionWindowStatus,
  WildShapeEndReason,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import {
  CreatureSchema,
  DisabledReasonCode,
  SpellDetailsSchema,
  TurnOptionsSchema,
} from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatErrorMessage } from '../../../core/combat/combat-errors';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { autoPassKey } from '../../../core/combat/reaction-autopass';
import { SpellCatalog } from '../../../core/combat/spell-catalog';
import { MapsClient } from '../../../core/maps/maps-client';
import { RosterClient } from '../../../core/maps/roster-client';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import { settle } from '../../../../test-settle';
import { mountCombatView } from './combat-view-testing';

// The derived state of the combat view and its constructor effects: what each computed answers for the combat it is
// given, which calls the effects make and what they set.

const toren = combatant({
  id: 't',
  label: 'Toren',
  kind: CombatantKind.PLAYER,
  side: CombatantSide.PARTY,
  characterId: 'toren-c',
  mine: true,
  placed: false,
});
/** The master's copy of the same character: for him no combatant is `mine`. */
const seen = { ...toren, mine: false } as typeof toren;
const goblin = combatant({ id: 'g', label: 'Goblin', placed: false });
const wolf = combatant({
  id: 'w',
  label: 'Lobo',
  kind: CombatantKind.CREATURE,
  side: CombatantSide.PARTY,
  controlledByMe: true,
  monsterKey: 'monster:wolf',
  summonGroupId: 'g1',
  placed: false,
});

/** A round of the combat log as the wire has it (every field defaulted: the log panel draws it too). */
const roundOf = (round: number, entries: readonly unknown[]) =>
  create(CombatLogRoundSchema, { round, entries } as never);

/** The same combat one revision later, with `over` changed. */
const next = (enc: Encounter, over: Partial<Encounter> = {}): Encounter =>
  ({ ...enc, ...over, revision: enc.revision + 1 }) as Encounter;

const player = (over: Partial<Encounter> = {}) =>
  encounter({ currentCombatantId: 't', combatants: [toren, goblin], ...over });

describe('CombatView state: who is who', () => {
  it('merges the party info of a player into the info map, with no kind and no race', () => {
    const { view } = mountCombatView({
      enc: player(),
      inputs: {
        partyInfo: new Map([['toren-c', { classSummary: 'Clérigo 5', playerName: 'Caio' }]]),
      },
    });
    expect(view.info().get('toren-c')).toEqual({
      classSummary: 'Clérigo 5',
      playerName: 'Caio',
      kindLabel: '',
      raceName: '',
    });
  });

  it('lets the roster entry win over the party info of the same character', async () => {
    const { view, fixture } = mountCombatView({
      enc: player(),
      master: true,
      inputs: { partyInfo: new Map([['toren-c', { classSummary: 'Da festa', playerName: 'P' }]]) },
    });
    TestBed.inject(RosterClient).list = async () =>
      [
        {
          id: 'toren-c',
          classSummary: 'Guerreiro 4',
          playerName: 'Caio',
          kind: CharacterKind.PLAYER,
          raceName: 'Humano',
        },
        {
          id: 'char-g',
          classSummary: '',
          playerName: '',
          kind: CharacterKind.ENEMY,
          raceName: '',
        },
      ] as never;
    await view.reads.loadRoster('c');
    await settle(fixture);
    expect(view.info().get('toren-c')).toEqual({
      classSummary: 'Guerreiro 4',
      playerName: 'Caio',
      kindLabel: '',
      raceName: 'Humano',
    });
    // An NPC says its kind in words.
    expect(view.info().get('char-g').kindLabel).toBe('Inimigo');
  });

  it('keeps the roster it had when the read fails', async () => {
    const { view, fixture } = mountCombatView({ enc: player(), master: true });
    // The read the master's screen makes by itself must be over before the roster is planted.
    await settle(fixture);
    view.rosterInfo.set(
      new Map([['x', { classSummary: 'a', playerName: 'b', kindLabel: '', raceName: '' }]]),
    );
    TestBed.inject(RosterClient).list = async () => {
      throw new ConnectError('down', Code.Unavailable);
    };
    await view.reads.loadRoster('c');
    expect([...view.rosterInfo().keys()]).toEqual(['x']);
  });

  it('reads the roster for the master, again when the campaign changes, and never for a player', async () => {
    const master = mountCombatView({ enc: player(), master: true });
    const masterList = vi.spyOn(TestBed.inject(RosterClient), 'list');
    master.fixture.componentRef.setInput('campaignId', 'c2');
    await settle(master.fixture);
    expect(masterList).toHaveBeenCalledWith('c2');

    const guest = mountCombatView({ enc: player() });
    const guestList = vi.spyOn(TestBed.inject(RosterClient), 'list');
    guest.fixture.componentRef.setInput('campaignId', 'c2');
    await settle(guest.fixture);
    expect(guestList).not.toHaveBeenCalled();
  });

  it('knows the vitals row of the own character, and none for the master or without a row', () => {
    const row = { characterId: 'toren-c', resources: [] };
    expect(mountCombatView({ enc: player(), inputs: { vitals: [row] } }).view.ownVitals()).toBe(
      row,
    );
    expect(mountCombatView({ enc: player() }).view.ownVitals()).toBeNull();
    expect(
      mountCombatView({
        enc: player({ combatants: [seen, goblin] }),
        master: true,
        inputs: { vitals: [row] },
      }).view.ownVitals(),
    ).toBeNull();
    expect(
      mountCombatView({ enc: player(), inputs: { vitals: [row] } })
        .view.adjustable()
        .has('toren-c'),
    ).toBe(true);
  });

  it('answers the own slots from the vitals, and empty ones without them', () => {
    const slots = [{ level: 1, total: 2, used: 1 }];
    const pact = { level: 1, total: 1, used: 0 };
    const withRow = mountCombatView({
      enc: player(),
      inputs: { vitals: [{ characterId: 'toren-c', spellSlots: slots, pactSlots: pact }] },
    });
    expect(withRow.view.ownSlots()).toEqual({ usage: slots, pact });
    expect(mountCombatView({ enc: player() }).view.ownSlots()).toEqual({ usage: [], pact: null });
    const bare = mountCombatView({
      enc: player(),
      inputs: { vitals: [{ characterId: 'toren-c' }] },
    });
    expect(bare.view.ownSlots()).toEqual({ usage: [], pact: null });
  });
});

describe('CombatView state: the tabs of what the player plays', () => {
  // The creature's page reads the book's armor class of its kind.
  const book = { statBlock: async () => create(CreatureSchema, {}) };
  const party = (over: Partial<Encounter> = {}) =>
    encounter({ currentCombatantId: 't', combatants: [toren, wolf, goblin], ...over });

  it('has one tab per group of what the player plays, and none for the master or a combat being set up', () => {
    const { view } = mountCombatView({ enc: party() });
    expect(view.tabs().map((t: { id: string; state: string }) => [t.id, t.state])).toEqual([
      ['character', 'turn'],
      ['g1', 'wait'],
    ]);
    expect(view.showTabs()).toBe(true);
    expect(mountCombatView({ enc: party(), master: true }).view.tabs()).toEqual([]);
    const setup = mountCombatView({ enc: party({ status: EncounterStatus.SETUP }) });
    expect(setup.view.tabs()).toEqual([]);
    // One combatant is not worth a tab bar.
    expect(mountCombatView({ enc: player() }).view.showTabs()).toBe(false);
  });

  it('shows the character tab, then the one a tap chose, and falls back to the first when it is gone', async () => {
    const { view, fixture } = mountCombatView({ enc: party(), creatures: book });
    expect(view.tab().id).toBe('character');
    expect(view.creatureTab()).toBeNull();
    view.tabId.set('g1');
    expect(view.tab().id).toBe('g1');
    expect(view.creatureTab().id).toBe('g1');
    view.tabId.set('nenhuma');
    expect(view.tab().id).toBe('character');
    await settle(fixture);
    // The effect puts the page back on the character when the chosen group is not in the tabs.
    expect(view.tabId()).toBe('character');
  });

  it('has no tab when the player plays nothing', () => {
    const { view } = mountCombatView({ enc: encounter({ combatants: [goblin] }) });
    expect(view.tab()).toBeNull();
    expect(view.creatureTab()).toBeNull();
  });

  it('follows the turn to the creatures and back, and goes back to the character when the creatures leave', async () => {
    const enc = party();
    const { view, fixture, state } = mountCombatView({ enc, creatures: book });
    expect(view.tabId()).toBe('character');
    state.apply(next(enc, { currentCombatantId: 'w' }));
    await settle(fixture);
    expect(view.tabId()).toBe('g1');
    state.apply(next(enc, { currentCombatantId: 'w', revision: 3, combatants: [toren, goblin] }));
    await settle(fixture);
    expect(view.tabId()).toBe('character');
  });

  it('puts the order strip under the actions on a phone, on the own turn, off the creature page and standing', () => {
    const phone = mountCombatView({ enc: party(), matches: [] });
    expect(phone.view.orderBelow()).toBe(true);
    const laptop = mountCombatView({ enc: party(), matches: ['(min-width: 1024px)'] });
    expect(laptop.view.orderBelow()).toBe(false);
    const offTurn = mountCombatView({ enc: party({ currentCombatantId: 'g' }) });
    expect(offTurn.view.orderBelow()).toBe(false);
    const down = mountCombatView({
      enc: party({
        combatants: [{ ...toren, state: CombatantState.DOWN } as never, wolf, goblin],
      }),
    });
    expect(down.view.orderBelow()).toBe(false);
    const onCreature = mountCombatView({ enc: party() });
    onCreature.view.tabId.set('g1');
    expect(onCreature.view.orderBelow()).toBe(false);
  });

  it('says who else ends their part in a joint turn, only on the own turn', () => {
    const joint = (over: Partial<Encounter>) =>
      mountCombatView({ enc: player({ turnGroupIds: ['t', 'g'], ...over }) }).view.jointOthers();
    expect(joint({ masterTurn: true })).toEqual(['Goblin', 'o mestre']);
    expect(joint({ masterTurn: false })).toEqual(['Goblin']);
    // Not a joint turn: a turn of one.
    expect(mountCombatView({ enc: player() }).view.jointOthers()).toBeNull();
    // A joint turn that is not the player's.
    const other = combatant({ id: 'g2', label: 'Goblin 2', placed: false });
    const offTurn = mountCombatView({
      enc: player({
        currentCombatantId: 'g',
        turnGroupIds: ['g', 'g2'],
        combatants: [toren, goblin, other],
      }),
    });
    expect(offTurn.view.jointOthers()).toBeNull();
  });
});

describe('CombatView state: the combat as the master reads it', () => {
  const master = (over: Partial<Encounter> = {}, extra: object = {}) =>
    mountCombatView({
      enc: encounter({
        currentCombatantId: 'g',
        combatants: [toren, goblin],
        ...over,
      }),
      master: true,
      ...extra,
    });

  it('whose options the screen asks for: the master gets the one on turn, a player the own character', () => {
    expect(master().view.subject().id).toBe('g');
    expect(master().view.masterCard().id).toBe('g');
    expect(master().view.turnLabel()).toBe('Goblin');
    expect(master().view.turnSide()).toBe(CombatantSide.ENEMY);
    const guest = mountCombatView({ enc: player({ currentCombatantId: 'g' }) });
    expect(guest.view.subject().id).toBe('t');
    expect(guest.view.masterCard()).toBeNull();
    expect(guest.view.turnLabel()).toBe('');
    const setup = master({ status: EncounterStatus.SETUP });
    expect(setup.view.subject()).toBeNull();
    expect(setup.view.turnSide()).toBe(CombatantSide.UNSPECIFIED);
  });

  it('gives the master the NPC that still acts in a joint turn that holds a player', () => {
    const { view } = master({ currentCombatantId: 't', turnGroupIds: ['t', 'g'] });
    expect(view.subject().id).toBe('g');
  });

  it('lists who the mover can have left the reach of, with the class line, and says when nobody can react', () => {
    const rows = master(
      {},
      {
        inputs: {
          partyInfo: new Map([['toren-c', { classSummary: 'Clérigo 5', playerName: 'Caio' }]]),
        },
      },
    );
    expect(rows.view.offerRows()).toEqual([
      {
        id: 't',
        label: 'Toren',
        sub: 'Clérigo 5',
        player: true,
        spent: false,
        offered: false,
        hidden: false,
      },
    ]);
    expect(rows.view.offerWhy()).toBe('');
    const spent = master({ combatants: [{ ...toren, reactionUsed: true } as never, goblin] });
    expect(spent.view.offerWhy()).toBe('Ninguém pode reagir agora.');
    const alone = master({ combatants: [goblin] });
    expect(alone.view.offerRows()).toEqual([]);
    expect(alone.view.offerWhy()).toBe('Ninguém pode reagir agora.');
    // The NPC kind label is the fallback line when the roster says no class.
    const npcs = master({ currentCombatantId: 't', combatants: [toren, goblin] });
    npcs.view.rosterInfo.set(new Map([['char-g', { kindLabel: 'Inimigo' } as never]]));
    expect(npcs.view.offerRows()[0].sub).toBe('Inimigo');
  });

  it('lists the characters at three failures for the master only', () => {
    const dying = { ...toren, state: CombatantState.DYING } as never;
    expect(
      master({ combatants: [dying, goblin] })
        .view.dying()
        .map((c: { id: string }) => c.id),
    ).toEqual(['t']);
    expect(mountCombatView({ enc: player({ combatants: [dying, goblin] }) }).view.dying()).toEqual(
      [],
    );
  });

  it('lists the downed players whose death save is due, for the master only', () => {
    const due = { ...toren, state: CombatantState.DOWN, deathSaveDue: true } as never;
    const notDue = { ...toren, id: 't2', state: CombatantState.DOWN, deathSaveDue: false } as never;
    const npcDue = { ...goblin, state: CombatantState.DOWN, deathSaveDue: true } as never;
    expect(
      master({ combatants: [due, notDue, npcDue] })
        .view.deathDue()
        .map((c: { id: string }) => c.id),
    ).toEqual(['t']);
    expect(mountCombatView({ enc: player({ combatants: [due] }) }).view.deathDue()).toEqual([]);
  });

  it('lists the unplaced combatants of a map combat, and none in a combat without a map', () => {
    const placed = combatant({ id: 'p', label: 'Colocado', placed: true });
    const map = master({ combatants: [toren, goblin, placed] });
    expect(map.view.unplaced().map((c: { id: string }) => c.id)).toEqual(['t', 'g']);
    expect(map.view.theatre()).toBe(false);
    const theatre = master({ mode: EncounterMode.THEATRE });
    expect(theatre.view.unplaced()).toEqual([]);
    expect(theatre.view.theatre()).toBe(true);
  });

  it('says what the master owes a hit, and nothing to a player', async () => {
    const turnOptions = () => ({
      pendingDamages: [{ status: PendingDamageStatus.ROLLED, amount: 5, attackerId: 'g' }] as never,
    });
    const m = master({}, { turnOptions });
    await settle(m.fixture);
    expect(m.view.owed()).toBe('Falta aplicar 5 de dano');
    const g = mountCombatView({ enc: player(), turnOptions });
    await settle(g.fixture);
    expect(g.view.owed()).toBeNull();
    const none = master();
    await settle(none.fixture);
    expect(none.view.owed()).toBeNull();
  });

  it('shows the master card for an NPC on turn, and for a player only while a damage waits or was just settled', async () => {
    const open = [{ status: PendingDamageStatus.AWAITING_ROLL, attackerId: 't' }];
    const npc = master();
    await settle(npc.fixture);
    expect(npc.view.cardVisible()).toBe(true);

    const idle = master({ currentCombatantId: 't' });
    await settle(idle.fixture);
    expect(idle.view.cardVisible()).toBe(false);

    const waiting = master(
      { currentCombatantId: 't' },
      { turnOptions: () => ({ pendingDamages: open as never }) },
    );
    await settle(waiting.fixture);
    expect(waiting.view.cardVisible()).toBe(true);

    idle.view.settledTurn.set('t');
    expect(idle.view.cardVisible()).toBe(true);
    // A player has no master card.
    const guest = mountCombatView({ enc: player() });
    expect(guest.view.cardVisible()).toBe(false);
  });

  it('reads whether the newest attack of the one on turn was stopped by a Shield, from the log', async () => {
    const rounds = (stopped: boolean, actorId = 'g') => [
      roundOf(2, [
        { id: 'a', kind: CombatLogKind.ATTACK, actorId: 'x', stoppedByReaction: true },
        { id: 'b', kind: CombatLogKind.ATTACK, actorId, stoppedByReaction: stopped },
        // The log lists the newest first: an older stopped attack of the same actor does not count.
        { id: 'c', kind: CombatLogKind.ATTACK, actorId, stoppedByReaction: true },
      ]),
    ];
    let answer = rounds(true);
    const m = master({}, { api: { log: () => ({ rounds: answer, undoableEventId: '' }) } });
    await settle(m.fixture);
    expect(m.view.reactionStopped()).toBe(true);
    answer = rounds(false);
    m.state.touchLog();
    await settle(m.fixture);
    expect(m.view.reactionStopped()).toBe(false);
    answer = rounds(true, 'outro');
    m.state.touchLog();
    await settle(m.fixture);
    // The attacks are someone else's.
    expect(m.view.reactionStopped()).toBe(false);
  });

  it('finds the own hit whose damage is still to roll', async () => {
    const turnOptions = () => ({
      pendingDamages: [
        { id: 'a', status: PendingDamageStatus.ROLLED, attackerId: 't' },
        { id: 'b', status: PendingDamageStatus.AWAITING_ROLL, attackerId: 'g' },
        { id: 'c', status: PendingDamageStatus.AWAITING_ROLL, attackerId: 't' },
      ] as never,
    });
    const mine = mountCombatView({ enc: player(), turnOptions });
    await settle(mine.fixture);
    expect(mine.view.ownPendingRoll().id).toBe('c');
    const none = mountCombatView({ enc: player() });
    await settle(none.fixture);
    expect(none.view.ownPendingRoll()).toBeNull();
  });

  it('maps who has cover against whoever is on turn, first listing first, skipping none and unspecified', async () => {
    const target = (combatantId: string, cover: CoverDegree, coverSource = 0) => ({
      combatantId,
      cover,
      coverSource,
    });
    const { view, fixture } = master(
      {},
      {
        turnOptions: () => ({
          attackTargets: [
            {
              attackKey: 'a',
              targets: [target('t', CoverDegree.HALF, 1), target('x', CoverDegree.NONE)],
            },
            { attackKey: 'b', targets: [target('t', CoverDegree.TOTAL, 2)] },
          ],
          spellTargets: [
            {
              spellKey: 's',
              targets: [
                target('y', CoverDegree.THREE_QUARTERS, 3),
                target('z', CoverDegree.UNSPECIFIED),
              ],
            },
          ],
        }),
      },
    );
    await settle(fixture);
    expect([...view.coverAgainst()]).toEqual([
      ['t', { cover: CoverDegree.HALF, source: 1 }],
      ['y', { cover: CoverDegree.THREE_QUARTERS, source: 3 }],
    ]);
    expect(master().view.coverAgainst().size).toBe(0);
  });

  it('lists the cover targets and everyone standing', () => {
    const dead = { ...goblin, id: 'd', defeated: true } as never;
    const { view } = master({ combatants: [toren, goblin, dead] });
    expect(view.coverTargets().map((c: { id: string }) => c.id)).toEqual(['t']);
    expect(view.everyone().map((c: { id: string }) => c.id)).toEqual(['t', 'g']);
    expect(master({ status: EncounterStatus.ENDED }).view.coverTargets()).toEqual([]);
  });

  it('reads the status of the combat', () => {
    const active = master();
    expect([active.view.setup(), active.view.active(), active.view.ended()]).toEqual([
      false,
      true,
      false,
    ]);
    const ended = master({ status: EncounterStatus.ENDED });
    expect([ended.view.setup(), ended.view.active(), ended.view.ended()]).toEqual([
      false,
      false,
      true,
    ]);
  });
});

describe('CombatView state: the master reach and the moves', () => {
  const placedGoblin = combatant({ id: 'g', label: 'Goblin', placed: true, col: 3, row: 4 });
  const reach = (over: Partial<Encounter> = {}) =>
    mountCombatView({
      enc: encounter({ currentCombatantId: 'g', combatants: [toren, placedGoblin], ...over }),
      master: true,
      api: { moveOptions: () => ({ movementLeftDft: 150, reachable: [{ col: 3, row: 5 }] }) },
    });

  it('draws nothing while the reach is off, and offers the switch for whoever is placed on turn', async () => {
    const { view, fixture, callsOf } = reach();
    await settle(fixture);
    expect(view.masterReach()).toBeNull();
    expect(view.reachSwitch()).toEqual({ name: 'do Goblin', on: false });
    expect(callsOf('moveOptions')).toHaveLength(0);
  });

  it('reads the squares of whoever is on turn when the reach is on, and draws them from the server answer', async () => {
    const { view, fixture, callsOf } = reach();
    view.reachOn.set(true);
    await settle(fixture);
    expect(callsOf('moveOptions').map((c) => c.args.slice(0, 3))).toEqual([['c', 'enc', 'g']]);
    expect(view.masterReach()).toEqual({
      origin: { col: 3, row: 4 },
      leftDft: 150,
      squares: [{ col: 3, row: 5 }],
    });
    expect(view.reachSwitch()).toEqual({ name: 'do Goblin', on: true });
  });

  it('asks for the long jump squares while the jump page is on, and the walk ones otherwise', async () => {
    const { view, fixture, callsOf, state } = reach();
    view.reachOn.set(true);
    view.moveJump.set({ runningStart: true });
    state.moving.set(true);
    await settle(fixture);
    expect(callsOf('moveOptions').at(-1)?.args[3]).toEqual({ runningStart: true });
    state.moving.set(false);
    state.apply(next(state.encounter() as Encounter));
    await settle(fixture);
    expect(callsOf('moveOptions').at(-1)?.args[3]).toBeUndefined();
  });

  it('has no reach for a combatant without a square, nor for a player, and clears it when the subject goes', async () => {
    const unplaced = mountCombatView({
      enc: encounter({ currentCombatantId: 'g', combatants: [toren, goblin] }),
      master: true,
    });
    unplaced.view.reachOn.set(true);
    await settle(unplaced.fixture);
    expect(unplaced.view.masterReach()).toBeNull();
    expect(unplaced.view.reachSwitch()).toBeNull();
    expect(unplaced.callsOf('moveOptions')).toHaveLength(0);

    const guest = mountCombatView({
      enc: encounter({ currentCombatantId: 'g', combatants: [toren, placedGoblin] }),
    });
    guest.view.reachOn.set(true);
    expect(guest.view.masterReach()).toBeNull();
    expect(guest.view.reachSwitch()).toBeNull();

    const { view, fixture, state } = reach();
    view.reachOn.set(true);
    await settle(fixture);
    expect(view.moveOptions.data()).not.toBeNull();
    state.apply(next(state.encounter() as Encounter, { status: EncounterStatus.ENDED }));
    await settle(fixture);
    expect(view.moveOptions.data()).toBeNull();
  });

  it('asks where the player can go only on the "Mover" page of a turn they act on, and clears it when the turn passes', async () => {
    const placedToren = { ...toren, placed: true } as never;
    const enc = encounter({ currentCombatantId: 't', combatants: [placedToren, goblin] });
    const { view, fixture, callsOf, state } = mountCombatView({
      enc,
      api: { moveOptions: () => ({ movementLeftDft: 300, reachable: [] }) },
    });
    await settle(fixture);
    expect(callsOf('moveOptions')).toHaveLength(0);
    state.moving.set(true);
    await settle(fixture);
    expect(callsOf('moveOptions').map((c) => c.args.slice(0, 3))).toEqual([['c', 'enc', 't']]);
    expect(view.moveOptions.data()).not.toBeNull();
    state.apply(next(enc, { currentCombatantId: 'g' }));
    await settle(fixture);
    expect(view.moveOptions.data()).toBeNull();
    // The page closes with the turn.
    expect(state.moving()).toBe(false);
    expect(view.moverId()).toBe('');
  });

  it('tells that a creature left the combat while its "Mover" page was open', async () => {
    const mine = mountCombatView({ enc: player() });
    mine.view.moves.moverName = 'Lobo';
    mine.view.moverId.set('sumiu');
    await settle(mine.fixture);
    expect(mine.view.moveNote()).toBe('O Lobo saiu do combate.');
    expect(mine.view.moverId()).toBe('');

    const fem = mountCombatView({ enc: player() });
    fem.view.moves.moverName = 'Loba';
    fem.view.moverId.set('sumiu');
    await settle(fem.fixture);
    expect(fem.view.moveNote()).toBe('A Loba saiu do combate.');
  });

  it('does not tell it for the character own page, nor without a name', async () => {
    const noName = mountCombatView({ enc: player() });
    noName.view.moverId.set('sumiu');
    await settle(noName.fixture);
    expect(noName.view.moveNote()).toBe('');
    expect(noName.view.moverId()).toBe('');
  });

  it('closes the full-screen map when the combat is not running any more', async () => {
    const enc = player();
    const { view, fixture, state } = mountCombatView({ enc });
    state.mapOpen.set(true);
    await settle(fixture);
    expect(state.mapOpen()).toBe(true);
    state.apply(next(enc, { status: EncounterStatus.ENDED }));
    await settle(fixture);
    expect(state.mapOpen()).toBe(false);
    expect(view.active()).toBe(false);
  });

  it('forgets what a turn said when the turn changes', async () => {
    const enc = player();
    const { view, fixture, state } = mountCombatView({ enc });
    view.actionNote.set('Surto de Ação');
    view.deathResult.set('x');
    view.masterDeathResult.set('y');
    view.passAsking.set({ discard: false });
    view.moveNote.set('z');
    await settle(fixture);
    // The same turn read again keeps them.
    state.apply(next(enc));
    await settle(fixture);
    expect(view.actionNote()).toBe('Surto de Ação');
    state.apply(next(enc, { revision: 3, currentCombatantId: 'g' }));
    await settle(fixture);
    expect([
      view.actionNote(),
      view.deathResult(),
      view.masterDeathResult(),
      view.moveNote(),
    ]).toEqual(['', '', '', '']);
    expect(view.passAsking()).toBeNull();
  });

  it('closes the offer form when the turn passes', async () => {
    const enc = player();
    const { view, fixture, state } = mountCombatView({ enc });
    view.offering.set(true);
    await settle(fixture);
    state.apply(next(enc));
    await settle(fixture);
    expect(view.offering()).toBe(true);
    state.apply(next(enc, { revision: 3, currentCombatantId: 'g' }));
    await settle(fixture);
    expect(view.offering()).toBe(false);
  });

  it('can disengage while the mover has the action and has not disengaged', () => {
    const can = (over: object) =>
      mountCombatView({
        enc: player({ combatants: [{ ...toren, ...over } as never, goblin] }),
      }).view.canDisengage();
    expect(can({})).toBe(true);
    expect(can({ actionUsed: true })).toBe(false);
    expect(can({ disengaged: true })).toBe(false);
    expect(
      mountCombatView({
        enc: player({ combatants: [seen, goblin] }),
        master: true,
      }).view.canDisengage(),
    ).toBe(false);
  });

  it('can disengage a creature on its own page', () => {
    const { view } = mountCombatView({
      enc: encounter({
        currentCombatantId: 't',
        combatants: [toren, { ...wolf, disengaged: true } as never],
      }),
    });
    view.moverId.set('w');
    expect(view.canDisengage()).toBe(false);
    view.moverId.set('');
    expect(view.canDisengage()).toBe(true);
  });

  it('reads the jump limits from the character options, or from the creature one on its page', async () => {
    const limits = (n: number) => ({ longRunningDft: n, runningStart: true });
    const { view, fixture } = mountCombatView({
      enc: encounter({ currentCombatantId: 't', combatants: [toren, wolf] }),
      turnOptions: (id) => ({
        options: create(TurnOptionsSchema, { jumps: limits(id === 't' ? 100 : 200) }),
      }),
    });
    await settle(fixture);
    expect(view.moverJumps().longRunningDft).toBe(100);
    view.moverId.set('w');
    expect(view.moverJumps().longRunningDft).toBe(200);
    view.moverId.set('nenhum');
    expect(view.moverJumps()).toBeUndefined();
  });

  it('marks the reactors on the map, the square the mover left only for the one who answers', () => {
    const reactor = combatant({ id: 'r', label: 'R', placed: true, col: 5, row: 6 });
    const hidden = combatant({ id: 'h', label: 'H', placed: false });
    const offer = (id: string, reactorId: string, forYou: boolean) => ({
      id,
      reactorId,
      forYou,
      leftCol: 2,
      leftRow: 3,
    });
    const { view } = mountCombatView({
      enc: encounter({
        combatants: [toren, reactor, hidden],
        opportunityOffers: [
          offer('o1', 'r', true),
          offer('o2', 'r', false),
          offer('o3', 'h', true),
          offer('o4', 'nobody', true),
        ] as never,
      }),
    });
    expect(view.offerMarks()).toEqual([
      { reactor: { col: 5, row: 6 }, left: { col: 2, row: 3 } },
      { reactor: { col: 5, row: 6 }, left: null },
    ]);
  });
});

describe('CombatView state: the opportunity attacks', () => {
  const offerOf = (id: string, reactorId: string) =>
    ({
      id,
      moverId: 't',
      moverLabel: 'Toren',
      reactorId,
      reactorLabel: reactorId === 'g' ? 'Goblin' : 'Outro',
      forYou: true,
      attacks: [{ key: 'scimitar', namePt: 'Cimitarra' }],
    }) as unknown as OpportunityOffer;
  const npcOffer = (
    turnOptions?: () => Partial<GetTurnOptionsResponse>,
    api?: Record<string, () => unknown>,
  ) =>
    mountCombatView({
      enc: encounter({
        currentCombatantId: 't',
        combatants: [toren, goblin],
        opportunityOffers: [offerOf('o1', 'g')],
      }),
      master: true,
      turnOptions,
      api,
    });

  it('reads the options of an NPC reactor once, for the master prompt, and joins them with the offer attacks', async () => {
    const { view, fixture, callsOf, state } = npcOffer(() => ({
      options: create(TurnOptionsSchema, {
        attacks: [{ attack: { key: 'scimitar', namePt: 'Cimitarra', name: 'Scimitar' } }],
      } as never),
    }));
    await settle(fixture);
    const reads = callsOf('turnOptions').filter((c) => c.args[2] === 'g');
    expect(reads.length).toBeGreaterThan(0);
    const before = callsOf('turnOptions').length;
    expect(view.reactorOptions().has('o1')).toBe(true);
    const attacks = view.masterAttacks().get('o1');
    expect(attacks).toHaveLength(1);
    expect(attacks[0].key).toBe('scimitar');
    expect(attacks[0].attack.key).toBe('scimitar');
    // Read once: the combat read again does not ask again for the reactor.
    state.apply(next(state.encounter() as Encounter));
    await settle(fixture);
    expect(callsOf('turnOptions').filter((c) => c.args[2] === 'g').length).toBeGreaterThanOrEqual(
      reads.length,
    );
    expect(before).toBeGreaterThan(0);
    expect(view.reactorFailed().size).toBe(0);
  });

  it('keeps the attack by name when the options of the reactor do not list it', async () => {
    const { view, fixture } = npcOffer();
    await settle(fixture);
    const [attack] = view.masterAttacks().get('o1');
    expect(attack).toEqual({
      key: 'scimitar',
      name: 'Cimitarra',
      detail: 'Cimitarra',
      attack: null,
    });
  });

  it('marks the offer as failed when the reactor options cannot be read, and "Tentar de novo" clears it', async () => {
    let fail = true;
    const { view, fixture } = npcOffer(undefined, {
      turnOptions: () => {
        if (fail) {
          throw new ConnectError('down', Code.Unavailable);
        }
        return { options: create(TurnOptionsSchema, {}) };
      },
    });
    await settle(fixture);
    expect([...view.reactorFailed()]).toEqual(['o1']);
    expect(view.reactorOptions().has('o1')).toBe(false);
    fail = false;
    await view.loadReactor(offerOf('o1', 'g'));
    expect(view.reactorFailed().size).toBe(0);
    expect(view.reactorOptions().has('o1')).toBe(true);
  });

  it('reads nothing for the reactor when there is no combat', async () => {
    const { view, state, callsOf } = npcOffer();
    state.encounter.set(null);
    const before = callsOf('turnOptions').length;
    await view.loadReactor(offerOf('o2', 'g'));
    expect(callsOf('turnOptions')).toHaveLength(before);
  });

  it('does not read the options of a reactor that is a player character', async () => {
    const { callsOf, fixture } = mountCombatView({
      enc: encounter({
        currentCombatantId: 'g',
        combatants: [toren, goblin],
        opportunityOffers: [{ ...offerOf('o1', 't'), moverId: 'g' }] as never,
      }),
      master: true,
    });
    await settle(fixture);
    expect(callsOf('turnOptions').every((c) => c.args[2] !== 't')).toBe(true);
  });
});

describe('CombatView state: the master bar and the clock of the automatic pass', () => {
  afterEach(() => vi.useRealTimers());

  const shield = (over: object = {}) =>
    reactionWindow({
      id: 'w1',
      kind: ReactionKind.SHIELD,
      reactorId: 't',
      reactorLabel: 'Toren',
      reactorIsPlayer: true,
      ...over,
    } as never);
  const withWindow = (w = shield(), over: Partial<Encounter> = {}) =>
    encounter({
      currentCombatantId: 'g',
      combatants: [toren, goblin],
      reactionWindows: [w],
      ...over,
    });
  const answered = (enc: Encounter) =>
    next(enc, { reactionWindows: [shield({ status: ReactionWindowStatus.ANSWERED })] });
  const offerToPlayer = (id = 'o1') =>
    ({
      id,
      moverId: 'g',
      moverLabel: 'Goblin',
      reactorId: 't',
      reactorLabel: 'Toren',
      forYou: true,
      attacks: [],
    }) as unknown as OpportunityOffer;

  it('says nothing to a player, and has no clocks', () => {
    const { view } = mountCombatView({ enc: withWindow() });
    expect(view.waitNote()).toBe('');
    expect(view.autoLeft()).toEqual({});
    expect(view.offerLeft()).toEqual({});
    expect(view.autoPass.timer).toBeUndefined();
  });

  it('says what the master waits for, with the time left of an optional reaction of a player', () => {
    vi.useFakeTimers();
    const { view } = mountCombatView({ enc: withWindow(), master: true });
    expect(view.autoLeft()).toEqual({ w1: 30 });
    expect(view.waitNote()).toBe('Esperando a reação de Toren · passa sozinho em 30 s');
    expect(view.autoPass.timer).toBeDefined();
  });

  it('counts the seconds down on the clock and passes the reaction when the 30 s are up, with its own key', async () => {
    vi.useFakeTimers();
    const enc = withWindow();
    const { view, fixture, callsOf, state } = mountCombatView({
      enc,
      master: true,
      api: { answerReaction: () => ({ encounter: answered(enc) }) },
    });
    await vi.advanceTimersByTimeAsync(10_000);
    expect(view.autoLeft()).toEqual({ w1: 20 });
    expect(view.waitNote()).toBe('Esperando a reação de Toren · passa sozinho em 20 s');
    expect(callsOf('answerReaction')).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(20_000);
    await settle(fixture);
    expect(callsOf('answerReaction')).toHaveLength(1);
    expect(callsOf('answerReaction')[0].args).toEqual([
      'c',
      'enc',
      'w1',
      { use: false },
      autoPassKey('w1'),
    ]);
    // The answer is applied: the window is gone, and so is the clock.
    expect(state.encounter()?.revision).toBe(2);
    expect(view.autoLeft()).toEqual({});
    expect(view.waitNote()).toBe('');
    expect(view.autoPass.timer).toBeUndefined();
    await vi.advanceTimersByTimeAsync(5000);
    expect(callsOf('answerReaction')).toHaveLength(1);
  });

  it('waits up to two minutes for a reaction its player is rolling', () => {
    vi.useFakeTimers();
    const { view } = mountCombatView({ enc: withWindow(shield({ rolling: true })), master: true });
    expect(view.autoLeft()).toEqual({ w1: 120 });
  });

  it('gives no clock to a window that is not an optional reaction of a player, and writes only the wait', () => {
    const concentration = shield({ kind: ReactionKind.CONCENTRATION_SAVE });
    const { view } = mountCombatView({ enc: withWindow(concentration), master: true });
    expect(view.autoLeft()).toEqual({});
    expect(view.autoPass.timer).toBeUndefined();
    expect(view.waitNote()).toBe('Esperando a reação de Toren');
    const npc = shield({ reactorIsPlayer: false, reactorId: 'g', reactorLabel: 'Goblin' });
    expect(mountCombatView({ enc: withWindow(npc), master: true }).view.waitNote()).toBe(
      'Esperando a sua reação: Goblin',
    );
    const check = shield({ kind: ReactionKind.MASTER_CHECK, reactorLabel: '' });
    expect(mountCombatView({ enc: withWindow(check), master: true }).view.waitNote()).toBe(
      'Esperando a sua resposta',
    );
  });

  it('puts the questions about hidden creatures and the opportunity offers before the windows', () => {
    const question = { id: 'q', spellKey: 'k', combatantIds: ['g'] };
    const asked = mountCombatView({
      enc: withWindow(shield(), { pendingHiddenReveals: [question] as never }),
      master: true,
    });
    expect(asked.view.waitNote()).toBe('Esperando a sua resposta: escondidas atingidas');
    const npcOffer = { ...offerToPlayer(), reactorId: 'g', reactorLabel: 'Goblin', moverId: 't' };
    const own = mountCombatView({
      enc: encounter({ combatants: [toren, goblin], opportunityOffers: [npcOffer] as never }),
      master: true,
    });
    expect(own.view.waitNote()).toBe('Esperando a sua reação: Goblin');
    const player = mountCombatView({
      enc: encounter({ combatants: [toren, goblin], opportunityOffers: [offerToPlayer()] }),
      master: true,
    });
    expect(player.view.waitNote()).toBe('Esperando a reação do jogador de Toren');
    const named = mountCombatView({
      enc: encounter({ combatants: [toren, goblin], opportunityOffers: [offerToPlayer()] }),
      master: true,
      inputs: { partyInfo: new Map([['toren-c', { classSummary: '', playerName: 'Caio' }]]) },
    });
    expect(named.view.waitNote()).toBe('Esperando a reação do Caio (Toren)');
  });

  it('why "Próximo turno" waits for the questions, for the master only', () => {
    const q = (n: number) =>
      Array.from({ length: n }, (_, i) => ({ id: `q${i}`, spellKey: 'k', combatantIds: [] }));
    const why = (n: number, master: boolean) =>
      mountCombatView({
        enc: encounter({ combatants: [toren, goblin], pendingHiddenReveals: q(n) as never }),
        master,
      }).view.waitWhy();
    expect(why(0, true)).toBe('');
    expect(why(1, true)).toBe('Responda ao pedido abaixo para seguir.');
    expect(why(2, true)).toBe('Responda aos pedidos abaixo para seguir.');
    expect(why(1, false)).toBe('');
  });

  it('counts down the opportunity offers a player answers, but not the NPC reactors', () => {
    vi.useFakeTimers();
    const npcOffer = {
      ...offerToPlayer('o2'),
      reactorId: 'g',
      reactorLabel: 'Goblin',
      moverId: 't',
    };
    const { view } = mountCombatView({
      enc: encounter({
        combatants: [toren, goblin],
        opportunityOffers: [offerToPlayer(), npcOffer] as never,
      }),
      master: true,
    });
    expect(view.offerLeft()).toEqual({ o1: 30 });
    expect(view.autoPass.timer).toBeDefined();
  });

  it('passes the offer by itself after 30 s with its own key, and applies the combat the server answers', async () => {
    vi.useFakeTimers();
    const enc = encounter({
      currentCombatantId: 'g',
      combatants: [toren, goblin],
      opportunityOffers: [offerToPlayer()],
    });
    const done = next(enc, { opportunityOffers: [] });
    const { view, fixture, callsOf, state } = mountCombatView({
      enc,
      master: true,
      api: { skipOpportunity: () => done },
    });
    await vi.advanceTimersByTimeAsync(29_000);
    expect(callsOf('skipOpportunity')).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1000);
    await settle(fixture);
    expect(callsOf('skipOpportunity').map((c) => c.args)).toEqual([
      ['c', 'enc', 'o1', autoPassKey('o1')],
    ]);
    expect(state.encounter()).toEqual(done);
    expect(view.offerLeft()).toEqual({});
    expect(view.autoPass.timer).toBeUndefined();
  });

  describe.each([
    Code.Unavailable,
    Code.DeadlineExceeded,
    Code.Unknown,
    Code.Internal,
    Code.Aborted,
  ])('a pass that fails with the network code %s', (code) => {
    it('is sent again on the next tick, for a window and for an offer', async () => {
      vi.useFakeTimers();
      const fail = () => {
        throw new ConnectError('x', code);
      };
      const offerEnc = encounter({
        combatants: [toren, goblin],
        opportunityOffers: [offerToPlayer()],
      });
      const a = mountCombatView({
        enc: withWindow(),
        master: true,
        api: { answerReaction: fail },
      });
      await vi.advanceTimersByTimeAsync(31_000);
      expect(a.callsOf('answerReaction').length).toBeGreaterThanOrEqual(2);
      const b = mountCombatView({ enc: offerEnc, master: true, api: { skipOpportunity: fail } });
      await vi.advanceTimersByTimeAsync(31_000);
      expect(b.callsOf('skipOpportunity').length).toBeGreaterThanOrEqual(2);
    });
  });

  it('does not send again a pass the server refused, for a window and for an offer', async () => {
    vi.useFakeTimers();
    const refuse = () => {
      throw new ConnectError('answered meanwhile', Code.FailedPrecondition);
    };
    const a = mountCombatView({ enc: withWindow(), master: true, api: { answerReaction: refuse } });
    await vi.advanceTimersByTimeAsync(40_000);
    expect(a.callsOf('answerReaction')).toHaveLength(1);
    const b = mountCombatView({
      enc: encounter({ combatants: [toren, goblin], opportunityOffers: [offerToPlayer()] }),
      master: true,
      api: { skipOpportunity: refuse },
    });
    await vi.advanceTimersByTimeAsync(40_000);
    expect(b.callsOf('skipOpportunity')).toHaveLength(1);
  });

  it('sends a pass that failed with something that is not a Connect error again', async () => {
    vi.useFakeTimers();
    const a = mountCombatView({
      enc: withWindow(),
      master: true,
      api: {
        answerReaction: () => {
          throw new Error('boom');
        },
      },
    });
    await vi.advanceTimersByTimeAsync(32_000);
    expect(a.callsOf('answerReaction').length).toBeGreaterThanOrEqual(2);
  });

  it('stops the clock with the component', async () => {
    vi.useFakeTimers();
    const { fixture, callsOf } = mountCombatView({ enc: withWindow(), master: true });
    fixture.destroy();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(callsOf('answerReaction')).toHaveLength(0);
  });

  it('does nothing at a tick when there is no combat', async () => {
    vi.useFakeTimers();
    const { view, fixture, callsOf, state } = mountCombatView({ enc: withWindow(), master: true });
    state.encounter.set(null);
    await vi.advanceTimersByTimeAsync(31_000);
    await settle(fixture);
    expect(callsOf('answerReaction')).toHaveLength(0);
    expect(view.encounter()).toBeNull();
  });
});

describe('CombatView state: what the effects read from the server', () => {
  it('asks for the options of the own character even off turn, again at every revision, and clears them when the combat ends', async () => {
    const enc = player({ currentCombatantId: 'g' });
    const { view, fixture, callsOf, state } = mountCombatView({ enc });
    await settle(fixture);
    expect(callsOf('turnOptions').map((c) => c.args)).toEqual([['c', 'enc', 't']]);
    expect(view.options()).not.toBeNull();
    state.apply(next(enc));
    await settle(fixture);
    expect(callsOf('turnOptions')).toHaveLength(2);
    state.apply(next(enc, { revision: 3, status: EncounterStatus.ENDED }));
    await settle(fixture);
    expect(callsOf('turnOptions')).toHaveLength(2);
    expect(view.options()).toBeNull();
  });

  it('asks for the options of the master subject, and for nobody while the combat is being set up', async () => {
    const m = mountCombatView({
      enc: encounter({ currentCombatantId: 'g', combatants: [toren, goblin] }),
      master: true,
    });
    await settle(m.fixture);
    expect(m.callsOf('turnOptions').map((c) => c.args[2])).toEqual(['g']);
    const setup = mountCombatView({
      enc: encounter({ status: EncounterStatus.SETUP, combatants: [toren, goblin] }),
      master: true,
    });
    await settle(setup.fixture);
    expect(setup.callsOf('turnOptions')).toHaveLength(0);
  });

  it('reads the combat log of a combat that started, again at every log tick, and clears it while being set up', async () => {
    let rounds: unknown[] = [roundOf(1, [])];
    const enc = player();
    const { view, fixture, callsOf, state } = mountCombatView({
      enc,
      api: { log: () => ({ rounds, undoableEventId: 'u' }) },
    });
    await settle(fixture);
    expect(callsOf('log').map((c) => c.args)).toEqual([['c', 'enc']]);
    expect(view.log.rounds()).toHaveLength(1);
    expect(view.log.undoableId()).toBe('u');
    rounds = [];
    state.touchLog();
    await settle(fixture);
    expect(callsOf('log')).toHaveLength(2);
    expect(view.log.rounds()).toEqual([]);
    state.apply(next(enc, { status: EncounterStatus.SETUP }));
    await settle(fixture);
    expect(callsOf('log')).toHaveLength(2);
    expect(view.log.loaded()).toBe(false);
  });

  it('reads the options of each creature the player plays, and nothing for the master', async () => {
    const enc = encounter({ currentCombatantId: 't', combatants: [toren, wolf, goblin] });
    const { view, fixture, callsOf, state } = mountCombatView({ enc });
    await settle(fixture);
    expect(
      callsOf('turnOptions')
        .map((c) => c.args[2])
        .sort(),
    ).toEqual(['t', 'w']);
    expect([...view.creatureOptions.data().keys()]).toEqual(['w']);
    state.apply(next(enc, { combatants: [toren, goblin] }));
    await settle(fixture);
    expect(view.creatureOptions.data().size).toBe(0);
    const m = mountCombatView({ enc, master: true });
    await settle(m.fixture);
    expect(m.view.creatureOptions.data().size).toBe(0);
  });

  it('reads the painted layers of the map the combat is on, and again when the layers revision changes', async () => {
    const { fixture, mapState } = mountCombatView({ enc: player() });
    const layers = vi.fn(async () => ({}));
    TestBed.inject(MapsClient).layers = layers as never;
    mapState.setMap({ id: 'other', layersRevision: 1 } as never);
    await settle(fixture);
    expect(layers).not.toHaveBeenCalled();
    mapState.setMap({ id: 'map', layersRevision: 1 } as never);
    await settle(fixture);
    expect(layers).toHaveBeenCalledTimes(1);
    expect(layers).toHaveBeenCalledWith('c', 'map');
    mapState.setMap({ id: 'map', layersRevision: 1, name: 'Renomeado' } as never);
    await settle(fixture);
    expect(layers).toHaveBeenCalledTimes(1);
    mapState.setMap({ id: 'map', layersRevision: 2 } as never);
    await settle(fixture);
    expect(layers).toHaveBeenCalledTimes(2);
  });

  it('draws the layers of the fog vision for a player when the fog is on, and the map ones otherwise', () => {
    const fogLayers = { walls: 'da névoa' };
    const fog = (vision: unknown) => ({ vision: () => vision, layers: () => fogLayers });
    const on = mountCombatView({ enc: player(), inputs: { fog: fog({ v: 1 }) } });
    expect(on.view.fogVision()).toEqual({ v: 1 });
    expect(on.view.layers()).toBe(fogLayers);
    // A fog that has no layers of its own yet draws the map ones.
    const bare = mountCombatView({
      enc: player(),
      inputs: { fog: { vision: () => ({ v: 1 }), layers: () => undefined } },
    });
    expect(bare.view.layers()).toBe(bare.view.layersState.layers());
    const off = mountCombatView({ enc: player(), inputs: { fog: fog(null) } });
    expect(off.view.fogVision()).toBeNull();
    expect(off.view.layers()).not.toBe(fogLayers);
    const master = mountCombatView({ enc: player(), master: true, inputs: { fog: fog({ v: 1 }) } });
    expect(master.view.fogVision()).toBeNull();
    expect(master.view.layers()).not.toBe(fogLayers);
    const noFog = mountCombatView({ enc: player() });
    expect(noFog.view.fogVision()).toBeNull();
  });

  it('writes the map name and the image only for the map of the combat', () => {
    const { view, mapState } = mountCombatView({ enc: player() });
    expect(view.mapName()).toBe('');
    expect(view.image()).toBeNull();
    mapState.setMap({
      id: 'map',
      name: 'Estrada',
      image: { url: '/m.png', width: 10, height: 20 },
    } as never);
    expect(view.mapName()).toBe('Estrada');
    expect(view.image()).toEqual({ url: '/m.png', width: 10, height: 20 });
    mapState.setMap({
      id: 'outro',
      name: 'Outro',
      image: { url: '/o.png', width: 1, height: 1 },
    } as never);
    expect(view.image()).toBeNull();
  });
});

describe('CombatView state: the spell catalog', () => {
  it('reads the details of each spell in the list once, skipping the ones with no key', async () => {
    const enc = player();
    const spells = [{ spell: { key: 'magic-missile' } }, { spell: { key: '' } }, {}];
    const { view, fixture, state } = mountCombatView({
      enc,
      turnOptions: () => ({ options: create(TurnOptionsSchema, { spells } as never) }),
    });
    await settle(fixture);
    const details = vi.fn(async () => create(SpellDetailsSchema, {}));
    TestBed.inject(SpellCatalog).details = details as never;
    state.apply(next(enc));
    await settle(fixture);
    expect(details.mock.calls).toEqual([['c', 'magic-missile']]);
    expect([...view.spellDetails().keys()]).toEqual(['magic-missile']);
    state.apply(next(enc, { revision: 3 }));
    await settle(fixture);
    expect(details).toHaveBeenCalledTimes(1);
  });

  it('keeps no details for a spell the catalog does not know', async () => {
    const enc = player();
    const { view, fixture, state } = mountCombatView({
      enc,
      turnOptions: () => ({
        options: create(TurnOptionsSchema, { spells: [{ spell: { key: 'x' } }] } as never),
      }),
    });
    await settle(fixture);
    state.apply(next(enc));
    await settle(fixture);
    expect(view.spellDetails().size).toBe(0);
  });

  it('names the spell of each question about hidden creatures through the catalog, only for the master', async () => {
    const question = (id: string, spellKey: string) => ({ id, spellKey, combatantIds: [] });
    const enc = encounter({
      combatants: [toren, goblin],
      pendingHiddenReveals: [
        question('q1', 'fireball'),
        question('q2', 'fireball'),
        question('q3', 'nameless'),
      ] as never,
    });
    const m = mountCombatView({ enc, master: true });
    await settle(m.fixture);
    const details = vi.fn(async (_c: string, key: string) =>
      key === 'fireball'
        ? { spell: { namePt: 'Bola de Fogo', name: 'Fireball' } }
        : { spell: { namePt: '', name: '' } },
    );
    TestBed.inject(SpellCatalog).details = details as never;
    m.state.apply(next(enc));
    await settle(m.fixture);
    // Once per spell, however many questions are about it.
    expect(details.mock.calls.map((c) => c[1]).sort()).toEqual(['fireball', 'nameless']);
    expect([...m.view.revealSpellNames()]).toEqual([['fireball', 'Bola de Fogo']]);
    m.state.apply(next(enc, { revision: 3 }));
    await settle(m.fixture);
    expect(details.mock.calls.map((c) => c[1]).filter((k) => k === 'fireball')).toHaveLength(1);

    const g = mountCombatView({ enc });
    await settle(g.fixture);
    const guestDetails = vi.fn(async () => create(SpellDetailsSchema, {}));
    TestBed.inject(SpellCatalog).details = guestDetails as never;
    g.state.apply(next(enc));
    await settle(g.fixture);
    expect(guestDetails).not.toHaveBeenCalled();
  });

  it('falls back to the English name of a spell with no Portuguese one', async () => {
    const enc = encounter({
      combatants: [toren, goblin],
      pendingHiddenReveals: [{ id: 'q1', spellKey: 'k', combatantIds: [] }] as never,
    });
    const m = mountCombatView({ enc, master: true });
    await settle(m.fixture);
    TestBed.inject(SpellCatalog).details = (async () => ({
      spell: { namePt: '', name: 'Fireball' },
    })) as never;
    m.state.apply(next(enc));
    await settle(m.fixture);
    expect(m.view.revealSpellNames().get('k')).toBe('Fireball');
  });
});

describe('CombatView state: the familiar and the form of the druid', () => {
  const familiars = (list: unknown) => ({ list: async () => list });
  const FAMILIAR = { source: CreatureSource.FAMILIAR, name: 'Nanquim' };

  it('does not know a familiar the list does not have, nor when the read fails', async () => {
    const none = mountCombatView({
      enc: player(),
      creatures: familiars([{ source: CreatureSource.CONJURE_ANIMALS, name: 'Lobo' }]),
    });
    await settle(none.fixture);
    expect(none.view.familiar()).toBeNull();
    const failing = mountCombatView({
      enc: player(),
      creatures: {
        list: async () => {
          throw new ConnectError('x', Code.Unavailable);
        },
      },
    });
    await settle(failing.fixture);
    expect(failing.view.familiar()).toBeNull();
  });

  it('forgets the familiar when the character looks through its eyes, and never reads it for the master', async () => {
    const list = vi.fn(async () => [FAMILIAR]);
    const enc = player();
    const { view, fixture, state } = mountCombatView({ enc, creatures: { list } });
    await settle(fixture);
    expect(view.familiar()).toBe('Nanquim');
    state.apply(
      next(enc, { combatants: [{ ...toren, familiarSightCreatureId: 'f' } as never, goblin] }),
    );
    await settle(fixture);
    expect(view.lookingThroughFamiliar()).toBe(true);
    expect(view.familiar()).toBeNull();
    expect(list).toHaveBeenCalledTimes(1);
    const m = mountCombatView({ enc, master: true, creatures: { list } });
    await settle(m.fixture);
    expect(m.view.familiar()).toBeNull();
    expect(list).toHaveBeenCalledTimes(1);
  });

  it('ignores the answer of a read that a newer one overtook', async () => {
    const pending: ((v: unknown) => void)[] = [];
    const list = vi.fn(() => new Promise((resolve) => pending.push(resolve)));
    const { view, fixture } = mountCombatView({ enc: player(), creatures: { list } });
    fixture.componentRef.setInput('campaignId', 'c2');
    await settle(fixture);
    expect(pending).toHaveLength(2);
    pending[1]([{ source: CreatureSource.FAMILIAR, name: 'Novo' }]);
    await settle(fixture);
    pending[0]([FAMILIAR]);
    await settle(fixture);
    expect(view.familiar()).toBe('Novo');
  });

  it('asks the question of the familiar eyes with the names, and only when there is one', async () => {
    const { view, fixture, opened } = mountCombatView({
      enc: player(),
      creatures: familiars([FAMILIAR]),
      matches: [PHONE_QUERY],
    });
    view.askFamiliarEyes();
    expect(opened).toHaveLength(0);
    await settle(fixture);
    view.askFamiliarEyes();
    expect(opened).toHaveLength(1);
    expect(opened[0].config.ariaLabel).toBe('Ver pelos olhos do Nanquim?');
    expect(opened[0].config.data).toEqual({
      campaignId: 'c',
      characterId: 'toren-c',
      characterName: 'Toren',
      familiarName: 'Nanquim',
      inCombat: true,
    });
    const m = mountCombatView({ enc: player(), master: true, creatures: familiars([FAMILIAR]) });
    await settle(m.fixture);
    m.view.askFamiliarEyes();
    expect(m.opened).toHaveLength(0);
  });

  const druid = (over: object = {}, who = toren) =>
    encounter({
      currentCombatantId: 't',
      combatants: [{ ...who, ...over } as never, goblin],
    });
  const wildOptions =
    (over: object = {}) =>
    () => ({
      options: create(TurnOptionsSchema, {
        featureActions: [
          {
            action: {
              key: 'feature:wild-shape',
              namePt: 'Forma Selvagem',
              resourceKey: 'wild_shape',
            },
            enabled: true,
            usesLeft: 2,
            ...over,
          },
        ],
      } as never),
    });
  const wildRow = {
    characterId: 'toren-c',
    resources: [{ key: 'wild_shape', total: 2, used: 0, recharge: 'long_rest' }],
  };

  it('writes the Wild Shape line of the Ação from the feature, its resource and the form', async () => {
    const { view, fixture } = mountCombatView({
      enc: druid(),
      turnOptions: wildOptions(),
      inputs: { vitals: [wildRow] },
    });
    await settle(fixture);
    expect(view.wild()).toEqual({
      key: 'feature:wild-shape',
      detail: 'Vire uma fera\u00a0· restam\u00a02 de\u00a02 usos\u00a0· volta no descanso longo',
      reason: '',
    });
    const noRow = mountCombatView({ enc: druid(), turnOptions: wildOptions({ enabled: false }) });
    await settle(noRow.fixture);
    expect(noRow.view.wild().detail).toBe(
      'Vire uma fera\u00a0· restam\u00a02 usos\u00a0· volta no descanso curto ou longo',
    );
    const reasonOf = async (code: DisabledReasonCode) => {
      const m = mountCombatView({
        enc: druid(),
        turnOptions: wildOptions({ enabled: false, reason: { code } }),
      });
      await settle(m.fixture);
      return m.view.wild().reason;
    };
    expect(await reasonOf(DisabledReasonCode.NO_USES)).toBe('Sem usos');
    expect(await reasonOf(DisabledReasonCode.ACTION_USED)).toBe('Sem ação disponível');
    // While it is a beast, and without the feature or the character, there is no line.
    const beast = mountCombatView({
      enc: druid({ wildShapeBeastKey: 'monster:wolf' }),
      turnOptions: wildOptions(),
      inputs: { vitals: [wildRow] },
    });
    await settle(beast.fixture);
    expect(beast.view.wild()).toBeNull();
    const none = mountCombatView({ enc: druid() });
    await settle(none.fixture);
    expect(none.view.wild()).toBeNull();
    const m = mountCombatView({ enc: druid({}, seen), master: true, turnOptions: wildOptions() });
    await settle(m.fixture);
    expect(m.view.wild()).toBeNull();
  });

  it('reads the book of the beast once for each beast, and forgets it when the druid is not one', async () => {
    const block = create(CreatureSchema, {});
    const statBlock = vi.fn(async () => block);
    const enc = druid({ wildShapeBeastKey: 'monster:wolf' });
    const { view, fixture, state } = mountCombatView({ enc, creatures: { statBlock } });
    await settle(fixture);
    expect(statBlock).toHaveBeenCalledWith('c', 'monster:wolf');
    expect(view.beastBlock()).toBe(block);
    state.apply(next(enc));
    await settle(fixture);
    expect(statBlock).toHaveBeenCalledTimes(1);
    state.apply(next(druid(), { revision: 3 }));
    await settle(fixture);
    expect(view.beastBlock()).toBeNull();
    expect(statBlock).toHaveBeenCalledTimes(1);
  });

  it('forgets the beast book when the read fails', async () => {
    const { view, fixture } = mountCombatView({
      enc: druid({ wildShapeBeastKey: 'monster:wolf' }),
      creatures: {
        statBlock: async () => {
          throw new ConnectError('x', Code.NotFound);
        },
      },
    });
    await settle(fixture);
    expect(view.beastBlock()).toBeNull();
  });

  it('opens the list of beasts with the uses that are left, and applies the combat the sheet answers', async () => {
    const enc = druid();
    const row = { characterId: 'toren-c', resources: [{ key: 'wild_shape', total: 3, used: 1 }] };
    const { view, opened, state } = mountCombatView({
      enc,
      matches: [PHONE_QUERY],
      inputs: { vitals: [row] },
    });
    view.openTransform();
    expect(opened).toHaveLength(1);
    expect(opened[0].config.ariaLabel).toBe('Forma Selvagem');
    expect(opened[0].config.data).toEqual({
      campaignId: 'c',
      characterId: 'toren-c',
      inCombat: true,
      uses: { left: 2, total: 3 },
    });
    const turned = next(enc, { name: 'Depois' });
    opened[0].close({ encounter: turned });
    expect(state.encounter()).toBe(turned);
  });

  it('opens the list with no uses without a row, does not apply a closed sheet, and opens nothing for the master', () => {
    const { view, opened, state } = mountCombatView({ enc: druid(), matches: [PHONE_QUERY] });
    view.openTransform();
    expect(opened[0].config.data['uses']).toBeNull();
    opened[0].close({});
    expect(state.encounter()?.revision).toBe(1);
    const m = mountCombatView({ enc: druid({}, seen), master: true });
    m.view.openTransform();
    expect(m.opened).toHaveLength(0);
  });

  it('leaves the form: applies the combat the server answers, with a new key for the next time', async () => {
    const enc = druid();
    const back = next(enc, { name: 'Voltou' });
    const leaveWildShape = vi.fn(async () => ({ encounter: back }));
    const { view, state } = mountCombatView({ enc, creatures: { leaveWildShape } });
    await view.leaveForm();
    expect(leaveWildShape).toHaveBeenCalledWith('c', 'toren-c', expect.any(String));
    expect(state.encounter()).toBe(back);
    expect(view.busy()).toBe(false);
    await view.leaveForm();
    const keys = leaveWildShape.mock.calls.map((c) => (c as unknown[])[2]);
    expect(keys[0]).not.toBe(keys[1]);
  });

  it('says why leaving the form failed, and keeps the key for the retry of what was never answered', async () => {
    const err = new ConnectError('no', Code.PermissionDenied);
    const leaveWildShape = vi.fn(async () => {
      throw err;
    });
    const { view, callsOf } = mountCombatView({ enc: druid(), creatures: { leaveWildShape } });
    await view.leaveForm();
    expect(view.error()).toBe(combatErrorMessage(err, 'voltar à forma normal'));
    expect(view.error()).not.toBe('');
    expect(view.busy()).toBe(false);
    await view.leaveForm();
    const keys = leaveWildShape.mock.calls.map((c) => (c as unknown[])[2]);
    expect(keys[0]).toBe(keys[1]);
    // A refusal that is not about a stale screen does not read the combat again.
    expect(callsOf('get')).toHaveLength(0);
  });

  it('reads the combat again when the refusal says the screen is stale', async () => {
    const enc = druid();
    const fresh = next(enc, { name: 'Lido de novo' });
    const leaveWildShape = vi.fn(async () => {
      throw new ConnectError('stale', Code.Aborted);
    });
    const { view, callsOf, state } = mountCombatView({
      enc,
      creatures: { leaveWildShape },
      api: { get: () => fresh },
    });
    await view.leaveForm();
    expect(callsOf('get')).toHaveLength(1);
    expect(state.encounter()).toBe(fresh);
  });

  it('leaves the form without touching the combat when the answer carries none', async () => {
    const enc = druid();
    const { view, state } = mountCombatView({
      enc,
      creatures: { leaveWildShape: async () => ({}) },
    });
    await view.leaveForm();
    expect(state.encounter()).toBe(enc);
    expect(view.error()).toBe('');
  });

  it('does not leave the form without a character, or while busy', async () => {
    const leaveWildShape = vi.fn(async () => ({}));
    const m = mountCombatView({
      enc: druid({}, seen),
      master: true,
      creatures: { leaveWildShape },
    });
    await m.view.leaveForm();
    const busy = mountCombatView({ enc: druid(), creatures: { leaveWildShape } });
    busy.view.busy.set(true);
    await busy.view.leaveForm();
    expect(leaveWildShape).not.toHaveBeenCalled();
  });
});

describe('CombatView state: what the log tells the player about the form and the concentration', () => {
  const form = (id: string, endReason: WildShapeEndReason, extra: object = {}) => ({
    id,
    kind: CombatLogKind.WILD_SHAPE,
    actorId: 't',
    wildShape: { started: false, endReason, beastNamePt: 'Lobo', carriedDamage: 6, ...extra },
  });
  const round = (...entries: unknown[]) => [roundOf(2, entries)];

  async function mountWithLog() {
    let rounds: unknown[] = round();
    const enc = player();
    const m = mountCombatView({ enc, api: { log: () => ({ rounds, undoableEventId: '' }) } });
    await settle(m.fixture);
    const read = async (...entries: unknown[]) => {
      rounds = round(...entries);
      m.state.touchLog();
      await settle(m.fixture);
    };
    return { ...m, enc, read };
  }

  it('tells the end of the form that the log brings after the first read, with the carried damage', async () => {
    const { view, read } = await mountWithLog();
    expect(view.formNotice()).toBe('');
    await read(form('l1', WildShapeEndReason.DAMAGE));
    expect(view.formNotice()).toBe(
      'O Lobo caiu a\u00a00 PV e você voltou à forma normal. 6 de dano passaram para você.',
    );
    // A re-read of the same line says nothing new, and the notice stays until it is touched.
    await read(form('l1', WildShapeEndReason.DAMAGE));
    expect(view.formNotice()).not.toBe('');
  });

  it('does not repeat as news a line the first read already held', async () => {
    let rounds: unknown[] = round(form('old', WildShapeEndReason.MASTER));
    const m = mountCombatView({
      enc: player(),
      api: { log: () => ({ rounds, undoableEventId: '' }) },
    });
    await settle(m.fixture);
    expect(m.view.formNotice()).toBe('');
    rounds = round(form('old', WildShapeEndReason.MASTER), form('new', WildShapeEndReason.ZERO_HP));
    m.state.touchLog();
    await settle(m.fixture);
    expect(m.view.formNotice()).toBe('Você caiu a 0 PV e voltou à forma normal.');
  });

  it('says nothing of a form the druid left by choice, or of a line of someone else', async () => {
    const { view, read } = await mountWithLog();
    await read(form('l1', WildShapeEndReason.LEFT));
    expect(view.formNotice()).toBe('');
    await read({ ...form('l2', WildShapeEndReason.DAMAGE), actorId: 'g' });
    expect(view.formNotice()).toBe('');
  });

  it('puts the notice away when the combat changes', async () => {
    const { view, read, state, fixture, enc } = await mountWithLog();
    await read(form('l1', WildShapeEndReason.UNCONSCIOUS));
    expect(view.formNotice()).toBe('Você ficou inconsciente e voltou à forma normal.');
    state.apply({ ...next(enc), id: 'outro' } as Encounter);
    await settle(fixture);
    expect(view.formNotice()).toBe('');
  });

  it('tells the player the concentration that held creatures ended, and takes it away when they are back', async () => {
    const lobo = { ...wolf, label: 'Lobo atroz 1' } as never;
    const concentrating = { ...toren, concentrationSpellNamePt: 'Conjurar Animais' } as never;
    const enc = encounter({ currentCombatantId: 't', combatants: [concentrating, lobo, goblin] });
    const { view, fixture, state } = mountCombatView({ enc });
    await settle(fixture);
    expect(view.lostNotice()).toBe('');
    state.apply(next(enc, { combatants: [toren, goblin] }));
    await settle(fixture);
    expect(view.lostNotice()).toBe(
      'Você perdeu a concentração em Conjurar Animais. Lobo atroz 1 sumiu.',
    );
    // An undo that brings the creature back takes the notice away.
    state.apply(next(enc, { revision: 3 }));
    await settle(fixture);
    expect(view.lostNotice()).toBe('');
  });

  it('starts clean in another combat', async () => {
    const concentrating = { ...toren, concentrationSpellNamePt: 'Conjurar Animais' } as never;
    const enc = encounter({ currentCombatantId: 't', combatants: [concentrating, wolf, goblin] });
    const { view, fixture, state } = mountCombatView({ enc });
    await settle(fixture);
    state.apply(next(enc, { combatants: [toren, goblin] }));
    await settle(fixture);
    expect(view.lostNotice()).not.toBe('');
    state.apply({
      ...next(enc, { revision: 5 }),
      id: 'outro',
      combatants: [toren, goblin],
    } as Encounter);
    await settle(fixture);
    expect(view.lostNotice()).toBe('');
  });
});

describe('CombatView state: the notes from the combat log about traps and the rage', () => {
  const trapEntry = (targetId: string, label = '') => ({
    id: `trap-${targetId}`,
    kind: CombatLogKind.TRAP_TRIGGERED,
    trap: {
      name: 'Fosso',
      caught: [
        {
          targetId,
          targetLabel: label,
          attacks: [],
          saves: [],
          conditionKeys: [],
          damages: [{ amount: 7, damageTypePt: 'concussão', status: PendingDamageStatus.ROLLED }],
        },
      ],
    },
  });
  const mountTrap = async (
    combatants: unknown[],
    entries: unknown[],
    round = 2,
    master = false,
  ) => {
    const m = mountCombatView({
      enc: encounter({ currentCombatantId: 't', combatants: combatants as never }),
      master,
      api: { log: () => ({ rounds: [roundOf(round, entries)], undoableEventId: '' }) },
    });
    await settle(m.fixture);
    return m;
  };

  it('tells the player what a trap did to the own character this round', async () => {
    const { view } = await mountTrap([toren, goblin], [trapEntry('x'), trapEntry('t')]);
    expect(view.trapNote()).toEqual({
      title: 'Você caiu na armadilha Fosso.',
      detail: '7 de concussão.',
      waiting: true,
    });
  });

  it('has no trap note for another round, another victim, an entry that is not a trap, or the master', async () => {
    expect((await mountTrap([toren], [trapEntry('t')], 1)).view.trapNote()).toBeNull();
    expect((await mountTrap([toren], [trapEntry('x')])).view.trapNote()).toBeNull();
    expect(
      (
        await mountTrap(
          [toren],
          [
            { id: 'a', kind: CombatLogKind.ATTACK },
            { id: 'b', kind: CombatLogKind.TRAP_TRIGGERED },
          ],
        )
      ).view.trapNote(),
    ).toBeNull();
    expect((await mountTrap([goblin], [trapEntry('t')], 2, true)).view.trapNote()).toBeNull();
  });

  it('names each creature of the player that a trap caught', async () => {
    const { view } = await mountTrap(
      [toren, wolf, goblin],
      [trapEntry('x'), trapEntry('w'), trapEntry('w')],
    );
    expect([...view.creatureTrapNotes().keys()]).toEqual(['w']);
    expect(view.creatureTrapNotes().get('w').title).toBe('O Lobo caiu na armadilha Fosso.');
    expect((await mountTrap([toren, wolf], [])).view.creatureTrapNotes().size).toBe(0);
    expect(
      (await mountTrap([toren, wolf], [trapEntry('w')], 2, true)).view.creatureTrapNotes().size,
    ).toBe(0);
  });

  const raging = (id: string) =>
    encounter({ ragePendingCombatantId: id, currentCombatantId: 't', combatants: [toren, goblin] });

  it('asks the owner, or the master, about the raging barbarian the turn waits for', () => {
    expect(mountCombatView({ enc: raging('t') }).view.rageAsked().id).toBe('t');
    expect(mountCombatView({ enc: raging('g'), master: true }).view.rageAsked().id).toBe('g');
    // Not a player's own character: the question is not theirs.
    expect(mountCombatView({ enc: raging('g') }).view.rageAsked()).toBeNull();
    // Nobody waits, or the id is not in the combat.
    expect(mountCombatView({ enc: raging(''), master: true }).view.rageAsked()).toBeFalsy();
    expect(mountCombatView({ enc: raging('fantasma'), master: true }).view.rageAsked()).toBeFalsy();
  });
});

describe('CombatView state: the saving throw of an effect that waits for the player', () => {
  const save = (over: object = {}) =>
    reactionWindow({
      id: 'e1',
      kind: ReactionKind.EFFECT_SAVE,
      reactorId: 't',
      reactorLabel: 'Toren',
      reactorIsPlayer: true,
      prompt: { case: 'effectSave', value: {} } as never,
      ...over,
    } as never);
  const mountSave = (over: Partial<Encounter> = {}, master = false) =>
    mountCombatView({
      enc: player({ reactionWindows: [save()], ...over }),
      master,
      matches: [PHONE_QUERY],
    });

  it('offers the window again once the sheet that opened by itself was closed', async () => {
    const { view, fixture, opened } = mountSave();
    await settle(fixture);
    expect(opened).toHaveLength(1);
    // The sheet is on screen: no second way in.
    expect(view.ownEffectSave()).toBeNull();
    opened[0].close();
    await settle(fixture);
    expect(view.ownEffectSave().id).toBe('e1');
  });

  it('is none for the master, for a combat that is not running, or for a window of another player', async () => {
    expect(mountSave({}, true).view.ownEffectSave()).toBeNull();
    const ended = mountSave({ status: EncounterStatus.ENDED });
    await settle(ended.fixture);
    expect(ended.view.ownEffectSave()).toBeNull();
    const other = mountSave({ reactionWindows: [save({ forYou: false })] });
    await settle(other.fixture);
    expect(other.view.ownEffectSave()).toBeNull();
  });
});

describe('CombatView state: the rule on the death saves', () => {
  const fallen = {
    ...toren,
    id: 'p2',
    mine: false,
    characterId: 'p2-c',
    state: CombatantState.DOWN,
  } as never;
  const hide = {
    get: async () => ({ saved: { deathSaves: DeathSaveVisibility.OWNER_AND_MASTER } }),
  };

  it('hides the marks of the others from a player when the table says so and someone is down', async () => {
    const { view, fixture } = mountCombatView({
      enc: player({ combatants: [toren, fallen] }),
      tableRules: hide,
    });
    await settle(fixture);
    expect(view.deathsHidden()).toBe(true);
    expect(view.deathNote()).toBe(true);
  });

  it('says nothing of it to the master, to a player when only they are down, or when nobody is down', async () => {
    const master = mountCombatView({
      enc: player({ combatants: [toren, fallen] }),
      master: true,
      tableRules: hide,
    });
    await settle(master.fixture);
    expect(master.view.deathsHidden()).toBe(true);
    expect(master.view.deathNote()).toBe(false);
    const mine = mountCombatView({
      enc: player({ combatants: [{ ...toren, state: CombatantState.DOWN } as never, goblin] }),
      tableRules: hide,
    });
    await settle(mine.fixture);
    expect(mine.view.deathNote()).toBe(false);
    const nobody = mountCombatView({ enc: player(), tableRules: hide });
    await settle(nobody.fixture);
    expect(nobody.view.deathNote()).toBe(false);
  });

  it('shows the marks as they always were while the rule is unread or unreadable', async () => {
    const unread = mountCombatView({ enc: player({ combatants: [toren, fallen] }) });
    await settle(unread.fixture);
    expect(unread.view.deathsHidden()).toBe(false);
    const failing = mountCombatView({
      enc: player({ combatants: [toren, fallen] }),
      tableRules: {
        get: async () => {
          throw new ConnectError('x', Code.Unavailable);
        },
      },
    });
    await settle(failing.fixture);
    expect(failing.view.deathsHidden()).toBe(false);
    expect(failing.view.deathNote()).toBe(false);
  });

  it('reads the rule once for the combat, and again when someone falls', async () => {
    const get = vi.fn(async () => ({ saved: {} }));
    const enc = player();
    const { fixture, state } = mountCombatView({ enc, tableRules: { get } });
    await settle(fixture);
    expect(get).toHaveBeenCalledTimes(1);
    state.apply(next(enc));
    await settle(fixture);
    expect(get).toHaveBeenCalledTimes(1);
    state.apply(next(enc, { revision: 3, combatants: [toren, fallen] }));
    await settle(fixture);
    expect(get).toHaveBeenCalledTimes(2);
    expect(get).toHaveBeenLastCalledWith('c');
    state.apply(next(enc, { revision: 4, combatants: [toren, fallen] }));
    await settle(fixture);
    expect(get).toHaveBeenCalledTimes(2);
  });
});
