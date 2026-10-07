// Findings U11-17 and U11-19 (review 11): NotesState write closures bump `generation`,
// discarding an in-flight refresh that carries a new clue (U11-17), and mutate the list
// after `await` with no campaign guard, so a write from campaign A lands in B (U11-19).
import type { Note } from '../../../gen/meurpg/notes/v1/notes_pb';
import { NotesState } from './notes-state';
import { note } from './notes-testing';

const AT = (min: number) => new Date(2026, 9, 3, 21, min);

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

describe('Review11 U11-17: own write invalidates the in-flight notes_changed refresh', () => {
  it('still applies the refresh that carries a new clue after the own update resolves', async () => {
    const n1 = note('n1', 'Antiga', AT(1));
    const clue = note('c1', 'Um brasão', AT(20), { clue: true });
    const listD = deferred<{ notes: Note[]; noteCount: number; maxNotes: number }>();
    const updD = deferred<Note>();
    let listCalls = 0;
    const api = {
      list: () =>
        ++listCalls === 1
          ? Promise.resolve({ notes: [n1], noteCount: 1, maxNotes: 300 })
          : listD.promise,
      scenes: () => Promise.resolve([]),
      create: () => Promise.reject(new Error('unused')),
      update: () => updD.promise,
      delete: () => Promise.resolve(),
    };
    const state = new NotesState(api as never, () => 'c1');
    await state.refresh();

    const refresh = state.refresh(true); // notes_changed: in flight
    const update = state.update('n1', { text: 'Nova' }); // player's own write in flight
    updD.resolve(note('n1', 'Nova', AT(30)));
    await update;
    listD.resolve({ notes: [n1, clue], noteCount: 1, maxNotes: 300 });
    await refresh;

    expect(state.notes().map((n) => n.id)).toContain('c1');
    expect(state.fresh().map((n) => n.id)).toEqual(['c1']);
    expect(state.notice()).toBe(true);
  });
});

describe('Review11 U11-19: write from campaign A resolving after the switch to B', () => {
  it('does not insert the note of A into the list of B', async () => {
    let campaign = 'A';
    const createD = deferred<Note>();
    const api = {
      list: (id: string) =>
        Promise.resolve({
          notes: id === 'B' ? [note('b1', 'De B', AT(1))] : [],
          noteCount: id === 'B' ? 1 : 0,
          maxNotes: 300,
        }),
      scenes: () => Promise.resolve([]),
      create: () => createD.promise,
      update: () => Promise.reject(new Error('unused')),
      delete: () => Promise.resolve(),
    };
    const state = new NotesState(api as never, () => campaign);
    await state.refresh();

    const creating = state.create('Segredo de A', '');
    campaign = 'B';
    state.clear();
    await state.refresh();
    expect(state.notes().map((n) => n.id)).toEqual(['b1']);

    createD.resolve(note('a1', 'Segredo de A', AT(40)));
    await creating;

    expect(state.notes().map((n) => n.id)).toEqual(['b1']);
    expect(state.noteCount()).toBe(1);
  });
});
