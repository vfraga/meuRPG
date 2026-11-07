import { TestBed } from '@angular/core/testing';

import { TableScreenKind } from '../../../gen/meurpg/play/v1/table_screen_pb';
import { CONNECT_TRANSPORT } from '../connect/transport';
import { TableScreenClient } from './table-screen-client';

describe('TableScreenClient', () => {
  it('sends the campaign, the revision and the choice, and returns the data', async () => {
    TestBed.configureTestingModule({
      providers: [TableScreenClient, { provide: CONNECT_TRANSPORT, useValue: {} }],
    });
    const client = TestBed.inject(TableScreenClient);
    const sent: unknown[] = [];
    (client as unknown as { client: unknown }).client = {
      getTableScreen: (req: unknown) => (sent.push(req), Promise.resolve({ party: ['Tor'] })),
      setTableScreen: (req: unknown) => (sent.push(req), Promise.resolve({ choiceRevision: 3 })),
      watchTableScreen: (req: unknown, opts: unknown) => (sent.push(req, opts), 'stream'),
    };
    expect(await client.get('c1')).toEqual({ party: ['Tor'] });
    expect(await client.set('c1', 2, { mirror: false, pin: TableScreenKind.MAP })).toEqual({
      choiceRevision: 3,
    });
    const signal = new AbortController().signal;
    expect(client.watch('c1', signal)).toBe('stream');
    expect(sent).toEqual([
      { campaignId: 'c1' },
      { campaignId: 'c1', choiceRevision: 2, choice: { mirror: false, pin: TableScreenKind.MAP } },
      { campaignId: 'c1' },
      { signal },
    ]);
  });
});
