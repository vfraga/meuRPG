import { create } from '@bufbuild/protobuf';
import { Code, ConnectError } from '@connectrpc/connect';

import { ListSpellsResponseSchema, SpellSchema } from '../../../gen/meurpg/rules/v1/rules_pb';
import { spellsErrorMessage } from './spells-errors';
import { SpellsState } from './spells-state';

const spell = (key: string, namePt: string) =>
  create(SpellSchema, { key, namePt, name: namePt, level: 1 });
const page = (keys: string[], total: number, next = '') =>
  create(ListSpellsResponseSchema, {
    spells: keys.map((k) => spell(k, k)),
    total,
    nextPageToken: next,
  });

describe('SpellsState (MR-045)', () => {
  const requests: Record<string, unknown>[] = [];
  let answer: (req: Record<string, unknown>) => Promise<ReturnType<typeof page>>;
  const make = (character: string | null = 'char-1') =>
    new SpellsState(
      {
        list: (req) => {
          requests.push(req as Record<string, unknown>);
          return answer(req as Record<string, unknown>);
        },
      },
      'camp-1',
      () => character,
    );

  beforeEach(() => {
    requests.length = 0;
    answer = async () => page(['a', 'b'], 2);
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
  });
  afterEach(() => vi.useRealTimers());

  it('asks the server for the first page and shows rows and the count', async () => {
    const s = make();
    expect(s.status()).toBe('loading');
    await s.search();
    expect(s.status()).toBe('ready');
    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b']);
    expect(s.total()).toBe(2);
    expect(requests[0]).toMatchObject({ campaignId: 'camp-1', pageToken: '' });
  });

  it('a change of filter asks again from the first page, with the filter in the request', async () => {
    const s = make();
    await s.search();
    await s.change({ classKey: 'class:wizard', onlyMine: true });
    expect(requests[1]).toMatchObject({
      classKey: 'class:wizard',
      characterId: 'char-1',
      pageToken: '',
    });
  });

  it('reads the list again after a content change, keeping the pages "Mostrar mais" had opened, and drops what went away (RN-23)', async () => {
    const s = make();
    answer = async (req) => (req['pageToken'] ? page(['c', 'd'], 4) : page(['a', 'b'], 4, 't2'));
    await s.search();
    await s.more();
    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b', 'c', 'd']);
    // The master switched 'b' off: three spells are left, and they all fit in the first page's worth and the second's.
    answer = async (req) => (req['pageToken'] ? page(['c', 'd'], 3) : page(['a', 'x'], 3, 't2'));
    requests.length = 0;
    await s.refresh();
    expect(requests.map((r) => r['pageToken'])).toEqual(['', 't2']);
    expect(s.spells().map((x) => x.key)).toEqual(['a', 'x', 'c', 'd']);
    expect(s.total()).toBe(3);
  });

  it('keeps the list when the read after a content change fails', async () => {
    const s = make();
    await s.search();
    answer = async () => {
      throw new ConnectError('offline', Code.Unavailable);
    };
    await s.refresh();
    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b']);
    expect(s.status()).toBe('ready');
  });

  it("pages with the server's token and adds the rows under the ones already there", async () => {
    answer = async (req) => (req['pageToken'] ? page(['c'], 3) : page(['a', 'b'], 3, 'next-1'));
    const s = make();
    await s.search();
    expect(s.nextToken()).toBe('next-1');
    await s.more();
    expect(requests[1]).toMatchObject({ pageToken: 'next-1' });
    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b', 'c']);
    expect(s.nextToken()).toBe('');
    // No token, no ask.
    await s.more();
    expect(requests).toHaveLength(2);
  });

  it('keeps the rows and says so when the next page fails, and asks it again', async () => {
    let fail = true;
    answer = async (req) => {
      if (req['pageToken'] && fail) {
        throw new ConnectError('x', Code.Unavailable);
      }
      return req['pageToken'] ? page(['c'], 3) : page(['a', 'b'], 3, 'n');
    };
    const s = make();
    await s.search();
    await s.more();
    expect(s.moreError()).toContain('o servidor não respondeu');
    expect(s.spells()).toHaveLength(2);
    fail = false;
    await s.more();
    expect(s.moreError()).toBe('');
    expect(s.spells()).toHaveLength(3);
  });

  it('waits for a pause in the typing and asks once', async () => {
    const s = make();
    await s.search();
    s.typeQuery('m');
    s.typeQuery('ma');
    s.typeQuery('mao');
    expect(requests).toHaveLength(1);
    expect(s.filter().query).toBe('mao');
    await vi.advanceTimersByTimeAsync(300);
    expect(requests).toHaveLength(2);
    expect(requests[1]).toMatchObject({ query: 'mao' });
  });

  it('drops the answer to an older ask', async () => {
    const resolvers: ((p: ReturnType<typeof page>) => void)[] = [];
    answer = () => new Promise((resolve) => resolvers.push(resolve));
    const s = make();
    const first = s.search();
    const second = s.change({ query: 'novo' });
    resolvers[1](page(['new'], 1));
    await second;
    resolvers[0](page(['old'], 1));
    await first;
    expect(s.spells().map((x) => x.key)).toEqual(['new']);
    expect(s.searching()).toBe(false);
  });

  it('clears the name only, or every filter', async () => {
    const s = make();
    await s.change({ query: 'x', classKey: 'class:wizard' });
    await s.clear(true);
    expect(s.filter()).toMatchObject({ query: '', classKey: 'class:wizard' });
    await s.clear();
    expect(s.filter()).toMatchObject({ query: '', classKey: '' });
  });

  it('reads a basic sheet with "Só as que posso aprender" as a state of its own, not an error', async () => {
    answer = async (req) => {
      if (req['characterId']) {
        throw new ConnectError('x', Code.FailedPrecondition);
      }
      return page(['a'], 1);
    };
    const s = make();
    await s.change({ onlyMine: true });
    expect(s.status()).toBe('basic-sheet');
    expect(s.error()).toBe('');
    await s.change({ onlyMine: false });
    expect(s.status()).toBe('ready');
  });

  it('says what failed by code, with a retry', async () => {
    answer = async () => {
      throw new ConnectError('x', Code.Unavailable);
    };
    const s = make();
    await s.search();
    expect(s.status()).toBe('error');
    expect(s.error()).toBe(
      'Não deu para abrir as magias: o servidor não respondeu. Tente de novo.',
    );
    answer = async () => page(['a'], 1);
    await s.search();
    expect(s.status()).toBe('ready');
  });
});

describe('SpellsState, the next page and the filters (M4)', () => {
  const calls: Record<string, unknown>[] = [];
  const state = (
    answer: (req: Record<string, unknown>) => Promise<ReturnType<typeof page>>,
    hooks = {},
  ) =>
    new SpellsState(
      {
        list: (req) => {
          calls.push(req as Record<string, unknown>);
          return answer(req as Record<string, unknown>);
        },
      },
      'camp-1',
      () => 'char-1',
      hooks,
    );
  beforeEach(() => {
    calls.length = 0;
  });

  it('asks the next page of the list on screen, never with a newer filter and an old token', async () => {
    const s = state(async () => page(['a'], 3, 'tok'));
    await s.change({ classKey: 'class:wizard' });
    // The name is typed (the box shows it) but its ask has not started: the rows are still the wizard's.
    s.filter.update((f) => ({ ...f, query: 'novo' }));
    await s.more();
    expect(calls[1]).toMatchObject({ classKey: 'class:wizard', query: '', pageToken: 'tok' });
  });

  it('does not ask the next page while a search is on its way', async () => {
    let release!: (p: ReturnType<typeof page>) => void;
    let first = true;
    const s = state(() => {
      if (first) {
        first = false;
        return Promise.resolve(page(['a'], 3, 'tok'));
      }
      return new Promise((resolve) => (release = resolve));
    });
    await s.search();
    const running = s.change({ query: 'x' });
    await s.more();
    expect(calls).toHaveLength(2);
    release(page(['b'], 1));
    await running;
  });

  it('clears the filters but keeps the name, for the sheet\'s "Limpar"', async () => {
    const s = state(async () => page(['a'], 1));
    await s.change({
      query: 'maos',
      classKey: 'class:wizard',
      levels: [1],
      schoolKey: 'school:evocation',
      onlyMine: true,
    });
    await s.clearFilters();
    expect(s.filter()).toEqual({
      query: 'maos',
      classKey: '',
      levels: [],
      schoolKey: '',
      onlyMine: false,
    });
  });

  it('tells the page when a search starts and when an answer arrives', async () => {
    const events: string[] = [];
    const s = state(async () => page(['a'], 1), {
      onSearch: () => events.push('search'),
      onAnswered: () => events.push('answered'),
    });
    await s.search();
    expect(events).toEqual(['search', 'answered']);
  });
});

describe('spellsErrorMessage', () => {
  it("maps codes to sentences, never the server's message", () => {
    expect(spellsErrorMessage(new ConnectError('raw secret', Code.NotFound))).toContain(
      'não existe',
    );
    expect(spellsErrorMessage(new ConnectError('raw secret', Code.InvalidArgument))).toContain(
      'confira a busca',
    );
    expect(
      spellsErrorMessage(new ConnectError('raw secret', Code.InvalidArgument), 'read'),
    ).toContain('abrir a descrição');
    expect(spellsErrorMessage(new ConnectError('raw secret', Code.Internal))).not.toContain(
      'raw secret',
    );
  });
});

interface Deferred {
  token: string;
  resolve: (r: ReturnType<typeof page>) => void;
}

describe('SpellsState, refreshes that overlap', () => {
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

  it('a slow multi-page refresh does not overwrite the newer one', async () => {
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

    // The older refresh stops asking for pages once a newer one has been issued.
    await Promise.resolve();
    await Promise.resolve();
    expect(calls.length).toBe(2);
    await r1;

    expect(s.spells().map((x) => x.key)).toEqual(['a', 'b', 'c', 'd']);
    expect(s.total()).toBe(4);
  });

  it('a late page asked before a refresh is not appended to the fresh rows', async () => {
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
