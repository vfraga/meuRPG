import { TestBed } from '@angular/core/testing';

import { RollMode } from '../../../gen/meurpg/play/v1/combat_rolls_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';

function clientWith(calls: Record<string, ReturnType<typeof vi.fn>>): CombatClient {
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  (client as unknown as { base: { client: unknown } }).base.client = calls;
  return client;
}

describe('CombatClient: roll modes, damage parts and requests', () => {
  it('sends the mode, the reason, the request id and the two physical faces of an attack', async () => {
    const rollAttack = vi.fn().mockResolvedValue({ encounter: {}, roll: {} });
    const client = clientWith({ rollAttack });
    await client.attacks.rollAttack(
      'c',
      'e',
      'a',
      'attack:x',
      't',
      { faces: [14, 7] },
      'k',
      false,
      '',
      {
        mode: RollMode.ADVANTAGE,
        reason: 'costas',
        requestId: 'r1',
      },
    );
    const req = rollAttack.mock.calls[0][0];
    expect(req.d20Faces).toEqual([14, 7]);
    expect(req.roll).toEqual({ case: undefined });
    expect(req.rollMode).toBe(RollMode.ADVANTAGE);
    expect(req.modeReason).toBe('costas');
    expect(req.rollModeRequestId).toBe('r1');
  });

  it('leaves the mode out when the suggestion stands and keeps the single typed face', async () => {
    const rollAttack = vi.fn().mockResolvedValue({ encounter: {}, roll: {} });
    const client = clientWith({ rollAttack });
    await client.attacks.rollAttack('c', 'e', 'a', 'attack:x', 't', { face: 12 }, 'k');
    const req = rollAttack.mock.calls[0][0];
    expect(req.roll).toEqual({ case: 'd20Face', value: 12 });
    expect(req.rollMode).toBeUndefined();
    expect(req.d20Faces).toEqual([]);
  });

  it('answers the extras as the whole choice, even none, and sends the typed parts', async () => {
    const rollDamage = vi.fn().mockResolvedValue({ encounter: {}, pendingDamage: {} });
    const client = clientWith({ rollDamage });
    await client.attacks.rollDamage('c', 'e', 'p', { parts: true }, 'k', {
      extras: [{ key: 'divine-smite', slotLevel: 2, pact: false }],
      typedParts: [{ partKey: 'weapon', sum: 5 }],
    });
    const req = rollDamage.mock.calls[0][0];
    expect(req.extrasChosen).toBe(true);
    expect(req.selectedExtras).toEqual([{ key: 'divine-smite', slotLevel: 2, pact: false }]);
    expect(req.typedParts).toEqual([{ partKey: 'weapon', sum: 5 }]);
    await client.attacks.rollDamage('c', 'e', 'p', { inApp: true }, 'k2', { extras: [] });
    expect(rollDamage.mock.calls[1][0].extrasChosen).toBe(true);
    await client.attacks.rollDamage('c', 'e', 'p', { inApp: true }, 'k3');
    expect(rollDamage.mock.calls[2][0].extrasChosen).toBeUndefined();
  });

  it('sends the sources to ignore when the damage is applied', async () => {
    const applyPendingDamage = vi.fn().mockResolvedValue({ encounter: {}, pendingDamage: {} });
    const client = clientWith({ applyPendingDamage });
    await client.attacks.applyDamage('c', 'e', 'p', undefined, 'k', ['race:tiefling']);
    expect(applyPendingDamage.mock.calls[0][0].ignoreModifiers).toEqual(['race:tiefling']);
  });

  it('sends the spell attack mode with the physical pair', async () => {
    const castSpell = vi
      .fn()
      .mockResolvedValue({ encounter: {}, cast: {}, summonedCombatantIds: [] });
    const client = clientWith({ castSpell });
    await client.casts.castSpell(
      'c',
      'e',
      'm',
      's',
      null,
      [],
      { faces: [3, 18] },
      'k',
      undefined,
      '',
      {
        mode: RollMode.DISADVANTAGE,
        reason: 'fumaça',
      },
    );
    const req = castSpell.mock.calls[0][0];
    expect(req.d20Faces).toEqual([3, 18]);
    expect(req.rollMode).toBe(RollMode.DISADVANTAGE);
    expect(req.modeReason).toBe('fumaça');
  });

  it('asks the master, answers, cancels and takes a part out', async () => {
    const requestRollMode = vi.fn().mockResolvedValue({ encounter: {}, request: { id: 'r' } });
    const answerRollModeRequest = vi.fn().mockResolvedValue({ encounter: {} });
    const cancelRollModeRequest = vi.fn().mockResolvedValue({ encounter: {} });
    const removeDamagePart = vi.fn().mockResolvedValue({ encounter: {}, pendingDamage: {} });
    const client = clientWith({
      requestRollMode,
      answerRollModeRequest,
      cancelRollModeRequest,
      removeDamagePart,
    });
    const res = await client.attacks.requestRollMode(
      'c',
      'e',
      'a',
      'attack:x',
      't',
      RollMode.ADVANTAGE,
      'costas',
      'k1',
    );
    expect(res.request.id).toBe('r');
    expect(requestRollMode.mock.calls[0][0]).toMatchObject({
      attackerId: 'a',
      attackKey: 'attack:x',
      targetId: 't',
      rollMode: RollMode.ADVANTAGE,
      reason: 'costas',
      idempotencyKey: 'k1',
    });
    await client.attacks.answerRollModeRequest('c', 'e', 'r', RollMode.NORMAL, 'k2');
    expect(answerRollModeRequest.mock.calls[0][0]).toMatchObject({
      requestId: 'r',
      decidedMode: RollMode.NORMAL,
    });
    await client.attacks.cancelRollModeRequest('c', 'e', 'r', 'k3');
    expect(cancelRollModeRequest.mock.calls[0][0]).toMatchObject({ requestId: 'r' });
    await client.attacks.removeDamagePart('c', 'e', 'p', 'sneak-attack', 'não vale', 'k4');
    expect(removeDamagePart.mock.calls[0][0]).toMatchObject({
      partKey: 'sneak-attack',
      reason: 'não vale',
    });
  });
});
