import { TestBed } from '@angular/core/testing';
import { create, type MessageInitShape } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';
import { vi } from 'vitest';

import {
  type Combatant,
  CombatantKind,
  CombatantSide,
  CombatantState,
  CoverDegree,
  DeathSaveOutcome,
  DeathSaveSchema,
  DiceRollSchema,
  type Encounter,
  EncounterMode,
  EncounterStatus,
  HitRiderChoice,
  type OpportunityOffer,
  OpportunityOfferSchema,
  OpportunityAttackSchema,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { TurnOptionsSchema } from '../../../../gen/meurpg/rules/v1/rules_pb';
import { combatant, encounter } from '../../../core/combat/combat-testing';
import { CombatUndone } from '../../../core/combat/combat-undone';
import { PHONE_QUERY } from '../../../shared/map-view/media-query';
import { RollAnimator } from '../../../shared/roll-overlay/roll-animator';
import { nextTurn, settle } from '../../../../test-settle';
import { StartCombatDialog } from '../../../shared/start-combat/start-combat-dialog';
import { mountCombatView, type MountOptions } from './combat-view-testing';
import { AttackSheet } from './attack-sheet/attack-sheet';
import { OpportunitySheet } from './opportunity/opportunity-sheet';
import { SpendSheet } from './theatre/spend-sheet';

// The calls and flows of CombatView: which client calls each action makes and with what, what the view keeps afterwards.

const toren = (over: Partial<Combatant> = {}) =>
  combatant({
    id: 't',
    label: 'Toren',
    kind: CombatantKind.PLAYER,
    side: CombatantSide.PARTY,
    characterId: 'toren-c',
    mine: true,
    col: 1,
    row: 1,
    ...over,
  });
const goblin = (over: Partial<Combatant> = {}) =>
  combatant({ id: 'g', label: 'Goblin', col: 3, row: 3, ...over });
const grid = (turn: string, ...members: Combatant[]) =>
  encounter({ mode: EncounterMode.GRID, currentCombatantId: turn, combatants: members });
const theatre = (turn: string, ...members: Combatant[]) =>
  encounter({
    mode: EncounterMode.THEATRE,
    mapId: '',
    gridColumns: 0,
    gridRows: 0,
    currentCombatantId: turn,
    combatants: members,
  });

const refuse = (code: Code) => () => {
  throw new ConnectError('refused', code);
};
/** The client methods `names` answer with the combat `e`, as the ones that return an `Encounter` do. */
const answers = (e: Encounter, ...names: string[]): NonNullable<MountOptions['api']> =>
  Object.fromEntries(names.map((n) => [n, () => e]));

describe('CombatView calls: run', () => {
  it('does nothing while another call is in flight (the busy guard)', async () => {
    let release: (e: Encounter) => void = () => undefined;
    const e = grid('g', goblin());
    const { view, callsOf } = mountCombatView({
      enc: e,
      master: true,
      api: {
        begin: () =>
          new Promise<Encounter>((resolve) => {
            release = resolve;
          }),
      },
    });
    const first = view.begin();
    expect(view.busy()).toBe(true);
    expect(await view.begin()).toBe(false);
    expect(callsOf('begin')).toHaveLength(1);
    release(e);
    expect(await first).toBe(true);
    expect(view.busy()).toBe(false);
  });

  it('does nothing when there is no combat', async () => {
    const { view, state, calls } = mountCombatView({ enc: grid('g', goblin()), master: true });
    state.apply(null);
    expect(await view.begin()).toBe(false);
    expect(calls.filter((c) => c.name === 'begin')).toEqual([]);
    expect(view.busy()).toBe(false);
  });

  it('applies the combat the call answers and clears an earlier error', async () => {
    const next = encounter({ round: 5, combatants: [goblin()] });
    const { view, state } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      api: answers(next, 'begin'),
    });
    view.error.set('old');
    expect(await view.begin()).toBe(true);
    expect(state.encounter()?.round).toBe(5);
    expect(view.error()).toBe('');
    expect(view.busy()).toBe(false);
  });

  it('says why a refusal happened, answers false and lets go of the busy flag', async () => {
    const { view } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      api: { begin: refuse(Code.PermissionDenied) },
    });
    expect(await view.begin()).toBe(false);
    expect(view.error()).toBe('Você não pode fazer isso agora.');
    expect(view.busy()).toBe(false);
  });

  it.each<[string, Code]>([
    ['Aborted', Code.Aborted],
    ['FailedPrecondition', Code.FailedPrecondition],
    ['NotFound', Code.NotFound],
    ['Unavailable', Code.Unavailable],
    ['DeadlineExceeded', Code.DeadlineExceeded],
  ])(
    'reads the combat again after a refusal of kind %s (the screen is stale)',
    async (_n, code) => {
      const fresh = encounter({ round: 9, combatants: [goblin()] });
      const { view, state, callsOf } = mountCombatView({
        enc: grid('g', goblin()),
        master: true,
        api: { begin: refuse(code), get: () => fresh },
      });
      expect(await view.begin()).toBe(false);
      expect(callsOf('get')).toEqual([{ name: 'get', args: ['c'] }]);
      expect(state.encounter()?.round).toBe(9);
      expect(view.error()).not.toBe('');
    },
  );

  it.each<[string, Code]>([
    ['InvalidArgument', Code.InvalidArgument],
    ['PermissionDenied', Code.PermissionDenied],
    ['Internal', Code.Internal],
    ['Unknown', Code.Unknown],
    ['ResourceExhausted', Code.ResourceExhausted],
  ])('does not read the combat again after a refusal of kind %s', async (_n, code) => {
    const { view, callsOf } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      api: { begin: refuse(code) },
    });
    await view.begin();
    expect(callsOf('get')).toEqual([]);
  });

  it('does not read the combat again after an error that is not from the server', async () => {
    const { view, callsOf } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      api: {
        begin: () => {
          throw new Error('boom');
        },
      },
    });
    expect(await view.begin()).toBe(false);
    expect(callsOf('get')).toEqual([]);
    expect(view.error()).not.toBe('');
  });

  it('keeps the refusal on screen when the re-read fails too', async () => {
    const { view, callsOf } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      api: { begin: refuse(Code.Aborted), get: refuse(Code.Unavailable) },
    });
    expect(await view.begin()).toBe(false);
    expect(callsOf('get')).toHaveLength(1);
    expect(view.error()).toBe(
      'O combate mudou enquanto você agia. A tela foi atualizada; tente de novo.',
    );
    expect(view.busy()).toBe(false);
  });
});

/** One row per simple action: the view method, its arguments, the client method it calls and the arguments that call gets. */
const OFFER = create(OpportunityOfferSchema, { id: 'o1', moverId: 't', reactorId: 'g' });
type Row = [string, unknown[], string, unknown[]];
const SIMPLE: Row[] = [
  ['begin', [], 'begin', ['c', 'enc']],
  ['endCombat', [], 'end', ['c', 'enc']],
  ['submitFace', ['t', 14], 'submitInitiative', ['c', 'enc', 't', { face: 14 }]],
  ['setOrder', [['g', 't']], 'setOrder', ['c', 'enc', ['g', 't']]],
  ['reveal', [{ id: 'g', hidden: false }], 'setHidden', ['c', 'enc', 'g', false]],
  ['loseConcentration', ['g'], 'setConditions', ['c', 'enc', 'g', { endConcentration: true }]],
  ['remove', ['g'], 'remove', ['c', 'enc', 'g']],
  ['endPart', ['t'], 'endTurn', ['c', 'enc', 't', false, 2]],
  [
    'answerRider',
    [{ id: 't', choice: HitRiderChoice.PRONE }],
    'useHitRider',
    ['c', 'enc', 't', HitRiderChoice.PRONE],
  ],
  ['endRageOf', ['t'], 'endRage', ['c', 'enc', 't']],
  [
    'setSide',
    [{ id: 'g', side: CombatantSide.PARTY }],
    'setSide',
    ['c', 'enc', 'g', CombatantSide.PARTY],
  ],
  [
    'setCover',
    [{ id: 'g', cover: CoverDegree.HALF }],
    'setCover',
    ['c', 'enc', 'g', CoverDegree.HALF],
  ],
  ['skipOpportunity', [OFFER], 'skipOpportunity', ['c', 'enc', 'o1']],
  ['withdrawOpportunity', [OFFER], 'withdrawOpportunity', ['c', 'enc', 'o1']],
  ['standUp', ['g'], 'standUp', ['c', 'enc', 'g']],
  ['answerReveal', [{ id: 'q1', reveal: true }], 'resolveHiddenReveal', ['c', 'enc', 'q1', true]],
];

describe('CombatView calls: one client call per action', () => {
  it.each(SIMPLE)('%s sends its call and applies the answer', async (method, args, name, sent) => {
    const next = encounter({ round: 4, combatants: [goblin(), toren()] });
    const { view, state, calls } = mountCombatView({
      enc: grid('g', goblin(), toren()),
      master: true,
      api: answers(next, name),
    });
    expect(await view[method](...args)).toBe(true);
    expect(calls.filter((c) => c.name === name)).toEqual([{ name, args: sent }]);
    expect(state.encounter()?.round).toBe(4);
  });

  it.each(SIMPLE)('%s answers false and says why when refused', async (method, args, name) => {
    const { view } = mountCombatView({
      enc: grid('g', goblin(), toren()),
      master: true,
      api: { [name]: refuse(Code.PermissionDenied) },
    });
    expect(await view[method](...args)).toBe(false);
    expect(view.error()).toBe('Você não pode fazer isso agora.');
  });

  it('takeActionFor sends the action for the combatant and applies the encounter of the answer', async () => {
    const next = encounter({ round: 6, combatants: [toren()] });
    const { view, state, callsOf } = mountCombatView({
      enc: grid('t', toren()),
      api: { takeAction: () => ({ encounter: next }) },
    });
    expect(await view.takeActionFor('w', 'standard:dash')).toBe(true);
    expect(callsOf('takeAction')[0].args).toEqual(['c', 'enc', 'w', 'standard:dash']);
    expect(state.encounter()?.round).toBe(6);
  });

  it("takeAction is for the player's own combatant, and for nobody when there is none", async () => {
    const withOwn = mountCombatView({ enc: grid('t', toren()) });
    expect(await withOwn.view.takeAction('standard:dodge')).toBe(true);
    expect(withOwn.callsOf('takeAction')[0].args).toEqual(['c', 'enc', 't', 'standard:dodge']);

    const without = mountCombatView({ enc: grid('g', goblin()), master: true });
    expect(await without.view.takeAction('standard:dodge')).toBe(false);
    expect(without.callsOf('takeAction')).toEqual([]);
  });

  it("creatureAction spends the creature's action for the keys that have no sheet of their own", async () => {
    const { view, callsOf, opened } = mountCombatView({ enc: grid('t', toren()) });
    view.creatureAction('w', 'standard:dash');
    await nextTurn();
    expect(callsOf('takeAction')[0].args).toEqual(['c', 'enc', 'w', 'standard:dash']);
    expect(opened).toEqual([]);
  });

  it('creatureAction opens the Help sheet and the Hide sheet for the creature instead of spending the action', () => {
    const { view, callsOf, opened } = mountCombatView({ enc: grid('t', toren()) });
    view.creatureAction('w', 'standard:help');
    expect(opened).toHaveLength(1);
    view.creatureAction('w', 'standard:hide');
    expect(opened).toHaveLength(2);
    expect(callsOf('takeAction')).toEqual([]);
  });
});

describe('CombatView calls: the end of the turn', () => {
  const brisa = toren({
    id: 'b',
    label: 'Brisa',
    mine: false,
    characterId: 'brisa-c',
    state: CombatantState.DOWN,
    deathSaveDue: true,
  });

  it('asks the master first when a death save is due, and sends nothing', async () => {
    const { view, callsOf } = mountCombatView({
      enc: grid('g', goblin(), brisa),
      master: true,
      api: answers(grid('g', goblin(), brisa), 'endTurn'),
    });
    expect(await view.nextTurn()).toBe(false);
    expect(view.passAsking()).toEqual({ discard: false });
    expect(callsOf('endTurn')).toEqual([]);
    expect(await view.nextTurn(true)).toBe(false);
    expect(view.passAsking()).toEqual({ discard: true });
  });

  it('"Passar sem rolar" ends the turn with the discard the master asked for, and puts the question away', async () => {
    const e = grid('g', goblin(), brisa);
    const { view, callsOf } = mountCombatView({ enc: e, master: true, api: answers(e, 'endTurn') });
    await view.nextTurn(true);
    expect(await view.passUnrolled()).toBe(true);
    expect(view.passAsking()).toBeNull();
    expect(callsOf('endTurn')[0].args).toEqual(['c', 'enc', 'g', true, 2]);
  });

  it('"Passar sem rolar" without a question behind it does not discard', async () => {
    const e = grid('g', goblin());
    const { view, callsOf } = mountCombatView({ enc: e, master: true, api: answers(e, 'endTurn') });
    await view.passUnrolled();
    expect(callsOf('endTurn')[0].args).toEqual(['c', 'enc', 'g', false, 2]);
  });

  it("passes the master's turn at once when no save is due, naming who is on turn and the round", async () => {
    const e = grid('g', goblin(), toren());
    const { view, callsOf } = mountCombatView({ enc: e, master: true, api: answers(e, 'endTurn') });
    expect(await view.nextTurn()).toBe(true);
    expect(callsOf('endTurn')[0].args).toEqual(['c', 'enc', 'g', false, 2]);
  });

  it("a player in a joint turn ends their own part, not the first member's", async () => {
    const e = encounter({
      mode: EncounterMode.GRID,
      currentCombatantId: 'g',
      turnGroupIds: ['g', 't'],
      combatants: [goblin(), toren()],
    });
    const { view, callsOf } = mountCombatView({ enc: e, api: answers(e, 'endTurn') });
    await view.nextTurn();
    expect(callsOf('endTurn')[0].args).toEqual(['c', 'enc', 't', false, 2]);
  });

  it('a player whose part already ended (or who is not in the turn) names the current combatant', async () => {
    const ended = encounter({
      mode: EncounterMode.GRID,
      currentCombatantId: 'g',
      turnGroupIds: ['g', 't'],
      combatants: [goblin(), toren({ turnPartEnded: true })],
    });
    const first = mountCombatView({ enc: ended, api: answers(ended, 'endTurn') });
    await first.view.nextTurn();
    expect(first.callsOf('endTurn')[0].args[2]).toBe('g');

    const away = grid('g', goblin(), toren());
    const second = mountCombatView({ enc: away, api: answers(away, 'endTurn') });
    await second.view.nextTurn();
    expect(second.callsOf('endTurn')[0].args[2]).toBe('g');
  });

  it('the master names the current combatant even when a character of his own table acts in the same turn', async () => {
    const e = encounter({
      mode: EncounterMode.GRID,
      currentCombatantId: 'g',
      turnGroupIds: ['g', 't'],
      combatants: [goblin(), toren()],
    });
    const { view, callsOf } = mountCombatView({ enc: e, master: true, api: answers(e, 'endTurn') });
    await view.nextTurn();
    expect(callsOf('endTurn')[0].args[2]).toBe('g');
  });
});

describe('CombatView calls: focusOrder', () => {
  it('puts the focus on the first stop of the order, once the page has rendered', async () => {
    const { fixture, el, view } = mountCombatView({ enc: grid('g', goblin()), master: true });
    const list = el.querySelector('app-order-list');
    expect(list).not.toBeNull();
    view.focusOrder();
    await settle(fixture);
    expect(list?.contains(document.activeElement)).toBe(true);
  });

  it('skips a disabled button and takes the next enabled one', async () => {
    const { fixture, el, view } = mountCombatView({ enc: grid('g', goblin()), master: true });
    el.querySelectorAll('app-order-list').forEach((n) => n.remove());
    const list = document.createElement('app-order-list');
    const disabled = document.createElement('button');
    disabled.disabled = true;
    const enabled = document.createElement('button');
    list.append(disabled, enabled);
    el.append(list);
    view.focusOrder();
    await settle(fixture);
    expect(document.activeElement).toBe(enabled);
  });

  it('is quiet when the order has no stop', async () => {
    const { fixture, el, view } = mountCombatView({ enc: grid('g', goblin()), master: true });
    el.querySelectorAll('app-order-list').forEach((n) => n.remove());
    view.focusOrder();
    await settle(fixture);
    expect(view.error()).toBe('');
  });
});

describe('CombatView calls: leaving a Wild Shape form', () => {
  const druid = toren({ wildShapeBeastKey: 'wolf' });
  function mountDruid(
    leave: (...args: unknown[]) => unknown,
    api: MountOptions['api'] = undefined,
    enc = grid('t', druid),
  ) {
    const sent: unknown[][] = [];
    const mounted = mountCombatView({
      enc,
      api,
      creatures: {
        leaveWildShape: (...args: unknown[]) => {
          sent.push(args);
          return leave(...args);
        },
      },
    });
    return { ...mounted, sent };
  }

  it('sends the character and applies the encounter of the answer', async () => {
    const next = encounter({ round: 7, combatants: [toren()] });
    const { view, state, sent } = mountDruid(() => ({ encounter: next }));
    await view.leaveForm();
    expect(sent).toHaveLength(1);
    expect(sent[0].slice(0, 2)).toEqual(['c', 'toren-c']);
    expect(state.encounter()?.round).toBe(7);
    expect(view.busy()).toBe(false);
  });

  it('keeps the combat as it is when the answer carries none', async () => {
    const { view, state } = mountDruid(() => ({}));
    await view.leaveForm();
    expect(state.encounter()?.round).toBe(2);
  });

  it('keeps its key for the retry of a refused try and takes a new one after success', async () => {
    let n = 0;
    const { view, sent } = mountDruid(() => {
      n += 1;
      if (n === 1) {
        throw new ConnectError('refused', Code.PermissionDenied);
      }
      return {};
    });
    await view.leaveForm();
    await view.leaveForm();
    await view.leaveForm();
    expect(sent[1][2]).toBe(sent[0][2]);
    expect(sent[2][2]).not.toBe(sent[1][2]);
  });

  it('says why when refused, and reads the combat again when it is stale', async () => {
    const { view, callsOf } = mountDruid(refuse(Code.InvalidArgument));
    await view.leaveForm();
    expect(view.error()).toBe(
      'Não deu para voltar à forma normal: confira os campos e tente de novo.',
    );
    expect(callsOf('get')).toEqual([]);

    const stale = mountDruid(refuse(Code.Aborted), { get: () => grid('t', druid) });
    await stale.view.leaveForm();
    expect(stale.callsOf('get')).toHaveLength(1);
  });

  it('does nothing without a character of the player, or while a call is in flight', async () => {
    const none = mountDruid(() => ({}), undefined, grid('g', goblin()));
    await none.view.leaveForm();
    expect(none.sent).toEqual([]);

    const busy = mountDruid(() => ({}));
    busy.view.busy.set(true);
    await busy.view.leaveForm();
    expect(busy.sent).toEqual([]);
  });
});

describe('CombatView calls: death saves', () => {
  const down = () => toren({ state: CombatantState.DOWN, deathSaveDue: true });
  const save = (
    outcome: DeathSaveOutcome,
    face: number | null = 14,
    over: { failures?: number; stable?: boolean } = {},
  ) =>
    create(DeathSaveSchema, {
      outcome,
      successes: 1,
      ...over,
      roll:
        face === null
          ? undefined
          : create(DiceRollSchema, {
              diceCount: 1,
              diceSides: 20,
              faces: [face],
              modifier: 0,
              total: face,
              countedIndex: 0,
            }),
    });
  function mountDying(saves: (call: number) => unknown, master = false) {
    const keys: string[] = [];
    let n = 0;
    const e = grid('t', down(), goblin());
    const mounted = mountCombatView({
      enc: e,
      master,
      api: {
        rollDeathSave: (...args: unknown[]) => {
          keys.push(args[4] as string);
          n += 1;
          return saves(n);
        },
      },
    });
    const play = vi.spyOn(TestBed.inject(RollAnimator), 'play');
    return { ...mounted, keys, play };
  }

  it('rolls the die for the character, keeps the sentence and plays the dice', async () => {
    const { view, callsOf, play } = mountDying(() => ({
      encounter: grid('t', down(), goblin()),
      save: save(DeathSaveOutcome.SUCCESS),
    }));
    await view.rollDeathSave({ inApp: true });
    expect(callsOf('rollDeathSave')[0].args.slice(0, 4)).toEqual([
      'c',
      'enc',
      't',
      { inApp: true },
    ]);
    expect(view.deathResult()).toBe('Teste contra a morte: 1d20 (14) = 14. Sucesso.');
    expect(play).toHaveBeenCalledTimes(1);
    expect(play.mock.calls[0][0]).toMatchObject({
      label: 'Teste contra a morte',
      outcome: { word: 'Sucesso', good: true },
      critical: false,
      fumble: false,
    });
    expect(view.actionNote()).toBe('');
  });

  it('marks a natural 1 as a fumble and a failure as not good', async () => {
    const { view, play } = mountDying(() => ({
      encounter: grid('t', down(), goblin()),
      save: save(DeathSaveOutcome.CRITICAL_FAILURE, 1, { failures: 2 }),
    }));
    await view.rollDeathSave({ inApp: true });
    expect(play.mock.calls[0][0]).toMatchObject({
      outcome: { word: 'Falha crítica', good: false },
      fumble: true,
      critical: false,
    });
    expect(view.deathResult()).toContain('Falha: um 1 conta duas falhas.');
  });

  it('a natural 20 brings the character back: critical, good, and the sentence is also the action note', async () => {
    const { view, play } = mountDying(() => ({
      encounter: grid('t', toren(), goblin()),
      save: save(DeathSaveOutcome.REVIVED, 20),
    }));
    await view.rollDeathSave({ inApp: true });
    expect(play.mock.calls[0][0]).toMatchObject({
      outcome: { word: 'Volta com 1 PV', good: true },
      critical: true,
    });
    expect(view.actionNote()).toBe('Teste contra a morte: 1d20 (20) = 20. Toren volta com 1 PV.');
    expect(view.deathResult()).toBe(view.actionNote());
  });

  it('plays nothing for a save that arrives without a roll (hidden), but still says the outcome', async () => {
    const { view, play } = mountDying(() => ({
      encounter: grid('t', down(), goblin()),
      save: save(DeathSaveOutcome.FAILURE, null, { failures: 3 }),
    }));
    await view.rollDeathSave({ inApp: true });
    expect(play).not.toHaveBeenCalled();
    expect(view.deathResult()).toBe('Teste contra a morte. Falha. Três falhas.');
  });

  it('keeps the same key for the retry of a refused roll and a new one after a roll that worked', async () => {
    const { view, keys } = mountDying((n) => {
      if (n === 1) {
        throw new ConnectError('lost', Code.PermissionDenied);
      }
      return { encounter: grid('t', down(), goblin()), save: save(DeathSaveOutcome.SUCCESS) };
    });
    await view.rollDeathSave({ inApp: true });
    expect(view.deathResult()).toBe('');
    expect(view.error()).not.toBe('');
    await view.rollDeathSave({ inApp: true });
    await view.rollDeathSave({ inApp: true });
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[1]);
  });

  it("a typed die is a different request from the app's roll, so it takes another key", async () => {
    const { view, keys } = mountDying(() => {
      throw new ConnectError('lost', Code.PermissionDenied);
    });
    await view.rollDeathSave({ inApp: true });
    await view.rollDeathSave({ face: 12 });
    expect(keys[1]).not.toBe(keys[0]);
  });

  it('nothing happens for a player without a character in the combat', async () => {
    const { view, callsOf } = mountCombatView({ enc: grid('g', goblin()) });
    await view.rollDeathSave({ inApp: true });
    expect(callsOf('rollDeathSave')).toEqual([]);
  });

  it('the master rolls for an absent player: puts the question away and keeps the sentence', async () => {
    const { view, callsOf } = mountDying(
      () => ({ encounter: grid('t', down(), goblin()), save: save(DeathSaveOutcome.SUCCESS) }),
      true,
    );
    view.passAsking.set({ discard: false });
    await view.rollMasterDeathSave({ id: 't', die: { face: 9 } });
    expect(view.passAsking()).toBeNull();
    expect(callsOf('rollDeathSave')[0].args.slice(0, 4)).toEqual(['c', 'enc', 't', { face: 9 }]);
    expect(view.masterDeathResult()).toBe('Teste contra a morte: 1d20 (14) = 14. Sucesso.');
    // The master's roll does not touch the player's line.
    expect(view.deathResult()).toBe('');
  });

  it("the master's roll for a combatant that is not in the combat sends nothing and keeps the question", async () => {
    const { view, callsOf } = mountDying(() => ({}), true);
    view.passAsking.set({ discard: true });
    await view.rollMasterDeathSave({ id: 'nobody', die: { inApp: true } });
    expect(callsOf('rollDeathSave')).toEqual([]);
    expect(view.passAsking()).toEqual({ discard: true });
  });

  it("leaves the master's sentence alone when his roll was refused", async () => {
    const { view } = mountDying(() => {
      throw new ConnectError('no', Code.PermissionDenied);
    }, true);
    view.masterDeathResult.set('before');
    await view.rollMasterDeathSave({ id: 't', die: { inApp: true } });
    expect(view.masterDeathResult()).toBe('before');
  });
});

describe('CombatView calls: confirming a death and the "Ainda não" memory', () => {
  it('confirmDeath keeps its key for the retry of a refusal and takes a new one after it worked', async () => {
    const keys: string[] = [];
    let n = 0;
    const e = grid('g', goblin(), toren({ state: CombatantState.DYING }));
    const { view, callsOf } = mountCombatView({
      enc: e,
      master: true,
      api: {
        confirmDeath: (...args: unknown[]) => {
          keys.push(args[3] as string);
          n += 1;
          if (n === 1) {
            throw new ConnectError('lost', Code.PermissionDenied);
          }
          return e;
        },
      },
    });
    await view.confirmDeath('t');
    await view.confirmDeath('t');
    await view.confirmDeath('t');
    expect(callsOf('confirmDeath')[0].args.slice(0, 3)).toEqual(['c', 'enc', 't']);
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[1]);
  });

  it('remembers the characters put away with "Ainda não" until they are asked again', () => {
    const { view } = mountCombatView({ enc: grid('g', goblin()), master: true });
    view.deathLaterFor('a');
    view.deathLaterFor('b');
    expect([...view.deathLater()]).toEqual(['a', 'b']);
    view.deathAskAgain('a');
    expect([...view.deathLater()]).toEqual(['b']);
    view.deathAskAgain('zzz');
    expect([...view.deathLater()]).toEqual(['b']);
  });
});

describe('CombatView calls: ending the concentration', () => {
  it('sends the end of concentration for the character and keeps the "ended by me" mark', async () => {
    const { view, callsOf } = mountCombatView({
      enc: grid('t', toren()),
      api: answers(grid('t', toren()), 'setConditions'),
    });
    expect(await view.endOwnConcentration()).toBe(true);
    expect(callsOf('setConditions')[0].args).toEqual(['c', 'enc', 't', { endConcentration: true }]);
    expect(view.concentration.endedByMe).toBe(true);
  });

  it('takes the mark back when the server refuses', async () => {
    const { view } = mountCombatView({
      enc: grid('t', toren()),
      api: { setConditions: refuse(Code.PermissionDenied) },
    });
    expect(await view.endOwnConcentration()).toBe(false);
    expect(view.concentration.endedByMe).toBe(false);
  });

  it('without a character of the player it sends nothing (the mark is still set)', async () => {
    const { view, callsOf } = mountCombatView({ enc: grid('g', goblin()), master: true });
    expect(await view.endOwnConcentration()).toBe(false);
    expect(callsOf('setConditions')).toEqual([]);
    // Pinned as it is today: the mark is set before the character is looked up.
    expect(view.concentration.endedByMe).toBe(true);
  });
});

describe('CombatView calls: endTab', () => {
  const wolf = (id: string, over: Partial<Combatant> = {}) =>
    combatant({
      id,
      label: `Lobo ${id}`,
      kind: CombatantKind.NPC,
      side: CombatantSide.PARTY,
      controlledByMe: true,
      summonGroupId: 'grp',
      monsterKey: 'monster:wolf',
      ...over,
    });
  const together = (...wolves: Combatant[]) =>
    encounter({
      mode: EncounterMode.GRID,
      currentCombatantId: wolves[0].id,
      turnGroupIds: wolves.map((w) => w.id),
      combatants: [toren(), ...wolves],
    });
  // The page draws the creatures' cards, which read the book's armor class.
  const mountTab = (enc: Encounter, api: MountOptions['api'] = {}) =>
    mountCombatView({ enc, api, creatures: { statBlock: async () => ({ armorClass: 12 }) } });
  const tabOf = (...members: Combatant[]) => ({
    id: 'grp',
    members,
    creature: true,
    label: 'Lobos',
    state: 'turn',
    acting: members,
  });

  it('ends the part of each member, one call each, in order', async () => {
    const [a, b] = [wolf('w1'), wolf('w2')];
    const e = together(a, b);
    const { view, callsOf } = mountTab(e, answers(e, 'endTurn'));
    await view.endTab(tabOf(a, b));
    expect(callsOf('endTurn').map((c) => c.args)).toEqual([
      ['c', 'enc', 'w1', false, 2],
      ['c', 'enc', 'w2', false, 2],
    ]);
  });

  it('reads the combat again before each call: a member whose part the answer already ended is skipped', async () => {
    const [a, b] = [wolf('w1'), wolf('w2')];
    const after = together(a, wolf('w2', { turnPartEnded: true }));
    const { view, callsOf } = mountTab(together(a, b), answers(after, 'endTurn'));
    await view.endTab(tabOf(a, b));
    expect(callsOf('endTurn').map((c) => c.args[2])).toEqual(['w1']);
  });

  it('skips a member that does not act now', async () => {
    const [a, b] = [wolf('w1', { turnPartEnded: true }), wolf('w2')];
    const e = together(a, b);
    const { view, callsOf } = mountTab(e, answers(e, 'endTurn'));
    await view.endTab(tabOf(a, b));
    expect(callsOf('endTurn').map((c) => c.args[2])).toEqual(['w2']);
  });

  it('stops at the first refusal and says why', async () => {
    const [a, b] = [wolf('w1'), wolf('w2')];
    const { view, callsOf } = mountTab(together(a, b), { endTurn: refuse(Code.PermissionDenied) });
    await view.endTab(tabOf(a, b));
    expect(callsOf('endTurn')).toHaveLength(1);
    expect(view.error()).toBe('Você não pode fazer isso agora.');
  });

  it('does nothing when the combat is gone', async () => {
    const [a, b] = [wolf('w1'), wolf('w2')];
    const { view, state, callsOf } = mountTab(together(a, b));
    state.apply(null);
    await view.endTab(tabOf(a, b));
    expect(callsOf('endTurn')).toEqual([]);
  });
});

describe('CombatView calls: small signals', () => {
  it("settled keeps the turn whose damage was settled, for the master's card", () => {
    const { view } = mountCombatView({ enc: grid('g', goblin()), master: true });
    expect(view.settledTurn()).toBe('');
    view.settled();
    expect(view.settledTurn()).toBe('g');
  });

  it('settled keeps nothing when nobody is on turn', () => {
    const { view } = mountCombatView({ enc: grid('', goblin()), master: true });
    view.settled();
    expect(view.settledTurn()).toBe('');
  });

  it('adjustFor emits the vitals row of the character, and nothing for one without a row', () => {
    const row = { characterId: 'toren-c' };
    const { fixture, view } = mountCombatView({
      enc: grid('t', toren()),
      inputs: { vitals: [row] },
    });
    const emitted: unknown[] = [];
    fixture.componentInstance.adjust.subscribe((v: unknown) => emitted.push(v));
    view.adjustFor('toren-c');
    view.adjustFor('other');
    expect(emitted).toEqual([row]);
  });

  it('leave dismisses the summary of an ended combat', () => {
    const ended = encounter({ status: EncounterStatus.ENDED, combatants: [goblin()] });
    const { view } = mountCombatView({ enc: ended });
    expect(view.encounter()).not.toBeNull();
    view.leave();
    expect(view.encounter()).toBeNull();
  });
});

describe('CombatView calls: initiative rolled in the app', () => {
  const rolled = (over: Partial<Combatant>) =>
    grid('g', goblin(), toren({ initiative: 19, initiativeFace: 15, initiativeBonus: 4, ...over }));
  function mountRoll(answer: Encounter) {
    const mounted = mountCombatView({
      enc: grid('g', goblin(), toren()),
      api: answers(answer, 'submitInitiative'),
    });
    const play = vi.spyOn(TestBed.inject(RollAnimator), 'play');
    return { ...mounted, play };
  }

  it('sends the app\'s roll and shows the d20 with its line "15 + 4 = 19"', async () => {
    const { view, play, callsOf } = mountRoll(rolled({}));
    expect(await view.rollInApp('t')).toBe(true);
    expect(callsOf('submitInitiative')[0].args).toEqual(['c', 'enc', 't', { inApp: true }]);
    expect(play).toHaveBeenCalledTimes(1);
    expect(play.mock.calls[0][0]).toEqual({
      label: 'Iniciativa',
      dice: [{ sides: 20, face: 15 }],
      line: '15 + 4 = 19',
    });
  });

  it('writes a negative modifier with a minus sign', async () => {
    const { view, play } = mountRoll(rolled({ initiative: 13, initiativeBonus: -2 }));
    await view.rollInApp('t');
    expect(play.mock.calls[0][0].line).toBe('15 − 2 = 13');
  });

  it('writes no modifier at all when it is zero', async () => {
    const { view, play } = mountRoll(rolled({ initiative: 15, initiativeBonus: 0 }));
    await view.rollInApp('t');
    expect(play.mock.calls[0][0].line).toBe('15 = 15');
  });

  it.each<[string, Partial<Combatant>]>([
    ['the face', { initiativeFace: undefined }],
    ['the bonus', { initiativeBonus: undefined }],
    ['the total', { initiative: undefined }],
  ])('plays nothing when %s is missing', async (_n, over) => {
    const { view, play } = mountRoll(rolled(over));
    expect(await view.rollInApp('t')).toBe(true);
    expect(play).not.toHaveBeenCalled();
  });

  it('plays nothing for a combatant the answer does not know', async () => {
    const { view, play } = mountRoll(rolled({}));
    await view.rollInApp('nobody');
    expect(play).not.toHaveBeenCalled();
  });

  it('plays nothing when the roll was refused', async () => {
    const mounted = mountCombatView({
      enc: grid('g', goblin(), toren()),
      api: { submitInitiative: refuse(Code.PermissionDenied) },
    });
    const play = vi.spyOn(TestBed.inject(RollAnimator), 'play');
    expect(await mounted.view.rollInApp('t')).toBe(false);
    expect(play).not.toHaveBeenCalled();
  });
});

describe('CombatView calls: adding combatants', () => {
  it('opens the start dialog for the NPCs of this combat, as a floating dialog on a desktop', () => {
    const { view, opened } = mountCombatView({
      enc: grid('g', goblin(), toren()),
      master: true,
    });
    view.addCombatants();
    expect(opened).toHaveLength(1);
    expect(opened[0].component).toBe(StartCombatDialog);
    expect(opened[0].config).toEqual({
      data: { campaignId: 'c', mode: 'add', map: null, encounterId: 'enc', existing: 2 },
      width: '760px',
      maxWidth: 'calc(100vw - 32px)',
      height: undefined,
      maxHeight: '92dvh',
      ariaLabelledBy: 'start-title',
      autoFocus: 'first-tabbable',
    });
  });

  it('fills the screen on a phone', () => {
    const { view, opened } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      matches: [PHONE_QUERY],
    });
    view.addCombatants();
    expect(opened[0].config).toMatchObject({
      width: '100vw',
      maxWidth: '100vw',
      height: '100dvh',
      maxHeight: '100dvh',
    });
  });

  it('applies the combat the dialog answers, and leaves it alone when closed without one', () => {
    const { view, opened, state } = mountCombatView({ enc: grid('g', goblin()), master: true });
    view.addCombatants();
    opened[0].close();
    expect(state.encounter()?.round).toBe(2);
    view.addCombatants();
    opened[1].close(encounter({ round: 8, combatants: [goblin(), goblin({ id: 'g2' })] }));
    expect(state.encounter()?.round).toBe(8);
  });

  it('opens nothing without a combat', () => {
    const { view, state, opened } = mountCombatView({ enc: grid('g', goblin()), master: true });
    state.apply(null);
    view.addCombatants();
    expect(opened).toEqual([]);
  });
});

describe('CombatView calls: opportunity attacks', () => {
  const scimitar = (namePt = 'Cimitarra') =>
    create(TurnOptionsSchema, {
      attacks: [{ attack: { key: 'scimitar', name: 'Scimitar', namePt } }],
    });
  const offer = (over: MessageInitShape<typeof OpportunityOfferSchema> = {}) =>
    create(OpportunityOfferSchema, {
      id: 'o1',
      moverId: 't',
      moverLabel: 'Toren',
      reactorId: 'g',
      reactorLabel: 'Goblin',
      forYou: true,
      attacks: [create(OpportunityAttackSchema, { key: 'scimitar', namePt: 'Cimitarra' })],
      ...over,
    });
  const withOffer = (o = offer()) => ({
    ...grid('t', toren(), goblin()),
    opportunityOffers: [o],
  });
  const reactorAnswer = (namePt?: string) => ({
    options: scimitar(namePt),
    attackTargets: [],
    spellTargets: [],
    pendingDamages: [],
    resourceTargets: [],
  });

  it('opens its prompt by itself for a player who answers, with the data the sheet needs', () => {
    const { opened, state } = mountCombatView({ enc: withOffer() });
    expect(opened).toHaveLength(1);
    expect(opened[0].component).toBe(OpportunitySheet);
    expect(opened[0].config).toMatchObject({ role: 'alertdialog', disableClose: true });
    expect(opened[0].config.data).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      round: 2,
      state,
    });
    expect((opened[0].config.data['offer'] as OpportunityOffer).id).toBe('o1');
  });

  it("the sheet reads the reactor's options itself, with its attacks matched to the offer's", async () => {
    const { opened, callsOf } = mountCombatView({
      enc: withOffer(),
      api: { turnOptions: () => reactorAnswer() },
    });
    const load = opened[0].config.data['load'] as () => Promise<{
      options: unknown;
      attacks: { key: string; name: string; attack: { key: string } | null }[];
    }>;
    const read = await load();
    expect(callsOf('turnOptions').at(-1)?.args).toEqual(['c', 'enc', 'g']);
    expect(read.attacks).toHaveLength(1);
    expect(read.attacks[0]).toMatchObject({ key: 'scimitar', name: 'Cimitarra' });
    expect(read.attacks[0].attack?.key).toBe('scimitar');
  });

  it('an options answer without attacks leaves each offered attack without its numbers', async () => {
    const { opened } = mountCombatView({
      enc: withOffer(),
      api: { turnOptions: () => ({ attackTargets: [] }) },
    });
    const load = opened[0].config.data['load'] as () => Promise<{
      attacks: { attack: unknown }[];
    }>;
    expect((await load()).attacks.map((a) => a.attack)).toEqual([null]);
  });

  it('an attack option without an attack is left out when matching', async () => {
    const { opened } = mountCombatView({
      enc: withOffer(),
      api: {
        turnOptions: () => ({ options: create(TurnOptionsSchema, { attacks: [{}] }) }),
      },
    });
    const load = opened[0].config.data['load'] as () => Promise<{
      attacks: { attack: unknown }[];
    }>;
    expect((await load()).attacks.map((a) => a.attack)).toEqual([null]);
  });

  it('the chosen attack opens the attack sheet as a reaction against the mover, named after the weapon', () => {
    const { opened, state } = mountCombatView({ enc: withOffer(), matches: [PHONE_QUERY] });
    opened[0].close({ attackKey: 'scimitar', options: reactorAnswer() });
    expect(opened).toHaveLength(2);
    expect(opened[1].component).toBe(AttackSheet);
    expect(opened[1].config.ariaLabel).toBe('Ataque de oportunidade com Cimitarra');
    expect(opened[1].config.data).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 'g',
      round: 2,
      targets: [],
      state,
      asReaction: true,
      opportunity: { offerId: 'o1', targetId: 't', targetLabel: 'Toren', byMaster: false },
    });
    expect((opened[1].config.data['attack'] as { key: string }).key).toBe('scimitar');
  });

  it("falls back to the weapon's English name when it has no Portuguese one", () => {
    const { opened } = mountCombatView({ enc: withOffer(), matches: [PHONE_QUERY] });
    opened[0].close({ attackKey: 'scimitar', options: reactorAnswer('') });
    expect(opened[1].config.ariaLabel).toBe('Ataque de oportunidade com Scimitar');
  });

  it('opens no attack sheet for an attack the options do not have, or when the sheet closes unanswered', () => {
    const { opened } = mountCombatView({ enc: withOffer() });
    opened[0].close({ attackKey: 'unknown', options: reactorAnswer() });
    expect(opened).toHaveLength(1);

    const second = mountCombatView({ enc: withOffer() });
    second.opened[0].close({ attackKey: 'scimitar', options: null });
    expect(second.opened).toHaveLength(1);

    const third = mountCombatView({ enc: withOffer() });
    third.opened[0].close();
    expect(third.opened).toHaveLength(1);
  });

  it('asks again when the attack sheet closed and the offer is still waiting for the player', () => {
    const { opened } = mountCombatView({ enc: withOffer() });
    opened[0].close({ attackKey: 'scimitar', options: reactorAnswer() });
    opened[1].close(true);
    expect(opened).toHaveLength(3);
    expect(opened[2].component).toBe(OpportunitySheet);
  });

  it('does not ask again once the offer was answered or taken back', () => {
    const { opened, state } = mountCombatView({ enc: withOffer() });
    opened[0].close({ attackKey: 'scimitar', options: reactorAnswer() });
    state.apply({ ...grid('t', toren(), goblin()), opportunityOffers: [] } as Encounter);
    opened[1].close(true);
    expect(opened).toHaveLength(2);
  });

  it('does not ask again when the offer is no longer for this player', () => {
    const { opened, state } = mountCombatView({ enc: withOffer() });
    opened[0].close({ attackKey: 'scimitar', options: reactorAnswer() });
    state.apply(withOffer(offer({ forYou: false })));
    opened[1].close(true);
    expect(opened).toHaveLength(2);
  });

  describe('the master answers for an NPC', () => {
    const masterOffer = () =>
      mountCombatView({
        enc: withOffer(),
        master: true,
        api: { turnOptions: () => reactorAnswer() },
      });

    it('"Não atacar" declines the offer', async () => {
      const { view, callsOf } = mountCombatView({
        enc: withOffer(),
        master: true,
        api: answers(grid('g', goblin()), 'declineOpportunity'),
      });
      const result = await view.answerOpportunity({ offer: offer(), attack: null });
      expect(result).toBe(true);
      expect(callsOf('declineOpportunity')[0].args).toEqual(['c', 'enc', 'o1']);
    });

    it('"Atacar com…" opens the attack sheet from the options read for that reactor, as the master\'s', async () => {
      const { view, opened, callsOf } = masterOffer();
      // Only the reactor's read has to land: rendering the NPC's card is not needed (and has no stat block here).
      await nextTurn();
      expect(callsOf('turnOptions').some((c) => c.args[2] === 'g')).toBe(true);
      expect(
        view.answerOpportunity({ offer: offer(), attack: { key: 'scimitar' } }),
      ).toBeUndefined();
      expect(opened).toHaveLength(1);
      expect(opened[0].component).toBe(AttackSheet);
      expect(opened[0].config.data).toMatchObject({
        attackerId: 'g',
        opportunity: { offerId: 'o1', targetId: 't', byMaster: true },
      });
    });

    it("does not ask again after the attack sheet closes: it is not the master's prompt", async () => {
      const { view, opened } = masterOffer();
      await nextTurn();
      view.answerOpportunity({ offer: offer(), attack: { key: 'scimitar' } });
      opened[0].close(true);
      expect(opened).toHaveLength(1);
    });

    it("opens nothing while the reactor's options are not in", () => {
      const { view, opened } = mountCombatView({ enc: withOffer(), master: true });
      view.reactorOptions.set(new Map());
      view.answerOpportunity({ offer: offer(), attack: { key: 'scimitar' } });
      expect(opened).toEqual([]);
    });
  });

  it('openOpportunity does nothing without a combat', () => {
    const { view, state, opened } = mountCombatView({ enc: withOffer(), master: true });
    state.apply(null);
    view.openOpportunity(offer());
    expect(opened).toEqual([]);
  });
});

describe('CombatView calls: undoing the move', () => {
  const entry = { id: 'ev1', undoable: true, kind: 0 };
  const log = { rounds: [{ round: 2, entries: [entry] }], undoableEventId: 'ev1' };

  it('takes back the last undoable entry and counts the undo', async () => {
    const e = grid('g', goblin());
    const { view, fixture, callsOf } = mountCombatView({
      enc: e,
      master: true,
      api: { log: () => log, undo: () => e },
    });
    await settle(fixture);
    const undone = TestBed.inject(CombatUndone);
    expect(await view.undoMove()).toBe(true);
    expect(callsOf('undo')[0].args).toEqual(['c', 'enc', 'ev1']);
    expect(undone.count()).toBe(1);
  });

  it('counts nothing when the server refuses', async () => {
    const { view, fixture } = mountCombatView({
      enc: grid('g', goblin()),
      master: true,
      api: { log: () => log, undo: refuse(Code.PermissionDenied) },
    });
    await settle(fixture);
    expect(await view.undoMove()).toBe(false);
    expect(TestBed.inject(CombatUndone).count()).toBe(0);
    expect(view.error()).toBe('Você não pode fazer isso agora.');
  });

  it('answers false and calls nothing when there is nothing to undo', async () => {
    const { view, callsOf } = mountCombatView({ enc: grid('g', goblin()), master: true });
    expect(await view.undoMove()).toBe(false);
    expect(callsOf('undo')).toEqual([]);
  });
});

describe('CombatView calls: offering an opportunity attack by hand', () => {
  const mount = (api: MountOptions['api'] = {}) =>
    mountCombatView({
      enc: theatre('t', toren(), goblin()),
      master: true,
      api: { offerOpportunity: () => theatre('t', toren(), goblin()), ...api },
    });

  it("sends the offer of the one on turn leaving the reactor's reach, and closes the form", async () => {
    const { view, callsOf } = mount();
    view.toggleOffer();
    expect(view.offering()).toBe(true);
    await view.makeOffer('g');
    const args = callsOf('offerOpportunity')[0].args;
    expect(args.slice(0, 4)).toEqual(['c', 'enc', 't', 'g']);
    expect(typeof args[4]).toBe('string');
    expect(view.offering()).toBe(false);
  });

  it('a repeated tap on the same reactor is the same request (same key)', async () => {
    const { view, callsOf } = mount();
    view.toggleOffer();
    await view.makeOffer('g');
    await view.makeOffer('g');
    const [first, second] = callsOf('offerOpportunity');
    expect(second.args[4]).toBe(first.args[4]);
  });

  it('another reactor is another request (another key)', async () => {
    const { view, callsOf } = mount();
    await view.makeOffer('g');
    await view.makeOffer('h');
    const [first, second] = callsOf('offerOpportunity');
    expect(second.args[4]).not.toBe(first.args[4]);
  });

  it('a refusal stays in the form, not on the page, and the next try is a new request', async () => {
    let refused = true;
    const { view, callsOf } = mount({
      offerOpportunity: () => {
        if (refused) {
          throw new ConnectError('no', Code.PermissionDenied);
        }
        return theatre('t', toren(), goblin());
      },
    });
    view.toggleOffer();
    await view.makeOffer('g');
    expect(view.offerError()).toBe('Você não pode fazer isso agora.');
    expect(view.error()).toBe('');
    expect(view.offering()).toBe(true);
    refused = false;
    await view.makeOffer('g');
    const [first, second] = callsOf('offerOpportunity');
    expect(second.args[4]).not.toBe(first.args[4]);
    expect(view.offering()).toBe(false);
  });

  it('a tap while another call is in flight sends nothing and keeps the key for the next one', async () => {
    const { view, callsOf } = mount();
    view.busy.set(true);
    await view.makeOffer('g');
    expect(callsOf('offerOpportunity')).toEqual([]);
    expect(view.offerError()).toBe('');
    expect(view.offerKeys.size).toBe(1);
  });

  it('clears the last refusal when it tries again', async () => {
    const { view } = mount();
    view.offerError.set('old');
    await view.makeOffer('g');
    expect(view.offerError()).toBe('');
  });

  it('does nothing when nobody is on turn', async () => {
    const { view, callsOf } = mountCombatView({
      enc: theatre('', toren(), goblin()),
      master: true,
    });
    await view.makeOffer('g');
    expect(callsOf('offerOpportunity')).toEqual([]);
  });

  it('opening the form starts with new keys and no old refusal; closing it keeps them', async () => {
    const { view } = mount();
    view.busy.set(true);
    await view.makeOffer('g');
    expect(view.offerKeys.size).toBe(1);
    view.offerError.set('old');
    view.toggleOffer();
    expect(view.offering()).toBe(true);
    expect(view.offerKeys.size).toBe(0);
    expect(view.offerError()).toBe('');
    await view.makeOffer('g');
    expect(view.offerKeys.size).toBe(1);
    view.toggleOffer();
    expect(view.offering()).toBe(false);
    expect(view.offerKeys.size).toBe(1);
  });
});

describe("CombatView calls: moving a token (the master's drag and a player's drop)", () => {
  const moved = (col: number, row: number) => grid('g', toren(), goblin({ col, row }));
  const spot = (view: { encounter: () => Encounter | null }, id: string) => {
    const c = view.encounter()?.combatants.find((x) => x.id === id);
    return [c?.col, c?.row];
  };
  const moveAnswer =
    (e: Encounter, over: Record<string, unknown> = {}) =>
    () => ({
      encounter: e,
      stoppedEarly: false,
      lockedDoor: false,
      provoked: false,
      ...over,
    });

  it('a player\'s drop opens the "Mover" page on that square and never moves by itself', async () => {
    const { view, state, callsOf } = mountCombatView({ enc: grid('t', toren(), goblin()) });
    view.moveError.set('old');
    await view.drop({ id: 't', col: 4, row: 4 });
    expect(view.dropStart()).toEqual({ col: 4, row: 4 });
    expect(state.moving()).toBe(true);
    expect(view.moveError()).toBe('');
    expect(callsOf('move')).toEqual([]);
  });

  it('ignores a drop of an unknown token and a drop on the square the token already holds', async () => {
    const { view, state, callsOf } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
    });
    await view.drop({ id: 'nobody', col: 5, row: 5 });
    await view.drop({ id: 'g', col: 3, row: 3 });
    expect(callsOf('move')).toEqual([]);
    expect(state.moving()).toBe(false);
  });

  it('a token that is not on the map yet can be dropped on any square, even 0,0', async () => {
    const e = grid('g', toren(), goblin({ placed: false, col: 0, row: 0 }));
    const { view, callsOf } = mountCombatView({
      enc: e,
      master: true,
      api: { move: moveAnswer(e) },
    });
    await view.drop({ id: 'g', col: 0, row: 0 });
    expect(callsOf('move')[0].args.slice(0, 5)).toEqual(['c', 'enc', 'g', 0, 0]);
  });

  it('shows the token on the new square at once and sends the move; the answer is the truth', async () => {
    let finish: (() => void) | undefined;
    const answer = moved(5, 5);
    const { view, callsOf } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: {
        move: () =>
          new Promise((resolve) => {
            finish = () => resolve(moveAnswer(answer)());
          }),
      },
    });
    view.error.set('old');
    const dropping = view.drop({ id: 'g', col: 5, row: 5 });
    expect(spot(view, 'g')).toEqual([5, 5]);
    expect(view.error()).toBe('');
    expect(callsOf('move')[0].args).toEqual(['c', 'enc', 'g', 5, 5, undefined, undefined, false]);
    finish?.();
    await dropping;
    expect(spot(view, 'g')).toEqual([5, 5]);
  });

  it('a second drop of the same token waits for the first save and then sends the last square only', async () => {
    const finish: (() => void)[] = [];
    const { view, callsOf } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: {
        move: () =>
          new Promise((resolve) => {
            finish.push(() => resolve(moveAnswer(moved(6, 6))()));
          }),
      },
    });
    const first = view.drop({ id: 'g', col: 5, row: 5 });
    const second = view.drop({ id: 'g', col: 6, row: 6 });
    await second;
    expect(callsOf('move')).toHaveLength(1);
    finish[0]();
    await nextTurn();
    finish[1]();
    await first;
    expect(callsOf('move').map((c) => c.args.slice(3, 5))).toEqual([
      [5, 5],
      [6, 6],
    ]);
  });

  it('says a locked door stopped the token, and where to go from there', async () => {
    const e = moved(4, 3);
    const { view } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: { move: moveAnswer(e, { lockedDoor: true }) },
    });
    await view.drop({ id: 'g', col: 5, row: 3 });
    expect(view.error()).toBe(
      'Uma porta trancada parou o movimento. Destranque a porta (toque nela no mapa) e mova de novo.',
    );
    expect(spot(view, 'g')).toEqual([4, 3]);
  });

  it('says a barred or secret door stopped the token', async () => {
    const { view } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: { move: moveAnswer(moved(4, 3), { stoppedEarly: true }) },
    });
    await view.drop({ id: 'g', col: 5, row: 3 });
    expect(view.error()).toBe(
      'Uma grade levadiça ou uma porta secreta parou o movimento. Abra a porta (toque nela no mapa) e mova de novo.',
    );
  });

  it('a locked door wins when the answer says both', async () => {
    const { view } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: { move: moveAnswer(moved(4, 3), { stoppedEarly: true, lockedDoor: true }) },
    });
    await view.drop({ id: 'g', col: 5, row: 3 });
    expect(view.error()).toContain('Uma porta trancada');
  });

  it('a refusal puts the token back where it was and says why', async () => {
    const { view, callsOf } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: { move: refuse(Code.InvalidArgument) },
    });
    await view.drop({ id: 'g', col: 5, row: 5 });
    expect(spot(view, 'g')).toEqual([3, 3]);
    expect(view.error()).toBe('Não deu para mover o token: confira os campos e tente de novo.');
    expect(callsOf('get')).toEqual([]);
  });

  it('a refusal that means the screen is stale also reads the combat again', async () => {
    const { view, callsOf, state } = mountCombatView({
      enc: grid('g', toren(), goblin()),
      master: true,
      api: {
        move: refuse(Code.Aborted),
        get: () => encounter({ round: 3, combatants: [toren(), goblin()] }),
      },
    });
    await view.drop({ id: 'g', col: 5, row: 5 });
    await nextTurn();
    expect(callsOf('get')).toHaveLength(1);
    expect(state.encounter()?.round).toBe(3);
  });
});

describe('CombatView calls: the "Mover" page', () => {
  function mountMover(api: MountOptions['api'] = {}, enc = grid('t', toren(), goblin())) {
    const mounted = mountCombatView({ enc, api });
    mounted.state.moving.set(true);
    return mounted;
  }
  const result = (over: Record<string, unknown> = {}, e = grid('t', toren(), goblin())) => ({
    move: () => ({
      encounter: e,
      stoppedEarly: false,
      lockedDoor: false,
      provoked: false,
      ...over,
    }),
  });

  it('"Mover para cá" sends the square for the character and closes the page', async () => {
    const { view, state, callsOf } = mountMover(result());
    await view.confirmMove({ col: 4, row: 2 });
    expect(callsOf('move')[0].args).toEqual(['c', 'enc', 't', 4, 2]);
    expect(state.moving()).toBe(false);
    expect(view.moveError()).toBe('');
  });

  it('moves the creature the page was opened for', async () => {
    const wolf = combatant({ id: 'w', label: 'Lobo', controlledByMe: true });
    const e = grid('t', toren(), wolf);
    const { view, callsOf } = mountMover(result({}, e), e);
    view.moverId.set('w');
    await view.confirmMove({ col: 6, row: 6 });
    expect(callsOf('move')[0].args.slice(0, 5)).toEqual(['c', 'enc', 'w', 6, 6]);
  });

  it('does nothing when nobody moves (no character of the player)', async () => {
    const { view, callsOf } = mountMover(result(), grid('g', goblin()));
    await view.confirmMove({ col: 4, row: 2 });
    expect(callsOf('move')).toEqual([]);
  });

  it("a refusal keeps the page open with the refusal's words next to the map, not on the page", async () => {
    const { view, state } = mountMover({ move: refuse(Code.InvalidArgument) });
    view.error.set('old');
    await view.confirmMove({ col: 4, row: 2 });
    expect(state.moving()).toBe(true);
    expect(view.moveError()).toBe('Não deu para mover: confira os campos e tente de novo.');
    expect(view.error()).toBe('');
  });

  it('a move that stopped short says so and still closes the page', async () => {
    const { view, state } = mountMover(result({ stoppedEarly: true }));
    await view.confirmMove({ col: 4, row: 2 });
    expect(view.moveNote()).toBe('Você parou antes: algo bloqueou o caminho.');
    expect(state.moving()).toBe(false);
  });

  it('a locked door keeps the page open, with the notice', async () => {
    const { view, state } = mountMover(result({ lockedDoor: true }));
    await view.confirmMove({ col: 4, row: 2 });
    expect(view.lockedDoor()).toBe(true);
    expect(state.moving()).toBe(true);
    expect(view.moveNote()).toBe('');
  });

  it('a locked door wins over a stop-short in the answer', async () => {
    const { view } = mountMover(result({ lockedDoor: true, stoppedEarly: true }));
    await view.confirmMove({ col: 4, row: 2 });
    expect(view.lockedDoor()).toBe(true);
    expect(view.moveNote()).toBe('');
  });

  it("forgets the last move's note and locked door when a new move starts", async () => {
    const { view } = mountMover(result());
    view.moveNote.set('old');
    view.lockedDoor.set(true);
    await view.confirmMove({ col: 4, row: 2 });
    expect(view.moveNote()).toBe('');
    expect(view.lockedDoor()).toBe(false);
  });

  it('a move while another call is in flight sends nothing and is read as a refusal with no words', async () => {
    const { view, callsOf, state } = mountMover(result());
    view.busy.set(true);
    await view.confirmMove({ col: 4, row: 2 });
    expect(callsOf('move')).toEqual([]);
    expect(state.moving()).toBe(true);
    expect(view.moveError()).toBe('');
  });

  it('a stale refusal reads the combat again', async () => {
    const { view, callsOf } = mountMover({
      move: refuse(Code.Unavailable),
      get: () => grid('t', toren(), goblin()),
    });
    await view.confirmMove({ col: 4, row: 2 });
    expect(callsOf('get')).toHaveLength(1);
  });

  it('"Desengajar" takes the Disengage action for whoever moves', async () => {
    const { view, callsOf } = mountMover();
    expect(await view.disengage()).toBe(true);
    expect(callsOf('takeAction')[0].args).toEqual(['c', 'enc', 't', 'standard:disengage']);
  });

  it('"Desengajar" is nothing without a mover', async () => {
    const { view, callsOf } = mountMover({}, grid('g', goblin()));
    expect(await view.disengage()).toBe(false);
    expect(callsOf('takeAction')).toEqual([]);
  });

  it('closeFullPage puts every trace of the page away', () => {
    const { view, state } = mountMover();
    view.lockedDoor.set(true);
    view.moverId.set('w');
    view.dropStart.set({ col: 1, row: 1 });
    state.mapOpen.set(true);
    view.closeFullPage();
    expect(view.lockedDoor()).toBe(false);
    expect(view.moverId()).toBe('');
    expect(view.dropStart()).toBeNull();
    expect(state.moving()).toBe(false);
    expect(state.mapOpen()).toBe(false);
  });
});

describe("CombatView calls: a jump's note", () => {
  const jumper = (used: number) => theatre('t', toren({ movementUsedDft: used }));
  const answerWith = (used: number, over: Record<string, unknown> = {}) => ({
    move: () => ({
      encounter: jumper(used),
      stoppedEarly: false,
      lockedDoor: false,
      provoked: false,
      ...over,
    }),
  });

  it('says how far a long jump went, in meters', async () => {
    const { view, state } = mountCombatView({ enc: jumper(0), api: answerWith(100) });
    state.moving.set(true);
    await view.confirmJump({ kind: 'long', square: { col: 9, row: 3 } });
    expect(view.moveNote()).toBe('Você saltou 3,0 m.');
    expect(state.moving()).toBe(false);
  });

  it('says how high a high jump went', async () => {
    const { view } = mountCombatView({ enc: jumper(0), api: answerWith(60) });
    await view.confirmJump({ kind: 'high', heightDft: 60 });
    expect(view.moveNote()).toBe('Você saltou 1,8 m para cima.');
  });

  it('charges only what the jump added to the movement already used', async () => {
    const { view } = mountCombatView({ enc: jumper(40), api: answerWith(100) });
    await view.confirmJump({ kind: 'long', square: { col: 9, row: 3 } });
    expect(view.moveNote()).toBe('Você saltou 1,8 m.');
  });

  it("a high jump stays on the character's own square", async () => {
    const { view, callsOf } = mountCombatView({ enc: jumper(0), api: answerWith(60) });
    await view.confirmJump({ kind: 'high', heightDft: 60 });
    const args = callsOf('move')[0].args;
    expect(args.slice(0, 7).slice(0, 5)).toEqual(['c', 'enc', 't', 1, 1]);
    expect(args[5]).toEqual({ kind: 'high', heightDft: 60 });
  });

  it('a long jump sends the square and its kind', async () => {
    const { view, callsOf } = mountCombatView({ enc: jumper(0), api: answerWith(60) });
    await view.confirmJump({ kind: 'long', square: { col: 9, row: 3 } });
    const args = callsOf('move')[0].args;
    expect(args.slice(0, 6)).toEqual(['c', 'enc', 't', 9, 3, { kind: 'long' }]);
  });

  it('a new jump after one that worked takes a new key', async () => {
    const { view, callsOf } = mountCombatView({ enc: jumper(0), api: answerWith(60) });
    await view.confirmJump({ kind: 'high', heightDft: 60 });
    await view.confirmJump({ kind: 'high', heightDft: 60 });
    const [first, second] = callsOf('move');
    expect(second.args[6]).not.toBe(first.args[6]);
  });

  it('a refused jump says why next to the map and leaves the page open', async () => {
    const { view, state } = mountCombatView({
      enc: jumper(0),
      api: { move: refuse(Code.InvalidArgument) },
    });
    state.moving.set(true);
    await view.confirmJump({ kind: 'high', heightDft: 60 });
    expect(view.moveNote()).toBe('');
    expect(view.moveError()).toBe('Não deu para mover: confira os campos e tente de novo.');
    expect(state.moving()).toBe(true);
  });

  it('a jump that ended at a locked door keeps the page, and still says it jumped (as it does today)', async () => {
    const { view, state } = mountCombatView({
      enc: jumper(0),
      api: answerWith(60, { lockedDoor: true }),
    });
    state.moving.set(true);
    await view.confirmJump({ kind: 'long', square: { col: 9, row: 3 } });
    expect(view.moveNote()).toBe('Você saltou 1,8\u00a0m.');
    expect(view.lockedDoor()).toBe(true);
    expect(state.moving()).toBe(true);
  });

  it('does nothing when nobody jumps', async () => {
    const { view, callsOf } = mountCombatView({
      enc: theatre('g', goblin()),
      master: true,
    });
    await view.confirmJump({ kind: 'high', heightDft: 60 });
    expect(callsOf('move')).toEqual([]);
  });
});

describe('CombatView calls: opening the movement', () => {
  it('opens the "Mover" page of the character on a map, clearing the traces of the last one', () => {
    const { view, state, opened } = mountCombatView({ enc: grid('t', toren(), goblin()) });
    view.lockedDoor.set(true);
    view.moverId.set('w');
    view.dropStart.set({ col: 1, row: 1 });
    view.moveError.set('old');
    view.openMove();
    expect(state.moving()).toBe(true);
    expect(view.lockedDoor()).toBe(false);
    expect(view.moverId()).toBe('');
    expect(view.dropStart()).toBeNull();
    expect(view.moveError()).toBe('');
    expect(opened).toEqual([]);
  });

  it('without a map it opens the sheet that asks how far, for the character', () => {
    const { view, state, opened } = mountCombatView({ enc: theatre('t', toren()) });
    view.openMove();
    expect(state.moving()).toBe(false);
    expect(opened).toHaveLength(1);
    expect(opened[0].component).toBe(SpendSheet);
    expect(opened[0].config.data).toMatchObject({
      campaignId: 'c',
      encounterId: 'enc',
      combatantId: 't',
    });
  });

  it('the spend sheet closing forgets the move note', () => {
    const { view, opened } = mountCombatView({
      enc: theatre('t', toren()),
      matches: [PHONE_QUERY],
    });
    view.openMove();
    expect(opened[0].config.ariaLabel).toBe('Gastar movimento');
    view.moveNote.set('Você parou antes');
    opened[0].close(true);
    expect(view.moveNote()).toBe('');
  });

  it('opens no spend sheet without a combatant to spend for', () => {
    const { view, opened } = mountCombatView({ enc: theatre('g', goblin()), master: true });
    view.openSpend();
    expect(opened).toEqual([]);
  });

  it("opens the page of a creature of the player's, with its name kept for the day it leaves", () => {
    const wolf = combatant({ id: 'w', label: 'Lobo atroz 1', controlledByMe: true });
    const { view, state } = mountCombatView({ enc: grid('t', toren(), wolf) });
    view.lockedDoor.set(true);
    view.dropStart.set({ col: 1, row: 1 });
    view.moveError.set('old');
    view.openCreatureMove('w');
    expect(view.moverId()).toBe('w');
    expect(view.moverName).toBe('Lobo atroz 1');
    expect(state.moving()).toBe(true);
    expect(view.lockedDoor()).toBe(false);
    expect(view.dropStart()).toBeNull();
    expect(view.moveError()).toBe('');
  });

  it('an unknown creature opens the page with no name', () => {
    const { view } = mountCombatView({ enc: grid('t', toren()) });
    view.openCreatureMove('ghost');
    expect(view.moverName).toBe('');
  });

  it("without a map the creature's movement is the spend sheet for that creature", () => {
    const wolf = combatant({ id: 'w', label: 'Lobo', controlledByMe: true });
    const { view, state, opened } = mountCombatView({ enc: theatre('t', toren(), wolf) });
    view.openCreatureMove('w');
    expect(state.moving()).toBe(false);
    expect(opened).toHaveLength(1);
    expect(opened[0].config.data).toMatchObject({ combatantId: 'w' });
  });

  it('standUp stands up the character when no id is given, and nobody when there is none', async () => {
    const own = mountCombatView({ enc: grid('t', toren()) });
    expect(await own.view.standUp()).toBe(true);
    expect(own.callsOf('standUp')[0].args).toEqual(['c', 'enc', 't']);

    const none = mountCombatView({ enc: grid('g', goblin()), master: true });
    expect(await none.view.standUp()).toBe(false);
    expect(none.callsOf('standUp')).toEqual([]);
  });
});
