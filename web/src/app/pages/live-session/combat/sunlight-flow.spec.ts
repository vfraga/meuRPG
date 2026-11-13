import { signal } from '@angular/core';

import { SunlightScope } from '../../../../gen/meurpg/play/v1/combat_pb';
import { encounter } from '../../../core/combat/combat-testing';
import { SunlightFlow } from './sunlight-flow';

describe('SunlightFlow', () => {
  function setup(busyFirst = false) {
    const busy = signal(busyFirst);
    const setSunlight = vi.fn(async () => encounter());
    const run = vi.fn(async (call: (e: ReturnType<typeof encounter>) => Promise<unknown>) => {
      await call(encounter({ id: 'e1', sunlightOn: true }));
      return true;
    });
    const flow = new SunlightFlow({
      api: { combatants: { setSunlight } },
      calls: { busy, run } as never,
      campaignId: () => 'c1',
    });
    return { flow, setSunlight, busy };
  }

  it('asks for a scope with the sun on', async () => {
    const { flow, setSunlight } = setup();
    flow.ask({ scope: SunlightScope.SQUARES });
    await vi.waitFor(() =>
      expect(setSunlight).toHaveBeenCalledWith('c1', 'e1', {
        on: true,
        scope: SunlightScope.SQUARES,
      }),
    );
  });

  it('sends a stroke as add and remove squares', async () => {
    const { flow, setSunlight } = setup();
    flow.ask({ addSquares: [{ col: 1, row: 2 }] });
    await vi.waitFor(() =>
      expect(setSunlight).toHaveBeenCalledWith('c1', 'e1', {
        on: true,
        addSquares: [{ col: 1, row: 2 }],
        removeSquares: [],
      }),
    );
  });

  it('waits for the call in the air instead of dropping the stroke', async () => {
    vi.useFakeTimers();
    try {
      const { flow, setSunlight, busy } = setup(true);
      flow.ask({ addSquares: [{ col: 0, row: 0 }] });
      await vi.advanceTimersByTimeAsync(150);
      expect(setSunlight).not.toHaveBeenCalled();
      busy.set(false);
      await vi.advanceTimersByTimeAsync(150);
      expect(setSunlight).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});
