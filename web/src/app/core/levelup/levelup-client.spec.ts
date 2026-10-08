import { TestBed } from '@angular/core/testing';
import { create } from '@bufbuild/protobuf';

import { SpellDetailsSchema } from '../../../gen/meurpg/rules/v1/rules_pb';

import { CONNECT_TRANSPORT } from '../connect/transport';
import { LevelUpClient } from './levelup-client';

// The level-up page's catalog (10.12b fix round 1): read with the character, so a sheet keeps the entries the master
// retired, and never kept past the page: a client is the page's own, and `fresh` reads again inside it.

function clientWith(calls: object[]): LevelUpClient {
  TestBed.configureTestingModule({
    providers: [LevelUpClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
  });
  const client = TestBed.inject(LevelUpClient);
  let revision = 0;
  (client as unknown as { content: unknown }).content = {
    listContent: (req: object) => {
      calls.push(req);
      revision += 1;
      return Promise.resolve({
        tableRevision: revision,
        content: { spells: [], skills: [], classes: [{ key: 'class:wizard', namePt: 'Mago' }] },
      });
    },
  };
  return client;
}

describe('LevelUpClient.catalog', () => {
  it('asks with the character, and keeps the answer for the page', async () => {
    const calls: object[] = [];
    const client = clientWith(calls);
    const first = await client.catalog('camp', 'char');
    await client.catalog('camp', 'char');
    expect(calls).toEqual([{ campaignId: 'camp', characterId: 'char' }]);
    expect(first.classes).toEqual([{ key: 'class:wizard', namePt: 'Mago' }]);
    expect(first.revision).toBe(1);
  });

  it('reads again when asked fresh (a content_changed hint, a stale sheet), and says which revision it saw', async () => {
    const calls: object[] = [];
    const client = clientWith(calls);
    await client.catalog('camp', 'char');
    const again = await client.catalog('camp', 'char', true);
    expect(calls).toHaveLength(2);
    expect(again.revision).toBe(2);
    // The fresh answer is what the next ask gets.
    expect((await client.catalog('camp', 'char')).revision).toBe(2);
  });

  it('keeps one catalog per character: another sheet has its own entries', async () => {
    const calls: object[] = [];
    const client = clientWith(calls);
    await client.catalog('camp', 'a');
    await client.catalog('camp', 'b');
    expect(calls).toHaveLength(2);
  });
});

describe('LevelUpClient.spellDetails', () => {
  function withSpells(reads: string[]): LevelUpClient {
    const client = clientWith([]);
    (client as unknown as { content: unknown }).content = {
      getSpellDetails: (req: { spellKey: string }) => {
        reads.push(req.spellKey);
        return Promise.resolve({ spell: create(SpellDetailsSchema) });
      },
    };
    return client;
  }

  it('keeps an SRD spell for the page, and reads a spell of the table each time', async () => {
    const reads: string[] = [];
    const client = withSpells(reads);
    await client.spellDetails('camp', 'spell:fireball');
    await client.spellDetails('camp', 'spell:fireball');
    await client.spellDetails('camp', 'spell:brasa@mesa');
    await client.spellDetails('camp', 'spell:brasa@mesa');
    expect(reads).toEqual(['spell:fireball', 'spell:brasa@mesa', 'spell:brasa@mesa']);
  });
});
