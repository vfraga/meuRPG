import { LiveSessionEvents } from './live-session-events';

/** The handlers of the page's stream, with the rest of the page faked: only what a re-read touches. */
function setup(readSnapshot: () => Promise<boolean>, openCombat: boolean) {
  const touchLog = vi.fn();
  const calls: string[] = [];
  const services = {
    xpChanges: { bump: () => calls.push('xp') },
    spellCatalog: { forget: () => calls.push('catalog') },
  };
  const parts = {
    trapBoard: { refresh: () => calls.push('traps'), refreshDamages: () => undefined },
    conversation: { sync: () => calls.push('conversation') },
    combat: { touchLog, encounter: () => (openCombat ? { id: 'e1', revision: 3 } : null) },
    mapState: { refresh: () => undefined },
  };
  const state = { currentMapId: () => 'm1', isMaster: () => false };
  const loader = {
    readSnapshot: vi.fn(readSnapshot),
    scheduleVision: vi.fn(),
    loadCombat: vi.fn(),
  };
  const view = { inTheatre: () => false };
  const events = new LiveSessionEvents(
    services as never,
    state as never,
    parts as never,
    view as never,
    loader as never,
  );
  const priv = events as unknown as {
    sessionHandlers(c: string, g: number): { onBehind(): Promise<boolean> };
    mapHandlers(g: number): { onVisionChanged(mapId: string): void };
    combatHandlers(g: number): {
      onEncounterChanged(c: { encounterId: string; revision: number }): void;
    };
  };
  return { priv, loader, touchLog, calls };
}

describe('LiveSessionEvents: a heartbeat ahead of the stream', () => {
  it('reads what a `ready` reads and answers with whether the snapshot worked', async () => {
    for (const worked of [true, false]) {
      const { priv, loader, calls } = setup(() => Promise.resolve(worked), true);
      expect(await priv.sessionHandlers('c1', 1).onBehind()).toBe(worked);
      expect(loader.readSnapshot).toHaveBeenCalledTimes(1);
      expect(calls).toEqual(['xp', 'catalog', 'traps', 'conversation']);
    }
  });
});

describe('LiveSessionEvents: reading the whole combat log', () => {
  it('asks for it on a vision hint and a hint with no revision while a combat is open', () => {
    const { priv, touchLog } = setup(() => Promise.resolve(true), true);
    priv.mapHandlers(1).onVisionChanged('m1');
    priv.combatHandlers(1).onEncounterChanged({ encounterId: 'e1', revision: 0 });
    expect(touchLog).toHaveBeenCalledTimes(2);
    expect(touchLog).toHaveBeenCalledWith(true);
  });

  it('leaves the flag alone when no combat is open', () => {
    const { priv, touchLog } = setup(() => Promise.resolve(true), false);
    priv.mapHandlers(1).onVisionChanged('m1');
    priv.combatHandlers(1).onEncounterChanged({ encounterId: 'e1', revision: 0 });
    expect(touchLog).not.toHaveBeenCalled();
  });
});
