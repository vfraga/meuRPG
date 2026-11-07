import { Code, ConnectError } from '@connectrpc/connect';

import {
  TableScreenKind as K,
  TableScreenNotice as N,
  TableScreenUnavailable as U,
} from '../../../gen/meurpg/play/v1/table_screen_pb';
import { TableScreenPanel } from './table-screen-panel';
import { screenResponse } from './table-screen-testing';

const LOST = new Date(2026, 0, 5, 21, 14);

describe('TableScreenPanel', () => {
  let get: ReturnType<typeof vi.fn>;
  let set: ReturnType<typeof vi.fn>;
  let panel: TableScreenPanel;

  beforeEach(() => {
    get = vi.fn(() => Promise.resolve(screenResponse({ choiceRevision: 4 })));
    set = vi.fn(() => Promise.resolve({}));
    panel = new TableScreenPanel({ get, set } as never, () => 'c1');
  });

  it('is lost only when a TV tab was open and none is now', async () => {
    expect(panel.lost()).toBe(false);
    get.mockResolvedValue(screenResponse({ presence: { open: false, lostAt: LOST } }));
    await panel.load();
    expect(panel.lost()).toBe(true);
    panel.heard({ presenceKnown: true, open: true, lostAt: null, choiceRevision: 0 });
    expect(panel.lost()).toBe(false);
  });

  it('loads the choice, the options and the presence', async () => {
    get.mockResolvedValue(
      screenResponse({
        choice: { mirror: false, pin: K.MAP },
        options: [
          { kind: K.MAP, available: true },
          { kind: K.COMBAT, available: false, unavailable: U.NO_COMBAT },
        ],
        presence: { open: false, lostAt: LOST },
        notice: N.PIN_ENDED,
      }),
    );
    await panel.load();
    expect(get).toHaveBeenCalledWith('c1');
    expect(panel.mirror()).toBe(false);
    expect(panel.pin()).toBe(K.MAP);
    expect(panel.options().map((o) => o.kind)).toEqual([K.MAP, K.COMBAT]);
    expect(panel.presence()).toEqual({ open: false, lostAt: LOST });
    expect(panel.notice()).toBe('O mapa saiu da mesa. A TV foi para a tela de espera.');
    expect(panel.barLabel()).toEqual({ text: 'TV sem conexão', pinned: false });
    expect(panel.menuLine()).toBe('Sem conexão desde 21:14');
  });

  it('is mirroring with nothing open until it reads', () => {
    expect(panel.mirror()).toBe(true);
    expect(panel.barLabel().text).toBe('Tela da mesa');
    expect(panel.menuLine()).toBe('Nenhuma tela aberta');
  });

  it('keeps what it held when a read fails', async () => {
    await panel.load();
    get.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    await panel.load();
    expect(panel.data()?.choiceRevision).toBe(4);
  });

  it('sends the choice with the revision it read, then reads again', async () => {
    await panel.load();
    get.mockResolvedValue(
      screenResponse({ choiceRevision: 5, choice: { mirror: false, pin: K.COMBAT } }),
    );
    const result = await panel.choose({ mirror: false, pin: K.COMBAT });
    expect(result).toBe('ok');
    expect(set).toHaveBeenCalledWith('c1', 4, { mirror: false, pin: K.COMBAT });
    expect(panel.pin()).toBe(K.COMBAT);
    expect(panel.failed()).toBe(false);
    expect(panel.saving()).toBe(false);
  });

  it('says "stale" when the choice moved, and shows what is there now', async () => {
    await panel.load();
    set.mockRejectedValue(new ConnectError('moved', Code.Aborted));
    get.mockResolvedValue(
      screenResponse({ choiceRevision: 9, choice: { mirror: false, pin: K.IMAGE } }),
    );
    expect(await panel.choose({ mirror: true, pin: K.UNSPECIFIED })).toBe('stale');
    expect(panel.stale()).toBe(true);
    expect(panel.failed()).toBe(false);
    expect(panel.pin()).toBe(K.IMAGE);
    // Trying again sends the new revision and clears the failure.
    set.mockResolvedValue({});
    expect(await panel.choose({ mirror: true, pin: K.UNSPECIFIED })).toBe('ok');
    expect(set).toHaveBeenLastCalledWith('c1', 9, { mirror: true, pin: K.UNSPECIFIED });
    expect(panel.failed()).toBe(false);
    expect(panel.stale()).toBe(false);
  });

  it('says "failed" for any other error and keeps the panel as it was', async () => {
    await panel.load();
    set.mockRejectedValue(new ConnectError('down', Code.Unavailable));
    expect(await panel.choose({ mirror: true, pin: K.UNSPECIFIED })).toBe('failed');
    expect(panel.failed()).toBe(true);
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('refuses a choice before the first read', async () => {
    expect(await panel.choose({ mirror: true, pin: K.UNSPECIFIED })).toBe('failed');
    expect(set).not.toHaveBeenCalled();
  });

  it('keeps the presence current from the stream', async () => {
    await panel.load();
    panel.heard({ open: true, lostAt: null, choiceRevision: 4, presenceKnown: true });
    expect(panel.presence()).toEqual({ open: true, lostAt: null });
    expect(panel.barLabel().text).toBe('Na TV: Espera');
    panel.heard({ open: false, lostAt: LOST, choiceRevision: 4, presenceKnown: true });
    expect(panel.barLabel().text).toBe('TV sem conexão');
    expect(get).toHaveBeenCalledTimes(1);
  });

  it('keeps the presence of a choice-only event, and reads when the revision is newer', async () => {
    await panel.load();
    panel.heard({ open: true, lostAt: null, choiceRevision: 4, presenceKnown: true });
    get.mockResolvedValue(
      screenResponse({ choiceRevision: 5, choice: { mirror: false, pin: K.MAP } }),
    );
    panel.heard({ open: false, lostAt: null, choiceRevision: 5, presenceKnown: false });
    await Promise.resolve();
    await Promise.resolve();
    expect(panel.presence().open).toBe(true);
    expect(get).toHaveBeenCalledTimes(2);
    expect(panel.pin()).toBe(K.MAP);
  });
});
