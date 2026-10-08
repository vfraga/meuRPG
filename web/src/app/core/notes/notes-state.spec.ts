import { Code, ConnectError } from '@connectrpc/connect';

import { FakeNotesClient, note, scene } from './notes-testing';
import { NotesState } from './notes-state';

const AT = (min: number) => new Date(2026, 9, 3, 21, min);

describe('NotesState', () => {
  let api: FakeNotesClient;
  let state: NotesState;

  beforeEach(() => {
    api = new FakeNotesClient();
    api.scenesList = [scene('s1', 'A carroça tombada')];
    api.notes = [note('n1', 'Brisa me deve 5 PO', AT(3)), note('n2', 'Comprar tinta', AT(1))];
    state = new NotesState(api as never, () => 'c1');
  });

  it('reads the list and the discovered scenes, newest first, and says nothing the first time', async () => {
    await state.refresh(true);
    expect(state.notes().map((n) => n.id)).toEqual(['n1', 'n2']);
    expect(state.scenes().map((s) => s.name)).toEqual(['A carroça tombada']);
    expect(state.noteCount()).toBe(2);
    expect(state.loaded()).toBe(true);
    expect(state.fresh()).toEqual([]);
    expect(state.notice()).toBe(false);
  });

  it('counts a clue that arrives through notes_changed as new and raises the notice', async () => {
    await state.refresh();
    api.notes = [
      note('c1', 'Um brasão de lobo', AT(20), {
        clue: true,
        sceneId: 's1',
        sceneName: 'A carroça tombada',
      }),
      ...api.notes,
    ];
    await state.refresh(true);
    expect(state.fresh().map((n) => n.id)).toEqual(['c1']);
    expect(state.notice()).toBe(true);
    // A clue is in the list but is not one of the 300 notes.
    expect(state.noteCount()).toBe(2);
    expect(state.notes()[0].id).toBe('c1');
  });

  it('does not count again what it already had, nor a note, nor the clues of a read that is not an announcement', async () => {
    await state.refresh();
    api.notes = [
      note('c1', 'Pista', AT(20), { clue: true }),
      note('n3', 'Outra', AT(21)),
      ...api.notes,
    ];
    await state.refresh(false);
    expect(state.fresh()).toEqual([]);
    await state.refresh(true);
    // c1 was already read silently: only clues never seen count.
    expect(state.fresh()).toEqual([]);
    api.notes = [note('c2', 'Mais uma', AT(25), { clue: true }), ...api.notes];
    await state.refresh(true);
    expect(state.fresh().map((n) => n.id)).toEqual(['c2']);
  });

  it('keeps the news until the notes are opened, and the notice until it is dismissed or opened', async () => {
    await state.refresh();
    api.notes = [note('c1', 'Pista', AT(20), { clue: true }), ...api.notes];
    await state.refresh(true);
    state.dismissNotice();
    expect(state.notice()).toBe(false);
    expect(state.fresh()).toHaveLength(1);
    state.seen();
    expect(state.fresh()).toEqual([]);
    expect(state.notice()).toBe(false);
  });

  it('keeps the copy on screen when a read fails, and says so', async () => {
    await state.refresh();
    api.failWith = new ConnectError('x', Code.Unavailable);
    await state.refresh(true);
    expect(state.notes()).toHaveLength(2);
    expect(state.failed()).toBe(true);
    await state.refresh();
    expect(state.failed()).toBe(false);
  });

  it('never lets an older read overwrite a newer one', async () => {
    await state.refresh();
    let release!: () => void;
    const slow = new Promise<void>((r) => (release = r));
    const original = api.list.bind(api);
    api.list = async (c: string) => {
      const result = await original(c);
      await slow;
      return result;
    };
    const stale = state.refresh();
    await Promise.resolve();
    await state.create('Nova', '');
    release();
    await stale;
    expect(state.notes().map((n) => n.text)).toContain('Nova');
  });

  it('retries a note that failed with the same idempotency key, and uses a new one for the next note', async () => {
    await state.refresh();
    api.failNextCreate = true;
    await expect(state.create('Nova', 's1')).rejects.toThrow();
    await state.create('Nova', 's1'); // the retry of the same note
    await state.create('Nova', 's1'); // the same text again, after it worked: another note
    const [lost, retry, next] = api.createKeys;
    expect(retry).toBe(lost);
    expect(next).not.toBe(retry);
  });

  it('puts a new note first, counts it, and moves an edited one to the top', async () => {
    await state.refresh();
    const created = await state.create('Nova anotação', 's1');
    expect(created?.sceneName).toBe('A carroça tombada');
    expect(state.notes()[0].text).toBe('Nova anotação');
    expect(state.noteCount()).toBe(3);
    api.now = AT(40);
    await state.update('n2', { text: 'Comprar tinta e pergaminho' });
    expect(state.notes()[0].id).toBe('n2');
    expect(state.noteCount()).toBe(3);
  });

  it('removes a note and counts it out', async () => {
    await state.refresh();
    expect(await state.remove('n1')).toBe(true);
    expect(state.notes().map((n) => n.id)).toEqual(['n2']);
    expect(state.noteCount()).toBe(1);
  });

  it('allows one write at a time per note', async () => {
    await state.refresh();
    const first = state.update('n1', { text: 'A' });
    expect(state.isWriting('n1')).toBe(true);
    expect(await state.update('n1', { text: 'B' })).toBeNull();
    await first;
    expect(state.isWriting('n1')).toBe(false);
    expect(api.calls.filter((c) => c.startsWith('update'))).toHaveLength(1);
    // Another note is not blocked.
    const both = await Promise.all([
      state.update('n1', { text: 'C' }),
      state.update('n2', { text: 'D' }),
    ]);
    expect(both.every((n) => n !== null)).toBe(true);
  });

  it('is at the limit when the player has 300 notes', async () => {
    api.maxNotes = 2;
    await state.refresh();
    expect(state.atLimit()).toBe(true);
  });

  it('forgets everything for a new page', async () => {
    await state.refresh();
    state.clear();
    expect(state.notes()).toEqual([]);
    expect(state.loaded()).toBe(false);
  });

  describe('a refresh in flight when the own write answers', () => {
    function deferred<T>() {
      let resolve!: (v: T) => void;
      const promise = new Promise<T>((r) => (resolve = r));
      return { promise, resolve };
    }

    it('still applies what the refresh carries (a clue), by reading again, and announces it once', async () => {
      const clue = note('c1', 'Um brasão', AT(20), { clue: true });
      const lists = [deferred<never>(), deferred<never>()];
      const update = deferred<ReturnType<typeof note>>();
      let calls = 0;
      const gated = {
        list: () => (++calls === 1 ? api.list('c1') : lists[calls - 2].promise),
        scenes: () => api.scenes('c1'),
        create: () => Promise.reject(new Error('unused')),
        update: () => update.promise,
        delete: () => Promise.resolve(),
      };
      const own = new NotesState(gated as never, () => 'c1');
      await own.refresh();

      const refresh = own.refresh(true); // notes_changed: in flight
      const writing = own.update('n1', { text: 'Nova' }); // the player's own write in flight
      update.resolve(note('n1', 'Nova', AT(30)));
      await writing;
      const all = { notes: [note('n1', 'Antiga', AT(3)), clue], noteCount: 1, maxNotes: 300 };
      lists[0].resolve(all as never); // served before the write
      await Promise.resolve();
      expect(own.notes().map((n) => n.text)).not.toContain('Antiga');
      lists[1].resolve({ ...all, notes: [note('n1', 'Nova', AT(30)), clue] } as never);
      await refresh;

      expect(own.notes().map((n) => n.id)).toEqual(['n1', 'c1']);
      expect(own.notes().find((n) => n.id === 'n1')?.text).toBe('Nova');
      expect(own.fresh().map((n) => n.id)).toEqual(['c1']);
      expect(own.notice()).toBe(true);
    });
  });

  describe('a write that answers after the campaign changed', () => {
    it('leaves the list and the count of the new campaign alone', async () => {
      let campaign = 'A';
      let answer!: (n: ReturnType<typeof note>) => void;
      const slow = {
        list: (id: string) =>
          Promise.resolve({
            notes: id === 'B' ? [note('b1', 'De B', AT(1))] : [],
            noteCount: id === 'B' ? 1 : 0,
            maxNotes: 300,
          }),
        scenes: () => Promise.resolve([]),
        create: () => new Promise<ReturnType<typeof note>>((r) => (answer = r)),
        update: () => Promise.reject(new Error('unused')),
        delete: () => Promise.resolve(),
      };
      const own = new NotesState(slow as never, () => campaign);
      await own.refresh();

      const creating = own.create('Segredo de A', '');
      campaign = 'B';
      own.clear();
      await own.refresh();
      answer(note('a1', 'Segredo de A', AT(40)));
      await creating;

      expect(own.notes().map((n) => n.id)).toEqual(['b1']);
      expect(own.noteCount()).toBe(1);
      expect(own.isWriting('new')).toBe(false);
    });

    it('does not free the mark of a write that began in the new campaign', async () => {
      let campaign = 'A';
      const answers: ((n: ReturnType<typeof note>) => void)[] = [];
      const slow = {
        list: () => Promise.resolve({ notes: [], noteCount: 0, maxNotes: 300 }),
        scenes: () => Promise.resolve([]),
        create: () => new Promise<ReturnType<typeof note>>((r) => answers.push(r)),
        update: () => Promise.reject(new Error('unused')),
        delete: () => Promise.resolve(),
      };
      const own = new NotesState(slow as never, () => campaign);
      const first = own.create('De A', '');
      campaign = 'B';
      own.clear();
      const second = own.create('De B', '');
      answers[0](note('a1', 'De A', AT(1)));
      await first;
      expect(own.isWriting('new')).toBe(true);
      answers[1](note('b1', 'De B', AT(2)));
      await second;
      expect(own.notes().map((n) => n.id)).toEqual(['b1']);
      expect(own.isWriting('new')).toBe(false);
    });
  });

  it('sends no update that changes neither the text nor the tag, and returns the note as it is', async () => {
    await state.refresh(true);
    const calls = api.calls.length;
    const same = await state.update('n1', {});
    expect(same?.id).toBe('n1');
    expect(api.calls).toHaveLength(calls);
  });
});
