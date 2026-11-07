import { TestBed } from '@angular/core/testing';

import { CONNECT_TRANSPORT } from '../connect/transport';
import { CombatClient } from './combat-client';

function fake() {
  const calls: Record<string, Record<string, unknown>[]> = {};
  const answer = (name: string) => (req: Record<string, unknown>) => {
    (calls[name] ??= []).push(req);
    return Promise.resolve({ encounter: { id: 'enc' }, roll: { d20: {} } });
  };
  TestBed.configureTestingModule({
    providers: [CombatClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(CombatClient);
  (client as unknown as { base: { client: unknown } }).base.client = {
    spendLegendaryAction: answer('spend'),
    setMonsterReactionMode: answer('mode'),
    rollAttack: answer('attack'),
    moveCombatant: answer('move'),
    answerReaction: answer('answer'),
    resolveConcentrationSave: answer('concentration'),
  };
  return { client, calls };
}

describe('CombatClient, the legendary calls', () => {
  it('spends the option of a creature, with an idempotency key', async () => {
    const { client, calls } = fake();
    const enc = await client.combatants.spendLegendaryAction('c', 'e', 'dragon', 'detect');
    expect(enc.id).toBe('enc');
    expect(calls['spend'][0]).toMatchObject({
      campaignId: 'c',
      encounterId: 'e',
      combatantId: 'dragon',
      optionKey: 'detect',
    });
    expect(calls['spend'][0]['idempotencyKey']).toEqual(expect.any(String));
  });

  it('sends the legendary option with an attack and with a move, and none otherwise', async () => {
    const { client, calls } = fake();
    await client.attacks.rollAttack(
      'c',
      'e',
      'd',
      'm#tail',
      't',
      { inApp: true },
      'k',
      false,
      '',
      undefined,
      '',
      {
        legendaryOption: 'tail',
      },
    );
    await client.attacks.rollAttack('c', 'e', 'd', 'm#tail', 't', { inApp: true }, 'k2');
    await client.moves.move('c', 'e', 'd', 3, 4, undefined, undefined, false, 'wing');
    await client.moves.move('c', 'e', 'd', 3, 4);
    expect(calls['attack'][0]['legendaryOption']).toBe('tail');
    expect(calls['attack'][1]['legendaryOption']).toBe('');
    expect(calls['move'][0]['legendaryOption']).toBe('wing');
    expect(calls['move'][1]['legendaryOption']).toBe('');
  });

  it('answers a Legendary Resistance window with the ticked creatures, or passes with none', async () => {
    const { client, calls } = fake();
    await client.attacks.answerLegendaryResistance(
      'c',
      'e',
      'w',
      { creatureIds: ['d', 'v'] },
      'k1',
    );
    await client.attacks.answerLegendaryResistance('c', 'e', 'w', { creatureIds: [] }, 'k2');
    expect(calls['answer'][0]).toMatchObject({
      answer: 1,
      creatureIds: ['d', 'v'],
      idempotencyKey: 'k1',
    });
    expect(calls['answer'][1]).toMatchObject({ answer: 2, creatureIds: [] });
  });

  it('answers a Parry window: Aparar uses it, Deixar passar passes', async () => {
    const { client, calls } = fake();
    await client.attacks.answerParry('c', 'e', 'w', true, 'k1');
    await client.attacks.answerParry('c', 'e', 'w', false, 'k2');
    expect(calls['answer'].map((r) => r['answer'])).toEqual([1, 2]);
  });

  it('asks a concentration save to use Legendary Resistance only when told to', async () => {
    const { client, calls } = fake();
    await client.attacks.resolveConcentrationSave(
      'c',
      'e',
      'w',
      { kind: 'app', useLegendaryResistance: true },
      'k',
    );
    await client.attacks.resolveConcentrationSave('c', 'e', 'w', { kind: 'app' }, 'k');
    expect(calls['concentration'].map((r) => r['useLegendaryResistance'])).toEqual([true, false]);
  });

  it('sets the Parry switch of a creature: asking or deciding alone, with an idempotency key', async () => {
    const { client, calls } = fake();
    const enc = await client.combatants.setMonsterReactionMode('c', 'e', 'cap', true);
    await client.combatants.setMonsterReactionMode('c', 'e', 'cap', false);
    expect(enc.id).toBe('enc');
    expect(calls['mode'][0]).toMatchObject({
      campaignId: 'c',
      encounterId: 'e',
      combatantId: 'cap',
      ask: true,
    });
    expect(calls['mode'][1]).toMatchObject({ ask: false });
    expect(calls['mode'][0]['idempotencyKey']).toEqual(expect.any(String));
  });

  it('resends the same key when the caller keeps it across a retry', async () => {
    const { client, calls } = fake();
    await client.combatants.setMonsterReactionMode('c', 'e', 'cap', true, 'k1');
    await client.combatants.setMonsterReactionMode('c', 'e', 'cap', true, 'k1');
    expect(calls['mode'].map((r) => r['idempotencyKey'])).toEqual(['k1', 'k1']);
  });
});
