// Finding U11-12 / U11-13: SpellsState.refresh() overlapping refresh() (U11-12) and more() racing refresh() (U11-13)
// let an older answer overwrite a newer one (refresh does not bump `seq`; its guards never change between two refreshes).
import { create } from '@bufbuild/protobuf';

import { ListSpellsResponseSchema, SpellSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { SpellsState } from './spells-state';

const spell = (key: string) => create(SpellSchema, { key, namePt: key, name: key, level: 1 });
const page = (keys: string[], total: number, next = '') =>
  create(ListSpellsResponseSchema, { spells: keys.map(spell), total, nextPageToken: next });

interface Deferred {
  token: string;
  resolve: (r: ReturnType<typeof page>) => void;
}

describe('Review11 U11-12: overlapping refresh() lets the older answer win', () => {
  const calls: Deferred[] = [];
  let initial = true;
  const make = () =>
    new SpellsState(
      {
        list: (req) => {
          if (initial) {
            return Promise.resolve(page(['a', 'b', 'c'], 3));
          }
          return new Promise((resolve) =>
            calls.push({ token: (req as { pageToken: string }).pageToken, resolve }),
          );
        },
      },
      'camp-1',
      () => null,
    );

  beforeEach(() => {
    calls.length = 0;
    initial = true;
  });

  it('U11-12: slow multi-page refresh 1 must not overwrite the newer refresh 2', async () => {
    const s = make();
    await s.search();
    initial = false;

    const r1 = s.refresh(); // wants 3 rows: first page has 2 + token, needs a second page
    const r2 = s.refresh(); // newer hint: one page is enough
    expect(calls.length).toBe(2);
    calls[0].resolve(page(['a', 'b'], 3, 'p2')); // refresh 1 page 1 (old content)
    calls[1].resolve(page(['a', 'b', 'c', 'd'], 4)); // refresh 2 (new content) finishes first
    await r2;
    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b', 'c', 'd']);

    await Promise.resolve();
    await Promise.resolve();
    expect(calls.length).toBe(3);
    calls[2].resolve(page(['c'], 3)); // refresh 1 page 2 (old content)
    await r1;

    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b', 'c', 'd']);
    expect(s.total()).toBe(4);
  });

  it('U11-13: a late more() must not append its old page to the fresh rows nor overwrite nextToken', async () => {
    initial = false;
    const s = new SpellsState(
      {
        list: (req) => {
          const token = (req as { pageToken: string }).pageToken;
          if (calls.length === 0 && token === '' && !s.spells().length) {
            return Promise.resolve(page(['a', 'b'], 4, 'T1'));
          }
          return new Promise((resolve) => calls.push({ token, resolve }));
        },
      },
      'camp-1',
      () => null,
    );
    await s.search();
    expect(s.nextToken()).toBe('T1');

    const m = s.more(); // old-version page asked with T1
    const r = s.refresh(); // content_changed hint
    expect(calls.map((c) => c.token)).toEqual(['T1', '']);
    calls[1].resolve(page(['x', 'a', 'b'], 5, 'T2')); // refresh finishes first
    await r;
    expect(s.spells().map((k) => k.key)).toEqual(['x', 'a', 'b']);
    expect(s.nextToken()).toBe('T2');

    calls[0].resolve(page(['c', 'd'], 4)); // late old page
    await m;

    expect(s.spells().map((k) => k.key)).toEqual(['x', 'a', 'b']);
    expect(s.nextToken()).toBe('T2');
    expect(s.total()).toBe(5);
  });
});
