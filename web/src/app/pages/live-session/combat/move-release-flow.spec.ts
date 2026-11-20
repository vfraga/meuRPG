import { Injector } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import {
  CombatantKind,
  ReactionKind,
  ReadiedActionSchema,
  ReadiedKind,
  ReadiedStatus,
} from '../../../../gen/meurpg/play/v1/combat_pb';
import { CombatState } from '../../../core/combat/combat-state';
import { combatant, encounter, reactionWindow } from '../../../core/combat/combat-testing';
import { MoveReleaseFlow } from './move-release-flow';

const held = create(ReadiedActionSchema, {
  id: 'r1',
  kind: ReadiedKind.MOVE,
  triggerText: 'Quando o ogro passar pela porta',
  status: ReadiedStatus.WAITING,
});

function setup(master = false) {
  const state = new CombatState();
  const tor = combatant({
    id: 'tor',
    label: 'Tor',
    kind: CombatantKind.PLAYER,
    mine: true,
    readied: held,
  });
  const ogro = combatant({ id: 'og', label: 'Ogro' });
  const live = encounter({
    combatants: [tor, ogro],
    currentCombatantId: 'og',
    reactionWindows: [
      reactionWindow({
        id: 'w1',
        kind: ReactionKind.READIED,
        reactorId: 'tor',
        reactorLabel: 'Tor',
      } as never),
    ],
  } as never);
  state.apply(live);
  const mark = vi.fn().mockResolvedValue(live);
  const reopen = vi.fn();
  const flow = new MoveReleaseFlow(
    {
      api: { attacks: { markReactionRolling: mark } as never },
      campaignId: () => 'camp',
      encounter: () => state.encounter(),
      state: () => state,
      isMaster: () => master,
      own: () => tor,
      reopenWindow: reopen,
    },
    TestBed.inject(Injector),
  );
  return { flow, mark, reopen, state, tor, ogro };
}

describe('MoveReleaseFlow, the page of a readied move (#856)', () => {
  it('opens for the window’s held move, marking the window for the player and naming whose turn it is', () => {
    const { flow, mark } = setup();
    const release = flow.open('w1');
    expect(release).toMatchObject({
      windowId: 'w1',
      reactorId: 'tor',
      reactor: 'Tor',
      master: false,
    });
    expect(mark).toHaveBeenCalledWith('camp', 'enc', 'w1', false);
    expect(flow.view()?.band.detail.replace(/ /g, ' ')).toContain('vez do Ogro');
  });

  it('does not open for a window that holds no move', () => {
    const { flow, mark } = setup();
    expect(flow.open('nope')).toBeNull();
    expect(mark).not.toHaveBeenCalled();
    expect(flow.current()).toBeNull();
  });

  it('does not mark when the master moves for the player', () => {
    const { flow, mark } = setup(true);
    flow.open('w1');
    expect(mark).not.toHaveBeenCalled();
    expect(flow.view()?.foot).toContain('ainda pode responder no próprio aparelho');
  });

  it('goes back by taking the mark off and showing the window again', async () => {
    const { flow, mark, reopen } = setup();
    flow.open('w1');
    await Promise.resolve();
    await flow.back();
    expect(mark).toHaveBeenLastCalledWith('camp', 'enc', 'w1', true);
    expect(reopen).toHaveBeenCalledWith('w1');
    expect(flow.current()).toBeNull();
  });

  it('shows no window again for the master going back', async () => {
    const { flow, reopen } = setup(true);
    flow.open('w1');
    await flow.back();
    expect(reopen).not.toHaveBeenCalled();
  });

  it('ends without clearing anything once the move was made', async () => {
    const { flow, mark } = setup();
    flow.open('w1');
    await Promise.resolve();
    flow.finish();
    expect(flow.current()).toBeNull();
    expect(mark).toHaveBeenCalledTimes(1);
  });

  it('says what became of the window once it went away, and does not show it again', async () => {
    const { flow, reopen, state, tor, ogro } = setup();
    flow.open('w1');
    TestBed.tick();
    state.apply(encounter({ revision: 9, combatants: [tor, ogro], reactionWindows: [] } as never));
    TestBed.tick();
    expect(flow.gone()).toBe(true);
    expect(flow.view()?.gone?.lead).toBe('O mestre desfez o gatilho:');
    await flow.back();
    expect(reopen).not.toHaveBeenCalled();
  });

  it('takes the mark off and stops the watch when the page leaves the release another way (the turn ends)', async () => {
    const { flow, mark } = setup();
    flow.open('w1');
    await Promise.resolve();
    flow.end();
    await vi.waitFor(() => expect(mark).toHaveBeenLastCalledWith('camp', 'enc', 'w1', true));
    expect(flow.current()).toBeNull();
    expect(flow.view()).toBeNull();
  });

  it('clears nothing on leaving when the window is already gone', async () => {
    const { flow, mark, state, tor, ogro } = setup();
    flow.open('w1');
    TestBed.tick();
    state.apply(encounter({ revision: 9, combatants: [tor, ogro], reactionWindows: [] } as never));
    TestBed.tick();
    flow.end();
    expect(mark).toHaveBeenCalledTimes(1);
  });

  it('stops the one-second clock of the watch when the release ends, not only with the view', () => {
    vi.useFakeTimers();
    try {
      const { flow } = setup();
      flow.open('w1');
      const stop = vi.spyOn(globalThis, 'clearInterval');
      flow.end();
      expect(stop).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
