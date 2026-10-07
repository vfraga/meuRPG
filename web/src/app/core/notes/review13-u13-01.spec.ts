// Finding U13-01 (review/unit-13-web-live-rest.md): saving an untouched note edit sends an empty UpdateNote, which the server rejects as invalid_argument.
import { NoteEditing } from './note-editing';
import { FakeNotesClient, note, scene } from './notes-testing';
import { NotesState } from './notes-state';

describe('Review13 U13-01: saving an untouched note edit', () => {
  let api: FakeNotesClient;
  let state: NotesState;
  let editing: NoteEditing;

  beforeEach(async () => {
    api = new FakeNotesClient();
    api.scenesList = [scene('s2', 'A ponte do rio')];
    api.notes = [
      note('n1', 'Brisa me deve 5 PO', new Date(2026, 9, 1, 22, 3), {
        sceneId: 's2',
        sceneName: 'A ponte do rio',
      }),
    ];
    state = new NotesState(api as never, () => 'c1');
    editing = new NoteEditing(state, () => '');
    await state.refresh();
  });

  it('control: a real change is sent', async () => {
    editing.openEdit(state.notes()[0]);
    editing.text.setValue('Brisa me deve 10 PO');
    expect(await editing.save()).toBe(true);
    expect(api.calls).toContain('update n1 {"text":"Brisa me deve 10 PO"}');
  });

  it('never sends an empty update (the server answers "nothing to change") and closes the form', async () => {
    editing.openEdit(state.notes()[0]);
    editing.text.setValue('  Brisa me deve 5 PO  ');
    const saved = await editing.save();
    expect(api.calls.filter((c) => c.startsWith('update n1 {}'))).toEqual([]);
    expect(saved).toBe(true);
    expect(editing.stage()).toBe('list');
  });
});
