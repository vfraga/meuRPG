import { TestBed } from '@angular/core/testing';
import { ConnectError, Code } from '@connectrpc/connect';

import { CombatEffect, JumpKind } from '../../../gen/meurpg/play/v1/combat_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';

// A change the combat sends carries the key of its request: a retry after a lost answer reuses it (the server
// answers with what the first call did), other values take a new one, and a worked change frees the key.

interface Sent {
  readonly endTurn: { idempotencyKey: string; expectedCombatantId: string }[];
  readonly answers: (Error | null)[];
}

function clientWith(sent: Sent): CombatClient {
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  (client as unknown as { base: { client: unknown } }).base.client = {
    endTurn: (req: { idempotencyKey: string; expectedCombatantId: string }) => {
      sent.endTurn.push(req);
      const failure = sent.answers.shift();
      return failure ? Promise.reject(failure) : Promise.resolve({ encounter: { id: 'enc' } });
    },
  };
  return client;
}

describe('CombatClient keys', () => {
  it('sends the same key when the same change is tried again after a lost answer', async () => {
    const sent: Sent = { endTurn: [], answers: [new ConnectError('lost', Code.Unavailable), null] };
    const client = clientWith(sent);
    await expect(client.turn.endTurn('c', 'e', 'a')).rejects.toBeInstanceOf(ConnectError);
    await client.turn.endTurn('c', 'e', 'a');
    expect(sent.endTurn).toHaveLength(2);
    expect(sent.endTurn[1].idempotencyKey).toBe(sent.endTurn[0].idempotencyKey);
  });

  it('takes a new key for other values', async () => {
    const sent: Sent = { endTurn: [], answers: [new ConnectError('lost', Code.Unavailable), null] };
    const client = clientWith(sent);
    await expect(client.turn.endTurn('c', 'e', 'a')).rejects.toBeInstanceOf(ConnectError);
    await client.turn.endTurn('c', 'e', 'b');
    expect(sent.endTurn[1].idempotencyKey).not.toBe(sent.endTurn[0].idempotencyKey);
  });

  it('takes a new key for the same values once the change worked', async () => {
    const sent: Sent = { endTurn: [], answers: [] };
    const client = clientWith(sent);
    await client.turn.endTurn('c', 'e', 'a');
    await client.turn.endTurn('c', 'e', 'a');
    expect(sent.endTurn[1].idempotencyKey).not.toBe(sent.endTurn[0].idempotencyKey);
  });
});

describe('CombatClient area spells', () => {
  function withRequests(): { client: CombatClient; requests: Record<string, unknown>[] } {
    TestBed.configureTestingModule({
      providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(CombatClient);
    const requests: Record<string, unknown>[] = [];
    (client as unknown as { base: { client: unknown } }).base.client = {
      castSpell: (req: Record<string, unknown>) => {
        requests.push(req);
        return Promise.resolve({
          encounter: { id: 'enc' },
          cast: { targets: [] },
          summonedCombatantIds: [],
          area: { origin: { col: 3, row: 2 }, squares: [{ col: 3, row: 2 }] },
          hiddenHits: ['g3'],
          pendingRevealId: '',
        });
      },
      previewSpellArea: (req: Record<string, unknown>) => {
        requests.push(req);
        return Promise.resolve({ targets: [] });
      },
      resolveHiddenReveal: (req: Record<string, unknown>) => {
        requests.push(req);
        return Promise.resolve({ encounter: { id: 'enc' } });
      },
    };
    return { client, requests };
  }

  it("casts at a point with the master's reveal choice, and reads where it landed", async () => {
    const { client, requests } = withRequests();
    const res = await client.casts.castSpell(
      'c',
      'e',
      'zuk',
      'spell:fireball',
      { level: 3, pact: false },
      [],
      null,
      'k',
      undefined,
      '',
      undefined,
      [],
      { area: { origin: { col: 3, row: 2 } }, revealHidden: false },
    );
    expect(requests[0]['area']).toEqual({ case: 'origin', value: { col: 3, row: 2 } });
    expect(requests[0]['revealHidden']).toBe(false);
    expect(res.area?.origin).toEqual({ col: 3, row: 2 });
    expect(res.hiddenHits).toEqual(['g3']);
  });

  it('leaves the reveal out unless the master chose, and sends a direction for a cone', async () => {
    const { client, requests } = withRequests();
    await client.casts.castSpell(
      'c',
      'e',
      'p',
      'spell:burning-hands',
      { level: 1, pact: false },
      [],
      null,
      'k',
      undefined,
      '',
      undefined,
      [],
      { area: { direction: { dx: 1, dy: -1 } } },
    );
    expect('revealHidden' in requests[0]).toBe(false);
    expect(requests[0]['area']).toEqual({ case: 'direction', value: { dx: 1, dy: -1 } });
  });

  it('previews a sphere around the caster with no point, and answers a question with a key', async () => {
    const { client, requests } = withRequests();
    await client.casts.previewSpellArea('c', 'e', 'p', 'spell:x', null, null);
    expect(requests[0]['area']).toEqual({ case: undefined });
    await client.casts.resolveHiddenReveal('c', 'e', 'q1', true);
    expect(requests[1]).toMatchObject({ pendingRevealId: 'q1', reveal: true });
    expect(requests[1]['idempotencyKey']).toEqual(expect.any(String));
  });
});

describe('CombatClient.endCombatEffect', () => {
  it('ends Ajuda on a combatant, with a key that a retry keeps and another effect or combatant changes', async () => {
    TestBed.configureTestingModule({
      providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(CombatClient);
    const sent: { combatantId: string; effect: CombatEffect; idempotencyKey: string }[] = [];
    const answers: (Error | null)[] = [new ConnectError('lost', Code.Unavailable), null, null];
    (client as unknown as { base: { client: unknown } }).base.client = {
      endCombatEffect: (req: {
        combatantId: string;
        effect: CombatEffect;
        idempotencyKey: string;
      }) => {
        sent.push(req);
        const failure = answers.shift();
        return failure ? Promise.reject(failure) : Promise.resolve({ encounter: { id: 'enc' } });
      },
    };
    await expect(
      client.combatants.endCombatEffect('c', 'e', 'sal', CombatEffect.AID),
    ).rejects.toBeInstanceOf(ConnectError);
    await client.combatants.endCombatEffect('c', 'e', 'sal', CombatEffect.AID);
    await client.combatants.endCombatEffect('c', 'e', 'tor', CombatEffect.AID);
    expect(sent.map((s) => [s.combatantId, s.effect])).toEqual([
      ['sal', CombatEffect.AID],
      ['sal', CombatEffect.AID],
      ['tor', CombatEffect.AID],
    ]);
    expect(sent[1].idempotencyKey).toBe(sent[0].idempotencyKey);
    expect(sent[2].idempotencyKey).not.toBe(sent[0].idempotencyKey);
  });
});

describe('CombatClient.moveOptions', () => {
  function clientAsking() {
    TestBed.configureTestingModule({
      providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(CombatClient);
    const sent: object[] = [];
    (client as unknown as { base: { client: unknown } }).base.client = {
      getMoveOptions: (req: object) => {
        sent.push(req);
        return Promise.resolve({});
      },
    };
    return { client, sent };
  }

  it('asks for the walk with no jump', async () => {
    const { client, sent } = clientAsking();
    await client.moves.moveOptions('c', 'e', 't');
    expect(sent[0]).toMatchObject({ jump: JumpKind.UNSPECIFIED, jumpRunningStart: false });
  });

  it("asks for the long jump, with the running start, so the warning is the jump's", async () => {
    const { client, sent } = clientAsking();
    await client.moves.moveOptions('c', 'e', 't', { runningStart: true });
    expect(sent[0]).toMatchObject({
      combatantId: 't',
      jump: JumpKind.LONG,
      jumpRunningStart: true,
    });
  });
});

describe('CombatClient, the reaction windows (PM-04)', () => {
  function fake() {
    const calls = {
      answer: [] as Record<string, unknown>[],
      concentration: [] as Record<string, unknown>[],
      attack: [] as Record<string, unknown>[],
    };
    TestBed.configureTestingModule({
      providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(CombatClient);
    (client as unknown as { base: { client: unknown } }).base.client = {
      answerReaction: (req: Record<string, unknown>) => {
        calls.answer.push(req);
        return Promise.resolve({ encounter: { id: 'enc' }, result: { used: true } });
      },
      resolveConcentrationSave: (req: Record<string, unknown>) => {
        calls.concentration.push(req);
        return Promise.resolve({ encounter: { id: 'enc' } });
      },
      rollAttack: (req: Record<string, unknown>) => {
        calls.attack.push(req);
        return Promise.resolve({ encounter: { id: 'enc' }, roll: { d20: {} } });
      },
    };
    return { client, calls };
  }

  it('answers USE with the slot, the Infernal Legacy, the creatures and the die the window asked for', async () => {
    const { client, calls } = fake();
    await client.attacks.answerReaction(
      'c',
      'e',
      'w1',
      {
        use: true,
        slot: { level: 2, pact: true },
        useRacial: false,
        creatureIds: ['a', 'b'],
        die: { typed: 9 },
      },
      'key-1',
    );
    expect(calls.answer[0]).toMatchObject({
      campaignId: 'c',
      encounterId: 'e',
      windowId: 'w1',
      answer: 1,
      slot: { level: 2, pact: true },
      useRacial: false,
      creatureIds: ['a', 'b'],
      roll: { case: 'typed', value: 9 },
      idempotencyKey: 'key-1',
    });
  });

  it('answers PASS with nothing else, and asks the app to roll with roll_in_app', async () => {
    const { client, calls } = fake();
    await client.attacks.answerReaction('c', 'e', 'w1', { use: false }, 'k');
    await client.attacks.answerReaction('c', 'e', 'w1', { use: true, die: { inApp: true } }, 'k2');
    expect(calls.answer[0]).toMatchObject({
      answer: 2,
      creatureIds: [],
      roll: { case: undefined },
    });
    expect(calls.answer[0]['slot']).toBeUndefined();
    expect(calls.answer[1]).toMatchObject({ answer: 1, roll: { case: 'rollInApp', value: true } });
  });

  it('settles a concentration save four ways: the app, a typed d20, the master, or kept', async () => {
    const { client, calls } = fake();
    await client.attacks.resolveConcentrationSave('c', 'e', 'w', { kind: 'app' }, 'k1');
    await client.attacks.resolveConcentrationSave('c', 'e', 'w', { kind: 'typed', face: 12 }, 'k2');
    await client.attacks.resolveConcentrationSave('c', 'e', 'w', { kind: 'hand' }, 'k3');
    await client.attacks.resolveConcentrationSave('c', 'e', 'w', { kind: 'keep' }, 'k4');
    expect(calls.concentration.map((c) => c['roll'])).toEqual([
      { case: 'rollInApp', value: true },
      { case: 'd20Face', value: 12 },
      { case: 'handToMaster', value: true },
      { case: 'keep', value: true },
    ]);
  });

  it('sends the second d20 of a typed concentration save with advantage or disadvantage', async () => {
    const { client, calls } = fake();
    await client.attacks.resolveConcentrationSave(
      'c',
      'e',
      'w',
      { kind: 'typed', face: 15, second: 3 },
      'k',
    );
    expect(calls.concentration[0]).toMatchObject({
      roll: { case: 'd20Face', value: 15 },
      secondD20Face: 3,
    });
  });

  it("names the window that caught the missile on the monk's throw back", async () => {
    const { client, calls } = fake();
    await client.attacks.rollAttack(
      'c',
      'e',
      'a',
      'attack:bow',
      't',
      { inApp: true },
      'k',
      true,
      '',
      undefined,
      'w2',
    );
    expect(calls.attack[0]).toMatchObject({ asReaction: true, catchWindowId: 'w2' });
  });
});

describe('CombatClient Revivify', () => {
  it('casts on a dead target with the diamonds confirmed and no combatant target', async () => {
    TestBed.configureTestingModule({
      providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(CombatClient);
    let sent: unknown;
    (client as unknown as { base: { client: unknown } }).base.client = {
      castSpell: (req: unknown) => {
        sent = req;
        return Promise.resolve({
          encounter: { id: 'enc' },
          cast: { effectKind: 7 },
          summonedCombatantIds: [],
        });
      },
    };
    await client.casts.castRevivify('c', 'e', 'i', { level: 3, pact: false }, 'toren', 'k');
    expect(sent).toMatchObject({
      spellKey: 'spell:revivify',
      slot: { level: 3, pact: false },
      targets: [{ deadTargetId: 'toren' }],
      materialConfirmed: true,
      idempotencyKey: 'k',
    });
  });
});
