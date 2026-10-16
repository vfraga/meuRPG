import { signal } from '@angular/core';
import { Code, ConnectError } from '@connectrpc/connect';

import {
  RollMode,
  RollModeRequestStatus,
  type RollModeRequest,
} from '../../../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CombatState } from '../../../../core/combat/combat-state';
import { encounter } from '../../../../core/combat/combat-testing';
import { ModeRequests } from './mode-requests';
import { fakeCombatClient } from '../../../../core/combat/combat-client-testing';

const request = (status: RollModeRequestStatus, over: Record<string, unknown> = {}) =>
  ({
    id: 'r1',
    status,
    suggestedMode: RollMode.NORMAL,
    decidedMode: RollMode.ADVANTAGE,
    reason: 'x',
    ...over,
  }) as RollModeRequest;

describe('ModeRequests', () => {
  let api: {
    requestRollMode: ReturnType<typeof vi.fn>;
    cancelRollModeRequest: ReturnType<typeof vi.fn>;
  };
  let state: CombatState;
  let target: string | null;
  const busy = signal(false);
  const error = signal('');

  function requests() {
    return new ModeRequests({
      api: fakeCombatClient(api),
      campaignId: 'c',
      encounterId: 'enc',
      attackerId: 't',
      attackKey: 'attack:sword',
      state,
      targetId: () => target,
      chosen: () => RollMode.ADVANTAGE,
      busy,
      error,
    });
  }

  beforeEach(() => {
    api = { requestRollMode: vi.fn(), cancelRollModeRequest: vi.fn() };
    state = new CombatState();
    state.apply(encounter({}));
    target = 'g';
    busy.set(false);
    error.set('');
  });

  it('starts with no pick, no reason and no request', () => {
    const r = requests();
    expect(r.picked()).toBeNull();
    expect(r.reason()).toBe('');
    expect(r.request()).toBeNull();
    expect(r.answered()).toBeNull();
  });

  describe('asking the master', () => {
    it('sends the mode and the trimmed reason, applies the combat and keeps the request', async () => {
      const sent = request(RollModeRequestStatus.PENDING);
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({ revision: 3 }),
        request: sent,
      });
      const r = requests();
      r.reason.set('  estou escondido ');
      await r.ask();
      expect(api.requestRollMode).toHaveBeenCalledWith(
        'c',
        'enc',
        't',
        'attack:sword',
        'g',
        RollMode.ADVANTAGE,
        'estou escondido',
        expect.any(String),
      );
      expect(r.asked()).toBe(sent);
      expect(r.request()).toBe(sent);
      expect(busy()).toBe(false);
    });

    it('asks nothing without a target, or while busy', async () => {
      const r = requests();
      target = null;
      await r.ask();
      target = 'g';
      busy.set(true);
      await r.ask();
      expect(api.requestRollMode).not.toHaveBeenCalled();
    });

    it('says when it fails', async () => {
      api.requestRollMode.mockRejectedValue(new ConnectError('x', Code.Unavailable));
      await requests().ask();
      expect(error()).not.toBe('');
      expect(busy()).toBe(false);
    });

    it("sees the master's answer in the combat", async () => {
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({}),
        request: request(RollModeRequestStatus.PENDING),
      });
      const r = requests();
      await r.ask();
      state.apply(
        encounter({
          revision: 5,
          rollModeRequests: [request(RollModeRequestStatus.ANSWERED)] as never,
        }),
      );
      expect(r.answered()?.decidedMode).toBe(RollMode.ADVANTAGE);
    });
  });

  describe('taking it back', () => {
    it('cancels the request, clearing the pick and the reason', async () => {
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({}),
        request: request(RollModeRequestStatus.PENDING),
      });
      api.cancelRollModeRequest.mockResolvedValue(encounter({ revision: 4 }));
      const r = requests();
      r.picked.set(RollMode.ADVANTAGE);
      r.reason.set('x');
      await r.ask();
      await r.cancel();
      expect(api.cancelRollModeRequest).toHaveBeenCalledWith('c', 'enc', 'r1', expect.any(String));
      expect(r.asked()).toBeNull();
      expect(r.picked()).toBeNull();
      expect(r.reason()).toBe('');
    });

    it('cancels nothing that was not sent, or while busy, and says when it fails', async () => {
      const r = requests();
      await r.cancel();
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({}),
        request: request(RollModeRequestStatus.PENDING),
      });
      await r.ask();
      busy.set(true);
      await r.cancel();
      expect(api.cancelRollModeRequest).not.toHaveBeenCalled();
      busy.set(false);
      api.cancelRollModeRequest.mockRejectedValue(new Error('x'));
      await r.cancel();
      expect(error()).not.toBe('');
    });
  });

  describe('dropping the mode when the target changes', () => {
    it('takes back a request still waiting, without caring how that call ends', async () => {
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({}),
        request: request(RollModeRequestStatus.PENDING),
      });
      api.cancelRollModeRequest.mockRejectedValue(new Error('late'));
      const r = requests();
      await r.ask();
      r.drop();
      await Promise.resolve();
      expect(api.cancelRollModeRequest).toHaveBeenCalledTimes(1);
      expect(r.asked()).toBeNull();
    });

    it('applies the combat the cancel answered with', async () => {
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({}),
        request: request(RollModeRequestStatus.PENDING),
      });
      const after = encounter({ revision: 9 });
      api.cancelRollModeRequest.mockResolvedValue(after);
      const r = requests();
      await r.ask();
      r.drop();
      await new Promise<void>((resolve) => queueMicrotask(resolve));
      expect(state.encounter()?.revision).toBe(9);
    });

    it('only forgets an answered request, and does nothing with none', async () => {
      const r = requests();
      r.picked.set(RollMode.DISADVANTAGE);
      r.drop();
      expect(r.picked()).toBeNull();
      api.requestRollMode.mockResolvedValue({
        encounter: encounter({}),
        request: request(RollModeRequestStatus.ANSWERED),
      });
      await r.ask();
      r.drop();
      expect(api.cancelRollModeRequest).not.toHaveBeenCalled();
    });
  });
});
