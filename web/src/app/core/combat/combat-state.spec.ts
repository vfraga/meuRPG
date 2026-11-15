import {
  CombatantKind,
  CombatantState,
  EncounterMode,
  EncounterStatus,
} from '../../../gen/meurpg/play/v1/combat_pb';
import { CombatState } from './combat-state';
import { combatant, encounter } from './combat-testing';
import { turnBanner } from './combat-view';
import { acts, turnMembers } from './joint-turn';

describe('CombatState', () => {
  const two = [
    combatant({ id: 'a', label: 'A', col: 1, row: 1 }),
    combatant({ id: 'b', label: 'B' }),
  ];

  it('drops an answer with an older revision than the copy on screen', () => {
    const state = new CombatState();
    state.apply(encounter({ revision: 5, round: 3 }));
    state.apply(encounter({ revision: 4, round: 2 }));
    expect(state.encounter()?.round).toBe(3);
    state.apply(encounter({ revision: 6, round: 4 }));
    expect(state.encounter()?.round).toBe(4);
  });

  it('drops an older answer that arrives after a newer read, and does not count it', () => {
    const state = new CombatState();
    state.apply(encounter({ revision: 5, round: 2 }));
    const read = state.beginRead();
    state.apply(encounter({ revision: 5, round: 2 })); // an answer applied while the read is in flight
    const late = state.beginRead();
    expect(state.applyRead(late, encounter({ revision: 8, round: 4 }))).toBe(true);
    state.apply(encounter({ revision: 7, round: 3 }));
    expect(state.encounter()?.round).toBe(4);
    // The dropped answer did not count: the read that began after the last applied answer still applies.
    const next = state.beginRead();
    expect(state.applyRead(next, encounter({ revision: 9, round: 5 }))).toBe(true);
    expect(state.applyRead(read, encounter({ revision: 5 }))).toBe(false);
  });

  it('applies a read with a lower revision (a fog player counts what they saw), then compares answers to it', () => {
    const state = new CombatState();
    state.apply(encounter({ revision: 40, round: 2 }));
    expect(state.applyRead(state.beginRead(), encounter({ revision: 3, round: 3 }))).toBe(true);
    expect(state.encounter()?.round).toBe(3);
    state.apply(encounter({ revision: 4, round: 4 }));
    expect(state.encounter()?.round).toBe(4);
    state.apply(encounter({ revision: 3, round: 3 }));
    expect(state.encounter()?.round).toBe(4);
  });

  it('drops a read that started before a later read was applied', () => {
    const state = new CombatState();
    const first = state.beginRead();
    const second = state.beginRead();
    expect(state.applyRead(second, encounter({ round: 4 }))).toBe(true);
    expect(state.applyRead(first, encounter({ round: 3 }))).toBe(false);
    expect(state.encounter()?.round).toBe(4);
  });

  it('applies a read that started later even when an earlier one answers after it', () => {
    const state = new CombatState();
    const first = state.beginRead();
    const second = state.beginRead();
    expect(state.applyRead(first, encounter({ round: 3 }))).toBe(true);
    expect(state.applyRead(second, encounter({ round: 4 }))).toBe(true);
    expect(state.encounter()?.round).toBe(4);
  });

  it('drops a read that started before an answer to a call was applied', () => {
    const state = new CombatState();
    state.apply(encounter({ round: 2 }));
    const ticket = state.beginRead();
    state.apply(encounter({ round: 3 }));
    expect(state.applyRead(ticket, encounter({ round: 2 }))).toBe(false);
    expect(state.encounter()?.round).toBe(3);
  });

  it('says to read again after it drops a read for an answer that came while it was out (rehearsal 5: a reveal missed)', () => {
    const state = new CombatState();
    state.apply(encounter({ round: 2 }));
    // The master reveals a monster: the stream asks for a read.
    const ticket = state.beginRead();
    // Meanwhile the player's own move is answered, with a copy from before the reveal.
    state.apply(encounter({ round: 2, name: 'before the reveal' }));
    expect(state.applyRead(ticket, encounter({ round: 2, name: 'after the reveal' }))).toBe(false);
    expect(state.changedSince(ticket)).toBe(true);
    // The next read begins after both and is applied.
    const again = state.beginRead();
    expect(state.applyRead(again, encounter({ round: 2, name: 'after the reveal' }))).toBe(true);
    expect(state.encounter()?.name).toBe('after the reveal');
  });

  it('needs no new read when a read is dropped only because a later read was applied', () => {
    const state = new CombatState();
    const first = state.beginRead();
    const second = state.beginRead();
    expect(state.applyRead(second, encounter({ round: 4 }))).toBe(true);
    expect(state.applyRead(first, encounter({ round: 3 }))).toBe(false);
    expect(state.changedSince(first)).toBe(false);
  });

  it('takes another combat whatever its revision', () => {
    const state = new CombatState();
    state.apply(encounter({ id: 'old', revision: 9 }));
    state.apply(encounter({ id: 'new', revision: 1 }));
    expect(state.encounter()?.id).toBe('new');
  });

  it('applies turn_changed in place', () => {
    const state = new CombatState();
    state.apply(encounter({ combatants: two, currentCombatantId: 'a' }));
    expect(
      state.applyTurn({ encounterId: 'enc', round: 3, currentCombatantId: '', masterTurn: true }),
    ).toBe(true);
    expect(state.encounter()).toMatchObject({ round: 3, currentCombatantId: '', masterTurn: true });
  });

  it('asks to read again when the turn or the move is about something unknown', () => {
    const state = new CombatState();
    expect(
      state.applyTurn({ encounterId: 'enc', round: 1, currentCombatantId: 'a', masterTurn: false }),
    ).toBe(false);
    state.apply(encounter({ combatants: two }));
    expect(
      state.applyTurn({
        encounterId: 'other',
        round: 1,
        currentCombatantId: 'a',
        masterTurn: false,
      }),
    ).toBe(false);
    expect(state.applyMove({ encounterId: 'enc', combatantId: 'ghost', col: 1, row: 1 })).toBe(
      false,
    );
  });

  it('applies combatant_moved in place and marks the combatant placed', () => {
    const state = new CombatState();
    state.apply(encounter({ combatants: [combatant({ id: 'a', label: 'A', placed: false })] }));
    expect(state.applyMove({ encounterId: 'enc', combatantId: 'a', col: 7, row: 8 })).toBe(true);
    expect(state.encounter()?.combatants[0]).toMatchObject({ placed: true, col: 7, row: 8 });
  });

  it('does not move a combatant in a combat without a map', () => {
    const state = new CombatState();
    state.apply(
      encounter({
        mode: EncounterMode.THEATRE,
        combatants: [combatant({ id: 'a', label: 'A', placed: false })],
      }),
    );
    expect(state.applyMove({ encounterId: 'enc', combatantId: 'a', col: 7, row: 8 })).toBe(false);
    expect(state.encounter()?.combatants[0]).toMatchObject({ placed: false });
  });

  it('starts the surviving member of a joint turn with a fresh economy in the next round', () => {
    const state = new CombatState();
    const swords = [
      combatant({ id: 's1', label: 'Espada 1', initiative: 12, defeated: true, actionUsed: true }),
      combatant({
        id: 's2',
        label: 'Espada 2',
        initiative: 12,
        actionUsed: true,
        bonusActionUsed: true,
        reactionUsed: true,
        movementUsedDft: 100,
        movementLeftDft: 0,
        speedDft: 100,
      }),
    ];
    state.apply(
      encounter({
        combatants: swords,
        currentCombatantId: 's2',
        round: 1,
        turnGroupIds: ['s1', 's2'],
      }),
    );
    state.applyTurn({ encounterId: 'enc', round: 2, currentCombatantId: 's2', masterTurn: false });
    const [dead, alive] = state.encounter()!.combatants;
    expect(alive).toMatchObject({
      actionUsed: false,
      bonusActionUsed: false,
      reactionUsed: false,
      movementUsedDft: 0,
      movementLeftDft: 100,
    });
    expect(dead.actionUsed).toBe(true);
  });

  it('drops a read that started before turn_changed, which the read may not show, and says to read again', () => {
    const state = new CombatState();
    state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1 }));
    const read = state.beginRead();
    state.applyTurn({ encounterId: 'enc', round: 2, currentCombatantId: 'b', masterTurn: false });
    expect(state.patchedSince(read)).toBe(true);
    expect(
      state.applyRead(read, encounter({ combatants: two, currentCombatantId: 'a', round: 1 })),
    ).toBe(false);
    expect(state.encounter()).toMatchObject({ round: 2, currentCombatantId: 'b' });
    // The read that begins after the event is the one to trust.
    const next = state.beginRead();
    expect(state.patchedSince(next)).toBe(false);
    expect(
      state.applyRead(next, encounter({ combatants: two, currentCombatantId: 'b', round: 2 })),
    ).toBe(true);
  });

  it('keeps a patched turn from being overwritten by an older answer, and ignores an older turn event', () => {
    const state = new CombatState();
    state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1, revision: 4 }));
    // The event of the turn change carries the revision it made.
    state.applyTurn({
      encounterId: 'enc',
      round: 2,
      currentCombatantId: 'b',
      masterTurn: false,
      revision: 5,
    });
    expect(state.encounter()).toMatchObject({ round: 2, currentCombatantId: 'b', revision: 5 });
    // The answer of a call that began before the turn changed is older: it does not bring the turn back.
    state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1, revision: 4 }));
    expect(state.encounter()).toMatchObject({ round: 2, currentCombatantId: 'b' });
    // An event about a turn older than the one on screen changes nothing.
    expect(
      state.applyTurn({
        encounterId: 'enc',
        round: 1,
        currentCombatantId: 'a',
        masterTurn: false,
        revision: 4,
      }),
    ).toBe(true);
    expect(state.encounter()).toMatchObject({ round: 2, currentCombatantId: 'b', revision: 5 });
    // No number to compare (a player on a fog map): the turn applies and the revision stays.
    state.applyTurn({ encounterId: 'enc', round: 3, currentCombatantId: 'a', masterTurn: false });
    expect(state.encounter()).toMatchObject({ round: 3, revision: 5 });
  });

  describe('a turn event and the revisions of the copy on screen', () => {
    const turn = (revision: number, currentCombatantId = 'b') => ({
      encounterId: 'enc',
      round: 2,
      currentCombatantId,
      masterTurn: false,
      revision,
    });

    it('patches in place the event that is the very next revision', () => {
      const state = new CombatState();
      state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1, revision: 4 }));
      expect(state.applyTurn(turn(5))).toBe(true);
      expect(state.encounter()).toMatchObject({ round: 2, currentCombatantId: 'b', revision: 5 });
    });

    it('says to read again for an event two revisions ahead, and leaves the copy as it is', () => {
      const state = new CombatState();
      state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1, revision: 4 }));
      // The revision in between (the master set a character to 0) is not in the copy: a patched copy would claim it.
      expect(state.applyTurn(turn(6))).toBe(false);
      expect(state.encounter()).toMatchObject({ round: 1, currentCombatantId: 'a', revision: 4 });
    });

    it('says to read again for a turn that moves to a combatant who is down', () => {
      const down = combatant({
        id: 'p',
        label: 'Pensantus',
        kind: CombatantKind.PLAYER,
        state: CombatantState.DOWN,
      });
      const state = new CombatState();
      state.apply(
        encounter({ combatants: [...two, down], currentCombatantId: 'a', round: 1, revision: 4 }),
      );
      expect(state.applyTurn(turn(5, 'p'))).toBe(false);
      expect(state.encounter()).toMatchObject({ round: 1, currentCombatantId: 'a' });
      // A combatant at 0 hit points counts, whatever its state says.
      const empty = combatant({ id: 'z', label: 'Z', hitPointsCurrent: 0 });
      state.apply(encounter({ combatants: [...two, empty], currentCombatantId: 'a', revision: 5 }));
      expect(state.applyTurn(turn(6, 'z'))).toBe(false);
    });

    it('still drops an older answer after a turn patched in place', () => {
      const state = new CombatState();
      state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1, revision: 4 }));
      state.applyTurn(turn(5));
      state.apply(encounter({ combatants: two, currentCombatantId: 'a', round: 1, revision: 4 }));
      expect(state.encounter()).toMatchObject({ round: 2, currentCombatantId: 'b' });
    });
  });

  it('drops a read that started before combatant_moved', () => {
    const state = new CombatState();
    state.apply(encounter({ combatants: two }));
    const read = state.beginRead();
    state.applyMove({ encounterId: 'enc', combatantId: 'a', col: 7, row: 8 });
    expect(state.applyRead(read, encounter({ combatants: two }))).toBe(false);
    expect(state.encounter()?.combatants[0]).toMatchObject({ col: 7, row: 8 });
  });

  it('keeps a read that began after the patch', () => {
    const state = new CombatState();
    state.apply(encounter({ combatants: two }));
    state.applyMove({ encounterId: 'enc', combatantId: 'a', col: 7, row: 8 });
    const read = state.beginRead();
    expect(
      state.applyRead(
        read,
        encounter({ combatants: [combatant({ id: 'a', label: 'A', col: 7, row: 8 })] }),
      ),
    ).toBe(true);
  });

  it('hides an ended combat once the person left it, and shows a new one', () => {
    const state = new CombatState();
    state.apply(encounter({ status: EncounterStatus.ENDED }));
    expect(state.shown()).not.toBeNull();
    state.dismissEnded();
    expect(state.shown()).toBeNull();
    state.apply(encounter({ id: 'next', status: EncounterStatus.SETUP }));
    expect(state.shown()?.id).toBe('next');
  });

  describe('turn_changed on a joint turn', () => {
    const P = CombatantKind.PLAYER;
    const brisa = combatant({ id: 'b', label: 'Brisa', kind: P, initiative: 19 });
    const toren = combatant({ id: 't', label: 'Toren', kind: P, initiative: 19 });
    const pens = combatant({ id: 'p', label: 'Pensantus', kind: P, initiative: 14, mine: true });

    function onJointTurn(): CombatState {
      const state = new CombatState();
      state.apply(
        encounter({
          combatants: [brisa, toren, pens],
          currentCombatantId: 'b',
          turnGroupIds: ['b', 't'],
          revision: 5,
        }),
      );
      return state;
    }

    it('names the new current combatant as the turn, not the group that ended', () => {
      const state = onJointTurn();
      state.applyTurn({ encounterId: 'enc', round: 2, currentCombatantId: 'p', masterTurn: false });
      expect(turnMembers(state.encounter()!).map((c) => c.id)).toEqual(['p']);
    });

    it('lets the next player act, and the banner names them, before any reload', () => {
      const state = onJointTurn();
      state.applyTurn({ encounterId: 'enc', round: 2, currentCombatantId: 'p', masterTurn: false });
      const e = state.encounter()!;
      expect(acts(e, pens)).toBe(true);
      expect(acts(e, brisa)).toBe(false);
      expect(turnBanner(e).title).toBe('Sua vez, Pensantus');
    });

    it('reads the combat again when a part ends inside the group, since the event does not say whose', () => {
      const state = new CombatState();
      state.apply(
        encounter({
          combatants: [brisa, toren, pens],
          currentCombatantId: 'b',
          turnGroupIds: ['b', 't'],
          revision: 5,
        }),
      );
      const round = state.encounter()!.round;
      // Brisa's part was ended from another screen: the server moves "current" to Toren, in the same round. The copy
      // still shows Brisa acting, so patching only "current" would leave her turn open.
      const patched = state.applyTurn({
        encounterId: 'enc',
        round,
        currentCombatantId: 't',
        masterTurn: false,
        revision: 6,
      });
      expect(patched).toBe(false);
      expect(state.encounter()?.currentCombatantId).toBe('b');
    });

    it('has nothing to read when the event names the combatant already current in the group', () => {
      const state = onJointTurn();
      const round = state.encounter()!.round;
      expect(
        state.applyTurn({
          encounterId: 'enc',
          round,
          currentCombatantId: 'b',
          masterTurn: false,
          revision: 6,
        }),
      ).toBe(true);
    });

    it('starts the same group again in the next round, with nobody having ended their part', () => {
      const state = new CombatState();
      state.apply(
        encounter({
          combatants: [{ ...brisa, turnPartEnded: true }, { ...toren, turnPartEnded: true }, pens],
          currentCombatantId: 'b',
          turnGroupIds: ['b', 't'],
          revision: 5,
        }),
      );
      const round = state.encounter()!.round;
      state.applyTurn({
        encounterId: 'enc',
        round: round + 1,
        currentCombatantId: 'b',
        masterTurn: false,
      });
      const e = state.encounter()!;
      expect(e.turnGroupIds).toEqual([]);
      expect(e.combatants.some((c) => c.turnPartEnded)).toBe(false);
    });
  });
});

describe('CombatState, the reaction windows that closed (PM-04)', () => {
  it('keeps the sentence of a window that closed by itself until it is dismissed, and drops it with the combat', () => {
    const state = new CombatState();
    state.noteReactionClosed('w1', 'Queda Suave fechou. Você já usou a sua reação.');
    expect(state.reactionNotice()).toEqual({
      windowId: 'w1',
      text: 'Queda Suave fechou. Você já usou a sua reação.',
    });
    state.dismissReactionNotice();
    expect(state.reactionNotice()).toBeNull();
    state.noteReactionClosed('w2', 'A Contramágica fechou.');
    state.clear();
    expect(state.reactionNotice()).toBeNull();
  });

  it('a window closed with no sentence (it was answered) leaves no notice', () => {
    const state = new CombatState();
    state.noteReactionClosed('w1', '');
    expect(state.reactionNotice()).toBeNull();
  });

  it('knows which windows the stream announced: the others came with a read', () => {
    const state = new CombatState();
    state.noteReactionOpened('w1');
    expect(state.wasAnnounced('w1')).toBe(true);
    expect(state.wasAnnounced('w2')).toBe(false);
  });

  it("a turn that comes to a player's character at 0 hit points is read again: its death save due is the server's", () => {
    const state = new CombatState();
    state.apply(
      encounter({
        revision: 5,
        currentCombatantId: 'g',
        combatants: [
          combatant({
            id: 'p',
            label: 'Pensantus',
            kind: CombatantKind.PLAYER,
            state: CombatantState.DOWN,
            deathSaveDue: true,
          }),
          combatant({ id: 'g', label: 'Goblin' }),
        ],
      }),
    );
    expect(
      state.applyTurn({
        encounterId: 'enc',
        round: 2,
        currentCombatantId: 'p',
        masterTurn: false,
        revision: 6,
      }),
    ).toBe(false);
    expect(state.encounter()).toMatchObject({ currentCombatantId: 'g', revision: 5 });
    // A turn that moves on to somebody standing patches in place, and the death save is no longer due.
    const due = (id: string) =>
      state.encounter()?.combatants.find((c) => c.id === id)?.deathSaveDue;
    state.applyTurn({
      encounterId: 'enc',
      round: 2,
      currentCombatantId: 'g',
      masterTurn: false,
      revision: 6,
    });
    expect(due('p')).toBe(false);
  });

  it('keeps the removals the master just made before the log says them, newest first', () => {
    const state = new CombatState();
    const old = { attackerId: 'a', targetId: 't', key: 'rage', name: 'Fúria', reason: 'x' };
    const fresh = {
      attackerId: 'a',
      targetId: 't',
      key: 'sneak-attack',
      name: 'Ataque Furtivo',
      reason: 'y',
    };
    state.logged.set([old]);
    state.noteRemoval(fresh);
    expect(state.removals().map((r) => r.key)).toEqual(['sneak-attack', 'rage']);
  });
});
