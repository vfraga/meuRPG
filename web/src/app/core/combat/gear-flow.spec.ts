import { signal } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import type { GearRead, GearVm } from '../characters/gear';
import {
  GearFlow,
  type GearTurn,
  gearBlocked,
  gearCost,
  shieldBackAc,
  twoHandedWarnings,
} from './gear-flow';

const sword: GearVm = {
  key: 'equipment:longsword',
  namePt: 'Espada longa',
  kind: 'weapon',
  equipped: true,
  twoHanded: false,
  armorClassIfToggled: 0,
  count: 1,
};
const shield: GearVm = { ...sword, key: 'equipment:shield', namePt: 'Escudo', kind: 'shield' };
const bow: GearVm = {
  ...sword,
  key: 'equipment:longbow',
  namePt: 'Arco longo',
  equipped: false,
  twoHanded: true,
  armorClassIfToggled: 16,
  count: 1,
};

const read = (over: Partial<GearRead> = {}): GearRead => ({
  gear: [sword],
  armorClass: 14,
  shieldIdle: false,
  revision: 1,
  ...over,
});

function flow(readFn: () => Promise<GearRead>, set = () => Promise.resolve()) {
  const calls = { read: 0, set: [] as [string, boolean][] };
  const revision = signal(1);
  const characterId = signal('ch1');
  const f = TestBed.runInInjectionContext(
    () =>
      new GearFlow({
        client: {
          read: () => {
            calls.read++;
            return readFn();
          },
          set: (_c, _ch, key, equipped) => {
            calls.set.push([key, equipped]);
            return set();
          },
        },
        campaignId: () => 'c1',
        characterId,
        signature: () => String(revision()),
      }),
  );
  const settle = async () => {
    TestBed.tick();
    await Promise.resolve();
    await Promise.resolve();
  };
  return { f, calls, revision, characterId, settle };
}

describe('GearFlow', () => {
  it('reads the gear when the screen is built, and holds the armor class and the shield', async () => {
    const { f, calls, settle } = flow(() => Promise.resolve(read({ shieldIdle: true })));
    await settle();
    expect(calls.read).toBe(1);
    expect(f.gear()).toEqual([sword]);
    expect(f.armorClass()).toBe(14);
    expect(f.shieldIdle()).toBe(true);
  });

  it('reads again when the own turn changes, so a change from elsewhere does not linger', async () => {
    let gear = [sword];
    const { f, calls, revision, settle } = flow(() => Promise.resolve(read({ gear })));
    await settle();
    gear = [{ ...sword, equipped: false }];
    revision.set(2);
    await settle();
    expect(calls.read).toBe(2);
    expect(f.gear()[0].equipped).toBe(false);
  });

  it('reads again after the master’s undo of a swap, so the piece comes back to the list', async () => {
    let gear = [{ ...sword, equipped: false }];
    const { f, calls, revision, settle } = flow(() => Promise.resolve(read({ gear })));
    await settle();
    expect(f.gear()[0].equipped).toBe(false);
    gear = [sword]; // the undo put the piece back and gave the turn's free interaction back: the signature moves
    revision.set(2);
    await settle();
    expect(calls.read).toBe(2);
    expect(f.gear()[0].equipped).toBe(true);
  });

  it('does not read while the fold is closed, and reads what moved when it opens', async () => {
    const { f, calls, revision, settle } = flow(() => Promise.resolve(read()));
    await settle();
    f.setOpen(false);
    revision.set(2);
    revision.set(3);
    await settle();
    expect(calls.read).toBe(1);
    f.setOpen(true);
    revision.set(4);
    await settle();
    expect(calls.read).toBe(2);
  });

  it('asks the server, reads again and says the change with the server armor class', async () => {
    let ac = 14;
    const { f, calls, settle } = flow(() => Promise.resolve(read({ armorClass: ac })));
    await settle();
    ac = 12;
    expect(await f.toggle(sword.key, false)).toBe(true);
    expect(calls.set).toEqual([[sword.key, false]]);
    expect(f.notice()).toBe('Guardado: Espada longa. Classe de armadura 12.');
    expect(f.busyKey()).toBeNull();
  });

  it('says the refusal and keeps the list when the server refuses', async () => {
    const { f, settle } = flow(
      () => Promise.resolve(read()),
      () => Promise.reject(new ConnectError('nope', Code.FailedPrecondition)),
    );
    await settle();
    expect(await f.toggle(sword.key, false)).toBe(false);
    expect(f.error()).not.toBe('');
    expect(f.notice()).toBe('');
    expect(f.gear()).toEqual([sword]);
    expect(f.busyKey()).toBeNull();
  });

  it('keeps the answer of the newest read when an older one comes back last', async () => {
    const pending: ((r: GearRead) => void)[] = [];
    const { f, revision, settle } = flow(() => new Promise((res) => pending.push(res)));
    await settle();
    revision.set(2);
    await settle();
    expect(pending).toHaveLength(2);
    pending[1](read({ armorClass: 16 }));
    await settle();
    pending[0](read({ armorClass: 10 }));
    await settle();
    expect(f.armorClass()).toBe(16);
  });
});

describe('GearFlow, when a read fails', () => {
  it('says so and reads again on "Tentar de novo", in the live path (no hand-made load)', async () => {
    let fail = true;
    const { f, calls, settle } = flow(() =>
      fail ? Promise.reject(new Error('x')) : Promise.resolve(read()),
    );
    await settle();
    expect(f.readFailed()).toBe(true);
    expect(f.gear()).toEqual([]);
    fail = false;
    await f.retry();
    expect(calls.read).toBe(2);
    expect(f.readFailed()).toBe(false);
    expect(f.gear()).toEqual([sword]);
  });

  it('does not announce a change whose list could not be read again', async () => {
    let n = 0;
    const { f, settle } = flow(() =>
      n++ === 0 ? Promise.resolve(read()) : Promise.reject(new Error('x')),
    );
    await settle();
    expect(await f.toggle(sword.key, false)).toBe(true);
    expect(f.notice()).toBe('');
    expect(f.error()).toContain('não deu para ler');
  });
});

describe('the cost of a change', () => {
  const turn: GearTurn = { objectInteractionUsed: false, actionUsed: false, extraObject: false };
  it('is the free interaction for the first weapon and the action for the next one and the shield', () => {
    expect(gearCost(sword, turn)).toBe('grátis');
    expect(gearCost(sword, { ...turn, objectInteractionUsed: true })).toBe('ação');
    expect(gearCost(shield, turn)).toBe('ação');
  });

  it('is refused beforehand, in words, when neither the free interaction nor the action is left', () => {
    const spent = { ...turn, objectInteractionUsed: true, actionUsed: true };
    expect(gearBlocked(sword, spent)).toBe('Você já usou a ação deste turno.');
    expect(gearBlocked(shield, { ...turn, actionUsed: true })).not.toBe('');
    expect(gearBlocked(sword, { ...turn, actionUsed: true })).toBe(''); // the free interaction is left
    expect(gearBlocked(sword, turn)).toBe('');
  });

  it('counts the extra action (Velocidade) where the server does: it pays when the action is spent', () => {
    const spent = { objectInteractionUsed: true, actionUsed: true, extraObject: true };
    expect(gearCost(sword, spent)).toBe('ação extra');
    expect(gearBlocked(sword, spent)).toBe('');
  });
});

describe('the two-handed consequence', () => {
  it('warns before drawing a two-handed weapon with the shield in hand, with the server’s numbers', () => {
    expect(twoHandedWarnings([shield, bow], 18)).toEqual([
      {
        key: bow.key,
        text: 'Sacar Arco longo (duas mãos) deixa o escudo de lado: CA 18 → 16.',
      },
    ]);
    expect(twoHandedWarnings([sword, bow], 18)).toEqual([]); // no shield in hand: nothing is lost
    expect(twoHandedWarnings([shield, bow], null)).toEqual([]);
  });

  it('gives the armor class the shield would give back while it waits behind a two-handed weapon', () => {
    const drawn = { ...bow, equipped: true, armorClassIfToggled: 18, count: 1 };
    expect(shieldBackAc([shield, drawn], true)).toBe(18);
    expect(shieldBackAc([shield, drawn], false)).toBeNull();
  });
});
