import { TestBed } from '@angular/core/testing';

import {
  EffectAudience,
  EffectDurationKind,
  EffectEndScope,
} from '../../../gen/meurpg/play/v1/lasting_effects_pb';
import { ExhaustionLowerReason } from '../../../gen/meurpg/play/v1/lasting_effects_service_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { EffectsClient } from './effects-client';

function clientWith(calls: Record<string, ReturnType<typeof vi.fn>>): EffectsClient {
  TestBed.configureTestingModule({
    providers: [EffectsClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(EffectsClient);
  (client as unknown as { client: unknown }).client = calls;
  return client;
}

describe('EffectsClient', () => {
  it("sends the master's Legendary Resistance choice with the roll, and false otherwise", async () => {
    const rollEffectSave = vi.fn().mockResolvedValue({ encounter: { id: 'e' } });
    const client = clientWith({ rollEffectSave });
    await client.rollEffectSave('c', 'e', 'w', { kind: 'app', useLegendaryResistance: true }, 'k1');
    await client.rollEffectSave('c', 'e', 'w', { kind: 'typed', face: 4, extra: [] }, 'k2');
    await client.rollEffectSave('c', 'e', 'w', { kind: 'skip' }, 'k3');
    expect(rollEffectSave.mock.calls.map((c) => c[0].useLegendaryResistance)).toEqual([
      true,
      false,
      undefined,
    ]);
  });

  it('adds an effect with the key and the choices the master made', async () => {
    const addLastingEffect = vi.fn().mockResolvedValue({ encounter: { id: 'e' }, effects: [] });
    const client = clientWith({ addLastingEffect });
    await client.add(
      {
        campaignId: 'c',
        encounterId: 'e',
        targetIds: ['t1', 't2'],
        catalogKey: 'spell:bless',
        casterId: 'tavo',
        duration: { kind: EffectDurationKind.ROUNDS, rounds: 10 },
        saveDc: 13,
        playerVisible: true,
        audience: EffectAudience.OWNER,
        playerLabel: 'Abençoado',
      },
      'key-1',
    );
    expect(addLastingEffect).toHaveBeenCalledWith({
      campaignId: 'c',
      encounterId: 'e',
      idempotencyKey: 'key-1',
      targetIds: ['t1', 't2'],
      catalogKey: 'spell:bless',
      casterId: 'tavo',
      duration: { kind: EffectDurationKind.ROUNDS, rounds: 10, anchorCombatantId: '' },
      saveDc: 13,
      playerVisible: true,
      audience: EffectAudience.OWNER,
      playerLabel: 'Abençoado',
      abilityKey: '',
    });
  });

  it('sends the ability Aprimorar Habilidade is cast for', async () => {
    const addLastingEffect = vi.fn().mockResolvedValue({ effects: [] });
    const client = clientWith({ addLastingEffect });
    await client.add(
      {
        campaignId: 'c',
        encounterId: 'e',
        targetIds: ['t1'],
        catalogKey: 'spell:enhance-ability',
        duration: { kind: EffectDurationKind.UNTIL_DISMISSED },
        abilityKey: 'dex',
      },
      'k',
    );
    expect(addLastingEffect.mock.calls[0][0]).toMatchObject({ abilityKey: 'dex' });
  });

  it('gives an effect to characters outside a combat', async () => {
    const addCharacterEffect = vi.fn().mockResolvedValue({ effectIds: ['x'] });
    const client = clientWith({ addCharacterEffect });
    await client.addCharacterEffect(
      {
        campaignId: 'c',
        characterIds: ['a', 'b'],
        catalogKey: 'spell:bless',
        durationKind: EffectDurationKind.ROUNDS,
        seconds: 600,
        playerVisible: false,
        abilityKey: 'wis',
      },
      'key-2',
    );
    expect(addCharacterEffect).toHaveBeenCalledWith({
      campaignId: 'c',
      idempotencyKey: 'key-2',
      characterIds: ['a', 'b'],
      catalogKey: 'spell:bless',
      durationKind: EffectDurationKind.ROUNDS,
      seconds: 600,
      playerVisible: false,
      audience: undefined,
      playerLabel: '',
      abilityKey: 'wis',
    });
  });

  it('reads the catalog outside a combat with an empty encounter id', async () => {
    const listLastingEffects = vi.fn().mockResolvedValue({ catalog: [{ key: 'spell:bless' }] });
    const client = clientWith({ listLastingEffects });
    expect(await client.catalog('c')).toEqual([{ key: 'spell:bless' }]);
    expect(listLastingEffects).toHaveBeenCalledWith({ campaignId: 'c', encounterId: '' });
  });

  it('leaves the catalog to decide what the call does not say', async () => {
    const addLastingEffect = vi.fn().mockResolvedValue({ effects: [] });
    const client = clientWith({ addLastingEffect });
    await client.add(
      {
        campaignId: 'c',
        encounterId: 'e',
        targetIds: ['t1'],
        catalogKey: 'condition:prone',
        duration: { kind: EffectDurationKind.UNTIL_DISMISSED },
      },
      'k',
    );
    const sent = addLastingEffect.mock.calls[0][0] as Record<string, unknown>;
    expect(sent['saveDc']).toBeUndefined();
    expect(sent['playerVisible']).toBeUndefined();
    expect(sent['audience']).toBeUndefined();
    expect(sent['casterId']).toBe('');
  });

  it('ends an effect outside a combat with an empty encounter id', async () => {
    const endLastingEffect = vi.fn().mockResolvedValue({ ended: 1, createdEffectIds: [] });
    const client = clientWith({ endLastingEffect });
    await client.end('c', '', 'fx', EffectEndScope.THIS, 'k');
    expect(endLastingEffect).toHaveBeenCalledWith({
      campaignId: 'c',
      encounterId: '',
      effectId: 'fx',
      scope: EffectEndScope.THIS,
      idempotencyKey: 'k',
    });
  });

  it('answers the combat after a duration, a visibility and a removed target', async () => {
    const encounter = { id: 'e' };
    const client = clientWith({
      changeLastingEffectDuration: vi.fn().mockResolvedValue({ encounter }),
      setLastingEffectVisibility: vi.fn().mockResolvedValue({ encounter }),
      removeEffectTarget: vi.fn().mockResolvedValue({ encounter }),
    });
    expect(
      await client.changeDuration(
        'c',
        'e',
        'fx',
        { kind: EffectDurationKind.ROUNDS, rounds: 9 },
        'k',
      ),
    ).toBe(encounter);
    expect(
      await client.setVisibility(
        'c',
        'e',
        'fx',
        { playerVisible: false, audience: EffectAudience.ALL, playerLabel: '' },
        'k',
      ),
    ).toBe(encounter);
    expect(await client.removeTarget('c', 'e', 'fx', 't1', 'k')).toBe(encounter);
  });

  it('sets the exhaustion of a combatant with its combat, and of a character without one', async () => {
    const setExhaustion = vi.fn().mockResolvedValue({ level: 4, hitPointsMax: 20 });
    const lowerExhaustion = vi.fn().mockResolvedValue({ level: 3, hitPointsMax: 20 });
    const client = clientWith({ setExhaustion, lowerExhaustion });
    await client.setExhaustion(
      'c',
      { combatantId: 'k1', encounterId: 'e' },
      { level: 4, expectedLevel: 2, confirmDeath: false },
      'k',
    );
    expect(setExhaustion).toHaveBeenCalledWith({
      campaignId: 'c',
      idempotencyKey: 'k',
      subject: { case: 'combatantId', value: 'k1' },
      encounterId: 'e',
      level: 4,
      expectedLevel: 2,
      confirmDeath: false,
    });
    await client.lowerExhaustion(
      'c',
      { characterId: 'ch' },
      { by: 1, expectedLevel: 4, reason: ExhaustionLowerReason.MASTER },
      'k2',
    );
    expect(lowerExhaustion).toHaveBeenCalledWith({
      campaignId: 'c',
      idempotencyKey: 'k2',
      subject: { case: 'characterId', value: 'ch' },
      encounterId: '',
      by: 1,
      expectedLevel: 4,
      reason: ExhaustionLowerReason.MASTER,
    });
  });

  it('moves the game time and answers how many effects ended', async () => {
    const advanceGameTime = vi.fn().mockResolvedValue({ effectsEnded: 2 });
    const client = clientWith({ advanceGameTime });
    expect(await client.advanceTime('c', 600, 'k')).toBe(2);
    expect(advanceGameTime).toHaveBeenCalledWith({
      campaignId: 'c',
      seconds: 600,
      idempotencyKey: 'k',
    });
  });

  it('lists the effects of the combat and of the characters', async () => {
    const listLastingEffects = vi.fn().mockResolvedValue({ effects: [] });
    const listCharacterEffects = vi.fn().mockResolvedValue({ effects: [{ id: 'x' }] });
    const client = clientWith({ listLastingEffects, listCharacterEffects });
    await client.list('c', 'e');
    expect(listLastingEffects).toHaveBeenCalledWith({ campaignId: 'c', encounterId: 'e' });
    expect(await client.listCharacterEffects('c')).toEqual([{ id: 'x' }]);
  });
});
