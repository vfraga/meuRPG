import { TestBed } from '@angular/core/testing';
import { Code, ConnectError } from '@connectrpc/connect';

import { CONNECT_TRANSPORT } from '../connect/transport';
import { SpellCatalog } from './spell-catalog';

describe('SpellCatalog', () => {
  let reads: string[];
  let fail: boolean;

  beforeEach(() => {
    reads = [];
    fail = false;
    // A transport that answers every GetSpellDetails with a spell named after the read's number.
    const transport = {
      unary: async (
        _method: unknown,
        _signal: unknown,
        _timeout: unknown,
        _headers: unknown,
        input: unknown,
      ) => {
        reads.push((input as { spellKey: string }).spellKey);
        if (fail) {
          throw new ConnectError('down', Code.Unavailable);
        }
        return {
          stream: false,
          service: {},
          method: _method,
          header: new Headers(),
          trailer: new Headers(),
          message: { spell: { key: `read-${reads.length}` } },
        };
      },
    };
    TestBed.configureTestingModule({
      providers: [{ provide: CONNECT_TRANSPORT, useValue: transport }],
    });
  });

  it('keeps an SRD spell after the first read', async () => {
    const catalog = TestBed.inject(SpellCatalog);
    await catalog.details('camp-1', 'spell:fireball');
    await catalog.details('camp-1', 'spell:fireball');
    expect(reads).toEqual(['spell:fireball']);
  });

  it('reads a spell of the table each time, because the master can change it while the page is open', async () => {
    const catalog = TestBed.inject(SpellCatalog);
    const first = await catalog.details('camp-1', 'spell:brasa@mesa');
    const second = await catalog.details('camp-1', 'spell:brasa@mesa');
    expect(reads).toEqual(['spell:brasa@mesa', 'spell:brasa@mesa']);
    expect(first).not.toBe(second);
  });

  it('asks again after a failed read', async () => {
    const catalog = TestBed.inject(SpellCatalog);
    fail = true;
    expect(await catalog.details('camp-1', 'spell:fireball')).toBeNull();
    fail = false;
    expect(await catalog.details('camp-1', 'spell:fireball')).not.toBeNull();
    expect(reads).toHaveLength(2);
  });
});
